// Package store is the encrypted, transactional data store of one profile.
//
// Storage engine: bbolt (copy-on-write B+tree with fsync on commit), so every
// transaction is atomic and crash consistent. Every value is sealed with the
// profile data key (XChaCha20-Poly1305) and bound to its key as associated
// data; the database file reveals only random identifiers and sizes.
//
// Each change also enters a persistent synchronization queue and a local
// audit trail inside the same transaction, so a crash can never leave a
// change that is saved but not queued.
package store

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/oaovito/mne_lab/internal/secure"
)

// SchemaVersion is the current on-disk schema of a profile store.
const SchemaVersion = 1

var (
	bMeta  = []byte("meta")
	bRec   = []byte("rec")
	bBlob  = []byte("blob")
	bQueue = []byte("queue")
	bAudit = []byte("audit")
)

// ErrNotFound is returned when a record or blob does not exist.
var ErrNotFound = errors.New("store: not found")

// ErrNewerSchema is returned when the store was written by a newer build.
var ErrNewerSchema = errors.New("store: data was written by a newer MNE Lab build")

// SyncState is the synchronization state of a record or queued operation.
type SyncState string

const (
	StateLocal     SyncState = "local"     // exists only here; no remote destination
	StatePending   SyncState = "pending"   // waiting to be sent
	StateSyncing   SyncState = "syncing"   // being sent
	StateConfirmed SyncState = "confirmed" // remote copy verified
	StateConflict  SyncState = "conflict"  // diverged; needs resolution
	StateRetryable SyncState = "retryable" // failed; will retry
)

// Record is one versioned document.
type Record struct {
	Collection string          `json:"c"`
	ID         string          `json:"id"`
	Data       json.RawMessage `json:"d,omitempty"`
	Deleted    bool            `json:"x,omitempty"`
	Rev        int64           `json:"rev"`
	Hash       string          `json:"h"`
	Parent     string          `json:"p,omitempty"`
	Base       string          `json:"b,omitempty"`
	Updated    time.Time       `json:"u"`
	Device     string          `json:"dev"`
	Tx         string          `json:"tx"`
	Sync       SyncState       `json:"s"`
}

// Synced reports whether a collection is replicated to the cloud.
// Collections whose name starts with "_" hold machine-local state such as
// provider credentials and conflict bookkeeping; they never leave the device.
func Synced(collection string) bool { return !strings.HasPrefix(collection, "_") }

// Key returns the record's storage key.
func (r Record) Key() string { return r.Collection + "/" + r.ID }

// ContentHash returns the hash of the record's logical content: identity,
// data and deletion flag. Metadata such as timestamps is excluded so the
// same content always hashes the same on every machine.
func ContentHash(collection, id string, data []byte, deleted bool) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%t\x00", collection, id, deleted)
	h.Write(compactJSON(data))
	return hex.EncodeToString(h.Sum(nil))
}

func compactJSON(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		return b
	}
	return buf.Bytes()
}

// Op is a persistent synchronization queue entry.
type Op struct {
	Seq           uint64    `json:"seq"`
	ID            string    `json:"id"`
	Kind          string    `json:"kind"` // "record" or "blob"
	Key           string    `json:"key"`
	Rev           int64     `json:"rev"`
	Tx            string    `json:"tx"`
	Checksum      string    `json:"sum"`
	Origin        string    `json:"origin"`
	State         SyncState `json:"state"`
	Attempts      int       `json:"attempts"`
	LastError     string    `json:"err,omitempty"`
	LastConfirmed time.Time `json:"confirmed,omitzero"`
	Created       time.Time `json:"created"`
}

// AuditEntry is a local, private trace of an operation. It never contains
// record contents.
type AuditEntry struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"`
	Key    string    `json:"key"`
	Tx     string    `json:"tx"`
}

// Change is emitted after a committed transaction.
type Change struct {
	Keys []string
}

// Store is an open profile store.
type Store struct {
	db       *bolt.DB
	key      secure.Key
	blobKey  secure.Key
	device   string
	mu       sync.Mutex
	watchers []func(Change)
	queueOn  bool
}

// Options configure Open.
type Options struct {
	DeviceID string
	// Queue enables the synchronization queue (false for USB Drive Only).
	Queue bool
	// ReadOnly opens without write access (inspection, recovery checks).
	ReadOnly bool
}

