package app

import (
	"bytes"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/instance"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
)

type restartProbeShell struct {
	closed int
	quit   int
}

func (*restartProbeShell) OpenWindow(string, bool) error { return nil }
func (*restartProbeShell) Raise() bool                   { return false }
func (s *restartProbeShell) CloseWindow()                { s.closed++ }
func (*restartProbeShell) WindowClosed() <-chan bool     { return nil }
func (*restartProbeShell) RunTray(TrayMenu)              {}
func (*restartProbeShell) UpdateTray(string, bool)       {}
func (*restartProbeShell) OpenExternal(string) error     { return nil }
func (s *restartProbeShell) QuitTray()                   { s.quit++ }

func restartSession(t *testing.T) (*App, *client, *Profile, *restartProbeShell) {
	t.Helper()
	a, c := portableApp(t)
	res, err := c.hc.Get(a.srv.LaunchURL("/"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if _, _, err := a.CreateAccount("Lab", "correct horse battery staple", false); err != nil {
		t.Fatal(err)
	}
	pv, err := a.CreateProfile(ProfileInput{Username: "Ana", StorageMode: "usb_only"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.OpenProfile(pv.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.acquireInstance(); err != nil {
		t.Fatal(err)
	}
	if err := a.lock.Publish(instance.Info{Port: a.srv.Port(), Token: a.srv.focus, PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.releaseInstance)
	sh := &restartProbeShell{}
	a.SetShell(sh)
	return a, c, p, sh
}

func assertRestartSessionPreserved(t *testing.T, a *App, c *client, p *Profile, sh *restartProbeShell) {
	t.Helper()
	if a.exiting.Load() {
		t.Error("failed restart left the application in its exiting state")
	}
	if sh.closed != 0 || sh.quit != 0 {
		t.Errorf("failed restart tore down the shell: closed=%d quit=%d", sh.closed, sh.quit)
	}
	if got, err := a.Profile(); err != nil || got != p {
		t.Errorf("failed restart lost the open profile: %v", err)
	}
	if _, err := p.St.Stats(); err != nil {
		t.Errorf("failed restart closed the profile store: %v", err)
	}
	if !instance.Held(a.L.Data) {
		t.Error("failed restart released the instance lock")
	}
	select {
	case <-a.Done():
		t.Error("failed restart finished the application")
	default:
	}
	if code, _ := c.do("GET", "/api/state", nil, nil); code != http.StatusOK {
		t.Errorf("failed restart invalidated the authenticated session: HTTP %d", code)
	}
}

func TestRestartMissingTargetPreservesSession(t *testing.T) {
	a, c, p, sh := restartSession(t)
	if err := a.Restart(filepath.Join(t.TempDir(), "missing"), "/ls/files"); err == nil {
		t.Fatal("restart accepted a missing executable")
	}
	assertRestartSessionPreserved(t, a, c, p, sh)
}

func TestRestartDirectoryTargetPreservesSession(t *testing.T) {
	a, c, p, sh := restartSession(t)
	if err := a.Restart(t.TempDir(), "/ls/files"); err == nil {
		t.Fatal("restart accepted a directory as an executable")
	}
	assertRestartSessionPreserved(t, a, c, p, sh)
}

func TestRestartNonExecutableTargetPreservesSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use Unix executable permission bits")
	}
	a, c, p, sh := restartSession(t)
	exe := filepath.Join(t.TempDir(), "not-executable")
	if err := os.WriteFile(exe, []byte("not executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.Restart(exe, "/ls/files"); err == nil {
		t.Fatal("restart accepted a file without execute permission")
	}
	assertRestartSessionPreserved(t, a, c, p, sh)
}

func TestRestartInvalidExecutablePreservesSession(t *testing.T) {
	a, c, p, sh := restartSession(t)
	exe := filepath.Join(t.TempDir(), "invalid-executable")
	if err := os.WriteFile(exe, []byte("this is not an executable image\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	initialTurbo := a.Settings().Turbo
	if err := a.Restart(exe, "/ls/files"); err == nil {
		t.Fatal("restart accepted an invalid executable image")
	}
	assertRestartSessionPreserved(t, a, c, p, sh)
	if a.Settings().Turbo != initialTurbo {
		t.Error("failed restart changed the Turbo preference")
	}
}

func TestRestartPendingRejectsAppMutations(t *testing.T) {
	a, c, p, sh := restartSession(t)
	a.exiting.Store(true)
	defer a.exiting.Store(false)
	for _, path := range []string{"/api/app/save", "/api/app/turbo", "/api/app/exit"} {
		if code, _ := c.do("POST", path, map[string]bool{"on": true}, nil); code != http.StatusServiceUnavailable {
			t.Fatalf("pending restart admitted %s: HTTP %d", path, code)
		}
	}
	if code, _ := c.do("GET", "/api/state", nil, nil); code != http.StatusOK {
		t.Fatal("pending restart blocked state polling")
	}
	a.exiting.Store(false)
	assertRestartSessionPreserved(t, a, c, p, sh)
}

func TestRestartBackupFailurePreservesSession(t *testing.T) {
	a, c, p, sh := restartSession(t)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// A file at the backup directory prevents saving the restart snapshot.
	if err := os.WriteFile(p.backupDir(), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.Restart(exe, "/ls/files"); err == nil {
		t.Fatal("restart continued after its preservation backup failed")
	}
	assertRestartSessionPreserved(t, a, c, p, sh)
}

func TestSetTurboRestartFailureRestoresPreference(t *testing.T) {
	for _, initial := range []bool{false, true} {
		name := "initially_off"
		if initial {
			name = "initially_on"
		}
		t.Run(name, func(t *testing.T) {
			a, c, p, sh := restartSession(t)
			if err := a.saveSettings(func(s *Settings) { s.Turbo = initial }); err != nil {
				t.Fatal(err)
			}
			a.opt.Exe = filepath.Join(t.TempDir(), "missing")
			if err := a.SetTurbo(!initial, "/ls/files"); err == nil {
				t.Fatal("Turbo change succeeded without a restart executable")
			}
			if a.Settings().Turbo != initial {
				t.Error("failed restart left the changed Turbo preference in memory")
			}
			var persisted Settings
			if _, err := atomicfile.ReadJSON(a.settingsPath(), &persisted); err != nil {
				t.Fatal(err)
			}
			if persisted.Turbo != initial {
				t.Error("failed restart left the changed Turbo preference on disk")
			}
			assertRestartSessionPreserved(t, a, c, p, sh)
		})
	}
}

func restartChildExecutable(t *testing.T) string {
	t.Helper()
	if exe := os.Getenv("MNELAB_RESTART_TEST_EXECUTABLE"); exe != "" {
		if st, err := os.Stat(exe); err != nil || !st.Mode().IsRegular() {
			t.Fatal("prebuilt synthetic restart fixture is unavailable")
		}
		return exe
	}
	exe := filepath.Join(t.TempDir(), "restart-child")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", exe, "./testdata/restart-child")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build synthetic process fixture: %v\n%s", err, out)
	}
	return exe
}

func TestRestartChildFailuresPreserveSession(t *testing.T) {
	exe := restartChildExecutable(t)
	previousLAN := lanIPFunc
	lanIPFunc = func() (net.IP, error) { return net.ParseIP("127.0.0.1"), nil }
	t.Cleanup(func() { lanIPFunc = previousLAN })
	for _, mode := range []string{"exit-before-prepared", "hang-before-prepared", "oversized-prepared", "wrong-phase", "wrong-schema", "exit-after-prepared", "hang-after-resume", "resume-fails", "window-fails", "exit-after-ready", "exit-before-committed"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("MNELAB_RESTART_TEST", mode)
			a, c, p, sh := restartSession(t)
			a.opt.Exe, a.opt.RestartTimeout = exe, time.Second
			originalStore, originalKey := p.St, p.key
			var phone *mdevice
			var phoneKey secure.Key
			var pairingBefore *pairing
			if mode == "window-fails" || mode == "resume-fails" {
				if _, err := a.mobile.Start(); err != nil {
					t.Fatal(err)
				}
				phone = &mdevice{MobileDevice: MobileDevice{ID: "synthetic-phone"}, key: secure.NewKey()}
				phoneKey = phone.key
				a.mobile.mu.Lock()
				a.mobile.devices[phone.ID] = phone
				pairingBefore = a.mobile.pair
				a.mobile.mu.Unlock()
			}
			if err := p.St.Update(func(tx *store.Tx) error {
				_, err := tx.Put("synthetic", "continuity", map[string]string{"value": "preserve"})
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if err := a.SetTurbo(true, "/ls/files"); err == nil {
				t.Fatal("failed child was accepted as a restart")
			}
			assertRestartSessionPreserved(t, a, c, p, sh)
			if phone != nil {
				a.mobile.mu.Lock()
				preserved := a.mobile.ln != nil && a.mobile.devices[phone.ID] == phone && phone.key == phoneKey && a.mobile.pair == pairingBefore
				a.mobile.mu.Unlock()
				if !preserved {
					t.Fatal("aborted restart invalidated existing phone pairing")
				}
			}
			if p.St != originalStore || p.key != originalKey {
				t.Fatal("rollback replaced the original store or key")
			}
			var persisted Settings
			if _, err := atomicfile.ReadJSON(a.settingsPath(), &persisted); err != nil || persisted.Turbo || a.Settings().Turbo {
				t.Fatal("Turbo preference was not rolled back")
			}
			var record map[string]string
			if err := p.St.View(func(tx *store.Tx) error { _, err := tx.Get("synthetic", "continuity", &record); return err }); err != nil || record["value"] != "preserve" {
				t.Fatal("rollback lost the saved record")
			}
			// A new write proves the original profile/store/watchers still work.
			if err := p.St.Update(func(tx *store.Tx) error {
				_, err := tx.Put("synthetic", "after", map[string]int{"value": 2})
				return err
			}); err != nil {
				t.Fatal(err)
			}
			info, err := instance.Running(a.L.Data)
			if err != nil || info.Port != a.srv.Port() || info.Token != a.srv.focus || info.PID != os.Getpid() {
				t.Fatal("rollback did not republish the original instance")
			}
		})
	}
}

func TestRestartChildSuccessTransfersUsableSession(t *testing.T) {
	exe := restartChildExecutable(t)
	t.Setenv("MNELAB_RESTART_TEST", "success")
	a, _, p, sh := restartSession(t)
	key := p.key
	defer key.Wipe()
	accountKey := a.acct.Key()
	defer accountKey.Wipe()
	dbPath := p.St.Path()
	if err := p.St.Update(func(tx *store.Tx) error {
		_, err := tx.Put("synthetic", "continuity", map[string]string{"value": "transfer"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	a.opt.RestartTimeout = 5 * time.Second
	if err := a.Restart(exe, "/ls/files"); err != nil {
		t.Fatal(err)
	}
	if sh.closed != 1 || sh.quit != 1 {
		t.Fatal("committed restart did not close the original shell")
	}
	select {
	case <-a.Done():
	default:
		t.Fatal("committed restart left the original process running")
	}
	info, err := instance.Running(a.L.Data)
	if err != nil || info.PID == os.Getpid() || !instance.Held(a.L.Data) {
		t.Fatal("replacement did not own the instance")
	}
	child, err := os.FindProcess(info.PID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { child.Kill() })
	if code, err := http.Get("http://127.0.0.1:" + itoa(info.Port) + "/api/state"); err != nil {
		t.Fatal("replacement server is unavailable")
	} else {
		code.Body.Close()
		if code.StatusCode != http.StatusUnauthorized {
			t.Fatal("replacement API is not protected")
		}
	}
	// Session keys must never be present in app-owned files or diagnostics.
	secrets := [][]byte{[]byte(secure.B64(key[:])), []byte(secure.B64(accountKey[:]))}
	if err := filepath.WalkDir(a.L.Data, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, secret := range secrets {
			if bytes.Contains(b, secret) {
				t.Fatalf("restart key persisted in %s", filepath.Base(path))
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	child.Kill()
	deadline := time.Now().Add(5 * time.Second)
	for instance.Held(a.L.Data) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	st, err := store.OpenExisting(dbPath, key, store.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var record map[string]string
	if err := st.View(func(tx *store.Tx) error { _, err := tx.Get("synthetic", "continuity", &record); return err }); err != nil || record["value"] != "transfer" {
		t.Fatal("replacement did not preserve encrypted data")
	}
}
