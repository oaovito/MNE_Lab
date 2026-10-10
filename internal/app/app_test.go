package app

import (
	"bytes"
	"crypto/hmac"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/oaovito/mne_lab/internal/atomicfile"
	"github.com/oaovito/mne_lab/internal/export"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/secure"
)

func fastKDF() secure.KDFParams {
	p := secure.DefaultKDF()
	p.Time, p.MemoryKiB, p.Threads = 1, 8*1024, 1
	return p
}

type client struct {
	t    *testing.T
	base string
	hc   *http.Client
}

func (c *client) do(method, path string, body any, hdr map[string]string) (int, []byte) {
	c.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		rd = bytes.NewReader(b)
	default:
		j, _ := json.Marshal(b)
		rd = bytes.NewReader(j)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if method != http.MethodGet {
		req.Header.Set("X-MNE-Lab", "1")
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res.StatusCode, out
}

func (c *client) json(method, path string, body any, v any) {
	c.t.Helper()
	code, out := c.do(method, path, body, nil)
	if code >= 300 {
		c.t.Fatalf("%s %s: %d %s", method, path, code, out)
	}
	if v != nil && len(out) > 0 {
		if err := json.Unmarshal(out, v); err != nil {
			c.t.Fatalf("%s %s: %v (%s)", method, path, err, out)
		}
	}
}

func portableApp(t *testing.T) (*App, *client) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, paths.PortableMarker), nil, 0o644)
	a, err := New(Options{Exe: filepath.Join(root, "app", "0.1.0", "mnelab"), Root: root, Headless: true, KDF: fastKDF, UserHome: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	ui := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>MNE Lab</title>")}, "m.html": {Data: []byte("<!doctype html><title>MNE Lab</title>")}}
	a.ui = ui
	srv, err := newServer(a, ui)
	if err != nil {
		t.Fatal(err)
	}
	a.srv = srv
	t.Cleanup(a.Close)
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, base: "http://" + srv.host, hc: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
	return a, c
}

func TestServerAccess(t *testing.T) {
	a, c := portableApp(t)
	// No session yet.
	if code, _ := c.do("GET", "/api/state", nil, nil); code != http.StatusUnauthorized {
		t.Fatalf("state without session: %d", code)
	}
	res, err := c.hc.Get(a.srv.LaunchURL("/"))
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("launch: %v %v", err, res)
	}
	res.Body.Close()
	// The code is single use.
	u := a.srv.LaunchURL("/")
	jar, _ := cookiejar.New(nil)
	other := &http.Client{Jar: jar}
	r1, _ := other.Get(u)
	r1.Body.Close()
	r2, _ := (&http.Client{}).Get(u)
	if r2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("code reused: %d", r2.StatusCode)
	}
	r2.Body.Close()
	if code, _ := c.do("GET", "/api/state", nil, nil); code != 200 {
		t.Fatalf("state: %d", code)
	}
	// Mutations need the custom header and a matching origin.
	req, _ := http.NewRequest("POST", c.base+"/api/profile/close", nil)
	if res, _ := c.hc.Do(req); res.StatusCode != http.StatusForbidden {
		t.Fatalf("missing header accepted: %d", res.StatusCode)
	}
	if code, _ := c.do("POST", "/api/profile/close", nil, map[string]string{"Origin": "http://evil.example"}); code != http.StatusForbidden {
		t.Fatalf("foreign origin accepted: %d", code)
	}
	// DNS rebinding: a foreign Host is refused.
	req, _ = http.NewRequest("GET", c.base+"/api/state", nil)
	req.Host = "attacker.example:80"
	if res, _ := c.hc.Do(req); res.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign host accepted: %d", res.StatusCode)
	}
}

