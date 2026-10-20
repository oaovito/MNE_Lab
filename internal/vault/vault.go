// Package vault manages Accounts, Profiles and their keys.
//
// Key hierarchy (no MNE Lab server ever sees any of it):
//
//	Account Key (AK, random)          seals the account index and avatars
//	  wrapped by Argon2id(passphrase)  → offline unlock anywhere
//	  sealed to the Recovery Key (HPKE) → passphrase reset
//	  optionally wrapped by a drive key → "keep this drive signed in"
//	Profile Data Key (PDK, random)    seals all profile data
//	  wrapped by Argon2id(profile password) when the profile has one,
//	  otherwise wrapped by AK
//	  always sealed to the Recovery Key  → profile password reset
//
// A profile with a password cannot be opened with the Account Key: the
// isolation lives in the key material, not in the interface.
package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/secure"
)

// Limits and schema.
const (
	MaxProfiles   = 5
	SchemaVersion = 1
	MinPassphrase = 8
)

// Storage modes.
const (
	StorageUSBCloud  = "usb_cloud"
	StorageUSBOnly   = "usb_only"
	StorageCloudOnly = "cloud_only"
)

// Providers, in their mandatory display order.
var Providers = []string{"google", "icloud", "onedrive"}

// Stable error identifiers, localized by the interface.
var (
	ErrWrongSecret     = errors.New("auth.wrong_secret")
	ErrProfileLimit    = errors.New("profile.limit_reached")
	ErrUsernameInvalid = errors.New("profile.username_invalid")
	ErrUsernameTaken   = errors.New("profile.username_taken")
	ErrStorageMode     = errors.New("profile.storage_mode_invalid")
	ErrProviderNeeded  = errors.New("profile.provider_required")
	ErrPassphraseWeak  = errors.New("account.passphrase_too_short")
	ErrNoSuchProfile   = errors.New("profile.not_found")
	ErrNoSuchAccount   = errors.New("account.not_found")
	ErrNotRemembered   = errors.New("account.not_remembered")
)

// Header is the plaintext part of an account. It contains only wrapped keys
// and parameters, never names or data.
type Header struct {
	Schema              int              `json:"schema"`
	AccountID           string           `json:"accountId"`
	Created             time.Time        `json:"created"`
	KDF                 secure.KDFParams `json:"kdf"`
	WrappedByPassphrase string           `json:"wrappedByPassphrase"`
	RecoveryPublic      string           `json:"recoveryPublic"`
	WrappedByRecovery   string           `json:"wrappedByRecovery"`
	// Generation counts passphrase changes. A copy of the header that
	// another machine still holds has a lower one, so it can be told apart
	// from the current header instead of reinstating the old passphrase.
	// Changed orders two changes made from the same generation on
	// different machines, so every machine settles on the same one. Seal
	// binds both to the passphrase wrapping under the account key.
	Generation int       `json:"generation,omitempty"`
	Changed    time.Time `json:"changed,omitzero"`
	Seal       string    `json:"seal,omitempty"`
}

// recoveryHeader is the part of a header that only the recovery key opens.
// It never changes, so a copy of it is kept apart from account.json: the
// recovery key must still open the account when that file is damaged.
type recoveryHeader struct {
	Schema            int       `json:"schema"`
	AccountID         string    `json:"accountId"`
	Created           time.Time `json:"created"`
	RecoveryPublic    string    `json:"recoveryPublic"`
	WrappedByRecovery string    `json:"wrappedByRecovery"`
}

const (
	headerFile    = "account.json"
	recoveryFile  = "account.recovery.json"
	candidateFile = "account.next.json"
)

// ProfileEntry describes a profile inside the sealed account index.
type ProfileEntry struct {
	ID          string    `json:"id"`
	Username    string    `json:"username"`
	AvatarType  string    `json:"avatarType,omitempty"`
	AvatarHash  string    `json:"avatarHash,omitempty"`
	HasPassword bool      `json:"hasPassword"`
	StorageMode string    `json:"storageMode"`
	Provider    string    `json:"provider,omitempty"`
	Created     time.Time `json:"created"`
	Color       int       `json:"color"`
	// KeysUpdated is the Updated time of the newest key file written for
	// the profile. Older key files are superseded and never installed.
	KeysUpdated time.Time `json:"keysUpdated,omitzero"`
}

// Index is the sealed account document.
type Index struct {
	Schema   int            `json:"schema"`
	Name     string         `json:"name"`
	Profiles []ProfileEntry `json:"profiles"`
	Updated  time.Time      `json:"updated"`
	// Header is the account header the index was last sealed with. It
	// carries a passphrase change to machines that synchronize the index,
	// and lets them recognize their own header as superseded.
	Header *Header `json:"header,omitempty"`
	// Removed records removed profiles by ID, with the time of removal, so
	// a merge with an index that still lists one does not bring it back.
	// Profile IDs are random and never reused.
	Removed map[string]time.Time `json:"removed,omitempty"`
}

// ProfileKeys is the key file of one profile.
type ProfileKeys struct {
	Schema            int               `json:"schema"`
	ProfileID         string            `json:"profileId"`
	HasPassword       bool              `json:"hasPassword"`
	KDF               *secure.KDFParams `json:"kdf,omitempty"`
	WrappedByPassword string            `json:"wrappedByPassword,omitempty"`
	WrappedByAccount  string            `json:"wrappedByAccount,omitempty"`
	WrappedByRecovery string            `json:"wrappedByRecovery"`
	Updated           time.Time         `json:"updated"`
}

// Vault is the accounts directory.
type Vault struct {
	Dir string
	// KDF returns parameters for new secrets (tests lower the cost).
	KDF func() secure.KDFParams
}

