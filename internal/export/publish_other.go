//go:build !linux && !darwin && !windows

package export

func publishNoReplace(from, to string) error { return linkNoReplace(from, to) }
