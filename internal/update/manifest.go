// Package update implements Automatic Update and LockedBuild.
//
// Builds are published with a release manifest signed with Ed25519. The
// application embeds the public key and refuses anything whose signature or
// checksum cannot be confirmed. Update checks send only the version,
// platform, architecture and channel; the updater never touches User Data.
package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Errors (stable identifiers).
var (
	ErrNotConfigured = errors.New("update.not_configured") // build without a release key
	ErrSignature     = errors.New("update.bad_signature")
	ErrManifest      = errors.New("update.bad_manifest")
	ErrChecksum      = errors.New("update.bad_checksum")
	ErrNoAsset       = errors.New("update.no_build_for_platform")
	ErrIncompatible  = errors.New("update.incompatible_data")
	ErrRevoked       = errors.New("update.revoked")
	ErrPortableOnly  = errors.New("update.locked_build_portable_only")
	ErrBusy          = errors.New("update.busy")
)

// ManifestFormat identifies the manifest layout.
const ManifestFormat = "mnelab-releases/1"

// Release-time configuration, set with -ldflags -X.
var (
	// PublicKeys is a comma-separated list of base64 Ed25519 public keys
	// (more than one allows key rotation).
	PublicKeys = ""
	// ManifestURL is where the signed manifest is published.
	ManifestURL = "https://github.com/oaovito/MNE_Lab/releases/latest/download/mnelab-releases.json"
)

// Asset is a downloadable build for one platform.
type Asset struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Kind   string `json:"kind"` // update (application folder), portable, temporary
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// Release is one published build.
type Release struct {
	Version   string            `json:"version"`
	Build     int               `json:"build"`
	Channel   string            `json:"channel"`
	Date      time.Time         `json:"date"`
	Notes     map[string]string `json:"notes,omitempty"` // language → short notes
	Schema    int               `json:"schema"`          // data schema the build writes
	MinSchema int               `json:"minSchema"`       // oldest schema it migrates from
	Security  bool              `json:"security,omitempty"`
	Revoked   bool              `json:"revoked,omitempty"`
	Reason    string            `json:"reason,omitempty"`
	Assets    []Asset           `json:"assets"`
}

// Manifest lists every official build.
type Manifest struct {
	Format    string    `json:"format"`
	Generated time.Time `json:"generated"`
	Releases  []Release `json:"releases"`
}

func keys() ([]ed25519.PublicKey, error) {
	var out []ed25519.PublicKey
	for _, k := range strings.Split(PublicKeys, ",") {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		b, err := base64.StdEncoding.DecodeString(k)
		if err != nil || len(b) != ed25519.PublicKeySize {
			return nil, ErrNotConfigured
		}
		out = append(out, ed25519.PublicKey(b))
	}
	if len(out) == 0 {
		return nil, ErrNotConfigured
	}
	return out, nil
}

// Configured reports whether this build can verify releases.
func Configured() bool {
	_, err := keys()
	return err == nil
}

// Verify checks the detached signature (base64) of the exact manifest
// bytes and parses it.
func Verify(raw, sig []byte) (Manifest, error) {
	ks, err := keys()
	if err != nil {
		return Manifest{}, err
	}
	s, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(s) != ed25519.SignatureSize {
		return Manifest{}, ErrSignature
	}
	ok := false
	for _, k := range ks {
		if ed25519.Verify(k, raw, s) {
			ok = true
			break
		}
	}
	if !ok {
		return Manifest{}, ErrSignature
	}
	return Parse(raw)
}

// Parse decodes and validates a manifest (signature checked separately).
func Parse(raw []byte) (Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil || m.Format != ManifestFormat {
		return Manifest{}, ErrManifest
	}
	for _, r := range m.Releases {
		if _, ok := ParseVersion(r.Version); !ok {
			return Manifest{}, ErrManifest
		}
		for _, a := range r.Assets {
			if len(a.SHA256) != 64 || a.Size <= 0 || !strings.HasPrefix(a.URL, "https://") {
				return Manifest{}, ErrManifest
			}
		}
	}
	return m, nil
}

// Sign signs manifest bytes (release tooling only).
func Sign(priv ed25519.PrivateKey, raw []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, raw))
}

// Version is a parsed semantic version.
type Version struct {
	Major, Minor, Patch int
	Pre                 string
}

// ParseVersion reads "1.2.3" or "1.2.3-beta.1" (optional leading v).
func ParseVersion(s string) (Version, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	var v Version
	if i := strings.IndexByte(s, '-'); i >= 0 {
		v.Pre, s = s[i+1:], s[:i]
	}
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return v, false
	}
	n := make([]int, 3)
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 {
			return v, false
		}
		n[i] = x
	}
	v.Major, v.Minor, v.Patch = n[0], n[1], n[2]
	return v, true
}

// Compare returns -1, 0 or 1 (pre-releases sort before releases).
func Compare(a, b string) int {
	va, oka := ParseVersion(a)
	vb, okb := ParseVersion(b)
	if !oka || !okb {
		return strings.Compare(a, b)
	}
	for _, d := range [][2]int{{va.Major, vb.Major}, {va.Minor, vb.Minor}, {va.Patch, vb.Patch}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case va.Pre == vb.Pre:
		return 0
	case va.Pre == "":
		return 1
	case vb.Pre == "":
		return -1
	}
	return strings.Compare(va.Pre, vb.Pre)
}

// Asset returns the build of a release for a platform and kind.
func (r Release) Asset(goos, arch, kind string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.OS == goos && a.Arch == arch && a.Kind == kind {
			return a, true
		}
	}
	return Asset{}, false
}

// Compatibility explains whether a release can open the current data.
func (r Release) Compatibility(dataSchema int) error {
	if r.Revoked {
		return ErrRevoked
	}
	if dataSchema > r.Schema || dataSchema < r.MinSchema {
		return ErrIncompatible
	}
	return nil
}

// Choice is a build offered in the LockedBuild list.
type Choice struct {
	Release
	Current    bool   `json:"current"`
	Latest     bool   `json:"latest"`
	Newer      bool   `json:"newer"`
	Compatible bool   `json:"compatible"`
	Problem    string `json:"problem,omitempty"`
	Migration  bool   `json:"migration,omitempty"` // data schema changes
}

// Stable lists stable releases for a platform, newest first, hiding
// revoked builds and builds that cannot open the current data.
func Stable(m Manifest, goos, arch, current string, dataSchema int) []Choice {
	var out []Choice
	latest := ""
	for _, r := range m.Releases {
		if r.Channel != "stable" || r.Revoked {
			continue
		}
		if _, ok := r.Asset(goos, arch, "update"); !ok {
			continue
		}
		if latest == "" || Compare(r.Version, latest) > 0 {
			latest = r.Version
		}
		c := Choice{Release: r, Current: Compare(r.Version, current) == 0, Newer: Compare(r.Version, current) > 0}
		if err := r.Compatibility(dataSchema); err != nil {
			continue // destructively incompatible builds are not offered
		}
		c.Compatible = true
		c.Migration = r.Schema != dataSchema
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return Compare(out[i].Version, out[j].Version) > 0 })
	for i := range out {
		out[i].Latest = out[i].Version == latest
	}
	return out
}

// Latest returns the newest stable release that can open the data.
func Latest(m Manifest, goos, arch, current string, dataSchema int) (Release, bool) {
	l := Stable(m, goos, arch, current, dataSchema)
	if len(l) == 0 {
		return Release{}, false
	}
	return l[0].Release, true
}
