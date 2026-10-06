package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
	"github.com/oaovito/mne_lab/internal/vault"
	"github.com/oaovito/mne_lab/internal/version"
)

// ExitStep is reported to the interface while exiting.
type ExitStep struct {
	Step   string `json:"step"`  // save, sync, verify, mobile, clean, recovery, done
	State  string `json:"state"` // running, done, skipped, warning
	Detail string `json:"detail,omitempty"`
}

// ExitReport summarizes an exit.
type ExitReport struct {
	Verified bool       `json:"verified"`
	Recovery bool       `json:"recovery"` // unsynchronized work kept as encrypted recovery
	Cleaned  bool       `json:"cleaned"`
	Steps    []ExitStep `json:"steps"`
}

func (a *App) step(r *ExitReport, step, state, detail string) {
	s := ExitStep{step, state, detail}
	r.Steps = append(r.Steps, s)
	a.hub.Publish("exit", s)
}

// waitIdle waits for critical operations to finish.
func (a *App) waitIdle(max time.Duration) {
	deadline := time.Now().Add(max)
	for a.isBusy() && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
}

// Exit runs the safe exit of the current mode. Temporary Machine Mode:
// Save → Sync → Verify → Clean → Exit. Portable USB Mode: Save → Sync →
// Exit, keeping the drive's data. Nothing is killed abruptly and the only
// copy of any data is never deleted.
func (a *App) Exit() ExitReport {
	var r ExitReport
	if !a.exiting.CompareAndSwap(false, true) {
		return r
	}
	a.Log.Info("exit requested")
	a.waitIdle(30 * time.Second)
	a.mu.Lock()
	p, acct := a.prof, a.acct
	a.mu.Unlock()

	a.step(&r, "save", "running", "")
	if p != nil && a.L.Mode == paths.Portable {
		if _, err := p.Backup("exit"); err != nil {
			a.step(&r, "save", "warning", "backup.failed")
		}
	}
	a.step(&r, "save", "done", "")

	r.Verified = true
	if p != nil && p.cloudBacked() {
		a.step(&r, "sync", "running", "")
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		p.Sync(ctx)
		cancel()
		r.Verified = p.Verified()
		state := "done"
		if !r.Verified {
			state = "warning"
		}
		a.step(&r, "sync", state, p.Status().State)
		a.step(&r, "verify", state, "")
	}

	a.step(&r, "mobile", "running", "")
	a.mobile.Stop()
	a.step(&r, "mobile", "done", "")

	var cloudOnlyClean []string
	if p != nil {
		if a.L.Mode == paths.Portable && p.Entry.StorageMode == vault.StorageCloudOnly && r.Verified {
			cloudOnlyClean = append(cloudOnlyClean, p.dir)
		}
		a.closeSession(false)
	}
	if acct != nil && a.L.Mode == paths.Temporary {
		pending := a.pendingProfiles(acct)
		if len(pending) > 0 {
			r.Verified = false
		}
	}

	switch a.L.Mode {
	case paths.Temporary:
		a.step(&r, "clean", "running", "")
		if acct != nil && !r.Verified {
			// The only copy of some work: keep it encrypted for the next run.
			if err := a.saveRecovery(acct); err != nil {
				a.Log.Error("recovery package failed; session kept", "err", err)
				a.step(&r, "recovery", "warning", "recovery.failed")
				a.finishExit(&r, false)
				return r
			}
			r.Recovery = true
			a.step(&r, "recovery", "done", "")
		} else if acct != nil {
			a.dropRecovery(acct.ID())
		}
		if acct != nil {
			acct.Lock()
		}
		a.finishExit(&r, true)
	default:
		for _, d := range cloudOnlyClean {
			// Cloud Only: the cloud is the persistent destination; the local
			// copy is removed once everything is confirmed there.
			os.Remove(filepath.Join(d, "profile.db"))
			os.RemoveAll(filepath.Join(d, "backups"))
		}
		if acct != nil {
			acct.Lock()
		}
		a.finishExit(&r, false)
	}
	return r
}

