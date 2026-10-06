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
	"os"
	"path/filepath"
	"sort"
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
}

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
}

// Index is the sealed account document.
type Index struct {
	Schema   int            `json:"schema"`
	Name     string         `json:"name"`
	Profiles []ProfileEntry `json:"profiles"`
	Updated  time.Time      `json:"updated"`
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
		var h Header
		if _, err := atomicfile.ReadJSON(filepath.Join(v.Dir, e.Name(), "account.json"), &h); err != nil {
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
	v      *Vault
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
	a := &Account{v: v, Header: h, key: ak, Index: Index{Schema: SchemaVersion, Name: name, Updated: time.Now().UTC()}}
	if err := atomicfile.WriteJSON(filepath.Join(v.dir(h.AccountID), "account.json"), h); err != nil {
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
	return nil
}

func (v *Vault) header(id string) (Header, error) {
	var h Header
	if _, err := atomicfile.ReadJSON(filepath.Join(v.dir(id), "account.json"), &h); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return h, ErrNoSuchAccount
		}
		return h, err
	}
	if h.Schema > SchemaVersion {
		return h, fmt.Errorf("vault: account written by a newer build")
	}
	return h, nil
}

// Unlock opens an account with its passphrase (works fully offline).
func (v *Vault) Unlock(id, passphrase string) (*Account, error) {
	h, err := v.header(id)
	if err != nil {
		return nil, err
	}
	kek, err := secure.DeriveKey([]byte(passphrase), h.KDF)
	if err != nil {
		return nil, err
	}
	defer kek.Wipe()
	return v.open(h, kek, h.WrappedByPassphrase)
}

func (v *Vault) open(h Header, kek secure.Key, wrapped string) (*Account, error) {
	raw, err := secure.UnB64(wrapped)
	if err != nil {
		return nil, ErrWrongSecret
	}
	akb, err := secure.Open(kek, raw, []byte("account:"+h.AccountID))
	if err != nil {
		return nil, ErrWrongSecret
	}
	ak, _ := secure.KeyFromBytes(akb)
	return v.load(h, ak)
}