// New returns a vault rooted at dir (data/accounts).
func New(dir string) *Vault { return &Vault{Dir: dir, KDF: secure.DefaultKDF} }

// Summary is what can be shown before unlocking.
type Summary struct {
	ID         string    `json:"id"`
	Created    time.Time `json:"created"`
	Remembered bool      `json:"remembered"`
}

// List returns the accounts present in this data folder.
func (v *Vault) List() ([]Summary, error) {
	entries, err := os.ReadDir(v.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Summary
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		h, err := v.header(e.Name())
		if err != nil && !errors.Is(err, ErrNoSuchAccount) {
			// A damaged header still lists the account: its recovery key opens it.
			h, err = v.recoveryHeader(e.Name())
		}
		if err != nil {
			continue
		}
		_, rerr := os.Stat(filepath.Join(v.Dir, e.Name(), "drive.key"))
		out = append(out, Summary{ID: h.AccountID, Created: h.Created, Remembered: rerr == nil})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out, nil
}

// Account is an unlocked account.
type Account struct {
	v  *Vault
	id string // Header.AccountID, which never changes; read without the lock
	// mu guards Header, key and Index: a passphrase change, a merge and
	// profile changes replace them while other calls read them.
	mu     sync.Mutex
	Header Header
	key    secure.Key
	Index  Index
}

func (v *Vault) dir(id string) string { return filepath.Join(v.Dir, id) }

// Create makes a new account and returns its one-time recovery key.
func (v *Vault) Create(name, passphrase string, remember bool) (*Account, secure.RecoveryKey, error) {
	var rk secure.RecoveryKey
	if utf8.RuneCountInString(passphrase) < MinPassphrase {
		return nil, rk, ErrPassphraseWeak
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 48 {
		return nil, rk, ErrUsernameInvalid
	}
	rk = secure.NewRecoveryKey()
	ak := secure.NewKey()
	h := Header{Schema: SchemaVersion, AccountID: secure.NewID(), Created: time.Now().UTC().Truncate(time.Second), KDF: v.KDF()}
	if err := wrapPassphrase(&h, ak, passphrase); err != nil {
		return nil, rk, err
	}
	pub, err := rk.PublicKey()
	if err != nil {
		return nil, rk, err
	}
	h.RecoveryPublic = secure.B64(pub)
	sealed, err := secure.SealToRecovery(pub, "account:"+h.AccountID, ak[:])
	if err != nil {
		return nil, rk, err
	}
	h.WrappedByRecovery = secure.B64(sealed)
	a := &Account{v: v, id: h.AccountID, Header: h, key: ak, Index: Index{Schema: SchemaVersion, Name: name, Updated: time.Now().UTC()}}
	if err := atomicfile.WriteSecretJSON(filepath.Join(v.dir(h.AccountID), recoveryFile), recoveryOf(h)); err != nil {
		return nil, rk, err
	}
	if err := atomicfile.WriteSecretJSON(filepath.Join(v.dir(h.AccountID), headerFile), h); err != nil {
		return nil, rk, err
	}
	if err := a.Save(); err != nil {
		return nil, rk, err
	}
	if remember {
		if err := a.Remember(); err != nil {
			return nil, rk, err
		}
	}
	return a, rk, nil
}

func wrapPassphrase(h *Header, ak secure.Key, passphrase string) error {
	kek, err := secure.DeriveKey([]byte(passphrase), h.KDF)
	if err != nil {
		return err
	}
	defer kek.Wipe()
	h.WrappedByPassphrase = secure.B64(secure.Seal(kek, ak[:], []byte("account:"+h.AccountID)))
	h.Seal = secure.B64(secure.Seal(ak, nil, headerAAD(*h)))
	return nil
}

// headerAAD is what a header's seal vouches for.
func headerAAD(h Header) []byte {
	changed := strconv.FormatInt(h.Changed.Unix(), 10) + "." + strconv.Itoa(h.Changed.Nanosecond())
	return []byte("header:" + h.AccountID + ":" + strconv.Itoa(h.Generation) + ":" + changed + ":" + h.WrappedByPassphrase)
}

// supersedes reports whether h is a later passphrase state than o. Two
// changes made from the same generation are ordered by when they were
// made, then by their wrapping, so that every machine picks the same one.
// Headers of generation 0 predate generations and are never ordered.
func (h Header) supersedes(o Header) bool {
	if h.Generation != o.Generation {
		return h.Generation > o.Generation
	}
	if h.Generation == 0 || h.WrappedByPassphrase == o.WrappedByPassphrase {
		return false
	}
	if !h.Changed.Equal(o.Changed) {
		return h.Changed.After(o.Changed)
	}
	return h.WrappedByPassphrase > o.WrappedByPassphrase
}

// vouched reports whether the account key vouches for h's generation.
func (h Header) vouched(ak secure.Key) bool {
	raw, err := secure.UnB64(h.Seal)
	if err != nil {
		return false
	}
	_, err = secure.Open(ak, raw, headerAAD(h))
	return err == nil
}

// checkPassphrase reports whether passphrase opens h's wrapping.
func checkPassphrase(h Header, passphrase string) error {
	kek, err := secure.DeriveKey([]byte(passphrase), h.KDF)
	if err != nil {
		return err
	}
	defer kek.Wipe()
	raw, err := secure.UnB64(h.WrappedByPassphrase)
	if err != nil {
		return ErrWrongSecret
	}
	if _, err := secure.Open(kek, raw, []byte("account:"+h.AccountID)); err != nil {
		return ErrWrongSecret
	}
	return nil
}

func recoveryOf(h Header) recoveryHeader {
	return recoveryHeader{Schema: h.Schema, AccountID: h.AccountID, Created: h.Created, RecoveryPublic: h.RecoveryPublic, WrappedByRecovery: h.WrappedByRecovery}
}

func (v *Vault) header(id string) (Header, error) {
	return v.readHeader(id, headerFile)
}

func (v *Vault) readHeader(id, name string) (Header, error) {
	var h Header
	if err := atomicfile.ReadSecretJSON(filepath.Join(v.dir(id), name), &h); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return h, ErrNoSuchAccount
		}
		return h, err
	}
	if h.Schema > SchemaVersion {
		return h, fmt.Errorf("vault: account written by a newer build")
	}
	if h.AccountID != id {
		return h, errors.New("vault: account header does not match its folder")
	}
	return h, nil
}

