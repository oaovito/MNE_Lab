package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Google endpoints (overridable in tests).
var (
	GoogleAuthURL   = "https://accounts.google.com/o/oauth2/v2/auth"
	GoogleTokenURL  = "https://oauth2.googleapis.com/token"
	GoogleRevokeURL = "https://oauth2.googleapis.com/revoke"
	GoogleAPI       = "https://www.googleapis.com"
)

// GoogleScopes requests only the hidden application data folder: MNE Lab
// cannot see any other file in the user's Drive.
var GoogleScopes = []string{"https://www.googleapis.com/auth/drive.appdata"}

// GoogleDrive stores files in the Drive appDataFolder of Google Drive /
// Google One (Google One plans are Drive storage; there is no separate
// file system).
type GoogleDrive struct {
	ts    *tokenSource
	info  AccountInfo
	mu    sync.Mutex
	files map[string]driveFile // name → file, refreshed by List
}

type driveFile struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Size     string    `json:"size"`
	Version  string    `json:"version"`
	Modified time.Time `json:"modifiedTime"`
}

func googleOAuth(c Config) OAuthConfig {
	return OAuthConfig{AuthURL: GoogleAuthURL, TokenURL: GoogleTokenURL, RevokeURL: GoogleRevokeURL,
		ClientID: c.GoogleClientID, ClientSecret: c.GoogleClientSecret, Scopes: GoogleScopes,
		RedirectHost: "127.0.0.1", Extra: map[string]string{"access_type": "offline", "prompt": "consent"}}
}

// ConnectGoogle signs the user in through Google's own page.
func ConnectGoogle(ctx context.Context, c Config, open Opener) (*GoogleDrive, error) {
	cfg := googleOAuth(c)
	tok, err := Authorize(ctx, cfg, open)
	if err != nil {
		return nil, err
	}
	g := &GoogleDrive{ts: &tokenSource{cfg: cfg, tok: tok}, files: map[string]driveFile{}}
	return g, g.loadInfo(ctx)
}

// RestoreGoogle rebuilds a connection from stored credentials.
func RestoreGoogle(c Config, creds []byte) (*GoogleDrive, error) {
	var s struct {
		Token Token       `json:"token"`
		Info  AccountInfo `json:"info"`
	}
	if err := json.Unmarshal(creds, &s); err != nil {
		return nil, err
	}
	return &GoogleDrive{ts: &tokenSource{cfg: googleOAuth(c), tok: s.Token}, info: s.Info, files: map[string]driveFile{}}, nil
}

func (g *GoogleDrive) ID() string        { return Google }
func (g *GoogleDrive) Transport() string { return TransportAPI }
func (g *GoogleDrive) Info() AccountInfo { return g.info }

func (g *GoogleDrive) Credentials() ([]byte, error) {
	return json.Marshal(map[string]any{"transport": TransportAPI, "token": g.ts.snapshot(), "info": g.info})
}

func (g *GoogleDrive) do(ctx context.Context, method, u string, body []byte, ctype string) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := g.ts.token(ctx)
		if attempt == 1 {
			tok, err = g.ts.forceRefresh(ctx)
		}
		if err != nil {
			return nil, err
		}
		req, _ := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		resp, err := HTTPClient.Do(req)
		if err != nil {
			if IsNetworkError(err) {
				return nil, ErrOffline
			}
			return nil, err
		}
		b, rerr := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if resp.StatusCode == 401 && attempt == 0 {
			continue
		}
		if resp.StatusCode >= 300 {
			return nil, apiError(resp.StatusCode, string(b))
		}
		return b, rerr
	}
	return nil, ErrUnauthorized
}

func (g *GoogleDrive) loadInfo(ctx context.Context) error {
	b, err := g.do(ctx, "GET", GoogleAPI+"/drive/v3/about?fields=user(displayName,emailAddress)", nil, "")
	if err != nil {
		return err
	}
	var a struct {
		User struct {
			DisplayName  string `json:"displayName"`
			EmailAddress string `json:"emailAddress"`
		} `json:"user"`
	}
	json.Unmarshal(b, &a)
	g.info = AccountInfo{DisplayName: a.User.DisplayName, MaskedEmail: MaskEmail(a.User.EmailAddress)}
	return nil
}

func (g *GoogleDrive) CheckConnection(ctx context.Context) error {
	_, err := g.do(ctx, "GET", GoogleAPI+"/drive/v3/files?spaces=appDataFolder&pageSize=1&fields=files(id)", nil, "")
	return err
}

