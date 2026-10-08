package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/instance"
	"github.com/oaovito/mne_lab/internal/restartpipe"
	"github.com/oaovito/mne_lab/internal/store"
)

func TestCLIProcessHelper(t *testing.T) {
	root := os.Getenv("MNELAB_CLI_PROCESS_TEST_ROOT")
	if root == "" {
		t.Skip("only used as a synthetic child process")
	}
	flag.CommandLine = flag.NewFlagSet("mnelab", flag.ExitOnError)
	os.Args = []string{os.Args[0], "--handoff-v1", "--root", root}
	main()
	os.Exit(0)
}

func cliChild(t *testing.T, root string) (*exec.Cmd, *restartpipe.Pipe, func()) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestCLIProcessHelper$")
	cmd.Env = append(os.Environ(), "MNELAB_CLI_PROCESS_TEST_ROOT="+root)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	timer := time.AfterFunc(5*time.Second, func() { in.Close(); out.Close(); cmd.Process.Kill() })
	cleanup := func() { timer.Stop(); in.Close(); out.Close() }
	t.Cleanup(func() { cleanup(); cmd.Process.Kill() })
	return cmd, restartpipe.New(out, in), cleanup
}

func TestTransactionalChildPreparesBeforeSharedStateAndRejectsBadSession(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created-yet")
	cmd, pipe, cleanup := cliChild(t, root)
	prepared, err := pipe.Read("prepared")
	if err != nil || prepared.Schema != store.SchemaVersion {
		t.Fatalf("prepare failed: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("child modified shared layout before parent released it")
	}
	if err := pipe.Write("resume", store.SchemaVersion, []byte(`{"account":"missing","ak":"synthetic-secret-must-not-be-logged"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := pipe.Read("ready"); err == nil {
		t.Fatal("child acknowledged an invalid session")
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("invalid child returned success")
	}
	cleanup()
	if instance.Held(filepath.Join(root, "data")) {
		t.Fatal("failed child retained the instance lock")
	}
	logPath := filepath.Join(root, "data", "logs", "mnelab.log")
	if b, err := os.ReadFile(logPath); err == nil && strings.Contains(string(b), "synthetic-secret-must-not-be-logged") {
		t.Fatal("child logged handoff material")
	}
}

func TestTransactionalChildRefusesAnExistingInstance(t *testing.T) {
	root := t.TempDir()
	lock, err := instance.Acquire(filepath.Join(root, "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	cmd, pipe, cleanup := cliChild(t, root)
	if _, err := pipe.Read("prepared"); err != nil {
		t.Fatal(err)
	}
	if err := pipe.Write("resume", store.SchemaVersion, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := pipe.Read("ready"); err == nil {
		t.Fatal("child accepted an instance-lock conflict")
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("handoff lock conflict was treated as successful focus")
	}
	cleanup()
	if !instance.Held(filepath.Join(root, "data")) {
		t.Fatal("original instance lock was lost")
	}
}
