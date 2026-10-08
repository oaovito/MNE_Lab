// Package shell gives MNE Lab its window and its tray icon. The window is
// an application window of the Chromium-based browser already present on
// the computer (Microsoft Edge on every supported Windows), started with a
// private, isolated profile inside MNE Lab's own folder and with background
// services, sync, extensions and reporting turned off. No browser engine is
// bundled, which keeps downloads small and memory use low on inexpensive
// computers.
package shell

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/oaovito/mne_lab/internal/app"
)

// ErrNoBrowser means no compatible browser was found; the interface then
// opens in the default browser.
var ErrNoBrowser = errors.New("shell.no_browser")

// Shell implements app.Shell.
type Shell struct {
	// WindowDir is the isolated window profile (caches, local storage).
	WindowDir string
	// Lang is the interface language passed to the window.
	Lang func() string
	// Icon is the tray icon (ICO on Windows, PNG elsewhere).
	Icon []byte
	Log  *slog.Logger

	mu      sync.Mutex
	browser string
	main    *exec.Cmd
	closed  chan bool
	menu    app.TrayMenu
	ready   bool
	status  *systray.MenuItem
	open    *systray.MenuItem
	mobile  *systray.MenuItem
	setts   *systray.MenuItem
	exit    *systray.MenuItem
	temp    bool
	pending string
}

// New prepares the shell.
func New(windowDir string, icon []byte, lang func() string, log *slog.Logger) *Shell {
	s := &Shell{WindowDir: windowDir, Icon: icon, Lang: lang, Log: log, closed: make(chan bool, 4)}
	s.browser = findBrowser()
	return s
}

// Browser returns the browser used for the window ("" when none).
func (s *Shell) Browser() string { return s.browser }

func (s *Shell) args(url string, turbo bool) []string {
	a := []string{
		"--app=" + url,
		"--user-data-dir=" + s.WindowDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-sync",
		"--disable-extensions",
		"--disable-default-apps",
		"--disable-background-networking",
		"--disable-background-mode",
		"--disable-component-update",
		"--disable-client-side-phishing-detection",
		"--disable-domain-reliability",
		"--disable-breakpad",
		"--disable-crash-reporter",
		"--no-pings",
		"--metrics-recording-only",
		"--disable-session-crashed-bubble",
		"--hide-crash-restore-bubble",
		"--disable-features=Translate,MediaRouter,OptimizationHints,AutofillServerCommunication,InterestFeedContentSuggestions,msImplicitSignin,msEdgeShoppingAssistant,msEdgeCollections,EdgeShoppingAssistant",
		"--disk-cache-size=33554432",
		"--renderer-process-limit=2",
		"--window-size=1280,780",
	}
	if l := s.Lang(); l != "" {
		a = append(a, "--lang="+l)
	}
	if runtime.GOOS == "linux" {
		a = append(a, "--password-store=basic")
	}
	if turbo {
		a = append(a, "--force-prefers-reduced-motion", "--disable-smooth-scrolling")
	}
	return a
}

// OpenWindow opens the window at url. While the window already runs, the
// browser opens the new address in a new window of the same process.
func (s *Shell) OpenWindow(url string, turbo bool) error {
	if s.browser == "" {
		return s.OpenExternal(url)
	}
	cmd := exec.Command(s.browser, s.args(url, turbo)...)
	cmd.Env = os.Environ()
	hide(cmd)
	if err := cmd.Start(); err != nil {
		s.Log.Warn("window start failed", "err", err)
		return s.OpenExternal(url)
	}
	s.mu.Lock()
	first := s.main == nil
	if first {
		s.main = cmd
	}
	s.mu.Unlock()
	go func() {
		start := time.Now()
		err := cmd.Wait()
		if !first {
			return // a hand-off to the running window process
		}
		s.mu.Lock()
		s.main = nil
		s.mu.Unlock()
		intentional := err == nil
		if intentional && time.Since(start) < 2*time.Second {
			// The browser handed the window to another process of the same
			// profile; follow nothing more (the window still exists).
			s.Log.Info("window handed over to an existing browser process")
			return
		}
		s.closed <- intentional
	}()
	return nil
}

// Raise brings the running window to the front. It reports whether a
// window is running.
func (s *Shell) Raise() bool {
	s.mu.Lock()
	cmd := s.main
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return false
	}
	raise(cmd.Process.Pid)
	return true
}

// CloseWindow ends the window process.
func (s *Shell) CloseWindow() {
	s.mu.Lock()
	cmd := s.main
	s.main = nil
	s.mu.Unlock()
	if cmd != nil && cmd.Process != nil {
		terminate(cmd.Process)
	}
}

// WindowClosed signals the end of the window: true when the person closed
// it, false when the window process failed.
func (s *Shell) WindowClosed() <-chan bool { return s.closed }

// OpenExternal opens a web address in the default browser or a local folder
// in the file manager.
func (s *Shell) OpenExternal(u string) error {
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://127.0.0.1:") && !strings.HasPrefix(u, "file://") {
		return errors.New("shell.invalid_url")
	}
	return openExternal(u)
}

// RunTray runs the tray icon until QuitTray. It must be called from the
// main goroutine.
func (s *Shell) RunTray(menu app.TrayMenu) {
	s.mu.Lock()
	s.menu = menu
	s.mu.Unlock()
	systray.Run(s.onReady, func() {})
}

func (s *Shell) onReady() {
	m := s.menu
	if len(s.Icon) > 0 {
		systray.SetIcon(s.Icon)
	}
	systray.SetTitle("")
	systray.SetTooltip(m.Label("tray.tooltip"))
	s.mu.Lock()
	s.status = systray.AddMenuItem(m.Label("tray.status.signed_out"), "")
	s.status.Disable()
	systray.AddSeparator()
	s.open = systray.AddMenuItem(m.Label("tray.open"), "")
	s.mobile = systray.AddMenuItem(m.Label("tray.mobile"), "")
	s.setts = systray.AddMenuItem(m.Label("tray.settings"), "")
	systray.AddSeparator()
	s.exit = systray.AddMenuItem(m.Label("tray.exit"), "")
	s.ready = true
	pending, temp := s.pending, s.temp
	s.mu.Unlock()
	if pending != "" {
		s.UpdateTray(pending, temp)
	}
	systray.SetOnTapped(func() { m.Open() })
	go func() {
		for {
			select {
			case <-s.open.ClickedCh:
				m.Open()
			case <-s.mobile.ClickedCh:
				m.Mobile()
			case <-s.setts.ClickedCh:
				m.Settings()
			case <-s.exit.ClickedCh:
				m.Exit()
			}
		}
	}()
}

// UpdateTray shows the current state. In Temporary Machine Mode the exit
// item says that it saves, cleans and exits.
func (s *Shell) UpdateTray(status string, temporary bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending, s.temp = status, temporary
	if !s.ready {
		return
	}
	m := s.menu
	s.status.SetTitle(status)
	s.open.SetTitle(m.Label("tray.open"))
	s.mobile.SetTitle(m.Label("tray.mobile"))
	s.setts.SetTitle(m.Label("tray.settings"))
	if temporary {
		s.exit.SetTitle(m.Label("tray.exit_temporary"))
	} else {
		s.exit.SetTitle(m.Label("tray.exit"))
	}
	systray.SetTooltip(m.Label("tray.tooltip") + " · " + status)
}

// QuitTray ends the tray loop.
func (s *Shell) QuitTray() { systray.Quit() }
