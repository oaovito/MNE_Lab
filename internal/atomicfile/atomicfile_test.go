package atomicfile

import (
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
