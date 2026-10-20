package backup

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
)

func TestCreateVerifyRotateRestore(t *testing.T) {
	d := t.TempDir()
	key := secure.NewKey()
	dbp := filepath.Join(d, "profile.db")
	s, err := store.Open(dbp, key, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	bdir := filepath.Join(d, "backups")
	for i := 0; i < Keep+3; i++ {
		s.Update(func(tx *store.Tx) error { _, err := tx.Put("files", "f", map[string]int{"i": i}); return err })
		if _, err := Create(s, key, bdir, "test"); err != nil {
			t.Fatal(err)
		}
	}
	m, _ := Load(bdir)
	if len(m.Entries) != Keep {
		t.Fatalf("rotation: %d entries", len(m.Entries))
	}
	files, _ := filepath.Glob(filepath.Join(bdir, "*.mnebak"))
	if len(files) != Keep {
		t.Fatalf("rotation left %d files", len(files))
	}
	// Corrupt the newest; Latest falls back to the next verified one.
	newest := filepath.Join(bdir, m.Entries[0].File)
	raw, _ := os.ReadFile(newest)
	raw[len(raw)/2] ^= 0xFF
	os.WriteFile(newest, raw, 0o600)
	e, err := Latest(bdir, key)
	if err != nil || e.File != m.Entries[1].File {
		t.Fatalf("latest: %v %v", e.File, err)
	}
	s.Close()
	if err := Restore(bdir, e, key, dbp); err != nil {
		t.Fatal(err)
	}
	r, err := store.Open(dbp, key, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var v map[string]int
	r.View(func(tx *store.Tx) error { _, err := tx.Get("files", "f", &v); return err })
	if v["i"] != Keep+1 {
		t.Fatalf("restored wrong version: %v", v)
	}
	if _, err := os.Stat(dbp + ".before-restore"); err != nil {
		t.Fatal("previous store must be preserved")
	}
	if err := Verify(bdir, e, secure.NewKey()); err == nil {
		t.Fatal("verification with the wrong key must fail")
	}
}

func TestRotationKeepsPinnedSnapshot(t *testing.T) {
	d := t.TempDir()
	key := secure.NewKey()
	s, err := store.Open(filepath.Join(d, "profile.db"), key, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	bdir := filepath.Join(d, "backups")
	for i := 0; i < Keep; i++ {
		if _, err := Create(s, key, bdir, "test"); err != nil {
			t.Fatal(err)
		}
	}
	m, _ := Load(bdir)
	oldest := m.Entries[Keep-1]
	unpin := Pin(bdir, oldest.File)
	// Every snapshot made while pinned keeps it, not only the first.
	for i := 0; i < 3; i++ {
		if _, err := Create(s, key, bdir, "test"); err != nil {
			t.Fatal(err)
		}
	}
	m, _ = Load(bdir)
	if len(m.Entries) != Keep || m.Entries[Keep-1].File != oldest.File {
		t.Fatalf("rotation: %+v", m.Entries)
	}
	files, _ := filepath.Glob(filepath.Join(bdir, "*.mnebak"))
	if len(files) != Keep {
		t.Fatalf("rotation left %d files", len(files))
	}
	if err := Restore(bdir, oldest, key, filepath.Join(d, "restored.db")); err != nil {
		t.Fatalf("pinned snapshot: %v", err)
	}
	// Each holder releases only its own pin, however often it calls.
	again := Pin(bdir, oldest.File)
	unpin()
	unpin()
	if _, err := Create(s, key, bdir, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(bdir, oldest.File)); err != nil {
		t.Fatalf("snapshot still pinned by another holder: %v", err)
	}
	again()
	if _, err := Create(s, key, bdir, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(bdir, oldest.File)); !os.IsNotExist(err) {
		t.Fatalf("unpinned oldest kept: %v", err)
	}
}

// Concurrent snapshots (periodic, manual, exit) must all reach the
// manifest, and rotation must remove exactly what it drops from it.
func TestConcurrentCreatesKeepManifestComplete(t *testing.T) {
	d := t.TempDir()
	key := secure.NewKey()
	s, err := store.Open(filepath.Join(d, "profile.db"), key, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	bdir := filepath.Join(d, "backups")
	errs := make(chan error, 2*Keep)
	var wg sync.WaitGroup
	for range 2 * Keep {
		wg.Go(func() {
			if _, err := Create(s, key, bdir, "test"); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	m, _ := Load(bdir)
	files, _ := filepath.Glob(filepath.Join(bdir, "*.mnebak"))
	if len(m.Entries) != Keep || len(files) != Keep {
		t.Fatalf("%d snapshots listed, %d on disk", len(m.Entries), len(files))
	}
	for _, e := range m.Entries {
		if err := Verify(bdir, e, key); err != nil {
			t.Fatalf("listed snapshot %s: %v", e.File, err)
		}
	}
}

func TestFailedRestoreKeepsTheStore(t *testing.T) {
	d := t.TempDir()
	key := secure.NewKey()
	dbp := filepath.Join(d, "profile.db")
	s, err := store.Open(dbp, key, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	bdir := filepath.Join(d, "backups")
	e, err := Create(s, key, bdir, "test")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	before, err := os.ReadFile(dbp)
	if err != nil {
		t.Fatal(err)
	}
	failed := errors.New("disk full")
	replace = func(string, []byte, os.FileMode) error { return failed }
	t.Cleanup(func() { replace = atomicfile.ReplaceFile })
	if err := Restore(bdir, e, key, dbp); !errors.Is(err, failed) {
		t.Fatalf("restore error: %v", err)
	}
	if after, err := os.ReadFile(dbp); err != nil || !bytes.Equal(after, before) {
		t.Fatalf("failed restore left no store: %v", err)
	}
}
