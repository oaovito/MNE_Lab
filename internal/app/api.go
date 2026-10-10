package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/export"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/plot"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/module"
	"github.com/oaovito/mne_lab/internal/science/reference"
	"github.com/oaovito/mne_lab/internal/update"
	"github.com/oaovito/mne_lab/internal/vault"
	"github.com/oaovito/mne_lab/internal/version"
)

// StateView is everything the interface needs to decide what to show.
type StateView struct {
	Version      string            `json:"version"`
	Channel      string            `json:"channel"`
	BuildDate    string            `json:"buildDate,omitempty"`
	Commit       string            `json:"commit,omitempty"`
	Mode         paths.Mode        `json:"mode"`
	Lang         string            `json:"lang"`
	SystemLang   string            `json:"systemLang"`
	Languages    []string          `json:"languages"`
	Decimal      string            `json:"decimal"`
	Settings     Settings          `json:"settings"`
	Crashed      bool              `json:"crashed"`
	Exiting      bool              `json:"exiting"`
	Accounts     []vault.Summary   `json:"accounts"`
	Account      *AccountView      `json:"account,omitempty"`
	Profile      *ProfileState     `json:"profile,omitempty"`
	StorageModes []string          `json:"storageModes"`
	Providers    []ProviderOption  `json:"providers"`
	Recoveries   []Recovery        `json:"recoveries,omitempty"`
	Update       update.Status     `json:"update"`
	Mobile       MobileInfo        `json:"mobile"`
	Present      presentation      `json:"present"`
	Activities   []Activity        `json:"activities"`
	Exports      string            `json:"exports"`
	Science      map[string]string `json:"science"`
}

// ProfileState is the open profile.
type ProfileState struct {
	ProfileView
	Settings ProfileSettings `json:"settings"`
	Sync     SyncStatus      `json:"sync"`
}

func (a *App) State() StateView {
	s := StateView{Version: version.Version, Channel: version.Channel, BuildDate: version.Date, Commit: version.Commit,
		Mode: a.L.Mode, Lang: a.Lang(), SystemLang: a.sysLang, Languages: Languages(), Decimal: a.decimal(),
		Settings: a.Settings(), Crashed: a.crashed, Exiting: a.exiting.Load(), StorageModes: a.StorageModes(),
		Providers: a.ProviderOptions(), Recoveries: a.Recoveries(), Update: a.upd.Status(), Mobile: a.mobile.Info().public(),
		Present: a.Presentation(), Activities: a.Activities(), Exports: a.L.Exports,
		Science: map[string]string{"spec": reference.LightScatteringSpecID, "parser": lightscattering.Version, "graph": graph.EngineVersion}}
	s.Accounts, _ = a.Vault.List()
	if s.Accounts == nil {
		s.Accounts = []vault.Summary{}
	}
	if v, err := a.accountView(); err == nil {
		s.Account = v
	}
	if p, err := a.Profile(); err == nil {
		s.Profile = &ProfileState{ProfileView: ProfileView{ProfileEntry: p.Entry, Avatar: p.Entry.AvatarHash != ""}, Settings: p.Settings(), Sync: p.Status()}
	}
	return s
}

func pathID(r *http.Request) string { return r.PathValue("id") }

