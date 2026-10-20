package vault

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/secure"
)

func fastVault(t *testing.T) *Vault {
	v := New(filepath.Join(t.TempDir(), "accounts"))
	v.KDF = func() secure.KDFParams {
		p := secure.DefaultKDF()
		p.MemoryKiB, p.Time, p.Threads = 8*1024, 1, 1
		return p
	}
	return v
}

func TestAccountLifecycle(t *testing.T) {
	v := fastVault(t)
	if _, _, err := v.Create("Lab", "short", false); err != ErrPassphraseWeak {
		t.Fatalf("weak passphrase accepted: %v", err)
	}
	a, rk, err := v.Create("Lab A", "correct horse", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Unlock(a.ID(), "wrong pass"); err != ErrWrongSecret {
		t.Fatalf("wrong passphrase: %v", err)
	}
	b, err := v.Unlock(a.ID(), "correct horse")
	if err != nil || b.Index.Name != "Lab A" {
		t.Fatalf("unlock: %v", err)
	}
	if _, err := v.UnlockRemembered(a.ID()); err != ErrNotRemembered {
		t.Fatalf("not remembered: %v", err)
	}
	a.Remember()
	if _, err := v.UnlockRemembered(a.ID()); err != nil {
		t.Fatal(err)
	}
	// Recovery key resets the passphrase.
	c, err := v.UnlockWithRecovery(a.ID(), rk, "brand new pass")
	if err != nil || c.Index.Name != "Lab A" {
		t.Fatalf("recovery: %v", err)
	}
	if _, err := v.Unlock(a.ID(), "brand new pass"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.UnlockWithRecovery(a.ID(), secure.NewRecoveryKey(), "brand new pass"); err != ErrWrongSecret {
		t.Fatalf("foreign recovery key: %v", err)
	}
	list, _ := v.List()
	if len(list) != 1 || !list[0].Remembered {
		t.Fatalf("list: %+v", list)
	}
}

func TestFiveProfileLimitAndValidation(t *testing.T) {
	v := fastVault(t)
	a, _, _ := v.Create("Lab", "passphrase1", false)
	for i, name := range []string{"Ana", "Bruno", "Carla", "Davi", "Eva"} {
		mode, prov := StorageUSBOnly, ""
		if i%2 == 1 {
			mode, prov = StorageUSBCloud, Providers[i%3]
		}
		if _, err := a.CreateProfile(ProfileInput{Username: name, StorageMode: mode, Provider: prov}); err != nil {
			t.Fatalf("profile %d: %v", i, err)
		}
	}
	if _, err := a.CreateProfile(ProfileInput{Username: "Sexto", StorageMode: StorageUSBOnly}); err != ErrProfileLimit {
		t.Fatalf("sixth profile: %v", err)
	}
	b, _ := v.Unlock(a.ID(), "passphrase1")
	if len(b.Profiles()) != 5 {
		t.Fatal("profiles not persisted")
	}
	v2 := fastVault(t)
	a2, _, _ := v2.Create("Lab", "passphrase1", false)
	cases := []struct {
		in  ProfileInput
		err error
	}{
		{ProfileInput{Username: " ", StorageMode: StorageUSBOnly}, ErrUsernameInvalid},
		{ProfileInput{Username: "x", StorageMode: "ftp"}, ErrStorageMode},
		{ProfileInput{Username: "x", StorageMode: StorageCloudOnly}, ErrProviderNeeded},
		{ProfileInput{Username: "x", StorageMode: StorageCloudOnly, Provider: "dropbox"}, ErrProviderNeeded},
		{ProfileInput{Username: "x", StorageMode: StorageUSBOnly, Provider: "google"}, ErrStorageMode},
	}
	for _, c := range cases {
		if _, err := a2.CreateProfile(c.in); err != c.err {
			t.Fatalf("%+v: got %v want %v", c.in, err, c.err)
		}
	}
	a2.CreateProfile(ProfileInput{Username: "Ana", StorageMode: StorageUSBOnly})
	if _, err := a2.CreateProfile(ProfileInput{Username: "ana", StorageMode: StorageUSBOnly}); err != ErrUsernameTaken {
		t.Fatalf("duplicate username: %v", err)
	}
}

func TestProfilePasswordIsolation(t *testing.T) {
	v := fastVault(t)
	a, rk, _ := v.Create("Lab", "passphrase1", false)
	open, _ := a.CreateProfile(ProfileInput{Username: "Open", StorageMode: StorageUSBOnly})
	locked, _ := a.CreateProfile(ProfileInput{Username: "Locked", StorageMode: StorageUSBOnly, Password: "s3cret-pw"})
	if _, err := a.UnlockProfile(open.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.UnlockProfile(locked.ID, ""); !errors.Is(err, ErrWrongSecret) {
		t.Fatal("password profile opened without password")
	}
	if _, err := a.UnlockProfile(locked.ID, "nope"); !errors.Is(err, ErrWrongSecret) {
		t.Fatal("password profile opened with wrong password")
	}
	k1, err := a.UnlockProfile(locked.ID, "s3cret-pw")
	if err != nil {
		t.Fatal(err)
	}
	// The key file contains no secret that the account key can open.
	raw, _ := os.ReadFile(filepath.Join(a.ProfileDir(locked.ID), "profile.key.json"))
	if len(raw) == 0 {
		t.Fatal("missing key file")
	}
	k2, err := a.RecoverProfile(locked.ID, rk)
	if err != nil || k1 != k2 {
		t.Fatalf("recovery: %v", err)
	}
	if err := a.SetProfilePassword(locked.ID, k1, ""); err != nil {
		t.Fatal(err)
	}
	if k3, err := a.UnlockProfile(locked.ID, ""); err != nil || k3 != k1 {
		t.Fatalf("after removing password: %v", err)
	}
}

func TestAvatarSealed(t *testing.T) {
	v := fastVault(t)
	a, _, _ := v.Create("Lab", "passphrase1", false)
	p, _ := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: StorageUSBOnly, Avatar: []byte("GIF89a-avatar-bytes"), AvatarType: "image/gif"})
	raw, _ := os.ReadFile(filepath.Join(a.Dir(), "avatars", p.ID))
	if string(raw) == "" || contains(raw, "avatar-bytes") {
		t.Fatal("avatar must be sealed at rest")
	}
	got, err := a.Avatar(p.ID)
	if err != nil || string(got) != "GIF89a-avatar-bytes" {
		t.Fatalf("avatar: %v", err)
	}
}

func contains(b []byte, s string) bool {
	return len(s) > 0 && len(b) >= len(s) && string(b) != "" && indexOf(b, s) >= 0
}

func indexOf(b []byte, s string) int {
	for i := 0; i+len(s) <= len(b); i++ {
		if string(b[i:i+len(s)]) == s {
			return i
		}
	}
	return -1
}

func TestAccountIDDuringPassphraseChange(t *testing.T) {
	v := fastVault(t)
	a, _, err := v.Create("Synthetic scope", "correct horse", false)
	if err != nil {
		t.Fatal(err)
	}
	want := a.ID()
	stop := make(chan struct{})
	done := make(chan struct{})
	ready := make(chan struct{})
	go func() {
		defer close(done)
		close(ready)
		for {
			select {
			case <-stop:
				return
			default:
				if a.ID() != want {
					t.Error("account scope changed while rewrapping the account key")
					return
				}
			}
		}
	}()
	<-ready
	for i := 0; i < 3; i++ {
		if err := a.ChangePassphrase("another correct horse"); err != nil {
			t.Error(err)
		}
	}
	close(stop)
	<-done
	if a.ID() != want {
		t.Fatal("account identifier changed")
	}
}

// keyFileCopies lists the previous versions of key files under dir.
func keyFileCopies(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		switch info.Name() {
		case "account.json.prev", "drive.key.prev", "profile.key.json.prev":
			out = append(out, p)
		}
		return nil
	})
	return out
}

