package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skip2/go-qrcode"

	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/plot"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/version"
)

// Mobile access: a phone on the same network opens MNE Lab in its browser
// by scanning a QR code. The connection is direct (nothing passes through
// any server of the project). The QR code carries a single-use pairing
// secret in the URL fragment, which browsers never send over the network;
// knowing the address and port is never enough. After pairing, every
// request and event is encrypted and authenticated with a per-device
// session key, numbered against replay, and limited to a fixed scope:
// viewing and presentation control. Stopping mobile access, closing the
// profile or exiting invalidates every phone at once.

// Errors (stable identifiers).
var (
	ErrMobileNetwork = errors.New("mobile.no_network")
	ErrMobileOff     = errors.New("mobile.inactive")
	ErrMobileDevices = errors.New("mobile.device_limit_reached")
	ErrMobileOp      = errors.New("mobile.not_allowed")
)

const (
	pairingTTL    = 10 * time.Minute
	deviceIdle    = 30 * time.Minute
	deviceMaxAge  = 12 * time.Hour
	clockSkew     = 2 * time.Minute
	maxDevices    = 3
	maxPairFails  = 10
	maxMobileBody = 64 << 10
)

// MobileInfo is shown in the mobile access panel.
type MobileInfo struct {
	Active  bool           `json:"active"`
	URL     string         `json:"url,omitempty"` // contains the pairing secret: shown only as a QR code
	QR      string         `json:"qr,omitempty"`  // SVG
	Address string         `json:"address,omitempty"`
	Expires time.Time      `json:"expires,omitzero"`
	Devices []MobileDevice `json:"devices"`
}

// MobileDevice is a paired phone.
type MobileDevice struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Paired   time.Time `json:"paired"`
	LastSeen time.Time `json:"lastSeen"`
}

type pairing struct {
	id      string
	key     secure.Key
	expires time.Time
	fails   int
}

type mdevice struct {
	MobileDevice
	key     secure.Key
	seq     uint64
	eventTS int64
	cancel  []func()
}

// Mobile is the mobile access service.
type Mobile struct {
	app *App

	mu      sync.Mutex
	ln      net.Listener
	srv     *http.Server
	host    string
	pair    *pairing
	devices map[string]*mdevice
}

func newMobile(a *App) *Mobile { return &Mobile{app: a, devices: map[string]*mdevice{}} }

// Active reports whether mobile access is on.
func (m *Mobile) Active() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ln != nil
}

// Start turns mobile access on (when needed) and creates a fresh pairing
// code. It needs an open profile.
func (m *Mobile) Start() (MobileInfo, error) {
	if _, err := m.app.Profile(); err != nil {
		return MobileInfo{}, err
	}
	m.mu.Lock()
	if m.ln == nil {
		ip, err := lanIPFunc()
		if err != nil {
			m.mu.Unlock()
			return MobileInfo{}, err
		}
		ln, err := net.Listen("tcp4", net.JoinHostPort(ip.String(), "0"))
		if err != nil {
			m.mu.Unlock()
			return MobileInfo{}, ErrMobileNetwork
		}
		m.ln, m.host = ln, ln.Addr().String()
		m.srv = &http.Server{Handler: m, ReadHeaderTimeout: 10 * time.Second, MaxHeaderBytes: 16 << 10, IdleTimeout: 2 * time.Minute}
		go m.srv.Serve(ln)
		go m.expireLoop(ln)
		m.app.Log.Info("mobile access on")
	}
	m.newPairingLocked()
	m.mu.Unlock()
	info := m.Info()
	m.app.hub.Publish("mobile", info.public())
	return info, nil
}

func (m *Mobile) newPairingLocked() {
	if m.pair != nil {
		m.pair.key.Wipe()
	}
	m.pair = &pairing{id: secure.NewID()[:16], key: secure.NewKey(), expires: time.Now().Add(pairingTTL)}
}

