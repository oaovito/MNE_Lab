package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeOAuth simulates an authorization server and the user's browser.
type fakeOAuth struct {
	srv      *httptest.Server
	mu       sync.Mutex
	codes    map[string]string // code → challenge
	refresh  map[string]bool
	revoked  []string
	lastAuth url.Values
}

func newFakeOAuth(t *testing.T) *fakeOAuth {
	f := &fakeOAuth{codes: map[string]string{}, refresh: map[string]bool{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.URL.Path {
		case "/token":
			f.mu.Lock()
			defer f.mu.Unlock()
			switch r.Form.Get("grant_type") {
			case "authorization_code":
				ch, ok := f.codes[r.Form.Get("code")]
				if !ok || ch != s256(r.Form.Get("code_verifier")) {
					w.WriteHeader(400)
					fmt.Fprint(w, `{"error":"invalid_grant"}`)
					return
				}
				f.refresh["rt-1"] = true
				fmt.Fprint(w, `{"access_token":"at-1","refresh_token":"rt-1","expires_in":3600}`)
			case "refresh_token":
				if !f.refresh[r.Form.Get("refresh_token")] {
					w.WriteHeader(400)
					fmt.Fprint(w, `{"error":"invalid_grant"}`)
					return
				}
				fmt.Fprint(w, `{"access_token":"at-2","expires_in":3600}`)
			}
		case "/revoke":
			f.mu.Lock()
			f.revoked = append(f.revoked, r.Form.Get("token"))
			delete(f.refresh, r.Form.Get("token"))
			f.mu.Unlock()
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// browser plays the user: it "signs in" and follows the redirect.
func (f *fakeOAuth) browser(u string) error {
	pu, _ := url.Parse(u)
	q := pu.Query()
	f.mu.Lock()
	f.lastAuth = q
	f.codes["code-1"] = q.Get("code_challenge")
	f.mu.Unlock()
	go func() {
		resp, err := http.Get(q.Get("redirect_uri") + "?code=code-1&state=" + url.QueryEscape(q.Get("state")))
		if err == nil {
			resp.Body.Close()
		}
	}()
	return nil
}

func s256(v string) string {
	return b64sha(v)
}

// fakeDrive implements the subset of Drive v3 used by GoogleDrive.
type fakeDrive struct {
	mu     sync.Mutex
	files  map[string]*fakeFile
	next   int
	quota  int
	expire bool
}

type fakeFile struct {
	ID, Name string
	Data     []byte
	Version  int
}

func (d *fakeDrive) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		defer d.mu.Unlock()
		auth := r.Header.Get("Authorization")
		if auth != "Bearer at-1" && auth != "Bearer at-2" || (d.expire && auth == "Bearer at-1") {
			w.WriteHeader(401)
			return
		}
		meta := func(f *fakeFile) map[string]any {
			return map[string]any{"id": f.ID, "name": f.Name, "size": fmt.Sprint(len(f.Data)), "version": fmt.Sprint(f.Version), "modifiedTime": time.Now().UTC()}
		}
		switch {
		case r.URL.Path == "/drive/v3/about":
			json.NewEncoder(w).Encode(map[string]any{"user": map[string]string{"displayName": "Ana Lab", "emailAddress": "ana.lab@example.com"}})
		case r.URL.Path == "/drive/v3/files" && r.Method == "GET":
			if r.URL.Query().Get("spaces") != "appDataFolder" {
				t.Errorf("query outside appDataFolder")
			}
			var list []map[string]any
			q := r.URL.Query().Get("q")
			for _, f := range d.files {
				if strings.HasPrefix(q, "name = ") && !strings.Contains(q, "'"+f.Name+"'") {
					continue
				}
				list = append(list, meta(f))
			}
			json.NewEncoder(w).Encode(map[string]any{"files": list})
		case r.URL.Path == "/upload/drive/v3/files" && r.Method == "POST":
			_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			mr := multipart.NewReader(r.Body, params["boundary"])
			p1, _ := mr.NextPart()
			var m struct {
				Name    string   `json:"name"`
				Parents []string `json:"parents"`
			}
			json.NewDecoder(p1).Decode(&m)
			if len(m.Parents) != 1 || m.Parents[0] != "appDataFolder" {
				t.Errorf("upload outside appDataFolder")
			}
			p2, _ := mr.NextPart()
			data, _ := io.ReadAll(p2)
			if d.quota > 0 && len(data) > d.quota {
				w.WriteHeader(403)
				fmt.Fprint(w, `{"error":{"errors":[{"reason":"storageQuotaExceeded"}]}}`)
				return
			}
			d.next++
			f := &fakeFile{ID: fmt.Sprint("id", d.next), Name: m.Name, Data: data, Version: 1}
			d.files[f.ID] = f
			json.NewEncoder(w).Encode(meta(f))
		case strings.HasPrefix(r.URL.Path, "/upload/drive/v3/files/") && r.Method == "PATCH":
			f := d.files[strings.TrimPrefix(r.URL.Path, "/upload/drive/v3/files/")]
			if f == nil {
				w.WriteHeader(404)
				return
			}
			f.Data, _ = io.ReadAll(r.Body)
			f.Version++
			json.NewEncoder(w).Encode(meta(f))
		case strings.HasPrefix(r.URL.Path, "/drive/v3/files/"):
			id := strings.TrimPrefix(r.URL.Path, "/drive/v3/files/")
			f := d.files[id]
			if f == nil {
				w.WriteHeader(404)
				return
			}
			if r.Method == "DELETE" {
				delete(d.files, id)
				w.WriteHeader(204)
				return
			}
			w.Write(f.Data)
		default:
			w.WriteHeader(404)
		}
	})
}

func TestGoogleDriveEndToEnd(t *testing.T) {
	oa := newFakeOAuth(t)
	drive := &fakeDrive{files: map[string]*fakeFile{}}
	api := httptest.NewServer(drive.handler(t))
	defer api.Close()
	GoogleAuthURL, GoogleTokenURL, GoogleRevokeURL, GoogleAPI = oa.srv.URL+"/auth", oa.srv.URL+"/token", oa.srv.URL+"/revoke", api.URL
	cfg := Config{GoogleClientID: "cid"}
	ctx := context.Background()
	g, err := ConnectGoogle(ctx, cfg, oa.browser)
	if err != nil {
		t.Fatal(err)
	}
	if oa.lastAuth.Get("code_challenge_method") != "S256" || oa.lastAuth.Get("scope") != GoogleScopes[0] {
		t.Fatalf("PKCE or scope missing: %v", oa.lastAuth)
	}
	if g.Info().MaskedEmail != "an••••@e•••.com" || g.Info().DisplayName != "Ana Lab" {
		t.Fatalf("info %+v", g.Info())
	}
	if _, err := g.Write(ctx, "accounts/a/profiles/p/r/x.rec", []byte("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Write(ctx, "accounts/a/profiles/p/r/x.rec", []byte("two")); err != nil {
		t.Fatal(err)
	}
	g.Write(ctx, "accounts/a/profiles/p/b/y.blob", []byte("blob"))
	objs, err := g.List(ctx, "accounts/a/profiles/p/r")
	if err != nil || len(objs) != 1 || objs[0].Revision != "2" {
		t.Fatalf("list %+v %v", objs, err)
	}
	b, err := g.Read(ctx, "accounts/a/profiles/p/r/x.rec")
	if err != nil || string(b) != "two" {
		t.Fatalf("read %q %v", b, err)
	}
	// Access token expiry triggers one refresh, transparently.
	drive.expire = true
	g.ts.tok.Expiry = time.Now().Add(time.Hour)
	if err := g.CheckConnection(ctx); err != nil {
		t.Fatalf("refresh path: %v", err)
	}
	// Quota is reported as such, not as a generic failure.
	drive.quota = 2
	if _, err := g.Write(ctx, "accounts/a/big", []byte("too big")); err != ErrQuota {
		t.Fatalf("quota: %v", err)
	}
	creds, _ := g.Credentials()
	g2, err := RestoreGoogle(cfg, creds)
	if err != nil || g2.Info().DisplayName != "Ana Lab" {
		t.Fatalf("restore: %v", err)
	}
	if err := g.Delete(ctx, "accounts/a/profiles/p/r/x.rec"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Read(ctx, "accounts/a/profiles/p/r/x.rec"); err != ErrNotFound {
		t.Fatalf("after delete: %v", err)
	}
	if err := g.Disconnect(ctx); err != nil || len(oa.revoked) != 1 {
		t.Fatalf("revoke: %v %v", err, oa.revoked)
	}
	// After revocation the stored credentials no longer work.
	g2.ts.tok.Expiry = time.Time{}
	if err := g2.CheckConnection(ctx); err != ErrUnauthorized {
		t.Fatalf("revoked credentials: %v", err)
	}
}

func TestOneDriveAppFolder(t *testing.T) {
	oa := newFakeOAuth(t)
	items := map[string][]byte{}
	var mu sync.Mutex
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer at-") {
			w.WriteHeader(401)
			return
		}
		p := r.URL.EscapedPath()
		switch {
		case p == "/me":
			json.NewEncoder(w).Encode(map[string]string{"displayName": "Bruno", "userPrincipalName": "bruno@contoso.com"})
		case p == "/me/drive/special/approot":
			fmt.Fprint(w, `{"id":"root"}`)
		case strings.HasSuffix(p, ":/children"):
			dir, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(p, "/me/drive/special/approot:/"), ":/children"))
			var vals []map[string]any
			for k, v := range items {
				if strings.HasPrefix(k, dir+"/") && !strings.Contains(k[len(dir)+1:], "/") {
					vals = append(vals, map[string]any{"name": k[len(dir)+1:], "size": len(v), "eTag": "e", "file": map[string]any{}})
				}
			}
			if vals == nil {
				w.WriteHeader(404)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"value": vals})
		case strings.HasSuffix(p, ":/content"):
			name, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(p, "/me/drive/special/approot:/"), ":/content"))
			if r.Method == "PUT" {
				items[name], _ = io.ReadAll(r.Body)
				json.NewEncoder(w).Encode(map[string]any{"size": len(items[name]), "eTag": "e1"})
				return
			}
			if b, ok := items[name]; ok {
				w.Write(b)
				return
			}
			w.WriteHeader(404)
		case r.Method == "DELETE":
			name, _ := url.PathUnescape(strings.TrimPrefix(p, "/me/drive/special/approot:/"))
			delete(items, name)
			w.WriteHeader(204)
		default:
			w.WriteHeader(404)
		}
	}))
	defer api.Close()
	MicrosoftAuthURL, MicrosoftTokenURL, GraphAPI = oa.srv.URL+"/auth", oa.srv.URL+"/token", api.URL
	ctx := context.Background()
	o, err := ConnectOneDrive(ctx, Config{MicrosoftClientID: "cid"}, oa.browser)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(oa.lastAuth.Get("scope"), "Files.ReadWrite.AppFolder") || strings.Contains(oa.lastAuth.Get("scope"), "Files.ReadWrite.All") {
		t.Fatalf("scope: %s", oa.lastAuth.Get("scope"))
	}
	if !strings.HasPrefix(oa.lastAuth.Get("redirect_uri"), "http://localhost:") {
		t.Fatalf("redirect: %s", oa.lastAuth.Get("redirect_uri"))
	}
	if _, err := o.Write(ctx, "accounts/a/r/x y.rec", []byte("v")); err != nil {
		t.Fatal(err)
	}
	objs, err := o.List(ctx, "accounts/a/r")
	if err != nil || len(objs) != 1 || objs[0].Name != "accounts/a/r/x y.rec" {
		t.Fatalf("list %+v %v", objs, err)
	}
	if empty, err := o.List(ctx, "accounts/none"); err != nil || len(empty) != 0 {
		t.Fatalf("missing folder must list empty: %v", err)
	}
	if b, _ := o.Read(ctx, "accounts/a/r/x y.rec"); string(b) != "v" {
		t.Fatal("read")
	}
	if err := o.Delete(ctx, "accounts/a/r/x y.rec"); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizeRejectsStateMismatch(t *testing.T) {
	oa := newFakeOAuth(t)
	cfg := OAuthConfig{AuthURL: oa.srv.URL + "/auth", TokenURL: oa.srv.URL + "/token", ClientID: "c"}
	_, err := Authorize(context.Background(), cfg, func(u string) error {
		pu, _ := url.Parse(u)
		go http.Get(pu.Query().Get("redirect_uri") + "?code=x&state=forged")
		return nil
	})
	if err == nil {
		t.Fatal("forged state must be rejected")
	}
	if _, err := Authorize(context.Background(), OAuthConfig{}, nil); err != ErrNotConfigured {
		t.Fatal("unconfigured provider must say so")
	}
}

