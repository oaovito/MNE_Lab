package export

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentAskHasExactlyOneWinner(t *testing.T) {
	const writers = 32
	dir := t.TempDir()
	ready, release := make(chan struct{}, writers), make(chan struct{})
	type outcome struct {
		saved   Saved
		err     error
		payload string
	}
	results := make(chan outcome, writers)
	for i := range writers {
		go func() {
			payload := fmt.Sprintf("writer %d", i)
			saved, err := Save(dir, "result.csv", CollisionAsk, 1024, func(w io.Writer) error {
				ready <- struct{}{}
				<-release
				_, err := io.WriteString(w, payload)
				return err
			})
			results <- outcome{saved, err, payload}
		}()
	}
	for range writers {
		<-ready
	}
	close(release)
	winners := 0
	for range writers {
		r := <-results
		if r.err != nil {
			if !errors.Is(r.err, ErrExists) {
				t.Errorf("unexpected error: %v", r.err)
			}
			continue
		}
		winners++
		b, err := os.ReadFile(r.saved.Path)
		if err != nil || string(b) != r.payload {
			t.Error("winning export was replaced")
		}
	}
	if winners != 1 {
		t.Fatalf("expected one winner, got %d", winners)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("failed exports left partial files")
	}
}

func TestPublishNoReplacePreservesDestinations(t *testing.T) {
	for label, publish := range map[string]func(string, string) error{"platform": publishNoReplace, "hard-link fallback": linkNoReplace} {
		t.Run(label, func(t *testing.T) {
			for _, kind := range []string{"new", "file", "symlink", "dangling symlink"} {
				t.Run(kind, func(t *testing.T) {
					dir := t.TempDir()
					from, to, target := filepath.Join(dir, "partial"), filepath.Join(dir, "final"), filepath.Join(dir, "target")
					if err := os.WriteFile(from, []byte("new payload"), 0600); err != nil {
						t.Fatal(err)
					}
					if kind == "file" {
						if err := os.WriteFile(to, []byte("existing"), 0600); err != nil {
							t.Fatal(err)
						}
					}
					if kind == "symlink" {
						if err := os.WriteFile(target, []byte("existing"), 0600); err != nil {
							t.Fatal(err)
						}
					}
					if kind == "symlink" || kind == "dangling symlink" {
						if err := os.Symlink(target, to); err != nil {
							t.Skip("symlinks unavailable:", err)
						}
					}
					err := publish(from, to)
					if kind == "new" {
						if err != nil {
							t.Fatal(err)
						}
						b, err := os.ReadFile(to)
						if err != nil || string(b) != "new payload" {
							t.Fatal("final bytes differ")
						}
						if _, err := os.Lstat(from); !errors.Is(err, os.ErrNotExist) {
							t.Fatal("partial name retained")
						}
						return
					}
					if !errors.Is(err, os.ErrExist) {
						t.Fatalf("expected refusal to replace, got %v", err)
					}
					b, err := os.ReadFile(from)
					if err != nil || string(b) != "new payload" {
						t.Fatal("failed claim consumed source")
					}
					if kind == "dangling symlink" {
						if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
							t.Fatal("dangling target created")
						}
					} else {
						b, err := os.ReadFile(to)
						if err != nil || string(b) != "existing" {
							t.Fatal("destination replaced")
						}
					}
					if kind == "symlink" || kind == "dangling symlink" {
						dest, err := os.Readlink(to)
						if err != nil || dest != target {
							t.Fatal("symlink replaced")
						}
					}
				})
			}
		})
	}
}

func TestConcurrentKeepBothPreservesEveryPayload(t *testing.T) {
	const writers = 48
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "result.csv"), []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{}, writers)
	release := make(chan struct{})
	type result struct {
		saved   Saved
		err     error
		payload string
	}
	results := make(chan result, writers)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload := fmt.Sprintf("unique writer %d", i)
			saved, err := Save(dir, "result.csv", CollisionKeepBoth, 1024, func(w io.Writer) error {
				ready <- struct{}{}
				<-release
				_, err := io.WriteString(w, payload)
				return err
			})
			results <- result{saved, err, payload}
		}()
	}
	for range writers {
		<-ready
	}
	close(release)
	wg.Wait()
	close(results)
	seen := map[string]bool{}
	for r := range results {
		if r.err != nil {
			t.Errorf("save failed: %v", r.err)
			continue
		}
		raw, err := os.ReadFile(r.saved.Path)
		if err != nil || string(raw) != r.payload || seen[r.saved.Path] {
			t.Errorf("export was replaced or assigned twice: %s", filepath.Base(r.saved.Path))
		}
		seen[r.saved.Path] = true
	}
	original, err := os.ReadFile(filepath.Join(dir, "result.csv"))
	if err != nil || string(original) != "existing" {
		t.Fatal("original overwritten")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != writers+1 {
		t.Errorf("expected %d retained exports, got %d", writers+1, len(entries))
	}
}
