package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/provider"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
	"github.com/oaovito/mne_lab/internal/vault"
)

// lifecycleAPI supplies a deterministic remote API contract without cloud
// credentials. Production folder transports retain their own identity.
type lifecycleAPI struct{ provider.Provider }

func (lifecycleAPI) Transport() string { return provider.TransportAPI }

func lifecycleApp(t *testing.T, mode paths.Mode, base string) *App {
	t.Helper()
	opt := Options{ForceMode: mode, Headless: true, KDF: fastKDF, UserHome: t.TempDir()}
	if mode == paths.Portable {
		opt.Root = t.TempDir()
		if err := os.WriteFile(filepath.Join(opt.Root, paths.PortableMarker), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		if base == "" {
			base = t.TempDir()
		}
		opt.BaseTemp = base
	}
	a, err := New(opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}

func lifecycleProfile(t *testing.T, a *App) (*Profile, *provider.Folder) {
	t.Helper()
	if _, _, err := a.CreateAccount("Lab", "account passphrase", false); err != nil {
		t.Fatal(err)
	}
	entry, err := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: vault.StorageCloudOnly, Provider: provider.Google}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.OpenProfile(entry.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	// Explicit Sync/Exit own synchronization in these deterministic tests.
	close(p.stop)
	<-p.stopped
	cloud := provider.NewFolder(provider.Google, t.TempDir())
	p.setProvider(lifecycleAPI{cloud})
	return p, cloud
}

func TestProfileVerificationRequiresSuccessfulCurrentAPISync(t *testing.T) {
	key := secure.NewKey()
	st, err := store.Open(filepath.Join(t.TempDir(), "profile.db"), key, store.Options{Queue: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	folder := provider.NewFolder(provider.Google, t.TempDir())
	p := &Profile{St: st, Entry: vault.ProfileEntry{StorageMode: vault.StorageCloudOnly}, prov: lifecycleAPI{folder}}
	for _, tc := range []struct {
		name     string
		status   SyncStatus
		prov     provider.Provider
		verified bool
	}{
		{"no successful sync", SyncStatus{State: SyncSynced}, lifecycleAPI{folder}, false},
		{"API success", SyncStatus{State: SyncSynced, LastSync: time.Now()}, lifecycleAPI{folder}, true},
		{"latest integrity error", SyncStatus{State: SyncSynced, LastSync: time.Now(), LastError: "integrity failure"}, lifecycleAPI{folder}, false},
		{"sync in progress", SyncStatus{State: SyncSyncing, LastSync: time.Now()}, lifecycleAPI{folder}, false},
		{"folder handoff", SyncStatus{State: SyncSynced, LastSync: time.Now()}, folder, false},
		{"disconnected", SyncStatus{State: SyncSynced, LastSync: time.Now()}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p.status, p.prov = tc.status, tc.prov
			if got := p.Verified(); got != tc.verified {
				t.Fatalf("Verified()=%v, want %v", got, tc.verified)
			}
		})
	}
}

func TestCorruptAcknowledgedBlobRetainsOnlyLocalCopyOnExit(t *testing.T) {
	for _, mode := range []paths.Mode{paths.Portable, paths.Temporary} {
		t.Run(string(mode), func(t *testing.T) {
			a := lifecycleApp(t, mode, "")
			p, cloud := lifecycleProfile(t, a)
			key, accountID := p.key, p.acct.ID()
			original := []byte("only intact copy of the original experiment")
			var blobID string
			if err := p.St.Update(func(tx *store.Tx) error {
				var err error
				blobID, err = tx.PutBlob(original)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := p.Sync(context.Background()); err != nil || !p.Verified() {
				t.Fatalf("initial API sync: %v %+v", err, p.Status())
			}
			name := filepath.Join(cloud.Root(), "accounts", accountID, "profiles", p.Entry.ID, "b", blobID+".blob")
			sealed, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			sealed[len(sealed)-1] ^= 1
			if err := os.WriteFile(name, sealed, 0o600); err != nil {
				t.Fatal(err)
			}
			stats, err := p.St.Stats()
			if err != nil || stats.Pending+stats.Retryable != 0 {
				t.Fatalf("expected already acknowledged queue: %+v %v", stats, err)
			}
			report := a.Exit()
			if report.Verified || (mode == paths.Temporary && !report.Recovery) {
				t.Fatalf("integrity failure must preserve local data: %+v", report)
			}
			db := p.dbPath()
			if mode == paths.Temporary {
				db = filepath.Join(a.recoveryDir(accountID), "profiles", p.Entry.ID, "profile.db")
			}
			kept, err := store.Open(db, key, store.Options{})
			if err != nil {
				t.Fatal(err)
			}
			defer kept.Close()
			if err := kept.View(func(tx *store.Tx) error {
				got, err := tx.GetBlob(blobID)
				if err == nil && !bytes.Equal(got, original) {
					t.Fatal("intact local blob changed")
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestVerifiedAPIExitCleansTemporarySession(t *testing.T) {
	a := lifecycleApp(t, paths.Temporary, "")
	p, cloud := lifecycleProfile(t, a)
	if err := p.St.Update(func(tx *store.Tx) error {
		_, err := tx.Put("experiment", "saved", map[string]any{"result": 42})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	accountID := p.acct.ID()
	if report := a.Exit(); !report.Verified || !report.Cleaned || report.Recovery {
		t.Fatalf("confirmed remote API copy should permit cleanup: %+v", report)
	}
	if _, err := os.Stat(a.L.Root); !os.IsNotExist(err) {
		t.Fatalf("verified temporary session remains: %v", err)
	}
	if _, err := cloud.Read(context.Background(), "accounts/"+accountID+"/index.enc"); err != nil {
		t.Fatalf("remote account was not published: %v", err)
	}
	if len(a.recoveryPackages(accountID)) != 0 {
		t.Fatal("verified current work unnecessarily retained")
	}
}

func TestClosedProfileRemoteCorruptionKeepsTemporaryRecovery(t *testing.T) {
	a := lifecycleApp(t, paths.Temporary, "")
	p, cloud := lifecycleProfile(t, a)
	key, accountID := p.key, p.acct.ID()
	original := []byte("synthetic only intact blob from a closed profile")
	var blobID string
	if err := p.St.Update(func(tx *store.Tx) error {
		var err error
		blobID, err = tx.PutBlob(original)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.CloseProfile(); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(cloud.Root(), "accounts", accountID, "profiles", p.Entry.ID, "b", blobID+".blob")
	sealed, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	sealed[len(sealed)-1] ^= 1
	if err := os.WriteFile(name, sealed, 0o600); err != nil {
		t.Fatal(err)
	}
	if report := a.Exit(); report.Verified || !report.Recovery || !report.Cleaned {
		t.Fatalf("historical marker cannot authorize cleanup: %+v", report)
	}
	kept, err := store.Open(filepath.Join(a.recoveryDir(accountID), "profiles", p.Entry.ID, "profile.db"), key, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer kept.Close()
	if err := kept.View(func(tx *store.Tx) error {
		got, err := tx.GetBlob(blobID)
		if err == nil && !bytes.Equal(got, original) {
			t.Fatal("intact closed-profile data changed")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMultipleSyncedProfilesKeepTemporaryRecovery(t *testing.T) {
	a := lifecycleApp(t, paths.Temporary, "")
	first, cloud := lifecycleProfile(t, a)
	if _, err := first.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	entry, err := a.CreateProfile(ProfileInput{Username: "Bia", StorageMode: vault.StorageCloudOnly, Provider: provider.Google}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.OpenProfile(entry.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	close(second.stop)
	<-second.stopped
	second.setProvider(lifecycleAPI{cloud})
	if _, err := second.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	if pending := a.pendingProfiles(second.acct); len(pending) != 0 {
		t.Fatalf("test requires valid historical markers: %v", pending)
	}
	accountID := second.acct.ID()
	if report := a.Exit(); report.Verified || !report.Recovery || !report.Cleaned {
		t.Fatalf("all profile keys must be available for fresh verification: %+v", report)
	}
	for _, id := range []string{first.Entry.ID, second.Entry.ID} {
		if _, err := os.Stat(filepath.Join(a.recoveryDir(accountID), "profiles", id, "profile.db")); err != nil {
			t.Fatalf("profile %s not preserved: %v", id, err)
		}
	}
}

func TestBusyWaitRefusesExitAndRestartWithoutTeardown(t *testing.T) {
	for _, operation := range []string{"exit", "restart"} {
		t.Run(operation, func(t *testing.T) {
			a := lifecycleApp(t, paths.Temporary, "")
			p, _ := lifecycleProfile(t, a)
			done := a.begin("import")
			t.Cleanup(done)
			if a.waitIdle(0) {
				t.Fatal("expired wait treated active work as finished")
			}
			// Cancellation exercises the same refusal path without sleeping
			// through the thirty-second production deadline.
			a.cancel()
			if operation == "exit" {
				report := a.Exit()
				if report.Cleaned || len(report.Steps) != 1 || report.Steps[0].Detail != "app.operations_running" {
					t.Fatalf("active work did not block exit: %+v", report)
				}
			} else {
				exe, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				if err := a.Restart(exe, ""); err == nil || err.Error() != "app.operations_running" {
					t.Fatalf("active work did not block restart: %v", err)
				}
			}
			if a.exiting.Load() {
				t.Fatal("refused lifecycle operation is not retryable")
			}
			if current, err := a.Profile(); err != nil || current != p {
				t.Fatalf("active profile changed: %v", err)
			}
			if _, err := p.St.Stats(); err != nil {
				t.Fatalf("active store closed: %v", err)
			}
			if _, err := os.Stat(p.dbPath()); err != nil {
				t.Fatalf("session data removed: %v", err)
			}
			if _, err := os.Stat(p.backupDir()); !os.IsNotExist(err) {
				t.Fatalf("backup started before critical work finished: %v", err)
			}
			select {
			case <-a.Done():
				t.Fatal("refused lifecycle operation finished the application")
			default:
			}
		})
	}
}

func TestVerifiedExitKeepsUnrestoredSameAccountRecovery(t *testing.T) {
	a := lifecycleApp(t, paths.Temporary, "")
	p, _ := lifecycleProfile(t, a)
	put := func(value string) {
		t.Helper()
		if err := p.St.Update(func(tx *store.Tx) error {
			_, err := tx.Put("experiment", "saved", map[string]any{"result": value})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	put("older unsynchronized experiment")
	if err := a.saveRecovery(p.acct); err != nil {
		t.Fatal(err)
	}
	accountID, key := p.acct.ID(), p.key
	put("current experiment")
	if report := a.Exit(); !report.Verified || !report.Cleaned {
		t.Fatalf("current work was not verified: %+v", report)
	}
	kept, err := store.Open(filepath.Join(a.recoveryDir(accountID), "profiles", p.Entry.ID, "profile.db"), key, store.Options{})
	if err != nil {
		t.Fatalf("unrestored recovery was discarded: %v", err)
	}
	defer kept.Close()
	if err := kept.View(func(tx *store.Tx) error {
		var value struct{ Result string }
		_, err := tx.Get("experiment", "saved", &value)
		if err == nil && value.Result != "older unsynchronized experiment" {
			t.Fatalf("earlier work changed: %+v", value)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTemporaryExitRetainsSignedOutAccountsAndUnopenedProfiles(t *testing.T) {
	a := lifecycleApp(t, paths.Temporary, "")
	var ids []string
	for _, name := range []string{"first", "second"} {
		if _, view, err := a.CreateAccount(name, "account passphrase", false); err != nil {
			t.Fatal(err)
		} else {
			ids = append(ids, view.ID)
		}
		for _, username := range []string{"Ana", "Bia"} {
			if _, err := a.CreateProfile(ProfileInput{Username: username, StorageMode: vault.StorageCloudOnly, Provider: provider.Google}, nil); err != nil {
				t.Fatal(err)
			}
		}
		if err := a.ChangeAccount(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if report := a.Exit(); report.Verified || !report.Recovery || !report.Cleaned {
		t.Fatalf("signed-out work must survive exit: %+v", report)
	}
	recoveryVault := vault.New(a.L.Recovery)
	for _, id := range ids {
		acct, err := recoveryVault.Unlock(id, "account passphrase")
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range acct.Profiles() {
			if _, err := acct.UnlockProfile(entry.ID, ""); err != nil {
				t.Fatalf("unopened profile key not preserved: %v", err)
			}
		}
		if len(acct.Profiles()) != 2 {
			t.Fatalf("profiles not preserved: %+v", acct.Profiles())
		}
		acct.Lock()
	}
}

func TestRecoveryRetainsAndRestoresEarlierGeneration(t *testing.T) {
	base := t.TempDir()
	a := lifecycleApp(t, paths.Temporary, base)
	if _, _, err := a.CreateAccount("Lab", "account passphrase", false); err != nil {
		t.Fatal(err)
	}
	id, source := a.acct.ID(), filepath.Join(a.acct.Dir(), "only-copy.enc")
	for _, contents := range []string{"older unsynchronized work", "new unsynchronized work"} {
		if err := os.WriteFile(source, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := a.saveRecovery(a.acct); err != nil {
			t.Fatal(err)
		}
	}
	if packages := a.recoveryPackages(id); len(packages) != 2 {
		t.Fatalf("older recovery lost: %v", packages)
	}
	a.Close()
	b := lifecycleApp(t, paths.Temporary, base)
	if len(b.Recoveries()) != 2 {
		t.Fatalf("retained generations not discoverable: %+v", b.Recoveries())
	}
	if err := b.RestoreRecovery(id); err != nil {
		t.Fatal(err)
	}
	read := func(want string) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(b.Vault.Dir, id, "only-copy.enc"))
		if err != nil || string(got) != want {
			t.Fatalf("restored %q, want %q: %v", got, want, err)
		}
	}
	read("new unsynchronized work")
	// Simulate consumption of the newest package while retaining its bytes
	// outside recovery, then prove the older package remains restorable.
	if err := os.Rename(b.recoveryDir(id), filepath.Join(t.TempDir(), "consumed")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(b.Vault.Dir, id)); err != nil {
		t.Fatal(err)
	}
	if err := b.RestoreRecovery(id); err != nil {
		t.Fatal(err)
	}
	read("older unsynchronized work")
}

func TestDiscardRequiresVerifiedExactRestoreAndKeepsOtherAccounts(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged source", true: "new source work"}[changed], func(t *testing.T) {
			base := t.TempDir()
			stale := filepath.Join(base, paths.SessionPrefix+"interrupted")
			v := vault.New(filepath.Join(stale, "data", "accounts"))
			v.KDF = fastKDF
			acct, _, err := v.Create("Lab", "account passphrase", false)
			if err != nil {
				t.Fatal(err)
			}
			entry, err := acct.CreateProfile(vault.ProfileInput{Username: "Ana", StorageMode: vault.StorageCloudOnly, Provider: provider.Google})
			if err != nil {
				t.Fatal(err)
			}
			other, _, err := v.Create("Other", "other passphrase", false)
			if err != nil {
				t.Fatal(err)
			}
			b := lifecycleApp(t, paths.Temporary, base)
			b.DiscardStaleSessions(acct.ID())
			if _, err := os.Stat(acct.Dir()); err != nil {
				t.Fatal("unauthenticated cleanup deleted recovery")
			}
			if err := b.RestoreRecovery(acct.ID()); err != nil {
				t.Fatal(err)
			}
			if _, err := b.UnlockAccount(acct.ID(), "account passphrase", false); err != nil {
				t.Fatal(err)
			}
			p, err := b.OpenProfile(entry.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			close(p.stop)
			<-p.stopped
			b.DiscardStaleSessions(acct.ID())
			if _, err := os.Stat(acct.Dir()); err != nil {
				t.Fatal("unverified cleanup deleted recovery")
			}
			p.setProvider(lifecycleAPI{provider.NewFolder(provider.Google, t.TempDir())})
			if _, err := p.Sync(context.Background()); err != nil {
				t.Fatal(err)
			}
			if changed {
				if err := os.WriteFile(filepath.Join(acct.Dir(), "later.enc"), []byte("new unsynchronized source work"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			b.DiscardStaleSessions(acct.ID())
			if _, err := os.Stat(acct.Dir()); changed && err != nil {
				t.Fatal("changed source was discarded")
			} else if !changed && !os.IsNotExist(err) {
				t.Fatalf("verified unchanged source not discarded: %v", err)
			}
			if _, err := os.Stat(other.Dir()); err != nil {
				t.Fatal("another account was deleted with the stale session")
			}
		})
	}
}

func TestSyncMarkerRejectsUnverifiedNewerAndFallbackEvidence(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "profile.db")
	if err := os.WriteFile(db, []byte("encrypted store"), 0o600); err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	marker := filepath.Join(dir, "synced.json")
	write := func(verified bool) {
		t.Helper()
		if err := atomicfile.WriteJSON(marker, map[string]any{"pending": 0, "verified": verified, "at": at}); err != nil {
			t.Fatal(err)
		}
	}
	write(true)
	if _, ok := storePending(db); !ok {
		t.Fatal("successful current marker rejected")
	}
	if err := os.Chtimes(db, at.Add(time.Second), at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, ok := storePending(db); ok {
		t.Fatal("work written within the old three-second grace period was accepted")
	}
	if err := os.Chtimes(db, at.Add(-time.Second), at.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	write(false)
	if _, ok := storePending(db); ok {
		t.Fatal("failed sync with empty queue was accepted")
	}
	if err := os.WriteFile(marker, []byte("corrupt latest marker"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := storePending(db); ok {
		t.Fatal("older success fallback hid the newer sync failure")
	}
}

func TestFailedRestoreDoesNotPublishPartialAccount(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix directory permissions")
	}
	a := lifecycleApp(t, paths.Temporary, "")
	id := secure.NewID()
	src := a.recoveryDir(id)
	if err := atomicfile.WriteJSON(filepath.Join(src, "recovery.json"), map[string]any{"account": id, "created": time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "a.enc"), []byte("only encrypted copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(src, "z-unreadable")
	if err := os.Mkdir(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(blocked, 0o700) })
	if err := a.RestoreRecovery(id); err == nil {
		t.Fatal("unreadable source should fail")
	}
	if _, err := os.Stat(filepath.Join(a.Vault.Dir, id)); !os.IsNotExist(err) {
		t.Fatalf("partial final account published: %v", err)
	}
	if _, err := os.Stat(filepath.Join(src, "a.enc")); err != nil {
		t.Fatal("original recovery was lost")
	}
}
