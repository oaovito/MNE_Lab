package export

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func publishNoReplace(from, to string) error {
	err := unix.RenameatxNp(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_EXCL)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EINVAL) {
		return linkNoReplace(from, to)
	}
	return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
}
