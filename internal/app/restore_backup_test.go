package app

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/oaovito/mne_lab/internal/backup"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/store"
	"github.com/oaovito/mne_lab/internal/vault"
)

func putSaved(t *testing.T, p *Profile, i int) {
	t.Helper()
	if err := p.St.Update(func(tx *store.Tx) error {
		_, err := tx.Put("experiment", "saved", map[string]int{"i": i})
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func savedValue(t *testing.T, p *Profile) int {
	t.Helper()
	var v map[string]int
	if err := p.St.View(func(tx *store.Tx) error { _, err := tx.Get("experiment", "saved", &v); return err }); err != nil {
		t.Fatal(err)
	}
	return v["i"]
}

// fullRotation opens a profile with Keep backups, where backup i holds
// value i and the store holds Keep.
func fullRotation(t *testing.T) (*App, *Profile) {
	t.Helper()
	a := lifecycleApp(t, paths.Portable, "")
	if _, _, err := a.CreateAccount("Lab", "account passphrase", false); err != nil {
		t.Fatal(err)
	}
	entry, err := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: vault.StorageUSBOnly}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.OpenProfile(entry.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < backup.Keep; i++ {
		putSaved(t, p, i)
		if _, err := p.Backup("manual"); err != nil {
			t.Fatal(err)
		}
	}
	putSaved(t, p, backup.Keep)
	return a, p
}

func TestRestoreOldestOfFullBackupRotation(t *testing.T) {
	a, p := fullRotation(t)
	m, err := backup.Load(p.backupDir())
	if err != nil || len(m.Entries) != backup.Keep {
		t.Fatalf("backups: %d %v", len(m.Entries), err)
	}
	oldest := m.Entries[backup.Keep-1]
	if err := a.RestoreBackup(oldest.File); err != nil {
		t.Fatalf("restoring the oldest backup: %v", err)
	}
	restored, err := a.Profile()
	if err != nil {
		t.Fatal(err)
	}
	if v := savedValue(t, restored); v != 0 {
		t.Fatalf("restored %v", v)
	}
	m, err = backup.Load(restored.backupDir())
	if err != nil || len(m.Entries) != backup.Keep {
		t.Fatalf("backups after restore: %d %v", len(m.Entries), err)
	}
	if m.Entries[0].Reason != "before-restore" || m.Entries[backup.Keep-1].File != oldest.File {
		t.Fatalf("rotation after restore: %+v", m.Entries)
	}
	if err := backup.Verify(restored.backupDir(), oldest, restored.key); err != nil {
		t.Fatalf("restored snapshot lost: %v", err)
	}
	// The state replaced by the restore is the newest snapshot.
	if err := a.RestoreBackup(m.Entries[0].File); err != nil {
		t.Fatal(err)
	}
	restored, _ = a.Profile()
	if v := savedValue(t, restored); v != backup.Keep {
		t.Fatalf("state before restore %v", v)
	}
}

// Backups made by others while a restore runs (periodic, Save, exit) must
// neither delete the snapshot being restored nor drop each other's
// manifest entries.
func TestRestoreBackupWithConcurrentBackups(t *testing.T) {
	a, p := fullRotation(t)
	dir, st, key := p.backupDir(), p.St, p.key
	m, err := backup.Load(dir)
	if err != nil || len(m.Entries) != backup.Keep {
		t.Fatalf("backups: %d %v", len(m.Entries), err)
	}
	oldest := m.Entries[backup.Keep-1]
	// The other backups start once the restore has made its own and stop
	// when it closes the store they read.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				if m, _ := backup.Load(dir); len(m.Entries) > 0 && m.Entries[0].Reason == "before-restore" {
					break
				}
			}
			for {
				if _, err := backup.Create(st, key, dir, "manual"); err != nil {
					return
				}
			}
		})
	}
	err = a.RestoreBackup(oldest.File)
	close(stop)
	wg.Wait()
	if err != nil {
		t.Fatalf("restoring the oldest backup: %v", err)
	}
	restored, err := a.Profile()
	if err != nil {
		t.Fatal(err)
	}
	if v := savedValue(t, restored); v != 0 {
		t.Fatalf("restored %v", v)
	}
	m, err = backup.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, e := range m.Entries {
		listed[e.File] = true
		if _, err := os.Stat(filepath.Join(dir, e.File)); err != nil {
			t.Fatalf("listed snapshot missing: %v", err)
		}
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.mnebak"))
	for _, f := range files {
		if !listed[filepath.Base(f)] {
			t.Fatalf("snapshot %s missing from the manifest (%d listed, %d on disk)", filepath.Base(f), len(m.Entries), len(files))
		}
	}
}
