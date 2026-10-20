package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReadChecked(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a", "state.bin")
	if err := WriteChecked(p, []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if err := WriteChecked(p, []byte("v2")); err != nil {
		t.Fatal(err)
	}
	b, rec, err := ReadChecked(p)
	if err != nil || rec || string(b) != "v2" {
		t.Fatalf("got %q %v %v", b, rec, err)
	}
	// Simulate a torn write of the current file: the previous version wins.
	raw, _ := os.ReadFile(p)
	raw[len(raw)-1] ^= 0xFF
	os.WriteFile(p, raw, 0o600)
	b, rec, err = ReadChecked(p)
	if err != nil || !rec || string(b) != "v1" {
		t.Fatalf("fallback failed: %q %v %v", b, rec, err)
	}
	// Both damaged: corruption is reported, never silently ignored.
	os.WriteFile(p+".prev", []byte("junk"), 0o600)
	if _, _, err := ReadChecked(p); err != ErrCorrupt {
		t.Fatalf("expected ErrCorrupt, got %v", err)
	}
}

func TestNoTempLeftovers(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "x.json")
	for i := 0; i < 3; i++ {
		if err := WriteJSON(p, map[string]int{"i": i}); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(d)
	for _, e := range entries {
		if e.Name() != "x.json" && e.Name() != "x.json.prev" {
			t.Fatalf("leftover file %s", e.Name())
		}
	}
	var v map[string]int
	if _, err := ReadJSON(p, &v); err != nil || v["i"] != 2 {
		t.Fatalf("read json: %v %v", v, err)
	}
}

func TestMissing(t *testing.T) {
	if _, _, err := ReadChecked(filepath.Join(t.TempDir(), "none")); !os.IsNotExist(err) {
		t.Fatalf("expected not-exist, got %v", err)
	}
}

func TestWriteSecretKeepsNoPrevious(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "key.json")
	// An earlier build kept the superseded version next to the file.
	for _, v := range []string{"v1", "v2"} {
		if err := WriteChecked(p, []byte(v)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(p + ".prev"); err != nil {
		t.Fatal(err)
	}
	if err := WriteSecretJSON(p, map[string]string{"v": "v3"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p + ".prev"); !os.IsNotExist(err) {
		t.Fatalf("superseded version kept: %v", err)
	}
	if err := WriteSecretJSON(p, map[string]string{"v": "v4"}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(d)
	if len(entries) != 1 || entries[0].Name() != "key.json" {
		t.Fatalf("leftover files: %v", entries)
	}
	var v map[string]string
	if err := ReadSecretJSON(p, &v); err != nil || v["v"] != "v4" {
		t.Fatalf("read secret: %v %v", v, err)
	}
}

func TestReadSecretNeverFallsBack(t *testing.T) {
	p := filepath.Join(t.TempDir(), "key.json")
	for _, v := range []string{"old wrapping", "current wrapping"} {
		if err := WriteChecked(p, []byte(v)); err != nil {
			t.Fatal(err)
		}
	}
	if b, err := ReadSecret(p); err != nil || string(b) != "current wrapping" {
		t.Fatalf("got %q %v", b, err)
	}
	raw, _ := os.ReadFile(p)
	raw[len(raw)-1] ^= 0xFF
	os.WriteFile(p, raw, 0o600)
	if b, err := ReadSecret(p); !errors.Is(err, ErrCorrupt) || b != nil {
		t.Fatalf("damaged file: %q %v", b, err)
	}
	os.Remove(p)
	if b, err := ReadSecret(p); !errors.Is(err, os.ErrNotExist) || b != nil {
		t.Fatalf("missing file: %q %v", b, err)
	}
	var v any
	if err := ReadSecretJSON(p, &v); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing json: %v", err)
	}
	// Reading leaves the earlier copy alone; only a write or an explicit
	// removal deletes it.
	if _, err := os.Stat(p + ".prev"); err != nil {
		t.Fatal(err)
	}
	if err := RemovePrevious(p); err != nil {
		t.Fatal(err)
	}
	if err := RemovePrevious(p); err != nil {
		t.Fatalf("removing an absent copy: %v", err)
	}
	if _, err := os.Stat(p + ".prev"); !os.IsNotExist(err) {
		t.Fatalf("copy kept: %v", err)
	}
}

func TestFailedWriteSecretKeepsPrevious(t *testing.T) {
	p := filepath.Join(t.TempDir(), "key.json")
	// A directory in the way makes the final rename fail.
	if err := os.MkdirAll(filepath.Join(p, "occupied"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p+".prev", []byte("earlier copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteSecret(p, []byte("new")); err == nil {
		t.Fatal("write over a directory succeeded")
	}
	if b, err := os.ReadFile(p + ".prev"); err != nil || string(b) != "earlier copy" {
		t.Fatalf("earlier copy removed before the new file was in place: %q %v", b, err)
	}
}
