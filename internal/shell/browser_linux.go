//go:build !windows && !darwin

package shell

import (
	"os"
	"os/exec"
	"syscall"
)

func findBrowser() string {
	for _, n := range []string{"microsoft-edge", "microsoft-edge-stable", "google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "brave-browser"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}

func hide(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func terminate(p *os.Process) { p.Signal(syscall.SIGTERM) }

func openExternal(u string) error {
	cmd := exec.Command("xdg-open", u)
	cmd.Stdout, cmd.Stderr = nil, nil
	return cmd.Start()
}

func raise(pid int) {}
