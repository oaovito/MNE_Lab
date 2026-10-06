package backup

import (
	"os"
	"path/filepath"
	"testing"

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
