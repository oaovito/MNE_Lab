package export

import (
	"golang.org/x/sys/windows"
	"os"
)

func publishNoReplace(from, to string) error {
	source, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	dest, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	// No MOVEFILE_REPLACE_EXISTING: Windows refuses an existing destination.
	if err := windows.MoveFileEx(source, dest, windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: err}
	}
	return nil
}
