package diskfree

import (
	"path/filepath"
	"testing"
)

func TestAvailable(t *testing.T) {
	dir := t.TempDir()
	n, err := Available(filepath.Join(dir, "missing", "child"))
	if err != nil || n == 0 {
		t.Fatalf("available: %d %v", n, err)
	}
	if Ensure(dir, 1) != nil {
		t.Fatal("small write refused")
	}
	if Ensure(dir, 1<<62) != ErrNoSpace {
		t.Fatal("huge write accepted")
	}
}