// recoveryHeader returns the recovery wrapping of an account whose header
// cannot be read: from its separate copy or, for accounts written by
// earlier builds, from the previous version of the header. Nothing else of
// those copies is used.
func (v *Vault) recoveryHeader(id string) (Header, error) {
	var r recoveryHeader
	if err := atomicfile.ReadSecretJSON(filepath.Join(v.dir(id), recoveryFile), &r); err == nil && r.AccountID == id && r.Schema <= SchemaVersion {
		return Header{Schema: r.Schema, AccountID: r.AccountID, Created: r.Created, RecoveryPublic: r.RecoveryPublic, WrappedByRecovery: r.WrappedByRecovery}, nil
	}
	prev, err := v.readHeader(id, headerFile+".prev")
	if err != nil {
		return Header{}, err
	}
	return Header{Schema: prev.Schema, AccountID: prev.AccountID, Created: prev.Created, RecoveryPublic: prev.RecoveryPublic, WrappedByRecovery: prev.WrappedByRecovery}, nil
}

// Unlock opens an account with its passphrase (works fully offline).
func (v *Vault) Unlock(id, passphrase string) (*Account, error) {
	h, err := v.header(id)
	if err != nil {
		return nil, err
	}
	a, err := v.unlockHeader(h, passphrase)
	if errors.Is(err, ErrWrongSecret) {
		// A Sign In may have fetched a later header from the cloud. It is
		// only a candidate until the account key vouches for it.
		c, cerr := v.readHeader(id, candidateFile)
		if cerr != nil || !c.supersedes(h) {
			return nil, err
		}
		if a, err = v.unlockCandidate(c, passphrase); err != nil {
			return nil, err
		}
		h = c
	}
	if err != nil {
		return nil, err
	}
	if a.Header.WrappedByPassphrase != h.WrappedByPassphrase {
		// The index held a later header, so the passphrase opened a
		// superseded wrapping; it must open the current one as well.
		if err := checkPassphrase(a.Header, passphrase); err != nil {
			a.Lock()
			return nil, err
		}
	}
	return a, nil
}

func (v *Vault) unlockHeader(h Header, passphrase string) (*Account, error) {
	kek, err := secure.DeriveKey([]byte(passphrase), h.KDF)
	if err != nil {
		return nil, err
	}
	defer kek.Wipe()
	return v.open(h, kek, h.WrappedByPassphrase)
}

// unlockCandidate opens an account with a candidate header, which must be
// vouched for by the account key it opens.
func (v *Vault) unlockCandidate(c Header, passphrase string) (*Account, error) {
	kek, err := secure.DeriveKey([]byte(passphrase), c.KDF)
	if err != nil {
		return nil, err
	}
	defer kek.Wipe()
	ak, err := unwrap(c, kek, c.WrappedByPassphrase)
	if err != nil {
		return nil, err
	}
	if !c.vouched(ak) {
		ak.Wipe()
		return nil, ErrWrongSecret
	}
	return v.load(c, ak)
}

func unwrap(h Header, kek secure.Key, wrapped string) (secure.Key, error) {
	var zero secure.Key
	raw, err := secure.UnB64(wrapped)
	if err != nil {
		return zero, ErrWrongSecret
	}
	akb, err := secure.Open(kek, raw, []byte("account:"+h.AccountID))
	if err != nil {
		return zero, ErrWrongSecret
	}
	return secure.KeyFromBytes(akb)
}

func (v *Vault) open(h Header, kek secure.Key, wrapped string) (*Account, error) {
	ak, err := unwrap(h, kek, wrapped)
	if err != nil {
		return nil, err
	}
	return v.load(h, ak)
}

func (v *Vault) load(h Header, ak secure.Key) (*Account, error) {
	a := &Account{v: v, id: h.AccountID, Header: h, key: ak}
	b, _, err := atomicfile.ReadChecked(filepath.Join(v.dir(h.AccountID), "account.data"))
	if err != nil {
		return nil, err
	}
	pt, err := secure.Open(ak, b, []byte("index:"+h.AccountID))
	if err != nil {
		return nil, ErrWrongSecret
	}
	if err := json.Unmarshal(pt, &a.Index); err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.reconcileHeaderLocked()
	a.mu.Unlock()
	v.removeSuperseded(h.AccountID)
	return a, nil
}

// adoptCandidateLocked installs the header a Sign In fetched when the
// account key vouches for it and it is later than the one in use. Forged
// or superseded candidates are discarded.
func (a *Account) adoptCandidateLocked() {
	id := a.Header.AccountID
	c, err := a.v.readHeader(id, candidateFile)
	if errors.Is(err, ErrNoSuchAccount) {
		return
	}
	if err == nil && c.vouched(a.key) && c.supersedes(a.Header) {
		a.adoptHeaderLocked(c)
	}
	atomicfile.Remove(filepath.Join(a.v.dir(id), candidateFile))
}

