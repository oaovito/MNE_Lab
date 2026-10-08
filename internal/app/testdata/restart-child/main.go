// This executable is a synthetic process fixture for restart integration tests.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"testing/fstest"
	"time"

	"github.com/oaovito/mne_lab/internal/app"
	"github.com/oaovito/mne_lab/internal/restartpipe"
	"github.com/oaovito/mne_lab/internal/store"
)

type testShell struct {
	a    *app.App
	fail bool
}

func (s *testShell) OpenWindow(string, bool) error {
	if s.fail {
		return errors.New("synthetic window failure")
	}
	return nil
}
func (*testShell) Raise() bool               { return false }
func (*testShell) CloseWindow()              {}
func (*testShell) WindowClosed() <-chan bool { return nil }
func (s *testShell) RunTray(app.TrayMenu)    { <-s.a.Done() }
func (*testShell) UpdateTray(string, bool)   {}
func (*testShell) OpenExternal(string) error { return nil }
func (*testShell) QuitTray()                 {}

func main() {
	transaction := flag.Bool("handoff-v1", false, "synthetic transaction")
	root := flag.String("root", "", "synthetic root")
	session := flag.String("session", "", "synthetic session")
	flag.String("build", "", "synthetic build")
	flag.Parse()
	if !*transaction {
		os.Exit(2)
	}
	mode := os.Getenv("MNELAB_RESTART_TEST")
	pipe := restartpipe.New(os.Stdin, os.Stdout)
	switch mode {
	case "exit-before-prepared":
		return
	case "hang-before-prepared":
		time.Sleep(time.Hour)
		return
	case "oversized-prepared":
		var size [4]byte
		binary.BigEndian.PutUint32(size[:], restartpipe.MaxFrame+1)
		os.Stdout.Write(size[:])
		return
	case "wrong-phase":
		pipe.Write("ready", store.SchemaVersion, nil)
		return
	case "wrong-schema":
		pipe.Write("prepared", store.SchemaVersion+1, nil)
		return
	}
	if pipe.Write("prepared", store.SchemaVersion, nil) != nil {
		os.Exit(3)
	}
	if mode == "exit-after-prepared" {
		return
	}
	message, err := pipe.Read("resume")
	defer restartpipe.Wipe(message.Payload)
	if err != nil {
		return
	}
	if mode == "hang-after-resume" {
		time.Sleep(time.Hour)
		return
	}
	if mode == "resume-fails" {
		var h map[string]any
		json.Unmarshal(message.Payload, &h)
		h["pdk"] = "invalid"
		message.Payload, _ = json.Marshal(h)
	}
	var child *app.App
	opt := app.Options{Root: *root, Session: *session, Handoff: bytes.NewReader(message.Payload),
		HandoffConfirm: func() error {
			restartpipe.Wipe(message.Payload)
			if mode == "success" {
				p, err := child.Profile()
				if err != nil {
					return err
				}
				var record map[string]string
				if err := p.St.View(func(tx *store.Tx) error { _, err := tx.Get("synthetic", "continuity", &record); return err }); err != nil || record["value"] != "transfer" {
					return errors.New("synthetic record unavailable")
				}
			}
			if err := pipe.Write("ready", 0, nil); err != nil {
				return err
			}
			if mode == "exit-after-ready" {
				os.Exit(0)
			}
			if _, err := pipe.Read("commit"); err != nil {
				return err
			}
			if mode == "exit-before-committed" {
				os.Exit(0)
			}
			return pipe.Write("committed", 0, nil)
		}}
	a, err := app.New(opt)
	if err != nil {
		os.Exit(4)
	}
	child = a
	defer a.Close()
	a.SetShell(&testShell{a: a, fail: mode == "window-fails"})
	if err := a.Run(fstest.MapFS{"index.html": {Data: []byte("<title>Synthetic restart fixture</title>")}}); err != nil {
		return // EOF tells the parent to restore; never print key-bearing errors
	}
}
