package syncer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/provider"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/vault"
)

func fastVault(t *testing.T) *vault.Vault {
	v := vault.New(filepath.Join(t.TempDir(), "accounts"))
	v.KDF = func() secure.KDFParams {
		p := secure.DefaultKDF()
		p.MemoryKiB, p.Time, p.Threads = 8*1024, 1, 1
		return p
	}
	return v
}

// Sign In on a second machine recovers the account and profiles from the
// provider, and nothing readable is stored in the provider.
func TestSignInOnAnotherMachine(t *testing.T) {
	cloudDir := t.TempDir()
	cloud := provider.NewFolder(provider.Google, cloudDir)
	ctx := context.Background()
	v1 := fastVault(t)
	a, _, err := v1.Create("Grupo de Nanopartículas", "lab passphrase", true)
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.CreateProfile(vault.ProfileInput{Username: "Marina", StorageMode: vault.StorageCloudOnly, Provider: provider.Google,
		Password: "pw-123456", Avatar: []byte("GIF89a....avatar"), AvatarType: "image/gif"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SyncAccount(ctx, cloud, a, nil); err != nil {
		t.Fatal(err)
	}
	// Nothing personal is readable in the cloud copy.
	filepath.Walk(cloudDir, func(path string, info os.FileInfo, err error) error {
		if info == nil || info.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(path)
		for _, s := range []string{"Marina", "Nanopart", "avatar", "pw-123456", "lab passphrase"} {
			if strings.Contains(string(b), s) {
				t.Errorf("%q readable in %s", s, filepath.Base(path))
			}
		}
		return nil
	})
	// Machine 2.
	v2 := fastVault(t)
	found, err := FindAccounts(ctx, cloud)
	if err != nil || len(found) != 1 || found[0].ID != a.ID() {
		t.Fatalf("discovery: %+v %v", found, err)
	}
	if err := ImportAccount(ctx, cloud, v2, a.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := v2.Unlock(a.ID(), "wrong"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	b, err := v2.Unlock(a.ID(), "lab passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Profiles()) != 1 || b.Profiles()[0].Username != "Marina" {
		t.Fatalf("profiles: %+v", b.Profiles())
	}
	if _, err := SyncAccount(ctx, cloud, b, nil); err != nil {
		t.Fatal(err)
	}
	av, err := b.Avatar(p.ID)
	if err != nil || string(av) != "GIF89a....avatar" {
		t.Fatalf("avatar not restored: %v", err)
	}
	k1, _ := a.UnlockProfile(p.ID, "pw-123456")
	k2, err := b.UnlockProfile(p.ID, "pw-123456")
	if err != nil || k1 != k2 {
		t.Fatalf("profile key not restored: %v", err)
	}
	// A profile created on machine 2 appears on machine 1 (union merge).
	b.CreateProfile(vault.ProfileInput{Username: "Teo", StorageMode: vault.StorageUSBOnly})
	SyncAccount(ctx, cloud, b, nil)
	if changed, err := SyncAccount(ctx, cloud, a, nil); err != nil || !changed || len(a.Profiles()) != 2 {
		t.Fatalf("merge: changed=%v err=%v n=%d", changed, err, len(a.Profiles()))
	}
}
