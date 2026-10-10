package export

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func publishNoReplace(from, to string) error {
	err := unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
		return linkNoReplace(from, to)
	}
	return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
}