func (s *Server) routes() {
	s.statisticsRoutes()
	a := s.app
	ctxT := func(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
		return context.WithTimeout(r.Context(), d)
	}
	prof := func() (*Profile, error) { return a.Profile() }

	// ---- state and events ----
	s.handle("GET /api/state", func(w http.ResponseWriter, r *http.Request) (any, error) { return a.State(), nil })
	s.mux.HandleFunc("GET /api/events", s.events)

	// ---- application ----
	s.handle("PUT /api/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Language  *string `json:"language"`
			Theme     *string `json:"theme"`
			Onboarded *bool   `json:"onboarded"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if in.Language != nil && *in.Language != "" && !contains(Languages(), *in.Language) {
			return nil, errors.New("settings.invalid_language")
		}
		if err := a.saveSettings(func(st *Settings) {
			if in.Language != nil {
				st.Language = *in.Language
			}
			if in.Theme != nil {
				st.Theme = *in.Theme
			}
			if in.Onboarded != nil {
				st.Onboarded = *in.Onboarded
			}
		}); err != nil {
			return nil, err
		}
		a.updateTray()
		a.hub.Publish("state", nil)
		return a.Settings(), nil
	})
	s.handle("POST /api/app/turbo", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			On    bool   `json:"on"`
			Route string `json:"route"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		go func() {
			time.Sleep(150 * time.Millisecond) // let the response reach the window
			if err := a.SetTurbo(in.On, safeRoute(in.Route)); err != nil {
				a.Log.Warn("turbo restart", "err", err)
				a.hub.Publish("restart-error", map[string]string{"code": "restart.failed"})
			}
		}()
		return map[string]bool{"restarting": true}, nil
	})
	s.handle("POST /api/app/save", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		if _, err := p.Backup("manual"); err != nil {
			return nil, err
		}
		ctx, cancel := ctxT(r, 60*time.Second)
		defer cancel()
		st, _ := p.Sync(ctx)
		return st, nil
	})
	s.handle("POST /api/app/exit", func(w http.ResponseWriter, r *http.Request) (any, error) {
		go func() {
			time.Sleep(100 * time.Millisecond)
			a.Exit()
		}()
		return map[string]bool{"exiting": true}, nil
	})
	s.handle("POST /api/app/open-external", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			URL string `json:"url"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		u, err := url.Parse(in.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return nil, errors.New("request.invalid_url")
		}
		if a.opt.Shell == nil {
			return nil, errors.New("provider.no_browser")
		}
		return nil, a.opt.Shell.OpenExternal(u.String())
	})

	// ---- account ----
	s.handle("POST /api/account/create", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Name       string `json:"name"`
			Passphrase string `json:"passphrase"`
			Remember   bool   `json:"remember"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		rk, v, err := a.CreateAccount(in.Name, in.Passphrase, in.Remember)
		if err != nil {
			return nil, err
		}
		a.saveSettings(func(st *Settings) { st.LastAccount = v.ID })
		return map[string]any{"recoveryKey": rk, "account": v}, nil
	})
	s.handle("POST /api/account/unlock", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			ID         string `json:"id"`
			Passphrase string `json:"passphrase"`
			Remember   bool   `json:"remember"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		v, err := a.UnlockAccount(in.ID, in.Passphrase, in.Remember)
		if err == nil {
			a.saveSettings(func(st *Settings) { st.LastAccount = v.ID })
		}
		return v, err
	})
	s.handle("POST /api/account/remembered", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			ID string `json:"id"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return a.UnlockRemembered(in.ID)
	})
	s.handle("POST /api/account/recover", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			ID          string `json:"id"`
			RecoveryKey string `json:"recoveryKey"`
			Passphrase  string `json:"passphrase"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return a.RecoverAccount(in.ID, in.RecoveryKey, in.Passphrase)
	})
	s.handle("POST /api/account/change", func(w http.ResponseWriter, r *http.Request) (any, error) {
		ctx, cancel := ctxT(r, 60*time.Second)
		defer cancel()
		return nil, a.ChangeAccount(ctx)
	})
	s.handle("POST /api/account/passphrase", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Current string `json:"current"`
			Next    string `json:"next"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return nil, a.ChangePassphrase(in.Current, in.Next)
	})
	s.handle("POST /api/account/remember", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			On bool `json:"on"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if err := a.SetRemember(in.On); err != nil {
			return nil, err
		}
		return a.accountView()
	})

	// ---- sign in (an existing account from the cloud) ----
	s.handle("POST /api/signin/start", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Provider  string `json:"provider"`
			Transport string `json:"transport"`
			Folder    string `json:"folder"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		ctx, cancel := ctxT(r, 10*time.Minute)
		defer cancel()
		return a.StartSignIn(ctx, in.Provider, in.Transport, in.Folder)
	})
	s.handle("POST /api/signin/finish", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			ID         string `json:"id"`
			Passphrase string `json:"passphrase"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		ctx, cancel := ctxT(r, 5*time.Minute)
		defer cancel()
		v, err := a.SignIn(ctx, in.ID, in.Passphrase)
		if err == nil {
			a.saveSettings(func(st *Settings) { st.LastAccount = v.ID })
		}
		return v, err
	})

	// ---- providers ----
	s.handle("GET /api/providers", func(w http.ResponseWriter, r *http.Request) (any, error) { return a.ProviderOptions(), nil })
	s.handle("POST /api/providers/connect", func(w http.ResponseWriter, r *http.Request) (any, error) {
		// A connection for a profile being created (kept briefly in memory).
		var in struct {
			Provider  string `json:"provider"`
			Transport string `json:"transport"`
			Folder    string `json:"folder"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		ctx, cancel := ctxT(r, 10*time.Minute)
		defer cancel()
		p, err := a.Connect(ctx, in.Provider, in.Transport, in.Folder)
		if err != nil {
			return nil, err
		}
		return map[string]any{"connection": keepConn(p), "provider": p.ID(), "transport": p.Transport(), "account": p.Info()}, nil
	})
	s.handle("POST /api/profile/provider", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Provider  string `json:"provider"`
			Transport string `json:"transport"`
			Folder    string `json:"folder"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		ctx, cancel := ctxT(r, 10*time.Minute)
		defer cancel()
		return a.ConnectProfileProvider(ctx, in.Provider, in.Transport, in.Folder)
	})
	s.handle("DELETE /api/profile/provider", func(w http.ResponseWriter, r *http.Request) (any, error) {
		ctx, cancel := ctxT(r, 2*time.Minute)
		defer cancel()
		return nil, a.DisconnectProfileProvider(ctx, r.URL.Query().Get("force") == "1")
	})
	s.handle("GET /api/profile/provider", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		p.mu.Lock()
		pv := p.prov
		p.mu.Unlock()
		out := map[string]any{"status": p.Status()}
		if pv != nil {
			out["provider"], out["transport"], out["account"] = pv.ID(), pv.Transport(), pv.Info()
		}
		return out, nil
	})

	// ---- profiles ----
	s.handle("POST /api/profiles", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in ProfileInput
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return a.CreateProfile(in, nil)
	})
	s.handle("PATCH /api/profiles/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in ProfileUpdate
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return a.UpdateProfile(pathID(r), in)
	})
	s.handle("DELETE /api/profiles/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Password string `json:"password"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return nil, a.DeleteProfile(pathID(r), in.Password)
	})
	s.handle("POST /api/profiles/{id}/open", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Password string `json:"password"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if _, err := a.OpenProfile(pathID(r), in.Password); err != nil {
			return nil, err
		}
		return a.State().Profile, nil
	})
	s.handle("POST /api/profiles/{id}/recover", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			RecoveryKey string `json:"recoveryKey"`
			Password    string `json:"password"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if _, err := a.RecoverProfile(pathID(r), in.RecoveryKey, in.Password); err != nil {
			return nil, err
		}
		return a.State().Profile, nil
	})
	s.handle("GET /api/profiles/{id}/avatar", func(w http.ResponseWriter, r *http.Request) (any, error) {
		b, mime, err := a.Avatar(pathID(r))
		if err != nil {
			return nil, errors.New("profile.avatar_not_found")
		}
		w.Header().Set("Cache-Control", "private, max-age=3600")
		return rawResponse{mime: mime, data: b}, nil
	})
	s.handle("PUT /api/profiles/{id}/avatar", func(w http.ResponseWriter, r *http.Request) (any, error) {
		b, err := io.ReadAll(io.LimitReader(r.Body, 5<<20+1))
		if err != nil {
			return nil, err
		}
		return a.SetAvatar(pathID(r), b)
	})
	s.handle("DELETE /api/profiles/{id}/avatar", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return a.SetAvatar(pathID(r), nil)
	})
	s.handle("POST /api/profile/close", func(w http.ResponseWriter, r *http.Request) (any, error) { return nil, a.CloseProfile() })
	s.handle("POST /api/profile/password", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Current string `json:"current"`
			Next    string `json:"next"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return nil, a.SetProfilePassword(in.Current, in.Next)
	})
	s.handle("PUT /api/profile/settings", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		var in ProfileSettings
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if in.Language != "" && !contains(Languages(), in.Language) {
			return nil, errors.New("settings.invalid_language")
		}
		if in.Decimal != "" && in.Decimal != "." && in.Decimal != "," {
			return nil, errors.New("settings.invalid_decimal")
		}
		st, err := p.UpdateSettings(func(ps *ProfileSettings) {
			in.Export = ps.Export // export choices are remembered by the export itself
			*ps = in
		})
		a.updateTray()
		a.hub.Publish("state", nil)
		return st, err
	})

	// ---- synchronization, backups, recovery ----
	s.handle("POST /api/sync", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		ctx, cancel := ctxT(r, 2*time.Minute)
		defer cancel()
		st, _ := p.Sync(ctx)
		return st, nil
	})
	s.handle("GET /api/sync/conflicts", func(w http.ResponseWriter, r *http.Request) (any, error) { return a.Conflicts() })
	s.handle("POST /api/sync/conflicts/resolve", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Collection string `json:"collection"`
			ID         string `json:"id"`
			Choice     string `json:"choice"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		ctx, cancel := ctxT(r, time.Minute)
		defer cancel()
		return nil, a.ResolveConflict(ctx, in.Collection, in.ID, in.Choice)
	})
	s.handle("GET /api/backups", func(w http.ResponseWriter, r *http.Request) (any, error) { return a.Backups() })
	s.handle("POST /api/backups", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		return p.Backup("manual")
	})
	s.handle("POST /api/backups/restore", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			File string `json:"file"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return nil, a.RestoreBackup(in.File)
	})
	s.handle("GET /api/recoveries", func(w http.ResponseWriter, r *http.Request) (any, error) { return a.Recoveries(), nil })
	s.handle("POST /api/recoveries/restore", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Account string `json:"account"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return nil, a.RestoreRecovery(in.Account)
	})
	s.handle("POST /api/recoveries/discard", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Account string `json:"account"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		a.DiscardStaleSessions(in.Account)
		return nil, nil
	})

	// ---- LIGHTSCATTERING: files and measurements ----
	// Saved import selections live only in the active Profile's encrypted store.
	importProfileScope := func(r *http.Request) (*Profile, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		if p.closed.Load() || r.Header.Get("X-Account-ID") != p.acct.ID() || r.Header.Get("X-Profile-ID") != p.Entry.ID {
			return nil, ErrImportScope
		}
		return p, nil
	}
	s.handle("GET /api/import/profiles", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := importProfileScope(r)
		if err != nil {
			return nil, err
		}
		return p.ImportProfiles()
	})
	s.handle("POST /api/import/profiles", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := importProfileScope(r)
		if err != nil {
			return nil, err
		}
		var in struct {
			Name    string `json:"name"`
			Receipt string `json:"receipt"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		recipe, err := p.SaveImportProfile(in.Name, in.Receipt)
		if err == nil {
			a.hub.Publish("library", nil)
		}
		return recipe, err
	})
	s.handle("DELETE /api/import/profiles/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := importProfileScope(r)
		if err != nil {
			return nil, err
		}
		err = p.DeleteImportProfile(pathID(r))
		if err == nil {
			a.hub.Publish("library", nil)
		}
		return nil, err
	})
	// Inspection and confirmation are explicitly bound to the chooser's scope.
	for _, action := range []string{"inspect", "confirm"} {
		s.handle("POST /api/import/"+action, func(w http.ResponseWriter, r *http.Request) (any, error) {
			p, err := prof()
			if err != nil {
				return nil, err
			}
			if r.Header.Get("X-Account-ID") != p.acct.ID() || r.Header.Get("X-Profile-ID") != p.Entry.ID {
				return nil, ErrImportScope
			}
			name, err := url.QueryUnescape(r.Header.Get("X-File-Name"))
			if err != nil || name == "" {
				return nil, errors.New("ls.name_required")
			}
			data, err := io.ReadAll(io.LimitReader(r.Body, lightscattering.MaxFileSize+1))
			if err != nil {
				return nil, err
			}
			var selection *model.ImportSelection
			rawSelection := r.Header.Get("X-Import-Selection")
			if len(rawSelection) > 32768 {
				return nil, lightscattering.ErrImportSelection
			}
			if rawSelection != "" {
				decoded, err := url.QueryUnescape(rawSelection)
				if err != nil {
					return nil, lightscattering.ErrImportSelection
				}
				decoder := json.NewDecoder(strings.NewReader(decoded))
				decoder.DisallowUnknownFields()
				if decoder.Decode(&selection) != nil || selection == nil || decoder.Decode(new(any)) != io.EOF {
					return nil, lightscattering.ErrImportSelection
				}
			}
			if action == "inspect" {
				return p.InspectImportSelection(name, data, selection)
			}
			result, err := p.ConfirmImportSelection(name, data, r.Header.Get("X-Import-Receipt"), r.URL.Query().Get("force") == "1", selection)
			if err == nil && result.FileID != "" {
				a.hub.Publish("library", nil)
			}
			return result, err
		})
	}
	s.handle("POST /api/files", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		name, _ := url.QueryUnescape(r.Header.Get("X-File-Name"))
		if name == "" {
			return nil, errors.New("ls.name_required")
		}
		b, err := io.ReadAll(io.LimitReader(r.Body, lightscattering.MaxFileSize+1))
		if err != nil {
			return nil, err
		}
		res := p.Import(name, b, r.URL.Query().Get("force") == "1")
		a.hub.Publish("library", nil)
		return res, nil
	})
	s.handle("GET /api/files", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		return p.Files(r.URL.Query().Get("trash") == "1")
	})
	s.handle("GET /api/files/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		f, ms, err := p.File(pathID(r))
		if err != nil {
			return nil, err
		}
		return map[string]any{"file": f, "measurements": ms}, nil
	})
	s.handle("GET /api/files/{id}/original", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		f, b, err := p.Original(pathID(r))
		if err != nil {
			return nil, err
		}
		if r.URL.Query().Get("download") != "1" {
			return rawResponse{mime: "text/plain; charset=utf-8", data: b}, nil
		}
		return rawResponse{mime: "application/octet-stream", name: f.Name, data: b}, nil
	})
	s.handle("PATCH /api/files/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		var in FilePatch
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		defer a.hub.Publish("library", nil)
		return p.UpdateFile(pathID(r), in)
	})
	s.handle("POST /api/files/{id}/trash", trashRoute(a, func(p *Profile, id string, t bool) error { return p.TrashFile(id, t) }))
	s.handle("DELETE /api/files/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		defer a.hub.Publish("library", nil)
		return nil, p.DeleteFile(pathID(r))
	})
	s.handle("PATCH /api/measurements/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		var in MeasurementPatch
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		defer a.hub.Publish("library", nil)
		return p.UpdateMeasurement(pathID(r), in)
	})

	s.handle("GET /api/relations", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		return p.Relations()
	})
	s.handle("GET /api/science/capabilities", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if _, err := prof(); err != nil {
			return nil, err
		}
		return module.Capabilities(), nil
	})

	// ---- graphs ----
	s.handle("GET /api/graphs", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		return p.Graphs(r.URL.Query().Get("trash") == "1")
	})
	s.handle("POST /api/graphs", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		var def graph.Definition
		if err := decode(r, &def); err != nil {
			return nil, err
		}
		defer a.hub.Publish("library", nil)
		return p.SaveGraph(def)
	})
	s.handle("GET /api/graphs/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		return p.Graph(pathID(r))
	})
	s.handle("POST /api/graphs/{id}/trash", trashRoute(a, func(p *Profile, id string, t bool) error { return p.TrashGraph(id, t) }))
	s.handle("DELETE /api/graphs/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		defer a.hub.Publish("library", nil)
		return nil, p.DeleteGraph(pathID(r))
	})
	// Render lays out an unsaved or saved definition for the screen; the
	// interface draws exactly this display list (the same one exports use).
	s.handle("POST /api/graphs/render", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		var in struct {
			Definition graph.Definition `json:"definition"`
			Width      float64          `json:"width"`
			Height     float64          `json:"height"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		spec := screenSpec(in.Width, in.Height, in.Definition.Visual)
		fig, res, err := p.Render(in.Definition, spec)
		if err != nil {
			return nil, err
		}
		return map[string]any{"figure": fig, "warnings": res.Warnings, "series": res.Series, "provenance": res.Provenance,
			"x": res.X, "y": res.Y, "gaps": res.Gaps, "weightings": res.Weightings}, nil
	})
	s.handle("GET /api/graphs/{id}/thumb.svg", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		def, err := p.Graph(pathID(r))
		if err != nil {
			return nil, err
		}
		v := def.Visual
		v.Legend, v.Metadata = false, false
		spec := screenSpec(480, 300, v)
		spec.Title = false
		fig, _, err := p.Render(def, spec)
		if err != nil {
			return nil, err
		}
		return rawResponse{mime: "image/svg+xml", data: plot.SVG(fig, spec)}, nil
	})

	// ---- cycles ----
	s.handle("GET /api/cycles", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		return p.Cycles(r.URL.Query().Get("trash") == "1")
	})
	s.handle("POST /api/cycles", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		var doc CycleDoc
		if err := decode(r, &doc); err != nil {
			return nil, err
		}
		defer a.hub.Publish("library", nil)
		return p.SaveCycle(doc)
	})
	s.handle("POST /api/cycles/preview", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		var doc CycleDoc
		if err := decode(r, &doc); err != nil {
			return nil, err
		}
		return p.PreviewCycle(doc)
	})
	s.handle("GET /api/cycles/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		return p.CycleDetail(pathID(r))
	})
	s.handle("POST /api/cycles/{id}/assign", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		var in struct {
			Measurement string `json:"measurement"`
			Point       int    `json:"point"`
			Replicate   int    `json:"replicate"`
			Clear       bool   `json:"clear"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		defer a.hub.Publish("library", nil)
		if in.Clear {
			return p.ClearAssignment(pathID(r), in.Measurement)
		}
		return p.Assign(pathID(r), in.Measurement, in.Point, in.Replicate)
	})
	s.handle("POST /api/cycles/{id}/trash", trashRoute(a, func(p *Profile, id string, t bool) error { return p.TrashCycle(id, t) }))
	s.handle("DELETE /api/cycles/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		defer a.hub.Publish("library", nil)
		return nil, p.DeleteCycle(pathID(r))
	})

	// ---- Smart Export ----
	s.handle("GET /api/export/options", func(w http.ResponseWriter, r *http.Request) (any, error) {
		kind := r.URL.Query().Get("kind")
		var prefs ExportPrefs
		if p, err := prof(); err == nil {
			prefs = p.Settings().Export
		}
		opts := export.For(kind)
		metadataOnly := false
		if kind == export.KindFile && r.URL.Query().Get("file") != "" {
			p, err := prof()
			if err != nil {
				return nil, err
			}
			f, _, err := p.File(r.URL.Query().Get("file"))
			if err != nil {
				return nil, err
			}
			metadataOnly = f.SourceInfo != nil && len(f.Measurements) == 0
			if metadataOnly {
				opts.Sections = []export.Section{{ID: "original", Formats: []string{"original"}}, {ID: "all", Formats: []string{"original", "json"}}}
			}
		}
		defaults := map[string]export.Choice{}
		for _, ps := range opts.Presets {
			defaults[ps] = export.SmartDefault(opts.Kind, ps)
		}
		return map[string]any{"options": opts, "defaults": defaults, "formats": export.Formats, "destinations": a.ExportDestinations(), "prefs": prefs, "metadataOnly": metadataOnly,
			"presets": map[string]plot.Spec{"screen": plot.Preset(plot.PresetScreen), "presentation": plot.Preset(plot.PresetPresentation),
				"print": plot.Preset(plot.PresetPrint), "publication": plot.Preset(plot.PresetPublication)}}, nil
	})
	s.handle("POST /api/export/preview", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req ExportRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		fig, est, err := a.Preview(req)
		if err != nil {
			return nil, err
		}
		return map[string]any{"figure": fig, "estimate": est}, nil
	})
	s.handle("POST /api/export", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var req ExportRequest
		if err := decode(r, &req); err != nil {
			return nil, err
		}
		return a.Export(req)
	})
	s.handle("GET /api/export/history", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := prof()
		if err != nil {
			return nil, err
		}
		return p.ExportHistory()
	})
	s.handle("POST /api/export/reveal", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Path string `json:"path"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if a.opt.Shell == nil {
			return nil, errors.New("provider.no_browser")
		}
		st, err := os.Stat(in.Path)
		if err != nil {
			return nil, export.ErrDestination
		}
		dir := in.Path
		if !st.IsDir() {
			dir = dirOf(in.Path)
		}
		return nil, a.opt.Shell.OpenExternal(fileURL(dir))
	})
	s.handle("GET /api/fs/dirs", func(w http.ResponseWriter, r *http.Request) (any, error) {
		return a.ListDirs(r.URL.Query().Get("path"))
	})

	// ---- mobile ----
	s.handle("GET /api/mobile", func(w http.ResponseWriter, r *http.Request) (any, error) { return a.mobile.Info(), nil })
	s.handle("POST /api/mobile/start", func(w http.ResponseWriter, r *http.Request) (any, error) { return a.mobile.Start() })
	s.handle("POST /api/mobile/stop", func(w http.ResponseWriter, r *http.Request) (any, error) {
		a.mobile.Stop()
		return a.mobile.Info(), nil
	})
	s.handle("DELETE /api/mobile/devices/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		a.mobile.Revoke(pathID(r))
		return a.mobile.Info(), nil
	})
	s.handle("POST /api/present", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Action string   `json:"action"`
			Graphs []string `json:"graphs"`
			Index  int      `json:"index"`
			Series string   `json:"series"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return a.Present(in.Action, in.Graphs, in.Index, in.Series)
	})

	// ---- updates and builds ----
	s.handle("GET /api/update", func(w http.ResponseWriter, r *http.Request) (any, error) { return a.upd.Status(), nil })
	s.handle("POST /api/update/check", func(w http.ResponseWriter, r *http.Request) (any, error) {
		ctx, cancel := ctxT(r, 10*time.Minute)
		defer cancel()
		return a.upd.Check(ctx), nil
	})
	s.handle("POST /api/update/locked", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			On bool `json:"on"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		ctx, cancel := ctxT(r, 10*time.Minute)
		defer cancel()
		if err := a.upd.SetLocked(ctx, in.On); err != nil {
			return nil, err
		}
		return a.upd.Status(), nil
	})
	s.handle("POST /api/update/choose", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			Version string `json:"version"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		ctx, cancel := ctxT(r, 15*time.Minute)
		defer cancel()
		return nil, a.upd.Choose(ctx, in.Version)
	})
}

