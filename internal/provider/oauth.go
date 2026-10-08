package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/oaovito/mne_lab/internal/secure"
)

// OAuthConfig describes an OAuth 2.0 public-client registration.
type OAuthConfig struct {
	AuthURL      string
	TokenURL     string
	RevokeURL    string
	ClientID     string
	ClientSecret string
	Scopes       []string
	// RedirectHost is "127.0.0.1" (Google) or "localhost" (Microsoft).
	RedirectHost string
	// FixedPort pins the loopback port when the provider requires a fixed
	// callback URL (CloudKit). Zero picks a free port.
	FixedPort int
	Extra     map[string]string
}

// Token is an OAuth token set.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	Expiry       time.Time `json:"expiry"`
}

// Valid reports whether the access token is usable for at least a minute.
func (t Token) Valid() bool {
	return t.AccessToken != "" && time.Until(t.Expiry) > time.Minute
}

// Opener opens a URL in the user's browser.
type Opener func(string) error

// callbackPage is the refined page shown in the browser after sign-in.
const callbackPage = `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>MNE Lab</title>
<style>:root{color-scheme:light dark}body{margin:0;min-height:100vh;display:grid;place-items:center;font:15px/1.5 system-ui,-apple-system,"Segoe UI",sans-serif;background:#0b1220;color:#e7ecf5}
.c{max-width:420px;padding:40px 36px;border-radius:18px;background:#121b2e;border:1px solid #23314d;box-shadow:0 20px 60px rgba(0,0,0,.35);text-align:center}
.m{width:44px;height:44px;margin:0 auto 18px;border-radius:12px;display:grid;place-items:center;background:%s}
h1{font-size:19px;margin:0 0 8px;font-weight:600}p{margin:0;color:#9fb0cc}</style></head>
<body><div class="c"><div class="m">%s</div><h1>%s</h1><p>%s</p></div></body></html>`

var (
	okMark  = `<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="#0b1220" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12.5l4.5 4.5L19 7.5"/></svg>`
	errMark = `<svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="#0b1220" stroke-width="2.4" stroke-linecap="round"><path d="M7 7l10 10M17 7L7 17"/></svg>`
)

