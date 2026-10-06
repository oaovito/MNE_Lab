package update

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// CheckInterval is how often a running application looks for builds.
const CheckInterval = 6 * time.Hour

// Hooks connect the updater to the application without giving it access
// to User Data: it can only ask the application to reach a safe point.
type Hooks struct {
	// Busy reports critical work in progress (import, export, sync, save).
	Busy func() bool
	// Prepare saves everything, flushes the journal, keeps pending sync,
	// makes a backup and a recovery point before a build switch.
	Prepare func(ctx context.Context) error
	// Restart restarts into the active build (Save → Preserve → Restart →
	// Restore; never the Temporary Mode cleanup).
	Restart func(exe string) error
	// Notify tells the interface something changed (discreetly).
	Notify func(Status)
}

// Status is shown on the LockedBuild screen and in Settings.
type Status struct {
	Configured  bool      `json:"configured"`
	Portable    bool      `json:"portable"`
	Auto        bool      `json:"auto"`
	Locked      bool      `json:"locked"`
	LockedAt    time.Time `json:"lockedAt,omitzero"`
	Current     string    `json:"current"`
	Channel     string    `json:"channel"`
	BuildDate   string    `json:"buildDate,omitempty"`
	Latest      string    `json:"latest,omitempty"`
	Newer       bool      `json:"newer"`
	Staged      string    `json:"staged,omitempty"` // downloaded, waiting for a safe point
	Applying    bool      `json:"applying,omitempty"`
	LastCheck   time.Time `json:"lastCheck,omitzero"`
	Error       string    `json:"error,omitempty"`
	Choices     []Choice  `json:"choices,omitempty"`
	DataSchema  int       `json:"dataSchema"`
	Platform    string    `json:"platform"`
	Online      bool      `json:"online"`
	ManifestErr string    `json:"manifestError,omitempty"`
}

// Service runs the update policy of one installation.
type Service struct {
	Builds     string
	Portable   bool
	Version    string
	Channel    string
	BuildDate  string
	DataSchema int
	Client     *http.Client
	URL        string
	Hooks      Hooks

	mu       sync.Mutex
	manifest *Manifest
	staged   string
	applying bool
	lastErr  string
	online   bool
}

func (s *Service) state() State { return LoadState(s.Builds) }

// Auto reports whether builds update automatically: always, except in a
// Portable USB install that the person explicitly set to LockedBuild.
func (s *Service) Auto() bool {
	return !(s.Portable && s.state().Locked)
}

// Status returns the current status (without network access).
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state()
	out := Status{Configured: Configured(), Portable: s.Portable, Auto: !(s.Portable && st.Locked), Locked: s.Portable && st.Locked, LockedAt: st.LockedAt,
		Current: s.Version, Channel: s.Channel, BuildDate: s.BuildDate, Staged: s.staged, Applying: s.applying, LastCheck: st.LastCheck,
		Error: s.lastErr, DataSchema: s.DataSchema, Platform: runtime.GOOS + "/" + runtime.GOARCH, Online: s.online}
	if s.manifest != nil {
		out.Choices = Stable(*s.manifest, runtime.GOOS, runtime.GOARCH, s.Version, s.DataSchema)
		if l, ok := Latest(*s.manifest, runtime.GOOS, runtime.GOARCH, s.Version, s.DataSchema); ok {
			out.Latest = l.Version
			out.Newer = Compare(l.Version, s.Version) > 0
		}
	} else if st.Available != "" {
		out.Latest = st.Available
		out.Newer = Compare(st.Available, s.Version) > 0
	}
	return out
}

// SetLocked turns LockedBuild on or off. It exists only in Portable USB
// Mode and only as an explicit choice; turning it off returns to automatic
// updates right away.
func (s *Service) SetLocked(ctx context.Context, locked bool) error {
	if !s.Portable {
		return ErrPortableOnly
	}
	st := s.state()
	st.Locked = locked
	if locked {
		st.LockedAt = time.Now().UTC()
		if st.Current == "" {
			st.Current = s.Version
		}
	} else {
		st.LockedAt = time.Time{}
	}
	if err := SaveState(s.Builds, st); err != nil {
		return err
	}
	if !locked {
		go s.Check(context.WithoutCancel(ctx))
	}
	s.notify()
	return nil
}