// Stop turns mobile access off and invalidates every phone.
func (m *Mobile) Stop() {
	m.mu.Lock()
	if m.ln == nil {
		m.mu.Unlock()
		return
	}
	srv := m.srv
	m.ln, m.srv, m.host = nil, nil, ""
	if m.pair != nil {
		m.pair.key.Wipe()
		m.pair = nil
	}
	for id, d := range m.devices {
		d.key.Wipe()
		for _, c := range d.cancel {
			c()
		}
		delete(m.devices, id)
	}
	m.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	srv.Shutdown(ctx)
	cancel()
	m.app.Log.Info("mobile access off")
	m.app.hub.Publish("mobile", MobileInfo{Devices: []MobileDevice{}})
}

// Revoke disconnects one phone.
func (m *Mobile) Revoke(id string) {
	m.mu.Lock()
	if d, ok := m.devices[id]; ok {
		d.key.Wipe()
		for _, c := range d.cancel {
			c()
		}
		delete(m.devices, id)
	}
	m.mu.Unlock()
	m.app.hub.Publish("mobile", m.Info().public())
}

// Info describes mobile access, including the current QR code.
func (m *Mobile) Info() MobileInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	info := MobileInfo{Active: m.ln != nil, Devices: []MobileDevice{}}
	for _, d := range m.devices {
		info.Devices = append(info.Devices, d.MobileDevice)
	}
	sort.Slice(info.Devices, func(i, j int) bool { return info.Devices[i].Paired.Before(info.Devices[j].Paired) })
	if m.ln == nil {
		return info
	}
	info.Address = m.host
	if m.pair != nil && time.Now().Before(m.pair.expires) {
		info.URL = "http://" + m.host + "/m#p=" + m.pair.id + "&k=" + secure.B64(m.pair.key[:])
		info.QR = qrSVG(info.URL)
		info.Expires = m.pair.expires
	}
	return info
}

// public strips the pairing secret for broadcast events.
func (i MobileInfo) public() MobileInfo {
	i.URL, i.QR = "", ""
	return i
}

func (m *Mobile) expireLoop(ln net.Listener) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for range t.C {
		m.mu.Lock()
		if m.ln != ln {
			m.mu.Unlock()
			return
		}
		changed := false
		now := time.Now()
		for id, d := range m.devices {
			if now.Sub(d.LastSeen) > deviceIdle || now.Sub(d.Paired) > deviceMaxAge {
				d.key.Wipe()
				for _, c := range d.cancel {
					c()
				}
				delete(m.devices, id)
				changed = true
			}
		}
		m.mu.Unlock()
		if changed {
			m.app.hub.Publish("mobile", m.Info().public())
		}
	}
}

var lanIPFunc = lanIP // tests use the loopback address

// lanIP picks the private IPv4 address of the network this computer uses.
func lanIP() (net.IP, error) {
	// Connecting a UDP socket sends nothing; it only selects the route.
	if c, err := net.Dial("udp4", "192.0.2.1:9"); err == nil {
		ip := c.LocalAddr().(*net.UDPAddr).IP
		c.Close()
		if ip.IsPrivate() {
			return ip, nil
		}
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, ErrMobileNetwork
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, ad := range addrs {
			if n, ok := ad.(*net.IPNet); ok {
				if ip := n.IP.To4(); ip != nil && ip.IsPrivate() {
					return ip, nil
				}
			}
		}
	}
	return nil, ErrMobileNetwork
}

