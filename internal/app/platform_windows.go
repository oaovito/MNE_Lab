package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// deleteAfterExit removes the session folder once this process has ended
// (Windows keeps a running executable locked). A hidden helper retries for
// up to a minute and only ever targets the MNE Lab session folder.
func deleteAfterExit(root string) {
	if strings.ContainsAny(root, "\"%^&|<>") {
		return
	}
	q := `"` + root + `"`
	script := `/C for /L %i in (1,1,60) do @(if exist ` + q + ` (rmdir /s /q ` + q + ` 2>nul & ping -n 2 127.0.0.1 >nul))`
	cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"))
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "cmd.exe " + script, HideWindow: true,
		CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP}
	cmd.Dir = filepath.Dir(root)
	cmd.Start()
}

// removableDrives lists mounted removable drives (USB sticks, cards).
func removableDrives() []string {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil
	}
	var out []string
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, _ := windows.UTF16PtrFromString(root)
		if windows.GetDriveType(p) == windows.DRIVE_REMOVABLE {
			out = append(out, root)
		}
	}
	return out
}