// reconcileHeaderLocked compares the plaintext header with the one sealed
// in the index. A later one there means the plaintext header is a
// superseded copy, for example one another machine republished, and it is
// replaced; otherwise the index is brought up to date. Both writes are
// repairs: if one fails, the right header is still the one in use.
func (a *Account) reconcileHeaderLocked() {
	if a.Header.Generation != 0 && !a.Header.vouched(a.key) {
		a.Header.Generation, a.Header.Changed, a.Header.Seal = 0, time.Time{}, ""
	}
	// The recovery copy is written before the header may be replaced:
	// that drops the previous version an earlier build kept.
	rp := filepath.Join(a.Dir(), recoveryFile)
	var r recoveryHeader
	if atomicfile.ReadSecretJSON(rp, &r) != nil || r != recoveryOf(a.Header) {
		atomicfile.WriteSecretJSON(rp, recoveryOf(a.Header))
	}
	disk, err := a.v.header(a.Header.AccountID)
	if err != nil || disk.WrappedByPassphrase != a.Header.WrappedByPassphrase || disk.Seal != a.Header.Seal {
		// The header in use came from a candidate or a recovery copy.
		atomicfile.WriteSecretJSON(filepath.Join(a.Dir(), headerFile), a.Header)
	}
	a.adoptCandidateLocked()
	ih := a.Index.Header
	switch {
	case ih != nil && ih.AccountID == a.Header.AccountID && ih.WrappedByPassphrase != "" && (a.Header.WrappedByPassphrase == "" || ih.supersedes(a.Header)):
		// Without a passphrase wrapping, the header in use came from the
		// recovery copy; the index holds the last one sealed.
		a.adoptHeaderLocked(*ih)
	case ih == nil || ih.Generation != a.Header.Generation || ih.WrappedByPassphrase != a.Header.WrappedByPassphrase || !ih.Changed.Equal(a.Header.Changed):
		a.sealIndexLocked()
	}
}

// adoptHeaderLocked installs a later header. Only the passphrase wrapping
// changes between versions, and the account identifier is read without
// the lock, so only those fields are replaced.
func (a *Account) adoptHeaderLocked(later Header) error {
	a.Header.KDF, a.Header.WrappedByPassphrase = later.KDF, later.WrappedByPassphrase
	a.Header.Generation, a.Header.Changed, a.Header.Seal = later.Generation, later.Changed, later.Seal
	return atomicfile.WriteSecretJSON(filepath.Join(a.Dir(), headerFile), a.Header)
}

// removeSuperseded deletes previous versions of key files kept by earlier
// builds. Each still opens with what it was wrapped by: an old passphrase,
// or the account key for a profile that has since been given a password.
// While the current file is damaged or missing, its previous version may
// hold the only intact recovery wrapping and stays until the file is
// written again. A drive key is never needed for that.
func (v *Vault) removeSuperseded(id string) {
	dir := v.dir(id)
	atomicfile.RemovePrevious(filepath.Join(dir, "drive.key"))
	files := []string{filepath.Join(dir, "account.json")}
	for _, parent := range []string{"profiles", "trash"} {
		entries, _ := os.ReadDir(filepath.Join(dir, parent))
		for _, e := range entries {
			if e.IsDir() {
				files = append(files, filepath.Join(dir, parent, e.Name(), "profile.key.json"))
			}
		}
	}
	for _, f := range files {
		if _, err := atomicfile.ReadSecret(f); err == nil {
			atomicfile.RemovePrevious(f)
		}
	}
}

// UnlockRemembered opens an account using this drive's stored key.
func (v *Vault) UnlockRemembered(id string) (*Account, error) {
	h, err := v.header(id)
	if err != nil {
		return nil, err
	}
	var dk struct {
		Key     string `json:"key"`
		Wrapped string `json:"wrapped"`
	}
	if err := atomicfile.ReadSecretJSON(filepath.Join(v.dir(id), "drive.key"), &dk); err != nil {
		return nil, ErrNotRemembered
	}
	kb, err := secure.UnB64(dk.Key)
	if err != nil {
		return nil, ErrNotRemembered
	}
	k, err := secure.KeyFromBytes(kb)
	if err != nil {
		return nil, ErrNotRemembered
	}
	return v.open(h, k, dk.Wrapped)
}

// UnlockWithRecovery opens an account with the recovery key and sets a new
// passphrase.
func (v *Vault) UnlockWithRecovery(id string, rk secure.RecoveryKey, newPassphrase string) (*Account, error) {
	if utf8.RuneCountInString(newPassphrase) < MinPassphrase {
		return nil, ErrPassphraseWeak
	}
	h, err := v.header(id)
	if err != nil && !errors.Is(err, ErrNoSuchAccount) {
		// The header is damaged; its recovery wrapping is kept apart.
		if rh, rerr := v.recoveryHeader(id); rerr == nil {
			h, err = rh, nil
		}
	}
	if err != nil {
		return nil, err
	}
	sealed, err := secure.UnB64(h.WrappedByRecovery)
	if err != nil {
		return nil, ErrWrongSecret
	}
	akb, err := secure.OpenWithRecovery(rk, "account:"+h.AccountID, sealed)
	if err != nil {
		return nil, ErrWrongSecret
	}
	ak, _ := secure.KeyFromBytes(akb)
	a, err := v.load(h, ak)
	if err != nil {
		return nil, err
	}
	return a, a.ChangePassphrase(newPassphrase)
}

