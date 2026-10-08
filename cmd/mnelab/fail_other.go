//go:build !windows

package main

import (
	"fmt"
	"os"
)

func fail(err error) { fmt.Fprintln(os.Stderr, "MNE Lab could not start:", err) }
