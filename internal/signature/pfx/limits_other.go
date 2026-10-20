//go:build !unix && !windows

package pfx

import "os/exec"

func limitProcess() {}

func prepare(cmd *exec.Cmd) {}

func contain(cmd *exec.Cmd) (release func(), err error) { return func() {}, nil }