// ImportHeader installs an account header and sealed index fetched from a
// cloud provider so the account can be unlocked on this machine. Readable
// local copies are kept, since they can hold a passphrase change or
// profiles not uploaded yet; synchronizing after the unlock reconciles
// them. A later remote header is kept apart as a candidate: the cloud
// copy is not authenticated, so it replaces the local header only once the
// account key opened by unlocking vouches for it.
func (v *Vault) ImportHeader(headerJSON, sealedIndex []byte) (string, error) {
	var h Header
	if err := json.Unmarshal(headerJSON, &h); err != nil || h.AccountID == "" || strings.ContainsAny(h.AccountID, `/\.`) {
		return "", errors.New("vault: remote account header is not valid")
	}
	if cur, err := v.header(h.AccountID); err != nil {
		if err := atomicfile.WriteSecretJSON(filepath.Join(v.dir(h.AccountID), headerFile), h); err != nil {
			return "", err
		}
	} else if h.supersedes(cur) {
		if err := atomicfile.WriteSecretJSON(filepath.Join(v.dir(h.AccountID), candidateFile), h); err != nil {
			return "", err
		}
	}
	data := filepath.Join(v.dir(h.AccountID), "account.data")
	if _, _, err := atomicfile.ReadChecked(data); err == nil {
		return h.AccountID, nil
	}
	return h.AccountID, atomicfile.WriteChecked(data, sealedIndex)
}

// ---- Account methods ----

// ID returns the account identifier.
func (a *Account) ID() string { return a.id }

// Name returns the account name.
func (a *Account) Name() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Index.Name
}

// Dir returns the account directory.
func (a *Account) Dir() string { return a.v.dir(a.id) }

// Save persists the sealed index.
func (a *Account) Save() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.saveLocked()
}

// SealedIndex returns the current sealed index bytes (for cloud upload).
func (a *Account) SealedIndex() ([]byte, error) {
	b, _, err := atomicfile.ReadChecked(filepath.Join(a.Dir(), "account.data"))
	return b, err
}

// HeaderJSON returns the header document (for cloud upload).
func (a *Account) HeaderJSON() ([]byte, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return json.Marshal(a.Header)
}

// ChangePassphrase re-wraps the account key under a new passphrase.
func (a *Account) ChangePassphrase(p string) error {
	if utf8.RuneCountInString(p) < MinPassphrase {
		return ErrPassphraseWeak
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	h := a.Header
	h.KDF = a.v.KDF()
	h.Generation++
	h.Changed = time.Now().UTC()
	if err := wrapPassphrase(&h, a.key, p); err != nil {
		return err
	}
	if err := atomicfile.WriteSecretJSON(filepath.Join(a.Dir(), headerFile), h); err != nil {
		return err
	}
	a.Header = h
	// Other machines learn of the change from the index they synchronize.
	return a.sealIndexLocked()
}

// Remember stores a drive key so this drive opens the account without the
// passphrase. Profiles protected by a password stay protected.
func (a *Account) Remember() error {
	k := secure.NewKey()
	a.mu.Lock()
	ak := a.key
	a.mu.Unlock()
	doc := map[string]string{
		"key":     secure.B64(k[:]),
		"wrapped": secure.B64(secure.Seal(k, ak[:], []byte("account:"+a.id))),
	}
	return atomicfile.WriteSecretJSON(filepath.Join(a.Dir(), "drive.key"), doc)
}

// Forget removes the drive key.
func (a *Account) Forget() error { return atomicfile.Remove(filepath.Join(a.Dir(), "drive.key")) }

// Remembered reports whether this drive keeps the account signed in.
func (a *Account) Remembered() bool {
	_, err := os.Stat(filepath.Join(a.Dir(), "drive.key"))
	return err == nil
}

// Lock wipes the account key from memory.
func (a *Account) Lock() {
	a.mu.Lock()
	a.key.Wipe()
	a.mu.Unlock()
}

// Profiles returns the profile entries in creation order.
func (a *Account) Profiles() []ProfileEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]ProfileEntry(nil), a.Index.Profiles...)
}

// Profile finds a profile entry.
func (a *Account) Profile(id string) (ProfileEntry, error) {
	for _, p := range a.Profiles() {
		if p.ID == id {
			return p, nil
		}
	}
	return ProfileEntry{}, ErrNoSuchProfile
}

// ProfileDir is the directory holding a profile's store and backups.
func (a *Account) ProfileDir(id string) string { return filepath.Join(a.Dir(), "profiles", id) }

// ProfileInput describes a new profile.
type ProfileInput struct {
	Username    string
	StorageMode string
	Provider    string
	Password    string
	Avatar      []byte
	AvatarType  string
}

// ValidateUsername normalizes and checks a username.
func ValidateUsername(s string) (string, error) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if n < 1 || n > 32 {
		return "", ErrUsernameInvalid
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == '/' || r == '\\' {
			return "", ErrUsernameInvalid
		}
	}
	return s, nil
}

// ValidateStorage checks a storage mode and provider combination.
func ValidateStorage(mode, provider string) error {
	switch mode {
	case StorageUSBOnly:
		if provider != "" {
			return ErrStorageMode
		}
		return nil
	case StorageUSBCloud, StorageCloudOnly:
		for _, p := range Providers {
			if p == provider {
				return nil
			}
		}
		return ErrProviderNeeded
	}
	return ErrStorageMode
}