func qrSVG(text string) string {
	q, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		return ""
	}
	bm := q.Bitmap()
	n := len(bm)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges"><rect width="%d" height="%d" fill="#fff"/><path fill="#0E1116" d="`, n, n, n, n)
	for y, row := range bm {
		for x := 0; x < len(row); x++ {
			if !row[x] {
				continue
			}
			start := x
			for x < len(row) && row[x] {
				x++
			}
			fmt.Fprintf(&b, "M%d %dh%dv1h-%dz", start, y, x-start, x-start)
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String()
}

// ---- LAN server ----

func (m *Mobile) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	host := m.host
	m.mu.Unlock()
	if host == "" || r.Host != host {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; font-src 'self'; connect-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	h.Set("Cache-Control", "no-store")
	if o := r.Header.Get("Origin"); o != "" && o != "http://"+host {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	switch {
	case r.URL.Path == "/m" || r.URL.Path == "/m/":
		m.serveShell(w, r, "m.html")
	case strings.HasPrefix(r.URL.Path, "/m/assets/") || mobileStatic[r.URL.Path]:
		m.serveShell(w, r, strings.TrimPrefix(r.URL.Path, "/m/"))
	case r.URL.Path == "/m/api/pair" && r.Method == http.MethodPost:
		m.handlePair(w, r)
	case r.URL.Path == "/m/api/call" && r.Method == http.MethodPost:
		m.handleCall(w, r)
	case r.URL.Path == "/m/api/events" && r.Method == http.MethodGet:
		m.handleEvents(w, r)
	default:
		http.NotFound(w, r)
	}
}

var mobileStatic = map[string]bool{"/m/icon.svg": true, "/m/manifest.webmanifest": true, "/m/icon-192.png": true, "/m/icon-512.png": true, "/m/apple-touch-icon.png": true}

func (m *Mobile) serveShell(w http.ResponseWriter, r *http.Request, name string) {
	ui := m.app.ui
	if ui == nil {
		http.NotFound(w, r)
		return
	}
	if st, err := fs.Stat(ui, name); err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	http.ServeFileFS(w, r, ui, name)
}

func mac(key []byte, parts ...string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(strings.Join(parts, "|")))
	return h.Sum(nil)
}

func freshTS(ms int64) bool {
	d := time.Since(time.UnixMilli(ms))
	return d < clockSkew && d > -clockSkew
}

type pairRequest struct {
	P     string `json:"p"`
	TS    int64  `json:"ts"`
	N     string `json:"n"`
	Label string `json:"label"`
	MAC   string `json:"mac"`
}

func (m *Mobile) handlePair(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
		writeErr(w, errors.New("request.invalid_json"), http.StatusBadRequest)
		return
	}
	label := clean(req.Label, 60)
	if label == "" {
		label = "Smartphone"
	}
	m.mu.Lock()
	pr := m.pair
	if pr == nil || pr.id != req.P || time.Now().After(pr.expires) {
		m.mu.Unlock()
		writeErr(w, errors.New("mobile.pairing_expired"), http.StatusUnauthorized)
		return
	}
	got, _ := secure.UnB64(req.MAC)
	want := mac(pr.key[:], "mnelab-pair", req.P, strconv.FormatInt(req.TS, 10), req.N, req.Label)
	if !freshTS(req.TS) || !hmac.Equal(got, want) {
		pr.fails++
		if pr.fails >= maxPairFails {
			pr.key.Wipe()
			m.pair = nil
		}
		m.mu.Unlock()
		writeErr(w, errors.New("mobile.pairing_invalid"), http.StatusUnauthorized)
		return
	}
	if len(m.devices) >= maxDevices {
		m.mu.Unlock()
		writeErr(w, ErrMobileDevices, http.StatusConflict)
		return
	}
	// Single use: this code cannot pair a second phone.
	m.pair = nil
	now := time.Now().UTC()
	d := &mdevice{MobileDevice: MobileDevice{ID: secure.NewID()[:16], Label: label, Paired: now, LastSeen: now}, key: secure.NewKey()}
	m.devices[d.ID] = d
	payload, _ := json.Marshal(map[string]string{"key": secure.B64(d.key[:])})
	box := secure.Seal(pr.key, payload, []byte("mnelab-pair-resp|"+d.ID))
	pr.key.Wipe()
	m.mu.Unlock()
	m.app.hub.Publish("mobile", m.Info().public())
	m.app.hub.Publish("notice", map[string]string{"key": "mobile.device_connected", "label": label})
	writeJSON(w, http.StatusOK, map[string]string{"sid": d.ID, "box": secure.B64(box)})
}

type callRequest struct {
	Seq  uint64          `json:"seq"`
	TS   int64           `json:"ts"`
	Op   string          `json:"op"`
	Args json.RawMessage `json:"args,omitempty"`
}

func (m *Mobile) device(sid string) (*mdevice, secure.Key, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.devices[sid]
	if !ok {
		return nil, secure.Key{}, false
	}
	return d, d.key, true
}

