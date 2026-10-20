// Package atomicfile writes critical files so that a crash, power loss or a
// removed USB drive never leaves a half-written file in place.
//
// Write sequence: temporary file in the same directory → write → fsync →
// close → keep the previous version as <name>.prev → atomic rename → fsync
// of the directory where the platform supports it.
//
// Files that wrap secrets use WriteSecret and ReadSecret instead, which keep
// and consult no previous version: a superseded wrapping would still open
// with whatever it was wrapped by, such as an old passphrase.
package atomicfile

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var magic = []byte("MNE1")

// ErrCorrupt is returned when neither the file nor its previous version
// passes checksum verification (for ReadSecret, when the file itself fails).
var ErrCorrupt = errors.New("atomicfile: file failed integrity verification")

// WriteFile atomically replaces path with data.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	return write(path, data, perm, true)
}

// ReplaceFile writes data like WriteFile but keeps no previous version, for
// callers that keep their own copy of what is replaced.
func ReplaceFile(path string, data []byte, perm os.FileMode) error {
	return write(path, data, perm, false)
}

func write(path string, data []byte, perm os.FileMode, keepPrev bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil && runtime.GOOS != "windows" {
		return err
	}
	// Validate what reached the disk before it replaces anything.
	back, err := os.ReadFile(tmpName)
	if err != nil || !bytes.Equal(back, data) {
		return fmt.Errorf("atomicfile: verification of %s failed", filepath.Base(path))
	}
	if _, err := os.Stat(path); err == nil && keepPrev {
		prev := path + ".prev"
		os.Remove(prev)
		if err := copyFile(path, prev); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	syncDir(dir)
	if !keepPrev {
		// Only now may an earlier .prev go: until the rename is durable it
		// could still be the sole intact copy for a manual repair.
		RemovePrevious(path)
	}
	return nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}

// WriteChecked writes data framed with a SHA-256 checksum so later reads can
// detect corruption and fall back to the previous version.
func WriteChecked(path string, data []byte) error {
	return WriteFile(path, frame(data), 0o600)
}

func frame(data []byte) []byte {
	sum := sha256.Sum256(data)
	framed := make([]byte, 0, len(magic)+len(sum)+len(data))
	framed = append(framed, magic...)
	framed = append(framed, sum[:]...)
	return append(framed, data...)
}

// ReadChecked reads a file written by WriteChecked, falling back to the
// previous version when the current one is damaged. recovered reports
// whether the fallback was used.
func ReadChecked(path string) (data []byte, recovered bool, err error) {
	data, err = readFramed(path)
	if err == nil {
		return data, false, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		if _, perr := os.Stat(path + ".prev"); perr != nil {
			return nil, false, err
		}
	}
	prev, perr := readFramed(path + ".prev")
	if perr != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, err
		}
		return nil, false, ErrCorrupt
	}
	return prev, true, nil
}

func readFramed(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(b) < len(magic)+sha256.Size || !bytes.Equal(b[:len(magic)], magic) {
		return nil, ErrCorrupt
	}
	payload := b[len(magic)+sha256.Size:]
	sum := sha256.Sum256(payload)
	if !bytes.Equal(sum[:], b[len(magic):len(magic)+sha256.Size]) {
		return nil, ErrCorrupt
	}
	return payload, nil
}

// WriteSecret writes data like WriteChecked but keeps no previous version.
// A <name>.prev left by an earlier WriteChecked is removed once the new file
// is in place. An error means path still holds its earlier content; a failed
// removal is not reported, since ReadSecret never consults the copy and the
// caller can retry RemovePrevious.
func WriteSecret(path string, data []byte) error {
	return write(path, frame(data), 0o600, false)
}

// ReadSecret reads a file written by WriteSecret or WriteChecked without
// ever falling back to a previous version: a missing file reports
// os.ErrNotExist and a damaged one ErrCorrupt.
func ReadSecret(path string) ([]byte, error) { return readFramed(path) }

// WriteSecretJSON marshals v and writes it with WriteSecret.
func WriteSecretJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteSecret(path, b)
}

// ReadSecretJSON reads a file written by WriteSecretJSON into v.
func ReadSecretJSON(path string, v any) error {
	b, err := ReadSecret(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// RemovePrevious deletes the previous version of path, if any.
func RemovePrevious(path string) error {
	err := os.Remove(path + ".prev")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err == nil {
		syncDir(filepath.Dir(path))
	}
	return err
}

// WriteJSON marshals v and writes it with WriteChecked.
func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteChecked(path, b)
}

// ReadJSON reads a file written by WriteJSON into v.
func ReadJSON(path string, v any) (recovered bool, err error) {
	b, recovered, err := ReadChecked(path)
	if err != nil {
		return false, err
	}
	return recovered, json.Unmarshal(b, v)
}

// Remove deletes path and its previous version.
func Remove(path string) error {
	os.Remove(path + ".prev")
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
