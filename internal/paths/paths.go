// Package paths resolves where MNE Lab keeps its build, user data, temporary
// files and recovery state for each official execution mode.
//
// Portable USB Mode never stores absolute paths: everything is resolved from
// the folder that holds the application at launch time, so D:\MNE Lab\ can
// later become F:\MNE Lab\ without breaking anything.
package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/oaovito/mne_lab/internal/secure"
)

// Mode is an official execution mode.
type Mode string

const (
	Portable  Mode = "portable"
	Temporary Mode = "temporary"
)

// PortableMarker is the file that marks a folder as a Portable USB install.
const PortableMarker = "MNE Lab.portable"

// SessionPrefix names Temporary Machine Mode session folders.
const SessionPrefix = "MNE Lab session-"

// Layout is the resolved set of directories for one run.
type Layout struct {
	Mode Mode `json:"mode"`
	// Root is the portable folder, or the session folder in Temporary Mode.
	Root string `json:"-"`
	// Data holds accounts and profiles (User Storage).
	Data string `json:"-"`
	// Builds holds application builds (Software Distribution), separate from Data.
	Builds string `json:"-"`
	// Logs holds local diagnostic logs.
	Logs string `json:"-"`
	// Temp is scratch space that is always safe to delete.
	Temp string `json:"-"`
	// Window holds the window engine profile (caches, local storage).
	Window string `json:"-"`
	// Recovery holds encrypted recovery packages that must survive cleanup
	// when work is not yet confirmed by the cloud.
	Recovery string `json:"-"`
	// Exports is the default destination for user exports. In Temporary Mode
	// it lies outside the session folder so cleanup never deletes exports.
	Exports string `json:"-"`
	// Sessions lists the folders that hold Temporary Mode sessions, the
	// current one first, then where earlier builds kept them.
	Sessions []string `json:"-"`
	// LegacyRecovery lists where earlier builds kept recovery packages;
	// they are still read, and new packages go to Recovery.
	LegacyRecovery []string `json:"-"`
}

// Options control layout detection.
type Options struct {
	// Root forces a portable root (passed by the launcher).
	Root string
	// ForceMode overrides detection (used by tests and the first-run chooser).
	ForceMode Mode
	// BaseTemp overrides the OS temp directory (tests).
	BaseTemp string
	// UserHome overrides the user home directory (tests).
	UserHome string
}

// Detect resolves the layout for this run.
func Detect(exe string, opt Options) (*Layout, error) {
	if opt.ForceMode == Temporary {
		return temporary(opt)
	}
	root := opt.Root
	if root == "" {
		root = FindPortableRoot(exe)
	}
	if root != "" {
		return portable(root)
	}
	if opt.ForceMode == Portable {
		return nil, errors.New("paths: portable mode requested but no portable folder was found")
	}
	return temporary(opt)
}

// FindPortableRoot looks for the portable marker next to the executable or
// two levels above it (app/<version>/mnelab.exe).
func FindPortableRoot(exe string) string {
	if exe == "" {
		return ""
	}
	dir := filepath.Dir(exe)
	for _, cand := range []string{dir, filepath.Dir(filepath.Dir(dir))} {
		if _, err := os.Stat(filepath.Join(cand, PortableMarker)); err == nil {
			return cand
		}
	}
	return ""
}

func portable(root string) (*Layout, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	l := &Layout{
		Mode:     Portable,
		Root:     root,
		Data:     filepath.Join(root, "data"),
		Builds:   filepath.Join(root, "app"),
		Logs:     filepath.Join(root, "data", "logs"),
		Temp:     filepath.Join(root, "data", "tmp"),
		Window:   filepath.Join(root, "data", "window"),
		Recovery: filepath.Join(root, "data", "recovery"),
		Exports:  filepath.Join(root, "Exports"),
	}
	return l, l.ensure()
}

func temporary(opt Options) (*Layout, error) {
	roots := SessionRoots(opt)
	if err := os.MkdirAll(roots[0], 0o700); err != nil {
		return nil, fmt.Errorf("paths: cannot prepare sessions: %w", err)
	}
	return sessionLayout(filepath.Join(roots[0], SessionPrefix+secure.NewID()[:12]), opt)
}

func sessionLayout(session string, opt Options) (*Layout, error) {
	recovery := RecoveryRoots(opt)
	l := &Layout{
		Mode:           Temporary,
		Root:           session,
		Data:           filepath.Join(session, "data"),
		Builds:         filepath.Join(session, "app"),
		Logs:           filepath.Join(session, "logs"),
		Temp:           filepath.Join(session, "tmp"),
		Window:         filepath.Join(session, "window"),
		Recovery:       recovery[0],
		Exports:        defaultExports(opt),
		Sessions:       SessionRoots(opt),
		LegacyRecovery: recovery[1:],
	}
	return l, l.ensure()
}

