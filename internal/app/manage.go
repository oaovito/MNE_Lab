package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/oaovito/mne_lab/internal/backup"
	"github.com/oaovito/mne_lab/internal/i18n"
	"github.com/oaovito/mne_lab/internal/media"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/syncer"
	"github.com/oaovito/mne_lab/internal/vault"
)

// ProfileUpdate edits the open profile's identity.
type ProfileUpdate struct {
	Username *string `json:"username"`
	Color    *int    `json:"color"`
}

// UpdateProfile renames or recolors a profile of the unlocked account.
func (a *App) UpdateProfile(id string, u ProfileUpdate) (ProfileView, error) {
	acct, err := a.account()
	if err != nil {
		return ProfileView{}, err
	}
	e, err := acct.UpdateProfile(id, func(pe *vault.ProfileEntry) error {
		if u.Username != nil {
			name, err := vault.ValidateUsername(*u.Username)
			if err != nil {
				return err
			}
			pe.Username = name
		}
		if u.Color != nil && *u.Color >= 0 && *u.Color < 16 {
			pe.Color = *u.Color
		}
		return nil
	})
	if err != nil {
		return ProfileView{}, err
	}
	a.refreshOpenEntry(e)
	return ProfileView{ProfileEntry: e, Avatar: e.AvatarHash != ""}, nil
}

func (a *App) refreshOpenEntry(e vault.ProfileEntry) {
	a.mu.Lock()
	if a.prof != nil && a.prof.Entry.ID == e.ID {
		a.prof.Entry = e
	}
	a.mu.Unlock()
	a.hub.Publish("state", nil)
}

// SetAvatar validates and stores a profile photo (PNG, JPEG, WebP or GIF,
// up to 5 MB); an empty body removes it.
func (a *App) SetAvatar(id string, data []byte) (ProfileView, error) {
	acct, err := a.account()
	if err != nil {
		return ProfileView{}, err
	}
	var norm []byte
	var mime string
	if len(data) > 0 {
		if norm, mime, err = media.NormalizeAvatar(data); err != nil {
			return ProfileView{}, err
		}
	}
	if err := acct.SetAvatar(id, norm, mime); err != nil {
		return ProfileView{}, err
	}
	e, err := acct.Profile(id)
	if err != nil {
		return ProfileView{}, err
	}
	a.refreshOpenEntry(e)
	if p, err := a.Profile(); err == nil {
		p.Kick() // the avatar travels with the account to the cloud
	}
	return ProfileView{ProfileEntry: e, Avatar: e.AvatarHash != ""}, nil
}

// Avatar returns a profile photo and its type.
func (a *App) Avatar(id string) ([]byte, string, error) {
	acct, err := a.account()
	if err != nil {
		return nil, "", err
	}
	e, err := acct.Profile(id)
	if err != nil {
		return nil, "", err
	}
	if e.AvatarHash == "" {
		return nil, "", os.ErrNotExist
	}
	b, err := acct.Avatar(id)
	return b, e.AvatarType, err
}

// SetProfilePassword sets, changes or removes (empty) the open profile's
// password.
func (a *App) SetProfilePassword(current, next string) error {
	p, err := a.Profile()
	if err != nil {
		return err
	}
	if p.Entry.HasPassword {
		k, err := p.acct.UnlockProfile(p.Entry.ID, current)
		if err != nil {
			return err
		}
		k.Wipe()
	}
	if err := p.acct.SetProfilePassword(p.Entry.ID, p.key, next); err != nil {
		return err
	}
	if e, err := p.acct.Profile(p.Entry.ID); err == nil {
		a.refreshOpenEntry(e)
	}
	p.Kick()
	return nil
}

// RecoverProfile opens a profile whose password was forgotten, with the
// account recovery key, and sets a new password (empty: none).
func (a *App) RecoverProfile(id, recoveryKey, newPassword string) (*Profile, error) {
	acct, err := a.account()
	if err != nil {
		return nil, err
	}
	rk, err := secure.ParseRecoveryKey(recoveryKey)
	if err != nil {
		return nil, vault.ErrWrongSecret
	}
	defer rk.Wipe()
	key, err := acct.RecoverProfile(id, rk)
	if err != nil {
		return nil, err
	}
	if err := acct.SetProfilePassword(id, key, newPassword); err != nil {
		key.Wipe()
		return nil, err
	}
	e, err := acct.Profile(id)
	if err != nil {
		key.Wipe()
		return nil, err
	}
	a.closeSession(true)
	p, err := a.openProfile(acct, e, key)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.prof = p
	a.mu.Unlock()
	a.hub.Publish("state", nil)
	a.updateTray()
	return p, nil
}

