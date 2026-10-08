package update

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/diskfree"
	"github.com/oaovito/mne_lab/internal/instance"
)

// StateFile lives in the builds folder (Software Distribution), never in
// User Data.
const StateFile = "current.json"

// MaxBootAttempts is how many failed starts a build gets before the
// launcher returns to the previous build.
const MaxBootAttempts = 3

// Boot records whether a build started correctly on this drive.
type Boot struct {
	Attempts int  `json:"attempts"`
	OK       bool `json:"ok"`
}

// State is the build selection of an installation.
type State struct {
	Current  string `json:"current"`
	Previous string `json:"previous,omitempty"`
	// SelectionID identifies the activation that owns Current/Previous,
	// including a later activation choosing the same build again.
	SelectionID string `json:"selectionId,omitempty"`
	// Locked is LockedBuild: the current build stays pinned until the
	// person chooses another one. Only Portable USB Mode can set it.
	Locked    bool            `json:"locked,omitempty"`
	LockedAt  time.Time       `json:"lockedAt,omitzero"`
	LastCheck time.Time       `json:"lastCheck,omitzero"`
	Available string          `json:"available,omitempty"` // newest stable seen
	Boots     map[string]Boot `json:"boots,omitempty"`
	Bad       []string        `json:"bad,omitempty"` // builds that failed to start here
}

// ExeName is the application executable inside a build folder.
func ExeName() string {
	if runtime.GOOS == "windows" {
		return "mnelab.exe"
	}
	return "mnelab"
}

// stateMu serializes state read-modify-write operations in this process.
// Network requests and restart hooks run outside it.
var stateMu sync.Mutex

