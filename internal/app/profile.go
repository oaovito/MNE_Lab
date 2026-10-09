package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/backup"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/plot"
	"github.com/oaovito/mne_lab/internal/provider"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
	"github.com/oaovito/mne_lab/internal/syncer"
	"github.com/oaovito/mne_lab/internal/vault"
)

// Collections of a profile store. Names starting with "_" never leave the
// device.
const (
	CollFiles          = "ls.file"
	CollMeasurements   = "ls.measurement"
	CollGraphs         = "ls.graph"
	CollCycles         = "ls.cycle"
	CollSettings       = "settings"
	CollExports        = "export.history"
	CollRecent         = "recent"
	CollImportProfiles = "import.profile"
)

// ProfileSettings are the preferences of one profile (synchronized).
type ProfileSettings struct {
	Language     string          `json:"language,omitempty"`
	Theme        string          `json:"theme,omitempty"`
	Onboarding   map[string]bool `json:"onboarding,omitempty"` // seen introductions
	WhatsNewSeen string          `json:"whatsNewSeen,omitempty"`
	Export       ExportPrefs     `json:"export"`
	Decimal      string          `json:"decimal,omitempty"` // number display: "" follows language
}

// ExportPrefs are remembered export choices (they stay with the person).
type ExportPrefs struct {
	Preset      string            `json:"preset,omitempty"`
	Formats     map[string]string `json:"formats,omitempty"` // kind → last format
	Figure      *plot.Spec        `json:"figure,omitempty"`
	CSVDecimal  string            `json:"csvDecimal,omitempty"`
	Destination string            `json:"destination,omitempty"` // portable-relative or label
}

// Sync states shown to the person.
const (
	SyncLocal    = "local"     // USB Drive Only
	SyncSynced   = "synced"    // everything confirmed by the cloud
	SyncPending  = "pending"   // waiting to be sent
	SyncSyncing  = "syncing"   // sending now
	SyncOffline  = "offline"   // no connection; work is protected locally
	SyncReauth   = "reconnect" // the cloud authorization must be renewed
	SyncNoCloud  = "not_connected"
	SyncError    = "error"
	SyncConflict = "conflict"
)

// SyncStatus summarizes synchronization of the open profile.
type SyncStatus struct {
	State       string               `json:"state"`
	Provider    string               `json:"provider,omitempty"`
	Transport   string               `json:"transport,omitempty"`
	Account     provider.AccountInfo `json:"account,omitempty"`
	Pending     int                  `json:"pending"`
	Conflicts   int                  `json:"conflicts"`
	LastSync    time.Time            `json:"lastSync,omitzero"`
	LastError   string               `json:"lastError,omitempty"`
	StorageMode string               `json:"storageMode"`
}

// Profile is an unlocked profile session.
type Profile struct {
	app   *App
	acct  *vault.Account
	Entry vault.ProfileEntry
	key   secure.Key
	St    *store.Store
	dir   string

	mu               sync.Mutex
	prov             provider.Provider
	eng              *syncer.Engine
	status           SyncStatus
	settings         ProfileSettings
	kick             chan struct{}
	stop             chan struct{}
	stopped          chan struct{}
	changed          atomic.Bool // records changed since the last periodic snapshot
	syncMu           sync.Mutex
	loopMu           sync.Mutex
	loopRun          bool
	paused           atomic.Bool
	closed           atomic.Bool
	deferredProvider bool // pending child must not enqueue before commit
}

func (p *Profile) dbPath() string      { return filepath.Join(p.dir, "profile.db") }
func (p *Profile) backupDir() string   { return filepath.Join(p.dir, "backups") }
func (p *Profile) credsPath() string   { return filepath.Join(p.dir, "provider.sealed") }
func (p *Profile) cloudBacked() bool   { return p.Entry.StorageMode != vault.StorageUSBOnly }
func (p *Profile) credKey() secure.Key { return secure.SubKey(p.key, "provider-credentials") }

