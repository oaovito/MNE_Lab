package update

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/instance"
)

func stagedBuild(t *testing.T, builds, version string) string {
	t.Helper()
	dir := filepath.Join(builds, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, ExeName())
	if err := os.WriteFile(exe, []byte("staged build"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestRestartFailureRestoresActivation(t *testing.T) {
	builds := t.TempDir()
	stagedBuild(t, builds, "1.0.0")
	exe := stagedBuild(t, builds, "1.1.0")
	before := State{
		Current: "1.0.0", Previous: "0.9.0", Locked: true,
		LockedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		LastCheck: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Available: "1.1.0",
		Boots: map[string]Boot{"1.0.0": {OK: true}, "1.1.0": {Attempts: 2}},
		Bad:   []string{"1.1.0", "0.8.0"},
	}
	if err := SaveState(builds, before); err != nil {
		t.Fatal(err)
	}
	restartErr := errors.New("child startup failed")
	s := &Service{Builds: builds, Version: "1.0.0", Portable: true,
		Hooks: Hooks{Restart: func(got string) error {
			active := LoadState(builds)
			if got != exe || active.Current != "1.1.0" || active.Previous != "1.0.0" || !active.Locked || active.LockedAt.Equal(before.LockedAt) {
				t.Fatalf("activation did not prepare selected build: %+v, %q", active, got)
			}
			return restartErr
		}}}
	if err := s.apply(context.Background(), "1.1.0"); !errors.Is(err, restartErr) {
		t.Fatalf("restart error lost: %v", err)
	}
	if got := LoadState(builds); !reflect.DeepEqual(got, before) {
		t.Fatalf("failed restart changed prior selection: got %+v, want %+v", got, before)
	}
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("failed restart discarded staged build: %v", err)
	}
	// Recovery from a damaged state file must also retain the rollback;
	// otherwise current.json.prev reactivates the failed update.
	if err := os.WriteFile(filepath.Join(builds, StateFile), []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, version, err := Resolve(builds); err != nil || version != "1.0.0" {
		t.Fatalf("launcher chose failed update: %q, %v", version, err)
	}
}

func TestRestartFailurePreservesLaterStateChanges(t *testing.T) {
	for _, newSelection := range []bool{false, true} {
		name := "metadata and lock"
		if newSelection {
			name = "new selection"
		}
		t.Run(name, func(t *testing.T) {
			builds := t.TempDir()
			stagedBuild(t, builds, "1.1.0")
			before := State{Current: "1.0.0", Previous: "0.9.0", Locked: true,
				LockedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				Boots:    map[string]Boot{"1.0.0": {OK: true}}, Bad: []string{"1.1.0"}}
			if err := SaveState(builds, before); err != nil {
				t.Fatal(err)
			}
			var later State
			restartErr := errors.New("child startup failed")
			s := &Service{Builds: builds, Version: "1.0.0", Portable: true,
				Hooks: Hooks{Restart: func(string) error {
					later = LoadState(builds)
					later.LastCheck = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
					later.Available = "2.0.0"
					later.Locked, later.LockedAt = false, time.Time{}
					later.Boots["1.1.0"] = Boot{Attempts: 1}
					later.Boots["2.0.0"] = Boot{OK: true}
					later.Bad = append(later.Bad, "0.8.0")
					if newSelection {
						later.Current, later.Previous = "2.0.0", "1.1.0"
					}
					if err := SaveState(builds, later); err != nil {
						t.Fatal(err)
					}
					return restartErr
				}}}
			if err := s.apply(context.Background(), "1.1.0"); !errors.Is(err, restartErr) {
				t.Fatal(err)
			}
			if !newSelection {
				later.Current, later.Previous = before.Current, before.Previous
				later.SelectionID = before.SelectionID
				later.Bad = append(later.Bad, "1.1.0")
			}
			if got := LoadState(builds); !reflect.DeepEqual(got, later) {
				t.Fatalf("rollback lost later changes: got %+v, want %+v", got, later)
			}
		})
	}
}

func TestRestartFailureWithoutPriorSelectionAvoidsFailedBuild(t *testing.T) {
	builds := t.TempDir()
	stagedBuild(t, builds, "1.0.0")
	stagedBuild(t, builds, "1.1.0")
	s := &Service{Builds: builds, Version: "1.0.0", Hooks: Hooks{Restart: func(string) error {
		return errors.New("child startup failed")
	}}}
	if err := s.apply(context.Background(), "1.1.0"); err == nil {
		t.Fatal("restart failure not reported")
	}
	if _, version, err := Resolve(builds); err != nil || version != "1.0.0" {
		t.Fatalf("empty previous selection made launcher retry failed build: %q, %v", version, err)
	}
}

func TestRestartFailureReportsRollbackWriteFailure(t *testing.T) {
	builds := t.TempDir()
	stagedBuild(t, builds, "1.1.0")
	if err := SaveState(builds, State{Current: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	restartErr := errors.New("child startup failed")
	s := &Service{Builds: builds, Version: "1.0.0", Hooks: Hooks{Restart: func(string) error {
		statePath := filepath.Join(builds, StateFile) + ".prev"
		if err := os.Remove(statePath); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(statePath, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(statePath, "block-removal"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		return restartErr
	}}}
	err := s.apply(context.Background(), "1.1.0")
	if !errors.Is(err, restartErr) || !strings.Contains(err.Error(), "update.activation_rollback") {
		t.Fatalf("rollback storage failure not reported alongside restart failure: %v", err)
	}
	if !strings.Contains(s.Status().Error, "update.activation_rollback") {
		t.Fatal("automatic apply failure absent from updater status")
	}
}

func TestRestartFailurePreservesLaterActivationOfSameBuild(t *testing.T) {
	builds := t.TempDir()
	stagedBuild(t, builds, "1.1.0")
	if err := SaveState(builds, State{Current: "1.0.0"}); err != nil {
		t.Fatal(err)
	}
	var newer State
	s := &Service{Builds: builds, Version: "1.0.0", Hooks: Hooks{Restart: func(string) error {
		if err := Activate(builds, "1.1.0"); err != nil {
			t.Fatal(err)
		}
		newer = LoadState(builds)
		return errors.New("older child startup failed")
	}}}
	if err := s.apply(context.Background(), "1.1.0"); err == nil {
		t.Fatal("restart failure not reported")
	}
	if got := LoadState(builds); !reflect.DeepEqual(got, newer) {
		t.Fatalf("older failure undid newer activation of same build: got %+v, want %+v", got, newer)
	}
}

func TestConcurrentBootAndLockChangesAreRetained(t *testing.T) {
	builds := t.TempDir()
	s := &Service{Builds: builds, Version: "1.0.0", Portable: true}
	start := make(chan struct{})
	errCh := make(chan error, 33)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		version := strings.Repeat("x", i+1)
		wg.Go(func() {
			<-start
			errCh <- MarkBootOK(builds, version)
		})
	}
	wg.Go(func() {
		<-start
		errCh <- s.SetLocked(context.Background(), true)
	})
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	st := LoadState(builds)
	if !st.Locked || st.LockedAt.IsZero() || len(st.Boots) != 32 {
		t.Fatalf("concurrent state changes lost: %+v", st)
	}
}

func TestBuildStateSubprocessWriter(t *testing.T) {
	builds := os.Getenv("MNELAB_UPDATE_TEST_BUILDS")
	if builds == "" {
		t.Skip("subprocess helper")
	}
	fmt.Println("attempt")
	if err := MarkBootOK(builds, "1.1.0"); err != nil {
		t.Fatal(err)
	}
	fmt.Println("done")
}

func TestBuildStateMutationWaitsForOtherProcessAndReloads(t *testing.T) {
	builds := t.TempDir()
	before := State{Current: "1.0.0"}
	if err := SaveState(builds, before); err != nil {
		t.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Join(builds, ".state-lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, exe, "-test.run=^TestBuildStateSubprocessWriter$")
	child.Env = append(os.Environ(), "MNELAB_UPDATE_TEST_BUILDS="+builds)
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if child.ProcessState == nil {
			child.Process.Kill()
			child.Wait()
		}
	}()
	reader := bufio.NewReader(stdout)
	if line, err := reader.ReadString('\n'); err != nil || line != "attempt\n" {
		t.Fatalf("child did not attempt mutation: %q, %v", line, err)
	}
	done := make(chan string, 1)
	go func() {
		line, _ := reader.ReadString('\n')
		done <- line
	}()
	select {
	case line := <-done:
		t.Fatalf("child ignored another process's state lock: %q", line)
	case <-time.After(150 * time.Millisecond):
	}
	before.LastCheck = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	before.Available = "2.0.0"
	// This process owns the disk lock, simulating another updater writing
	// after the child requested the lock but before it can read the state.
	if err := atomicfile.WriteJSON(filepath.Join(builds, StateFile), before); err != nil {
		t.Fatal(err)
	}
	lock.Release()
	select {
	case line := <-done:
		if line != "done\n" {
			t.Fatalf("child mutation failed after releasing lock: %q", line)
		}
	case <-ctx.Done():
		t.Fatal("child did not finish after state lock release")
	}
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
	st := LoadState(builds)
	if st.Current != before.Current || !st.LastCheck.Equal(before.LastCheck) || st.Available != before.Available || !st.Boots["1.1.0"].OK {
		t.Fatalf("child used stale state after acquiring lock: %+v", st)
	}
	if installed := Installed(builds); len(installed) != 0 {
		t.Fatalf("state lock became a fake installed build: %v", installed)
	}
}

func TestBuildStateContentionReturnsErrorAndKeepsReadableState(t *testing.T) {
	builds := t.TempDir()
	before := State{Current: "1.0.0", Previous: "0.9.0", Locked: true}
	if err := SaveState(builds, before); err != nil {
		t.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Join(builds, ".state-lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	if got := LoadState(builds); !reflect.DeepEqual(got, before) {
		t.Fatalf("lock contention changed a state read: %+v", got)
	}
	start := time.Now()
	if err := MarkBootOK(builds, "1.1.0"); !errors.Is(err, instance.ErrRunning) {
		t.Fatalf("contended mutation did not fail with lock error: %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("state lock contention was not bounded")
	}
	if got := LoadState(builds); !reflect.DeepEqual(got, before) {
		t.Fatalf("failed mutation overwrote state during contention: %+v", got)
	}
}