func (a *App) finishExit(r *ExitReport, clean bool) {
	if a.opt.Shell != nil {
		a.opt.Shell.CloseWindow()
	}
	if a.srv != nil {
		a.srv.Close()
	}
	a.markClean()
	a.releaseInstance()
	if clean {
		a.Log.Info("temporary session cleaned")
		if a.logFile != nil {
			a.logFile.Close()
		}
		r.Cleaned = removeSession(a.L.Root, a.opt.Exe)
		a.step(r, "clean", "done", "")
	}
	a.step(r, "done", "done", "")
	if a.opt.Shell != nil {
		a.opt.Shell.QuitTray()
	}
	a.finish()
}

// removeSession deletes the Temporary Mode session folder: data, caches,
// window profile, logs and downloaded builds. Only MNE Lab's own folder is
// touched. If the running executable lives inside it, deletion completes
// right after the process ends.
func removeSession(root, exe string) bool {
	if !strings.HasPrefix(filepath.Base(root), paths.SessionPrefix) {
		return false
	}
	if exe != "" && paths.Within(root, exe) {
		deleteAfterExit(root)
		return true
	}
	for i := 0; i < 10; i++ {
		if err := os.RemoveAll(root); err == nil {
			return true
		}
		time.Sleep(300 * time.Millisecond) // the window process may still hold files
	}
	deleteAfterExit(root)
	return true
}

func (a *App) pendingProfiles(acct *vault.Account) []string {
	var out []string
	for _, e := range acct.Profiles() {
		db := filepath.Join(acct.ProfileDir(e.ID), "profile.db")
		if _, err := os.Stat(db); err != nil {
			continue
		}
		if e.StorageMode == vault.StorageUSBOnly {
			continue
		}
		pending, ok := storePending(db)
		if !ok || pending > 0 {
			out = append(out, e.ID)
		}
	}
	return out
}

// storePending counts queued operations without unlocking the store: the
// queue is readable only as a count through a read-only open with the key,
// so here the conservative answer is used when the store is closed.
func storePending(db string) (int, bool) {
	marker := filepath.Join(filepath.Dir(db), "synced.json")
	var m struct {
		Pending int       `json:"pending"`
		At      time.Time `json:"at"`
	}
	if _, err := atomicfile.ReadJSON(marker, &m); err != nil {
		return 0, false
	}
	st, err := os.Stat(db)
	if err != nil || st.ModTime().After(m.At.Add(2*time.Second)) {
		return 0, false
	}
	return m.Pending, true
}

// recoveryDir is this account's recovery folder (outside the session).
func (a *App) recoveryDir(accountID string) string {
	return filepath.Join(a.L.Recovery, accountID)
}

// saveRecovery copies the account's encrypted files to the recovery
// folder: header, sealed index, wrapped keys and profile stores. Nothing
// readable is written; the passphrase is needed to open it.
func (a *App) saveRecovery(acct *vault.Account) error {
	src := acct.Dir()
	dst := a.recoveryDir(acct.ID())
	tmp := dst + ".partial"
	os.RemoveAll(tmp)
	if err := copyTree(src, tmp, func(rel string) bool {
		// Backups and caches are not needed to recover.
		return !strings.Contains(filepath.ToSlash(rel), "/backups/")
	}); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if err := atomicfile.WriteJSON(filepath.Join(tmp, "recovery.json"), map[string]any{"account": acct.ID(), "created": time.Now().UTC(), "version": version.Version}); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	os.RemoveAll(dst)
	return os.Rename(tmp, dst)
}

func (a *App) dropRecovery(accountID string) {
	os.RemoveAll(a.recoveryDir(accountID))
}

// Recovery describes unsynchronized work kept from an earlier session.
type Recovery struct {
	Account string    `json:"account"`
	Created time.Time `json:"created"`
	Source  string    `json:"source"` // recovery, interrupted
}

