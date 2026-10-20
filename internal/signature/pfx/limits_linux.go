package pfx

import "syscall"

// workerDataBytes is a hard limit on the worker's writable memory. Linux
// counts the Go heap against RLIMIT_DATA; address space the runtime only
// reserves is not counted.
const workerDataBytes = 512 << 20

func limitMemory() {
	syscall.Setrlimit(syscall.RLIMIT_DATA, &syscall.Rlimit{Cur: workerDataBytes, Max: workerDataBytes})
}
