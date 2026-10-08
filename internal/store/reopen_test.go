package store

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/oaovito/mne_lab/internal/secure"
)

func TestSuspendReopenRetainsRecordsQueueWatchersAndDevice(t *testing.T) {
	s, path, key := open(t, true)
	var changes atomic.Int32
	s.Watch(func(Change) { changes.Add(1) })
	if err := s.Update(func(tx *Tx) error {
		_, err := tx.Put("experiment", "original", doc{Name: "original", V: 42})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	before, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Suspend(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stats(); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("suspended read: %v", err)
	}
	if s.Path() != path {
		t.Fatal("suspend lost the original path")
	}
	// Another key must remain unable to unlock the released file.
	if foreign, err := OpenExisting(path, secure.NewKey(), Options{}); err == nil {
		foreign.Close()
		t.Fatal("suspend weakened profile key isolation")
	}
	child, err := OpenExisting(path, key, Options{DeviceID: "child", Queue: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Update(func(tx *Tx) error {
		_, err := tx.Put("experiment", "child", doc{Name: "child", V: 7})
		return err
	}); err != nil {
		child.Close()
		t.Fatal(err)
	}
	if err := child.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Reopen(); err != nil {
		t.Fatal(err)
	}
	if err := s.View(func(tx *Tx) error {
		var got doc
		if _, err := tx.Get("experiment", "original", &got); err != nil {
			return err
		}
		if got.Name != "original" || got.V != 42 {
			t.Fatal("original record changed across rollback")
		}
		_, err := tx.Get("experiment", "child", &got)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(tx *Tx) error {
		record, err := tx.Put("experiment", "after", doc{Name: "after", V: 8})
		if err == nil && record.Device != "dev1" {
			t.Fatal("rollback changed the original device identity")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if changes.Load() != 2 {
		t.Fatalf("original watcher not retained: %d changes", changes.Load())
	}
	after, err := s.Stats()
	if err != nil || after.Pending != before.Pending+2 {
		t.Fatalf("queue continuity lost: before=%+v after=%+v error=%v", before, after, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Reopen(); !errors.Is(err, bolt.ErrDatabaseNotOpen) {
		t.Fatalf("permanently closed store reopened: %v", err)
	}
}

func TestReopenNeverCreatesMissingOrInitializesEmptyStore(t *testing.T) {
	for _, empty := range []bool{false, true} {
		name := "missing"
		if empty {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			s, path, _ := open(t, true)
			if err := s.Suspend(); err != nil {
				t.Fatal(err)
			}
			kept := filepath.Join(t.TempDir(), "original.db")
			if err := os.Rename(path, kept); err != nil {
				t.Fatal(err)
			}
			if empty {
				if err := os.WriteFile(path, nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Reopen(); err == nil {
				t.Fatal("invalid store reopened")
			}
			if info, err := os.Stat(path); empty {
				if err != nil || info.Size() != 0 {
					t.Fatalf("empty store initialized: %v", err)
				}
				os.Remove(path)
			} else if !os.IsNotExist(err) {
				t.Fatalf("missing store created: %v", err)
			}
			if err := os.Rename(kept, path); err != nil {
				t.Fatal(err)
			}
			if err := s.Reopen(); err != nil {
				t.Fatalf("reopen not retryable after restoring original: %v", err)
			}
		})
	}
}

func TestReopenRejectsChangedSchemaWithoutMigrating(t *testing.T) {
	for _, schema := range []int{SchemaVersion - 1, SchemaVersion + 1} {
		name := "older"
		if schema > SchemaVersion {
			name = "newer"
		}
		t.Run(name, func(t *testing.T) {
			s, path, _ := open(t, true)
			if err := s.Suspend(); err != nil {
				t.Fatal(err)
			}
			changed, err := bolt.Open(path, 0o600, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := changed.Update(func(tx *bolt.Tx) error { return putSchema(tx.Bucket(bMeta), schema) }); err != nil {
				changed.Close()
				t.Fatal(err)
			}
			changed.Close()
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.Reopen(); err == nil {
				t.Fatal("changed schema silently accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
				t.Fatalf("failed strict reopen modified the file: %v", err)
			}
		})
	}
}

func TestSuspendWaitsForActiveReadTransaction(t *testing.T) {
	s, _, _ := open(t, false)
	inside, release, readDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		readDone <- s.View(func(*Tx) error {
			close(inside)
			<-release
			return nil
		})
	}()
	<-inside
	suspended := make(chan error, 1)
	go func() { suspended <- s.Suspend() }()
	select {
	case err := <-suspended:
		close(release)
		t.Fatalf("suspend finished with an active reader: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	if err := <-suspended; err != nil {
		t.Fatal(err)
	}
	if err := s.Reopen(); err != nil {
		t.Fatal(err)
	}
}

func TestSuspendReopenConcurrentReaders(t *testing.T) {
	s, _, _ := open(t, true)
	var wg sync.WaitGroup
	failures := make(chan error, 64)
	for range 3 {
		wg.Go(func() {
			for range 30 {
				for _, read := range []func() error{
					func() error { _, err := s.Stats(); return err },
					func() error { _, err := s.Schema(); return err },
					func() error { _, err := s.Audit(5); return err },
					func() error { return s.Check() },
					func() error { var b bytes.Buffer; _, err := s.Snapshot(&b); return err },
				} {
					if err := read(); err != nil && !errors.Is(err, bolt.ErrDatabaseNotOpen) {
						failures <- err
						return
					}
				}
			}
		})
	}
	for range 12 {
		if err := s.Suspend(); err != nil {
			t.Fatal(err)
		}
		if err := s.Reopen(); err != nil {
			t.Fatal(err)
		}
		if err := s.Update(func(tx *Tx) error {
			_, err := tx.Put("experiment", "current", doc{Name: "current", V: 42})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
}