// Recoveries lists recovery packages and interrupted Temporary Mode
// sessions that may hold the only copy of some work.
func (a *App) Recoveries() []Recovery {
	if a.L.Mode != paths.Temporary {
		return nil
	}
	var out []Recovery
	entries, _ := os.ReadDir(a.L.Recovery)
	for _, e := range entries {
		var m struct {
			Account string    `json:"account"`
			Created time.Time `json:"created"`
		}
		if e.IsDir() && !strings.HasSuffix(e.Name(), ".partial") {
			if _, err := atomicfile.ReadJSON(filepath.Join(a.L.Recovery, e.Name(), "recovery.json"), &m); err == nil {
				out = append(out, Recovery{Account: m.Account, Created: m.Created, Source: "recovery"})
			}
		}
	}
	for _, s := range a.staleSessions() {
		accts, _ := os.ReadDir(filepath.Join(s, "data", "accounts"))
		for _, e := range accts {
			if e.IsDir() {
				st, _ := os.Stat(filepath.Join(s, "data", "accounts", e.Name()))
				out = append(out, Recovery{Account: e.Name(), Created: st.ModTime(), Source: "interrupted"})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// staleSessions are earlier session folders of this user whose process is
// gone (crash, power loss, killed).
func (a *App) staleSessions() []string {
	base := filepath.Dir(a.L.Root)
	var out []string
	for _, s := range paths.StaleSessions(base, a.L.Root) {
		if instanceHeld(filepath.Join(s, "data")) {
			continue // another MNE Lab is using it right now
		}
		out = append(out, s)
	}
	return out
}

// RestoreRecovery brings recovered encrypted files into this session; the
// account is then unlocked with its passphrase and synchronized.
func (a *App) RestoreRecovery(accountID string) error {
	if a.L.Mode != paths.Temporary {
		return errors.New("recovery.not_available")
	}
	var src string
	if st, err := os.Stat(a.recoveryDir(accountID)); err == nil && st.IsDir() {
		src = a.recoveryDir(accountID)
	} else {
		for _, s := range a.staleSessions() {
			p := filepath.Join(s, "data", "accounts", accountID)
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				src = p
			}
		}
	}
	if src == "" {
		return errors.New("recovery.not_found")
	}
	dst := filepath.Join(a.Vault.Dir, accountID)
	if _, err := os.Stat(dst); err == nil {
		return nil // already present in this session
	}
	return copyTree(src, dst, func(rel string) bool { return rel != "recovery.json" })
}

// DiscardStaleSessions removes interrupted sessions that hold nothing
// unsynchronized (after their work was recovered and synchronized).
func (a *App) DiscardStaleSessions(accountID string) {
	for _, s := range a.staleSessions() {
		if _, err := os.Stat(filepath.Join(s, "data", "accounts", accountID)); err == nil {
			os.RemoveAll(s)
		}
	}
	a.dropRecovery(accountID)
}

func copyTree(src, dst string, keep func(rel string) bool) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel != "." && !keep(rel) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		if err := out.Sync(); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

// ---- restart (Turbo, update): Save → Preserve → Restart → Restore ----

type handoff struct {
	Account string `json:"account,omitempty"`
	AK      string `json:"ak,omitempty"`
	Profile string `json:"profile,omitempty"`
	PDK     string `json:"pdk,omitempty"`
	Route   string `json:"route,omitempty"`
	Mobile  bool   `json:"mobile,omitempty"`
}

func (a *App) prepareRestart(ctx context.Context) error {
	a.waitIdle(30 * time.Second)
	if p, err := a.Profile(); err == nil {
		if _, err := p.Backup("restart"); err != nil {
			return err
		}
	}
	return nil
}

// Restart restarts MNE Lab, optionally into another build, keeping the
// session: no cleanup runs and the person does not sign in again.
func (a *App) Restart(exe, route string) error {
	if !a.exiting.CompareAndSwap(false, true) {
		return ErrExiting
	}
	a.waitIdle(30 * time.Second)
	if exe == "" {
		exe = a.opt.Exe
	}
	var h handoff
	h.Route = route
	a.mu.Lock()
	acct, p := a.acct, a.prof
	a.mu.Unlock()
	if acct != nil {
		k := acct.Key()
		h.Account, h.AK = acct.ID(), secure.B64(k[:])
	}
	if p != nil {
		h.Profile, h.PDK = p.Entry.ID, secure.B64(p.key[:])
	}
	h.Mobile = a.mobile.Active()
	payload, _ := json.Marshal(h)
	h.AK, h.PDK = "", ""

	args := []string{"--handoff"}
	if a.L.Mode == paths.Temporary {
		args = append(args, "--session", a.L.Root)
	} else {
		args = append(args, "--root", a.L.Root)
		if rel, err := filepath.Rel(a.L.Builds, exe); err == nil && !strings.HasPrefix(rel, "..") {
			args = append(args, "--build", strings.Split(filepath.ToSlash(rel), "/")[0])
		}
	}
	a.mobile.Stop()
	if p != nil {
		a.mu.Lock()
		a.prof = nil
		a.mu.Unlock()
		p.close()
	}
	if a.opt.Shell != nil {
		a.opt.Shell.CloseWindow()
	}
	if a.srv != nil {
		a.srv.Close()
	}
	a.releaseInstance()
	atomicfile.WriteJSON(a.markerPath(), map[string]any{"clean": true, "restart": true, "at": time.Now().UTC()})

	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		a.Log.Error("restart failed", "err", err)
		return err
	}
	stdin.Write(payload)
	stdin.Close()
	for i := range payload {
		payload[i] = 0
	}
	a.restartTo = exe
	if a.opt.Shell != nil {
		a.opt.Shell.QuitTray()
	}
	a.finish()
	return nil
}

func (a *App) restartInto(exe string) error { return a.Restart(exe, "") }

// resume restores a session handed over by the previous process.
func (a *App) resume(r io.Reader) (string, error) {
	b, err := io.ReadAll(io.LimitReader(r, 8<<10))
	if err != nil || len(b) == 0 {
		return "", err
	}
	var h handoff
	err = json.Unmarshal(b, &h)
	for i := range b {
		b[i] = 0
	}
	if err != nil || h.Account == "" {
		return h.Route, err
	}
	akb, err := secure.UnB64(h.AK)
	if err != nil {
		return h.Route, err
	}
	ak, err := secure.KeyFromBytes(akb)
	if err != nil {
		return h.Route, err
	}
	acct, err := a.Vault.UnlockWithKey(h.Account, ak)
	if err != nil {
		return h.Route, err
	}
	a.setAccount(acct)
	if h.Profile != "" {
		pkb, err := secure.UnB64(h.PDK)
		if err != nil {
			return h.Route, err
		}
		pdk, err := secure.KeyFromBytes(pkb)
		if err != nil {
			return h.Route, err
		}
		e, err := acct.Profile(h.Profile)
		if err != nil {
			return h.Route, err
		}
		p, err := a.openProfile(acct, e, pdk)
		if err != nil {
			return h.Route, err
		}
		a.mu.Lock()
		a.prof = p
		a.mu.Unlock()
	}
	if h.Mobile {
		a.mobile.Start()
	}
	return h.Route, nil
}

// SetTurbo persists the Turbo preference and restarts to apply it.
func (a *App) SetTurbo(on bool, route string) error {
	if err := a.saveSettings(func(s *Settings) { s.Turbo = on }); err != nil {
		return err
	}
	return a.Restart("", route)
}

// writeSyncMarker records the queue size after a sync so the exit logic
// can tell whether a closed profile still holds unsynchronized work.
func (p *Profile) writeSyncMarker() {
	st, err := p.St.Stats()
	if err != nil {
		return
	}
	atomicfile.WriteJSON(filepath.Join(p.dir, "synced.json"), map[string]any{"pending": st.Pending + st.Retryable + st.Conflicts, "at": time.Now().UTC().Add(time.Second)})
}

var _ = store.StatePending
