package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

// Sessions and recovery packages must outlive a reboot: the system
// temporary folder can be memory, and cache folders may be emptied.
func TestTemporaryStateOutlivesTempAndCache(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("folder conventions checked on Linux")
	}
	state, cache, tmp := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("TMPDIR", tmp)
	l, err := Detect("", Options{ForceMode: Temporary, UserHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(l.Root)
	if !Within(filepath.Join(state, "MNE Lab"), l.Root) || !Within(filepath.Join(state, "MNE Lab"), l.Recovery) {
		t.Fatalf("session %s and recovery %s not in the state folder", l.Root, l.Recovery)
	}
	if !slices.Equal(l.Sessions, []string{filepath.Join(state, "MNE Lab", "Sessions"), tmp}) ||
		!slices.Contains(l.LegacyRecovery, filepath.Join(cache, "MNE Lab", "Recovery")) {
		t.Fatalf("earlier locations not read: %v %v", l.Sessions, l.LegacyRecovery)
	}
	// A session an earlier build started in the temporary folder resumes
	// where it is; other folders are refused.
	old := filepath.Join(tmp, SessionPrefix+"old")
	os.MkdirAll(old, 0o700)
	if r, err := Resume(old, Options{}); err != nil || r.Root != old || r.Recovery != l.Recovery {
		t.Fatalf("legacy session: %+v %v", r, err)
	}
	other := filepath.Join(t.TempDir(), SessionPrefix+"x")
	os.MkdirAll(other, 0o700)
	if _, err := Resume(other, Options{}); err == nil {
		t.Fatal("session outside the session folders resumed")
	}
}
