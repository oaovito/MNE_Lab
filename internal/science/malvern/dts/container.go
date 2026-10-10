// Package dts reads Malvern DTS containers without executing embedded content.
// Container validity is independent of scientific field interpretation.
package dts

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"strings"

	"github.com/richardlehane/mscfb"
)

const Version = "malvern-dts/0.1.0-partial"
const MaxFileSize = 32 << 20
const maxEntries = 2048

var ErrContainer = errors.New("dts.invalid_container")
var ErrLimit = errors.New("dts.resource_limit")
var ErrUnsupported = errors.New("dts.unsupported_structure")

var signature = []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}

func IsCompound(data []byte) bool { return len(data) >= 8 && bytes.Equal(data[:8], signature) }

type Stream struct {
	Path   string `json:"path"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
	Data   []byte `json:"-"`
}
type Container struct {
	MajorVersion uint16   `json:"majorVersion"`
	SectorSize   int      `json:"sectorSize"`
	Storages     []string `json:"storages"`
	Streams      []Stream `json:"streams"`
}

// reader bounds library I/O even for malformed chains. Directory traversal
// is additionally bounded by the validated file and directory-entry limit.
type boundedReader struct {
	data  []byte
	reads int
}

func (r *boundedReader) ReadAt(p []byte, off int64) (int, error) {
	r.reads++
	if r.reads > 200000 || off < 0 || off > int64(len(r.data)) {
		return 0, ErrLimit
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func DecodeContainer(data []byte) (out Container, err error) {
	// A dependency panic is converted to a stable identifier, never a dump of
	// source bytes, private names or paths. Allocation bounds precede decoding.
	defer func() {
		if recover() != nil {
			out = Container{}
			err = ErrContainer
		}
	}()
	if len(data) > MaxFileSize {
		return out, ErrLimit
	}
	if len(data) < 512 || !IsCompound(data) {
		return out, ErrContainer
	}
	u16 := func(n int) uint16 { return binary.LittleEndian.Uint16(data[n : n+2]) }
	u32 := func(n int) uint32 { return binary.LittleEndian.Uint32(data[n : n+4]) }
	major, shift := u16(26), u16(30)
	if u16(28) != 0xfffe || u16(32) != 6 || !((major == 3 && shift == 9) || (major == 4 && shift == 12)) || u32(56) != 4096 {
		return out, ErrContainer
	}
	sector := 1 << shift
	if len(data) < sector || len(data)%sector != 0 {
		return out, ErrContainer
	}
	count := uint32(len(data)/sector - 1)
	for _, n := range []int{40, 44, 64, 72} {
		if u32(n) > count {
			return out, ErrLimit
		}
	}
	// A small maximum directory graph protects recursive traversal independently
	// from stream byte limits. Count entries before invoking the library.
	if count == 0 || (major == 3 && u32(40) != 0) {
		return out, ErrContainer
	}
	if e := preflight(data, sector); e != nil {
		return out, e
	}
	reader := &boundedReader{data: data}
	doc, e := mscfb.New(reader)
	if e != nil {
		return out, ErrContainer
	}
	if len(doc.File) > maxEntries {
		return out, ErrLimit
	}
	out = Container{MajorVersion: major, SectorSize: sector, Streams: []Stream{}, Storages: []string{}}
	seen := map[string]bool{}
	total := 0
	for _, entry := range doc.File {
		if entry.Name == "Root Entry" && len(entry.Path) == 0 {
			continue
		}
		path := strings.Join(append(append([]string{}, entry.Path...), entry.Name), "/")
		if len(path) > 4096 || seen[path] {
			return Container{}, ErrContainer
		}
		seen[path] = true
		// Paths are metadata identifiers, never filesystem destinations.
		if entry.Size < 0 || entry.Size > int64(len(data)) {
			return Container{}, ErrLimit
		}
		if entry.FileInfo().IsDir() {
			out.Storages = append(out.Storages, path)
			continue
		}
		total += int(entry.Size)
		if total > len(data) {
			return Container{}, ErrLimit
		}
		b := make([]byte, int(entry.Size))
		if _, e := io.ReadFull(entry, b); e != nil {
			return Container{}, ErrContainer
		}
		sum := sha256.Sum256(b)
		out.Streams = append(out.Streams, Stream{Path: path, Size: len(b), SHA256: hex.EncodeToString(sum[:]), Data: b})
	}
	return out, nil
}
