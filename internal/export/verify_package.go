package export

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

var (
	ErrPackageInvalid = errors.New("export.package_invalid")
	ErrPackageLimit   = errors.New("export.package_limit")
)

const (
	packageMaxBytes     = 512 << 20
	packageMaxEntries   = 4096
	packageIndexBytes   = 8 << 20
	packagePathBytes    = 2048
	legacyPackageFormat = "mnelab-package/1"
)

// PackageIntegrity reports correspondence with the package's own checksums.
// An attacker can rewrite both content and checksums. This is never a digital
// signature, a trusted identity, or validation of scientific results.
type PackageIntegrity struct {
	Manifest            Manifest `json:"manifest"`
	CheckedFiles        int      `json:"checkedFiles"`
	WholePayloadCovered bool     `json:"wholePayloadCovered"`
	Uncovered           []string `json:"uncovered,omitempty"`
	Authenticated       bool     `json:"authenticated"`
}

// VerifyPackage reads and hashes without extracting, invoking programs or
// modifying the source. ZIP bytes and total expanded bytes are each bounded
// to 512 MiB, with at most 4096 entries and 8 MiB per manifest/checksum index.
// Legacy /1 README content is explicitly reported as uncovered.
func VerifyPackage(source io.ReaderAt, size int64) (PackageIntegrity, error) {
	fail := func(err error) (PackageIntegrity, error) { return PackageIntegrity{}, err }
	if size < 0 || size > packageMaxBytes {
		return fail(ErrPackageLimit)
	}
	z, err := zip.NewReader(source, size)
	if err != nil {
		return fail(ErrPackageInvalid)
	}
	if len(z.File) > packageMaxEntries {
		return fail(ErrPackageLimit)
	}
	entries := map[string]*zip.File{}
	names := map[string]bool{}
	root := ""
	var expanded uint64
	for _, f := range z.File {
		if len(f.Name) > packagePathBytes {
			return fail(ErrPackageLimit)
		}
		if !safePackagePath(f.Name) || !f.Mode().IsRegular() || f.Flags&1 != 0 {
			return fail(ErrPackageInvalid)
		}
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) != 2 || root != "" && root != parts[0] {
			return fail(ErrPackageInvalid)
		}
		root = parts[0]
		key := strings.ToLower(f.Name)
		if names[key] {
			return fail(ErrPackageInvalid)
		}
		names[key] = true
		if f.UncompressedSize64 > packageMaxBytes-expanded {
			return fail(ErrPackageLimit)
		}
		expanded += f.UncompressedSize64
		entries[parts[1]] = f
	}
	manifestFile, checksumFile := entries["manifest/manifest.json"], entries["manifest/checksums.sha256"]
	if manifestFile == nil || checksumFile == nil {
		return fail(ErrPackageInvalid)
	}
	readIndex := func(f *zip.File) ([]byte, error) {
		if f.UncompressedSize64 > packageIndexBytes {
			return nil, ErrPackageLimit
		}
		r, e := f.Open()
		if e != nil {
			return nil, ErrPackageInvalid
		}
		defer r.Close()
		b, e := io.ReadAll(io.LimitReader(r, packageIndexBytes+1))
		if len(b) > packageIndexBytes {
			return nil, ErrPackageLimit
		}
		if e != nil || uint64(len(b)) != f.UncompressedSize64 {
			return nil, ErrPackageInvalid
		}
		return b, nil
	}
	mb, err := readIndex(manifestFile)
	if err != nil {
		return fail(err)
	}
	var m Manifest
	d := json.NewDecoder(bytes.NewReader(mb))
	d.DisallowUnknownFields()
	if d.Decode(&m) != nil {
		return fail(ErrPackageInvalid)
	}
	canonical, err := json.MarshalIndent(m, "", "  ")
	// Both versions emitted canonical Go JSON. Require those exact bytes to
	// reject duplicate keys, trailing JSON and ambiguous alternate encodings.
	if err != nil || !bytes.Equal(mb, append(canonical, '\n')) {
		return fail(ErrPackageInvalid)
	}
	if m.Format != PackageFormat && m.Format != legacyPackageFormat {
		return fail(ErrPackageInvalid)
	}
	if len(m.Files) > packageMaxEntries-2 {
		return fail(ErrPackageLimit)
	}
	declared := map[string]bool{}
	var expectedSums strings.Builder
	for _, entry := range m.Files {
		key := strings.ToLower(entry.Path)
		if !safePackagePath(entry.Path) || declared[key] || key == "manifest/manifest.json" || key == "manifest/checksums.sha256" {
			return fail(ErrPackageInvalid)
		}
		declared[key] = true
		f := entries[entry.Path]
		if f == nil || entry.Size < 0 || uint64(entry.Size) != f.UncompressedSize64 || len(entry.SHA256) != 64 || strings.ToLower(entry.SHA256) != entry.SHA256 {
			return fail(ErrPackageInvalid)
		}
		if _, e := hex.DecodeString(entry.SHA256); e != nil {
			return fail(ErrPackageInvalid)
		}
		r, e := f.Open()
		if e != nil {
			return fail(ErrPackageInvalid)
		}
		h := sha256.New()
		n, e := io.Copy(h, io.LimitReader(r, entry.Size+1))
		r.Close()
		if e != nil || n != entry.Size || hex.EncodeToString(h.Sum(nil)) != entry.SHA256 {
			return fail(ErrPackageInvalid)
		}
		fmt.Fprintf(&expectedSums, "%s  %s\n", entry.SHA256, entry.Path)
	}
	cb, err := readIndex(checksumFile)
	if err != nil {
		return fail(err)
	}
	if string(cb) != expectedSums.String() {
		return fail(ErrPackageInvalid)
	}
	report := PackageIntegrity{Manifest: m, CheckedFiles: len(m.Files), WholePayloadCovered: true}
	for name, f := range entries {
		if name == "manifest/manifest.json" || name == "manifest/checksums.sha256" || declared[strings.ToLower(name)] {
			continue
		}
		if m.Format != legacyPackageFormat || name != "README.txt" {
			return fail(ErrPackageInvalid)
		}
		// Even an uncovered legacy README must have a valid ZIP stream/CRC.
		r, e := f.Open()
		if e != nil {
			return fail(ErrPackageInvalid)
		}
		n, e := io.Copy(io.Discard, io.LimitReader(r, int64(f.UncompressedSize64)+1))
		r.Close()
		if e != nil || uint64(n) != f.UncompressedSize64 {
			return fail(ErrPackageInvalid)
		}
		report.Uncovered = append(report.Uncovered, name)
		report.WholePayloadCovered = false
	}
	sort.Strings(report.Uncovered)
	return report, nil
}

func safePackagePath(s string) bool {
	if s == "" || len(s) > packagePathBytes || !utf8.ValidString(s) || path.IsAbs(s) || path.Clean(s) != s || strings.ContainsAny(s, "\\:\x00\r\n") {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	for _, part := range strings.Split(s, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}
