package export

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"
)

// Package folders of a Complete Research Package.
const (
	DirManifest   = "manifest"
	DirMetadata   = "metadata"
	DirOriginals  = "original-files"
	DirNormalized = "normalized-data"
	DirGraphs     = "graphs"
	DirStatistics = "statistics"
	DirProvenance = "provenance"
)

// PackageFormat identifies the package layout version.
const PackageFormat = "mnelab-package/2"

// ManifestEntry describes one file inside a package.
type ManifestEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	Role   string `json:"role"`
}

// Manifest lists the contents of a package so it can be audited and
// reproduced later. It contains no personal or device information.
type Manifest struct {
	Format   string            `json:"format"`
	Kind     string            `json:"kind"`
	Title    string            `json:"title"`
	Software string            `json:"software"`
	Schema   int               `json:"schema"`
	Created  time.Time         `json:"created"`
	Versions map[string]string `json:"versions,omitempty"` // parser, spec, engine
	Files    []ManifestEntry   `json:"files"`
}

// Archive collects files for a ZIP container (research package or batch).
type Archive struct {
	Root    string
	Kind    string
	Title   string
	Created time.Time
	App     string
	Schema  int
	Readme  string
	files   []archFile
	names   map[string]bool
	Version map[string]string
}

type archFile struct {
	path string
	role string
	data []byte
}

// NewArchive starts a container whose files live under root/.
func NewArchive(root, kind, title, app string, schema int, created time.Time) *Archive {
	return &Archive{Root: Sanitize(root), Kind: kind, Title: title, App: app, Schema: schema, Created: created.UTC(), names: map[string]bool{}, Version: map[string]string{}}
}

// Add stores a file at dir/name (the name is sanitized and made unique)
// and returns its path inside the container.
func (a *Archive) Add(dir, name, role string, data []byte) string {
	ext := path.Ext(name)
	base := Sanitize(strings.TrimSuffix(name, ext))
	p := path.Join(dir, base+ext)
	if a.names[strings.ToLower(p)] {
		p = path.Join(dir, KeepBothName(base+ext, func(n string) bool { return a.names[strings.ToLower(path.Join(dir, n))] }))
	}
	a.names[strings.ToLower(p)] = true
	a.files = append(a.files, archFile{p, role, data})
	return p
}

// AddJSON stores an indented JSON document.
func (a *Archive) AddJSON(dir, name, role string, v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return a.Add(dir, name, role, append(b, '\n')), nil
}

// Size estimates the uncompressed size for the disk space check.
func (a *Archive) Size() int64 {
	var n int64
	for _, f := range a.files {
		n += int64(len(f.data))
	}
	return n + int64(len(a.files))*512 + 64<<10
}

// Len is the number of files added.
func (a *Archive) Len() int { return len(a.files) }

// WriteZip writes the ZIP: README, the files, the manifest and a checksum
// list compatible with "sha256sum -c".
func (a *Archive) WriteZip(w io.Writer) (Manifest, error) {
	m := Manifest{Format: PackageFormat, Kind: a.Kind, Title: a.Title, Software: a.App, Schema: a.Schema, Created: a.Created, Versions: a.Version}
	if !safePackagePath(a.Root) || strings.Contains(a.Root, "/") {
		return m, ErrPackageInvalid
	}
	zw := zip.NewWriter(w)
	put := func(p string, data []byte, method uint16) error {
		h := &zip.FileHeader{Name: a.Root + "/" + p, Method: method, Modified: a.Created}
		h.SetMode(0o644)
		fw, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = fw.Write(data)
		return err
	}
	files := append([]archFile(nil), a.files...)
	if a.Readme != "" {
		files = append(files, archFile{"README.txt", "readme", []byte(a.Readme)})
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].path < files[j].path })
	names := map[string]bool{}
	for _, f := range files {
		key := strings.ToLower(f.path)
		if !safePackagePath(f.path) || names[key] || key == "manifest/manifest.json" || key == "manifest/checksums.sha256" {
			return m, ErrPackageInvalid
		}
		names[key] = true
	}
	var sums strings.Builder
	for _, f := range files {
		method := uint16(zip.Deflate)
		switch strings.ToLower(path.Ext(f.path)) {
		case ".png", ".jpg", ".jpeg", ".webp", ".zip", ".xlsx", ".gif":
			method = zip.Store // already compressed
		}
		if err := put(f.path, f.data, method); err != nil {
			return m, err
		}
		sum := sha256.Sum256(f.data)
		hx := hex.EncodeToString(sum[:])
		m.Files = append(m.Files, ManifestEntry{Path: f.path, Size: int64(len(f.data)), SHA256: hx, Role: f.role})
		fmt.Fprintf(&sums, "%s  %s\n", hx, f.path)
	}
	mb, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return m, err
	}
	if err := put(path.Join(DirManifest, "manifest.json"), append(mb, '\n'), zip.Deflate); err != nil {
		return m, err
	}
	if err := put(path.Join(DirManifest, "checksums.sha256"), []byte(sums.String()), zip.Deflate); err != nil {
		return m, err
	}
	return m, zw.Close()
}
