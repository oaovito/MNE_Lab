package app

import (
	"context"
	"errors"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/oaovito/mne_lab/internal/media"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/provider"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/syncer"
	"github.com/oaovito/mne_lab/internal/vault"
)

// Errors (stable identifiers).
var (
	ErrNoAccount       = errors.New("session.locked_no_account")
	ErrNoProfile       = errors.New("profile.locked_no_profile")
	ErrStorageMode     = errors.New("profile.storage_needs_usb")
	ErrConnection      = errors.New("provider.connection_not_found")
	ErrKeysUnavailable = errors.New("profile.keys_unavailable")
)

// Pending provider connections made before a profile exists (profile
// creation) or before an account is unlocked (Sign In). They live only in
// memory and expire.
type pendingConn struct {
	prov    provider.Provider
	expires time.Time
}

type signInState struct {
	prov     provider.Provider
	accounts []syncer.RemoteAccount
}

var conns = struct {
	sync.Mutex
	m map[string]pendingConn
}{m: map[string]pendingConn{}}

func keepConn(p provider.Provider) string {
	id := secure.NewID()
	conns.Lock()
	defer conns.Unlock()
	for k, c := range conns.m {
		if time.Now().After(c.expires) {
			delete(conns.m, k)
		}
	}
	conns.m[id] = pendingConn{p, time.Now().Add(30 * time.Minute)}
	return id
}

func takeConn(id string) (provider.Provider, bool) {
	conns.Lock()
	defer conns.Unlock()
	c, ok := conns.m[id]
	delete(conns.m, id)
	if !ok || time.Now().After(c.expires) {
		return nil, false
	}
	return c.prov, true
}

// AccountView is what the interface shows about the unlocked account.
type AccountView struct {
	ID         string        `json:"id"`
	Name       string        `json:"name"`
	Remembered bool          `json:"remembered"`
	Profiles   []ProfileView `json:"profiles"`
	MaxReached bool          `json:"maxReached"`
}

// ProfileView describes a profile on the selection screen.
type ProfileView struct {
	vault.ProfileEntry
	Avatar bool `json:"avatar"`
}

func (a *App) account() (*vault.Account, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.acct == nil {
		return nil, ErrNoAccount
	}
	return a.acct, nil
}

// Profile returns the open profile.
func (a *App) Profile() (*Profile, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.prof == nil {
		return nil, ErrNoProfile
	}
	return a.prof, nil
}

func (a *App) accountView() (*AccountView, error) {
	acct, err := a.account()
	if err != nil {
		return nil, err
	}
	v := &AccountView{ID: acct.ID(), Name: acct.Index.Name, Remembered: acct.Remembered()}
	for _, e := range acct.Profiles() {
		v.Profiles = append(v.Profiles, ProfileView{ProfileEntry: e, Avatar: e.AvatarHash != ""})
	}
	v.MaxReached = len(v.Profiles) >= vault.MaxProfiles
	return v, nil
}

// CreateAccount sets up a new account. The recovery key is returned once
// and never stored in readable form.
func (a *App) CreateAccount(name, passphrase string, remember bool) (string, *AccountView, error) {
	acct, rk, err := a.Vault.Create(name, passphrase, remember && a.L.Mode == paths.Portable)
	if err != nil {
		return "", nil, err
	}
	key := rk.String()
	rk.Wipe()
	a.setAccount(acct)
	v, err := a.accountView()
	return key, v, err
}

func (a *App) setAccount(acct *vault.Account) {
	a.mu.Lock()
	a.acct = acct
	a.mu.Unlock()
	a.saveSettings(func(s *Settings) { s.LastAccount = acct.ID() })
	a.hub.Publish("state", nil)
}

// UnlockAccount opens an account with its passphrase. "Remember on this
// drive" is offered only in Portable USB Mode (offline sign-in, spec 38).
func (a *App) UnlockAccount(id, passphrase string, remember bool) (*AccountView, error) {
	acct, err := a.Vault.Unlock(id, passphrase)
	if err != nil {
		return nil, err
	}
	if remember && a.L.Mode == paths.Portable {
		acct.Remember()
	}
	a.setAccount(acct)
	return a.accountView()
}

