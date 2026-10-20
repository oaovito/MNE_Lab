//go:build unix && !linux

package pfx

// Other systems do not count mapped heap memory against a data limit; the
// Go memory limit and the deadlines bound the worker.
func limitMemory() {}