// Open opens or creates a store at path sealed with key.
func Open(path string, key secure.Key, opt Options) (*Store, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 3 * time.Second, ReadOnly: opt.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	s := &Store{db: db, key: key, blobKey: secure.SubKey(key, "blob-names"), device: opt.DeviceID, queueOn: opt.Queue}
	if opt.ReadOnly {
		return s, s.verifyKey()
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bMeta, bRec, bBlob, bQueue, bAudit} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		meta := tx.Bucket(bMeta)
		if meta.Get([]byte("check")) == nil {
			// A sealed constant lets Open detect a wrong key immediately.
			if err := meta.Put([]byte("check"), secure.Seal(key, []byte("mnelab-store"), []byte("check"))); err != nil {
				return err
			}
			return putSchema(meta, SchemaVersion)
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	if err := s.verifyKey(); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func putSchema(meta *bolt.Bucket, v int) error {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return meta.Put([]byte("schema"), b)
}

// Schema returns the on-disk schema version.
func (s *Store) Schema() (int, error) {
	v := 0
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bMeta).Get([]byte("schema"))
		if len(b) == 8 {
			v = int(binary.BigEndian.Uint64(b))
		}
		return nil
	})
	return v, err
}

func (s *Store) verifyKey() error {
	return s.db.View(func(tx *bolt.Tx) error {
		m := tx.Bucket(bMeta)
		if m == nil {
			return errors.New("store: not an MNE Lab profile store")
		}
		if _, err := secure.Open(s.key, m.Get([]byte("check")), []byte("check")); err != nil {
			return errors.New("store: key does not match this profile")
		}
		return nil
	})
}

// migrations maps a schema version to the function that upgrades it to the
// next version. Every migration runs inside one transaction.
var migrations = map[int]func(tx *bolt.Tx, s *Store) error{}

func (s *Store) migrate() error {
	v, err := s.Schema()
	if err != nil {
		return err
	}
	if v > SchemaVersion {
		return ErrNewerSchema
	}
	for v < SchemaVersion {
		fn := migrations[v]
		if fn == nil {
			return fmt.Errorf("store: no migration from schema %d", v)
		}
		next := v + 1
		if err := s.db.Update(func(tx *bolt.Tx) error {
			if err := fn(tx, s); err != nil {
				return err
			}
			return putSchema(tx.Bucket(bMeta), next)
		}); err != nil {
			return fmt.Errorf("store: migration %d→%d: %w", v, next, err)
		}
		v = next
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// Path returns the database file path.
func (s *Store) Path() string { return s.db.Path() }

// Watch registers a callback for committed changes.
func (s *Store) Watch(fn func(Change)) {
	s.mu.Lock()
	s.watchers = append(s.watchers, fn)
	s.mu.Unlock()
}

func (s *Store) emit(keys []string) {
	if len(keys) == 0 {
		return
	}
	s.mu.Lock()
	ws := append([]func(Change){}, s.watchers...)
	s.mu.Unlock()
	for _, w := range ws {
		w(Change{Keys: keys})
	}
}

// Tx is a read-write transaction.
type Tx struct {
	s       *Store
	tx      *bolt.Tx
	id      string
	changed []string
}

// Update runs fn in one atomic transaction.
func (s *Store) Update(fn func(*Tx) error) error {
	t := &Tx{s: s, id: secure.NewID()}
	err := s.db.Update(func(tx *bolt.Tx) error {
		t.tx = tx
		return fn(t)
	})
	if err == nil {
		s.emit(t.changed)
	}
	return err
}

// View runs fn in a read-only transaction.
func (s *Store) View(fn func(*Tx) error) error {
	return s.db.View(func(tx *bolt.Tx) error { return fn(&Tx{s: s, tx: tx}) })
}

func (t *Tx) seal(b []byte, key string) []byte { return secure.Seal(t.s.key, b, []byte(key)) }

func (t *Tx) open(b []byte, key string) ([]byte, error) {
	pt, err := secure.Open(t.s.key, b, []byte(key))
	if err != nil {
		return nil, fmt.Errorf("store: %s failed integrity verification", key)
	}
	return pt, nil
}

// GetRecord loads a record including tombstones.
func (t *Tx) GetRecord(collection, id string) (Record, error) {
	k := collection + "/" + id
	raw := t.tx.Bucket(bRec).Get([]byte(k))
	if raw == nil {
		return Record{}, ErrNotFound
	}
	pt, err := t.open(raw, "rec:"+k)
	if err != nil {
		return Record{}, err
	}
	var r Record
	if err := json.Unmarshal(pt, &r); err != nil {
		return Record{}, err
	}
	return r, nil
}

// Get decodes a live record into v.
func (t *Tx) Get(collection, id string, v any) (Record, error) {
	r, err := t.GetRecord(collection, id)
	if err != nil {
		return r, err
	}
	if r.Deleted {
		return r, ErrNotFound
	}
	if v != nil {
		return r, json.Unmarshal(r.Data, v)
	}
	return r, nil
}

func (t *Tx) writeRecord(r Record) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	k := r.Key()
	if err := t.tx.Bucket(bRec).Put([]byte(k), t.seal(b, "rec:"+k)); err != nil {
		return err
	}
	t.changed = append(t.changed, k)
	return nil
}

// Put stores v as the new version of collection/id. Unchanged content is a
// no-op so repeated saves never create spurious versions or sync traffic.
func (t *Tx) Put(collection, id string, v any) (Record, error) {
	if strings.ContainsAny(collection, "/~") || id == "" || strings.ContainsAny(id, "/~") {
		return Record{}, fmt.Errorf("store: invalid record key %q/%q", collection, id)
	}
	data, err := json.Marshal(v)
	if err != nil {
		return Record{}, err
	}
	return t.put(collection, id, data, false, "put")
}

// Delete writes a tombstone. Tombstones synchronize like any other version,
// so a deletion on one machine is never resurrected by another.
func (t *Tx) Delete(collection, id string) error {
	if _, err := t.GetRecord(collection, id); err != nil {
		return err
	}
	_, err := t.put(collection, id, nil, true, "delete")
	return err
}

func (t *Tx) put(collection, id string, data []byte, deleted bool, action string) (Record, error) {
	old, err := t.GetRecord(collection, id)
	exists := err == nil
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Record{}, err
	}
	h := ContentHash(collection, id, data, deleted)
	if exists && old.Hash == h {
		return old, nil
	}
	r := Record{
		Collection: collection, ID: id, Data: compactJSON(data), Deleted: deleted,
		Hash: h, Updated: time.Now().UTC(), Device: t.s.device, Tx: t.id,
	}
	if exists {
		r.Rev = old.Rev + 1
		r.Parent = old.Hash
		r.Base = old.Base
		if old.Sync == StateConflict {
			r.Sync = StateConflict
		}
	} else {
		r.Rev = 1
	}
	if r.Sync == "" {
		if t.s.queueOn && Synced(collection) {
			r.Sync = StatePending
		} else {
			r.Sync = StateLocal
		}
	}
	if err := t.writeRecord(r); err != nil {
		return Record{}, err
	}
	if t.s.queueOn && Synced(collection) {
		if err := t.enqueue("record", r.Key(), r.Rev, r.Hash); err != nil {
			return Record{}, err
		}
	}
	return r, t.audit(action, r.Key())
}