// lockState also excludes launchers and handoff children modifying the
// same installation. Contention is bounded and returned as an error,
// rather than interpreting an unreadable or busy state as empty.
func lockState(builds string) (func(), error) {
	stateMu.Lock()
	deadline := time.Now().Add(2 * time.Second)
	for {
		lock, err := instance.Acquire(filepath.Join(builds, ".state-lock"))
		if err == nil {
			return func() {
				lock.Release()
				stateMu.Unlock()
			}, nil
		}
		if !errors.Is(err, instance.ErrRunning) || time.Now().After(deadline) {
			stateMu.Unlock()
			return nil, fmt.Errorf("update.state_lock: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// LoadState reads the state (an empty state when there is none).
func LoadState(builds string) State {
	stateMu.Lock()
	defer stateMu.Unlock()
	return loadState(builds)
}

func loadState(builds string) State {
	var s State
	if _, err := atomicfile.ReadJSON(filepath.Join(builds, StateFile), &s); err != nil {
		return State{}
	}
	return s
}

// SaveState writes the state atomically.
func SaveState(builds string, s State) error {
	unlock, err := lockState(builds)
	if err != nil {
		return err
	}
	defer unlock()
	return saveState(builds, s)
}

func saveState(builds string, s State) error {
	return atomicfile.WriteJSON(filepath.Join(builds, StateFile), s)
}

func changeState(builds string, change func(*State)) error {
	unlock, err := lockState(builds)
	if err != nil {
		return err
	}
	defer unlock()
	s := loadState(builds)
	change(&s)
	return saveState(builds, s)
}

func (s State) isBad(v string) bool {
	for _, b := range s.Bad {
		if b == v {
			return true
		}
	}
	return false
}

func exists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// Resolve picks the build to start (used by the launcher). A build that
// failed to start MaxBootAttempts times is set aside in favor of the
// previous build, so a bad update never leaves the drive unusable.
func Resolve(builds string) (exe, version string, err error) {
	unlock, err := lockState(builds)
	if err != nil {
		return "", "", err
	}
	defer unlock()
	s := loadState(builds)
	if s.Boots == nil {
		s.Boots = map[string]Boot{}
	}
	pick := func(v string) bool {
		return v != "" && !s.isBad(v) && exists(filepath.Join(builds, v, ExeName()))
	}
	cur := s.Current
	if b := s.Boots[cur]; pick(cur) && !b.OK && b.Attempts >= MaxBootAttempts {
		s.Bad = append(s.Bad, cur)
		if pick(s.Previous) {
			cur, s.Current, s.Previous = s.Previous, s.Previous, ""
		} else {
			cur = ""
		}
	}
	if !pick(cur) {
		cur = ""
		for _, v := range Installed(builds) {
			if pick(v) {
				cur = v
				break
			}
		}
		if cur == "" {
			return "", "", errors.New("update.no_build_installed")
		}
		s.Current = cur
	}
	if b := s.Boots[cur]; !b.OK {
		b.Attempts++
		s.Boots[cur] = b
	}
	if err := saveState(builds, s); err != nil {
		return "", "", err
	}
	return filepath.Join(builds, cur, ExeName()), cur, nil
}

// MarkBootOK records that a build started and showed its interface.
func MarkBootOK(builds, version string) error {
	unlock, err := lockState(builds)
	if err != nil {
		return err
	}
	defer unlock()
	s := loadState(builds)
	if s.Boots == nil {
		s.Boots = map[string]Boot{}
	}
	if b := s.Boots[version]; b.OK && s.Current == version {
		return nil
	}
	s.Boots[version] = Boot{OK: true}
	if s.Current == "" {
		s.Current = version
	}
	return saveState(builds, s)
}

// Installed lists build folders, newest first.
func Installed(builds string) []string {
	entries, _ := os.ReadDir(builds)
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasSuffix(e.Name(), ".partial") {
			if _, ok := ParseVersion(e.Name()); ok {
				out = append(out, e.Name())
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return Compare(out[i], out[j]) > 0 })
	return out
}

// UserAgent is the only information sent when checking or downloading.
func UserAgent(version, channel string) string {
	return fmt.Sprintf("MNE-Lab/%s (%s; %s; %s)", version, runtime.GOOS, runtime.GOARCH, channel)
}

// Fetch downloads and verifies the signed manifest.
func Fetch(ctx context.Context, c *http.Client, url, ua string) (Manifest, error) {
	if !Configured() {
		return Manifest{}, ErrNotConfigured
	}
	raw, err := get(ctx, c, url, ua, 4<<20)
	if err != nil {
		return Manifest{}, err
	}
	sig, err := get(ctx, c, url+".sig", ua, 4<<10)
	if err != nil {
		return Manifest{}, err
	}
	return Verify(raw, sig)
}

func get(ctx context.Context, c *http.Client, url, ua string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update.http_%d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, ErrManifest
	}
	return b, nil
}

// Progress reports download progress.
type Progress func(done, total int64)

// Stage downloads a build, verifies its size and SHA-256 against the
// signed manifest, and unpacks it into builds/<version>. Nothing in use is
// touched; Activate switches builds afterwards.
func Stage(ctx context.Context, c *http.Client, builds string, r Release, a Asset, ua string, progress Progress) (string, error) {
	if !Configured() {
		return "", ErrNotConfigured
	}
	if r.Revoked {
		return "", ErrRevoked
	}
	final := filepath.Join(builds, r.Version)
	if exists(filepath.Join(final, ExeName())) {
		return final, nil
	}
	if err := os.MkdirAll(builds, 0o755); err != nil {
		return "", err
	}
	if err := diskfree.Ensure(builds, a.Size*3); err != nil {
		return "", err
	}
	zipPath := filepath.Join(builds, ".download-"+r.Version+".zip")
	defer os.Remove(zipPath)
	if err := download(ctx, c, a, zipPath, ua, progress); err != nil {
		return "", err
	}
	partial := final + ".partial"
	os.RemoveAll(partial)
	if err := unpack(zipPath, partial, a.Size*8); err != nil {
		os.RemoveAll(partial)
		return "", err
	}
	if !exists(filepath.Join(partial, ExeName())) {
		os.RemoveAll(partial)
		return "", ErrManifest
	}
	if runtime.GOOS != "windows" {
		os.Chmod(filepath.Join(partial, ExeName()), 0o755)
	}
	os.RemoveAll(final)
	if err := os.Rename(partial, final); err != nil {
		os.RemoveAll(partial)
		return "", err
	}
	return final, nil
}

func download(ctx context.Context, c *http.Client, a Asset, dst, ua string, progress Progress) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", ua)
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update.http_%d", resp.StatusCode)
	}
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	h := sha256.New()
	var done int64
	buf := make([]byte, 64<<10)
	body := io.LimitReader(resp.Body, a.Size+1)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			done += int64(n)
			if done > a.Size {
				f.Close()
				return ErrChecksum
			}
			h.Write(buf[:n])
			if _, err := f.Write(buf[:n]); err != nil {
				f.Close()
				return err
			}
			if progress != nil {
				progress(done, a.Size)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return rerr
		}
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	f.Close()
	if done != a.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), a.SHA256) {
		return ErrChecksum
	}
	return nil
}

