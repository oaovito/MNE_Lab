// Package backup keeps verified, rotating snapshots of profile stores.
//
// A snapshot is a consistent copy of the encrypted store (values stay
// sealed), written atomically and registered in a checksummed manifest.
// Every snapshot is re-opened and integrity-checked before it counts.
package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
)

// Keep is the number of snapshots retained.
const Keep = 8

// Entry describes one snapshot.
type Entry struct {
	File     string    `json:"file"`
	Created  time.Time `json:"created"`
	Size     int64     `json:"size"`
	SHA256   string    `json:"sha256"`
	Reason   string    `json:"reason"`
	Schema   int       `json:"schema"`
	Verified bool      `json:"verified"`
}

// Manifest lists snapshots, newest first.
type Manifest struct {
	Schema  int     `json:"schema"`
	Entries []Entry `json:"entries"`
}

func manifestPath(dir string) string { return filepath.Join(dir, "manifest.json") }

// Load reads the manifest of a backup directory.
func Load(dir string) (Manifest, error) {
	var m Manifest
	_, err := atomicfile.ReadJSON(manifestPath(dir), &m)
	if errors.Is(err, os.ErrNotExist) {
		return Manifest{Schema: 1}, nil
	}
	return m, err
}

// Create writes and verifies a snapshot of s into dir.
func Create(s *store.Store, key secure.Key, dir, reason string) (Entry, error) {
	var buf bytes.Buffer
	if _, err := s.Snapshot(&buf); err != nil {
		return Entry{}, err
	}
	schema, _ := s.Schema()
	now := time.Now().UTC()
	// Multiple snapshots within a millisecond must not overwrite each other;
	// rotation could otherwise delete a file still referenced by the manifest.
	name := now.Format("20060102T150405.000") + "-" + secure.NewID() + ".mnebak"
	p := filepath.Join(dir, name)
	if err := atomicfile.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		return Entry{}, err
	}
	sum := sha256.Sum256(buf.Bytes())
	e := Entry{File: name, Created: now, Size: int64(buf.Len()), SHA256: hex.EncodeToString(sum[:]), Reason: reason, Schema: schema}
	if err := Verify(dir, e, key); err != nil {
		os.Remove(p)
		return Entry{}, fmt.Errorf("backup: new snapshot failed verification: %w", err)
	}
	e.Verified = true
	m, err := Load(dir)
	if err != nil {
		m = Manifest{Schema: 1}
	}
	m.Entries = append([]Entry{e}, m.Entries...)
	var drop []Entry
	if len(m.Entries) > Keep {
		drop = m.Entries[Keep:]
		m.Entries = m.Entries[:Keep]
	}
	if err := atomicfile.WriteJSON(manifestPath(dir), m); err != nil {
		return Entry{}, err
	}
	for _, d := range drop {
		os.Remove(filepath.Join(dir, d.File))
	}
	return e, nil
}

// Verify checks a snapshot's checksum and opens it to validate structure
// and authentication of every value.
func Verify(dir string, e Entry, key secure.Key) error {
	p := filepath.Join(dir, e.File)
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != e.SHA256 {
		return errors.New("backup: checksum mismatch")
	}
	tmp, err := os.CreateTemp("", "mnelab-verify-*.db")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	c, err := store.Open(tmpName, key, store.Options{ReadOnly: true})
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Check()
}

// Latest returns the newest snapshot that passes verification.
func Latest(dir string, key secure.Key) (Entry, error) {
	m, err := Load(dir)
	if err != nil {
		return Entry{}, err
	}
	sort.SliceStable(m.Entries, func(i, j int) bool { return m.Entries[i].Created.After(m.Entries[j].Created) })
	for _, e := range m.Entries {
		if Verify(dir, e, key) == nil {
			return e, nil
		}
	}
	return Entry{}, errors.New("backup: no verified snapshot available")
}

// Restore replaces the store file at dbPath with snapshot e. The current
// file is preserved next to it as <db>.before-restore.
func Restore(dir string, e Entry, key secure.Key, dbPath string) error {
	if err := Verify(dir, e, key); err != nil {
		return err
	}
	b, err := os.ReadFile(filepath.Join(dir, e.File))
	if err != nil {
		return err
	}
	if _, err := os.Stat(dbPath); err == nil {
		keep := dbPath + ".before-restore"
		os.Remove(keep)
		if err := os.Rename(dbPath, keep); err != nil {
			return err
		}
	}
	return atomicfile.WriteFile(dbPath, b, 0o600)
}
