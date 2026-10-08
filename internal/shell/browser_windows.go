package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func findBrowser() string {
	// Registered application paths first (Edge, then Chrome, Brave).
	for _, exe := range []string{"msedge.exe", "chrome.exe", "brave.exe"} {
		for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
			k, err := registry.OpenKey(root, `SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\`+exe, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			p, _, err := k.GetStringValue("")
			k.Close()
			if err == nil && fileExists(p) {
				return p
			}
		}
	}
	var cands []string
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LOCALAPPDATA"} {
		base := os.Getenv(env)
		if base == "" {
			continue
		}
		cands = append(cands,
			filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
			filepath.Join(base, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
			filepath.Join(base, "Chromium", "Application", "chrome.exe"))
	}
	for _, c := range cands {
		if fileExists(c) {
			return c
		}
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func hide(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

func terminate(p *os.Process) { p.Kill() }

func openExternal(u string) error {
	verb, _ := windows.UTF16PtrFromString("open")
	file, _ := windows.UTF16PtrFromString(u)
	return windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
	procIsIconic            = user32.NewProc("IsIconic")
)

// raise restores and focuses the visible top-level windows of the window
// process.
func raise(pid int) {
	cb := syscall.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		var owner uint32
		windows.GetWindowThreadProcessId(h, &owner)
		if int(owner) == pid && windows.IsWindowVisible(h) {
			if r, _, _ := procIsIconic.Call(uintptr(h)); r != 0 {
				procShowWindow.Call(uintptr(h), 9) // SW_RESTORE
			}
			procSetForegroundWindow.Call(uintptr(h))
		}
		return 1
	})
	windows.EnumWindows(cb, unsafe.Pointer(nil))
}
