package pfx

import "syscall"

// workerDataBytes is a hard limit on the worker's writable mappings. Linux
// counts the Go heap's mapped arenas against RLIMIT_DATA, not only the pages
// in use: a release-built worker maps about 150 MiB. Address space the
// runtime only reserves is not counted.
const workerDataBytes = 512 << 20

func limitMemory() {
	// The race detector maps several times the heap as shadow memory;
	// test builds with it keep the soft limit only.
	if raceDetector {
		return
	}
	syscall.Setrlimit(syscall.RLIMIT_DATA, &syscall.Rlimit{Cur: workerDataBytes, Max: workerDataBytes})
}
