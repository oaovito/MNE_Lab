// Command mnelab is the MNE Lab application.
//
// Started directly (a downloaded executable) it runs in Temporary Machine
// Mode; started by the Portable USB launcher it runs from the drive.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/oaovito/mne_lab/internal/app"
	"github.com/oaovito/mne_lab/internal/brand"
	"github.com/oaovito/mne_lab/internal/instance"
	"github.com/oaovito/mne_lab/internal/restartpipe"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/shell"
	"github.com/oaovito/mne_lab/internal/store"
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
	handoffV1 := flag.Bool("handoff-v1", false, "use a confirmed restart transaction over private pipes")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.String())
		return
	}
	if err := run(*root, *build, *session, *handoff, *handoffV1); err != nil {
		if *handoff || *handoffV1 {
			// A modal error in the child would block the parent's rollback.
			fmt.Fprintln(os.Stderr, "MNE Lab could not restore the restart session.")
		} else {
			fail(err)
		}
		os.Exit(1)
	}
}

func run(root, build, session string, handoff, handoffV1 bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	if !handoff && !handoffV1 && session == "" && app.FocusRunning(exe, root, "") {
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
	if handoffV1 {
		// No shared layout/log/session state is touched before preparation.
		pipe := restartpipe.New(os.Stdin, os.Stdout)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		stop := context.AfterFunc(ctx, func() {
			os.Stdin.Close()
			os.Stdout.Close()
		})
		defer func() { stop(); cancel() }()
		if err := pipe.Write("prepared", store.SchemaVersion, nil); err != nil {
			return err
		}
		message, err := pipe.Read("resume")
		defer restartpipe.Wipe(message.Payload)
		if err != nil || message.Schema != store.SchemaVersion || len(message.Payload) == 0 {
			return restartpipe.ErrProtocol
		}
		opt.Handoff = bytes.NewReader(message.Payload)
		opt.HandoffConfirm = func() error {
			restartpipe.Wipe(message.Payload)
			if err := pipe.Write("ready", 0, nil); err != nil {
				return err
			}
			if _, err := pipe.Read("commit"); err != nil {
				return err
			}
			if !stop() || ctx.Err() != nil {
				return errors.New("restart.handoff_timeout")
			}
			cancel()
			if err := pipe.Write("committed", 0, nil); err != nil {
				return err
			}
			os.Stdin.Close()
			os.Stdout.Close()
			return nil
		}
	}
	a, err := app.New(opt)
	if err != nil {
		return err
	}
	defer a.Close()
	window := a.L.Window
	if handoffV1 {
		// A separate browser profile prevents the replacement window from
		// being handed to the Chromium process the parent will close.
		window = filepath.Join(a.L.Temp, "restart-window-"+secure.NewID()[:12])
		defer os.RemoveAll(window)
	}
	a.SetShell(shell.New(window, brand.TrayIcon(), a.Lang, a.Log))
	err = a.Run(ui)
	if errors.Is(err, instance.ErrRunning) && !handoff && !handoffV1 {
		app.FocusRunning(exe, root, "")
		return nil
	}
	return err
}
