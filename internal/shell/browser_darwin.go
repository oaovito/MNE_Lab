package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func findBrowser() string {
	apps := []string{
		"Google Chrome.app/Contents/MacOS/Google Chrome",
		"Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		"Brave Browser.app/Contents/MacOS/Brave Browser",
		"Chromium.app/Contents/MacOS/Chromium",
	}
	home, _ := os.UserHomeDir()
	for _, base := range []string{"/Applications", filepath.Join(home, "Applications")} {
		for _, a := range apps {
			p := filepath.Join(base, a)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	return ""
}

func hide(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func terminate(p *os.Process) { p.Signal(syscall.SIGTERM) }

func openExternal(u string) error { return exec.Command("open", u).Start() }

func raise(pid int) {}