func trashRoute(a *App, fn func(p *Profile, id string, trashed bool) error) handler {
	return func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := a.Profile()
		if err != nil {
			return nil, err
		}
		var in struct {
			Trashed bool `json:"trashed"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		defer a.hub.Publish("library", nil)
		return nil, fn(p, pathID(r), in.Trashed)
	}
}

// screenSpec is the on-screen figure for a viewport (CSS pixels).
func screenSpec(w, h float64, v graph.Visual) plot.Spec {
	s := plot.Preset(plot.PresetScreen)
	if w >= 200 && h >= 150 && w <= 8000 && h <= 8000 {
		s.Width, s.Height, s.DPI = w, h, 72
	}
	return WithVisual(s, v)
}

func safeRoute(r string) string {
	if !strings.HasPrefix(r, "/") || strings.HasPrefix(r, "//") || strings.ContainsAny(r, "\\\r\n") {
		return "/"
	}
	return r
}

func dirOf(p string) string {
	i := strings.LastIndexAny(p, `/\`)
	if i <= 0 {
		return p
	}
	return p[:i]
}

func fileURL(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// events streams application events to the window (Server-Sent Events).
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch, cancel := s.app.hub.Subscribe()
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": ok\n\n")
	fl.Flush()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case b, ok := <-ch:
			if !ok {
				fmt.Fprint(w, "data: {\"type\":\"closed\"}\n\n")
				fl.Flush()
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
	}
}
