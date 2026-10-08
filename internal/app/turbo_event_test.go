package app

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestTurboAPIFailureReportsUsableSession(t *testing.T) {
	a, c := portableApp(t)
	res, err := c.hc.Get(a.srv.LaunchURL("/"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	a.opt.Exe = filepath.Join(t.TempDir(), "missing-build")
	previous := a.Settings().Turbo
	events, cancel := a.hub.Subscribe()
	defer cancel()
	c.json("POST", "/api/app/turbo", map[string]any{"on": !previous, "route": "/"}, nil)
	deadline := time.After(3 * time.Second)
	for {
		select {
		case b := <-events:
			var event struct {
				Type string `json:"type"`
				Data struct {
					Code string `json:"code"`
				} `json:"data"`
			}
			if err := json.Unmarshal(b, &event); err != nil {
				t.Fatal(err)
			}
			if event.Type != "restart-error" {
				continue
			}
			if event.Data.Code != "restart.failed" || a.Settings().Turbo != previous || a.exiting.Load() {
				t.Fatal("failed restart did not preserve settings/session")
			}
			code, _ := c.do("GET", "/api/state", nil, nil)
			if code != 200 {
				t.Fatalf("session unusable after failure: %d", code)
			}
			return
		case <-deadline:
			t.Fatal("restart failure was not reported to the interface")
		}
	}
}
