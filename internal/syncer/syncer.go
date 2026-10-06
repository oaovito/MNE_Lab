// Package syncer replicates a profile store to the user's cloud provider.
//
// Remote layout (inside the provider's app-specific area):
//
//	accounts/<aid>/profiles/<pid>/r/<rk>~<hash>~<parents>.rec   record versions
//	accounts/<aid>/profiles/<pid>/b/<blob>.blob                 immutable blobs
//
// Every record version is an immutable, content-addressed file that names
// the version(s) it was derived from. The remote history is therefore a
// graph that needs no conditional writes from the provider: uploads are
// idempotent (same content, same name), an interrupted upload can simply be
// repeated, and two machines editing the same record produce two heads,
// which is detected as a conflict instead of one silently overwriting the
// other. Last-write-wins is never used for record data.
package syncer

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/logging"
	"github.com/oaovito/mne_lab/internal/provider"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
)

const short = 32

// ConflictCollection holds local bookkeeping of unresolved conflicts.
const ConflictCollection = "_conflict"

// Engine synchronizes one profile with one provider.
type Engine struct {
	Store  *store.Store
	Key    secure.Key
	Prov   provider.Provider
	Prefix string // accounts/<aid>/profiles/<pid>
	Log    *slog.Logger

	names secure.Key
	data  secure.Key
}

// New prepares an engine.
func New(st *store.Store, pdk secure.Key, prov provider.Provider, accountID, profileID string, log *slog.Logger) *Engine {
	if log == nil {
		log = logging.Discard()
	}
	return &Engine{Store: st, Key: pdk, Prov: prov, Prefix: "accounts/" + accountID + "/profiles/" + profileID, Log: log,
		names: secure.SubKey(pdk, "remote-names"), data: secure.SubKey(pdk, "remote-data")}
}

// Report summarizes a run.
type Report struct {
	At        time.Time `json:"at"`
	Pushed    int       `json:"pushed"`
	Pulled    int       `json:"pulled"`
	Confirmed int       `json:"confirmed"`
	BlobsUp   int       `json:"blobsUp"`
	BlobsDown int       `json:"blobsDown"`
	Conflicts int       `json:"conflicts"`
	Pending   int       `json:"pending"`
}

type version struct {
	hash    string
	parents []string
	name    string
}

func (e *Engine) rk(key string) string {
	m := hmac.New(sha256.New, e.names[:])
	m.Write([]byte(key))
	return hex.EncodeToString(m.Sum(nil))[:short]
}

func sh(h string) string {
	if len(h) > short {
		return h[:short]
	}
	return h
}

func (e *Engine) recName(rk, hash string, parents []string) string {
	ps := "0"
	if len(parents) > 0 {
		sorted := append([]string(nil), parents...)
		sort.Strings(sorted)
		ps = strings.Join(sorted, ".")
	}
	return fmt.Sprintf("%s/r/%s~%s~%s.rec", e.Prefix, rk, sh(hash), ps)
}

func parseName(name string) (rk string, v version, ok bool) {
	base := name[strings.LastIndex(name, "/")+1:]
	if !strings.HasSuffix(base, ".rec") {
		return "", v, false
	}
	parts := strings.Split(strings.TrimSuffix(base, ".rec"), "~")
	if len(parts) != 3 {
		return "", v, false
	}
	v = version{hash: parts[1], name: name}
	if parts[2] != "0" {
		v.parents = strings.Split(parts[2], ".")
	}
	return parts[0], v, true
}

// heads returns the version hashes that no other version descends from.
func heads(vs []version) []string {
	isParent := map[string]bool{}
	seen := map[string]bool{}
	for _, v := range vs {
		for _, p := range v.parents {
			isParent[p] = true
		}
	}
	var out []string
	for _, v := range vs {
		if !isParent[v.hash] && !seen[v.hash] {
			out = append(out, v.hash)
			seen[v.hash] = true
		}
	}
	sort.Strings(out)
	return out
}