// ApplyRemote stores a version received from the cloud as confirmed.
func (t *Tx) ApplyRemote(r Record) error {
	if ContentHash(r.Collection, r.ID, r.Data, r.Deleted) != r.Hash {
		return errors.New("store: remote record failed checksum verification")
	}
	r.Base = r.Hash
	r.Sync = StateConfirmed
	if old, err := t.GetRecord(r.Collection, r.ID); err == nil {
		r.Rev = old.Rev + 1
	}
	if err := t.writeRecord(r); err != nil {
		return err
	}
	return t.audit("pull", r.Key())
}

// SetSyncState updates the sync metadata of a record without creating a new
// version.
func (t *Tx) SetSyncState(collection, id string, state SyncState, base string) error {
	r, err := t.GetRecord(collection, id)
	if err != nil {
		return err
	}
	r.Sync = state
	if base != "" {
		r.Base = base
	}
	return t.writeRecord(r)
}

// List returns live records of a collection sorted by ID.
func (t *Tx) List(collection string) ([]Record, error) {
	return t.list(collection, false)
}

// ListAll returns records of a collection including tombstones.
func (t *Tx) ListAll(collection string) ([]Record, error) {
	return t.list(collection, true)
}

func (t *Tx) list(collection string, tombstones bool) ([]Record, error) {
	var out []Record
	prefix := []byte(collection + "/")
	if collection == "" {
		prefix = nil
	}
	c := t.tx.Bucket(bRec).Cursor()
	for k, v := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, v = c.Next() {
		pt, err := t.open(v, "rec:"+string(k))
		if err != nil {
			return nil, err
		}
		var r Record
		if err := json.Unmarshal(pt, &r); err != nil {
			return nil, err
		}
		if r.Deleted && !tombstones {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// Count returns the number of live records in a collection.
func (t *Tx) Count(collection string) (int, error) {
	rs, err := t.List(collection)
	return len(rs), err
}

// BlobID returns the identifier of content without storing it. Identifiers
// are keyed hashes so they do not reveal file checksums to the cloud.
func (s *Store) BlobID(data []byte) string {
	m := hmac.New(sha256.New, s.blobKey[:])
	m.Write(data)
	return hex.EncodeToString(m.Sum(nil))[:40]
}

// PutBlob stores immutable content and returns its identifier. Identical
// content is stored once.
func (t *Tx) PutBlob(data []byte) (string, error) {
	id := t.s.BlobID(data)
	b := t.tx.Bucket(bBlob)
	if b.Get([]byte(id)) != nil {
		return id, nil
	}
	if err := b.Put([]byte(id), t.seal(data, "blob:"+id)); err != nil {
		return "", err
	}
	if t.s.queueOn {
		if err := t.enqueue("blob", id, 1, id); err != nil {
			return "", err
		}
	}
	return id, t.audit("blob", id)
}

// PutBlobRaw stores content received from the cloud, verifying its id.
func (t *Tx) PutBlobRaw(id string, data []byte) error {
	if t.s.BlobID(data) != id {
		return errors.New("store: blob failed checksum verification")
	}
	return t.tx.Bucket(bBlob).Put([]byte(id), t.seal(data, "blob:"+id))
}

// GetBlob loads content by identifier and verifies it.
func (t *Tx) GetBlob(id string) ([]byte, error) {
	raw := t.tx.Bucket(bBlob).Get([]byte(id))
	if raw == nil {
		return nil, ErrNotFound
	}
	pt, err := t.open(raw, "blob:"+id)
	if err != nil {
		return nil, err
	}
	if t.s.BlobID(pt) != id {
		return nil, fmt.Errorf("store: blob %s failed checksum verification", id[:8])
	}
	return pt, nil
}

// DeleteBlob removes content permanently (with its queued upload). The
// caller records the deletion in CollBlobGone so other machines and the
// cloud copy follow.
func (t *Tx) DeleteBlob(id string) error {
	if err := t.tx.Bucket(bBlob).Delete([]byte(id)); err != nil {
		return err
	}
	ops, err := t.Ops()
	if err != nil {
		return err
	}
	for _, op := range ops {
		if op.Kind == "blob" && op.Key == id {
			if err := t.RemoveOp(op.Seq); err != nil {
				return err
			}
		}
	}
	return t.audit("blob-delete", id)
}

// CollBlobGone lists permanently deleted blobs (synchronized).
const CollBlobGone = "blob.gone"

// HasBlob reports whether a blob exists.
func (t *Tx) HasBlob(id string) bool { return t.tx.Bucket(bBlob).Get([]byte(id)) != nil }

// BlobIDs lists all blob identifiers.
func (t *Tx) BlobIDs() []string {
	var out []string
	t.tx.Bucket(bBlob).ForEach(func(k, _ []byte) error {
		out = append(out, string(k))
		return nil
	})
	return out
}

// ---- synchronization queue ----

func (t *Tx) enqueue(kind, key string, rev int64, sum string) error {
	q := t.tx.Bucket(bQueue)
	// Coalesce: a newer pending change to the same key supersedes older
	// pending entries, which keeps the queue small and idempotent.
	c := q.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		op, err := t.decodeOp(k, v)
		if err != nil {
			return err
		}
		if op.Key == key && op.Kind == kind && (op.State == StatePending || op.State == StateRetryable) {
			if err := q.Delete(k); err != nil {
				return err
			}
		}
	}
	seq, _ := q.NextSequence()
	op := Op{Seq: seq, ID: secure.NewID(), Kind: kind, Key: key, Rev: rev, Tx: t.id, Checksum: sum,
		Origin: t.s.device, State: StatePending, Created: time.Now().UTC()}
	return t.putOp(op)
}

func seqKey(seq uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, seq)
	return b
}

func (t *Tx) putOp(op Op) error {
	b, _ := json.Marshal(op)
	k := seqKey(op.Seq)
	return t.tx.Bucket(bQueue).Put(k, t.seal(b, "op:"+hex.EncodeToString(k)))
}

func (t *Tx) decodeOp(k, v []byte) (Op, error) {
	pt, err := t.open(v, "op:"+hex.EncodeToString(k))
	if err != nil {
		return Op{}, err
	}
	var op Op
	return op, json.Unmarshal(pt, &op)
}

// Ops returns queue entries in order, optionally filtered by state.
func (t *Tx) Ops(states ...SyncState) ([]Op, error) {
	var out []Op
	err := t.tx.Bucket(bQueue).ForEach(func(k, v []byte) error {
		op, err := t.decodeOp(k, v)
		if err != nil {
			return err
		}
		if len(states) == 0 {
			out = append(out, op)
			return nil
		}
		for _, s := range states {
			if op.State == s {
				out = append(out, op)
				break
			}
		}
		return nil
	})
	return out, err
}

// UpdateOp rewrites a queue entry.
func (t *Tx) UpdateOp(op Op) error { return t.putOp(op) }

// RemoveOp deletes a queue entry after confirmation.
func (t *Tx) RemoveOp(seq uint64) error { return t.tx.Bucket(bQueue).Delete(seqKey(seq)) }

// EnqueueAll queues every record and blob (used when a profile gains a
// cloud destination or switches provider).
func (t *Tx) EnqueueAll() error {
	recs, err := t.ListAll("")
	if err != nil {
		return err
	}
	for _, r := range recs {
		if !Synced(r.Collection) {
			continue
		}
		r.Sync = StatePending
		r.Base = ""
		if err := t.writeRecord(r); err != nil {
			return err
		}
		if err := t.enqueue("record", r.Key(), r.Rev, r.Hash); err != nil {
			return err
		}
	}
	for _, id := range t.BlobIDs() {
		if err := t.enqueue("blob", id, 1, id); err != nil {
			return err
		}
	}
	return nil
}

// ---- audit ----

func (t *Tx) audit(action, key string) error {
	a := t.tx.Bucket(bAudit)
	seq, _ := a.NextSequence()
	b, _ := json.Marshal(AuditEntry{At: time.Now().UTC(), Action: action, Key: key, Tx: t.id})
	k := seqKey(seq)
	if err := a.Put(k, t.seal(b, "audit:"+hex.EncodeToString(k))); err != nil {
		return err
	}
	// Retention: keep the most recent 5000 entries.
	if seq > 5000 {
		if err := a.Delete(seqKey(seq - 5000)); err != nil {
			return err
		}
	}
	return nil
}

// Audit returns the most recent audit entries, newest first.
func (s *Store) Audit(limit int) ([]AuditEntry, error) {
	var out []AuditEntry
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(bAudit).Cursor()
		for k, v := c.Last(); k != nil && len(out) < limit; k, v = c.Prev() {
			pt, err := secure.Open(s.key, v, []byte("audit:"+hex.EncodeToString(k)))
			if err != nil {
				return err
			}
			var e AuditEntry
			if json.Unmarshal(pt, &e) == nil {
				out = append(out, e)
			}
		}
		return nil
	})
	return out, err
}

