package store

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/secure"
)

type doc struct {
	Name string  `json:"name"`
	V    float64 `json:"v"`
}

func open(t *testing.T, queue bool) (*Store, string, secure.Key) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "profile.db")
	k := secure.NewKey()
	s, err := Open(p, k, Options{DeviceID: "dev1", Queue: queue})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, p, k
}

func TestPutGetVersioning(t *testing.T) {
	s, _, _ := open(t, true)
	var r1, r2, r3 Record
	err := s.Update(func(tx *Tx) error {
		var err error
		r1, err = tx.Put("graphs", "g1", doc{"a", 1.25})
		if err != nil {
			return err
		}
		r2, _ = tx.Put("graphs", "g1", doc{"a", 1.25}) // unchanged
		r3, _ = tx.Put("graphs", "g1", doc{"b", 2})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Rev != 1 || r2.Hash != r1.Hash {
		t.Fatal("identical content must not create a version")
	}
	if r3.Rev != 2 || r3.Parent != r1.Hash || r3.Sync != StatePending {
		t.Fatalf("bad version chain: %+v", r3)
	}
	var d doc
	s.View(func(tx *Tx) error { _, err := tx.Get("graphs", "g1", &d); return err })
	if d.Name != "b" || d.V != 2 {
		t.Fatalf("got %+v", d)
	}
	// Two puts to the same key coalesce into one pending operation.
	var ops []Op
	s.View(func(tx *Tx) error { ops, _ = tx.Ops(StatePending); return nil })
	if len(ops) != 1 || ops[0].Rev != 2 {
		t.Fatalf("queue not coalesced: %+v", ops)
	}
}

func TestEncryptedAtRest(t *testing.T) {
	s, p, _ := open(t, false)
	secret := "Sample-A1-unique-marker-7781"
	s.Update(func(tx *Tx) error {
		_, err := tx.Put("files", "f1", doc{Name: secret})
		if err != nil {
			return err
		}
		_, err = tx.PutBlob([]byte("RAW " + secret))
		return err
	})
	s.Close()
	raw, _ := os.ReadFile(p)
	if bytes.Contains(raw, []byte(secret)) {
		t.Fatal("plaintext found in database file")
	}
}

func TestWrongKeyRejected(t *testing.T) {
	s, p, _ := open(t, false)
	s.Close()
	if _, err := Open(p, secure.NewKey(), Options{}); err == nil || !strings.Contains(err.Error(), "key") {
		t.Fatalf("expected key error, got %v", err)
	}
}

func TestBlobsDedupAndVerify(t *testing.T) {
	s, _, _ := open(t, true)
	var a, b string
	s.Update(func(tx *Tx) error {
		a, _ = tx.PutBlob([]byte("same"))
		b, _ = tx.PutBlob([]byte("same"))
		return nil
	})
	if a != b {
		t.Fatal("identical blobs must share an id")
	}
	var got []byte
	s.View(func(tx *Tx) error { got, _ = tx.GetBlob(a); return nil })
	if string(got) != "same" {
		t.Fatal("blob round trip failed")
	}
	err := s.Update(func(tx *Tx) error { return tx.PutBlobRaw(a, []byte("different")) })
	if err == nil {
		t.Fatal("mismatched remote blob must be rejected")
	}
}

func TestTombstoneAndRemote(t *testing.T) {
	s, _, _ := open(t, true)
	s.Update(func(tx *Tx) error { _, err := tx.Put("cycles", "c1", doc{Name: "x"}); return err })
	s.Update(func(tx *Tx) error { return tx.Delete("cycles", "c1") })
	err := s.View(func(tx *Tx) error { _, err := tx.Get("cycles", "c1", nil); return err })
	if err != ErrNotFound {
		t.Fatalf("deleted record still visible: %v", err)
	}
	remote := Record{Collection: "cycles", ID: "c2", Data: []byte(`{"name":"r"}`)}
	remote.Hash = ContentHash(remote.Collection, remote.ID, remote.Data, false)
	if err := s.Update(func(tx *Tx) error { return tx.ApplyRemote(remote) }); err != nil {
		t.Fatal(err)
	}
	bad := remote
	bad.ID = "c3"
	if err := s.Update(func(tx *Tx) error { return tx.ApplyRemote(bad) }); err == nil {
		t.Fatal("remote record with wrong hash must be rejected")
	}
	if err := s.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotReopens(t *testing.T) {
	s, _, k := open(t, false)
	s.Update(func(tx *Tx) error { _, err := tx.Put("files", "f", doc{Name: "n"}); return err })
	var buf bytes.Buffer
	if _, err := s.Snapshot(&buf); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "copy.db")
	os.WriteFile(p, buf.Bytes(), 0o600)
	c, err := Open(p, k, Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Check(); err != nil {
		t.Fatal(err)
	}
}