func (m *Mobile) handleCall(w http.ResponseWriter, r *http.Request) {
	sid := r.Header.Get("X-Sid")
	d, key, ok := m.device(sid)
	if !ok {
		writeErr(w, errors.New("mobile.session_ended"), http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxMobileBody))
	if err != nil {
		writeErr(w, errors.New("request.invalid"), http.StatusBadRequest)
		return
	}
	raw, err := secure.UnB64(string(body))
	if err != nil {
		writeErr(w, errors.New("request.invalid"), http.StatusBadRequest)
		return
	}
	pt, err := secure.Open(key, raw, []byte("mnelab-req|"+sid))
	if err != nil {
		writeErr(w, errors.New("mobile.session_ended"), http.StatusUnauthorized)
		return
	}
	var req callRequest
	if err := json.Unmarshal(pt, &req); err != nil {
		writeErr(w, errors.New("request.invalid_json"), http.StatusBadRequest)
		return
	}
	m.mu.Lock()
	if req.Seq <= d.seq || !freshTS(req.TS) {
		m.mu.Unlock()
		writeErr(w, errors.New("mobile.replayed"), http.StatusUnauthorized)
		return
	}
	d.seq = req.Seq
	d.LastSeen = time.Now().UTC()
	m.mu.Unlock()

	data, cerr := m.call(req.Op, req.Args)
	resp := map[string]any{"ok": cerr == nil}
	if cerr != nil {
		msg := cerr.Error()
		if errorStatus(cerr) == http.StatusInternalServerError {
			msg = "app.internal_error"
			m.app.Log.Warn("mobile call failed", "op", req.Op, "err", cerr)
		}
		resp["error"] = msg
	} else {
		resp["data"] = data
	}
	out, _ := json.Marshal(resp)
	box := secure.Seal(key, out, []byte("mnelab-res|"+sid+"|"+strconv.FormatUint(req.Seq, 10)))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, secure.B64(box))
}

// mobileEvents are the event types a phone may see.
var mobileEvents = map[string]bool{"sync": true, "present": true, "activity": true, "state": true, "mobile": true, "library": true}

func (m *Mobile) handleEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	sid := q.Get("sid")
	ts, _ := strconv.ParseInt(q.Get("ts"), 10, 64)
	d, key, ok := m.device(sid)
	got, _ := secure.UnB64(q.Get("mac"))
	if !ok || !freshTS(ts) || !hmac.Equal(got, mac(key[:], "mnelab-events", sid, strconv.FormatInt(ts, 10))) {
		writeErr(w, errors.New("mobile.session_ended"), http.StatusUnauthorized)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, errors.New("request.invalid"), http.StatusBadRequest)
		return
	}
	m.mu.Lock()
	if ts <= d.eventTS {
		m.mu.Unlock()
		writeErr(w, errors.New("mobile.replayed"), http.StatusUnauthorized)
		return
	}
	d.eventTS = ts
	ch, cancel := m.app.hub.Subscribe()
	ended := make(chan struct{})
	var once sync.Once
	stop := func() { once.Do(func() { close(ended) }) }
	d.cancel = append(d.cancel, stop)
	m.mu.Unlock()
	defer cancel()
	defer key.Wipe()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": ok\n\n")
	fl.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ended:
			// Tell the phone the session is over before the connection closes.
			box := secure.Seal(key, []byte(`{"type":"ended"}`), []byte("mnelab-evt|"+sid))
			fmt.Fprintf(w, "data: %s\n\n", secure.B64(box))
			fl.Flush()
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case b, ok := <-ch:
			if !ok {
				return
			}
			var ev struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(b, &ev) != nil || !mobileEvents[ev.Type] {
				continue
			}
			if ev.Type == "mobile" {
				b = []byte(`{"type":"mobile"}`) // the device list stays on the computer
			}
			box := secure.Seal(key, b, []byte("mnelab-evt|"+sid))
			fmt.Fprintf(w, "data: %s\n\n", secure.B64(box))
			fl.Flush()
		}
	}
}

// ---- the mobile scope ----

// MobileState is the overview a phone shows.
type MobileState struct {
	Version     string       `json:"version"`
	Mode        paths.Mode   `json:"mode"`
	Lang        string       `json:"lang"`
	Decimal     string       `json:"decimal"`
	Profile     string       `json:"profile"`
	Color       int          `json:"color"`
	Sync        SyncStatus   `json:"sync"`
	Activities  []Activity   `json:"activities"`
	Present     presentation `json:"present"`
	Files       int          `json:"files"`
	Graphs      int          `json:"graphs"`
	Cycles      int          `json:"cycles"`
	Measurement int          `json:"measurements"`
}

