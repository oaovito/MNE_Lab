// Package provider defines the StorageProvider contract and its
// implementations for the three supported clouds, in their mandatory order:
// Google Drive / Google One, iCloud and Microsoft OneDrive.
//
// MNE Lab talks to the user's own provider directly. There is no MNE Lab
// server in between, and only app-specific storage is requested.
//
// Each provider has up to two transports:
//   - "api": the provider's official web API with OAuth 2.0 + PKCE (Google,
//     Microsoft) or CloudKit Web Services sign-in (Apple). Works on any
//     machine, including Temporary Machine Mode.
//   - "folder": the folder of the provider's official desktop app (Google
//     Drive for desktop, iCloud for Windows/macOS, OneDrive). The app
//     uploads the files; MNE Lab only writes inside its own subfolder.
package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// Provider identifiers in display order.
const (
	Google   = "google"
	ICloud   = "icloud"
	OneDrive = "onedrive"
)

// Order is the mandatory display order.
var Order = []string{Google, ICloud, OneDrive}

// Transports.
const (
	TransportAPI    = "api"
	TransportFolder = "folder"
)

// Stable errors, localized by the interface.
var (
	ErrNotFound      = errors.New("provider.not_found")
	ErrUnauthorized  = errors.New("provider.authorization_revoked")
	ErrQuota         = errors.New("provider.quota_exceeded")
	ErrOffline       = errors.New("provider.offline")
	ErrNotConfigured = errors.New("provider.not_configured")
	ErrUnavailable   = errors.New("provider.unavailable")
	ErrCanceled      = errors.New("provider.authorization_canceled")
)

// Object is a remote file.
type Object struct {
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	Revision string    `json:"revision"`
	Modified time.Time `json:"modified"`
}

// AccountInfo identifies the connected cloud account with minimal data.
type AccountInfo struct {
	DisplayName string `json:"displayName,omitempty"`
	MaskedEmail string `json:"maskedEmail,omitempty"`
}

// Provider is the common contract. Names are slash-separated paths relative
// to MNE Lab's app-specific storage area.
type Provider interface {
	ID() string
	Transport() string
	// CheckConnection verifies reachability and authorization.
	CheckConnection(ctx context.Context) error
	Info() AccountInfo
	// List returns the files directly inside dir ("" is the root). It never
	// returns subfolders.
	List(ctx context.Context, dir string) ([]Object, error)
	Read(ctx context.Context, name string) ([]byte, error)
	Write(ctx context.Context, name string, data []byte) (Object, error)
	Delete(ctx context.Context, name string) error
	// Disconnect revokes authorization where the provider supports it and
	// forgets local credentials.
	Disconnect(ctx context.Context) error
	// Credentials serializes what is needed to reconnect (sealed by the
	// caller with the profile key; never logged, never sent elsewhere).
	Credentials() ([]byte, error)
}

// MaskEmail keeps just enough of an address to tell accounts apart.
func MaskEmail(e string) string {
	at := strings.LastIndex(e, "@")
	if at <= 0 {
		return ""
	}
	local, domain := e[:at], e[at+1:]
	keep := 1
	if len(local) > 4 {
		keep = 2
	}
	dot := strings.LastIndex(domain, ".")
	tld := ""
	if dot > 0 {
		tld = domain[dot:]
		domain = domain[:dot]
	}
	if len(domain) > 1 {
		domain = domain[:1] + strings.Repeat("•", 3)
	}
	return local[:keep] + strings.Repeat("•", 4) + "@" + domain + tld
}

// HTTPClient is the client used by API transports. It honors the system
// proxy and never sends cookies.
var HTTPClient = &http.Client{
	Timeout: 60 * time.Second,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          4,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 45 * time.Second,
	},
}

// IsNetworkError reports whether err looks like missing connectivity.
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrOffline) {
		return true
	}
	s := err.Error()
	for _, m := range []string{"no such host", "connection refused", "network is unreachable", "i/o timeout", "dial tcp", "TLS handshake timeout", "connection reset"} {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}
