//go:build unix

package pfx

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// workerCPUSeconds is a processor-time limit the worker sets on itself
// before it reads anything. The kernel signals the worker at the limit,
// which then ends; Linux also kills it outright there.
const workerCPUSeconds = 60

func limitProcess() {
	// The Go runtime ignores SIGXCPU unless asked for it.
	xcpu := make(chan os.Signal, 1)
	signal.Notify(xcpu, syscall.SIGXCPU)
	go func() {
		<-xcpu
		terminate()
	}()
	syscall.Setrlimit(syscall.RLIMIT_CPU, &syscall.Rlimit{Cur: workerCPUSeconds, Max: workerCPUSeconds})
	limitMemory()
}

// prepare starts the worker in its own process group, so terminal signals
// meant for the application do not reach it. No parent-death signal is
// used: Linux sends it when the starting thread ends, not the application.
// A worker whose application ended reads end of file on its pipe or misses
// a deadline.
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// contain has nothing to add on these systems: the worker's own limits and
// deadlines apply.
func contain(cmd *exec.Cmd) (release func(), err error) { return func() {}, nil }
