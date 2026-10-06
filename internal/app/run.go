package app

import (
	"errors"
	"io/fs"
	"os"
	"strconv"
	"time"

	"github.com/oaovito/mne_lab/internal/instance"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/update"
)

// Run serves the interface, opens the window and the tray, and returns when
// the application has exited (or restarted into another process).
func (a *App) Run(ui fs.FS) error {
	if err := a.acquireInstance(); err != nil {
		return err
	}
	a.ui = ui
	srv, err := newServer(a, ui)
	if err != nil {
		a.releaseInstance()
		return err
	}
	a.srv = srv
	a.lock.Publish(instance.Info{Port: srv.Port(), Token: srv.focus, PID: os.Getpid()})
	route := "/"
	if a.opt.Handoff != nil {
		r, err := a.resume(a.opt.Handoff)
		if err != nil {
			a.Log.Warn("session handoff incomplete", "err", err)
		}
		if r != "" {
			route = r
		}
	}
	if a.L.Mode == paths.Portable && a.opt.Build != "" {
		// Reaching this point counts as a successful start for rollback.
		update.MarkBootOK(a.L.Builds, a.opt.Build)
	}
	go a.upd.Run(a.ctx)
	if a.opt.Headless || a.opt.Shell == nil {
		<-a.quit
		return nil
	}
	sh := a.opt.Shell
	if err := sh.OpenWindow(srv.LaunchURL(route), a.Settings().Turbo); err != nil {
		a.Log.Error("window", "err", err)
		a.releaseInstance()
		return err
	}
	go a.watchWindow()
	a.updateTray()
	// The tray loop owns the main thread until the application quits.
	sh.RunTray(TrayMenu{
		Open:     func() { a.ShowWindow("") },
		Mobile:   func() { a.ShowWindow("/?panel=mobile") },
		Settings: func() { a.ShowWindow("/settings") },
		Exit:     func() { go a.exitFromShell() },
		Label:    a.T,
	})
	<-a.quit
	return nil
}

// watchWindow follows the window: closing it is an intentional exit; an
// abnormal end of the window process is not an exit, so it is reopened.
func (a *App) watchWindow() {
	sh := a.opt.Shell
	var crashes []time.Time
	for {
		select {
		case <-a.quit:
			return
		case intentional, ok := <-sh.WindowClosed():
			if !ok {
				return
			}
			if a.exiting.Load() {
				continue
			}
			if intentional {
				go a.exitFromShell()
				continue
			}
			crashes = append(crashes, time.Now())
			if len(crashes) > 3 {
				crashes = crashes[1:]
			}
			if len(crashes) == 3 && time.Since(crashes[0]) < time.Minute {
				a.Log.Error("window keeps failing; staying in the tray")
				continue
			}
			a.Log.Warn("window ended unexpectedly; reopening")
			a.ShowWindow("")
		}
	}
}

// exitFromShell runs the safe exit started by the window close button or
// the tray. When the exit takes a while, a small window shows its steps.
func (a *App) exitFromShell() {
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-time.After(1500 * time.Millisecond):
			if a.srv != nil && a.opt.Shell != nil {
				a.opt.Shell.OpenWindow(a.srv.LaunchURL("/exit"), false)
			}
		}
	}()
	a.Exit()
	close(done)
}

// ShowWindow brings the window back (or opens it) at route.
func (a *App) ShowWindow(route string) {
	if a.opt.Shell == nil || a.srv == nil || a.exiting.Load() {
		return
	}
	if a.opt.Shell.Raise() {
		a.hub.Publish("focus", nil)
		if route != "" {
			a.hub.Publish("navigate", safeRoute(route))
		}
		return
	}
	if route == "" {
		route = "/"
	}
	if err := a.opt.Shell.OpenWindow(a.srv.LaunchURL(route), a.Settings().Turbo); err != nil {
		a.Log.Warn("window", "err", err)
	}
}

