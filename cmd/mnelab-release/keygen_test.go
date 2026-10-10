package main

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestKeygenPreservesExistingAndDanglingSymlinkTargets(t *testing.T) {
	t.Run("existing file", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "test.key")
		original := []byte("existing-test-file")
		if e := os.WriteFile(file, original, 0600); e != nil {
			t.Fatal(e)
		}
		if e := keygen([]string{"-out", file}); e == nil {
			t.Fatal("existing file replaced")
		}
		b, e := os.ReadFile(file)
		if e != nil || string(b) != string(original) {
			t.Fatal("existing bytes changed")
		}
	})
	t.Run("dangling symlink", func(t *testing.T) {
		root := t.TempDir()
		file, target := filepath.Join(root, "test.key"), filepath.Join(root, "absent.key")
		if e := os.Symlink(target, file); e != nil {
			t.Skip("symlink unavailable on this system")
		}
		if e := keygen([]string{"-out", file}); e == nil {
			t.Fatal("dangling symlink followed")
		}
		if _, e := os.Stat(target); !errors.Is(e, os.ErrNotExist) {
			t.Fatal("symlink target created")
		}
		if _, e := os.Readlink(file); e != nil {
			t.Fatal("symlink modified")
		}
	})
}
func TestKeygenConcurrentCreationHasExactlyOneWinner(t *testing.T) {
	file := filepath.Join(t.TempDir(), "test.key")
	const n = 32
	start := make(chan struct{})
	results := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; results <- keygen([]string{"-out", file}) }()
	}
	close(start)
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("expected one exclusive creation, got %d", success)
	}
	b, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	seed, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
	if e != nil || len(seed) != 32 {
		t.Fatal("invalid/incomplete generated seed")
	}
	info, e := os.Stat(file)
	if e != nil {
		t.Fatal(e)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("private key mode", info.Mode())
	}
}
