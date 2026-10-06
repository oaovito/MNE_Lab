package update

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fixture struct {
	priv     ed25519.PrivateKey
	srv      *httptest.Server
	files    map[string][]byte
	uaSeen   atomic.Value
	builds   string
	restarts chan string // builds the service restarted into
}

func zipOf(t *testing.T, files map[string]string) []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	return b.Bytes()
}

func newFixture(t *testing.T) *fixture {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	old := PublicKeys
	PublicKeys = base64.StdEncoding.EncodeToString(pub)
	t.Cleanup(func() { PublicKeys = old })
	f := &fixture{priv: priv, files: map[string][]byte{}, builds: t.TempDir(), restarts: make(chan string, 4)}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.uaSeen.Store(r.Header.Get("User-Agent") + "|" + r.URL.RawQuery + "|" + r.Header.Get("Cookie") + r.Header.Get("Authorization"))
		b, ok := f.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(b)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fixture) publish(t *testing.T, rels []Release, assets map[string][]byte) {
	for name, b := range assets {
		f.files["/"+name] = b
	}
	m := Manifest{Format: ManifestFormat, Generated: time.Now().UTC(), Releases: rels}
	raw, _ := json.Marshal(m)
	f.files["/mnelab-releases.json"] = raw
	f.files["/mnelab-releases.json.sig"] = []byte(Sign(f.priv, raw))
}

func (f *fixture) asset(name string, body []byte) Asset {
	sum := sha256.Sum256(body)
	return Asset{OS: runtime.GOOS, Arch: runtime.GOARCH, Kind: "update", URL: f.srv.URL + "/" + name, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body))}
}

func (f *fixture) service(portable bool) *Service {
	return &Service{Builds: f.builds, Portable: portable, Version: "1.0.0", Channel: "stable", DataSchema: 1,
		Client: f.srv.Client(), URL: f.srv.URL + "/mnelab-releases.json",
		Hooks: Hooks{Restart: func(exe string) error { f.restarts <- exe; return nil }}}
}

func TestSignatureRequired(t *testing.T) {
	f := newFixture(t)
	f.publish(t, nil, nil)
	raw := f.files["/mnelab-releases.json"]
	if _, err := Verify(raw, f.files["/mnelab-releases.json.sig"]); err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(raw, []byte(ManifestFormat), []byte(ManifestFormat+" "), 1)
	if _, err := Verify(tampered, f.files["/mnelab-releases.json.sig"]); err != ErrSignature {
		t.Fatalf("tampered manifest accepted: %v", err)
	}
	PublicKeys = ""
	if _, err := Fetch(context.Background(), f.srv.Client(), f.srv.URL+"/mnelab-releases.json", "x"); err != ErrNotConfigured {
		t.Fatal("a build without a release key must never install anything")
	}
}

func TestVersions(t *testing.T) {
	cases := [][3]string{{"1.2.3", "1.2.4", "-1"}, {"1.10.0", "1.9.9", "1"}, {"2.0.0-beta.1", "2.0.0", "-1"}, {"v1.0.0", "1.0.0", "0"}}
	for _, c := range cases {
		if got := Compare(c[0], c[1]); (got < 0 && c[2] != "-1") || (got > 0 && c[2] != "1") || (got == 0 && c[2] != "0") {
			t.Fatalf("Compare(%s,%s)=%d", c[0], c[1], got)
		}
	}
}

func TestStableListHidesBadBuilds(t *testing.T) {
	f := newFixture(t)
	a := f.asset("x.zip", []byte("x"))
	m := Manifest{Releases: []Release{
		{Version: "1.0.0", Channel: "stable", Schema: 1, MinSchema: 1, Assets: []Asset{a}},
		{Version: "1.1.0", Channel: "stable", Schema: 1, MinSchema: 1, Revoked: true, Assets: []Asset{a}},
		{Version: "1.2.0", Channel: "stable", Schema: 2, MinSchema: 1, Assets: []Asset{a}},
		{Version: "1.3.0-beta.1", Channel: "beta", Schema: 2, MinSchema: 1, Assets: []Asset{a}},
		{Version: "0.9.0", Channel: "stable", Schema: 0, MinSchema: 0, Assets: []Asset{a}}, // cannot open schema 1 data
	}}
	l := Stable(m, runtime.GOOS, runtime.GOARCH, "1.0.0", 1)
	var vs []string
	for _, c := range l {
		vs = append(vs, c.Version)
	}
	if strings.Join(vs, " ") != "1.2.0 1.0.0" || !l[0].Latest || !l[0].Newer || !l[0].Migration || !l[1].Current {
		t.Fatalf("stable list: %v %+v", vs, l)
	}
	// Data written by schema 2 cannot be opened by 1.0.0: downgrade refused.
	if m.Releases[0].Compatibility(2) != ErrIncompatible {
		t.Fatal("unsafe downgrade allowed")
	}
}

