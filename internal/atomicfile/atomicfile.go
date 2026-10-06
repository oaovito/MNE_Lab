// Package atomicfile writes critical files so that a crash, power loss or a
// removed USB drive never leaves a half-written file in place.
//
// Write sequence: temporary file in the same directory → write → fsync →
// close → keep the previous version as <name>.prev → atomic rename → fsync
// of the directory where the platform supports it.
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
// passes checksum verification.
var ErrCorrupt = errors.New("atomicfile: file failed integrity verification")

// WriteFile atomically replaces path with data.
func WriteFile(path string, data []byte, perm os.FileMode) error {
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
	if _, err := os.Stat(path); err == nil {
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
	sum := sha256.Sum256(data)
	framed := make([]byte, 0, len(magic)+len(sum)+len(data))
	framed = append(framed, magic...)
	framed = append(framed, sum[:]...)
	framed = append(framed, data...)
	return WriteFile(path, framed, 0o600)
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