// CreateProfile adds a profile, enforcing the five-profile limit.
func (a *Account) CreateProfile(in ProfileInput) (ProfileEntry, error) {
	user, err := ValidateUsername(in.Username)
	if err != nil {
		return ProfileEntry{}, err
	}
	if err := ValidateStorage(in.StorageMode, in.Provider); err != nil {
		return ProfileEntry{}, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.Index.Profiles) >= MaxProfiles {
		return ProfileEntry{}, ErrProfileLimit
	}
	for _, p := range a.Index.Profiles {
		if strings.EqualFold(p.Username, user) {
			return ProfileEntry{}, ErrUsernameTaken
		}
	}
	pe := ProfileEntry{ID: secure.NewID(), Username: user, StorageMode: in.StorageMode, Provider: in.Provider,
		Created: time.Now().UTC().Truncate(time.Second), HasPassword: in.Password != "", Color: len(a.Index.Profiles)}
	pdk := secure.NewKey()
	defer pdk.Wipe()
	pk, err := a.wrapProfileLocked(pe.ID, pdk, in.Password)
	if err != nil {
		return ProfileEntry{}, err
	}
	if err := atomicfile.WriteSecretJSON(filepath.Join(a.ProfileDir(pe.ID), "profile.key.json"), pk); err != nil {
		return ProfileEntry{}, err
	}
	pe.KeysUpdated = pk.Updated
	if len(in.Avatar) > 0 {
		if err := a.writeAvatarLocked(pe.ID, in.Avatar); err != nil {
			return ProfileEntry{}, err
		}
		pe.AvatarType = in.AvatarType
		pe.AvatarHash = secure.HashHex(in.Avatar)[:16]
	}
	a.Index.Profiles = append(a.Index.Profiles, pe)
	if err := a.saveLocked(); err != nil {
		a.Index.Profiles = a.Index.Profiles[:len(a.Index.Profiles)-1]
		return ProfileEntry{}, err
	}
	return pe, nil
}

func (a *Account) saveLocked() error {
	a.Index.Updated = time.Now().UTC()
	return a.sealIndexLocked()
}

// sealIndexLocked writes the index with the current header. It leaves
// Updated alone: MergeIndex orders profile entries by it, and a header
// change is not a change to them.
func (a *Account) sealIndexLocked() error {
	h := a.Header
	a.Index.Header = &h
	b, err := json.Marshal(a.Index)
	if err != nil {
		return err
	}
	return atomicfile.WriteChecked(filepath.Join(a.Dir(), "account.data"), secure.Seal(a.key, b, []byte("index:"+a.Header.AccountID)))
}

func (a *Account) wrapProfileLocked(pid string, pdk secure.Key, password string) (ProfileKeys, error) {
	pk := ProfileKeys{Schema: SchemaVersion, ProfileID: pid, HasPassword: password != "", Updated: time.Now().UTC()}
	aad := []byte("profile:" + pid)
	if password != "" {
		kdf := a.v.KDF()
		kek, err := secure.DeriveKey([]byte(password), kdf)
		if err != nil {
			return pk, err
		}
		defer kek.Wipe()
		pk.KDF = &kdf
		pk.WrappedByPassword = secure.B64(secure.Seal(kek, pdk[:], aad))
	} else {
		pk.WrappedByAccount = secure.B64(secure.Seal(a.key, pdk[:], aad))
	}
	pub, err := secure.UnB64(a.Header.RecoveryPublic)
	if err != nil {
		return pk, err
	}
	sealed, err := secure.SealToRecovery(pub, "profile:"+pid, pdk[:])
	if err != nil {
		return pk, err
	}
	pk.WrappedByRecovery = secure.B64(sealed)
	return pk, nil
}

// ProfileKeysJSON returns a profile's key file (for cloud upload).
func (a *Account) ProfileKeysJSON(pid string) ([]byte, error) {
	return atomicfile.ReadSecret(filepath.Join(a.ProfileDir(pid), "profile.key.json"))
}

// ImportProfileKeys installs a profile key file fetched from the cloud when
// it is newer than the local one. It reports whether it was installed.
// A file older than the one the sealed index records is superseded: it
// may still open with the account key or an old password, so it is not
// installed even when the local file is missing or damaged. Nor is a file
// without a password for a profile the index says has one; that change
// reaches the index with the key file that made it. A newer file that adds
// a password is installed and recorded in the index, since indexes written
// by earlier builds may not know of it.
func (a *Account) ImportProfileKeys(pid string, b []byte) (bool, error) {
	var pk ProfileKeys
	if err := json.Unmarshal(b, &pk); err != nil || pk.ProfileID != pid || pk.WrappedByRecovery == "" {
		return false, errors.New("vault: remote profile keys are not valid")
	}
	e, err := a.Profile(pid)
	if err != nil {
		return false, err
	}
	if pk.Updated.Before(e.KeysUpdated) || (!pk.HasPassword && e.HasPassword) || (pk.HasPassword != e.HasPassword && !pk.Updated.After(e.KeysUpdated)) {
		return false, nil
	}
	if cur, err := a.profileKeys(pid); err == nil && !pk.Updated.After(cur.Updated) {
		return false, nil
	}
	if err := atomicfile.WriteSecret(filepath.Join(a.ProfileDir(pid), "profile.key.json"), b); err != nil {
		return false, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.Index.Profiles {
		if p := &a.Index.Profiles[i]; p.ID == pid && pk.Updated.After(p.KeysUpdated) {
			p.HasPassword, p.KeysUpdated = pk.HasPassword, pk.Updated
			return true, a.saveLocked()
		}
	}
	return true, nil
}

// ProfileKeysCurrent reports whether the local key file of a profile is the
// one the index records, so that it may be published.
func (a *Account) ProfileKeysCurrent(pid string) bool {
	pk, err := a.profileKeys(pid)
	if err != nil {
		return false
	}
	e, err := a.Profile(pid)
	return err == nil && pk.HasPassword == e.HasPassword && !pk.Updated.Before(e.KeysUpdated)
}

// ProfileKeysUpdated returns when a profile's key file last changed.
func (a *Account) ProfileKeysUpdated(pid string) time.Time {
	pk, err := a.profileKeys(pid)
	if err != nil {
		return time.Time{}
	}
	return pk.Updated
}

func (a *Account) profileKeys(pid string) (ProfileKeys, error) {
	var pk ProfileKeys
	if err := atomicfile.ReadSecretJSON(filepath.Join(a.ProfileDir(pid), "profile.key.json"), &pk); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return pk, ErrNoSuchProfile
		}
		return pk, err
	}
	return pk, nil
}