type mobileSeries struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Color  string `json:"color,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
}

type mobileGraph struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Kind    string    `json:"kind"`
	Updated time.Time `json:"updated"`
}

type mobileCycle struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Sample   string    `json:"sample,omitempty"`
	Points   int       `json:"points"`
	Assigned int       `json:"assigned"`
	Updated  time.Time `json:"updated"`
}

// call runs one operation allowed to phones: viewing (state, files,
// graphs, cycles) and control (presentation, synchronization).
func (m *Mobile) call(op string, args json.RawMessage) (any, error) {
	p, err := m.app.Profile()
	if err != nil {
		return nil, err
	}
	var a struct {
		ID     string   `json:"id"`
		Action string   `json:"action"`
		Graphs []string `json:"graphs"`
		Index  int      `json:"index"`
		Width  float64  `json:"width"`
		Height float64  `json:"height"`
		Series string   `json:"series"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, errors.New("request.invalid_json")
		}
	}
	switch op {
	case "state":
		st := MobileState{Version: version.Version, Mode: m.app.L.Mode, Lang: m.app.Lang(), Decimal: m.app.decimal(),
			Profile: p.Entry.Username, Color: p.Entry.Color, Sync: p.Status(), Activities: m.app.Activities(), Present: m.app.Presentation()}
		if fs, err := p.Files(false); err == nil {
			st.Files = len(fs)
			for _, f := range fs {
				st.Measurement += len(f.Items)
			}
		}
		if gs, err := p.Graphs(false); err == nil {
			st.Graphs = len(gs)
		}
		if cs, err := p.Cycles(false); err == nil {
			st.Cycles = len(cs)
		}
		return st, nil
	case "files":
		return p.Files(false)
	case "graphs":
		gs, err := p.Graphs(false)
		if err != nil {
			return nil, err
		}
		out := []mobileGraph{}
		for _, g := range gs {
			out = append(out, mobileGraph{ID: g.ID, Title: g.Title, Kind: g.Kind, Updated: g.Updated})
		}
		return out, nil
	case "graph":
		def, err := p.Graph(a.ID)
		if err != nil {
			return nil, err
		}
		// The graph being presented is shown as the audience sees it.
		if pr := m.app.Presentation(); pr.Active && pr.Index < len(pr.Graphs) && pr.Graphs[pr.Index] == def.ID {
			def = presented(def, pr)
		}
		spec := WithVisual(plot.Preset(plot.PresetPresentation), def.Visual)
		if a.Width >= 240 && a.Height >= 180 && a.Width <= 4000 && a.Height <= 4000 {
			spec.Width, spec.Height, spec.DPI = a.Width, a.Height, 96
			spec.FontSize = 10
		}
		fig, res, err := p.Render(def, spec)
		if err != nil {
			return nil, err
		}
		series := []mobileSeries{}
		for _, sr := range res.Series {
			series = append(series, mobileSeries{ID: sr.ID, Label: sr.Label, Color: sr.Color, Hidden: sr.Hidden})
		}
		return map[string]any{"id": def.ID, "title": def.Title, "kind": def.Kind, "svg": string(plot.SVG(fig, spec)), "series": series}, nil
	case "cycles":
		cs, err := p.Cycles(false)
		if err != nil {
			return nil, err
		}
		out := []mobileCycle{}
		for _, c := range cs {
			mc := mobileCycle{ID: c.ID, Name: c.Config.Name, Sample: c.Config.SampleID, Updated: c.Updated}
			if v, err := p.cycleView(c); err == nil {
				mc.Points = len(v.Points)
				for _, as := range v.Assignments {
					if as.Point >= 0 {
						mc.Assigned++
					}
				}
			}
			out = append(out, mc)
		}
		return out, nil
	case "cycle":
		return p.CycleDetail(a.ID)
	case "present":
		return m.app.Present(a.Action, a.Graphs, a.Index, a.Series)
	case "sync":
		p.Kick()
		return p.Status(), nil
	}
	return nil, ErrMobileOp
}
