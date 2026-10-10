package dts

import "encoding/binary"

const chainEnd uint32 = 0xfffffffe
const unused uint32 = 0xffffffff

// preflight bounds and validates the directory graph before the mature CFB
// library recursively traverses it. Offsets come from sector tables, never
// guessed absolute locations in a sample. Stream decoding remains separate.
func preflight(data []byte, sector int) error {
	u32 := func(b []byte, n int) uint32 { return binary.LittleEndian.Uint32(b[n : n+4]) }
	count := uint32(len(data)/sector - 1)
	block := func(id uint32) ([]byte, error) {
		if id >= count {
			return nil, ErrContainer
		}
		off := (int(id) + 1) * sector
		return data[off : off+sector], nil
	}
	ids := []uint32{}
	for i := 76; i < 512; i += 4 {
		if id := u32(data, i); id != unused {
			ids = append(ids, id)
		}
	}
	seenDifat := map[uint32]bool{}
	difat := u32(data, 68)
	for i := uint32(0); i < u32(data, 72); i++ {
		if seenDifat[difat] {
			return ErrContainer
		}
		seenDifat[difat] = true
		b, e := block(difat)
		if e != nil {
			return e
		}
		for j := 0; j < sector-4; j += 4 {
			if id := u32(b, j); id != unused {
				ids = append(ids, id)
			}
		}
		difat = u32(b, sector-4)
	}
	if len(ids) != int(u32(data, 44)) || len(ids) == 0 {
		return ErrContainer
	}
	fat := make([]uint32, 0, len(ids)*(sector/4))
	seenFat := map[uint32]bool{}
	for _, id := range ids {
		if seenFat[id] || seenDifat[id] {
			return ErrContainer
		}
		seenFat[id] = true
		b, e := block(id)
		if e != nil {
			return e
		}
		for i := 0; i < sector; i += 4 {
			fat = append(fat, u32(b, i))
		}
	}
	if len(fat) < int(count) {
		return ErrContainer
	}
	seen := map[uint32]bool{}
	entries := [][]byte{}
	id := u32(data, 48)
	for id != chainEnd {
		if seen[id] || id >= count || seenFat[id] || seenDifat[id] {
			return ErrContainer
		}
		seen[id] = true
		b, e := block(id)
		if e != nil {
			return e
		}
		for i := 0; i < sector; i += 128 {
			entries = append(entries, b[i:i+128])
		}
		if len(entries) > maxEntries {
			return ErrLimit
		}
		id = fat[id]
	}
	if len(entries) == 0 || entries[0][66] != 5 || u32(entries[0], 68) != unused || u32(entries[0], 72) != unused {
		return ErrContainer
	}
	visited := map[uint32]bool{}
	stack := []uint32{0}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if id >= uint32(len(entries)) || visited[id] {
			return ErrContainer
		}
		visited[id] = true
		b := entries[id]
		typ := b[66]
		if typ != 1 && typ != 2 && typ != 5 {
			return ErrContainer
		}
		length := binary.LittleEndian.Uint16(b[64:66])
		if length < 2 || length > 64 || length%2 != 0 || b[length-2] != 0 || b[length-1] != 0 {
			return ErrContainer
		}
		for _, n := range []int{68, 72, 76} {
			if child := u32(b, n); child != unused {
				if typ == 2 && n == 76 {
					return ErrContainer
				}
				stack = append(stack, child)
			}
		}
	}
	return nil
}