func (s *Service) notify() {
	if s.Hooks.Notify != nil {
		s.Hooks.Notify(s.Status())
	}
}

func (s *Service) ua() string { return UserAgent(s.Version, s.Channel) }

// Check fetches the manifest. With automatic updates it also downloads the
// newest compatible stable build and applies it at the next safe point.
// LockedBuild only records that a newer build exists.
func (s *Service) Check(ctx context.Context) Status {
	m, err := Fetch(ctx, s.Client, s.URL, s.ua())
	s.mu.Lock()
	if err != nil {
		s.lastErr = err.Error()
		if isNetErr(err) {
			s.online = false
		}
		s.mu.Unlock()
		s.notify()
		return s.Status()
	}
	s.manifest, s.lastErr, s.online = &m, "", true
	s.mu.Unlock()
	st := s.state()
	st.LastCheck = time.Now().UTC()
	latest, ok := Latest(m, runtime.GOOS, runtime.GOARCH, s.Version, s.DataSchema)
	if ok {
		st.Available = latest.Version
	}
	SaveState(s.Builds, st)
	if ok && s.Auto() && Compare(latest.Version, s.Version) > 0 {
		if _, err := s.stage(ctx, latest); err == nil {
			s.ApplyWhenSafe(ctx)
		}
	}
	s.notify()
	return s.Status()
}

func isNetErr(err error) bool {
	var ne net.Error
	return errors.As(err, &ne)
}

func (s *Service) stage(ctx context.Context, r Release) (string, error) {
	a, ok := r.Asset(runtime.GOOS, runtime.GOARCH, "update")
	if !ok {
		return "", ErrNoAsset
	}
	if err := r.Compatibility(s.DataSchema); err != nil {
		return "", err
	}
	dir, err := Stage(ctx, s.Client, s.Builds, r, a, s.ua(), nil)
	s.mu.Lock()
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.staged = r.Version
	}
	s.mu.Unlock()
	return dir, err
}

// ApplyWhenSafe waits for a safe point (no critical work) and switches to
// the staged build, then restarts.
func (s *Service) ApplyWhenSafe(ctx context.Context) {
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			if s.Hooks.Busy == nil || !s.Hooks.Busy() {
				s.mu.Lock()
				v := s.staged
				s.mu.Unlock()
				if v != "" && s.Auto() {
					s.apply(ctx, v)
				}
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// Choose installs a specific stable build (LockedBuild's Update Build and
// the downgrade path). Downgrades that cannot open the current data are
// refused before anything is changed.
func (s *Service) Choose(ctx context.Context, version string) error {
	s.mu.Lock()
	m := s.manifest
	s.mu.Unlock()
	if m == nil {
		if s.Check(ctx); s.manifest == nil {
			return ErrManifest
		}
		m = s.manifest
	}
	var rel *Release
	for i := range m.Releases {
		if Compare(m.Releases[i].Version, version) == 0 && m.Releases[i].Channel == "stable" {
			rel = &m.Releases[i]
		}
	}
	if rel == nil {
		return ErrManifest
	}
	if err := rel.Compatibility(s.DataSchema); err != nil {
		return err
	}
	if s.Hooks.Busy != nil && s.Hooks.Busy() {
		return ErrBusy
	}
	if _, err := s.stage(ctx, *rel); err != nil {
		return err
	}
	return s.apply(ctx, rel.Version)
}

func (s *Service) apply(ctx context.Context, version string) error {
	s.mu.Lock()
	if s.applying {
		s.mu.Unlock()
		return ErrBusy
	}
	s.applying = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.applying = false
		s.mu.Unlock()
	}()
	s.notify()
	if s.Hooks.Prepare != nil {
		if err := s.Hooks.Prepare(ctx); err != nil {
			return err
		}
	}
	if err := Activate(s.Builds, version); err != nil {
		return err
	}
	if s.Portable && s.state().Locked {
		// LockedBuild pins the build the person just chose.
		st := s.state()
		st.LockedAt = time.Now().UTC()
		SaveState(s.Builds, st)
	}
	if s.Hooks.Restart != nil {
		return s.Hooks.Restart(filepath.Join(s.Builds, version, ExeName()))
	}
	return nil
}

// Run checks at start and then periodically until ctx ends.
func (s *Service) Run(ctx context.Context) {
	s.Check(ctx)
	t := time.NewTicker(CheckInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Check(ctx)
		}
	}
}