// stateDir is the per-user folder for state that must outlive a reboot and
// is not a cache: the system temporary folder can be memory that a restart
// clears, and cache folders may be emptied at any time.
func stateDir() string {
	switch runtime.GOOS {
	case "windows":
		if la := os.Getenv("LOCALAPPDATA"); la != "" {
			return filepath.Join(la, "MNE Lab")
		}
	case "darwin":
		if d, err := os.UserConfigDir(); err == nil {
			return filepath.Join(d, "MNE Lab")
		}
	default:
		if d := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(d) {
			return filepath.Join(d, "MNE Lab")
		}
		if h, err := os.UserHomeDir(); err == nil && h != "" {
			return filepath.Join(h, ".local", "state", "MNE Lab")
		}
	}
	return ""
}

// SessionRoots lists the folders holding Temporary Mode sessions: where
// new sessions start, then the system temporary folder earlier builds used,
// which may still hold an interrupted session.
func SessionRoots(opt Options) []string {
	if opt.BaseTemp != "" {
		return []string{opt.BaseTemp}
	}
	if d := stateDir(); d != "" {
		return unique(filepath.Join(d, "Sessions"), os.TempDir())
	}
	return []string{os.TempDir()}
}

// RecoveryRoots lists the stable per-user folders for recovery packages,
// so the next Temporary Mode run on this machine finds unsynchronized work
// after an offline exit: where new packages go, then where earlier builds
// put them.
func RecoveryRoots(opt Options) []string {
	if opt.BaseTemp != "" {
		return []string{filepath.Join(opt.BaseTemp, "MNE Lab Recovery")}
	}
	var out []string
	if d := stateDir(); d != "" {
		out = append(out, filepath.Join(d, "Recovery"))
	}
	if d, err := os.UserCacheDir(); err == nil {
		out = append(out, filepath.Join(d, "MNE Lab", "Recovery"))
	}
	return unique(append(out, filepath.Join(os.TempDir(), "MNE Lab Recovery"))...)
}

func unique(dirs ...string) []string {
	var out []string
	for _, d := range dirs {
		if !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}

func defaultExports(opt Options) string {
	home := opt.UserHome
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	if home == "" {
		return filepath.Join(os.TempDir(), "MNE Lab Exports")
	}
	return filepath.Join(home, "Downloads", "MNE Lab Exports")
}

func (l *Layout) ensure() error {
	for _, d := range []string{l.Data, l.Logs, l.Temp, l.Window} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("paths: cannot prepare %s: %w", filepath.Base(d), err)
		}
	}
	return nil
}

// Rel converts an absolute path inside Root into a portable relative path.
func (l *Layout) Rel(abs string) (string, error) {
	r, err := filepath.Rel(l.Root, abs)
	if err != nil || strings.HasPrefix(r, "..") {
		return "", fmt.Errorf("paths: %s is outside the application folder", abs)
	}
	return filepath.ToSlash(r), nil
}

// Abs resolves a relative path stored by Rel.
func (l *Layout) Abs(rel string) string { return filepath.Join(l.Root, filepath.FromSlash(rel)) }

// Within reports whether p is inside dir (after cleaning).
func Within(dir, p string) bool {
	r, err := filepath.Rel(dir, p)
	return err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}

// StaleSessions lists Temporary Mode session folders left behind by a run
// that did not exit normally (crash, power loss). They may hold the only
// copy of unsynchronized work, so they are recovered, never blindly deleted.
func StaleSessions(baseTemp, current string) []string {
	if baseTemp == "" {
		baseTemp = os.TempDir()
	}
	entries, err := os.ReadDir(baseTemp)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), SessionPrefix) {
			p := filepath.Join(baseTemp, e.Name())
			if p != current {
				out = append(out, p)
			}
		}
	}
	return out
}

// Resume returns the layout of an existing Temporary Mode session folder
// (a restart for Turbo or an update keeps the session; it is not an exit).
// A session an earlier build started in the system temporary folder is
// resumed where it is.
func Resume(session string, opt Options) (*Layout, error) {
	session, err := filepath.Abs(session)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(filepath.Base(session), SessionPrefix) || !slices.Contains(SessionRoots(opt), filepath.Dir(session)) {
		return nil, errors.New("paths: not a Temporary Mode session folder")
	}
	if st, err := os.Stat(session); err != nil || !st.IsDir() {
		return nil, errors.New("paths: session folder not found")
	}
	return sessionLayout(session, opt)
}
