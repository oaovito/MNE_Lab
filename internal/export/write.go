package export

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/oaovito/mne_lab/internal/diskfree"
	"github.com/oaovito/mne_lab/internal/secure"
)

// Collision policies offered when a file already exists. There is no
// silent overwrite: without a policy an existing file stops the export.
const (
	CollisionAsk      = ""
	CollisionReplace  = "replace"
	CollisionKeepBoth = "keep_both"
	CollisionCancel   = "cancel"
)

// Errors (stable identifiers).
var (
	ErrExists      = errors.New("export.file_exists")
	ErrCanceled    = errors.New("export.canceled")
	ErrDestination = errors.New("export.destination_unavailable")
	ErrInvalidName = errors.New("export.invalid_name")
)

const partialPrefix = ".mnelab-partial-"

// Saved describes a finished export file.
type Saved struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// Exists reports whether name exists in dir.
func Exists(dir, name string) bool {
	_, err := os.Lstat(filepath.Join(dir, name))
	return err == nil
}

// Save writes a file atomically: content goes to a hidden partial file in
// the destination folder, is flushed to disk, and only then takes its
// final name. A failed or interrupted export leaves no broken file behind.
func Save(dir, name, policy string, estimate int64, write func(io.Writer) error) (Saved, error) {
	if policy == CollisionCancel {
		return Saved{}, ErrCanceled
	}
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, partialPrefix) {
		return Saved{}, ErrInvalidName
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return Saved{}, ErrDestination
	}
	if Exists(dir, name) {
		switch policy {
		case CollisionReplace:
		case CollisionKeepBoth:
			// Choose and claim a unique name after the bytes are finalized.
		default:
			return Saved{}, ErrExists
		}
	}
	if err := diskfree.Ensure(dir, estimate); err != nil {
		return Saved{}, err
	}
	tmp := filepath.Join(dir, partialPrefix+secure.NewID()[:12])
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return Saved{}, ErrDestination
	}
	ok := false
	defer func() {
		if !ok {
			f.Close()
			os.Remove(tmp)
		}
	}()
	bw := bufio.NewWriterSize(f, 256<<10)
	cw := &countWriter{w: bw}
	if err := write(cw); err != nil {
		return Saved{}, err
	}
	if err := bw.Flush(); err != nil {
		return Saved{}, mapWriteErr(err)
	}
	if err := f.Sync(); err != nil {
		return Saved{}, mapWriteErr(err)
	}
	if err := f.Close(); err != nil {
		return Saved{}, mapWriteErr(err)
	}
	final := filepath.Join(dir, name)
	if policy == CollisionReplace {
		if err := os.Rename(tmp, final); err != nil {
			return Saved{}, mapWriteErr(err)
		}
	} else {
		// Existence checks only suggest names. The filesystem operation must
		// refuse replacement atomically, including on FAT32/exFAT where hard
		// links are unavailable. Retry every keep-both collision this way.
		for attempt := 0; ; attempt++ {
			err := publishNoReplace(tmp, final)
			if err == nil {
				break
			}
			if !errors.Is(err, os.ErrExist) {
				return Saved{}, mapWriteErr(err)
			}
			if policy != CollisionKeepBoth || attempt >= 9999 {
				return Saved{}, ErrExists
			}
			final = filepath.Join(dir, KeepBothName(name, func(n string) bool { return Exists(dir, n) }))
		}
	}
	ok = true
	syncDir(dir)
	return Saved{Path: final, Name: filepath.Base(final), Size: cw.n}, nil
}

// Bytes is a convenience writer for in-memory content.
func Bytes(b []byte) func(io.Writer) error {
	return func(w io.Writer) error {
		_, err := w.Write(b)
		return err
	}
}

// CleanPartials removes partial files left by an interrupted export.
func CleanPartials(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), partialPrefix) && !e.IsDir() {
			os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func mapWriteErr(err error) error {
	if isNoSpace(err) {
		return diskfree.ErrNoSpace
	}
	return err
}

func isNoSpace(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "no space") || strings.Contains(s, "disk full") || strings.Contains(s, "not enough space")
}

func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}