// ---- integrity and snapshots ----

// Check verifies the database structure and that every value authenticates
// under the profile key.
func (s *Store) Check() error {
	return s.db.View(func(tx *bolt.Tx) error {
		for err := range tx.Check() {
			return fmt.Errorf("store: structural check failed: %w", err)
		}
		t := &Tx{s: s, tx: tx}
		if _, err := t.ListAll(""); err != nil {
			return err
		}
		for _, id := range t.BlobIDs() {
			if _, err := t.GetBlob(id); err != nil {
				return err
			}
		}
		_, err := t.Ops()
		return err
	})
}

// Snapshot writes a consistent copy of the whole database to w.
func (s *Store) Snapshot(w io.Writer) (int64, error) {
	var n int64
	err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		n, err = tx.WriteTo(w)
		return err
	})
	return n, err
}

// Stats summarizes the store for status displays.
type Stats struct {
	Records   int            `json:"records"`
	Blobs     int            `json:"blobs"`
	Pending   int            `json:"pending"`
	Conflicts int            `json:"conflicts"`
	Retryable int            `json:"retryable"`
	ByColl    map[string]int `json:"byCollection"`
}

// Stats computes a summary.
func (s *Store) Stats() (Stats, error) {
	st := Stats{ByColl: map[string]int{}}
	err := s.View(func(t *Tx) error {
		recs, err := t.List("")
		if err != nil {
			return err
		}
		st.Records = len(recs)
		for _, r := range recs {
			st.ByColl[r.Collection]++
			if r.Sync == StateConflict {
				st.Conflicts++
			}
		}
		st.Blobs = len(t.BlobIDs())
		ops, err := t.Ops()
		if err != nil {
			return err
		}
		for _, op := range ops {
			switch op.State {
			case StatePending, StateSyncing:
				st.Pending++
			case StateRetryable:
				st.Retryable++
			case StateConflict:
				st.Conflicts++
			}
		}
		return nil
	})
	return st, err
}

// SortByUpdated orders records newest first.
func SortByUpdated(rs []Record) {
	sort.SliceStable(rs, func(i, j int) bool { return rs[i].Updated.After(rs[j].Updated) })
}