func unpack(zipPath, dst string, limit int64) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return ErrManifest
	}
	defer zr.Close()
	var total int64
	for _, f := range zr.File {
		name := path.Clean(strings.ReplaceAll(f.Name, "\\", "/"))
		if name == "." || strings.HasPrefix(name, "../") || name == ".." || path.IsAbs(name) || strings.Contains(name, ":") {
			return ErrManifest
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return ErrManifest
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		total += int64(f.UncompressedSize64)
		if total > limit {
			return ErrManifest
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return ErrManifest
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, io.LimitReader(rc, int64(f.UncompressedSize64)+1))
		rc.Close()
		if err == nil {
			err = out.Sync()
		}
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// Activate makes a staged build current, keeping the one in use as the
// fallback. LockedBuild stays as it is: in LockedBuild only an explicit
// choice of the person calls Activate.
func Activate(builds, version string) error {
	_, err := activate(builds, version, false)
	return err
}

// activation holds only the fields needed to undo this selection if its
// restart fails. A later selection owns its state and must not be undone.
type activation struct {
	before, after State
	version       string
}

func cloneState(s State) State {
	if s.Boots != nil {
		boots := make(map[string]Boot, len(s.Boots))
		for version, boot := range s.Boots {
			boots[version] = boot
		}
		s.Boots = boots
	}
	s.Bad = slices.Clone(s.Bad)
	return s
}

func activate(builds, version string, pinLocked bool) (activation, error) {
	if !exists(filepath.Join(builds, version, ExeName())) {
		return activation{}, errors.New("update.build_not_staged")
	}
	unlock, err := lockState(builds)
	if err != nil {
		return activation{}, err
	}
	defer unlock()
	before := loadState(builds)
	s := cloneState(before)
	var selection [16]byte
	if _, err := rand.Read(selection[:]); err != nil {
		return activation{}, err
	}
	s.SelectionID = hex.EncodeToString(selection[:])
	if s.Current != version {
		s.Previous, s.Current = s.Current, version
	}
	if s.Boots == nil {
		s.Boots = map[string]Boot{}
	}
	s.Boots[version] = Boot{}
	var bad []string
	for _, b := range s.Bad {
		if b != version {
			bad = append(bad, b)
		}
	}
	s.Bad = bad
	if pinLocked && s.Locked {
		s.LockedAt = time.Now().UTC()
	}
	return activation{before: before, after: s, version: version}, saveState(builds, s)
}

func (a activation) rollback(builds string) error {
	unlock, err := lockState(builds)
	if err != nil {
		return fmt.Errorf("update.activation_rollback: %w", err)
	}
	defer unlock()
	s := loadState(builds)
	if s.Current != a.after.Current || s.Previous != a.after.Previous || s.SelectionID != a.after.SelectionID {
		return nil
	}
	s.Current, s.Previous = a.before.Current, a.before.Previous
	s.SelectionID = a.before.SelectionID
	if s.Locked == a.after.Locked && s.LockedAt.Equal(a.after.LockedAt) {
		s.LockedAt = a.before.LockedAt
	}
	if boot, ok := s.Boots[a.version]; ok && boot == a.after.Boots[a.version] {
		if previous, ok := a.before.Boots[a.version]; ok {
			s.Boots[a.version] = previous
		} else {
			delete(s.Boots, a.version)
			if len(s.Boots) == 0 && a.before.Boots == nil {
				s.Boots = nil
			}
		}
	}
	if slices.Equal(s.Bad, a.after.Bad) {
		s.Bad = slices.Clone(a.before.Bad)
	} else if a.before.isBad(a.version) && !s.isBad(a.version) {
		s.Bad = append(s.Bad, a.version)
	}
	// With no prior selection, Resolve would otherwise choose the newest
	// staged executable and immediately retry the restart that just failed.
	if a.before.Current == "" && !s.isBad(a.version) {
		s.Bad = append(s.Bad, a.version)
	}
	if err := saveState(builds, s); err != nil {
		return fmt.Errorf("update.activation_rollback: %w", err)
	}
	// Atomicfile keeps the superseded state as .prev. Refresh that checked
	// copy too, so a later damaged current.json cannot revive this failure.
	if err := saveState(builds, s); err != nil {
		return fmt.Errorf("update.activation_rollback: %w", err)
	}
	return nil
}

// Prune deletes builds other than the current and previous ones.
func Prune(builds string) {
	s := LoadState(builds)
	for _, v := range Installed(builds) {
		if v != s.Current && v != s.Previous {
			os.RemoveAll(filepath.Join(builds, v))
		}
	}
	entries, _ := os.ReadDir(builds)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".partial") || strings.HasPrefix(e.Name(), ".download-") {
			os.RemoveAll(filepath.Join(builds, e.Name()))
		}
	}
}