func TestAutoUpdateStagesVerifiesAndActivates(t *testing.T) {
	f := newFixture(t)
	good := zipOf(t, map[string]string{ExeName(): "new build", "notes.txt": "n"})
	f.publish(t, []Release{
		{Version: "1.0.0", Channel: "stable", Schema: 1, MinSchema: 1, Assets: []Asset{f.asset("old.zip", good)}},
		{Version: "1.1.0", Channel: "stable", Schema: 1, MinSchema: 1, Assets: []Asset{f.asset("new.zip", good)}},
	}, map[string][]byte{"new.zip": good, "old.zip": good})
	os.MkdirAll(filepath.Join(f.builds, "1.0.0"), 0o755)
	os.WriteFile(filepath.Join(f.builds, "1.0.0", ExeName()), []byte("old build"), 0o755)
	SaveState(f.builds, State{Current: "1.0.0"})

	s := f.service(false)
	st := s.Check(context.Background())
	if st.Latest != "1.1.0" || !st.Newer || !st.Auto {
		t.Fatalf("status %+v", st)
	}
	var restart string
	select {
	case restart = <-f.restarts:
	case <-time.After(10 * time.Second):
	}
	if restart != filepath.Join(f.builds, "1.1.0", ExeName()) {
		t.Fatalf("restart into %q", restart)
	}
	if b, _ := os.ReadFile(restart); string(b) != "new build" {
		t.Fatal("staged build content")
	}
	state := LoadState(f.builds)
	if state.Current != "1.1.0" || state.Previous != "1.0.0" {
		t.Fatalf("state %+v", state)
	}
	ua := f.uaSeen.Load().(string)
	if !strings.HasPrefix(ua, "MNE-Lab/1.0.0 (") || strings.Count(ua, "|") != 2 || !strings.HasSuffix(ua, "||") {
		t.Fatalf("update requests must carry only version/platform/channel: %q", ua)
	}
}

func TestLockedBuildNeverUpdatesAutomatically(t *testing.T) {
	f := newFixture(t)
	good := zipOf(t, map[string]string{ExeName(): "new"})
	f.publish(t, []Release{{Version: "1.1.0", Channel: "stable", Schema: 1, MinSchema: 1, Assets: []Asset{f.asset("new.zip", good)}}}, map[string][]byte{"new.zip": good})
	tmp := f.service(false)
	if tmp.SetLocked(context.Background(), true) != ErrPortableOnly {
		t.Fatal("LockedBuild must be portable-only")
	}
	s := f.service(true)
	if err := s.SetLocked(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	st := s.Check(context.Background())
	time.Sleep(200 * time.Millisecond)
	if !st.Locked || st.Auto || !st.Newer || len(f.restarts) != 0 || len(Installed(f.builds)) != 0 {
		t.Fatalf("locked build changed: %+v restarts=%d", st, len(f.restarts))
	}
	// Update Build: an explicit choice installs the chosen build.
	if err := s.Choose(context.Background(), "1.1.0"); err != nil {
		t.Fatal(err)
	}
	if !LoadState(f.builds).Locked || LoadState(f.builds).Current != "1.1.0" {
		t.Fatal("chosen build not pinned")
	}
}

func TestStageRejectsBadPackages(t *testing.T) {
	f := newFixture(t)
	good := zipOf(t, map[string]string{ExeName(): "x"})
	slip := zipOf(t, map[string]string{"../evil": "x", ExeName(): "x"})
	bad := f.asset("good.zip", good)
	bad.SHA256 = strings.Repeat("0", 64)
	f.files["/good.zip"] = good
	f.files["/slip.zip"] = slip
	ctx := context.Background()
	if _, err := Stage(ctx, f.srv.Client(), f.builds, Release{Version: "2.0.0"}, bad, "ua", nil); err != ErrChecksum {
		t.Fatalf("checksum mismatch accepted: %v", err)
	}
	if _, err := Stage(ctx, f.srv.Client(), f.builds, Release{Version: "2.0.1"}, f.asset("slip.zip", slip), "ua", nil); err != ErrManifest {
		t.Fatalf("path traversal accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(f.builds), "evil")); err == nil {
		t.Fatal("file written outside the builds folder")
	}
	if len(Installed(f.builds)) != 0 {
		t.Fatal("failed stage left a build")
	}
}

func TestLauncherRollsBackAfterFailedStarts(t *testing.T) {
	b := t.TempDir()
	for _, v := range []string{"1.0.0", "1.1.0"} {
		os.MkdirAll(filepath.Join(b, v), 0o755)
		os.WriteFile(filepath.Join(b, v, ExeName()), []byte(v), 0o755)
	}
	SaveState(b, State{Current: "1.0.0"})
	MarkBootOK(b, "1.0.0")
	Activate(b, "1.1.0")
	for i := 0; i < MaxBootAttempts; i++ {
		if _, v, _ := Resolve(b); v != "1.1.0" {
			t.Fatalf("attempt %d started %s", i, v)
		}
	}
	if _, v, _ := Resolve(b); v != "1.0.0" {
		t.Fatalf("no rollback, started %s", v)
	}
	s := LoadState(b)
	if len(s.Bad) != 1 || s.Bad[0] != "1.1.0" {
		t.Fatalf("bad builds %v", s.Bad)
	}
	// A build that started correctly is never rolled back.
	MarkBootOK(b, "1.0.0")
	for i := 0; i < 5; i++ {
		if _, v, _ := Resolve(b); v != "1.0.0" {
			t.Fatal("good build replaced")
		}
	}
	os.MkdirAll(filepath.Join(b, "0.9.0"), 0o755)
	Activate(b, "1.1.0")
	MarkBootOK(b, "1.1.0")
	Prune(b)
	if got := strings.Join(Installed(b), " "); got != "1.1.0 1.0.0" {
		t.Fatalf("prune must keep the current build and its fallback: %v", got)
	}
}
