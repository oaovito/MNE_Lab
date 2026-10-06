package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/oaovito/mne_lab/internal/provider"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
)

// flaky wraps a provider and fails on demand to simulate lost connectivity.
type flaky struct {
	provider.Provider
	mu      sync.Mutex
	offline bool
	writes  int
	failAt  int // fail the n-th write (1-based), 0 = never
}

func (f *flaky) Write(ctx context.Context, name string, b []byte) (provider.Object, error) {
	f.mu.Lock()
	f.writes++
	n := f.writes
	off := f.offline || (f.failAt > 0 && n == f.failAt)
	f.mu.Unlock()
	if off {
		return provider.Object{}, provider.ErrOffline
	}
	return f.Provider.Write(ctx, name, b)
}

func (f *flaky) List(ctx context.Context, dir string) ([]provider.Object, error) {
	f.mu.Lock()
	off := f.offline
	f.mu.Unlock()
	if off {
		return nil, provider.ErrOffline
	}
	return f.Provider.List(ctx, dir)
}

type machine struct {
	st  *store.Store
	eng *Engine
}

func newMachine(t *testing.T, dev string, pdk secure.Key, p provider.Provider) machine {
	st, err := store.Open(filepath.Join(t.TempDir(), "p.db"), pdk, store.Options{DeviceID: dev, Queue: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return machine{st, New(st, pdk, p, "acc", "prof", nil)}
}

func put(t *testing.T, m machine, id string, v any) {
	if err := m.st.Update(func(tx *store.Tx) error { _, err := tx.Put("graphs", id, v); return err }); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, m machine, id string) map[string]any {
	var v map[string]any
	m.st.View(func(tx *store.Tx) error { _, err := tx.Get("graphs", id, &v); return err })
	return v
}

func TestTwoMachinesConverge(t *testing.T) {
	cloud := provider.NewFolder(provider.Google, t.TempDir())
	pdk := secure.NewKey()
	a := newMachine(t, "A", pdk, cloud)
	b := newMachine(t, "B", pdk, cloud)
	ctx := context.Background()
	put(t, a, "g1", map[string]any{"title": "DLS A1"})
	a.st.Update(func(tx *store.Tx) error { _, err := tx.PutBlob([]byte("original file bytes")); return err })
	if _, err := a.eng.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := b.eng.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if get(t, b, "g1")["title"] != "DLS A1" {
		t.Fatal("B did not receive the record")
	}
	var blobs []string
	b.st.View(func(tx *store.Tx) error { blobs = tx.BlobIDs(); return nil })
	if len(blobs) != 1 {
		t.Fatal("B did not receive the blob")
	}
	// B edits, A pulls.
	put(t, b, "g1", map[string]any{"title": "DLS A1 (week 2)"})
	b.eng.Run(ctx)
	a.eng.Run(ctx)
	if get(t, a, "g1")["title"] != "DLS A1 (week 2)" {
		t.Fatal("A did not receive B's edit")
	}
	st, _ := a.st.Stats()
	if st.Pending != 0 || st.Conflicts != 0 {
		t.Fatalf("A should be fully confirmed: %+v", st)
	}
	// Running again changes nothing (idempotent).
	rep, _ := a.eng.Run(ctx)
	if rep.Pushed+rep.Pulled+rep.Conflicts != 0 {
		t.Fatalf("second run not idempotent: %+v", rep)
	}
}

func TestConcurrentEditsBecomeConflictNotOverwrite(t *testing.T) {
	cloud := provider.NewFolder(provider.OneDrive, t.TempDir())
	pdk := secure.NewKey()
	a := newMachine(t, "A", pdk, cloud)
	b := newMachine(t, "B", pdk, cloud)
	ctx := context.Background()
	put(t, a, "g1", map[string]any{"title": "base"})
	a.eng.Run(ctx)
	b.eng.Run(ctx)
	// Both edit the same record without seeing each other.
	put(t, a, "g1", map[string]any{"title": "edited on A"})
	put(t, b, "g1", map[string]any{"title": "edited on B"})
	if _, err := a.eng.Run(ctx); err != nil {
		t.Fatal(err)
	}
	rep, err := b.eng.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Conflicts != 1 {
		t.Fatalf("divergence not detected: %+v", rep)
	}
	if get(t, b, "g1")["title"] != "edited on B" {
		t.Fatal("local edit must be preserved")
	}
	cs, _ := Conflicts(b.st)
	if len(cs) != 1 || len(cs[0].Remote) != 1 {
		t.Fatalf("conflict not recorded: %+v", cs)
	}
	var remote map[string]any
	json.Unmarshal(cs[0].Remote[0].Data, &remote)
	if remote["title"] != "edited on A" {
		t.Fatal("remote version must be preserved")
	}
	// Keep both: B's stays, A's becomes a copy.
	err = b.eng.Resolve(ctx, "graphs", "g1", KeepBoth, func(d []byte) (string, []byte) { return "g1-copy", d })
	if err != nil {
		t.Fatal(err)
	}
	if get(t, b, "g1-copy")["title"] != "edited on A" || get(t, b, "g1")["title"] != "edited on B" {
		t.Fatal("keep both lost data")
	}
	b.eng.Run(ctx)
	a.eng.Run(ctx)
	if get(t, a, "g1")["title"] != "edited on B" || get(t, a, "g1-copy")["title"] != "edited on A" {
		t.Fatalf("A did not converge after resolution: %v %v", get(t, a, "g1"), get(t, a, "g1-copy"))
	}
	if cs, _ := Conflicts(a.st); len(cs) != 0 {
		t.Fatal("A should have no conflict after resolution")
	}
}

func TestInterruptedUploadResumesWithoutDuplicates(t *testing.T) {
	inner := provider.NewFolder(provider.Google, t.TempDir())
	f := &flaky{Provider: inner, failAt: 2}
	pdk := secure.NewKey()
	a := newMachine(t, "A", pdk, f)
	for _, id := range []string{"g1", "g2", "g3"} {
		put(t, a, id, map[string]any{"title": id})
	}
	ctx := context.Background()
	if _, err := a.eng.Run(ctx); !errors.Is(err, provider.ErrOffline) {
		t.Fatalf("expected offline failure, got %v", err)
	}
	st, _ := a.st.Stats()
	if st.Pending+st.Retryable == 0 {
		t.Fatal("unsent work must stay queued")
	}
	f.failAt = 0
	if _, err := a.eng.Run(ctx); err != nil {
		t.Fatal(err)
	}
	objs, _ := inner.List(ctx, a.eng.Prefix+"/r")
	if len(objs) != 3 {
		t.Fatalf("expected exactly 3 remote versions, got %d", len(objs))
	}
	st, _ = a.st.Stats()
	if st.Pending != 0 || st.Retryable != 0 {
		t.Fatalf("queue not drained: %+v", st)
	}
}

func TestOfflineKeepsWorkLocal(t *testing.T) {
	f := &flaky{Provider: provider.NewFolder(provider.ICloud, t.TempDir()), offline: true}
	a := newMachine(t, "A", secure.NewKey(), f)
	put(t, a, "g1", map[string]any{"v": 1})
	if _, err := a.eng.Run(context.Background()); !errors.Is(err, provider.ErrOffline) {
		t.Fatalf("got %v", err)
	}
	if get(t, a, "g1")["v"] != float64(1) {
		t.Fatal("local data lost while offline")
	}
	f.offline = false
	if _, err := a.eng.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestTamperedRemoteRejected(t *testing.T) {
	cloud := provider.NewFolder(provider.Google, t.TempDir())
	pdk := secure.NewKey()
	a := newMachine(t, "A", pdk, cloud)
	put(t, a, "g1", map[string]any{"v": 1})
	a.eng.Run(context.Background())
	ctx := context.Background()
	objs, _ := cloud.List(ctx, a.eng.Prefix+"/r")
	b, _ := cloud.Read(ctx, objs[0].Name)
	b[len(b)-1] ^= 1
	cloud.Write(ctx, objs[0].Name, b)
	c := newMachine(t, "C", pdk, cloud)
	if _, err := c.eng.Run(ctx); err == nil {
		t.Fatal("tampered remote data must be rejected")
	}
	if get(t, c, "g1") != nil {
		t.Fatal("tampered data must not be applied")
	}
	// A different account's key cannot read the data at all.
	d := newMachine(t, "D", secure.NewKey(), cloud)
	if _, err := d.eng.Run(ctx); err == nil {
		t.Fatal("foreign key must not decrypt")
	}
}

func TestDeletionPropagates(t *testing.T) {
	cloud := provider.NewFolder(provider.Google, t.TempDir())
	pdk := secure.NewKey()
	a := newMachine(t, "A", pdk, cloud)
	b := newMachine(t, "B", pdk, cloud)
	ctx := context.Background()
	put(t, a, "g1", map[string]any{"v": 1})
	a.eng.Run(ctx)
	b.eng.Run(ctx)
	a.st.Update(func(tx *store.Tx) error { return tx.Delete("graphs", "g1") })
	a.eng.Run(ctx)
	b.eng.Run(ctx)
	if get(t, b, "g1") != nil {
		t.Fatal("deletion did not propagate")
	}
}
