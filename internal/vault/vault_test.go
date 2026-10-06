package vault

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

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
