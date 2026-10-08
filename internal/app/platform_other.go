//go:build !windows

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
)

// deleteAfterExit removes the session folder after this process ends, in
// case something still held files open during the exit.
func deleteAfterExit(root string) {
	cmd := exec.Command("/bin/sh", "-c", `i=0; while [ $i -lt 60 ]; do [ -e "$1" ] || exit 0; rm -rf -- "$1" 2>/dev/null; i=$((i+1)); sleep 1; done`, "sh", root)
	cmd.Dir = filepath.Dir(root)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Start()
}

// removableDrives lists mounted removable volumes.
func removableDrives() []string {
	var bases []string
	if runtime.GOOS == "darwin" {
		bases = []string{"/Volumes"}
	} else if u := os.Getenv("USER"); u != "" {
		bases = []string{filepath.Join("/media", u), filepath.Join("/run/media", u)}
	}
	var out []string
	for _, b := range bases {
		entries, err := os.ReadDir(b)
		if err != nil {
			continue
		}
		for _, e := range entries {
			p := filepath.Join(b, e.Name())
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				if runtime.GOOS == "darwin" {
					if l, err := os.Readlink(p); err == nil && l == "/" {
						continue // the system volume
					}
				}
				out = append(out, p)
			}
		}
	}
	return out
}
