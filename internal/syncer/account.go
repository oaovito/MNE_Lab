package syncer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/provider"
	"github.com/oaovito/mne_lab/internal/vault"
)

// Account-level remote files. Only wrapped keys, KDF parameters and sealed
// data are uploaded; the provider never receives a readable name, avatar,
// password, hash or key.
//
//	directory/<aid>.json                           account header (for Sign In discovery)
//	accounts/<aid>/index.enc                       sealed account index
//	accounts/<aid>/avatars/<pid>-<hash>.img        sealed avatars
//	accounts/<aid>/profiles/<pid>/profile.key.json wrapped profile keys

// RemoteAccount is an account found in a provider.
type RemoteAccount struct {
	ID      string    `json:"id"`
	Created time.Time `json:"created"`
}

// writeAccountFile confirms the exact bytes needed to discover and unlock
// an account before local recovery material can be safely removed.
func writeAccountFile(ctx context.Context, p provider.Provider, name string, body []byte) error {
	if _, err := p.Write(ctx, name, body); err != nil {
		return err
	}
	back, err := p.Read(ctx, name)
	if err != nil {
		return err
	}
	if !bytes.Equal(back, body) {
		return errors.New("syncer: remote account file differs after upload")
	}
	return nil
}

// FindAccounts lists MNE Lab accounts stored in a provider.
func FindAccounts(ctx context.Context, p provider.Provider) ([]RemoteAccount, error) {
	objs, err := p.List(ctx, "directory")
	if err != nil {
		return nil, err
	}
	var out []RemoteAccount
	for _, o := range objs {
		if !strings.HasSuffix(o.Name, ".json") {
			continue
		}
		b, err := p.Read(ctx, o.Name)
		if err != nil {
			continue
		}
		var h vault.Header
		if json.Unmarshal(b, &h) == nil && h.AccountID != "" {
			out = append(out, RemoteAccount{ID: h.AccountID, Created: h.Created})
		}
	}
	return out, nil
}

// ImportAccount downloads an account header and sealed index so it can be
// unlocked locally with its passphrase.
func ImportAccount(ctx context.Context, p provider.Provider, v *vault.Vault, id string) error {
	h, err := p.Read(ctx, "directory/"+id+".json")
	if err != nil {
		return err
	}
	idx, err := p.Read(ctx, "accounts/"+id+"/index.enc")
	if err != nil {
		return err
	}
	got, err := v.ImportHeader(h, idx)
	if err != nil {
		return err
	}
	if got != id {
		return errors.New("syncer: account identifier mismatch")
	}
	return nil
}

// SyncAccount reconciles account-level files with a provider. profiles
// limits which profile key files are handled (those stored in this
// provider); nil means all.
func SyncAccount(ctx context.Context, p provider.Provider, a *vault.Account, profiles map[string]bool) (changed bool, err error) {
	base := "accounts/" + a.ID()
	hdr, err := a.HeaderJSON()
	if err != nil {
		return false, err
	}
	if cur, err := p.Read(ctx, "directory/"+a.ID()+".json"); err != nil || !bytes.Equal(cur, hdr) {
		if errors.Is(err, provider.ErrOffline) {
			return false, err
		}
		if err := writeAccountFile(ctx, p, "directory/"+a.ID()+".json", hdr); err != nil {
			return false, err
		}
	}
	// Index: union merge, then publish the merged result.
	if remote, err := p.Read(ctx, base+"/index.enc"); err == nil {
		idx, err := a.OpenIndex(remote)
		if err != nil {
			return false, err
		}
		if c, err := a.MergeIndex(idx); err != nil {
			return false, err
		} else if c {
			changed = true
		}
	} else if !errors.Is(err, provider.ErrNotFound) {
		return false, err
	}
	sealed, err := a.SealedIndex()
	if err != nil {
		return changed, err
	}
	if err := writeAccountFile(ctx, p, base+"/index.enc", sealed); err != nil {
		return changed, err
	}
	// Avatar names identify their expected content; verify the bytes too.
	objs, err := p.List(ctx, base+"/avatars")
	if err != nil {
		return changed, err
	}
	remoteAv := map[string]bool{}
	for _, o := range objs {
		remoteAv[o.Name] = true
	}
	for _, pe := range a.Profiles() {
		if pe.AvatarHash == "" {
			continue
		}
		name := base + "/avatars/" + pe.ID + "-" + pe.AvatarHash + ".img"
		local, lerr := a.SealedAvatar(pe.ID)
		haveLocal := lerr == nil
		if haveLocal {
			if plain, err := a.Avatar(pe.ID); err != nil || len(plain) == 0 || !strings.HasPrefix(hashHex(plain), pe.AvatarHash) {
				haveLocal = false
			}
		}
		switch {
		case haveLocal:
			var remote []byte
			if remoteAv[name] {
				var err error
				remote, err = p.Read(ctx, name)
				if err != nil {
					return changed, err
				}
			}
			if !bytes.Equal(remote, local) {
				// The authenticated local avatar is authoritative for this
				// content-named object, even if it uses a different nonce.
				if err := writeAccountFile(ctx, p, name, local); err != nil {
					return changed, err
				}
			}
		case !haveLocal && remoteAv[name]:
			b, err := p.Read(ctx, name)
			if err != nil {
				return changed, err
			}
			if err := a.ImportSealedAvatar(pe.ID, b); err != nil {
				return changed, err
			}
			changed = true
		}
	}
	// Profile key files: the most recently updated wins; every version wraps
	// the same data key, so no data can be lost by this choice.
	for _, pe := range a.Profiles() {
		if profiles != nil && !profiles[pe.ID] {
			continue
		}
		name := base + "/profiles/" + pe.ID + "/profile.key.json"
		remote, rerr := p.Read(ctx, name)
		local, lerr := a.ProfileKeysJSON(pe.ID)
		if rerr == nil {
			installed, err := a.ImportProfileKeys(pe.ID, remote)
			if err != nil {
				return changed, err
			}
			if installed {
				changed = true
				continue
			}
		} else if !errors.Is(rerr, provider.ErrNotFound) {
			return changed, rerr
		}
		if lerr == nil && !bytes.Equal(local, remote) {
			if err := writeAccountFile(ctx, p, name, local); err != nil {
				return changed, err
			}
		}
	}
	return changed, nil
}

// ImportProfileKeys downloads a profile's key file (Sign In on a new machine).
func ImportProfileKeys(ctx context.Context, p provider.Provider, a *vault.Account, pid string) error {
	b, err := p.Read(ctx, "accounts/"+a.ID()+"/profiles/"+pid+"/profile.key.json")
	if err != nil {
		return err
	}
	_, err = a.ImportProfileKeys(pid, b)
	return err
}