type remoteRecord struct {
	Collection string          `json:"c"`
	ID         string          `json:"id"`
	Data       json.RawMessage `json:"d,omitempty"`
	Deleted    bool            `json:"x,omitempty"`
	Hash       string          `json:"h"`
	Updated    time.Time       `json:"u"`
	Device     string          `json:"dev"`
	Tx         string          `json:"tx"`
}

func (e *Engine) seal(r store.Record, rk string) ([]byte, error) {
	b, err := json.Marshal(remoteRecord{r.Collection, r.ID, r.Data, r.Deleted, r.Hash, r.Updated, r.Device, r.Tx})
	if err != nil {
		return nil, err
	}
	return secure.Seal(e.data, b, []byte("rec:"+rk)), nil
}

func (e *Engine) fetch(ctx context.Context, rk string, v version) (store.Record, error) {
	b, err := e.Prov.Read(ctx, v.name)
	if err != nil {
		return store.Record{}, err
	}
	pt, err := secure.Open(e.data, b, []byte("rec:"+rk))
	if err != nil {
		return store.Record{}, errors.New("syncer: remote record failed authentication")
	}
	var rr remoteRecord
	if err := json.Unmarshal(pt, &rr); err != nil {
		return store.Record{}, err
	}
	if sh(rr.Hash) != v.hash || e.rk(rr.Collection+"/"+rr.ID) != rk ||
		store.ContentHash(rr.Collection, rr.ID, rr.Data, rr.Deleted) != rr.Hash {
		return store.Record{}, errors.New("syncer: remote record failed integrity verification")
	}
	return store.Record{Collection: rr.Collection, ID: rr.ID, Data: rr.Data, Deleted: rr.Deleted, Hash: rr.Hash,
		Updated: rr.Updated, Device: rr.Device, Tx: rr.Tx}, nil
}

// push uploads one version and verifies it by reading it back.
func (e *Engine) push(ctx context.Context, r store.Record, rk string, parents []string) error {
	body, err := e.seal(r, rk)
	if err != nil {
		return err
	}
	name := e.recName(rk, r.Hash, parents)
	if _, err := e.Prov.Write(ctx, name, body); err != nil {
		return err
	}
	back, err := e.Prov.Read(ctx, name)
	if err != nil {
		return err
	}
	if string(back) != string(body) {
		return errors.New("syncer: remote copy differs after upload")
	}
	return nil
}

// Run performs one synchronization pass. It is safe to interrupt at any
// point and to run again.
func (e *Engine) Run(ctx context.Context) (Report, error) {
	rep := Report{At: time.Now().UTC()}
	objs, err := e.Prov.List(ctx, e.Prefix+"/r")
	if err != nil {
		return rep, err
	}
	remote := map[string][]version{}
	for _, o := range objs {
		if rk, v, ok := parseName(o.Name); ok {
			remote[rk] = append(remote[rk], v)
		}
	}
	var locals []store.Record
	if err := e.Store.View(func(t *store.Tx) error {
		var err error
		locals, err = t.ListAll("")
		return err
	}); err != nil {
		return rep, err
	}
	seen := map[string]bool{}
	var firstErr error
	for _, r := range locals {
		if !store.Synced(r.Collection) {
			continue
		}
		rk := e.rk(r.Key())
		seen[rk] = true
		if err := e.reconcile(ctx, r, rk, remote[rk], &rep); err != nil {
			e.Log.Warn("sync record failed", "rk", rk[:8], "err", err.Error())
			if firstErr == nil {
				firstErr = err
			}
			e.markOps(r.Key(), err)
			if provider.IsNetworkError(err) || errors.Is(err, provider.ErrUnauthorized) || errors.Is(err, provider.ErrQuota) {
				return rep, err
			}
		}
	}
	for rk, vs := range remote {
		if seen[rk] {
			continue
		}
		hs := heads(vs)
		if err := e.pullNew(ctx, rk, vs, hs, &rep); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			if provider.IsNetworkError(err) {
				return rep, err
			}
		}
	}
	if err := e.syncBlobs(ctx, &rep); err != nil {
		return rep, err
	}
	e.cleanQueue()
	st, _ := e.Store.Stats()
	rep.Pending = st.Pending + st.Retryable
	return rep, firstErr
}