func TestPasswordProfileIgnoresSupersededKeyFile(t *testing.T) {
	for damage, want := range map[string]error{"deleted": ErrNoSuchProfile, "garbage": atomicfile.ErrCorrupt} {
		t.Run(damage, func(t *testing.T) {
			v := fastVault(t)
			a, _, err := v.Create("Lab", "passphrase1", false)
			if err != nil {
				t.Fatal(err)
			}
			pe, err := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: StorageUSBOnly})
			if err != nil {
				t.Fatal(err)
			}
			keyFile := filepath.Join(a.ProfileDir(pe.ID), "profile.key.json")
			accountWrapped, err := os.ReadFile(keyFile)
			if err != nil {
				t.Fatal(err)
			}
			pdk, err := a.UnlockProfile(pe.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			if err := a.SetProfilePassword(pe.ID, pdk, "s3cret-pw"); err != nil {
				t.Fatal(err)
			}
			if copies := keyFileCopies(t, a.Dir()); len(copies) != 0 {
				t.Fatalf("superseded key files kept: %v", copies)
			}
			passwordWrapped, err := os.ReadFile(keyFile)
			if err != nil {
				t.Fatal(err)
			}
			// An earlier build kept the account-wrapped version on every write.
			if err := os.WriteFile(keyFile+".prev", accountWrapped, 0o600); err != nil {
				t.Fatal(err)
			}
			if damage == "deleted" {
				err = os.Remove(keyFile)
			} else {
				err = os.WriteFile(keyFile, []byte("not a key file"), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			var legacy ProfileKeys
			if recovered, err := atomicfile.ReadJSON(keyFile, &legacy); err != nil || !recovered || legacy.HasPassword {
				t.Fatalf("setup does not reproduce the earlier fallback: %v %v %+v", recovered, err, legacy)
			}
			if _, err := a.UnlockProfile(pe.ID, ""); !errors.Is(err, want) {
				t.Fatalf("password profile opened from a superseded key file: %v", err)
			}
			if _, err := a.ProfileKeysJSON(pe.ID); err == nil {
				t.Fatal("superseded key file offered for upload")
			}
			if err := os.WriteFile(keyFile, passwordWrapped, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := a.UnlockProfile(pe.ID, ""); !errors.Is(err, ErrWrongSecret) {
				t.Fatalf("password no longer required: %v", err)
			}
			if k, err := a.UnlockProfile(pe.ID, "s3cret-pw"); err != nil || k != pdk {
				t.Fatalf("password unlock: %v", err)
			}
		})
	}
}

func TestOldPassphraseIgnoresSupersededHeader(t *testing.T) {
	v := fastVault(t)
	a, _, err := v.Create("Lab", "old passphrase", false)
	if err != nil {
		t.Fatal(err)
	}
	header := filepath.Join(a.Dir(), "account.json")
	oldHeader, err := os.ReadFile(header)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ChangePassphrase("new passphrase"); err != nil {
		t.Fatal(err)
	}
	if copies := keyFileCopies(t, a.Dir()); len(copies) != 0 {
		t.Fatalf("superseded key files kept: %v", copies)
	}
	newHeader, err := os.ReadFile(header)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(header+".prev", oldHeader, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(header); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Unlock(a.ID(), "old passphrase"); err == nil {
		t.Fatal("old passphrase opened the account from a superseded header")
	}
	if list, _ := v.List(); len(list) != 0 {
		t.Fatalf("account listed from a superseded header: %+v", list)
	}
	if err := os.WriteFile(header, []byte("not a header"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Unlock(a.ID(), "old passphrase"); !errors.Is(err, atomicfile.ErrCorrupt) {
		t.Fatalf("damaged header: %v", err)
	}
	if err := os.WriteFile(header, newHeader, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Unlock(a.ID(), "old passphrase"); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("old passphrase: %v", err)
	}
	if _, err := v.Unlock(a.ID(), "new passphrase"); err != nil {
		t.Fatal(err)
	}
}

func TestOpeningAccountRemovesSupersededKeyFiles(t *testing.T) {
	v := fastVault(t)
	a, _, err := v.Create("Lab", "passphrase1", true)
	if err != nil {
		t.Fatal(err)
	}
	pe, err := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: StorageUSBOnly})
	if err != nil {
		t.Fatal(err)
	}
	removed, err := a.CreateProfile(ProfileInput{Username: "Bia", StorageMode: StorageUSBOnly})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.RemoveProfile(removed.ID); err != nil {
		t.Fatal(err)
	}
	trashed, _ := os.ReadDir(filepath.Join(a.Dir(), "trash"))
	if len(trashed) != 1 {
		t.Fatalf("trash: %v", trashed)
	}
	stale := []string{
		filepath.Join(a.Dir(), "account.json.prev"),
		filepath.Join(a.Dir(), "drive.key.prev"),
		filepath.Join(a.ProfileDir(pe.ID), "profile.key.json.prev"),
		filepath.Join(a.Dir(), "trash", trashed[0].Name(), "profile.key.json.prev"),
	}
	for _, p := range stale {
		if err := os.WriteFile(p, []byte("superseded wrapping"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := v.Unlock(a.ID(), "passphrase1"); err != nil {
		t.Fatal(err)
	}
	if copies := keyFileCopies(t, a.Dir()); len(copies) != 0 {
		t.Fatalf("after unlock: %v", copies)
	}
	// The sealed index keeps its previous version: the account key never
	// changes, so that copy opens with nothing new.
	if _, err := os.Stat(filepath.Join(a.Dir(), "account.data.prev")); err != nil {
		t.Fatalf("sealed index lost its previous version: %v", err)
	}
	if _, err := v.UnlockRemembered(a.ID()); err != nil {
		t.Fatal(err)
	}
}

func TestRememberAndForgetKeepNoSupersededDriveKey(t *testing.T) {
	v := fastVault(t)
	a, _, err := v.Create("Lab", "passphrase1", false)
	if err != nil {
		t.Fatal(err)
	}
	driveKey := filepath.Join(a.Dir(), "drive.key")
	if err := a.Remember(); err != nil {
		t.Fatal(err)
	}
	earlier, err := os.ReadFile(driveKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Remember(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(driveKey + ".prev"); !os.IsNotExist(err) {
		t.Fatalf("superseded drive key kept: %v", err)
	}
	if !a.Remembered() {
		t.Fatal("not remembered")
	}
	if _, err := v.UnlockRemembered(a.ID()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(driveKey+".prev", earlier, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(driveKey); err != nil {
		t.Fatal(err)
	}
	if _, err := v.UnlockRemembered(a.ID()); err != ErrNotRemembered {
		t.Fatalf("superseded drive key opened the account: %v", err)
	}
	if err := a.Remember(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(driveKey + ".prev"); !os.IsNotExist(err) {
		t.Fatalf("superseded drive key kept: %v", err)
	}
	if err := a.Forget(); err != nil {
		t.Fatal(err)
	}
	if a.Remembered() {
		t.Fatal("still remembered")
	}
	if _, err := v.UnlockRemembered(a.ID()); err != ErrNotRemembered {
		t.Fatalf("after forget: %v", err)
	}
	if list, _ := v.List(); len(list) != 1 || list[0].Remembered {
		t.Fatalf("list: %+v", list)
	}
	if _, err := v.Unlock(a.ID(), "passphrase1"); err != nil {
		t.Fatal(err)
	}
}

func TestImportedKeyFilesKeepNoSupersededVersion(t *testing.T) {
	v := fastVault(t)
	a, _, err := v.Create("Lab", "passphrase1", false)
	if err != nil {
		t.Fatal(err)
	}
	pe, err := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: StorageUSBOnly})
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(a.ProfileDir(pe.ID), "profile.key.json")
	if err := os.WriteFile(keyFile+".prev", []byte("superseded wrapping"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := a.ProfileKeysJSON(pe.ID)
	if err != nil {
		t.Fatal(err)
	}
	var pk ProfileKeys
	if err := json.Unmarshal(b, &pk); err != nil {
		t.Fatal(err)
	}
	pk.Updated = pk.Updated.Add(time.Second)
	b, _ = json.Marshal(pk)
	if installed, err := a.ImportProfileKeys(pe.ID, b); err != nil || !installed {
		t.Fatalf("import: %v %v", installed, err)
	}
	headerJSON, err := a.HeaderJSON()
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := a.SealedIndex()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a.Dir(), "account.json.prev"), []byte("superseded wrapping"), 0o600); err != nil {
		t.Fatal(err)
	}
	// An intact local header is kept, so only a missing one is installed.
	if err := os.Remove(filepath.Join(a.Dir(), "account.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := v.ImportHeader(headerJSON, sealed); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{keyFile + ".prev", filepath.Join(a.Dir(), "account.json.prev")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s kept: %v", filepath.Base(p), err)
		}
	}
	if _, err := a.UnlockProfile(pe.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Unlock(a.ID(), "passphrase1"); err != nil {
		t.Fatal(err)
	}
}

func TestPassphraseChangeSupersedesOtherCopiesOfTheHeader(t *testing.T) {
	v1 := fastVault(t)
	a, _, err := v1.Create("Lab", "old passphrase", false)
	if err != nil {
		t.Fatal(err)
	}
	staleHeader, err := a.HeaderJSON()
	if err != nil {
		t.Fatal(err)
	}
	staleIndex, err := a.SealedIndex()
	if err != nil {
		t.Fatal(err)
	}
	// Another machine changes the passphrase.
	v2 := fastVault(t)
	if _, err := v2.ImportHeader(staleHeader, staleIndex); err != nil {
		t.Fatal(err)
	}
	b, err := v2.Unlock(a.ID(), "old passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.ChangePassphrase("new passphrase"); err != nil {
		t.Fatal(err)
	}
	laterHeader, err := b.HeaderJSON()
	if err != nil {
		t.Fatal(err)
	}
	laterIndex, err := b.SealedIndex()
	if err != nil {
		t.Fatal(err)
	}
	// The first machine learns of it from the index it synchronizes and
	// stops publishing its own header.
	idx, err := a.OpenIndex(laterIndex)
	if err != nil {
		t.Fatal(err)
	}
	if changed, err := a.MergeIndex(idx); err != nil || !changed {
		t.Fatalf("merge: changed=%v err=%v", changed, err)
	}
	if got, err := a.HeaderJSON(); err != nil || !bytes.Equal(got, laterHeader) {
		t.Fatalf("superseded header still published: %v", err)
	}
	if _, err := v1.Unlock(a.ID(), "old passphrase"); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("old passphrase after merge: %v", err)
	}
	if _, err := v1.Unlock(a.ID(), "new passphrase"); err != nil {
		t.Fatal(err)
	}
	// A machine given the superseded header with the later index rejects
	// the old passphrase and keeps the later header from then on.
	v3 := fastVault(t)
	if _, err := v3.ImportHeader(staleHeader, laterIndex); err != nil {
		t.Fatal(err)
	}
	if _, err := v3.Unlock(a.ID(), "old passphrase"); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("old passphrase with the later index: %v", err)
	}
	if _, err := v3.Unlock(a.ID(), "new passphrase"); err != nil {
		t.Fatal(err)
	}
	// A superseded header edited to claim a later generation is not vouched
	// for by the account key.
	var forged Header
	if err := json.Unmarshal(staleHeader, &forged); err != nil {
		t.Fatal(err)
	}
	forged.Generation = 99
	forgedJSON, _ := json.Marshal(forged)
	if _, err := v2.ImportHeader(forgedJSON, laterIndex); err != nil {
		t.Fatal(err)
	}
	if _, err := v2.Unlock(a.ID(), "old passphrase"); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("old passphrase with an edited generation: %v", err)
	}
	if _, err := v2.Unlock(a.ID(), "new passphrase"); err != nil {
		t.Fatal(err)
	}
}

func TestImportHeaderKeepsLocalChanges(t *testing.T) {
	v := fastVault(t)
	a, _, err := v.Create("Lab", "old passphrase", false)
	if err != nil {
		t.Fatal(err)
	}
	remoteHeader, err := a.HeaderJSON()
	if err != nil {
		t.Fatal(err)
	}
	remoteIndex, err := a.SealedIndex()
	if err != nil {
		t.Fatal(err)
	}
	// Changes not uploaded yet.
	if err := a.ChangePassphrase("new passphrase"); err != nil {
		t.Fatal(err)
	}
	pe, err := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: StorageUSBOnly})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.ImportHeader(remoteHeader, remoteIndex); err != nil {
		t.Fatal(err)
	}
	b, err := v.Unlock(a.ID(), "new passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Profile(pe.ID); err != nil {
		t.Fatalf("local profile lost: %v", err)
	}
	if _, err := v.Unlock(a.ID(), "old passphrase"); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("old passphrase after import: %v", err)
	}
	// A later remote header still replaces an earlier local one.
	v2 := fastVault(t)
	if _, err := v2.ImportHeader(remoteHeader, remoteIndex); err != nil {
		t.Fatal(err)
	}
	laterHeader, err := a.HeaderJSON()
	if err != nil {
		t.Fatal(err)
	}
	laterIndex, err := a.SealedIndex()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v2.ImportHeader(laterHeader, laterIndex); err != nil {
		t.Fatal(err)
	}
	if _, err := v2.Unlock(a.ID(), "new passphrase"); err != nil {
		t.Fatal(err)
	}
	if _, err := v2.Unlock(a.ID(), "old passphrase"); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("old passphrase after a later import: %v", err)
	}
}

func TestSupersededRemoteProfileKeysAreNotInstalled(t *testing.T) {
	for _, damage := range []string{"deleted", "garbage"} {
		t.Run(damage, func(t *testing.T) {
			v := fastVault(t)
			a, _, err := v.Create("Lab", "passphrase1", false)
			if err != nil {
				t.Fatal(err)
			}
			pe, err := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: StorageUSBCloud, Provider: "google"})
			if err != nil {
				t.Fatal(err)
			}
			// Versions as earlier synchronizations uploaded them.
			accountWrapped, err := a.ProfileKeysJSON(pe.ID)
			if err != nil {
				t.Fatal(err)
			}
			pdk, err := a.UnlockProfile(pe.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			if err := a.SetProfilePassword(pe.ID, pdk, "first-pw"); err != nil {
				t.Fatal(err)
			}
			firstPassword, err := a.ProfileKeysJSON(pe.ID)
			if err != nil {
				t.Fatal(err)
			}
			// Key files are ordered by their Updated time.
			first := a.ProfileKeysUpdated(pe.ID)
			for !time.Now().UTC().After(first) {
				time.Sleep(time.Millisecond)
			}
			if err := a.SetProfilePassword(pe.ID, pdk, "second-pw"); err != nil {
				t.Fatal(err)
			}
			current, err := a.ProfileKeysJSON(pe.ID)
			if err != nil {
				t.Fatal(err)
			}
			keyFile := filepath.Join(a.ProfileDir(pe.ID), "profile.key.json")
			if damage == "deleted" {
				err = os.Remove(keyFile)
			} else {
				err = os.WriteFile(keyFile, []byte("not a key file"), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			for name, b := range map[string][]byte{"account-wrapped": accountWrapped, "first password": firstPassword} {
				if installed, err := a.ImportProfileKeys(pe.ID, b); err != nil || installed {
					t.Fatalf("%s key file installed: %v %v", name, installed, err)
				}
			}
			if _, err := a.UnlockProfile(pe.ID, ""); err == nil {
				t.Fatal("profile opened with the account key")
			}
			if installed, err := a.ImportProfileKeys(pe.ID, current); err != nil || !installed {
				t.Fatalf("current key file: %v %v", installed, err)
			}
			if _, err := a.UnlockProfile(pe.ID, "first-pw"); !errors.Is(err, ErrWrongSecret) {
				t.Fatalf("first password: %v", err)
			}
			if k, err := a.UnlockProfile(pe.ID, "second-pw"); err != nil || k != pdk {
				t.Fatalf("second password: %v", err)
			}
		})
	}
}

func TestMergeKeepsNewestProfileKeyState(t *testing.T) {
	v := fastVault(t)
	a, _, err := v.Create("Lab", "passphrase1", false)
	if err != nil {
		t.Fatal(err)
	}
	pe, err := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: StorageUSBOnly})
	if err != nil {
		t.Fatal(err)
	}
	pdk, err := a.UnlockProfile(pe.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetProfilePassword(pe.ID, pdk, "s3cret-pw"); err != nil {
		t.Fatal(err)
	}
	local, err := a.Profile(pe.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Another machine renamed the profile later, still recording the key
	// file from before the password was set.
	renamed := pe
	renamed.Username = "Ana B"
	remote := Index{Schema: SchemaVersion, Name: "Lab", Updated: a.Index.Updated.Add(time.Hour), Profiles: []ProfileEntry{renamed}}
	if _, err := a.MergeIndex(remote); err != nil {
		t.Fatal(err)
	}
	got, err := a.Profile(pe.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "Ana B" || !got.HasPassword || !got.KeysUpdated.Equal(local.KeysUpdated) {
		t.Fatalf("merged entry: %+v", got)
	}
}

func TestUnlockKeepsPreviousVersionOfDamagedKeyFile(t *testing.T) {
	v := fastVault(t)
	a, rk, err := v.Create("Lab", "passphrase1", false)
	if err != nil {
		t.Fatal(err)
	}
	pe, err := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: StorageUSBOnly, Password: "first-pw"})
	if err != nil {
		t.Fatal(err)
	}
	pdk, err := a.UnlockProfile(pe.ID, "first-pw")
	if err != nil {
		t.Fatal(err)
	}
	// An earlier build changed the password and kept the previous version;
	// the current file was damaged later.
	keyFile := filepath.Join(a.ProfileDir(pe.ID), "profile.key.json")
	pk, err := a.wrapProfile(pe.ID, pdk, "second-pw")
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicfile.WriteJSON(keyFile, pk); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A drive key is never the only way in, so its copy always goes.
	driveKeyCopy := filepath.Join(a.Dir(), "drive.key.prev")
	if err := os.WriteFile(driveKeyCopy, []byte("superseded drive key"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := v.Unlock(a.ID(), "passphrase1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keyFile + ".prev"); err != nil {
		t.Fatalf("only intact copy of the profile key removed: %v", err)
	}
	if _, err := os.Stat(driveKeyCopy); !os.IsNotExist(err) {
		t.Fatalf("drive key copy kept: %v", err)
	}
	if _, err := b.UnlockProfile(pe.ID, "first-pw"); err == nil {
		t.Fatal("superseded password accepted")
	}
	k, err := b.RecoverProfile(pe.ID, rk)
	if err != nil || k != pdk {
		t.Fatalf("recovery key: %v", err)
	}
	if err := b.SetProfilePassword(pe.ID, k, "third-pw"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(keyFile + ".prev"); !os.IsNotExist(err) {
		t.Fatalf("copy kept after the key file was written again: %v", err)
	}
	if _, err := b.UnlockProfile(pe.ID, "third-pw"); err != nil {
		t.Fatal(err)
	}
}

func TestNewerPasswordKeyFileReachesAStaleIndex(t *testing.T) {
	vm := fastVault(t)
	m, _, err := vm.Create("Lab", "passphrase1", false)
	if err != nil {
		t.Fatal(err)
	}
	pe, err := m.CreateProfile(ProfileInput{Username: "Ana", StorageMode: StorageUSBCloud, Provider: "google"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := m.CreateProfile(ProfileInput{Username: "Bo", StorageMode: StorageUSBOnly})
	if err != nil {
		t.Fatal(err)
	}
	// Indexes written by earlier builds record no key file times.
	legacy := func(a *Account) {
		for i := range a.Index.Profiles {
			a.Index.Profiles[i].KeysUpdated = time.Time{}
		}
		if err := a.Save(); err != nil {
			t.Fatal(err)
		}
	}
	legacy(m)
	h, _ := m.HeaderJSON()
	idx, _ := m.SealedIndex()
	noPassword, _ := m.ProfileKeysJSON(pe.ID)
	vs := fastVault(t)
	if _, err := vs.ImportHeader(h, idx); err != nil {
		t.Fatal(err)
	}
	s, err := vs.Unlock(m.ID(), "passphrase1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportProfileKeys(pe.ID, noPassword); err != nil {
		t.Fatal(err)
	}
	// An earlier build on the first machine sets a password, then the
	// second machine edits another profile later, so its index is newer.
	pdk, err := m.UnlockProfile(pe.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	set := m.ProfileKeysUpdated(pe.ID)
	for !time.Now().UTC().After(set) {
		time.Sleep(time.Millisecond)
	}
	if err := m.SetProfilePassword(pe.ID, pdk, "secret pw"); err != nil {
		t.Fatal(err)
	}
	legacy(m)
	time.Sleep(2 * time.Millisecond)
	if _, err := s.UpdateProfile(other.ID, func(p *ProfileEntry) error { p.Username = "Bob"; return nil }); err != nil {
		t.Fatal(err)
	}
	mi, _ := m.SealedIndex()
	x, err := s.OpenIndex(mi)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.MergeIndex(x); err != nil {
		t.Fatal(err)
	}
	withPassword, _ := m.ProfileKeysJSON(pe.ID)
	if installed, err := s.ImportProfileKeys(pe.ID, withPassword); err != nil || !installed {
		t.Fatalf("newer password key file: %v %v", installed, err)
	}
	if _, err := s.UnlockProfile(pe.ID, ""); err == nil {
		t.Fatal("profile still opens with the account key")
	}
	if k, err := s.UnlockProfile(pe.ID, "secret pw"); err != nil || k != pdk {
		t.Fatalf("password: %v", err)
	}
	got, _ := s.Profile(pe.ID)
	if !got.HasPassword || got.KeysUpdated.IsZero() || !s.ProfileKeysCurrent(pe.ID) {
		t.Fatalf("index entry after install: %+v", got)
	}
	// The account-wrapped file is now superseded everywhere.
	if installed, err := s.ImportProfileKeys(pe.ID, noPassword); err != nil || installed {
		t.Fatalf("superseded file installed: %v %v", installed, err)
	}
}

func TestRecoveryKeyOpensAccountWithDamagedHeader(t *testing.T) {
	for _, copy := range []string{"recovery copy", "previous version"} {
		t.Run(copy, func(t *testing.T) {
			v := fastVault(t)
			a, rk, err := v.Create("Lab", "passphrase1", false)
			if err != nil {
				t.Fatal(err)
			}
			pe, err := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: StorageUSBOnly})
			if err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(a.Dir(), "account.json")
			if copy == "previous version" {
				// An account written by an earlier build: no recovery copy,
				// the previous version of the header next to it.
				b, err := atomicfile.ReadSecret(p)
				if err != nil {
					t.Fatal(err)
				}
				if err := atomicfile.WriteSecret(p+".prev", b); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(a.Dir(), "account.recovery.json")); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(p, []byte("damaged"), 0o600); err != nil {
				t.Fatal(err)
			}
			if list, err := v.List(); err != nil || len(list) != 1 || list[0].ID != a.ID() {
				t.Fatalf("list with a damaged header: %+v %v", list, err)
			}
			if _, err := v.Unlock(a.ID(), "passphrase1"); err == nil {
				t.Fatal("passphrase opened a damaged header")
			}
			b, err := v.UnlockWithRecovery(a.ID(), rk, "brand new pass")
			if err != nil {
				t.Fatalf("recovery key: %v", err)
			}
			if _, err := b.Profile(pe.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := v.Unlock(a.ID(), "brand new pass"); err != nil {
				t.Fatalf("new passphrase: %v", err)
			}
			if _, err := v.Unlock(a.ID(), "passphrase1"); !errors.Is(err, ErrWrongSecret) {
				t.Fatalf("old passphrase: %v", err)
			}
			if _, err := os.Stat(filepath.Join(a.Dir(), "account.recovery.json")); err != nil {
				t.Fatalf("recovery copy: %v", err)
			}
		})
	}
}

func TestPassphraseChangesFromTheSameGenerationConverge(t *testing.T) {
	vm := fastVault(t)
	m, rk, err := vm.Create("Lab", "passphrase A", false)
	if err != nil {
		t.Fatal(err)
	}
	h, _ := m.HeaderJSON()
	idx, _ := m.SealedIndex()
	vs := fastVault(t)
	if _, err := vs.ImportHeader(h, idx); err != nil {
		t.Fatal(err)
	}
	if err := m.ChangePassphrase("passphrase B"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	s, err := vs.UnlockWithRecovery(m.ID(), rk, "passphrase C")
	if err != nil {
		t.Fatal(err)
	}
	// Each machine merges the other's index, twice over.
	for range 2 {
		for _, pair := range [][2]*Account{{m, s}, {s, m}} {
			sealed, _ := pair[0].SealedIndex()
			x, err := pair[1].OpenIndex(sealed)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pair[1].MergeIndex(x); err != nil {
				t.Fatal(err)
			}
		}
	}
	hm, _ := m.HeaderJSON()
	hs, _ := s.HeaderJSON()
	if !bytes.Equal(hm, hs) {
		t.Fatal("machines kept different headers")
	}
	for _, v := range []*Vault{vm, vs} {
		if _, err := v.Unlock(m.ID(), "passphrase C"); err != nil {
			t.Fatalf("later change: %v", err)
		}
		if _, err := v.Unlock(m.ID(), "passphrase B"); !errors.Is(err, ErrWrongSecret) {
			t.Fatalf("earlier change: %v", err)
		}
	}
}

func TestForgedRemoteHeaderDoesNotReplaceLocalHeader(t *testing.T) {
	v := fastVault(t)
	a, rk, err := v.Create("Lab", "passphrase1", false)
	if err != nil {
		t.Fatal(err)
	}
	h, _ := a.HeaderJSON()
	idx, _ := a.SealedIndex()
	var forged Header
	if err := json.Unmarshal(h, &forged); err != nil {
		t.Fatal(err)
	}
	forged.Generation = 99
	forged.WrappedByPassphrase = "garbled"
	forged.WrappedByRecovery = "garbled"
	fj, _ := json.Marshal(forged)
	if _, err := v.ImportHeader(fj, idx); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Unlock(a.ID(), "passphrase1"); err != nil {
		t.Fatalf("current passphrase after a forged import: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a.Dir(), "account.next.json")); !os.IsNotExist(err) {
		t.Fatalf("forged candidate kept: %v", err)
	}
	if _, err := v.ImportHeader(fj, idx); err != nil {
		t.Fatal(err)
	}
	b, err := v.UnlockWithRecovery(a.ID(), rk, "another passphrase")
	if err != nil {
		t.Fatalf("recovery key after a forged import: %v", err)
	}
	// A genuine later header is adopted once it unlocks.
	v2 := fastVault(t)
	if _, err := v2.ImportHeader(h, idx); err != nil {
		t.Fatal(err)
	}
	later, _ := b.HeaderJSON()
	if _, err := v2.ImportHeader(later, idx); err != nil {
		t.Fatal(err)
	}
	if _, err := v2.Unlock(a.ID(), "another passphrase"); err != nil {
		t.Fatalf("genuine later header: %v", err)
	}
	if _, err := v2.Unlock(a.ID(), "passphrase1"); !errors.Is(err, ErrWrongSecret) {
		t.Fatalf("superseded passphrase: %v", err)
	}
	if _, err := os.Stat(filepath.Join(v2.dir(a.ID()), "account.next.json")); !os.IsNotExist(err) {
		t.Fatalf("adopted candidate kept: %v", err)
	}
}
