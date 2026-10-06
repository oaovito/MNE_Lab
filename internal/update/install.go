package update

import (
	"archive/zip"
	"context"
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
	"sort"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/diskfree"
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

// LoadState reads the state (an empty state when there is none).
func LoadState(builds string) State {
	var s State
	if _, err := atomicfile.ReadJSON(filepath.Join(builds, StateFile), &s); err != nil {
		return State{}
	}
	return s
}

// SaveState writes the state atomically.
func SaveState(builds string, s State) error {
	return atomicfile.WriteJSON(filepath.Join(builds, StateFile), s)
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
	s := LoadState(builds)
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
	if err := SaveState(builds, s); err != nil {
		return "", "", err
	}
	return filepath.Join(builds, cur, ExeName()), cur, nil
}

// MarkBootOK records that a build started and showed its interface.
func MarkBootOK(builds, version string) error {
	s := LoadState(builds)
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
	return SaveState(builds, s)
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
	if !exists(filepath.Join(builds, version, ExeName())) {
		return errors.New("update.build_not_staged")
	}
	s := LoadState(builds)
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
	return SaveState(builds, s)
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