func versionByHash(vs []version, h string) (version, bool) {
	for _, v := range vs {
		if v.hash == h {
			return v, true
		}
	}
	return version{}, false
}

func (e *Engine) reconcile(ctx context.Context, r store.Record, rk string, vs []version, rep *Report) error {
	lh, bh := sh(r.Hash), sh(r.Base)
	if r.Sync == store.StateConflict {
		rep.Conflicts++
		return nil
	}
	hs := heads(vs)
	switch {
	case len(vs) == 0:
		var parents []string
		if bh != "" && bh != lh {
			parents = []string{bh}
		}
		if err := e.push(ctx, r, rk, parents); err != nil {
			return err
		}
		rep.Pushed++
		return e.confirm(r)
	case len(hs) == 1 && hs[0] == lh:
		if r.Sync != store.StateConfirmed || r.Base != r.Hash {
			rep.Confirmed++
			return e.confirm(r)
		}
		return nil
	case len(hs) == 1 && hs[0] == bh && lh != bh:
		if err := e.push(ctx, r, rk, []string{bh}); err != nil {
			return err
		}
		rep.Pushed++
		return e.confirm(r)
	case len(hs) == 1 && lh == bh:
		v, _ := versionByHash(vs, hs[0])
		rr, err := e.fetch(ctx, rk, v)
		if err != nil {
			return err
		}
		rep.Pulled++
		return e.Store.Update(func(t *store.Tx) error { return t.ApplyRemote(rr) })
	}
	// Diverged: local changed and the remote moved, or several remote heads.
	var others []store.Record
	for _, h := range hs {
		if h == lh {
			continue
		}
		v, _ := versionByHash(vs, h)
		rr, err := e.fetch(ctx, rk, v)
		if err != nil {
			return err
		}
		if rr.Hash == r.Hash {
			continue
		}
		others = append(others, rr)
	}
	if len(others) == 0 {
		// Every head carries our exact content: write a merge version.
		if err := e.push(ctx, r, rk, hs); err != nil {
			return err
		}
		rep.Pushed++
		return e.confirm(r)
	}
	rep.Conflicts++
	return e.recordConflict(r, others, hs)
}

func (e *Engine) pullNew(ctx context.Context, rk string, vs []version, hs []string, rep *Report) error {
	if len(hs) == 0 {
		return nil
	}
	first, _ := versionByHash(vs, hs[0])
	rr, err := e.fetch(ctx, rk, first)
	if err != nil {
		return err
	}
	if err := e.Store.Update(func(t *store.Tx) error { return t.ApplyRemote(rr) }); err != nil {
		return err
	}
	rep.Pulled++
	if len(hs) == 1 {
		return nil
	}
	var others []store.Record
	for _, h := range hs[1:] {
		v, _ := versionByHash(vs, h)
		o, err := e.fetch(ctx, rk, v)
		if err != nil {
			return err
		}
		others = append(others, o)
	}
	rep.Conflicts++
	var cur store.Record
	e.Store.View(func(t *store.Tx) error { cur, _ = t.GetRecord(rr.Collection, rr.ID); return nil })
	return e.recordConflict(cur, others, hs)
}

func (e *Engine) confirm(r store.Record) error {
	return e.Store.Update(func(t *store.Tx) error {
		cur, err := t.GetRecord(r.Collection, r.ID)
		if err != nil {
			return err
		}
		if cur.Hash != r.Hash {
			// Changed locally during the upload; the new version stays pending.
			return t.SetSyncState(r.Collection, r.ID, store.StatePending, r.Hash)
		}
		return t.SetSyncState(r.Collection, r.ID, store.StateConfirmed, r.Hash)
	})
}

// Conflict describes an unresolved divergence for the interface.
type Conflict struct {
	Collection string          `json:"collection"`
	ID         string          `json:"id"`
	Local      json.RawMessage `json:"local,omitempty"`
	LocalDel   bool            `json:"localDeleted"`
	LocalAt    time.Time       `json:"localUpdated"`
	Remote     []RemoteVersion `json:"remote"`
	Heads      []string        `json:"heads"`
}

