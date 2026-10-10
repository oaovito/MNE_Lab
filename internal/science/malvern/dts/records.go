package dts

import (
	"encoding/binary"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

type StringField struct {
	Offset  int    `json:"offset"`
	Bytes   int    `json:"bytes"`
	Value   string `json:"value"`
	Mapping string `json:"mapping"` // unmapped; location is known, semantic role is not
}
type Record struct {
	Stream                  string        `json:"stream"`
	ID                      uint32        `json:"id"`
	TypeCode                uint32        `json:"typeCode"` // observed code, not an authoritative scientific type
	AuxCode                 uint32        `json:"auxCode"`
	VersionText             string        `json:"versionText"`
	MetadataVersionObserved bool          `json:"metadataVersionObserved"`
	AnalysisMethod          string        `json:"analysisMethod"`
	Strings                 []StringField `json:"unmappedStrings,omitempty"`
}
type Capabilities struct {
	Container      bool `json:"container"`
	RecordEnvelope bool `json:"recordEnvelope"`
	Metadata       bool `json:"metadata"`
	DLSSummary     bool `json:"dlsSummary"`
	ZetaSummary    bool `json:"zetaSummary"`
	Distributions  bool `json:"distributions"`
	Correlation    bool `json:"correlation"`
}
type Report struct {
	Parser               string       `json:"parser"`
	Status               string       `json:"status"`
	Classification       string       `json:"classification"`
	ScientificValidation string       `json:"scientificValidation"`
	Container            Container    `json:"container"`
	HeaderCode           uint16       `json:"headerCode,omitempty"`
	Records              []Record     `json:"records"`
	Capabilities         Capabilities `json:"capabilities"`
	Warnings             []string     `json:"warnings"`
}

var recordName = regexp.MustCompile(`^REC([1-9][0-9]{0,8})$`)
var softwareVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)*(?: [A-Za-z0-9 _.-]+)?$`)

// utf16Field decodes a length-prefixed, encoding-tagged string. This grammar
// was checked across record generations in the private reference. It assigns
// no scientific meaning to an unlabelled field and preserves its byte origin.
func utf16Field(b []byte, offset int) (StringField, bool) {
	if offset < 0 || offset+5 > len(b) || b[offset+4] != 1 {
		return StringField{}, false
	}
	n := int(binary.LittleEndian.Uint32(b[offset : offset+4]))
	if n < 2 || n > 8192 || n%2 != 0 || n > len(b)-offset-5 {
		return StringField{}, false
	}
	raw := b[offset+5 : offset+5+n]
	if raw[n-2] != 0 || raw[n-1] != 0 {
		return StringField{}, false
	}
	units := make([]uint16, n/2-1)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[2*i : 2*i+2])
		if units[i] < 32 && units[i] != '\t' {
			return StringField{}, false
		}
	}
	// Reject malformed surrogate pairs instead of inventing replacement text.
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u >= 0xd800 && u <= 0xdbff {
			if i+1 >= len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
				return StringField{}, false
			}
			i++
		} else if u >= 0xdc00 && u <= 0xdfff {
			return StringField{}, false
		}
	}
	return StringField{Offset: offset, Bytes: n + 5, Value: string(utf16.Decode(units)), Mapping: "unmapped"}, true
}

func decodeRecord(stream Stream) (Record, bool) {
	match := recordName.FindStringSubmatch(stream.Path)
	b := stream.Data
	if match == nil || len(b) < 21 || binary.LittleEndian.Uint16(b) != 13 || binary.LittleEndian.Uint16(b[10:]) != 12 {
		return Record{}, false
	}
	id, _ := strconv.ParseUint(match[1], 10, 32)
	if binary.LittleEndian.Uint32(b[12:]) != uint32(id) {
		return Record{}, false
	}
	v, ok := utf16Field(b, 16)
	if !ok || len(v.Value) > 64 || !softwareVersion.MatchString(v.Value) {
		return Record{}, false
	}
	r := Record{Stream: stream.Path, ID: uint32(id), TypeCode: binary.LittleEndian.Uint32(b[2:]), AuxCode: binary.LittleEndian.Uint32(b[6:]), VersionText: v.Value, AnalysisMethod: "UNKNOWN", Strings: []StringField{}}
	// Version-aware coverage: only envelope/strings, never numeric field layouts.
	for _, prefix := range []string{"5.10", "6.30", "7.0", "7.11"} {
		if v.Value == prefix || strings.HasPrefix(v.Value, prefix+".") || strings.HasPrefix(v.Value, prefix+" ") {
			r.MetadataVersionObserved = true
		}
	}
	for offset := 16; offset+5 < len(b) && len(r.Strings) < 256; offset++ {
		if field, ok := utf16Field(b, offset); ok {
			r.Strings = append(r.Strings, field)
			offset += field.Bytes - 1
		}
	}
	return r, true
}

func Inspect(data []byte) (Report, error) {
	c, err := DecodeContainer(data)
	if err != nil {
		return Report{}, err
	}
	r := Report{Parser: Version, Status: "partial", Classification: "UNKNOWN COMPOUND FILE", ScientificValidation: "UNVALIDATED", Container: c, Records: []Record{}, Capabilities: Capabilities{Container: true}, Warnings: []string{"dts.scientific_fields_unmapped"}}
	header := false
	for _, s := range c.Streams {
		if s.Path == "Header" && len(s.Data) >= 24 {
			r.HeaderCode = binary.LittleEndian.Uint16(s.Data)
			header = true
		}
	}
	for _, s := range c.Streams {
		if rec, ok := decodeRecord(s); ok {
			r.Records = append(r.Records, rec)
		} else if recordName.MatchString(s.Path) {
			r.Warnings = append(r.Warnings, "dts.record_structure_unsupported")
		}
	}
	if !header || len(r.Records) == 0 {
		return r, ErrUnsupported
	}
	r.Classification = "UNKNOWN MALVERN DTS"
	r.Capabilities.RecordEnvelope = true
	r.Capabilities.Metadata = true
	for _, rec := range r.Records {
		if !rec.MetadataVersionObserved {
			r.Warnings = append(r.Warnings, "dts.software_version_unvalidated")
			break
		}
	}
	// Numeric type codes and historical SOP paths are not sufficient authority
	// to assign DLS/ZETA or normalize an unlabelled float into a result.
	return r, nil
}
