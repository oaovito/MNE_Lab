package app

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/instance"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/restartpipe"
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

// waitIdle reports whether critical operations finished before the deadline.
func (a *App) waitIdle(max time.Duration) bool {
	deadline := time.Now().Add(max)
	for a.isBusy() && time.Now().Before(deadline) {
		if a.ctx != nil {
			select {
			case <-a.ctx.Done():
				return false
			default:
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return !a.isBusy()
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
	if !a.waitIdle(30 * time.Second) {
		a.exiting.Store(false)
		a.step(&r, "save", "warning", "app.operations_running")
		return r
	}
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
		if a.L.Mode == paths.Temporary && r.Verified {
			a.DiscardStaleSessions(acct.ID())
		}
		a.closeSession(false)
	}
	if acct != nil && a.L.Mode == paths.Temporary {
		profiles := acct.Profiles()
		// A closed profile's marker is historical evidence. Its remote
		// copy cannot be checked again without reopening its data key.
		if p == nil || len(profiles) != 1 || profiles[0].ID != p.Entry.ID || len(a.pendingProfiles(acct)) > 0 {
			r.Verified = false
		}
	}

	switch a.L.Mode {
	case paths.Temporary:
		a.step(&r, "clean", "running", "")
		// Accounts closed or switched earlier still belong to this session.
		// Their keys need not be unlocked to retain their encrypted files.
		entries, err := os.ReadDir(a.Vault.Dir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			a.step(&r, "recovery", "warning", "recovery.failed")
			a.finishExit(&r, false)
			return r
		}
		currentVerified := r.Verified
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if acct != nil && entry.Name() == acct.ID() && currentVerified {
				// This account's only profile was freshly verified above.
				continue
			}
			r.Verified = false
			if err := a.saveAccountRecovery(entry.Name(), filepath.Join(a.Vault.Dir, entry.Name())); err != nil {
				a.Log.Error("recovery package failed; session kept", "err", err)
				a.step(&r, "recovery", "warning", "recovery.failed")
				a.finishExit(&r, false)
				return r
			}
			r.Recovery = true
		}
		if r.Recovery {
			a.step(&r, "recovery", "done", "")
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
	known := map[string]bool{}
	for _, e := range acct.Profiles() {
		known[e.ID] = true
		db := filepath.Join(acct.ProfileDir(e.ID), "profile.db")
		if _, err := os.Stat(db); err != nil {
			// An unopened profile can still hold the only wrapped data key.
			out = append(out, e.ID)
			continue
		}
		if e.StorageMode == vault.StorageUSBOnly {
			if a.L.Mode == paths.Temporary {
				out = append(out, e.ID)
			}
			continue
		}
		pending, ok := storePending(db)
		// The account header/index and profile key must also still match
		// the account state covered by the successful sync marker.
		if ok {
			var marker struct {
				At time.Time `json:"at"`
			}
			_, err := atomicfile.ReadJSON(filepath.Join(acct.ProfileDir(e.ID), "synced.json"), &marker)
			for _, name := range []string{filepath.Join(acct.Dir(), "account.json"), filepath.Join(acct.Dir(), "account.data"), filepath.Join(acct.ProfileDir(e.ID), "profile.key.json")} {
				st, serr := os.Stat(name)
				if err != nil || serr != nil || st.ModTime().After(marker.At) {
					ok = false
					break
				}
			}
		}
		if !ok || pending > 0 {
			out = append(out, e.ID)
		}
	}
	// Locally removed or unindexed profiles can still contain the only copy
	// of their stores and backups. They have no current remote sync proof.
	for _, directory := range []string{"profiles", "trash"} {
		entries, err := os.ReadDir(filepath.Join(acct.Dir(), directory))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			out = append(out, directory)
		}
		for _, entry := range entries {
			if directory == "trash" || !known[entry.Name()] {
				out = append(out, entry.Name())
			}
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
		Pending  int       `json:"pending"`
		At       time.Time `json:"at"`
		Verified bool      `json:"verified"`
	}
	if recovered, err := atomicfile.ReadJSON(marker, &m); err != nil || recovered || !m.Verified || m.At.IsZero() {
		return 0, false
	}
	st, err := os.Stat(db)
	if err != nil || st.ModTime().After(m.At) {
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
	return a.saveAccountRecovery(acct.ID(), acct.Dir())
}

func (a *App) saveAccountRecovery(accountID, src string) error {
	if !recoveryAccountID(accountID) {
		return errors.New("recovery.not_found")
	}
	dst := a.recoveryDir(accountID)
	if err := os.MkdirAll(a.L.Recovery, 0o700); err != nil {
		return err
	}
	// Unique staging avoids overwriting another interrupted recovery copy.
	tmp, err := os.MkdirTemp(a.L.Recovery, accountID+"-*.partial")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	digest, err := recoveryDigest(src)
	if err != nil {
		return err
	}
	if err := copyTree(src, tmp, func(rel string) bool {
		// Verified backups may be needed if the current database is damaged.
		return true
	}); err != nil {
		return err
	}
	if copied, err := recoveryDigest(tmp); err != nil {
		return err
	} else if copied != digest {
		return errors.New("recovery.copy_failed")
	}
	if current, err := recoveryDigest(src); err != nil {
		return err
	} else if current != digest {
		return errors.New("recovery.copy_failed")
	}
	if err := atomicfile.WriteJSON(filepath.Join(tmp, "recovery.json"), map[string]any{"account": accountID, "created": time.Now().UTC(), "version": version.Version}); err != nil {
		return err
	}
	if err := syncRecoveryDirectories(tmp); err != nil {
		return err
	}
	previous := dst + ".previous-" + secure.NewID()
	if _, err := os.Lstat(dst); err == nil {
		if err := os.Rename(dst, previous); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		// If publication fails, retain the previous package and make it
		// available under its original name whenever possible.
		if _, serr := os.Lstat(previous); serr == nil {
			os.Rename(previous, dst)
		}
		return err
	}
	// Earlier generations can contain work never restored in this session.
	// They remain discoverable until individually restored and verified.
	return syncRecoveryDirectory(a.L.Recovery)
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
				if st, err := os.Stat(filepath.Join(s, "data", "accounts", e.Name())); err == nil {
					out = append(out, Recovery{Account: e.Name(), Created: st.ModTime(), Source: "interrupted"})
				}
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
	if !recoveryAccountID(accountID) {
		return errors.New("recovery.not_found")
	}
	done := a.begin("recovery")
	defer done()
	var src string
	if packages := a.recoveryPackages(accountID); len(packages) > 0 {
		src = packages[0]
	} else {
		var newest time.Time
		for _, s := range a.staleSessions() {
			p := filepath.Join(s, "data", "accounts", accountID)
			if st, err := os.Stat(p); err == nil && st.IsDir() && (src == "" || st.ModTime().After(newest)) {
				src = p
				newest = st.ModTime()
			}
		}
	}
	if src == "" {
		return errors.New("recovery.not_found")
	}
	dst := filepath.Join(a.Vault.Dir, accountID)
	if _, err := os.Stat(dst); err == nil {
		return nil // already present in this session
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(a.Vault.Dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(a.Vault.Dir, ".restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	digest, err := recoveryDigest(src)
	if err != nil {
		return err
	}
	if err := copyTree(src, tmp, recoveryFile); err != nil {
		return err
	}
	// A partially copied account must never become an "already present"
	// account on a retry. Verify the complete staged copy before publishing.
	if copied, err := recoveryDigest(tmp); err != nil {
		return err
	} else if copied != digest {
		return errors.New("recovery.copy_failed")
	}
	if err := atomicfile.WriteJSON(filepath.Join(tmp, "recovery-origin.json"), recoveryOrigin{Source: src, Digest: digest}); err != nil {
		return err
	}
	if err := syncRecoveryDirectories(tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	return syncRecoveryDirectory(a.Vault.Dir)
}

// DiscardStaleSessions removes interrupted sessions that hold nothing
// unsynchronized (after their work was recovered and synchronized).
func (a *App) DiscardStaleSessions(accountID string) {
	if a.L.Mode != paths.Temporary || !recoveryAccountID(accountID) {
		return
	}
	a.mu.Lock()
	acct, p := a.acct, a.prof
	a.mu.Unlock()
	if acct == nil || acct.ID() != accountID || p == nil {
		return
	}
	profiles := acct.Profiles()
	if len(profiles) != 1 || len(a.pendingProfiles(acct)) > 0 || !p.Verified() || p.Entry.ID != profiles[0].ID {
		return
	}
	var origin recoveryOrigin
	marker := filepath.Join(acct.Dir(), "recovery-origin.json")
	if recovered, err := atomicfile.ReadJSON(marker, &origin); err != nil || recovered || origin.Digest == "" {
		return
	}
	// Only the exact source restored into the authenticated account is
	// covered by its synchronization. Other sessions or older generations
	// can contain additional unsynchronized work under the same account ID.
	allowed := false
	for _, candidate := range a.recoveryPackages(accountID) {
		allowed = allowed || candidate == origin.Source
	}
	for _, s := range a.staleSessions() {
		allowed = allowed || filepath.Join(s, "data", "accounts", accountID) == origin.Source
	}
	if !allowed {
		return
	}
	if digest, err := recoveryDigest(origin.Source); err != nil || digest != origin.Digest {
		return
	}
	// Remove only this account copy, preserving every other account and
	// its files in an interrupted session.
	if err := os.RemoveAll(origin.Source); err == nil {
		atomicfile.Remove(marker)
	}
}

type recoveryOrigin struct {
	Source string `json:"source"`
	Digest string `json:"digest"`
}

func recoveryAccountID(id string) bool {
	return id != "" && id != "." && id != ".." && filepath.Base(id) == id && !strings.ContainsAny(id, `/\\`)
}

// recoveryPackages includes retained earlier generations, newest first.
func (a *App) recoveryPackages(accountID string) []string {
	type candidate struct {
		path string
		at   time.Time
	}
	var candidates []candidate
	entries, _ := os.ReadDir(a.L.Recovery)
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasSuffix(entry.Name(), ".partial") {
			continue
		}
		path := filepath.Join(a.L.Recovery, entry.Name())
		var marker struct {
			Account string    `json:"account"`
			Created time.Time `json:"created"`
		}
		if _, err := atomicfile.ReadJSON(filepath.Join(path, "recovery.json"), &marker); err == nil && marker.Account == accountID {
			candidates = append(candidates, candidate{path, marker.Created})
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].at.After(candidates[j].at) })
	var out []string
	for _, candidate := range candidates {
		out = append(out, candidate.path)
	}
	return out
}

func recoveryFile(rel string) bool {
	return rel != "recovery.json" && rel != "recovery.json.prev" && rel != "recovery-origin.json" && rel != "recovery-origin.json.prev"
}

// recoveryDigest covers relative names and encrypted file contents, so a
// source that changed since restore cannot be discarded as verified.
func recoveryDigest(root string) (string, error) {
	h := sha256.New()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || !recoveryFile(rel) {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		name := []byte(filepath.ToSlash(rel))
		var lengths [16]byte
		binary.LittleEndian.PutUint64(lengths[:8], uint64(len(name)))
		binary.LittleEndian.PutUint64(lengths[8:], uint64(info.Size()))
		_, err = h.Write(lengths[:])
		if err == nil {
			_, err = h.Write(name)
		}
		if err == nil {
			_, err = io.Copy(h, file)
		}
		cerr := file.Close()
		if err != nil {
			return err
		}
		if cerr != nil {
			return cerr
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Files are flushed by copyTree. Flush their directory entries before a
// recovery copy authorizes removing its original session. Windows does not
// support opening directories for fsync through os.File.
func syncRecoveryDirectories(root string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	var dirs []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			dirs = append(dirs, path)
		}
		return nil
	}); err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if err := syncRecoveryDirectory(dirs[i]); err != nil {
			return err
		}
	}
	return nil
}

func syncRecoveryDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	err = dir.Sync()
	cerr := dir.Close()
	if err != nil {
		return err
	}
	return cerr
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
	if !a.waitIdle(30 * time.Second) {
		return errors.New("app.operations_running")
	}
	if p, err := a.Profile(); err == nil {
		if _, err := p.Backup("restart"); err != nil {
			return err
		}
	}
	return nil
}

// Restart restarts MNE Lab, optionally into another build, keeping the
// session: no cleanup runs and the person does not sign in again.
func (a *App) Restart(exe, route string) (result error) {
	if !a.exiting.CompareAndSwap(false, true) {
		return ErrExiting
	}
	started := false
	defer func() {
		if !started {
			a.exiting.Store(false)
			if p, err := a.Profile(); err == nil {
				p.startLoop()
			}
		}
	}()
	if exe == "" {
		exe = a.opt.Exe
	}
	exe, err := filepath.Abs(exe)
	if err != nil {
		return err
	}
	st, err := os.Stat(exe)
	if err != nil || !st.Mode().IsRegular() || (runtime.GOOS != "windows" && st.Mode().Perm()&0o111 == 0) {
		return errors.New("restart.target_unavailable")
	}
	// Snapshot and validate the target while the current window, instance
	// lock and unlocked profile still work. Preflight failures are retryable.
	if err := a.prepareRestart(context.Background()); err != nil {
		return err
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
	defer func() {
		for i := range payload {
			payload[i] = 0
		}
	}()
	h.AK, h.PDK = "", ""

	// Older builds cannot silently accept this transaction: an unsupported
	// flag or protocol fails while the original session is still running.
	args := []string{"--handoff-v1"}
	if a.L.Mode == paths.Temporary {
		args = append(args, "--session", a.L.Root)
	} else {
		args = append(args, "--root", a.L.Root)
		if rel, err := filepath.Rel(a.L.Builds, exe); err == nil && !strings.HasPrefix(rel, "..") {
			args = append(args, "--build", strings.Split(filepath.ToSlash(rel), "/")[0])
		}
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = filepath.Dir(exe)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return err
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return err
	}
	timeout := a.opt.RestartTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	timedOut := make(chan struct{})
	timer := time.AfterFunc(timeout, func() {
		stdin.Close()
		stdout.Close()
		close(timedOut)
	})
	suspended, released, timerStopped := false, false, false
	defer func() {
		if !timerStopped && !timer.Stop() {
			<-timedOut
		}
		stdin.Close()
		stdout.Close()
		if started {
			go cmd.Wait() // reap the replacement when it eventually exits
			return
		}
		// A failed child must be gone before the old store/lock reopen.
		// Give a cooperative child a bounded chance to close its staged
		// window/store after pipe EOF; then kill and reap an unresponsive one.
		waited := make(chan error, 1)
		go func() { waited <- cmd.Wait() }()
		select {
		case <-waited:
		case <-time.After(2 * time.Second):
			cmd.Process.Kill()
			<-waited
		}
		if released {
			if err := a.acquireInstance(); err != nil {
				result = errors.Join(result, err)
			} else if a.srv != nil {
				result = errors.Join(result, a.lock.Publish(instance.Info{Port: a.srv.Port(), Token: a.srv.focus, PID: os.Getpid()}))
			}
		}
		if suspended {
			result = errors.Join(result, p.resumeAfterRestart())
		}
		a.hub.Publish("state", nil)
	}()
	pipe := restartpipe.New(stdout, stdin)
	prepared, err := pipe.Read("prepared")
	if err != nil || prepared.Schema != store.SchemaVersion || len(prepared.Payload) != 0 {
		restartpipe.Wipe(prepared.Payload)
		return restartpipe.ErrProtocol
	}
	if p != nil {
		suspended = true // pause failures also need their loop/mobile restored
		if err := p.pauseForRestart(); err != nil {
			return err
		}
	}
	a.releaseInstance()
	released = true
	if err := pipe.Write("resume", store.SchemaVersion, payload); err != nil {
		return err
	}
	if _, err := pipe.Read("ready"); err != nil {
		return err
	}
	if err := pipe.Write("commit", 0, nil); err != nil {
		return err
	}
	if _, err := pipe.Read("committed"); err != nil {
		return err
	}
	if !timer.Stop() {
		<-timedOut
		return errors.New("restart.handoff_timeout")
	}
	timerStopped = true
	started = true
	// The child owns a usable session and its own browser profile now. The
	// original authenticated server/window survive every preceding failure.
	a.mobile.Stop()
	if p != nil {
		a.mu.Lock()
		a.prof = nil
		a.mu.Unlock()
		p.close()
	}
	if acct != nil {
		acct.Lock()
	}
	if a.opt.Shell != nil {
		a.opt.Shell.CloseWindow()
	}
	if a.srv != nil {
		a.srv.Close()
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
	b, err := io.ReadAll(io.LimitReader(r, (8<<10)+1))
	defer restartpipe.Wipe(b)
	if err != nil {
		return "", err
	}
	if len(b) == 0 || len(b) > 8<<10 {
		return "", restartpipe.ErrProtocol
	}
	var h handoff
	err = json.Unmarshal(b, &h)
	if err != nil {
		return "", restartpipe.ErrProtocol
	}
	h.Route = safeRoute(h.Route)
	if h.Account == "" {
		if h.AK != "" || h.Profile != "" || h.PDK != "" || h.Mobile {
			return "", restartpipe.ErrProtocol
		}
		return h.Route, nil
	}
	akb, err := secure.UnB64(h.AK)
	defer restartpipe.Wipe(akb)
	if err != nil {
		return h.Route, err
	}
	ak, err := secure.KeyFromBytes(akb)
	if err != nil {
		return h.Route, err
	}
	defer ak.Wipe()
	acct, err := a.Vault.UnlockWithKey(h.Account, ak)
	if err != nil {
		return h.Route, err
	}
	resumed := false
	defer func() {
		if !resumed {
			acct.Lock()
		}
	}()
	var p *Profile
	if h.Profile != "" {
		pkb, err := secure.UnB64(h.PDK)
		defer restartpipe.Wipe(pkb)
		if err != nil {
			return h.Route, err
		}
		pdk, err := secure.KeyFromBytes(pkb)
		if err != nil {
			return h.Route, err
		}
		defer pdk.Wipe()
		e, err := acct.Profile(h.Profile)
		if err != nil {
			return h.Route, err
		}
		p, err = a.openProfile(acct, e, pdk)
		if err != nil {
			return h.Route, err
		}
	} else if h.PDK != "" || h.Mobile {
		return h.Route, restartpipe.ErrProtocol
	}
	a.setAccount(acct)
	a.mu.Lock()
	a.prof = p
	a.mu.Unlock()
	resumed = true
	if h.Mobile && a.opt.HandoffConfirm == nil {
		a.mobile.Start()
	}
	// Transactional children activate mobile access only after commitment.
	if h.Mobile && a.opt.HandoffConfirm != nil {
		confirm := a.opt.HandoffConfirm
		a.opt.HandoffConfirm = func() error {
			if err := confirm(); err != nil {
				return err
			}
			a.mobile.Start()
			return nil
		}
	}
	return h.Route, nil
}

// SetTurbo persists the Turbo preference and restarts to apply it.
func (a *App) SetTurbo(on bool, route string) error {
	previous := a.Settings().Turbo
	if err := a.saveSettings(func(s *Settings) { s.Turbo = on }); err != nil {
		return err
	}
	if err := a.Restart("", route); err != nil {
		if restoreErr := a.saveSettings(func(s *Settings) { s.Turbo = previous }); restoreErr != nil {
			return errors.Join(err, restoreErr)
		}
		a.hub.Publish("state", nil)
		return err
	}
	return nil
}

// writeSyncMarker records the queue size after a sync so the exit logic
// can tell whether a closed profile still holds unsynchronized work.
func (p *Profile) writeSyncMarker() {
	st, err := p.St.Stats()
	if err != nil {
		return
	}
	atomicfile.WriteJSON(filepath.Join(p.dir, "synced.json"), map[string]any{"pending": st.Pending + st.Retryable + st.Conflicts, "verified": p.Verified(), "at": time.Now().UTC()})
}

var _ = store.StatePending
