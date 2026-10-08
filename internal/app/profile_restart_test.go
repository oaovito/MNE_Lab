package app

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/store"
)

func TestProfilePauseResumeRetainsExactUnlockedSession(t *testing.T) {
	a := lifecycleApp(t, paths.Temporary, "")
	p, _ := lifecycleProfile(t, a)
	if _, err := p.UpdateSettings(func(settings *ProfileSettings) { settings.Language = "pt" }); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	st, eng, acct, prov := p.St, p.eng, p.acct, p.prov
	key, accountKey, settings, status, kick := p.key, acct.Key(), p.Settings(), p.Status(), p.kick
	p.changed.Store(true)
	a.exiting.Store(true)
	for range 2 {
		if err := p.pauseForRestart(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.St.Stats(); err == nil {
		t.Fatal("paused store still accepts readers")
	}
	if _, err := p.Sync(context.Background()); err == nil {
		t.Fatal("paused profile still synchronizes")
	}
	if err := p.resumeAfterRestart(); err != nil {
		t.Fatal(err)
	}
	if current, err := a.Profile(); err != nil || current != p {
		t.Fatalf("rollback replaced the profile: %v", err)
	}
	if p.St != st || p.eng != eng || eng.Store != st || p.acct != acct || p.prov != prov || p.kick != kick {
		t.Fatal("rollback replaced an existing session object")
	}
	if p.key != key || acct.Key() != accountKey || !reflect.DeepEqual(p.Settings(), settings) || p.Status() != status || !p.changed.Load() {
		t.Fatal("rollback lost keys, preferences, status or snapshot state")
	}
	p.changed.Store(false)
	if err := p.St.Update(func(tx *store.Tx) error {
		_, err := tx.Put("experiment", "after-rollback", map[string]any{"result": 42})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !p.changed.Load() || len(kick) == 0 {
		t.Fatal("original profile watcher no longer tracks changes")
	}
	a.exiting.Store(false)
	p.startLoop()
	p.loopMu.Lock()
	running := p.loopRun
	p.loopMu.Unlock()
	if !running {
		t.Fatal("rollback did not restart background processing")
	}
}

func TestPendingChildProfileDoesNotReplayQueueBeforeCommit(t *testing.T) {
	a := lifecycleApp(t, paths.Portable, "")
	p, cloud := lifecycleProfile(t, a)
	p.setProvider(cloud)
	if err := p.saveCredentials(cloud); err != nil {
		t.Fatal(err)
	}
	if err := p.St.Update(func(tx *store.Tx) error {
		_, err := tx.Put("experiment", "saved", map[string]any{"result": 42})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	a.exiting.Store(true)
	if err := p.pauseForRestart(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p.dbPath())
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(Options{Root: a.L.Root, Headless: true, KDF: fastKDF, HandoffConfirm: func() error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	b.exiting.Store(true)
	acct, err := b.Vault.UnlockWithKey(p.acct.ID(), p.acct.Key())
	if err != nil {
		t.Fatal(err)
	}
	child, err := b.openProfile(acct, p.Entry, p.key)
	if err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	b.acct, b.prof = acct, child
	b.mu.Unlock()
	stats, err := child.St.Stats()
	if err != nil || stats.Pending+stats.Retryable != 0 {
		t.Fatalf("child mutated the queue before commit: %+v %v", stats, err)
	}
	child.loopMu.Lock()
	running := child.loopRun
	child.loopMu.Unlock()
	if running {
		t.Fatal("child started background work before commit")
	}
	after, err := os.ReadFile(p.dbPath())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("pending child changed original encrypted database: %v", err)
	}
	b.exiting.Store(false)
	child.startLoop()
	stats, err = child.St.Stats()
	if err != nil || stats.Pending == 0 {
		t.Fatalf("committed child did not activate provider replay: %+v %v", stats, err)
	}
}