// UnlockRemembered opens an account remembered on this drive.
func (a *App) UnlockRemembered(id string) (*AccountView, error) {
	if a.L.Mode != paths.Portable {
		return nil, vault.ErrNotRemembered
	}
	acct, err := a.Vault.UnlockRemembered(id)
	if err != nil {
		return nil, err
	}
	a.setAccount(acct)
	return a.accountView()
}

// RecoverAccount opens an account with its recovery key and sets a new
// passphrase.
func (a *App) RecoverAccount(id, recoveryKey, newPassphrase string) (*AccountView, error) {
	rk, err := secure.ParseRecoveryKey(recoveryKey)
	if err != nil {
		return nil, err
	}
	defer rk.Wipe()
	acct, err := a.Vault.UnlockWithRecovery(id, rk, newPassphrase)
	if err != nil {
		return nil, err
	}
	a.setAccount(acct)
	return a.accountView()
}

// ChangeAccount closes the profile and the account (spec 32): changes are
// saved and synchronized, keys are removed from memory, and the interface
// returns to the start screen.
func (a *App) ChangeAccount(ctx context.Context) error {
	a.closeSession(true)
	a.mu.Lock()
	acct := a.acct
	a.acct = nil
	a.signIn = nil
	a.mu.Unlock()
	if acct != nil {
		acct.Lock()
	}
	a.mobile.Stop()
	a.hub.Publish("state", nil)
	return nil
}

// closeSession closes the open profile, synchronizing first when asked.
func (a *App) closeSession(syncFirst bool) {
	a.mobile.Stop() // phones only ever see the profile that was open
	a.mu.Lock()
	p := a.prof
	a.prof = nil
	a.mu.Unlock()
	if p == nil {
		return
	}
	if syncFirst {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		p.Sync(ctx)
		cancel()
	}
	p.close()
	a.hub.Publish("state", nil)
	a.updateTray()
}

// CloseProfile returns to profile selection.
func (a *App) CloseProfile() error {
	a.closeSession(true)
	a.mobile.Stop()
	return nil
}

// ProfileInput is the profile creation form.
type ProfileInput struct {
	Username    string `json:"username"`
	StorageMode string `json:"storageMode"`
	Provider    string `json:"provider"`
	Password    string `json:"password"`
	Connection  string `json:"connection"` // pending provider connection
	Color       int    `json:"color"`
}

// StorageModes lists the storage modes valid in the current execution mode.
// Temporary Machine Mode has no USB drive, so only Cloud Only applies.
func (a *App) StorageModes() []string {
	if a.L.Mode == paths.Temporary {
		return []string{vault.StorageCloudOnly}
	}
	return []string{vault.StorageUSBCloud, vault.StorageUSBOnly, vault.StorageCloudOnly}
}

// CreateProfile creates a profile and, for cloud storage, attaches the
// provider connection made in the creation flow.
func (a *App) CreateProfile(in ProfileInput, avatar []byte) (ProfileView, error) {
	acct, err := a.account()
	if err != nil {
		return ProfileView{}, err
	}
	ok := false
	for _, m := range a.StorageModes() {
		if m == in.StorageMode {
			ok = true
		}
	}
	if !ok {
		return ProfileView{}, ErrStorageMode
	}
	var prov provider.Provider
	if in.StorageMode != vault.StorageUSBOnly && in.Connection != "" {
		p, ok := takeConn(in.Connection)
		if !ok || p.ID() != in.Provider {
			return ProfileView{}, ErrConnection
		}
		prov = p
	}
	pi := vault.ProfileInput{Username: in.Username, StorageMode: in.StorageMode, Provider: in.Provider, Password: in.Password}
	if len(avatar) > 0 {
		norm, mime, err := media.NormalizeAvatar(avatar)
		if err != nil {
			return ProfileView{}, err
		}
		pi.Avatar, pi.AvatarType = norm, mime
	}
	e, err := acct.CreateProfile(pi)
	if err != nil {
		return ProfileView{}, err
	}
	if in.Color > 0 {
		e, _ = acct.UpdateProfile(e.ID, func(pe *vault.ProfileEntry) error { pe.Color = in.Color; return nil })
	}
	if prov != nil {
		key, err := acct.UnlockProfile(e.ID, in.Password)
		if err == nil {
			p := &Profile{app: a, acct: acct, Entry: e, key: key, dir: acct.ProfileDir(e.ID)}
			os.MkdirAll(p.dir, 0o700)
			if err := p.saveCredentials(prov); err != nil {
				a.Log.Warn("could not store provider connection", "err", err)
			}
			key.Wipe()
		}
	}
	a.hub.Publish("state", nil)
	return ProfileView{ProfileEntry: e, Avatar: e.AvatarHash != ""}, nil
}

