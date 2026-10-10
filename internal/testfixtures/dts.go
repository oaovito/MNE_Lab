// Package testfixtures generates wholly invented development fixtures.
// It is imported by tests only and is not a demo dataset or production input.
package testfixtures

import (
	"encoding/binary"
	"unicode/utf16"
)

func syntheticString(value string) []byte {
	u := append(utf16.Encode([]rune(value)), 0)
	b := make([]byte, 5+len(u)*2)
	binary.LittleEndian.PutUint32(b, uint32(len(u)*2))
	b[4] = 1
	for i, v := range u {
		binary.LittleEndian.PutUint16(b[5+i*2:], v)
	}
	return b
}

// Invented CFB v3 fixture with regular streams. No bytes, identities or
// numeric results are copied from a real DTS. Sector/FAT rules follow MS-CFB.
func CompoundDTS(version string) []byte {
	const fat = 17
	b := make([]byte, (fat+2)*512)
	copy(b, []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1})
	put16 := func(n int, v uint16) { binary.LittleEndian.PutUint16(b[n:], v) }
	put32 := func(n int, v uint32) { binary.LittleEndian.PutUint32(b[n:], v) }
	put16(24, 62)
	put16(26, 3)
	put16(28, 0xfffe)
	put16(30, 9)
	put16(32, 6)
	put32(44, 1)
	put32(48, 0)
	put32(56, 4096)
	put32(60, uint32(0xfffffffe))
	put32(68, uint32(0xfffffffe))
	for i := 76; i < 512; i += 4 {
		put32(i, uint32(0xffffffff))
	}
	put32(76, fat)
	entry := func(index int, name string, typ byte, start, size uint32) {
		off := 512 + 128*index
		u := append(utf16.Encode([]rune(name)), 0)
		for i, v := range u {
			put16(off+i*2, v)
		}
		put16(off+64, uint16(len(u)*2))
		b[off+66] = typ
		b[off+67] = 1
		for _, n := range []int{68, 72, 76} {
			put32(off+n, uint32(0xffffffff))
		}
		put32(off+116, start)
		put32(off+120, size)
	}
	entry(0, "Root Entry", 5, uint32(0xfffffffe), 0)
	put32(512+76, 1)
	entry(1, "Header", 2, 1, 4096)
	put32(512+128+72, 2)
	entry(2, "REC1", 2, 9, 4096)
	put16(1024, 1) // observed header version; all opaque bytes invented zero
	rec := 5120
	put16(rec, 13)
	put32(rec+2, 1)
	put16(rec+10, 12)
	put32(rec+12, 1)
	v := syntheticString(version)
	copy(b[rec+16:], v)
	copy(b[rec+16+len(v)+8:], syntheticString(`C:\invented\DTS\SOP\Size\Synthetic.sop`))
	for i := 0; i < 128; i++ {
		put32((fat+1)*512+4*i, uint32(0xffffffff))
	}
	put32((fat+1)*512, uint32(0xfffffffe))
	for i := 1; i <= 16; i++ {
		next := uint32(i + 1)
		if i == 8 || i == 16 {
			next = uint32(0xfffffffe)
		}
		put32((fat+1)*512+4*i, next)
	}
	put32((fat+1)*512+4*fat, 0xfffffffd)
	return b
}
