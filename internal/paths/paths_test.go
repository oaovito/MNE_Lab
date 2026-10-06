package paths

import (
	"os"
	"path/filepath"
	"testing"
)

// The portable folder must work after being moved to another drive letter or
// path: nothing depends on the absolute location.
func TestPortableSurvivesMove(t *testing.T) {
	base := t.TempDir()
	d := filepath.Join(base, "D", "MNE Lab")
	os.MkdirAll(filepath.Join(d, "app", "1.0.0"), 0o755)
	os.WriteFile(filepath.Join(d, PortableMarker), nil, 0o644)
	exe := filepath.Join(d, "app", "1.0.0", "mnelab.exe")
	l, err := Detect(exe, Options{})
	if err != nil || l.Mode != Portable || l.Root != d {
		t.Fatalf("detect: %+v %v", l, err)
	}
	rel, err := l.Rel(filepath.Join(l.Data, "accounts", "x"))
	if err != nil || rel != "data/accounts/x" {
		t.Fatalf("rel %q %v", rel, err)
	}
	f := filepath.Join(base, "F", "MNE Lab")
	os.MkdirAll(filepath.Dir(f), 0o755)
	if err := os.Rename(d, f); err != nil {
		t.Fatal(err)
	}
	l2, err := Detect(filepath.Join(f, "app", "1.0.0", "mnelab.exe"), Options{})
	if err != nil || l2.Root != f {
		t.Fatalf("after move: %+v %v", l2, err)
	}
	if got := l2.Abs(rel); got != filepath.Join(f, "data", "accounts", "x") {
		t.Fatalf("abs after move: %s", got)
	}
}

func TestTemporaryLayoutKeepsExportsOutsideSession(t *testing.T) {
	base := t.TempDir()
	home := t.TempDir()
	l, err := Detect("", Options{BaseTemp: base, UserHome: home})
	if err != nil || l.Mode != Temporary {
		t.Fatalf("%+v %v", l, err)
	}
	if Within(l.Root, l.Exports) || Within(l.Root, l.Recovery) {
		t.Fatal("exports and recovery must live outside the session folder")
	}
	if !Within(l.Root, l.Data) || !Within(l.Root, l.Window) {
		t.Fatal("data and window profile must live inside the session folder")
	}
	other := filepath.Join(base, SessionPrefix+"old")
	os.MkdirAll(other, 0o700)
	if s := StaleSessions(base, l.Root); len(s) != 1 || s[0] != other {
		t.Fatalf("stale sessions: %v", s)
	}
}