// OpenProfile unlocks a profile and makes it current.
func (a *App) OpenProfile(id, password string) (*Profile, error) {
	acct, err := a.account()
	if err != nil {
		return nil, err
	}
	e, err := acct.Profile(id)
	if err != nil {
		return nil, err
	}
	key, err := acct.UnlockProfile(id, password)
	if errors.Is(err, os.ErrNotExist) {
		// Signed in on another computer: the profile keys come from the cloud.
		key, err = a.importProfileKeys(acct, e, password)
	}
	if err != nil {
		return nil, err
	}
	a.closeSession(true)
	p, err := a.openProfile(acct, e, key)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	si := a.signIn
	a.prof = p
	a.mu.Unlock()
	if p.prov == nil && si != nil && si.prov != nil && si.prov.ID() == e.Provider && e.StorageMode != vault.StorageUSBOnly {
		if err := p.saveCredentials(si.prov); err == nil {
			p.setProvider(si.prov)
		}
	}
	a.hub.Publish("state", nil)
	a.updateTray()
	return p, nil
}

func (a *App) importProfileKeys(acct *vault.Account, e vault.ProfileEntry, password string) (secure.Key, error) {
	a.mu.Lock()
	si := a.signIn
	a.mu.Unlock()
	if si == nil || si.prov == nil || si.prov.ID() != e.Provider {
		return secure.Key{}, ErrKeysUnavailable
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	defer cancel()
	if err := syncer.ImportProfileKeys(ctx, si.prov, acct, e.ID); err != nil {
		return secure.Key{}, ErrKeysUnavailable
	}
	return acct.UnlockProfile(e.ID, password)
}

// Connect starts a provider connection: the official authorization page
// opens in the person's browser (API transport) or the desktop client
// folder is used (folder transport). The storage area is tested with a
// real write, read and delete before the connection counts as working.
func (a *App) Connect(ctx context.Context, id, transport, folder string) (provider.Provider, error) {
	var p provider.Provider
	var err error
	open := func(u string) error {
		if a.opt.Shell != nil {
			return a.opt.Shell.OpenExternal(u)
		}
		return errors.New("provider.no_browser")
	}
	switch transport {
	case provider.TransportFolder:
		if folder == "" {
			folder = provider.DetectFolders()[id]
		}
		if folder == "" {
			return nil, provider.ErrUnavailable
		}
		p = provider.NewFolder(id, folder)
	default:
		if !a.ProvCfg.APIAvailable(id) {
			return nil, provider.ErrNotConfigured
		}
		switch id {
		case provider.Google:
			p, err = provider.ConnectGoogle(ctx, a.ProvCfg, open)
		case provider.OneDrive:
			p, err = provider.ConnectOneDrive(ctx, a.ProvCfg, open)
		case provider.ICloud:
			p, err = provider.ConnectICloud(ctx, a.ProvCfg, open)
		default:
			return nil, errUnknownProvider
		}
		if err != nil {
			return nil, err
		}
	}
	if err := p.CheckConnection(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

// ProviderOption describes a provider choice on the cards.
type ProviderOption struct {
	ID         string `json:"id"`
	API        bool   `json:"api"`    // official API sign-in available in this build
	Folder     string `json:"folder"` // detected desktop client folder
	HasFolder  bool   `json:"hasFolder"`
	Configured bool   `json:"configured"` // usable at all
}

// ProviderOptions lists the three official providers in the required order.
func (a *App) ProviderOptions() []ProviderOption {
	folders := provider.DetectFolders()
	var out []ProviderOption
	for _, id := range vault.Providers {
		o := ProviderOption{ID: id, API: a.ProvCfg.APIAvailable(id), Folder: folders[id]}
		o.HasFolder = o.Folder != ""
		o.Configured = o.API || o.HasFolder
		out = append(out, o)
	}
	return out
}

// StartSignIn connects a provider and lists the MNE Lab accounts in it.
func (a *App) StartSignIn(ctx context.Context, id, transport, folder string) ([]syncer.RemoteAccount, error) {
	p, err := a.Connect(ctx, id, transport, folder)
	if err != nil {
		return nil, err
	}
	accts, err := syncer.FindAccounts(ctx, p)
	if err != nil {
		return nil, err
	}
	sort.Slice(accts, func(i, j int) bool { return accts[i].Created.After(accts[j].Created) })
	a.mu.Lock()
	a.signIn = &signInState{prov: p, accounts: accts}
	a.mu.Unlock()
	return accts, nil
}

// SignIn imports an account found in the connected provider and unlocks
// it; its profiles, settings and data then come from the cloud.
func (a *App) SignIn(ctx context.Context, id, passphrase string) (*AccountView, error) {
	a.mu.Lock()
	si := a.signIn
	a.mu.Unlock()
	if si == nil {
		return nil, ErrConnection
	}
	found := false
	for _, r := range si.accounts {
		if r.ID == id {
			found = true
		}
	}
	if !found {
		return nil, vault.ErrNoSuchAccount
	}
	if err := syncer.ImportAccount(ctx, si.prov, a.Vault, id); err != nil {
		return nil, err
	}
	v, err := a.UnlockAccount(id, passphrase, false)
	if err != nil {
		return nil, err
	}
	acct, _ := a.account()
	// Avatars and profile keys stored in this provider.
	syncer.SyncAccount(ctx, si.prov, acct, nil)
	return a.accountViewOr(v)
}

func (a *App) accountViewOr(v *AccountView) (*AccountView, error) {
	if nv, err := a.accountView(); err == nil {
		return nv, nil
	}
	return v, nil
}

// ConnectProfileProvider connects (or reconnects) the open profile's cloud.
// Switching to another provider keeps the previous one untouched until the
// new one holds a verified copy (spec 77 and 78).
func (a *App) ConnectProfileProvider(ctx context.Context, id, transport, folder string) (SyncStatus, error) {
	p, err := a.Profile()
	if err != nil {
		return SyncStatus{}, err
	}
	if !p.cloudBacked() {
		return SyncStatus{}, ErrStorageMode
	}
	prov, err := a.Connect(ctx, id, transport, folder)
	if err != nil {
		return SyncStatus{}, err
	}
	if p.Entry.Provider != id {
		// Make sure the old provider received everything first, when reachable.
		p.Sync(ctx)
	}
	if err := p.saveCredentials(prov); err != nil {
		return SyncStatus{}, err
	}
	if p.Entry.Provider != id {
		e, err := p.acct.UpdateProfile(p.Entry.ID, func(pe *vault.ProfileEntry) error { pe.Provider = id; return nil })
		if err != nil {
			return SyncStatus{}, err
		}
		p.Entry = e
	}
	p.setProvider(prov)
	st, _ := p.Sync(ctx)
	return st, nil
}

// DisconnectProfileProvider forgets the cloud connection of the profile
// after confirming nothing exists only locally (unless forced).
func (a *App) DisconnectProfileProvider(ctx context.Context, force bool) error {
	p, err := a.Profile()
	if err != nil {
		return err
	}
	p.mu.Lock()
	prov := p.prov
	p.mu.Unlock()
	if prov == nil {
		return nil
	}
	if !force {
		if st, _ := p.Sync(ctx); st.Pending > 0 {
			return errors.New("provider.pending_changes")
		}
	}
	prov.Disconnect(ctx)
	os.Remove(p.credsPath())
	p.setProvider(nil)
	p.refreshStatus()
	return nil
}