// RemoteVersion is one competing version.
type RemoteVersion struct {
	Hash    string          `json:"hash"`
	Data    json.RawMessage `json:"data,omitempty"`
	Deleted bool            `json:"deleted"`
	Updated time.Time       `json:"updated"`
	Device  string          `json:"device"`
}

func conflictID(coll, id string) string { return coll + "." + id }

func (e *Engine) recordConflict(local store.Record, others []store.Record, hs []string) error {
	c := Conflict{Collection: local.Collection, ID: local.ID, Local: local.Data, LocalDel: local.Deleted, LocalAt: local.Updated, Heads: hs}
	for _, o := range others {
		c.Remote = append(c.Remote, RemoteVersion{Hash: o.Hash, Data: o.Data, Deleted: o.Deleted, Updated: o.Updated, Device: o.Device})
	}
	return e.Store.Update(func(t *store.Tx) error {
		if _, err := t.Put(ConflictCollection, conflictID(local.Collection, local.ID), c); err != nil {
			return err
		}
		return t.SetSyncState(local.Collection, local.ID, store.StateConflict, "")
	})
}

// Conflicts lists unresolved conflicts.
func Conflicts(st *store.Store) ([]Conflict, error) {
	var out []Conflict
	err := st.View(func(t *store.Tx) error {
		rs, err := t.List(ConflictCollection)
		for _, r := range rs {
			var c Conflict
			if json.Unmarshal(r.Data, &c) == nil {
				out = append(out, c)
			}
		}
		return err
	})
	return out, err
}

// Resolution choices.
const (
	KeepMine   = "mine"
	KeepTheirs = "theirs"
	KeepBoth   = "both"
)

