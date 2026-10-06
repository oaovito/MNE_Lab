// Package app is the MNE Lab application core: it owns the execution mode,
// the unlocked account and profile, synchronization, the scientific
// libraries, exports, updates, mobile access and the safe lifecycle, and
// serves the interface over a private loopback connection.
package app

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/i18n"
	"github.com/oaovito/mne_lab/internal/instance"
	"github.com/oaovito/mne_lab/internal/logging"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/provider"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/update"
	"github.com/oaovito/mne_lab/internal/vault"
	"github.com/oaovito/mne_lab/internal/version"
)

// Options configure a run.
type Options struct {
	Exe       string
	Root      string     // portable root passed by the launcher
	Build     string     // build folder name passed by the launcher
	ForceMode paths.Mode // first-run choice or tests
	Session   string     // Temporary Mode session folder to resume (restart)
	Handoff   io.Reader  // restart handoff (keys of the open session)
	BaseTemp  string     // tests
	UserHome  string     // tests
	Headless  bool       // no window or tray (tests, diagnostics)
	Shell     Shell      // window and tray integration
	KDF       func() secure.KDFParams
	HTTP      *http.Client
}

// Shell opens the application window and runs the tray. The app core
// never depends on a particular window technology.
type Shell interface {
	OpenWindow(url string, turbo bool) error
	// Raise brings a running window to the front; false when none runs.
	Raise() bool
	CloseWindow()
	// WindowClosed is signaled when the person closes the window
	// (intentional) or the window process ends abnormally.
	WindowClosed() <-chan bool
	RunTray(menu TrayMenu)
	UpdateTray(status string, temporary bool)
	OpenExternal(url string) error
	QuitTray()
}

// TrayMenu are the tray callbacks.
type TrayMenu struct {
	Open     func()
	Mobile   func()
	Settings func()
	Exit     func()
	Label    func(key string, kv ...string) string
}

// Settings are machine-local application preferences (no personal data).
type Settings struct {
	Language    string `json:"language,omitempty"` // empty: operating system language
	Turbo       bool   `json:"turbo"`
	Theme       string `json:"theme,omitempty"` // system, light, dark
	LastAccount string `json:"lastAccount,omitempty"`
	Onboarded   bool   `json:"onboarded,omitempty"`
}

// App is one running MNE Lab.
type App struct {
	opt     Options
	L       *paths.Layout
	Log     *slog.Logger
	logFile *logging.RotatingFile
	Vault   *vault.Vault
	ProvCfg provider.Config
	hub     *Hub
	srv     *Server
	ui      fs.FS
	upd     *update.Service
	mobile  *Mobile
	client  *http.Client
	device  string
	started time.Time

	mu        sync.Mutex
	settings  Settings
	acct      *vault.Account
	prof      *Profile
	signIn    *signInState
	exiting   atomic.Bool
	busy      atomic.Int32
	quit      chan struct{}
	quitOnce  sync.Once
	ctx       context.Context
	cancel    context.CancelFunc
	crashed   bool // the previous run ended abnormally
	sysLang   string
	restartTo string
	lock      *instance.Lock
	actMu     sync.Mutex
	acts      map[string]Activity
	present   presentation
}

// New prepares the application (no network, no window yet).
func New(opt Options) (*App, error) {
	l, err := layout(opt)
	if err != nil {
		return nil, err
	}
	a := &App{opt: opt, L: l, hub: newHub(), quit: make(chan struct{}), started: time.Now(), acts: map[string]Activity{}, client: opt.HTTP, sysLang: i18n.Detect()}
	if a.client == nil {
		a.client = provider.HTTPClient
	}
	a.ctx, a.cancel = context.WithCancel(context.Background())
	if lf, err := logging.OpenRotating(l.Logs, "mnelab.log"); err == nil {
		a.logFile = lf
		a.Log = logging.New(lf, slog.LevelInfo)
	} else {
		a.Log = logging.Discard()
	}
	a.Vault = vault.New(filepath.Join(l.Data, "accounts"))
	if opt.KDF != nil {
		a.Vault.KDF = opt.KDF
	}
	a.ProvCfg = provider.LoadConfig(l.Data)
	a.loadSettings()
	a.device = a.deviceID()
	a.crashed = a.detectCrash()
	a.upd = &update.Service{Builds: l.Builds, Portable: l.Mode == paths.Portable, Version: version.Version, Channel: version.Channel,
		BuildDate: version.Date, DataSchema: version.Schema, Client: a.client, URL: update.ManifestURL,
		Hooks: update.Hooks{Busy: a.isBusy, Prepare: a.prepareRestart, Restart: a.restartInto, Notify: func(s update.Status) { a.hub.Publish("update", s) }}}
	a.mobile = newMobile(a)
	a.Log.Info("start", "mode", string(l.Mode), "version", version.Version, "crashRecovered", a.crashed)
	return a, nil
}

