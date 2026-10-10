package export

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func packageFixture(t *testing.T, version string) []byte {
	t.Helper()
	b, e := os.ReadFile("../../testdata/package-integrity/invented-v" + version + ".zip")
	if e != nil {
		t.Fatal(e)
	}
	return b
}

type packagePart struct {
	name string
	data []byte
	mode os.FileMode
}

func packageParts(t *testing.T, b []byte) []packagePart {
	t.Helper()
	z, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if e != nil {
		t.Fatal(e)
	}
	p := []packagePart{}
	for _, f := range z.File {
		r, e := f.Open()
		if e != nil {
			t.Fatal(e)
		}
		v, e := io.ReadAll(r)
		r.Close()
		if e != nil {
			t.Fatal(e)
		}
		p = append(p, packagePart{f.Name, v, f.Mode()})
	}
	return p
}
func packageBytes(t *testing.T, parts []packagePart) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, p := range parts {
		h := &zip.FileHeader{Name: p.name, Method: zip.Store}
		h.SetMode(p.mode)
		w, e := z.CreateHeader(h)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write(p.data); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestVerifyPackageIndependentFixtures(t *testing.T) {
	for _, version := range []string{"1", "2"} {
		t.Run(version, func(t *testing.T) {
			b := packageFixture(t, version)
			original := append([]byte{}, b...)
			r, e := VerifyPackage(bytes.NewReader(b), int64(len(b)))
			if e != nil {
				t.Fatal(e)
			}
			if r.Authenticated || r.Manifest.Format != "mnelab-package/"+version || !bytes.Equal(b, original) {
				t.Fatal("identity/source changed")
			}
			if version == "1" {
				if r.CheckedFiles != 1 || r.WholePayloadCovered || len(r.Uncovered) != 1 || r.Uncovered[0] != "README.txt" {
					t.Fatal(r)
				}
			} else if r.CheckedFiles != 2 || !r.WholePayloadCovered || len(r.Uncovered) != 0 {
				t.Fatal(r)
			}
		})
	}
}
func TestPackageWriterCoversREADME(t *testing.T) {
	a := NewArchive("Invented", "dataset", "Invented", "MNE Lab synthetic", 1, time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC))
	a.Readme = "Invented README\n"
	a.Add(DirOriginals, "invented.txt", "original", []byte("unchanged"))
	var b bytes.Buffer
	m, e := a.WriteZip(&b)
	if e != nil {
		t.Fatal(e)
	}
	r, e := VerifyPackage(bytes.NewReader(b.Bytes()), int64(b.Len()))
	if e != nil || !r.WholePayloadCovered || r.Authenticated || len(m.Files) != 2 {
		t.Fatal(r, e)
	}
	if m.Files[0].Path != "README.txt" || m.Files[0].Role != "readme" {
		t.Fatal("README missing from manifest")
	}
}
func TestVerifyPackageRejectsTampering(t *testing.T) {
	base := packageFixture(t, "2")
	for name, mutate := range map[string]func([]packagePart) []packagePart{
		"modified README": func(p []packagePart) []packagePart {
			for i := range p {
				if strings.HasSuffix(p[i].name, "/README.txt") {
					p[i].data = []byte("changed")
				}
			}
			return p
		},
		"modified original": func(p []packagePart) []packagePart { p[len(p)-1].data = []byte("changed"); return p },
		"missing original":  func(p []packagePart) []packagePart { return p[:len(p)-1] },
		"extra file": func(p []packagePart) []packagePart {
			return append(p, packagePart{"InventedPackage/extra.txt", []byte("extra"), 0600})
		},
		"duplicate": func(p []packagePart) []packagePart { return append(p, p[0]) },
		"case duplicate": func(p []packagePart) []packagePart {
			q := p[0]
			q.name = "InventedPackage/" + strings.ToUpper(strings.TrimPrefix(q.name, "InventedPackage/"))
			return append(p, q)
		},
		"traversal":      func(p []packagePart) []packagePart { p[0].name = "InventedPackage/../escape.txt"; return p },
		"absolute":       func(p []packagePart) []packagePart { p[0].name = "/InventedPackage/README.txt"; return p },
		"backslash":      func(p []packagePart) []packagePart { p[0].name = "InventedPackage\\README.txt"; return p },
		"multiple roots": func(p []packagePart) []packagePart { p[0].name = "Other/README.txt"; return p },
		"symlink":        func(p []packagePart) []packagePart { p[0].mode = os.ModeSymlink | 0777; return p },
		"checksum mismatch": func(p []packagePart) []packagePart {
			for i := range p {
				if strings.HasSuffix(p[i].name, "checksums.sha256") {
					p[i].data = []byte("wrong\n")
				}
			}
			return p
		},
		"duplicate JSON key": func(p []packagePart) []packagePart {
			for i := range p {
				if strings.HasSuffix(p[i].name, "manifest.json") {
					p[i].data = bytes.Replace(p[i].data, []byte("{\n"), []byte("{\n  \"format\": \"mnelab-package/2\",\n"), 1)
				}
			}
			return p
		},
		"trailing JSON": func(p []packagePart) []packagePart {
			for i := range p {
				if strings.HasSuffix(p[i].name, "manifest.json") {
					p[i].data = append(p[i].data, []byte("{}")...)
				}
			}
			return p
		},
		"uncovered version2 README": func(p []packagePart) []packagePart {
			old := packageParts(t, packageFixture(t, "1"))
			for i := range old {
				if strings.HasSuffix(old[i].name, "manifest.json") {
					old[i].data = bytes.Replace(old[i].data, []byte("mnelab-package/1"), []byte("mnelab-package/2"), 1)
				}
			}
			return old
		},
	} {
		t.Run(name, func(t *testing.T) {
			b := packageBytes(t, mutate(packageParts(t, base)))
			r, e := VerifyPackage(bytes.NewReader(b), int64(len(b)))
			if !errors.Is(e, ErrPackageInvalid) || r.CheckedFiles != 0 || r.Authenticated {
				t.Fatal("unsafe/partial success", r, e)
			}
		})
	}
}
func TestVerifyPackageLimitsAndCRC(t *testing.T) {
	t.Run("path size", func(t *testing.T) {
		p := packageParts(t, packageFixture(t, "2"))
		p[0].name = "InventedPackage/" + strings.Repeat("x", packagePathBytes)
		b := packageBytes(t, p)
		if _, e := VerifyPackage(bytes.NewReader(b), int64(len(b))); !errors.Is(e, ErrPackageLimit) {
			t.Fatal(e)
		}
	})
	t.Run("entry count", func(t *testing.T) {
		p := make([]packagePart, packageMaxEntries+1)
		for i := range p {
			p[i] = packagePart{name: "InventedPackage/" + strings.Repeat("x", i+1), mode: 0600}
		}
		b := packageBytes(t, p)
		if _, e := VerifyPackage(bytes.NewReader(b), int64(len(b))); !errors.Is(e, ErrPackageLimit) {
			t.Fatal(e)
		}
	})
	t.Run("input", func(t *testing.T) {
		_, e := VerifyPackage(nil, packageMaxBytes+1)
		if !errors.Is(e, ErrPackageLimit) {
			t.Fatal(e)
		}
	})
	t.Run("expanded size", func(t *testing.T) {
		b := append([]byte{}, packageFixture(t, "2")...)
		i := bytes.Index(b, []byte{'P', 'K', 1, 2})
		if i < 0 {
			t.Fatal("central directory missing")
		}
		binary.LittleEndian.PutUint32(b[i+24:], packageMaxBytes+1)
		_, e := VerifyPackage(bytes.NewReader(b), int64(len(b)))
		if !errors.Is(e, ErrPackageLimit) {
			t.Fatal(e)
		}
	})
	t.Run("CRC", func(t *testing.T) {
		b := append([]byte{}, packageFixture(t, "2")...)
		i := bytes.Index(b, []byte{'P', 'K', 1, 2})
		b[i+16] ^= 1
		_, e := VerifyPackage(bytes.NewReader(b), int64(len(b)))
		if !errors.Is(e, ErrPackageInvalid) {
			t.Fatal(e)
		}
	})
	t.Run("manifest size", func(t *testing.T) {
		p := packageParts(t, packageFixture(t, "2"))
		for i := range p {
			if strings.HasSuffix(p[i].name, "manifest.json") {
				p[i].data = bytes.Repeat([]byte(" "), packageIndexBytes+1)
			}
		}
		b := packageBytes(t, p)
		_, e := VerifyPackage(bytes.NewReader(b), int64(len(b)))
		if !errors.Is(e, ErrPackageLimit) {
			t.Fatal(e)
		}
	})
}
func TestPackageWriterRejectsUnsafeAndReservedPaths(t *testing.T) {
	t.Run("unsafe root", func(t *testing.T) {
		a := NewArchive("Invented", "dataset", "Invented", "synthetic", 1, time.Now())
		a.Root = "../outside"
		var b bytes.Buffer
		if _, e := a.WriteZip(&b); !errors.Is(e, ErrPackageInvalid) {
			t.Fatal(e)
		}
		if b.Len() != 0 {
			t.Fatal("unsafe root emitted bytes")
		}
	})
	for _, dir := range []string{"../outside", "manifest"} {
		t.Run(dir, func(t *testing.T) {
			a := NewArchive("Invented", "dataset", "Invented", "synthetic", 1, time.Now())
			a.Add(dir, "manifest.json", "metadata", []byte("{}"))
			var b bytes.Buffer
			if _, e := a.WriteZip(&b); !errors.Is(e, ErrPackageInvalid) {
				t.Fatal(e)
			}
		})
	}
}
func FuzzVerifyPackage(f *testing.F) {
	for _, v := range []string{"1", "2"} {
		b, e := os.ReadFile("../../testdata/package-integrity/invented-v" + v + ".zip")
		if e != nil {
			f.Fatal(e)
		}
		f.Add(b)
	}
	f.Add([]byte("not zip"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			t.Skip()
		}
		r, e := VerifyPackage(bytes.NewReader(b), int64(len(b)))
		if e != nil && r.CheckedFiles != 0 {
			t.Fatal("partial success")
		}
		if r.Authenticated {
			t.Fatal("checksums authenticated identity")
		}
	})
}
