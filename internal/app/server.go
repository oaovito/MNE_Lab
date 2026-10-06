package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oaovito/mne_lab/internal/secure"
)

// Server serves the interface and its API on 127.0.0.1 only. Access needs
// a session cookie obtained through a single-use code that only the
// window launched by MNE Lab receives; requests with a foreign Host or
// Origin are refused (DNS rebinding and cross-site requests).
type Server struct {
	app   *App
	ln    net.Listener
	srv   *http.Server
	mux   *http.ServeMux
	ui    fs.FS
	host  string
	mu    sync.Mutex
	codes map[string]time.Time
	sess  map[string]bool
	focus string // token a second launch uses to bring the window back
}

const sessionCookie = "mnelab_s"

// errorStatus maps stable error identifiers to HTTP status codes.
func errorStatus(err error) int {
	s := err.Error()
	switch {
	case strings.HasPrefix(s, "auth."), strings.HasSuffix(s, ".wrong_secret"), strings.HasSuffix(s, "wrong_password"):
		return http.StatusUnauthorized
	case strings.HasSuffix(s, "not_found"), strings.Contains(s, "not found"):
		return http.StatusNotFound
	case strings.HasSuffix(s, "exists"), strings.Contains(s, "conflict"), strings.HasSuffix(s, "limit_reached"), strings.HasSuffix(s, ".busy"):
		return http.StatusConflict
	case strings.HasPrefix(s, "session.locked"), strings.HasPrefix(s, "profile.locked"):
		return http.StatusForbidden
	case errors.Is(err, ErrExiting):
		return http.StatusServiceUnavailable
	}
	if strings.Count(s, ".") >= 1 && !strings.ContainsAny(s, " :") {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func newServer(a *App, ui fs.FS) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{app: a, ln: ln, ui: ui, mux: http.NewServeMux(), codes: map[string]time.Time{}, sess: map[string]bool{}, focus: secure.Token()}
	s.host = ln.Addr().String()
	s.srv = &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second}
	s.routes()
	go s.srv.Serve(ln)
	return s, nil
}

// Port is the loopback port.
func (s *Server) Port() int { return s.ln.Addr().(*net.TCPAddr).Port }

// Close stops the server.
func (s *Server) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	s.srv.Shutdown(ctx)
}

// LaunchURL returns a URL with a fresh single-use code for a window.
func (s *Server) LaunchURL(path string) string {
	code := secure.Token()
	s.mu.Lock()
	for c, exp := range s.codes {
		if time.Now().After(exp) {
			delete(s.codes, c)
		}
	}
	s.codes[code] = time.Now().Add(2 * time.Minute)
	s.mu.Unlock()
	if path == "" {
		path = "/"
	}
	return "http://" + s.host + "/auth?code=" + code + "&to=" + path
}

func (s *Server) authorized(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sess[c.Value]
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Host != s.host {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	switch {
	case r.URL.Path == "/auth":
		s.handleAuth(w, r)
		return
	case r.URL.Path == "/focus" && r.Method == http.MethodPost:
		// A second launch asks the running instance to show its window.
		if secure.EqualStrings(r.Header.Get("X-Focus"), s.focus) {
			go s.app.ShowWindow("")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		if !s.authorized(r) {
			writeErr(w, errors.New("auth.session_required"), http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if o := r.Header.Get("Origin"); o != "" && o != "http://"+s.host {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if r.Header.Get("X-MNE-Lab") != "1" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		h.Set("Cache-Control", "no-store")
		s.mux.ServeHTTP(w, r)
		return
	}
	if !s.authorized(r) {
		// The interface itself is only served to the authorized window.
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		io.WriteString(w, "<!doctype html><meta charset=utf-8><title>MNE Lab</title><body style=\"font-family:system-ui;background:#0E1116;color:#C9D1DB;display:grid;place-items:center;height:100vh;margin:0\">MNE Lab")
		return
	}
	s.serveUI(w, r)
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	s.mu.Lock()
	exp, ok := s.codes[code]
	delete(s.codes, code)
	s.mu.Unlock()
	if !ok || time.Now().After(exp) {
		http.Error(w, "expired", http.StatusUnauthorized)
		return
	}
	tok := secure.Token()
	s.mu.Lock()
	s.sess[tok] = true
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	to := r.URL.Query().Get("to")
	if !strings.HasPrefix(to, "/") || strings.HasPrefix(to, "//") {
		to = "/"
	}
	http.Redirect(w, r, to, http.StatusFound)
}

func (s *Server) serveUI(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	if st, err := fs.Stat(s.ui, p); err != nil || st.IsDir() {
		p = "index.html" // client-side routes
	}
	if strings.HasPrefix(p, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFileFS(w, r, s.ui, p)
}

// Handler helpers.

type handler func(w http.ResponseWriter, r *http.Request) (any, error)

func (s *Server) handle(pattern string, h handler) {
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if s.app.exiting.Load() && !strings.Contains(pattern, "/api/app/") && !strings.Contains(pattern, "/api/state") && !strings.Contains(pattern, "/api/events") {
			writeErr(w, ErrExiting, http.StatusServiceUnavailable)
			return
		}
		v, err := h(w, r)
		if err != nil {
			writeErr(w, err, errorStatus(err))
			return
		}
		if v == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if raw, ok := v.(rawResponse); ok {
			w.Header().Set("Content-Type", raw.mime)
			if raw.name != "" {
				w.Header().Set("Content-Disposition", contentDisposition(raw.name))
			}
			w.Write(raw.data)
			return
		}
		writeJSON(w, http.StatusOK, v)
	})
}

type rawResponse struct {
	mime string
	name string
	data []byte
}

func contentDisposition(name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return `attachment; filename="` + ascii + `"; filename*=UTF-8''` + urlEscape(name)
}

func urlEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || strings.IndexByte("-._~", c) >= 0 {
			b.WriteByte(c)
		} else {
			b.WriteString("%" + strings.ToUpper(strconv.FormatInt(int64(c)>>4, 16)+strconv.FormatInt(int64(c)&15, 16)))
		}
	}
	return b.String()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error, code int) {
	msg := err.Error()
	if code == http.StatusInternalServerError {
		msg = "app.internal_error"
	}
	writeJSON(w, code, map[string]string{"error": msg})
}

// decode reads a JSON body (1 MiB limit).
func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(v); err != nil && err != io.EOF {
		return errors.New("request.invalid_json")
	}
	return nil
}