// UnlockProfile returns the profile data key. Profiles with a password
// require it; the account key alone cannot open them.
func (a *Account) UnlockProfile(pid, password string) (secure.Key, error) {
	var zero secure.Key
	pk, err := a.profileKeys(pid)
	if err != nil {
		return zero, err
	}
	aad := []byte("profile:" + pid)
	var kek secure.Key
	var wrapped string
	if pk.HasPassword {
		if pk.KDF == nil {
			return zero, ErrWrongSecret
		}
		kek, err = secure.DeriveKey([]byte(password), *pk.KDF)
		if err != nil {
			return zero, err
		}
		defer kek.Wipe()
		wrapped = pk.WrappedByPassword
	} else {
		a.mu.Lock()
		kek = a.key
		a.mu.Unlock()
		wrapped = pk.WrappedByAccount
	}
	raw, err := secure.UnB64(wrapped)
	if err != nil {
		return zero, ErrWrongSecret
	}
	b, err := secure.Open(kek, raw, aad)
	if err != nil {
		return zero, ErrWrongSecret
	}
	return secure.KeyFromBytes(b)
}

// SetProfilePassword changes, adds or (with an empty value) removes a
// profile password. The current key must already be unlocked.
func (a *Account) SetProfilePassword(pid string, pdk secure.Key, newPassword string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	pk, err := a.wrapProfileLocked(pid, pdk, newPassword)
	if err != nil {
		return err
	}
	if err := atomicfile.WriteSecretJSON(filepath.Join(a.ProfileDir(pid), "profile.key.json"), pk); err != nil {
		return err
	}
	for i := range a.Index.Profiles {
		if a.Index.Profiles[i].ID == pid {
			a.Index.Profiles[i].HasPassword = newPassword != ""
			a.Index.Profiles[i].KeysUpdated = pk.Updated
		}
	}
	return a.saveLocked()
}

// RecoverProfile opens a profile with the recovery key (forgotten password).
func (a *Account) RecoverProfile(pid string, rk secure.RecoveryKey) (secure.Key, error) {
	var zero secure.Key
	pk, err := a.profileKeys(pid)
	if err != nil {
		// An earlier build may have left the previous version next to a
		// damaged or missing key file. Only its recovery wrapping is used,
		// which seals the same data key to the same recovery key.
		var prev ProfileKeys
		if perr := atomicfile.ReadSecretJSON(filepath.Join(a.ProfileDir(pid), "profile.key.json.prev"), &prev); perr != nil || prev.ProfileID != pid {
			return zero, err
		}
		pk = prev
	}
	sealed, err := secure.UnB64(pk.WrappedByRecovery)
	if err != nil {
		return zero, ErrWrongSecret
	}
	b, err := secure.OpenWithRecovery(rk, "profile:"+pid, sealed)
	if err != nil {
		return zero, ErrWrongSecret
	}
	return secure.KeyFromBytes(b)
}

// UpdateProfile applies changes to a profile entry (storage, provider,
// username) after validation.
func (a *Account) UpdateProfile(pid string, fn func(*ProfileEntry) error) (ProfileEntry, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.Index.Profiles {
		if a.Index.Profiles[i].ID != pid {
			continue
		}
		p := a.Index.Profiles[i]
		if err := fn(&p); err != nil {
			return p, err
		}
		u, err := ValidateUsername(p.Username)
		if err != nil {
			return p, err
		}
		p.Username = u
		for j, o := range a.Index.Profiles {
			if j != i && strings.EqualFold(o.Username, u) {
				return p, ErrUsernameTaken
			}
		}
		if err := ValidateStorage(p.StorageMode, p.Provider); err != nil {
			return p, err
		}
		old := a.Index.Profiles[i]
		a.Index.Profiles[i] = p
		if err := a.saveLocked(); err != nil {
			a.Index.Profiles[i] = old
			return p, err
		}
		return p, nil
	}
	return ProfileEntry{}, ErrNoSuchProfile
}

// SetAvatar stores or clears a profile avatar.
func (a *Account) SetAvatar(pid string, data []byte, mime string) error {
	if len(data) == 0 {
		atomicfile.Remove(filepath.Join(a.Dir(), "avatars", pid))
	} else {
		a.mu.Lock()
		err := a.writeAvatarLocked(pid, data)
		a.mu.Unlock()
		if err != nil {
			return err
		}
	}
	_, err := a.UpdateProfile(pid, func(p *ProfileEntry) error {
		if len(data) == 0 {
			p.AvatarType, p.AvatarHash = "", ""
		} else {
			p.AvatarType, p.AvatarHash = mime, secure.HashHex(data)[:16]
		}
		return nil
	})
	return err
}

func (a *Account) writeAvatarLocked(pid string, data []byte) error {
	return atomicfile.WriteChecked(filepath.Join(a.Dir(), "avatars", pid), secure.Seal(a.key, data, []byte("avatar:"+pid)))
}

// Avatar returns a profile's avatar bytes, if any.
func (a *Account) Avatar(pid string) ([]byte, error) {
	b, _, err := atomicfile.ReadChecked(filepath.Join(a.Dir(), "avatars", pid))
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	k := a.key
	a.mu.Unlock()
	return secure.Open(k, b, []byte("avatar:"+pid))
}