func (g *GoogleDrive) refresh(ctx context.Context) error {
	files := map[string]driveFile{}
	page := ""
	for {
		q := url.Values{"spaces": {"appDataFolder"}, "pageSize": {"1000"}, "q": {"trashed = false"},
			"fields": {"nextPageToken,files(id,name,size,version,modifiedTime)"}}
		if page != "" {
			q.Set("pageToken", page)
		}
		b, err := g.do(ctx, "GET", GoogleAPI+"/drive/v3/files?"+q.Encode(), nil, "")
		if err != nil {
			return err
		}
		var r struct {
			Next  string      `json:"nextPageToken"`
			Files []driveFile `json:"files"`
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return err
		}
		for _, f := range r.Files {
			files[f.Name] = f
		}
		if r.Next == "" {
			break
		}
		page = r.Next
	}
	g.mu.Lock()
	g.files = files
	g.mu.Unlock()
	return nil
}

func (g *GoogleDrive) List(ctx context.Context, dir string) ([]Object, error) {
	if err := g.refresh(ctx); err != nil {
		return nil, err
	}
	prefix := strings.TrimSuffix(dir, "/")
	if prefix != "" {
		prefix += "/"
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []Object
	for name, f := range g.files {
		if !strings.HasPrefix(name, prefix) || strings.Contains(name[len(prefix):], "/") {
			continue
		}
		size, _ := strconv.ParseInt(f.Size, 10, 64)
		out = append(out, Object{Name: name, Size: size, Revision: f.Version, Modified: f.Modified})
	}
	return out, nil
}

func (g *GoogleDrive) lookup(ctx context.Context, name string) (driveFile, error) {
	g.mu.Lock()
	f, ok := g.files[name]
	g.mu.Unlock()
	if ok {
		return f, nil
	}
	q := url.Values{"spaces": {"appDataFolder"}, "fields": {"files(id,name,size,version,modifiedTime)"},
		"q": {fmt.Sprintf("name = '%s' and trashed = false", strings.ReplaceAll(strings.ReplaceAll(name, `\`, `\\`), "'", `\'`))}}
	b, err := g.do(ctx, "GET", GoogleAPI+"/drive/v3/files?"+q.Encode(), nil, "")
	if err != nil {
		return f, err
	}
	var r struct {
		Files []driveFile `json:"files"`
	}
	json.Unmarshal(b, &r)
	if len(r.Files) == 0 {
		return f, ErrNotFound
	}
	g.mu.Lock()
	g.files[name] = r.Files[0]
	g.mu.Unlock()
	return r.Files[0], nil
}

func (g *GoogleDrive) Read(ctx context.Context, name string) ([]byte, error) {
	f, err := g.lookup(ctx, name)
	if err != nil {
		return nil, err
	}
	return g.do(ctx, "GET", GoogleAPI+"/drive/v3/files/"+url.PathEscape(f.ID)+"?alt=media", nil, "")
}

func (g *GoogleDrive) Write(ctx context.Context, name string, data []byte) (Object, error) {
	fields := "id,name,size,version,modifiedTime"
	var b []byte
	f, err := g.lookup(ctx, name)
	switch {
	case err == nil:
		b, err = g.do(ctx, "PATCH", GoogleAPI+"/upload/drive/v3/files/"+url.PathEscape(f.ID)+"?uploadType=media&fields="+fields, data, "application/octet-stream")
	case errors.Is(err, ErrNotFound):
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		h := textproto.MIMEHeader{"Content-Type": {"application/json; charset=UTF-8"}}
		pw, _ := mw.CreatePart(h)
		json.NewEncoder(pw).Encode(map[string]any{"name": name, "parents": []string{"appDataFolder"}})
		pw, _ = mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"application/octet-stream"}})
		pw.Write(data)
		mw.Close()
		b, err = g.do(ctx, "POST", GoogleAPI+"/upload/drive/v3/files?uploadType=multipart&fields="+fields, body.Bytes(), "multipart/related; boundary="+mw.Boundary())
	}
	if err != nil {
		return Object{}, err
	}
	var nf driveFile
	if err := json.Unmarshal(b, &nf); err != nil {
		return Object{}, err
	}
	g.mu.Lock()
	g.files[name] = nf
	g.mu.Unlock()
	size, _ := strconv.ParseInt(nf.Size, 10, 64)
	return Object{Name: name, Size: size, Revision: nf.Version, Modified: nf.Modified}, nil
}

func (g *GoogleDrive) Delete(ctx context.Context, name string) error {
	f, err := g.lookup(ctx, name)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = g.do(ctx, "DELETE", GoogleAPI+"/drive/v3/files/"+url.PathEscape(f.ID), nil, "")
	if errors.Is(err, ErrNotFound) {
		err = nil
	}
	g.mu.Lock()
	delete(g.files, name)
	g.mu.Unlock()
	return err
}

// Disconnect revokes the refresh token at Google and forgets it locally.
func (g *GoogleDrive) Disconnect(ctx context.Context) error {
	t := g.ts.snapshot()
	defer g.ts.clear()
	tok := t.RefreshToken
	if tok == "" {
		tok = t.AccessToken
	}
	if tok == "" {
		return nil
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", g.ts.cfg.RevokeURL, strings.NewReader(url.Values{"token": {tok}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := HTTPClient.Do(req)
	if err != nil {
		return ErrOffline
	}
	resp.Body.Close()
	return nil
}
