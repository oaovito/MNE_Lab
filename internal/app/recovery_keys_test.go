package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/provider"
	"github.com/oaovito/mne_lab/internal/syncer"
	"github.com/oaovito/mne_lab/internal/vault"
)

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func writeTestFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryLeavesSupersededKeyFilesBehind(t *testing.T) {
	base := t.TempDir()
	a := lifecycleApp(t, paths.Temporary, base)
	if _, _, err := a.CreateAccount("Lab", "old passphrase", false); err != nil {
		t.Fatal(err)
	}
	entry, err := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: vault.StorageCloudOnly, Provider: provider.Google}, nil)
	if err != nil {
		t.Fatal(err)
	}
	removed, err := a.CreateProfile(ProfileInput{Username: "Bia", StorageMode: vault.StorageCloudOnly, Provider: provider.Google}, nil)
	if err != nil {
		t.Fatal(err)
	}
	acct := a.acct
	id := acct.ID()
	keyFile := filepath.Join("profiles", entry.ID, "profile.key.json")
	// Real earlier versions, as earlier builds kept them next to the
	// current files: the header from before the passphrase change and the
	// account-wrapped key file from before the profile got a password.
	earlier := map[string][]byte{
		"account.json": readTestFile(t, filepath.Join(acct.Dir(), "account.json")),
		keyFile:        readTestFile(t, filepath.Join(acct.Dir(), keyFile)),
	}
	if err := a.ChangePassphrase("old passphrase", "new passphrase"); err != nil {
		t.Fatal(err)
	}
	pdk, err := acct.UnlockProfile(entry.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := acct.SetProfilePassword(entry.ID, pdk, "profile password"); err != nil {
		t.Fatal(err)
	}
	current := map[string][]byte{}
	for rel, b := range earlier {
		current[rel] = readTestFile(t, filepath.Join(acct.Dir(), rel))
		if bytes.Equal(current[rel], b) {
			t.Fatalf("%s was not rewritten", rel)
		}
	}
	var keys vault.ProfileKeys
	if err := atomicfile.ReadSecretJSON(filepath.Join(acct.Dir(), keyFile), &keys); err != nil || !keys.HasPassword {
		t.Fatalf("current key file: %v %+v", err, keys)
	}
	if err := acct.RemoveProfile(removed.ID); err != nil {
		t.Fatal(err)
	}
	trashed, _ := os.ReadDir(filepath.Join(acct.Dir(), "trash"))
	if len(trashed) != 1 {
		t.Fatalf("trash: %v", trashed)
	}
	trashKey := filepath.Join("trash", trashed[0].Name(), "profile.key.json")
	earlier[trashKey] = readTestFile(t, filepath.Join(acct.Dir(), trashKey))
	current[trashKey] = earlier[trashKey]
	plant := func(root string) {
		t.Helper()
		for rel, b := range earlier {
			writeTestFile(t, filepath.Join(root, rel+".prev"), b)
		}
		// A drive key copy is left out even without a current drive key.
		writeTestFile(t, filepath.Join(root, "drive.key.prev"), []byte("superseded drive key"))
	}
	check := func(root string) {
		t.Helper()
		if err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			switch info.Name() {
			case "account.json.prev", "drive.key.prev", "profile.key.json.prev":
				t.Errorf("superseded key file copied: %s", p)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		for rel, b := range current {
			if got, err := os.ReadFile(filepath.Join(root, rel)); err != nil || !bytes.Equal(got, b) {
				t.Errorf("%s is not the current version: %v", rel, err)
			}
		}
	}
	plant(acct.Dir())
	if err := a.saveRecovery(acct); err != nil {
		t.Fatal(err)
	}
	check(a.recoveryDir(id))
	srcDigest, err := recoveryDigest(acct.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if pkgDigest, err := recoveryDigest(a.recoveryDir(id)); err != nil || pkgDigest != srcDigest {
		t.Fatalf("digest covers superseded key files: %v", err)
	}
	// A package written by an earlier build still holds them; restoring it
	// must not bring them back.
	plant(a.recoveryDir(id))
	a.Close()
	b := lifecycleApp(t, paths.Temporary, base)
	if err := b.RestoreRecovery(id); err != nil {
		t.Fatal(err)
	}
	check(filepath.Join(b.Vault.Dir, id))
	if _, err := b.UnlockAccount(id, "old passphrase", false); !errors.Is(err, vault.ErrWrongSecret) {
		t.Fatalf("old passphrase after restore: %v", err)
	}
	if _, err := b.UnlockAccount(id, "new passphrase", false); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryKeepsOnlyIntactCopyOfDamagedKeyFile(t *testing.T) {
	base := t.TempDir()
	a := lifecycleApp(t, paths.Temporary, base)
	rk, _, err := a.CreateAccount("Lab", "account passphrase", false)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: vault.StorageCloudOnly, Provider: provider.Google, Password: "first password"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	acct := a.acct
	id := acct.ID()
	keyFile := filepath.Join(acct.ProfileDir(entry.ID), "profile.key.json")
	// An earlier build changed the password and kept the previous version;
	// the current file was damaged later.
	pdk, err := acct.UnlockProfile(entry.ID, "first password")
	if err != nil {
		t.Fatal(err)
	}
	earlier := readTestFile(t, keyFile)
	if err := acct.SetProfilePassword(entry.ID, pdk, "second password"); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, keyFile+".prev", earlier)
	writeTestFile(t, keyFile, []byte("damaged"))
	if err := a.saveRecovery(acct); err != nil {
		t.Fatal(err)
	}
	rel := filepath.Join("profiles", entry.ID, "profile.key.json.prev")
	if got, err := os.ReadFile(filepath.Join(a.recoveryDir(id), rel)); err != nil || !bytes.Equal(got, earlier) {
		t.Fatalf("only intact key file left out of the recovery package: %v", err)
	}
	a.Close()
	b := lifecycleApp(t, paths.Temporary, base)
	if err := b.RestoreRecovery(id); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(b.Vault.Dir, id, rel)
	if got, err := os.ReadFile(restored); err != nil || !bytes.Equal(got, earlier) {
		t.Fatalf("only intact key file left out of the restored account: %v", err)
	}
	if _, err := b.UnlockAccount(id, "account passphrase", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(restored); err != nil {
		t.Fatalf("unlocking removed the only intact key file: %v", err)
	}
	if _, err := b.OpenProfile(entry.ID, "first password"); err == nil {
		t.Fatal("superseded password accepted")
	}
	if _, err := b.RecoverProfile(entry.ID, rk, "third password"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(restored); !os.IsNotExist(err) {
		t.Fatalf("copy kept after the key file was written again: %v", err)
	}
}

// signInFrom makes cloud the provider a Sign In uses, as StartSignIn does
// after connecting.
func signInFrom(t *testing.T, a *App, cloud provider.Provider) {
	t.Helper()
	found, err := syncer.FindAccounts(context.Background(), cloud)
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.signIn = &signInState{prov: cloud, accounts: found}
	a.mu.Unlock()
}

func TestSignInKeepsUnsyncedPassphraseChange(t *testing.T) {
	ctx := context.Background()
	cloud := provider.NewFolder(provider.Google, t.TempDir())
	a := lifecycleApp(t, paths.Portable, "")
	if _, _, err := a.CreateAccount("Lab", "old passphrase", false); err != nil {
		t.Fatal(err)
	}
	id := a.acct.ID()
	if _, err := syncer.SyncAccount(ctx, cloud, a.acct, nil); err != nil {
		t.Fatal(err)
	}
	if err := a.ChangePassphrase("old passphrase", "new passphrase"); err != nil {
		t.Fatal(err)
	}
	signInFrom(t, a, cloud)
	if _, err := a.SignIn(ctx, id, "new passphrase"); err != nil {
		t.Fatalf("new passphrase after signing in over the local account: %v", err)
	}
	if _, err := a.SignIn(ctx, id, "old passphrase"); !errors.Is(err, vault.ErrWrongSecret) {
		t.Fatalf("old passphrase: %v", err)
	}
	// Signing in synchronized the account, so another machine gets the
	// later header as well.
	b := lifecycleApp(t, paths.Portable, "")
	signInFrom(t, b, cloud)
	if _, err := b.SignIn(ctx, id, "new passphrase"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SignIn(ctx, id, "old passphrase"); !errors.Is(err, vault.ErrWrongSecret) {
		t.Fatalf("old passphrase on another machine: %v", err)
	}
}

func TestPassphraseChangeIsNotUndoneByAnotherMachine(t *testing.T) {
	ctx := context.Background()
	cloud := provider.NewFolder(provider.Google, t.TempDir())
	first := lifecycleApp(t, paths.Portable, "")
	if _, _, err := first.CreateAccount("Lab", "old passphrase", false); err != nil {
		t.Fatal(err)
	}
	id := first.acct.ID()
	if _, err := syncer.SyncAccount(ctx, cloud, first.acct, nil); err != nil {
		t.Fatal(err)
	}
	second := lifecycleApp(t, paths.Portable, "")
	signInFrom(t, second, cloud)
	if _, err := second.SignIn(ctx, id, "old passphrase"); err != nil {
		t.Fatal(err)
	}
	if err := second.ChangePassphrase("old passphrase", "new passphrase"); err != nil {
		t.Fatal(err)
	}
	if _, err := syncer.SyncAccount(ctx, cloud, second.acct, nil); err != nil {
		t.Fatal(err)
	}
	// The first machine still holds the earlier header when it synchronizes.
	if _, err := syncer.SyncAccount(ctx, cloud, first.acct, nil); err != nil {
		t.Fatal(err)
	}
	// It merges the index before publishing its header, so the cloud never
	// offers the earlier header, not even after this first pass.
	early := lifecycleApp(t, paths.Portable, "")
	signInFrom(t, early, cloud)
	if _, err := early.SignIn(ctx, id, "new passphrase"); err != nil {
		t.Fatalf("new passphrase right after the stale machine synchronized: %v", err)
	}
	if _, err := first.UnlockAccount(id, "old passphrase", false); !errors.Is(err, vault.ErrWrongSecret) {
		t.Fatalf("old passphrase on the first machine: %v", err)
	}
	if _, err := first.UnlockAccount(id, "new passphrase", false); err != nil {
		t.Fatal(err)
	}
	third := lifecycleApp(t, paths.Portable, "")
	signInFrom(t, third, cloud)
	if _, err := third.SignIn(ctx, id, "old passphrase"); !errors.Is(err, vault.ErrWrongSecret) {
		t.Fatalf("old passphrase on a new machine: %v", err)
	}
	if _, err := syncer.SyncAccount(ctx, cloud, first.acct, nil); err != nil {
		t.Fatal(err)
	}
	want, err := second.acct.HeaderJSON()
	if err != nil {
		t.Fatal(err)
	}
	if got, err := cloud.Read(ctx, "directory/"+id+".json"); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("cloud keeps a superseded header: %v", err)
	}
	fourth := lifecycleApp(t, paths.Portable, "")
	signInFrom(t, fourth, cloud)
	if _, err := fourth.SignIn(ctx, id, "new passphrase"); err != nil {
		t.Fatal(err)
	}
	if _, err := fourth.SignIn(ctx, id, "old passphrase"); !errors.Is(err, vault.ErrWrongSecret) {
		t.Fatalf("old passphrase on a later machine: %v", err)
	}
}

func TestLostKeyFileDoesNotBringBackPasswordlessProfile(t *testing.T) {
	for _, damage := range []string{"deleted", "garbage"} {
		t.Run(damage, func(t *testing.T) {
			ctx := context.Background()
			cloud := provider.NewFolder(provider.Google, t.TempDir())
			a := lifecycleApp(t, paths.Portable, "")
			if _, _, err := a.CreateAccount("Lab", "account passphrase", false); err != nil {
				t.Fatal(err)
			}
			entry, err := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: vault.StorageUSBCloud, Provider: provider.Google}, nil)
			if err != nil {
				t.Fatal(err)
			}
			acct := a.acct
			if _, err := syncer.SyncAccount(ctx, cloud, acct, nil); err != nil {
				t.Fatal(err)
			}
			// A password added while offline, then the key file is lost.
			pdk, err := acct.UnlockProfile(entry.ID, "")
			if err != nil {
				t.Fatal(err)
			}
			if err := acct.SetProfilePassword(entry.ID, pdk, "s3cret-pw"); err != nil {
				t.Fatal(err)
			}
			keyFile := filepath.Join(acct.ProfileDir(entry.ID), "profile.key.json")
			if damage == "deleted" {
				err = os.Remove(keyFile)
			} else {
				err = os.WriteFile(keyFile, []byte("not a key file"), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := syncer.SyncAccount(ctx, cloud, acct, map[string]bool{entry.ID: true}); err != nil {
				t.Fatal(err)
			}
			// Signing in synchronizes the key files of every profile.
			signInFrom(t, a, cloud)
			if _, err := a.SignIn(ctx, acct.ID(), "account passphrase"); err != nil {
				t.Fatal(err)
			}
			var pk vault.ProfileKeys
			if err := atomicfile.ReadSecretJSON(keyFile, &pk); err == nil && !pk.HasPassword {
				t.Fatal("superseded key file installed")
			}
			if _, err := a.OpenProfile(entry.ID, ""); err == nil {
				t.Fatal("profile with a password opened without it")
			}
		})
	}
}