func (a *App) openProfile(acct *vault.Account, e vault.ProfileEntry, key secure.Key) (*Profile, error) {
	dir := acct.ProfileDir(e.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	p := &Profile{app: a, acct: acct, Entry: e, key: key, dir: dir, kick: make(chan struct{}, 1), stop: make(chan struct{}), stopped: make(chan struct{})}
	deferred := a.opt.HandoffConfirm != nil && a.exiting.Load()
	p.deferredProvider = deferred
	open := store.Open
	if deferred {
		open = store.OpenExisting
	}
	st, err := open(p.dbPath(), key, store.Options{DeviceID: a.device, Queue: p.cloudBacked()})
	if err != nil && !deferred {
		// A damaged store is restored from the newest verified backup.
		if e, berr := backup.Latest(p.backupDir(), key); berr == nil {
			a.Log.Warn("profile store damaged; restoring backup", "backup", e.File)
			if rerr := backup.Restore(p.backupDir(), e, key, p.dbPath()); rerr == nil {
				st, err = store.Open(p.dbPath(), key, store.Options{DeviceID: a.device, Queue: p.cloudBacked()})
			}
		}
		if err != nil {
			return nil, err
		}
	}
	if err != nil {
		return nil, err
	}
	if err := st.Check(); err != nil {
		if deferred {
			st.Close()
			return nil, err
		}
		a.Log.Warn("profile check reported a problem", "err", err)
	}
	p.St = st
	p.loadSettings()
	p.status = SyncStatus{State: SyncLocal, StorageMode: e.StorageMode, Provider: e.Provider}
	if p.cloudBacked() {
		p.status.State = SyncNoCloud
		if prov, err := p.restoreProvider(); err == nil {
			p.setProvider(prov)
		}
	}
	st.Watch(func(store.Change) {
		p.changed.Store(true)
		p.Kick()
	})
	if !deferred {
		p.startLoop()
	}
	p.refreshStatus()
	return p, nil
}

func (p *Profile) loadSettings() {
	var s ProfileSettings
	p.St.View(func(t *store.Tx) error {
		_, err := t.Get(CollSettings, "profile", &s)
		return err
	})
	p.mu.Lock()
	p.settings = s
	p.mu.Unlock()
}

// Settings returns the profile preferences.
func (p *Profile) Settings() ProfileSettings {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.settings
}

// UpdateSettings changes the profile preferences.
func (p *Profile) UpdateSettings(fn func(*ProfileSettings)) (ProfileSettings, error) {
	p.mu.Lock()
	s := p.settings
	p.mu.Unlock()
	fn(&s)
	err := p.St.Update(func(t *store.Tx) error {
		_, err := t.Put(CollSettings, "profile", s)
		return err
	})
	if err != nil {
		return s, err
	}
	p.mu.Lock()
	p.settings = s
	p.mu.Unlock()
	return s, nil
}

// Provider credentials are sealed with a key derived from the profile key
// and stored next to the profile: no other profile can read them.
func (p *Profile) saveCredentials(prov provider.Provider) error {
	creds, err := prov.Credentials()
	if err != nil {
		return err
	}
	env, _ := json.Marshal(map[string]any{"provider": prov.ID(), "transport": prov.Transport(), "creds": json.RawMessage(creds)})
	return writeSealed(p.credsPath(), p.credKey(), env, "provider")
}

func (p *Profile) restoreProvider() (provider.Provider, error) {
	b, err := readSealed(p.credsPath(), p.credKey(), "provider")
	if err != nil {
		return nil, err
	}
	var env struct {
		Provider  string          `json:"provider"`
		Transport string          `json:"transport"`
		Creds     json.RawMessage `json:"creds"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, err
	}
	return p.app.restoreProvider(env.Provider, env.Transport, env.Creds)
}

func (a *App) restoreProvider(id, transport string, creds []byte) (provider.Provider, error) {
	if transport == provider.TransportFolder {
		var f struct {
			Folder string `json:"folder"`
		}
		json.Unmarshal(creds, &f)
		if st, err := os.Stat(f.Folder); err != nil || !st.IsDir() {
			// The desktop client folder can live elsewhere on another computer.
			f.Folder = provider.DetectFolders()[id]
		}
		if f.Folder == "" {
			return nil, provider.ErrUnavailable
		}
		return provider.NewFolder(id, f.Folder), nil
	}
	switch id {
	case provider.Google:
		return provider.RestoreGoogle(a.ProvCfg, creds)
	case provider.OneDrive:
		return provider.RestoreOneDrive(a.ProvCfg, creds)
	case provider.ICloud:
		return provider.RestoreICloud(a.ProvCfg, creds)
	}
	return nil, errUnknownProvider
}

func (p *Profile) setProvider(prov provider.Provider) {
	p.mu.Lock()
	p.prov = prov
	p.eng = nil
	p.status.LastSync, p.status.LastError = time.Time{}, ""
	if prov != nil {
		p.eng = syncer.New(p.St, p.key, prov, p.acct.ID(), p.Entry.ID, p.app.Log)
		p.status.Provider, p.status.Transport, p.status.Account = prov.ID(), prov.Transport(), prov.Info()
		p.status.State = SyncPending
	} else {
		p.status.Transport, p.status.Account = "", provider.AccountInfo{}
		p.status.State = SyncNoCloud
	}
	deferred := p.deferredProvider
	p.mu.Unlock()
	if prov != nil && !deferred {
		// Everything already saved locally gets a remote destination.
		p.St.Update(func(t *store.Tx) error { return t.EnqueueAll() })
		p.Kick()
	}
}

// Kick asks for a synchronization soon.
func (p *Profile) Kick() {
	select {
	case p.kick <- struct{}{}:
	default:
	}
}

func (p *Profile) startLoop() {
	p.loopMu.Lock()
	if p.loopRun || p.closed.Load() || p.paused.Load() || p.app.exiting.Load() {
		p.loopMu.Unlock()
		return
	}
	select {
	case <-p.stop:
		p.stop, p.stopped = make(chan struct{}), make(chan struct{})
	default:
	}
	stop, stopped := p.stop, p.stopped
	p.loopRun = true
	p.loopMu.Unlock()
	p.mu.Lock()
	replay := p.deferredProvider && p.prov != nil
	p.deferredProvider = false
	p.mu.Unlock()
	if replay {
		p.St.Update(func(tx *store.Tx) error { return tx.EnqueueAll() })
		p.Kick()
	}
	go p.loop(stop, stopped)
}

func (p *Profile) stopLoop() {
	p.loopMu.Lock()
	stop, stopped := p.stop, p.stopped
	select {
	case <-stop:
	default:
		close(stop)
	}
	if !p.loopRun {
		select {
		case <-stopped:
		default:
			close(stopped)
		}
	}
	p.loopMu.Unlock()
	<-stopped
}

func (p *Profile) loop(stop, stopped chan struct{}) {
	defer func() {
		p.loopMu.Lock()
		p.loopRun = false
		close(stopped)
		p.loopMu.Unlock()
	}()
	t := time.NewTicker(2 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-p.kick:
			// Coalesce bursts of changes.
			select {
			case <-stop:
				return
			case <-time.After(1500 * time.Millisecond):
			}
		case <-t.C:
			p.maybeSnapshot()
		}
		if p.app.exiting.Load() || p.paused.Load() {
			continue
		}
		ctx, cancel := context.WithTimeout(p.app.ctx, 5*time.Minute)
		p.Sync(ctx)
		cancel()
	}
}

// Sync runs one synchronization (records, blobs, account files).
func (p *Profile) Sync(ctx context.Context) (SyncStatus, error) {
	p.syncMu.Lock()
	defer p.syncMu.Unlock()
	if p.paused.Load() || p.closed.Load() {
		return p.Status(), ErrExiting
	}
	p.mu.Lock()
	eng, prov := p.eng, p.prov
	p.mu.Unlock()
	if eng == nil {
		p.refreshStatus()
		return p.Status(), nil
	}
	done := p.app.begin("sync")
	defer done()
	p.setState(SyncSyncing, "")
	_, err := eng.Run(ctx)
	if err == nil {
		_, err = syncer.SyncAccount(ctx, prov, p.acct, map[string]bool{p.Entry.ID: true})
	}
	p.mu.Lock()
	switch {
	case err == nil:
		p.status.LastSync, p.status.LastError = time.Now().UTC(), ""
	case provider.IsNetworkError(err) || errors.Is(err, provider.ErrOffline):
		p.status.LastError = provider.ErrOffline.Error()
	case errors.Is(err, provider.ErrUnauthorized):
		p.status.LastError = provider.ErrUnauthorized.Error()
	default:
		p.status.LastError = err.Error()
	}
	p.mu.Unlock()
	p.refreshStatus()
	p.writeSyncMarker()
	if err != nil {
		p.app.Log.Info("sync incomplete", "err", err)
	}
	return p.Status(), err
}

func (p *Profile) setState(state, errText string) {
	p.mu.Lock()
	p.status.State = state
	if errText != "" {
		p.status.LastError = errText
	}
	s := p.status
	p.mu.Unlock()
	p.app.hub.Publish("sync", s)
}

// refreshStatus recomputes the state from the queue.
func (p *Profile) refreshStatus() {
	stats, _ := p.St.Stats()
	conflicts, _ := syncer.Conflicts(p.St)
	p.mu.Lock()
	s := &p.status
	s.Pending, s.Conflicts = stats.Pending+stats.Retryable, len(conflicts)
	switch {
	case !p.cloudBacked():
		s.State = SyncLocal
	case p.prov == nil:
		s.State = SyncNoCloud
	case s.LastError == provider.ErrUnauthorized.Error():
		s.State = SyncReauth
	case s.LastError == provider.ErrOffline.Error():
		s.State = SyncOffline
	case len(conflicts) > 0:
		s.State = SyncConflict
	case s.LastError != "":
		s.State = SyncError
	case s.Pending > 0:
		s.State = SyncPending
	case s.LastSync.IsZero() || p.prov.Transport() != provider.TransportAPI:
		// A desktop client's folder confirms a local handoff, but cannot
		// confirm that the client uploaded an intact copy to the cloud.
		s.State = SyncPending
	default:
		s.State = SyncSynced
	}
	out := *s
	p.mu.Unlock()
	p.app.hub.Publish("sync", out)
	p.app.updateTray()
}

// Status returns the synchronization status.
func (p *Profile) Status() SyncStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

// Verified reports whether every change is confirmed by the cloud (or the
// profile is USB Drive Only, where the drive is the destination).
func (p *Profile) Verified() bool {
	if !p.cloudBacked() {
		return true
	}
	p.mu.Lock()
	verified := p.prov != nil && p.prov.Transport() == provider.TransportAPI && !p.status.LastSync.IsZero() && p.status.LastError == "" && p.status.State == SyncSynced
	p.mu.Unlock()
	if !verified {
		return false
	}
	stats, err := p.St.Stats()
	return err == nil && stats.Pending == 0 && stats.Retryable == 0 && stats.Conflicts == 0
}

// Backup creates and verifies a snapshot.
func (p *Profile) Backup(reason string) (backup.Entry, error) {
	done := p.app.begin("backup")
	defer done()
	return backup.Create(p.St, p.key, p.backupDir(), reason)
}

// maybeSnapshot keeps a recent verified local snapshot on the drive in
// Portable USB Mode (the cloud never replaces local redundancy): at most one
// every six hours, and only after changes.
func (p *Profile) maybeSnapshot() {
	if p.app.L.Mode != paths.Portable || !p.changed.Load() || p.app.exiting.Load() {
		return
	}
	if m, err := backup.Load(p.backupDir()); err == nil {
		for _, e := range m.Entries {
			if time.Since(e.Created) < 6*time.Hour {
				return
			}
		}
	}
	if _, err := p.Backup("periodic"); err == nil {
		p.changed.Store(false)
	} else {
		p.app.Log.Warn("periodic snapshot", "err", err)
	}
}

// pauseForRestart releases the data file without destroying this unlocked
// Profile, its provider, preferences, queue or key. Rollback can reuse it.
func (p *Profile) pauseForRestart() error {
	if p.closed.Load() {
		return ErrExiting
	}
	p.paused.Store(true)
	p.stopLoop()
	p.syncMu.Lock()
	defer p.syncMu.Unlock()
	return p.St.Suspend()
}

func (p *Profile) resumeAfterRestart() error {
	p.syncMu.Lock()
	if p.closed.Load() {
		p.syncMu.Unlock()
		return ErrExiting
	}
	if err := p.St.Reopen(); err != nil {
		p.syncMu.Unlock()
		return err
	}
	p.paused.Store(false)
	p.syncMu.Unlock()
	p.startLoop()
	return nil
}

// close stops synchronization and permanently closes the store. Keys are wiped.
func (p *Profile) close() {
	if p.closed.Swap(true) {
		return
	}
	p.paused.Store(true)
	p.stopLoop()
	p.syncMu.Lock()
	defer p.syncMu.Unlock()
	p.St.Close()
	p.key.Wipe()
}

var errUnknownProvider = errors.New("provider.unknown")

func writeSealed(path string, key secure.Key, data []byte, aad string) error {
	return atomicfile.WriteFile(path, secure.Seal(key, data, []byte(aad)), 0o600)
}

func readSealed(path string, key secure.Key, aad string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return secure.Open(key, b, []byte(aad))
}