func TestFolderTransport(t *testing.T) {
	d := t.TempDir()
	f := NewFolder(OneDrive, d)
	ctx := context.Background()
	if err := f.CheckConnection(ctx); err != nil {
		t.Fatal(err)
	}
	f.Write(ctx, "a/b/c.rec", []byte("x"))
	objs, _ := f.List(ctx, "a/b")
	if len(objs) != 1 || objs[0].Name != "a/b/c.rec" {
		t.Fatalf("%+v", objs)
	}
	if _, err := f.Read(ctx, "../escape"); err == nil {
		t.Fatal("path escape must be refused")
	}
	if _, err := f.Read(ctx, "a/none"); err != ErrNotFound {
		t.Fatal("missing file")
	}
	if !strings.Contains(f.Root(), "Apps") {
		t.Fatal("OneDrive folder transport must use the Apps folder")
	}
	if NewFolder(Google, "/x").Root() == "" {
		t.Fatal("root")
	}
	if err := NewFolder(Google, "/definitely/missing/path").CheckConnection(ctx); err != ErrUnavailable {
		t.Fatal("missing client folder must be unavailable")
	}
}

func TestMaskEmail(t *testing.T) {
	for in, want := range map[string]string{"ana.lab@example.com": "an••••@e•••.com", "jo@uni.br": "j••••@u•••.br", "bad": ""} {
		if got := MaskEmail(in); got != want {
			t.Fatalf("%s → %s want %s", in, got, want)
		}
	}
}