func TestEndToEnd(t *testing.T) {
	a, c := portableApp(t)
	res, _ := c.hc.Get(a.srv.LaunchURL("/"))
	res.Body.Close()

	var created struct {
		RecoveryKey string      `json:"recoveryKey"`
		Account     AccountView `json:"account"`
	}
	c.json("POST", "/api/account/create", map[string]any{"name": "Lab", "passphrase": "correct horse battery staple"}, &created)
	if created.RecoveryKey == "" || created.Account.ID == "" {
		t.Fatal("no recovery key")
	}
	var pv ProfileView
	c.json("POST", "/api/profiles", map[string]any{"username": "Ana", "storageMode": "usb_only"}, &pv)
	c.json("POST", "/api/profiles/"+pv.ID+"/open", map[string]any{}, nil)

	// A sixth profile is refused.
	for i := 0; i < 4; i++ {
		c.json("POST", "/api/profiles", map[string]any{"username": "P" + strconv.Itoa(i), "storageMode": "usb_only"}, nil)
	}
	if code, _ := c.do("POST", "/api/profiles", map[string]any{"username": "Sixth", "storageMode": "usb_only"}, nil); code != http.StatusConflict {
		t.Fatalf("sixth profile: %d", code)
	}

	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-tab.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var imp ImportResult
	code, out := c.do("POST", "/api/files", data, map[string]string{"X-File-Name": url.QueryEscape("A1 run.txt")})
	if code != 200 {
		t.Fatalf("import: %d %s", code, out)
	}
	json.Unmarshal(out, &imp)
	if imp.Status != "parsed" || imp.Measurements == 0 {
		t.Fatalf("import result %+v", imp)
	}
	// The same bytes again are recognized as a duplicate.
	_, out = c.do("POST", "/api/files", data, map[string]string{"X-File-Name": "copy.txt"})
	var dup ImportResult
	json.Unmarshal(out, &dup)
	if dup.Status != "duplicate" {
		t.Fatalf("duplicate: %+v", dup)
	}
	var files []FileView
	c.json("GET", "/api/files", nil, &files)
	if len(files) != 1 || len(files[0].Items) == 0 {
		t.Fatalf("files %+v", files)
	}
	// The original is returned byte for byte.
	_, orig := c.do("GET", "/api/files/"+files[0].ID+"/original", nil, nil)
	if !bytes.Equal(orig, data) {
		t.Fatal("original changed")
	}

	mid := files[0].Items[0].ID
	var def struct {
		ID string `json:"id"`
	}
	c.json("POST", "/api/graphs", map[string]any{"kind": "dls_distribution", "title": "A1", "measurements": []string{mid}, "visual": map[string]any{"legend": true, "grid": true, "lineWidth": 1.75, "fontScale": 1}}, &def)
	var rendered struct {
		Figure struct {
			Ops []map[string]any `json:"Ops"`
		} `json:"figure"`
	}
	c.json("POST", "/api/graphs/render", map[string]any{"definition": map[string]any{"kind": "dls_distribution", "measurements": []string{mid}, "visual": map[string]any{"legend": true}}, "width": 900, "height": 560}, &rendered)
	if len(rendered.Figure.Ops) == 0 {
		t.Fatal("empty figure")
	}
	code, svg := c.do("GET", "/api/graphs/"+def.ID+"/thumb.svg", nil, nil)
	if code != 200 || !bytes.Contains(svg, []byte("<svg")) {
		t.Fatalf("thumb %d", code)
	}

	dest := t.TempDir()
	var ex ExportResult
	req := map[string]any{"items": []map[string]any{{"kind": "graph", "id": def.ID}}, "formats": []string{"png", "csv"}, "preset": "publication", "destination": dest, "name": "A1_DLS"}
	c.json("POST", "/api/export", req, &ex)
	if len(ex.Saved) != 1 || !strings.HasSuffix(ex.Saved[0].Path, ".zip") {
		t.Fatalf("export %+v", ex)
	}
	archiveFile, err := os.Open(ex.Saved[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := archiveFile.Stat()
	if err != nil {
		archiveFile.Close()
		t.Fatal(err)
	}
	integrity, err := export.VerifyPackage(archiveFile, info.Size())
	archiveFile.Close()
	if err != nil || integrity.Manifest.Format != export.PackageFormat || !integrity.WholePayloadCovered || integrity.Authenticated {
		t.Fatal("HTTP export package coverage/authentication", integrity, err)
	}
	// Exporting again does not overwrite silently.
	c.json("POST", "/api/export", req, &ex)
	if ex.Conflict == "" || ex.Suggest == "" {
		t.Fatalf("collision not reported: %+v", ex)
	}
	req["collision"] = "keep_both"
	c.json("POST", "/api/export", req, &ex)
	if len(ex.Saved) != 1 || !strings.Contains(filepath.Base(ex.Saved[0].Path), "(2)") {
		t.Fatalf("keep both: %+v", ex)
	}

	// Presentation state is shared.
	var pr presentation
	c.json("POST", "/api/present", map[string]any{"action": "start", "graphs": []string{def.ID}}, &pr)
	if !pr.Active || len(pr.Graphs) != 1 {
		t.Fatalf("present %+v", pr)
	}

	// Safe exit in Portable USB Mode keeps the drive's data.
	rep := a.Exit()
	if rep.Cleaned {
		t.Fatal("portable exit must not clean")
	}
	if _, err := os.Stat(filepath.Join(a.L.Data, "accounts")); err != nil {
		t.Fatal("portable data removed")
	}
	var m struct {
		Clean bool `json:"clean"`
	}
	atomicfile.ReadJSON(a.markerPath(), &m)
	if !m.Clean {
		t.Fatal("exit not marked clean")
	}
}

func TestMobilePairing(t *testing.T) {
	old := lanIPFunc
	lanIPFunc = func() (net.IP, error) { return net.IPv4(127, 0, 0, 1), nil }
	defer func() { lanIPFunc = old }()

	a, c := portableApp(t)
	res, _ := c.hc.Get(a.srv.LaunchURL("/"))
	res.Body.Close()
	c.json("POST", "/api/account/create", map[string]any{"name": "Lab", "passphrase": "correct horse battery staple"}, nil)
	var pv ProfileView
	c.json("POST", "/api/profiles", map[string]any{"username": "Ana", "storageMode": "usb_only"}, &pv)
	c.json("POST", "/api/profiles/"+pv.ID+"/open", map[string]any{}, nil)

	var info MobileInfo
	c.json("POST", "/api/mobile/start", nil, &info)
	if !info.Active || info.URL == "" || !strings.HasPrefix(info.QR, "<svg") {
		t.Fatalf("mobile info %+v", info)
	}
	u, _ := url.Parse(info.URL)
	frag, _ := url.ParseQuery(u.Fragment)
	pid, kb := frag.Get("p"), frag.Get("k")
	keyBytes, _ := secure.UnB64(kb)
	pk, _ := secure.KeyFromBytes(keyBytes)
	base := "http://" + u.Host
	phone := &http.Client{Timeout: 10 * time.Second}

	// The shell is reachable; the desktop interface and its API are not.
	if r, _ := phone.Get(base + "/m"); r.StatusCode != 200 {
		t.Fatalf("shell %d", r.StatusCode)
	}
	if r, _ := phone.Get(base + "/api/state"); r.StatusCode != 404 {
		t.Fatalf("desktop API exposed on the network: %d", r.StatusCode)
	}

	pair := func(mac string) (int, map[string]string) {
		ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
		if mac == "" {
			mac = secure.B64(macOf(pk[:], "mnelab-pair", pid, ts, "n1", "Phone"))
		}
		body := `{"p":"` + pid + `","ts":` + ts + `,"n":"n1","label":"Phone","mac":"` + mac + `"}`
		r, err := phone.Post(base+"/m/api/pair", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var out map[string]string
		json.NewDecoder(r.Body).Decode(&out)
		return r.StatusCode, out
	}
	// Knowing the address is not enough: a wrong proof is refused.
	if code, _ := pair(secure.B64(make([]byte, 32))); code != http.StatusUnauthorized {
		t.Fatalf("forged pairing accepted: %d", code)
	}
	code, out := pair("")
	if code != 200 {
		t.Fatalf("pair %d %v", code, out)
	}
	sid := out["sid"]
	box, _ := secure.UnB64(out["box"])
	pt, err := secure.Open(pk, box, []byte("mnelab-pair-resp|"+sid))
	if err != nil {
		t.Fatal(err)
	}
	var sk struct {
		Key string `json:"key"`
	}
	json.Unmarshal(pt, &sk)
	skb, _ := secure.UnB64(sk.Key)
	key, _ := secure.KeyFromBytes(skb)
	// The pairing code is single use.
	if code, _ := pair(""); code != http.StatusUnauthorized {
		t.Fatalf("pairing reused: %d", code)
	}

	seq := uint64(0)
	call := func(op string, args any, reuse []byte) (map[string]any, []byte, int) {
		var body []byte
		if reuse != nil {
			body = reuse
		} else {
			seq++
			a, _ := json.Marshal(args)
			req, _ := json.Marshal(map[string]any{"seq": seq, "ts": time.Now().UnixMilli(), "op": op, "args": json.RawMessage(a)})
			body = []byte(secure.B64(secure.Seal(key, req, []byte("mnelab-req|"+sid))))
		}
		r, _ := http.NewRequest("POST", base+"/m/api/call", bytes.NewReader(body))
		r.Header.Set("X-Sid", sid)
		res, err := phone.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 {
			return nil, body, res.StatusCode
		}
		enc, _ := secure.UnB64(string(raw))
		pt, err := secure.Open(key, enc, []byte("mnelab-res|"+sid+"|"+strconv.FormatUint(seq, 10)))
		if err != nil {
			t.Fatal("response not authenticated")
		}
		var v map[string]any
		json.Unmarshal(pt, &v)
		return v, body, 200
	}
	st, body, _ := call("state", nil, nil)
	if st["ok"] != true || st["data"].(map[string]any)["profile"] != "Ana" {
		t.Fatalf("state %v", st)
	}
	if _, _, code := call("", nil, body); code != http.StatusUnauthorized {
		t.Fatalf("replayed request accepted: %d", code)
	}
	if v, _, _ := call("deleteFile", map[string]any{"id": "x"}, nil); v["ok"] != false || v["error"] != ErrMobileOp.Error() {
		t.Fatalf("out-of-scope op: %v", v)
	}
	// Stopping mobile access ends every phone session.
	c.json("POST", "/api/mobile/stop", nil, nil)
	if _, err := phone.Get(base + "/m"); err == nil {
		t.Fatal("listener still open after stop")
	}
}

func macOf(key []byte, parts ...string) []byte {
	m := mac(key, parts...)
	if !hmac.Equal(m, mac(key, parts...)) {
		panic("mac")
	}
	return m
}

func temporaryApp(t *testing.T, base, home string) (*App, *client) {
	a, err := New(Options{Exe: filepath.Join(home, "Downloads", "mnelab"), ForceMode: paths.Temporary, Headless: true, KDF: fastKDF, BaseTemp: base, UserHome: home})
	if err != nil {
		t.Fatal(err)
	}
	ui := fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}}
	a.ui = ui
	srv, err := newServer(a, ui)
	if err != nil {
		t.Fatal(err)
	}
	a.srv = srv
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, base: "http://" + srv.host, hc: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
	res, err := c.hc.Get(srv.LaunchURL("/"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return a, c
}

// A desktop-folder handoff retains encrypted recovery at exit because the
// folder cannot prove a cloud upload. Sign In from the same transport still
// restores the complete encrypted account and original files.
func TestTemporaryModeRoundTrip(t *testing.T) {
	base, home, cloud := t.TempDir(), t.TempDir(), t.TempDir()
	a, c := temporaryApp(t, base, home)
	var modes []string
	var st StateView
	c.json("GET", "/api/state", nil, &st)
	modes = st.StorageModes
	if len(modes) != 1 || modes[0] != "cloud_only" {
		t.Fatalf("temporary storage modes %v", modes)
	}
	var created struct {
		Account AccountView `json:"account"`
	}
	c.json("POST", "/api/account/create", map[string]any{"name": "Lab", "passphrase": "correct horse battery staple"}, &created)
	var conn struct {
		Connection string `json:"connection"`
	}
	c.json("POST", "/api/providers/connect", map[string]any{"provider": "google", "transport": "folder", "folder": cloud}, &conn)
	var pv ProfileView
	c.json("POST", "/api/profiles", map[string]any{"username": "Ana", "storageMode": "cloud_only", "provider": "google", "connection": conn.Connection, "password": "profile pass 1"}, &pv)
	c.json("POST", "/api/profiles/"+pv.ID+"/open", map[string]any{"password": "profile pass 1"}, nil)
	data, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-two-runs.txt"))
	var imp ImportResult
	code, out := c.do("POST", "/api/files", data, map[string]string{"X-File-Name": "runs.txt"})
	json.Unmarshal(out, &imp)
	if code != 200 || imp.Status != "parsed" {
		t.Fatalf("import %d %s", code, out)
	}
	session := a.L.Root
	rep := a.Exit()
	if rep.Verified || !rep.Cleaned || !rep.Recovery {
		t.Fatalf("exit report %+v", rep)
	}
	if _, err := os.Stat(session); !os.IsNotExist(err) {
		t.Fatal("session folder not removed")
	}

	// Another machine: sign in from the cloud.
	b, c2 := temporaryApp(t, t.TempDir(), t.TempDir())
	defer b.Close()
	var found []map[string]any
	c2.json("POST", "/api/signin/start", map[string]any{"provider": "google", "transport": "folder", "folder": cloud}, &found)
	if len(found) != 1 {
		t.Fatalf("accounts found %v", found)
	}
	var av AccountView
	c2.json("POST", "/api/signin/finish", map[string]any{"id": created.Account.ID, "passphrase": "correct horse battery staple"}, &av)
	if len(av.Profiles) != 1 || av.Profiles[0].Username != "Ana" {
		t.Fatalf("profiles not restored %+v", av)
	}
	if code, _ := c2.do("POST", "/api/profiles/"+pv.ID+"/open", map[string]any{"password": "wrong"}, nil); code != http.StatusUnauthorized {
		t.Fatalf("wrong profile password: %d", code)
	}
	c2.json("POST", "/api/profiles/"+pv.ID+"/open", map[string]any{"password": "profile pass 1"}, nil)
	c2.json("POST", "/api/sync", nil, nil)
	var files []FileView
	c2.json("GET", "/api/files", nil, &files)
	if len(files) != 1 || len(files[0].Items) != imp.Measurements {
		t.Fatalf("files after sign in: %+v", files)
	}
	_, orig := c2.do("GET", "/api/files/"+files[0].ID+"/original", nil, nil)
	if !bytes.Equal(orig, data) {
		t.Fatal("original file not restored byte for byte")
	}
}

// Leaving Temporary Mode without internet keeps unsynchronized work as an
// encrypted recovery package outside the cleaned session.
func TestTemporaryOfflineExitKeepsRecovery(t *testing.T) {
	base, home, cloud := t.TempDir(), t.TempDir(), t.TempDir()
	a, c := temporaryApp(t, base, home)
	c.json("POST", "/api/account/create", map[string]any{"name": "Lab", "passphrase": "correct horse battery staple"}, nil)
	var conn struct {
		Connection string `json:"connection"`
	}
	c.json("POST", "/api/providers/connect", map[string]any{"provider": "google", "transport": "folder", "folder": cloud}, &conn)
	var pv ProfileView
	c.json("POST", "/api/profiles", map[string]any{"username": "Ana", "storageMode": "cloud_only", "provider": "google", "connection": conn.Connection}, &pv)
	c.json("POST", "/api/profiles/"+pv.ID+"/open", map[string]any{}, nil)
	// The cloud becomes unreachable.
	os.RemoveAll(cloud)
	os.WriteFile(cloud, []byte("not a folder"), 0o644)
	data, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-tab.txt"))
	c.do("POST", "/api/files", data, map[string]string{"X-File-Name": "a.txt"})
	rep := a.Exit()
	if rep.Verified || !rep.Recovery {
		t.Fatalf("offline exit report %+v", rep)
	}
	if _, err := os.Stat(a.L.Root); !os.IsNotExist(err) {
		t.Fatal("session should still be cleaned after the recovery package is saved")
	}
	// The next run on this machine finds it.
	b, _ := temporaryApp(t, base, home)
	defer b.Close()
	if len(b.Recoveries()) == 0 {
		t.Fatal("recovery not found by the next run")
	}
}