// SealedAvatar returns the sealed avatar file (for cloud upload).
func (a *Account) SealedAvatar(pid string) ([]byte, error) {
	b, _, err := atomicfile.ReadChecked(filepath.Join(a.Dir(), "avatars", pid))
	return b, err
}

// ImportSealedAvatar installs a sealed avatar fetched from the cloud after
// authenticating it.
func (a *Account) ImportSealedAvatar(pid string, b []byte) error {
	a.mu.Lock()
	k := a.key
	a.mu.Unlock()
	if _, err := secure.Open(k, b, []byte("avatar:"+pid)); err != nil {
		return ErrWrongSecret
	}
	return atomicfile.WriteChecked(filepath.Join(a.Dir(), "avatars", pid), b)
}

// OpenIndex authenticates and decodes a sealed index (from the cloud).
func (a *Account) OpenIndex(sealed []byte) (Index, error) {
	a.mu.Lock()
	k := a.key
	a.mu.Unlock()
	var idx Index
	pt, err := secure.Open(k, sealed, []byte("index:"+a.id))
	if err != nil {
		return idx, ErrWrongSecret
	}
	return idx, json.Unmarshal(pt, &idx)
}

// MergeIndex merges a remote index into the local one. Profiles are
// independent entries, so the merge is a union by profile ID; for a profile
// present in both, the entry from the more recently updated index wins.
// A profile removed in either index stays removed. Profiles created on two
// machines at once can exceed MaxProfiles; all are kept, since dropping an
// entry would hide its data, and no new profile can be created until one is
// removed. It reports whether the local index changed.
func (a *Account) MergeIndex(remote Index) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	local := a.Index
	byID := map[string]ProfileEntry{}
	order := []string{}
	add := func(p ProfileEntry, prefer bool) {
		cur, ok := byID[p.ID]
		if !ok {
			order = append(order, p.ID)
			byID[p.ID] = p
			return
		}
		win, other := cur, p
		if prefer {
			win, other = p, cur
		}
		// Whether a password protects the profile follows its newest key
		// file, whichever index recorded it.
		if other.KeysUpdated.After(win.KeysUpdated) {
			win.HasPassword, win.KeysUpdated = other.HasPassword, other.KeysUpdated
		}
		byID[p.ID] = win
	}
	removed := maps.Clone(local.Removed)
	for id, at := range remote.Removed {
		if cur, ok := removed[id]; !ok || at.After(cur) {
			if removed == nil {
				removed = map[string]time.Time{}
			}
			removed[id] = at
		}
	}
	remoteNewer := remote.Updated.After(local.Updated)
	for _, p := range local.Profiles {
		add(p, false)
	}
	for _, p := range remote.Profiles {
		add(p, remoteNewer)
	}
	merged := make([]ProfileEntry, 0, len(order))
	for _, id := range order {
		if _, gone := removed[id]; !gone {
			merged = append(merged, byID[id])
		}
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Created.Before(merged[j].Created) })
	name := local.Name
	if remoteNewer && remote.Name != "" {
		name = remote.Name
	}
	adopted := false
	if r := remote.Header; r != nil && r.AccountID == a.Header.AccountID && r.supersedes(a.Header) {
		// A passphrase change made on another machine. This header is
		// superseded and must not be published again.
		if err := a.adoptHeaderLocked(*r); err != nil {
			return false, err
		}
		adopted = true
	}
	before, _ := json.Marshal([]any{local.Profiles, local.Removed})
	after, _ := json.Marshal([]any{merged, removed})
	if string(before) == string(after) && name == local.Name {
		if adopted {
			return true, a.sealIndexLocked()
		}
		return false, nil
	}
	a.Index.Profiles = merged
	a.Index.Name = name
	a.Index.Removed = removed
	if err := a.saveLocked(); err != nil {
		a.Index.Profiles, a.Index.Name, a.Index.Removed = local.Profiles, local.Name, local.Removed
		return false, err
	}
	return true, nil
}

// RemoveProfile moves a profile into the account's trash folder. Nothing is
// destroyed: the folder can be restored.
func (a *Account) RemoveProfile(pid string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	idx := -1
	for i, p := range a.Index.Profiles {
		if p.ID == pid {
			idx = i
		}
	}
	if idx < 0 {
		return ErrNoSuchProfile
	}
	trash := filepath.Join(a.Dir(), "trash", pid+"-"+time.Now().UTC().Format("20060102T150405"))
	os.MkdirAll(filepath.Dir(trash), 0o700)
	if err := os.Rename(a.ProfileDir(pid), trash); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	profiles, tombstones := a.Index.Profiles, a.Index.Removed
	a.Index.Profiles = slices.Delete(slices.Clone(profiles), idx, idx+1)
	a.Index.Removed = maps.Clone(tombstones)
	if a.Index.Removed == nil {
		a.Index.Removed = map[string]time.Time{}
	}
	a.Index.Removed[pid] = time.Now().UTC().Truncate(time.Second)
	if err := a.saveLocked(); err != nil {
		a.Index.Profiles, a.Index.Removed = profiles, tombstones
		return err
	}
	return nil
}

// Key exposes the account key to packages that seal account-level files
// (synchronization of the index). It must not be logged or persisted.
func (a *Account) Key() secure.Key {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.key
}

// UnlockWithKey opens an account with its raw key. It is used only for the
// in-memory handoff of a restart (Turbo, update): the key travels over a
// private pipe to the new process and is never written anywhere.
func (v *Vault) UnlockWithKey(id string, ak secure.Key) (*Account, error) {
	h, err := v.header(id)
	if err != nil {
		return nil, err
	}
	return v.load(h, ak)
}