// Resolve settles a conflict. Nothing is discarded silently: "both" keeps
// the remote version as a separate record. duplicate builds that record's
// data (for example to mark the copy's title); it may be nil.
func (e *Engine) Resolve(ctx context.Context, coll, id, choice string, duplicate func(data []byte) (newID string, newData []byte)) error {
	var c Conflict
	err := e.Store.View(func(t *store.Tx) error {
		_, err := t.Get(ConflictCollection, conflictID(coll, id), &c)
		return err
	})
	if err != nil {
		return err
	}
	if len(c.Remote) == 0 {
		return errors.New("syncer: nothing to resolve")
	}
	theirs := c.Remote[0]
	for _, r := range c.Remote[1:] {
		if r.Updated.After(theirs.Updated) {
			theirs = r
		}
	}
	rk := e.rk(coll + "/" + id)
	err = e.Store.Update(func(t *store.Tx) error {
		switch choice {
		case KeepMine:
		case KeepTheirs:
			if theirs.Deleted {
				if err := t.Delete(coll, id); err != nil && !errors.Is(err, store.ErrNotFound) {
					return err
				}
			} else {
				var v json.RawMessage = theirs.Data
				if _, err := t.Put(coll, id, v); err != nil {
					return err
				}
			}
		case KeepBoth:
			if !theirs.Deleted && duplicate != nil {
				nid, nd := duplicate(theirs.Data)
				if _, err := t.Put(coll, nid, json.RawMessage(nd)); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("syncer: unknown choice %q", choice)
		}
		if err := t.Delete(ConflictCollection, conflictID(coll, id)); err != nil {
			return err
		}
		cur, err := t.GetRecord(coll, id)
		if err != nil {
			return err
		}
		cur.Sync = store.StatePending
		return t.SetSyncState(coll, id, store.StatePending, "")
	})
	if err != nil {
		return err
	}
	// Publish the resolution as a merge version descending from every head.
	var cur store.Record
	e.Store.View(func(t *store.Tx) error { cur, err = t.GetRecord(coll, id); return err })
	if err != nil {
		return err
	}
	if err := e.push(ctx, cur, rk, c.Heads); err != nil {
		return err // stays pending; the next run retries
	}
	return e.confirm(cur)
}

// ---- blobs ----

func (e *Engine) blobName(id string) string { return e.Prefix + "/b/" + id + ".blob" }

func (e *Engine) syncBlobs(ctx context.Context, rep *Report) error {
	objs, err := e.Prov.List(ctx, e.Prefix+"/b")
	if err != nil {
		return err
	}
	remote := map[string]int64{}
	for _, o := range objs {
		base := o.Name[strings.LastIndex(o.Name, "/")+1:]
		if strings.HasSuffix(base, ".blob") {
			remote[strings.TrimSuffix(base, ".blob")] = o.Size
		}
	}
	var local []string
	gone := map[string]bool{}
	e.Store.View(func(t *store.Tx) error {
		local = t.BlobIDs()
		recs, _ := t.List(store.CollBlobGone)
		for _, r := range recs {
			gone[r.ID] = true
		}
		return nil
	})
	have := map[string]bool{}
	for _, id := range local {
		have[id] = true
		if gone[id] {
			continue
		}
		if _, ok := remote[id]; ok {
			continue
		}
		var data []byte
		if err := e.Store.View(func(t *store.Tx) error { var err error; data, err = t.GetBlob(id); return err }); err != nil {
			return err
		}
		body := secure.Seal(e.data, data, []byte("blob:"+id))
		obj, err := e.Prov.Write(ctx, e.blobName(id), body)
		if err != nil {
			e.markOps(id, err)
			return err
		}
		if obj.Size != 0 && obj.Size != int64(len(body)) {
			return errors.New("syncer: blob size mismatch after upload")
		}
		rep.BlobsUp++
	}
	for id := range remote {
		if gone[id] {
			// Permanently deleted on some machine: remove the cloud copy too.
			if err := e.Prov.Delete(ctx, e.blobName(id)); err != nil && !errors.Is(err, provider.ErrNotFound) {
				return err
			}
			e.Store.Update(func(t *store.Tx) error {
				if t.HasBlob(id) {
					return t.DeleteBlob(id)
				}
				return nil
			})
			continue
		}
		if have[id] {
			continue
		}
		b, err := e.Prov.Read(ctx, e.blobName(id))
		if err != nil {
			return err
		}
		pt, err := secure.Open(e.data, b, []byte("blob:"+id))
		if err != nil {
			return errors.New("syncer: remote blob failed authentication")
		}
		if err := e.Store.Update(func(t *store.Tx) error { return t.PutBlobRaw(id, pt) }); err != nil {
			return err
		}
		rep.BlobsDown++
	}
	return nil
}

// ---- queue bookkeeping ----

func (e *Engine) markOps(key string, cause error) {
	e.Store.Update(func(t *store.Tx) error {
		ops, err := t.Ops()
		if err != nil {
			return err
		}
		for _, op := range ops {
			if op.Key == key {
				op.Attempts++
				op.State = store.StateRetryable
				op.LastError = cause.Error()
				t.UpdateOp(op)
			}
		}
		return nil
	})
}

func (e *Engine) cleanQueue() {
	e.Store.Update(func(t *store.Tx) error {
		ops, err := t.Ops()
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, op := range ops {
			switch op.Kind {
			case "record":
				i := strings.Index(op.Key, "/")
				if i < 0 {
					t.RemoveOp(op.Seq)
					continue
				}
				r, err := t.GetRecord(op.Key[:i], op.Key[i+1:])
				if err != nil || (r.Sync == store.StateConfirmed && r.Rev >= op.Rev) {
					t.RemoveOp(op.Seq)
				} else if r.Sync == store.StateConflict && op.State != store.StateConflict {
					op.State = store.StateConflict
					t.UpdateOp(op)
				}
			case "blob":
				// Blob uploads are verified in syncBlobs; reaching here means done.
				op.LastConfirmed = now
				t.RemoveOp(op.Seq)
			}
		}
		return nil
	})
}

func hashHex(b []byte) string { return secure.HashHex(b) }
