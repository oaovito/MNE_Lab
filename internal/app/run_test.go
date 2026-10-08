package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/oaovito/mne_lab/internal/instance"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/update"
)

type startupShell struct {
	open func() error
	tray func()
}

func (s *startupShell) OpenWindow(string, bool) error { return s.open() }
func (*startupShell) Raise() bool                     { return false }
func (*startupShell) CloseWindow()                    {}
func (*startupShell) WindowClosed() <-chan bool       { return nil }
func (s *startupShell) RunTray(TrayMenu)              { s.tray() }
func (*startupShell) UpdateTray(string, bool)         {}
func (*startupShell) OpenExternal(string) error       { return nil }
func (*startupShell) QuitTray()                       {}

func startupApp(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, paths.PortableMarker), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	builds := filepath.Join(root, "app")
	for _, v := range []string{"1.0.0", "1.1.0"} {
		if err := os.MkdirAll(filepath.Join(builds, v), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(builds, v, update.ExeName()), []byte(v), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := update.SaveState(builds, update.State{
		Current: "1.1.0", Previous: "1.0.0",
		Boots: map[string]update.Boot{
			"1.0.0": {OK: true},
			"1.1.0": {Attempts: update.MaxBootAttempts},
		},
	}); err != nil {
		t.Fatal(err)
	}
	a, err := New(Options{Exe: filepath.Join(builds, "1.1.0", update.ExeName()), Root: root, Build: "1.1.0", KDF: fastKDF, UserHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a.Close()
		a.releaseInstance()
	})
	return a
}

func startupUI() fstest.MapFS {
	return fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>MNE Lab</title>")}}
}

func TestWindowStartupFailureStillRollsBack(t *testing.T) {
	a := startupApp(t)
	want := errors.New("window failed")
	a.SetShell(&startupShell{
		open: func() error { return want },
		tray: func() { t.Error("tray started after window startup failed") },
	})
	if err := a.Run(startupUI()); !errors.Is(err, want) {
		t.Fatalf("window failure: got %v, want %v", err, want)
	}
	if update.LoadState(a.L.Builds).Boots["1.1.0"].OK {
		t.Fatal("failed window launch was marked as a successful boot")
	}
	if _, v, err := update.Resolve(a.L.Builds); err != nil || v != "1.0.0" {
		t.Fatalf("failed window launch did not roll back: version=%s, err=%v", v, err)
	}
}

func TestWindowStartupSuccessKeepsBuild(t *testing.T) {
	a := startupApp(t)
	a.SetShell(&startupShell{
		open: func() error {
			if update.LoadState(a.L.Builds).Boots["1.1.0"].OK {
				t.Error("boot was marked successful before opening the window")
			}
			return nil
		},
		tray: func() {
			if !update.LoadState(a.L.Builds).Boots["1.1.0"].OK {
				t.Error("successful window launch was not recorded")
			}
			a.finish()
		},
	})
	if err := a.Run(startupUI()); err != nil {
		t.Fatal(err)
	}
	if _, v, err := update.Resolve(a.L.Builds); err != nil || v != "1.1.0" {
		t.Fatalf("successful window launch rolled back: version=%s, err=%v", v, err)
	}
}

func TestHeadlessStartupKeepsBuild(t *testing.T) {
	a := startupApp(t)
	a.opt.Headless = true
	done := make(chan error, 1)
	go func() { done <- a.Run(startupUI()) }()
	// Run accepts the quit signal only after the server has finished startup.
	select {
	case a.quit <- struct{}{}:
	case <-time.After(5 * time.Second):
		t.Fatal("headless server did not finish startup")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !update.LoadState(a.L.Builds).Boots["1.1.0"].OK {
		t.Fatal("successful headless startup was not recorded")
	}
	if _, v, err := update.Resolve(a.L.Builds); err != nil || v != "1.1.0" {
		t.Fatalf("successful headless startup rolled back: version=%s, err=%v", v, err)
	}
}

func TestMalformedHandoffDoesNotStartWindowOrMarkBootOK(t *testing.T) {
	for _, payload := range []string{"", "{", `{"profile":"missing"}`, `{"ak":"unused-key"}`, `{"mobile":true}`} {
		t.Run(payload, func(t *testing.T) {
			a := startupApp(t)
			a.opt.Handoff = strings.NewReader(payload)
			a.opt.HandoffConfirm = func() error { t.Error("invalid session reached confirmation"); return nil }
			a.SetShell(&startupShell{open: func() error { t.Error("invalid session opened a window"); return nil }, tray: func() { t.Error("invalid session entered tray") }})
			if err := a.Run(startupUI()); err == nil {
				t.Fatal("malformed handoff was ignored")
			}
			if update.LoadState(a.L.Builds).Boots["1.1.0"].OK {
				t.Fatal("malformed handoff marked successful startup")
			}
			if instance.Held(a.L.Data) {
				t.Fatal("failed handoff retained the instance lock")
			}
		})
	}
}

func TestHandoffConfirmationFailureCleansStagedWindow(t *testing.T) {
	a := startupApp(t)
	a.opt.Handoff = strings.NewReader(`{}`)
	want := errors.New("synthetic parent pipe failure")
	a.opt.HandoffConfirm = func() error { return want }
	sh := &restartProbeShell{}
	a.SetShell(sh)
	if err := a.Run(startupUI()); !errors.Is(err, want) {
		t.Fatalf("confirmation failure: %v", err)
	}
	if sh.closed != 1 {
		t.Fatal("failed confirmation did not close the staged window")
	}
	if instance.Held(a.L.Data) {
		t.Fatal("failed confirmation did not release instance")
	}
	if update.LoadState(a.L.Builds).Boots["1.1.0"].OK {
		t.Fatal("uncommitted handoff marked startup successful")
	}
}

func TestPendingHandoffIsNotReportedAsACrash(t *testing.T) {
	parent := startupApp(t)
	child, err := New(Options{Root: parent.L.Root, Headless: true, HandoffConfirm: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	defer child.Close()
	if child.crashed {
		t.Fatal("supervised restart was reported as an abnormal previous exit")
	}
}
