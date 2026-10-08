package syncer

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/provider"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/vault"
)

type unverifiedAccountWrite struct {
	provider.Provider
	target string
	mode   string
	wrote  map[string]bool
}

func (p *unverifiedAccountWrite) Write(ctx context.Context, name string, b []byte) (provider.Object, error) {
	if strings.Contains(name, p.target) {
		p.wrote[name] = true
		if p.mode == "corrupt" {
			b = bytes.Clone(b)
			b[len(b)-1] ^= 1
		}
	}
	return p.Provider.Write(ctx, name, b)
}

func (p *unverifiedAccountWrite) Read(ctx context.Context, name string) ([]byte, error) {
	if p.mode == "unreadable" && p.wrote[name] {
		return nil, provider.ErrOffline
	}
	return p.Provider.Read(ctx, name)
}

func TestAccountWritesRequireReadBack(t *testing.T) {
	for _, target := range []string{"directory/", "/index.enc", "/avatars/", "/profile.key.json"} {
		for _, mode := range []string{"corrupt", "unreadable"} {
			t.Run(target+mode, func(t *testing.T) {
				cloud := provider.NewFolder(provider.Google, t.TempDir())
				p := &unverifiedAccountWrite{Provider: cloud, target: target, mode: mode, wrote: map[string]bool{}}
				a, _, err := fastVault(t).Create("Lab", "lab passphrase", false)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := a.CreateProfile(vault.ProfileInput{Username: "Ana", StorageMode: vault.StorageCloudOnly, Provider: provider.Google,
					Avatar: []byte("avatar file"), AvatarType: "image/png"}); err != nil {
					t.Fatal(err)
				}
				if _, err := SyncAccount(context.Background(), p, a, nil); err == nil {
					t.Fatal("account synchronization must not report success for unverified writes")
				} else if mode == "unreadable" && !errors.Is(err, provider.ErrOffline) {
					t.Fatalf("readback failure must propagate: %v", err)
				}
			})
		}
	}
}

func TestExistingAvatarBytesAreVerified(t *testing.T) {
	cloud := provider.NewFolder(provider.Google, t.TempDir())
	a, _, err := fastVault(t).Create("Lab", "lab passphrase", false)
	if err != nil {
		t.Fatal(err)
	}
	pe, err := a.CreateProfile(vault.ProfileInput{Username: "Ana", StorageMode: vault.StorageCloudOnly, Provider: provider.Google,
		Avatar: []byte("avatar file"), AvatarType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := SyncAccount(ctx, cloud, a, nil); err != nil {
		t.Fatal(err)
	}
	name := "accounts/" + a.ID() + "/avatars/" + pe.ID + "-" + pe.AvatarHash + ".img"
	want, err := a.SealedAvatar(pe.ID)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := bytes.Clone(want)
	corrupt[len(corrupt)-1] ^= 1
	if _, err := cloud.Write(ctx, name, corrupt); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncAccount(ctx, cloud, a, nil); err != nil {
		t.Fatal(err)
	}
	got, err := cloud.Read(ctx, name)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("existing avatar must be restored and verified from intact local copy: %v", err)
	}
}

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
