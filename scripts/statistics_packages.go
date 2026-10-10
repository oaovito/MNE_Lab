//go:build ignore

// Build-only assembler for a pinned, read-only Emscripten package image.
// The application never downloads packages or executes this helper.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type pin struct {
	Package string `json:"package"`
	Version string `json:"version"`
	URL     string `json:"url"`
	File    string `json:"file"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
}
type entry struct {
	Filename string `json:"filename"`
	Start    int64  `json:"start"`
	End      int64  `json:"end"`
}
type metadata struct {
	Files []entry `json:"files"`
	Gzip  bool    `json:"gzip"`
}

func main() {
	manifest := flag.String("manifest", "scripts/statistics-packages.json", "pinned package manifest")
	cache := flag.String("cache", "web/.statistics-package-cache", "build-only download cache")
	out := flag.String("out", "web/.statistics-package-cache/image", "image output directory")
	flag.Parse()
	if err := assemble(*manifest, *cache, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func assemble(manifest, cache, out string) error {
	raw, err := os.ReadFile(manifest)
	if err != nil {
		return err
	}
	var m struct {
		Schema   int   `json:"schema"`
		Packages []pin `json:"packages"`
	}
	if err = json.Unmarshal(raw, &m); err != nil {
		return err
	}
	if m.Schema != 1 || len(m.Packages) != 11 {
		return fmt.Errorf("invalid package manifest")
	}
	if err = os.MkdirAll(cache, 0700); err != nil {
		return err
	}
	files := map[string][]byte{}
	var total int64
	client := &http.Client{Timeout: 90 * time.Second}
	for _, p := range m.Packages {
		if path.Base(p.File) != p.File || p.Size <= 0 || p.Size > 32<<20 || len(p.SHA256) != 64 || !strings.HasPrefix(p.URL, "https://repo.r-wasm.org/bin/emscripten/contrib/4.6/") {
			return fmt.Errorf("invalid pin: %s", p.Package)
		}
		archive := filepath.Join(cache, p.File)
		data, readErr := os.ReadFile(archive)
		if os.IsNotExist(readErr) {
			response, e := client.Get(p.URL)
			if e != nil {
				return e
			}
			data, e = io.ReadAll(io.LimitReader(response.Body, p.Size+1))
			response.Body.Close()
			if e != nil {
				return e
			}
			if response.StatusCode != http.StatusOK {
				return fmt.Errorf("package HTTP status: %s %d", p.Package, response.StatusCode)
			}
		} else if readErr != nil {
			return readErr
		}
		digest := sha256.Sum256(data)
		if int64(len(data)) != p.Size || hex.EncodeToString(digest[:]) != p.SHA256 {
			return fmt.Errorf("package checksum mismatch: %s", p.Package)
		}
		if readErr != nil {
			if err = os.WriteFile(archive, data, 0600); err != nil {
				return err
			}
		}
		gz, e := gzip.NewReader(bytes.NewReader(data))
		if e != nil {
			return e
		}
		tr := tar.NewReader(gz)
		for {
			h, e := tr.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				gz.Close()
				return e
			}
			name := strings.TrimSuffix(h.Name, "/")
			// Upstream's install index is outside the package tree. The image
			// metadata we generate replaces it; it is never mounted or executed.
			if name == ".vfs-index.json" && h.Typeflag == tar.TypeReg && h.Size >= 0 && h.Size <= 1<<20 {
				continue
			}
			if path.Clean(name) != name || !strings.HasPrefix(name, p.Package+"/") && name != p.Package || strings.Contains(name, "\\") {
				gz.Close()
				return fmt.Errorf("unsafe package path")
			}
			if h.Typeflag == tar.TypeDir {
				continue
			}
			if h.Typeflag != tar.TypeReg || h.Size < 0 || h.Size > 32<<20 || total+h.Size > 128<<20 || len(files) >= 20000 {
				gz.Close()
				return fmt.Errorf("unsafe package entry")
			}
			if _, ok := files[name]; ok {
				gz.Close()
				return fmt.Errorf("duplicate package entry")
			}
			content, e := io.ReadAll(io.LimitReader(tr, h.Size+1))
			if e != nil || int64(len(content)) != h.Size {
				gz.Close()
				return fmt.Errorf("truncated package entry")
			}
			files[name] = content
			total += h.Size
		}
		if err = gz.Close(); err != nil {
			return err
		}
	}
	if err = os.MkdirAll(out, 0700); err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	image, err := os.Create(filepath.Join(out, "packages.data.gz"))
	if err != nil {
		return err
	}
	writer := gzip.NewWriter(image)
	meta := metadata{Files: []entry{}, Gzip: true}
	var position int64
	for _, name := range names {
		content := files[name]
		end := position + int64(len(content))
		meta.Files = append(meta.Files, entry{"/" + name, position, end})
		if _, err = writer.Write(content); err != nil {
			writer.Close()
			image.Close()
			return err
		}
		position = end
	}
	if err = writer.Close(); err != nil {
		image.Close()
		return err
	}
	if err = image.Close(); err != nil {
		return err
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(out, "packages.metadata.json"), encoded, 0600); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(out, "packages.manifest.json"), raw, 0600); err != nil {
		return err
	}
	fmt.Printf("Pinned package image: %d packages, %d files, %d uncompressed bytes\n", len(m.Packages), len(files), total)
	return nil
}
