package dts

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/oaovito/mne_lab/internal/testfixtures"
	"testing"
)

func TestCompoundRecordMetadataWithoutScientificInference(t *testing.T) {
	b := testfixtures.CompoundDTS("5.10")
	original := append([]byte{}, b...)
	r, err := Inspect(b)
	if err != nil || len(r.Records) != 1 || len(r.Container.Streams) != 2 {
		t.Fatal("synthetic container rejected", err)
	}
	if !bytes.Equal(b, original) {
		t.Fatal("original modified")
	}
	if r.Classification != "UNKNOWN MALVERN DTS" || r.ScientificValidation != "UNVALIDATED" || r.Records[0].AnalysisMethod != "UNKNOWN" {
		t.Fatal("unsupported science inferred")
	}
	if r.Capabilities.DLSSummary || r.Capabilities.ZetaSummary || r.Capabilities.Distributions || r.Capabilities.Correlation {
		t.Fatal("unsupported capability advertised")
	}
	if len(r.Records[0].Strings) != 2 || r.Records[0].Strings[1].Mapping != "unmapped" {
		t.Fatal("string origin lost")
	}
	other, err := Inspect(testfixtures.CompoundDTS("99.0"))
	if err != nil || other.Records[0].MetadataVersionObserved {
		t.Fatal("future version treated as validated", err)
	}
}

func TestCompoundRejectsMalformedChainsAndSizes(t *testing.T) {
	for name, change := range map[string]func([]byte) []byte{
		"truncated":              func(b []byte) []byte { return b[:511] },
		"signature":              func(b []byte) []byte { b[0] = 0; return b },
		"huge FAT":               func(b []byte) []byte { binary.LittleEndian.PutUint32(b[44:], 0xffffffff); return b },
		"directory sector cycle": func(b []byte) []byte { binary.LittleEndian.PutUint32(b[18*512:], 0); return b },
		"directory child cycle":  func(b []byte) []byte { binary.LittleEndian.PutUint32(b[512+128+76:], 0); return b },
		"oversized stream":       func(b []byte) []byte { binary.LittleEndian.PutUint32(b[512+128+120:], 0x7fffffff); return b },
		"duplicate stream":       func(b []byte) []byte { copy(b[512+2*128:512+2*128+66], b[512+128:512+128+66]); return b },
		"record mismatch":        func(b []byte) []byte { binary.LittleEndian.PutUint32(b[5120+12:], 2); return b },
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Inspect(change(testfixtures.CompoundDTS("5.10"))); err == nil {
				t.Fatal("malformed input accepted")
			}
		})
	}
	if _, err := DecodeContainer(make([]byte, MaxFileSize+1)); !errors.Is(err, ErrLimit) {
		t.Fatal("size limit missing", err)
	}
}

func FuzzDTSContainer(f *testing.F) {
	f.Add(testfixtures.CompoundDTS("5.10"))
	f.Add([]byte("not a compound document"))
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 1<<20 {
			return
		}
		Inspect(b)
	})
}