// Authorize runs the authorization code flow with PKCE (RFC 7636) on a
// loopback redirect (RFC 8252). The provider password is never seen by
// MNE Lab: the user signs in on the provider's own page.
func Authorize(ctx context.Context, cfg OAuthConfig, open Opener) (Token, error) {
	if cfg.ClientID == "" {
		return Token{}, ErrNotConfigured
	}
	host := cfg.RedirectHost
	if host == "" {
		host = "127.0.0.1"
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.FixedPort))
	if err != nil {
		return Token{}, err
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	redirect := fmt.Sprintf("http://%s:%d/", host, port)
	verifier := secure.B64(secure.RandomBytes(32))
	sum := sha256.Sum256([]byte(verifier))
	state := secure.B64(secure.RandomBytes(16))
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {redirect},
		"scope":                 {strings.Join(cfg.Scopes, " ")},
		"state":                 {state},
		"code_challenge":        {secure.B64(sum[:])},
		"code_challenge_method": {"S256"},
	}
	for k, v := range cfg.Extra {
		q.Set(k, v)
	}
	type result struct {
		code string
		err  error
	}
	done := make(chan result, 1)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		v := r.URL.Query()
		var res result
		switch {
		case v.Get("state") != state:
			res.err = errors.New("provider: authorization state mismatch")
		case v.Get("error") != "":
			res.err = ErrCanceled
		case v.Get("code") == "":
			res.err = errors.New("provider: no authorization code")
		default:
			res.code = v.Get("code")
		}
		if res.err == nil {
			fmt.Fprintf(w, callbackPage, "#5eead4", okMark, "Connected", "You can close this tab and return to MNE Lab.")
		} else {
			fmt.Fprintf(w, callbackPage, "#fca5a5", errMark, "Not connected", html.EscapeString("Authorization was not completed. Return to MNE Lab to try again."))
		}
		select {
		case done <- res:
		default:
		}
	})}
	go srv.Serve(ln)
	defer srv.Close()
	if err := open(cfg.AuthURL + "?" + q.Encode()); err != nil {
		return Token{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	select {
	case <-ctx.Done():
		return Token{}, ErrCanceled
	case res := <-done:
		if res.err != nil {
			return Token{}, res.err
		}
		form := url.Values{
			"grant_type":    {"authorization_code"},
			"code":          {res.code},
			"redirect_uri":  {redirect},
			"client_id":     {cfg.ClientID},
			"code_verifier": {verifier},
		}
		if cfg.ClientSecret != "" {
			form.Set("client_secret", cfg.ClientSecret)
		}
		return tokenRequest(ctx, cfg.TokenURL, form)
	}
}

func tokenRequest(ctx context.Context, endpoint string, form url.Values) (Token, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := HTTPClient.Do(req)
	if err != nil {
		if IsNetworkError(err) {
			return Token{}, ErrOffline
		}
		return Token{}, err
	}
	defer resp.Body.Close()
	var body struct {
		Token
		ExpiresIn int    `json:"expires_in"`
		Error     string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Token{}, fmt.Errorf("provider: token response: %w", err)
	}
	if resp.StatusCode != 200 || body.AccessToken == "" {
		if body.Error == "invalid_grant" || resp.StatusCode == 400 || resp.StatusCode == 401 {
			return Token{}, ErrUnauthorized
		}
		return Token{}, ErrUnavailable
	}
	t := body.Token
	if body.ExpiresIn > 0 {
		t.Expiry = time.Now().Add(time.Duration(body.ExpiresIn) * time.Second)
	} else {
		t.Expiry = time.Now().Add(time.Hour)
	}
	return t, nil
}

// tokenSource refreshes access tokens on demand.
type tokenSource struct {
	mu  sync.Mutex
	cfg OAuthConfig
	tok Token
}

func (s *tokenSource) token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tok.Valid() {
		return s.tok.AccessToken, nil
	}
	return s.refreshLocked(ctx)
}

func (s *tokenSource) forceRefresh(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshLocked(ctx)
}

func (s *tokenSource) refreshLocked(ctx context.Context) (string, error) {
	if s.tok.RefreshToken == "" {
		return "", ErrUnauthorized
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {s.tok.RefreshToken}, "client_id": {s.cfg.ClientID}}
	if s.cfg.ClientSecret != "" {
		form.Set("client_secret", s.cfg.ClientSecret)
	}
	if len(s.cfg.Scopes) > 0 && strings.Contains(s.cfg.TokenURL, "microsoft") {
		form.Set("scope", strings.Join(s.cfg.Scopes, " "))
	}
	t, err := tokenRequest(ctx, s.cfg.TokenURL, form)
	if err != nil {
		return "", err
	}
	if t.RefreshToken == "" {
		t.RefreshToken = s.tok.RefreshToken
	}
	s.tok = t
	return t.AccessToken, nil
}

func (s *tokenSource) snapshot() Token {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tok
}

func (s *tokenSource) clear() {
	s.mu.Lock()
	s.tok = Token{}
	s.mu.Unlock()
}

// apiError maps HTTP status codes to stable provider errors.
func apiError(status int, body string) error {
	switch {
	case status == 401:
		return ErrUnauthorized
	case status == 404:
		return ErrNotFound
	case status == 507 || strings.Contains(body, "storageQuotaExceeded") || strings.Contains(body, "quotaLimitReached") || strings.Contains(body, "QUOTA_EXCEEDED"):
		return ErrQuota
	case status == 403 && (strings.Contains(body, "insufficientPermissions") || strings.Contains(body, "accessDenied")):
		return ErrUnauthorized
	default:
		return fmt.Errorf("%w (status %d)", ErrUnavailable, status)
	}
}
