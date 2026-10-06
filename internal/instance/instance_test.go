package instance

import (
	"os"
	"os/exec"
	"testing"
)

func TestSingleInstance(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(dir)
	if err != nil {
		t.Fatal(err)
	}
	l.Publish(Info{Port: 1234, Token: "t"})
	// A second process cannot take the lock while the first holds it.
	if os.Getenv("MNELAB_CHILD") == "" {
		cmd := exec.Command(os.Args[0], "-test.run", "TestChildLock")
		cmd.Env = append(os.Environ(), "MNELAB_CHILD=1", "MNELAB_DIR="+dir)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child: %v %s", err, out)
		}
	}
	if i, err := Running(dir); err != nil || i.Port != 1234 {
		t.Fatal("published info")
	}
	l.Release()
	l2, err := Acquire(dir)
	if err != nil {
		t.Fatal("lock not released")
	}
	l2.Release()
}

func TestChildLock(t *testing.T) {
	dir := os.Getenv("MNELAB_DIR")
	if dir == "" {
		t.Skip()
	}
	if _, err := Acquire(dir); err != ErrRunning {
		t.Fatalf("second instance acquired the lock: %v", err)
	}
	if !Held(dir) {
		t.Fatal("Held")
	}
}