func layout(opt Options) (*paths.Layout, error) {
	if opt.Session != "" {
		return paths.Resume(opt.Session, paths.Options{BaseTemp: opt.BaseTemp, UserHome: opt.UserHome})
	}
	return paths.Detect(opt.Exe, paths.Options{Root: opt.Root, ForceMode: opt.ForceMode, BaseTemp: opt.BaseTemp, UserHome: opt.UserHome})
}

func (a *App) settingsPath() string { return filepath.Join(a.L.Data, "app.json") }

func (a *App) loadSettings() {
	var s Settings
	atomicfile.ReadJSON(a.settingsPath(), &s)
	a.settings = s
}

// SetShell attaches the window and tray integration before Run.
func (a *App) SetShell(s Shell) { a.opt.Shell = s }

// Settings returns the application preferences.
func (a *App) Settings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

func (a *App) saveSettings(fn func(*Settings)) error {
	a.mu.Lock()
	fn(&a.settings)
	s := a.settings
	a.mu.Unlock()
	return atomicfile.WriteJSON(a.settingsPath(), s)
}

// Lang is the active interface language: the open profile's choice, then
// the application choice, then the operating system language.
func (a *App) Lang() string {
	a.mu.Lock()
	p, s := a.prof, a.settings
	a.mu.Unlock()
	if p != nil {
		if l := p.Settings().Language; l != "" {
			return l
		}
	}
	if s.Language != "" {
		return s.Language
	}
	return a.sysLang
}

// T translates in the active language.
func (a *App) T(key string, kv ...string) string { return i18n.T(a.Lang(), key, kv...) }

func (a *App) deviceID() string {
	p := filepath.Join(a.L.Data, "device.json")
	var d struct {
		ID string `json:"id"`
	}
	if _, err := atomicfile.ReadJSON(p, &d); err == nil && d.ID != "" {
		return d.ID
	}
	d.ID = secure.NewID()
	atomicfile.WriteJSON(p, d)
	return d.ID
}

// Session marker: present while running, removed by a clean exit, so the
// next run knows when it must recover instead of starting fresh.
func (a *App) markerPath() string { return filepath.Join(a.L.Data, "session.json") }

func (a *App) detectCrash() bool {
	var m struct {
		Clean bool `json:"clean"`
	}
	_, err := atomicfile.ReadJSON(a.markerPath(), &m)
	crashed := err == nil && !m.Clean
	atomicfile.WriteJSON(a.markerPath(), map[string]any{"clean": false, "started": time.Now().UTC(), "version": version.Version})
	return crashed
}

func (a *App) markClean() {
	atomicfile.WriteJSON(a.markerPath(), map[string]any{"clean": true, "ended": time.Now().UTC()})
}

// Activity is a running critical operation, shown as a process in the
// interface and on paired phones.
type Activity struct {
	ID      string    `json:"id"`
	Kind    string    `json:"kind"` // import, export, sync, backup
	Started time.Time `json:"started"`
}

// Busy tracking: critical operations (import, export, sync, save) hold it
// so updates and restarts wait for a safe point.
func (a *App) begin(kind string) func() {
	a.busy.Add(1)
	act := Activity{ID: secure.NewID(), Kind: kind, Started: time.Now().UTC()}
	a.actMu.Lock()
	a.acts[act.ID] = act
	a.actMu.Unlock()
	a.hub.Publish("activity", a.Activities())
	var once sync.Once
	return func() {
		once.Do(func() {
			a.actMu.Lock()
			delete(a.acts, act.ID)
			a.actMu.Unlock()
			a.busy.Add(-1)
			a.hub.Publish("activity", a.Activities())
		})
	}
}

// Activities lists the operations in progress, oldest first.
func (a *App) Activities() []Activity {
	a.actMu.Lock()
	defer a.actMu.Unlock()
	out := make([]Activity, 0, len(a.acts))
	for _, v := range a.acts {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started.Before(out[j].Started) })
	return out
}

func (a *App) isBusy() bool { return a.busy.Load() > 0 }

// Done is closed when the application has finished exiting.
func (a *App) Done() <-chan struct{} { return a.quit }

// ErrExiting is returned for requests that arrive while exiting.
var ErrExiting = errors.New("app.exiting")

func (a *App) finish() {
	a.quitOnce.Do(func() {
		a.cancel()
		a.hub.Close()
		if a.logFile != nil {
			a.logFile.Close()
		}
		close(a.quit)
	})
}

// Close releases resources without the exit sequence (tests).
func (a *App) Close() {
	a.closeSession(false)
	if a.srv != nil {
		a.srv.Close()
	}
	a.mobile.Stop()
	a.finish()
	if a.opt.Session == "" && a.L.Mode == paths.Temporary && a.opt.BaseTemp != "" {
		os.RemoveAll(a.L.Root)
	}
}