// updateTray refreshes the tray status line.
func (a *App) updateTray() {
	if a.opt.Shell == nil {
		return
	}
	a.mu.Lock()
	p := a.prof
	a.mu.Unlock()
	status := a.T("tray.status.signed_out")
	if p != nil {
		st := p.Status()
		switch st.State {
		case SyncLocal:
			status = a.T("tray.status.local")
		case SyncSynced:
			status = a.T("tray.status.synced")
		case SyncOffline, SyncReauth:
			status = a.T("tray.status.offline")
		default:
			status = a.T("tray.status.pending", "n", strconv.Itoa(st.Pending))
		}
	}
	a.opt.Shell.UpdateTray(status, a.L.Mode == paths.Temporary)
}

// ---- presentation (desktop and paired phones control the same state) ----

type presentation struct {
	Active bool     `json:"active"`
	Graphs []string `json:"graphs"`
	Index  int      `json:"index"`
	// Display choices of the presentation only: they never change the
	// saved graph and reset when another graph is shown.
	NoLegend bool     `json:"noLegend,omitempty"`
	Hidden   []string `json:"hidden,omitempty"` // series ids
}

// ErrPresentation is returned for an invalid presentation command.
var ErrPresentation = errors.New("present.invalid")

// Present changes the presentation: start (with graphs), next, prev, goto
// (index), legend (show or hide), series (show or hide one series) or
// stop. Every window and phone follows the same state.
func (a *App) Present(action string, graphs []string, index int, series string) (presentation, error) {
	p, err := a.Profile()
	if err != nil {
		return presentation{}, err
	}
	a.mu.Lock()
	cur := a.present
	a.mu.Unlock()
	switch action {
	case "start":
		var ok []string
		for _, id := range graphs {
			if _, err := p.Graph(id); err == nil {
				ok = append(ok, id)
			}
		}
		if len(ok) == 0 {
			return cur, ErrPresentation
		}
		if index < 0 || index >= len(ok) {
			index = 0
		}
		cur = presentation{Active: true, Graphs: ok, Index: index}
	case "next", "prev", "goto":
		if !cur.Active {
			return cur, ErrPresentation
		}
		switch action {
		case "next":
			index = cur.Index + 1
		case "prev":
			index = cur.Index - 1
		}
		if index < 0 || index >= len(cur.Graphs) {
			return cur, nil
		}
		if index != cur.Index {
			cur.NoLegend, cur.Hidden = false, nil
		}
		cur.Index = index
	case "legend":
		if !cur.Active {
			return cur, ErrPresentation
		}
		cur.NoLegend = !cur.NoLegend
	case "series":
		if !cur.Active || series == "" || len(series) > 200 {
			return cur, ErrPresentation
		}
		hidden := []string{}
		found := false
		for _, h := range cur.Hidden {
			if h == series {
				found = true
			} else {
				hidden = append(hidden, h)
			}
		}
		if !found && len(hidden) < 500 {
			hidden = append(hidden, series)
		}
		cur.Hidden = hidden
	case "stop":
		cur = presentation{}
	default:
		return cur, ErrPresentation
	}
	a.mu.Lock()
	a.present = cur
	a.mu.Unlock()
	a.hub.Publish("present", cur)
	return cur, nil
}

// presented applies the presentation's display choices to a copy of a
// graph definition (the saved graph is never changed).
func presented(def graph.Definition, pr presentation) graph.Definition {
	if pr.NoLegend {
		def.Visual.Legend = false
	}
	if len(pr.Hidden) > 0 {
		series := map[string]graph.SeriesStyle{}
		for k, v := range def.Series {
			series[k] = v
		}
		for _, id := range pr.Hidden {
			st := series[id]
			st.Hidden = true
			series[id] = st
		}
		def.Series = series
	}
	return def
}

// Presentation returns the current presentation state.
func (a *App) Presentation() presentation {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.present
}
