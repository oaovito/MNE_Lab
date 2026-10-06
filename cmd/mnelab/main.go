// Command mnelab is the MNE Lab application.
//
// Started directly (a downloaded executable) it runs in Temporary Machine
// Mode; started by the Portable USB launcher it runs from the drive.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/oaovito/mne_lab/internal/app"
	"github.com/oaovito/mne_lab/internal/brand"
	"github.com/oaovito/mne_lab/internal/instance"
	"github.com/oaovito/mne_lab/internal/shell"
	"github.com/oaovito/mne_lab/internal/version"
	"github.com/oaovito/mne_lab/web"
)

func init() {
	// The tray loop needs the main thread on some systems.
	runtime.LockOSThread()
}

func main() {
	root := flag.String("root", "", "portable installation folder (set by the launcher)")
	build := flag.String("build", "", "build folder name (set by the launcher)")
	session := flag.String("session", "", "Temporary Mode session folder to resume after a restart")
	handoff := flag.Bool("handoff", false, "read the open session from the restarting process")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.String())
		return
	}
	if err := run(*root, *build, *session, *handoff); err != nil {
		fail(err)
		os.Exit(1)
	}
}

func run(root, build, session string, handoff bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	if !handoff && session == "" && app.FocusRunning(exe, root, "") {
		return nil // the running MNE Lab shows its window
	}
	ui, ok := web.FS()
	if !ok {
		return errors.New("this build does not include the interface")
	}
	opt := app.Options{Exe: exe, Root: root, Build: build, Session: session}
	if handoff {
		opt.Handoff = os.Stdin
	}
	a, err := app.New(opt)
	if err != nil {
		return err
	}
	a.SetShell(shell.New(a.L.Window, brand.TrayIcon(), a.Lang, a.Log))
	err = a.Run(ui)
	if errors.Is(err, instance.ErrRunning) {
		app.FocusRunning(exe, root, "")
		return nil
	}
	return err
}
