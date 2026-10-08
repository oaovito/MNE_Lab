package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/instance"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/provider"
)

func jsonIndent(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func providerFolders() map[string]string { return provider.DetectFolders() }

func instanceHeld(dir string) bool { return instance.Held(dir) }

// acquireInstance takes the one-instance lock of the data folder.
func (a *App) acquireInstance() error {
	l, err := instance.Acquire(a.L.Data)
	if err != nil {
		return err
	}
	a.lock = l
	return nil
}

func (a *App) releaseInstance() {
	if a.lock != nil {
		a.lock.Release()
		a.lock = nil
	}
}

// FocusRunning asks an MNE Lab already running for this installation (or,
// for a downloaded executable, any running Temporary Mode session) to show
// its window. It reports whether one answered, in which case this launch
// should simply end.
func FocusRunning(exe, root, baseTemp string) bool {
	var dirs []string
	if root == "" {
		root = paths.FindPortableRoot(exe)
	}
	if root != "" {
		dirs = append(dirs, filepath.Join(root, "data"))
	} else {
		for _, s := range paths.StaleSessions(baseTemp, "") {
			dirs = append(dirs, filepath.Join(s, "data"))
		}
	}
	for _, d := range dirs {
		if !instance.Held(d) {
			continue
		}
		info, err := instance.Running(d)
		if err != nil || info.Port == 0 {
			continue
		}
		if focus(info) == nil {
			return true
		}
	}
	return false
}

func focus(info instance.Info) error {
	req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+itoa(info.Port)+"/focus", strings.NewReader(""))
	req.Host = "127.0.0.1:" + itoa(info.Port)
	req.Header.Set("X-Focus", info.Token)
	c := &http.Client{Timeout: 3 * time.Second}
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return errors.New("instance.focus_refused")
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
