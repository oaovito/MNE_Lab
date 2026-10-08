// Package diskfree reports the space available to the user on a volume.
package diskfree

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNoSpace is returned when a volume cannot hold what is about to be written.
var ErrNoSpace = errors.New("storage.no_space")

// Available returns the free bytes usable by the current user on the
// volume holding path (or its nearest existing parent).
func Available(path string) (uint64, error) {
	p := filepath.Clean(path)
	for {
		if _, err := os.Stat(p); err == nil {
			break
		}
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	return available(p)
}

// Ensure checks that need bytes (plus a safety margin) fit on the volume.
// When the free space cannot be determined the write is allowed and any
// failure is caught by the write itself.
func Ensure(path string, need int64) error {
	free, err := Available(path)
	if err != nil {
		return nil
	}
	margin := uint64(8 << 20)
	if need < 0 {
		need = 0
	}
	if uint64(need)+margin > free {
		return ErrNoSpace
	}
	return nil
}