func (v *Vault) load(h Header, ak secure.Key) (*Account, error) {
	a := &Account{v: v, Header: h, key: ak}
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
	return a, nil
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
	if _, err := atomicfile.ReadJSON(filepath.Join(v.dir(id), "drive.key"), &dk); err != nil {
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
// cloud provider so the account can be unlocked on this machine.
func (v *Vault) ImportHeader(headerJSON, sealedIndex []byte) (string, error) {
	var h Header
	if err := json.Unmarshal(headerJSON, &h); err != nil || h.AccountID == "" || strings.ContainsAny(h.AccountID, `/\.`) {
		return "", errors.New("vault: remote account header is not valid")
	}
	if err := atomicfile.WriteJSON(filepath.Join(v.dir(h.AccountID), "account.json"), h); err != nil {
		return "", err
	}
	return h.AccountID, atomicfile.WriteChecked(filepath.Join(v.dir(h.AccountID), "account.data"), sealedIndex)
}

// ---- Account methods ----

// ID returns the account identifier.
func (a *Account) ID() string { return a.Header.AccountID }

// Dir returns the account directory.
func (a *Account) Dir() string { return a.v.dir(a.Header.AccountID) }

// Save persists the sealed index.
func (a *Account) Save() error {
	a.Index.Updated = time.Now().UTC()
	b, err := json.Marshal(a.Index)
	if err != nil {
		return err
	}
	return atomicfile.WriteChecked(filepath.Join(a.Dir(), "account.data"), secure.Seal(a.key, b, []byte("index:"+a.Header.AccountID)))
}

// SealedIndex returns the current sealed index bytes (for cloud upload).
func (a *Account) SealedIndex() ([]byte, error) {
	b, _, err := atomicfile.ReadChecked(filepath.Join(a.Dir(), "account.data"))
	return b, err
}

// HeaderJSON returns the header document (for cloud upload).
func (a *Account) HeaderJSON() ([]byte, error) { return json.Marshal(a.Header) }

// ChangePassphrase re-wraps the account key under a new passphrase.
func (a *Account) ChangePassphrase(p string) error {
	if utf8.RuneCountInString(p) < MinPassphrase {
		return ErrPassphraseWeak
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	h := a.Header
	h.KDF = a.v.KDF()
	if err := wrapPassphrase(&h, a.key, p); err != nil {
		return err
	}
	if err := atomicfile.WriteJSON(filepath.Join(a.Dir(), "account.json"), h); err != nil {
		return err
	}
	a.Header = h
	return nil
}

// Remember stores a drive key so this drive opens the account without the
// passphrase. Profiles protected by a password stay protected.
func (a *Account) Remember() error {
	k := secure.NewKey()
	doc := map[string]string{
		"key":     secure.B64(k[:]),
		"wrapped": secure.B64(secure.Seal(k, a.key[:], []byte("account:"+a.Header.AccountID))),
	}
	return atomicfile.WriteJSON(filepath.Join(a.Dir(), "drive.key"), doc)
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
	pk, err := a.wrapProfile(pe.ID, pdk, in.Password)
	if err != nil {
		return ProfileEntry{}, err
	}
	if err := atomicfile.WriteJSON(filepath.Join(a.ProfileDir(pe.ID), "profile.key.json"), pk); err != nil {
		return ProfileEntry{}, err
	}
	if len(in.Avatar) > 0 {
		if err := a.writeAvatar(pe.ID, in.Avatar); err != nil {
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
	b, err := json.Marshal(a.Index)
	if err != nil {
		return err
	}
	return atomicfile.WriteChecked(filepath.Join(a.Dir(), "account.data"), secure.Seal(a.key, b, []byte("index:"+a.Header.AccountID)))
}

func (a *Account) wrapProfile(pid string, pdk secure.Key, password string) (ProfileKeys, error) {
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
	b, _, err := atomicfile.ReadChecked(filepath.Join(a.ProfileDir(pid), "profile.key.json"))
	return b, err
}

// ImportProfileKeys installs a profile key file fetched from the cloud when
// it is newer than the local one. It reports whether it was installed.
func (a *Account) ImportProfileKeys(pid string, b []byte) (bool, error) {
	var pk ProfileKeys
	if err := json.Unmarshal(b, &pk); err != nil || pk.ProfileID != pid || pk.WrappedByRecovery == "" {
		return false, errors.New("vault: remote profile keys are not valid")
	}
	if cur, err := a.profileKeys(pid); err == nil && !pk.Updated.After(cur.Updated) {
		return false, nil
	}
	return true, atomicfile.WriteChecked(filepath.Join(a.ProfileDir(pid), "profile.key.json"), b)
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
	if _, err := atomicfile.ReadJSON(filepath.Join(a.ProfileDir(pid), "profile.key.json"), &pk); err != nil {
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
	pk, err := a.wrapProfile(pid, pdk, newPassword)
	if err != nil {
		return err
	}
	if err := atomicfile.WriteJSON(filepath.Join(a.ProfileDir(pid), "profile.key.json"), pk); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.Index.Profiles {
		if a.Index.Profiles[i].ID == pid {
			a.Index.Profiles[i].HasPassword = newPassword != ""
		}
	}
	return a.saveLocked()
}

// RecoverProfile opens a profile with the recovery key (forgotten password).
func (a *Account) RecoverProfile(pid string, rk secure.RecoveryKey) (secure.Key, error) {
	var zero secure.Key
	pk, err := a.profileKeys(pid)
	if err != nil {
		return zero, err
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
	} else if err := a.writeAvatar(pid, data); err != nil {
		return err
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

func (a *Account) writeAvatar(pid string, data []byte) error {
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
	pt, err := secure.Open(k, sealed, []byte("index:"+a.Header.AccountID))
	if err != nil {
		return idx, ErrWrongSecret
	}
	return idx, json.Unmarshal(pt, &idx)
}

// MergeIndex merges a remote index into the local one. Profiles are
// independent entries, so the merge is a union by profile ID; for a profile
// present in both, the entry from the more recently updated index wins.
// It reports whether the local index changed.
func (a *Account) MergeIndex(remote Index) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	local := a.Index
	byID := map[string]ProfileEntry{}
	order := []string{}
	add := func(p ProfileEntry, prefer bool) {
		if _, ok := byID[p.ID]; !ok {
			order = append(order, p.ID)
			byID[p.ID] = p
		} else if prefer {
			byID[p.ID] = p
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
		merged = append(merged, byID[id])
	}
	sort.SliceStable(merged, func(i, j int) bool { return merged[i].Created.Before(merged[j].Created) })
	if len(merged) > MaxProfiles {
		merged = merged[:MaxProfiles]
	}
	name := local.Name
	if remoteNewer && remote.Name != "" {
		name = remote.Name
	}
	before, _ := json.Marshal(local.Profiles)
	after, _ := json.Marshal(merged)
	if string(before) == string(after) && name == local.Name {
		return false, nil
	}
	a.Index.Profiles = merged
	a.Index.Name = name
	return true, a.saveLocked()
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
	removed := a.Index.Profiles[idx]
	a.Index.Profiles = append(a.Index.Profiles[:idx], a.Index.Profiles[idx+1:]...)
	if err := a.saveLocked(); err != nil {
		a.Index.Profiles = append(a.Index.Profiles, removed)
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