// DeleteProfile removes a profile from the account after confirming its
// password. Its local data on this computer is removed; the account index
// (and with it the profile list on other computers) follows on sync.
func (a *App) DeleteProfile(id, password string) error {
	acct, err := a.account()
	if err != nil {
		return err
	}
	k, err := acct.UnlockProfile(id, password)
	if err != nil {
		return err
	}
	k.Wipe()
	if p, err := a.Profile(); err == nil && p.Entry.ID == id {
		a.closeSession(true)
	}
	if err := acct.RemoveProfile(id); err != nil {
		return err
	}
	os.RemoveAll(acct.ProfileDir(id))
	a.hub.Publish("state", nil)
	return nil
}

// ChangePassphrase changes the account passphrase.
func (a *App) ChangePassphrase(current, next string) error {
	acct, err := a.account()
	if err != nil {
		return err
	}
	check, err := a.Vault.Unlock(acct.ID(), current)
	if err != nil {
		return err
	}
	check.Lock()
	return acct.ChangePassphrase(next)
}

// SetRemember keeps the account unlockable on this drive without the
// passphrase (Portable USB Mode only).
func (a *App) SetRemember(on bool) error {
	acct, err := a.account()
	if err != nil {
		return err
	}
	if !on {
		return acct.Forget()
	}
	if a.L.Mode != paths.Portable {
		return errors.New("account.remember_portable_only")
	}
	return acct.Remember()
}

// Backups lists the open profile's backups, newest first.
func (a *App) Backups() ([]backup.Entry, error) {
	p, err := a.Profile()
	if err != nil {
		return nil, err
	}
	m, err := backup.Load(p.backupDir())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	out := append([]backup.Entry(nil), m.Entries...)
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

// RestoreBackup replaces the open profile's local data with a verified
// backup. A backup of the current state is made first, so nothing is lost.
// For cloud profiles, synchronization then reconciles with the cloud.
func (a *App) RestoreBackup(file string) error {
	p, err := a.Profile()
	if err != nil {
		return err
	}
	m, err := backup.Load(p.backupDir())
	if err != nil {
		return err
	}
	var target *backup.Entry
	for i := range m.Entries {
		if m.Entries[i].File == file {
			target = &m.Entries[i]
		}
	}
	if target == nil {
		return errors.New("backup.not_found")
	}
	key := p.key // copy: closing the profile wipes its key
	if err := backup.Verify(p.backupDir(), *target, key); err != nil {
		return err
	}
	if _, err := p.Backup("before-restore"); err != nil {
		return err
	}
	acct, e := p.acct, p.Entry
	a.mobile.Stop()
	a.mu.Lock()
	a.prof = nil
	a.mu.Unlock()
	p.close()
	rerr := backup.Restore(p.backupDir(), *target, key, p.dbPath())
	np, err := a.openProfile(acct, e, key)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.prof = np
	a.mu.Unlock()
	a.hub.Publish("state", nil)
	a.hub.Publish("library", nil)
	return rerr
}

// Conflicts lists unresolved synchronization conflicts.
func (a *App) Conflicts() ([]syncer.Conflict, error) {
	p, err := a.Profile()
	if err != nil {
		return nil, err
	}
	c, err := syncer.Conflicts(p.St)
	if c == nil {
		c = []syncer.Conflict{}
	}
	return c, err
}

// ResolveConflict settles one conflict (mine, theirs or both).
func (a *App) ResolveConflict(ctx context.Context, coll, id, choice string) error {
	p, err := a.Profile()
	if err != nil {
		return err
	}
	p.mu.Lock()
	eng := p.eng
	p.mu.Unlock()
	if eng == nil {
		return ErrConnection
	}
	switch choice {
	case syncer.KeepMine, syncer.KeepTheirs, syncer.KeepBoth:
	default:
		return errors.New("sync.invalid_choice")
	}
	dup := func(data []byte) (string, []byte) { return secure.NewID(), data }
	if err := eng.Resolve(ctx, coll, id, choice, dup); err != nil {
		return err
	}
	p.refreshStatus()
	p.Kick()
	a.hub.Publish("library", nil)
	return nil
}

// Languages lists the interface languages.
func Languages() []string { return i18n.Supported }

// DirEntry is a folder in the destination picker.
type DirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// ListDirs lists the folders inside dir (no files), or the starting points
// when dir is empty.
func (a *App) ListDirs(dir string) ([]DirEntry, error) {
	if dir == "" {
		var out []DirEntry
		for _, d := range a.ExportDestinations() {
			out = append(out, DirEntry{Name: d.ID, Path: d.Path})
		}
		if runtime.GOOS == "windows" {
			for c := 'C'; c <= 'Z'; c++ {
				r := string(c) + `:\`
				if st, err := os.Stat(r); err == nil && st.IsDir() {
					out = append(out, DirEntry{Name: r, Path: r})
				}
			}
		} else {
			out = append(out, DirEntry{Name: "/", Path: "/"})
		}
		return out, nil
	}
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		return nil, errors.New("fs.invalid_path")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.New("fs.unavailable")
	}
	out := []DirEntry{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "$") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			if a.L.Mode == paths.Temporary && paths.Within(a.L.Root, p) {
				continue
			}
			out = append(out, DirEntry{Name: e.Name(), Path: p})
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}
