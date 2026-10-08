// Package logging provides local, size-bounded, redacting diagnostic logs.
//
// Logs never leave the machine. Attribute keys that may carry secrets are
// redacted, and string values that look like bearer tokens are masked.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

const (
	maxFileBytes = 1 << 20 // 1 MiB per file
	keepFiles    = 4       // current + 4 rotated = 5 MiB maximum
)

var sensitiveKeys = []string{"password", "passphrase", "secret", "token", "key", "credential", "authorization", "cookie", "code_verifier", "recovery", "email", "username", "content", "data"}

var tokenLike = regexp.MustCompile(`[A-Za-z0-9_\-]{32,}`)

// RotatingFile is an io.Writer that rotates by size.
type RotatingFile struct {
	mu   sync.Mutex
	path string
	f    *os.File
	size int64
}

// OpenRotating opens (or creates) dir/name.log.
func OpenRotating(dir, name string) (*RotatingFile, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	r := &RotatingFile{path: filepath.Join(dir, name+".log")}
	return r, r.open()
}

func (r *RotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	st, _ := f.Stat()
	r.f = f
	if st != nil {
		r.size = st.Size()
	}
	return nil
}

func (r *RotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return len(p), nil
	}
	if r.size+int64(len(p)) > maxFileBytes {
		r.rotate()
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *RotatingFile) rotate() {
	r.f.Close()
	for i := keepFiles - 1; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.%d", r.path, i), fmt.Sprintf("%s.%d", r.path, i+1))
	}
	os.Rename(r.path, r.path+".1")
	os.Remove(fmt.Sprintf("%s.%d", r.path, keepFiles+1))
	r.size = 0
	if err := r.open(); err != nil {
		r.f = nil
	}
}

// Close closes the file.
func (r *RotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

// Redact masks secrets in a log attribute.
func Redact(_ []string, a slog.Attr) slog.Attr {
	k := strings.ToLower(a.Key)
	for _, s := range sensitiveKeys {
		if strings.Contains(k, s) {
			return slog.String(a.Key, "[redacted]")
		}
	}
	if a.Value.Kind() == slog.KindString {
		v := a.Value.String()
		if tokenLike.MatchString(v) {
			return slog.String(a.Key, tokenLike.ReplaceAllString(v, "[redacted]"))
		}
	}
	return a
}

// New returns a JSON logger writing to w with redaction.
func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level, ReplaceAttr: Redact}))
}

// Discard returns a logger that drops everything.
func Discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
