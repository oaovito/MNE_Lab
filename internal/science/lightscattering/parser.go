// Package lightscattering reads Dynamic Light Scattering exports from the
// NanoBrook 90Plus (Brookhaven Instruments).
//
// The parser recognizes only fields named by the scientific specification
// (Effective Diameter, Polydispersity, Current Count Rate, BaseLine Index,
// Diameter / Particle Size, Intensity, plus Volume and Number when present)
// and keeps every other labeled value untouched as an unrecognized field.
// It never invents a field: a value absent from the file stays absent.
package lightscattering

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/reference"
)

// Version identifies this parser in provenance records.
const Version = "lightscattering-parser/1.0.0"

// MaxFileSize bounds imports to keep parsing predictable on small machines.
const MaxFileSize = 32 << 20

// Result is the outcome of parsing one file.
type Result struct {
	Status       string              `json:"status"` // parsed, partial, failed
	Measurements []model.Measurement `json:"measurements"`
	Encoding     string              `json:"encoding"`
	Delimiter    string              `json:"delimiter,omitempty"`
	Decimal      string              `json:"decimal,omitempty"`
	Recognized   []string            `json:"recognized"`
	Warnings     []string            `json:"warnings,omitempty"`
	Error        string              `json:"error,omitempty"`
	Parser       string              `json:"parser"`
	Spec         string              `json:"spec"`
}

// Stable error identifiers.
var (
	ErrEmpty        = errors.New("ls.empty_file")
	ErrBinary       = errors.New("ls.not_text")
	ErrTooLarge     = errors.New("ls.too_large")
	ErrUnrecognized = errors.New("ls.no_recognized_data")
)

type alias struct {
	key   string
	names []string
}

// Summary field aliases (normalized: lowercase, punctuation removed).
var summaryAliases = []alias{
	{model.EffectiveDiameter, []string{"effective diameter", "eff diameter", "eff diam", "effective diam", "effective dia"}},
	{model.Polydispersity, []string{"polydispersity", "polydispersity index", "pdi", "polydisp", "poly"}},
	{model.CountRate, []string{"current count rate", "count rate", "countrate", "current countrate"}},
	{model.BaselineIndex, []string{"baseline index", "base line index", "baselineindex", "bl index", "baseline idx"}},
	{model.SampleID, []string{"sample id", "sampleid", "sample name", "sample", "sample identifier"}},
	{model.MeasuredAt, []string{"date time", "date and time", "measurement date", "measured", "date", "acquired", "timestamp", "date of measurement"}},
	{"_time", []string{"time", "measurement time", "time of measurement"}},
}

// Table column aliases.
var columnAliases = []alias{
	{"diameter", []string{"diameter", "diam", "d", "size", "particle size", "hydrodynamic diameter", "dh", "diameter nm", "d nm"}},
	{"intensity", []string{"intensity", "int", "g d", "gd", "intensity weighted", "intensity distribution", "relative intensity"}},
	{"volume", []string{"volume", "vol", "volume weighted", "volume distribution"}},
	{"number", []string{"number", "num", "number weighted", "number distribution"}},
}

var unitPattern = regexp.MustCompile(`(?i)^(nm|µm|um|μm|mm|%|kcps|mcps|cps|s|ms|°c|c|k|mv|cp|a\.u\.|au|deg|°)$`)

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space && b.Len() > 0 {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// splitLabelUnit separates "Effective Diameter (nm)" into label and unit.
func splitLabelUnit(label string) (string, string) {
	label = strings.TrimSpace(label)
	for _, p := range [][2]string{{"(", ")"}, {"[", "]"}} {
		if i := strings.LastIndex(label, p[0]); i > 0 && strings.HasSuffix(label, p[1]) {
			u := strings.TrimSpace(label[i+1 : len(label)-1])
			if u != "" && len(u) <= 12 {
				return strings.TrimSpace(label[:i]), canonicalUnit(u)
			}
		}
	}
	return label, ""
}

func canonicalUnit(u string) string {
	switch strings.ToLower(strings.TrimSpace(u)) {
	case "nm":
		return "nm"
	case "um", "µm", "μm":
		return "µm"
	case "kcps":
		return "kcps"
	case "mcps":
		return "Mcps"
	case "cps":
		return "cps"
	case "%":
		return "%"
	case "°c", "c", "deg c", "degc":
		return "°C"
	}
	return strings.TrimSpace(u)
}

func lookup(aliases []alias, label string) string {
	n := normalize(label)
	for _, a := range aliases {
		for _, name := range a.names {
			if n == name {
				return a.key
			}
		}
	}
	return ""
}

// decode converts the file to UTF-8, detecting UTF-16 and legacy Windows
// encodings.
func decode(b []byte) (string, string, error) {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		b = b[3:]
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}), bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		le := b[0] == 0xFF
		b = b[2:]
		u := make([]uint16, len(b)/2)
		for i := range u {
			if le {
				u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
			} else {
				u[i] = uint16(b[2*i])<<8 | uint16(b[2*i+1])
			}
		}
		return string(utf16.Decode(u)), "UTF-16", nil
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return "", "", ErrBinary
	}
	if utf8.Valid(b) {
		return string(b), "UTF-8", nil
	}
	s, err := charmap.Windows1252.NewDecoder().Bytes(b)
	if err != nil {
		return "", "", ErrBinary
	}
	return string(s), "Windows-1252", nil
}

type line struct {
	n    int
	text string
}

// Parse reads one exported file.
func Parse(data []byte) Result {
	res := Result{Parser: Version, Spec: reference.LightScatteringSpecID}
	fail := func(err error) Result {
		res.Status, res.Error = "failed", err.Error()
		return res
	}
	if len(data) == 0 {
		return fail(ErrEmpty)
	}
	if len(data) > MaxFileSize {
		return fail(ErrTooLarge)
	}
	text, enc, err := decode(data)
	if err != nil {
		return fail(err)
	}
	res.Encoding = enc
	var lines []line
	for i, l := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), "\n") {
		lines = append(lines, line{i + 1, strings.TrimRight(l, " \t")})
	}
	ctrl := 0
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			ctrl++
		}
	}
	if ctrl > len(text)/20+2 {
		return fail(ErrBinary)
	}
	delim, dec := detectTableFormat(lines)
	res.Delimiter, res.Decimal = delimName(delim), dec
	blocks := segment(lines)
	recognized := map[string]bool{}
	for bi, blk := range blocks {
		m := model.Measurement{Index: bi, Params: map[string]model.Quantity{}, Parser: Version, Spec: reference.LightScatteringSpecID}
		tbl := findTable(blk, delim, dec)
		inTable := map[int]bool{}
		timeText := ""
		if tbl != nil {
			for n := tbl.FirstLine; n <= tbl.LastLine; n++ {
				inTable[n] = true
			}
			if tbl.headerLine > 0 {
				inTable[tbl.headerLine] = true
			}
			m.Dist = &tbl.Distribution
			for _, c := range tbl.Columns {
				if c.Key != "" {
					recognized[c.Key] = true
				}
			}
		}
		for _, l := range blk {
			if inTable[l.n] || strings.TrimSpace(l.text) == "" {
				continue
			}
			label, value, ok := splitKeyValue(l.text)
			if !ok {
				continue
			}
			baseLabel, unit := splitLabelUnit(label)
			key := lookup(summaryAliases, baseLabel)
			f := model.Field{Label: strings.TrimSpace(label), Text: value, Unit: unit, Line: l.n}
			numText, valUnit := splitValueUnit(value)
			if unit == "" {
				unit = valUnit
				f.Unit = unit
			}
			if v, ok := parseScalar(numText, dec); ok {
				f.Num = &v
			} else if strings.Count(numText, ",") == 1 && isNumeric(strings.Replace(numText, ",", ".", 1)) && key != "" && key != model.SampleID && key != model.MeasuredAt && key != "_time" {
				res.Warnings = append(res.Warnings, fmt.Sprintf("ls.ambiguous_number:%d", l.n))
			}
			switch key {
			case model.SampleID:
				if m.SampleID == "" {
					m.SampleID = strings.TrimSpace(value)
					f.Key = key
					recognized[key] = true
				}
			case model.MeasuredAt:
				if m.MeasuredAt == nil {
					if ts, ok := parseTimestamp(value); ok {
						m.MeasuredAt = ts
						f.Key = key
						recognized[key] = true
					}
				}
			case "_time":
				timeText = strings.TrimSpace(value)
				f.Key = model.MeasuredAt
			case "":
			default:
				if _, dup := m.Params[key]; !dup && f.Num != nil {
					m.Params[key] = model.Quantity{Value: *f.Num, Raw: numText, Unit: unit, Label: f.Label, Line: l.n}
					f.Key = key
					recognized[key] = true
				}
			}
			m.Fields = append(m.Fields, f)
		}
		if timeText != "" {
			// Separate "Date" and "Time" fields: combine them.
			if m.MeasuredAt != nil && m.MeasuredAt.Time.Hour() == 0 && m.MeasuredAt.Time.Minute() == 0 && !strings.Contains(m.MeasuredAt.Raw, ":") {
				if ts, ok := parseTimestamp(m.MeasuredAt.Raw + " " + timeText); ok {
					m.MeasuredAt = ts
				}
			}
		}
		if len(m.Params) > 0 || m.Dist != nil {
			res.Measurements = append(res.Measurements, m)
		}
	}
	for k := range recognized {
		res.Recognized = append(res.Recognized, k)
	}
	sort.Strings(res.Recognized)
	if len(res.Measurements) == 0 {
		return fail(ErrUnrecognized)
	}
	res.Status = "parsed"
	for i := range res.Measurements {
		m := &res.Measurements[i]
		if m.Dist == nil {
			res.Status = "partial"
			res.Warnings = append(res.Warnings, fmt.Sprintf("ls.no_distribution:%d", i+1))
		} else if m.Dist.Column("diameter") == nil || len(m.Dist.Weightings()) == 0 {
			res.Status = "partial"
			res.Warnings = append(res.Warnings, fmt.Sprintf("ls.table_without_diameter_or_weighting:%d", i+1))
		}
		if len(m.Params) == 0 {
			res.Warnings = append(res.Warnings, fmt.Sprintf("ls.no_summary_parameters:%d", i+1))
		}
		if m.MeasuredAt == nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("ls.no_measurement_date:%d", i+1))
		} else if m.MeasuredAt.Ambiguous {
			res.Warnings = append(res.Warnings, fmt.Sprintf("ls.ambiguous_date:%d", i+1))
		}
		for _, k := range []string{model.EffectiveDiameter, model.CountRate} {
			if q, ok := m.Params[k]; ok && q.Unit == "" {
				res.Warnings = append(res.Warnings, fmt.Sprintf("ls.unit_missing:%s:%d", k, i+1))
			}
		}
		if c := m.Dist.Column("diameter"); c != nil && c.Unit == "" {
			res.Warnings = append(res.Warnings, fmt.Sprintf("ls.unit_missing:diameter:%d", i+1))
		}
	}
	return res
}

func delimName(d string) string {
	switch d {
	case "\t":
		return "tab"
	case ";":
		return "semicolon"
	case ",":
		return "comma"
	case " ":
		return "whitespace"
	}
	return ""
}

// splitKeyValue recognizes "Label: value", "Label = value" and
// "Label<TAB>value" lines.
func splitKeyValue(s string) (string, string, bool) {
	t := strings.TrimSpace(s)
	for _, sep := range []string{":", "="} {
		if i := strings.Index(t, sep); i > 0 {
			label, value := strings.TrimSpace(t[:i]), strings.TrimSpace(t[i+1:])
			// "Date: 10/05/2026 14:30:00" — only the first colon splits.
			if label != "" && value != "" && !isNumeric(label) && len(label) <= 64 {
				return label, value, true
			}
		}
	}
	if parts := strings.Split(t, "\t"); len(parts) == 2 {
		label, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if label != "" && value != "" && !isNumeric(label) {
			return label, value, true
		}
	}
	return "", "", false
}

func isNumeric(s string) bool {
	_, ok := parseNumber(s, "")
	return ok
}

// splitValueUnit separates "245.3 nm" into number text and unit.
func splitValueUnit(v string) (string, string) {
	v = strings.TrimSpace(v)
	fields := strings.Fields(v)
	if len(fields) == 2 && unitPattern.MatchString(fields[1]) {
		return fields[0], canonicalUnit(fields[1])
	}
	for _, u := range []string{"nm", "kcps", "Kcps", "Mcps", "%"} {
		if strings.HasSuffix(v, u) && len(v) > len(u) {
			num := strings.TrimSpace(strings.TrimSuffix(v, u))
			if isNumeric(num) {
				return num, canonicalUnit(u)
			}
		}
	}
	return v, ""
}

// parseNumber reads a decimal number. dec is "." or "," (or "" to accept a
// plain dot form only). Thousands separators are not accepted, because they
// are indistinguishable from decimal commas in instrument exports.
func parseNumber(s, dec string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if dec == "," {
		if strings.Contains(s, ".") {
			return 0, false
		}
		s = strings.Replace(s, ",", ".", 1)
	} else if strings.Contains(s, ",") {
		return 0, false
	}
	for _, r := range s {
		if !(unicode.IsDigit(r) || r == '.' || r == '-' || r == '+' || r == 'e' || r == 'E') {
			return 0, false
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseScalar reads a key-value number. When the file gives no table to
// learn the decimal separator from, a single comma is read as a decimal
// comma only when it cannot be a thousands separator (for example "0,182"
// or "312,45"); otherwise the value is left unparsed and flagged.
func parseScalar(s, dec string) (float64, bool) {
	if v, ok := parseNumber(s, dec); ok {
		return v, true
	}
	if dec == "," || strings.Count(s, ",") != 1 || strings.Contains(s, ".") {
		return 0, false
	}
	i := strings.Index(s, ",")
	intPart, frac := strings.TrimLeft(s[:i], "+-"), s[i+1:]
	if intPart == "0" || len(frac) != 3 || len(intPart) > 3 {
		return parseNumber(s, ",")
	}
	return 0, false
}

// segment splits a file into measurement blocks when a summary parameter
// label repeats (several measurements exported in one file).
func segment(lines []line) [][]line {
	var blocks [][]line
	var cur []line
	seen := map[string]bool{}
	for _, l := range lines {
		if label, _, ok := splitKeyValue(l.text); ok {
			base, _ := splitLabelUnit(label)
			if k := lookup(summaryAliases, base); k == model.EffectiveDiameter || k == model.Polydispersity {
				if seen[k] {
					blocks = append(blocks, cur)
					cur = nil
					seen = map[string]bool{}
					// Carry a preceding sample/date header into the new block.
				}
				seen[k] = true
			}
		}
		cur = append(cur, l)
	}
	if len(cur) > 0 {
		blocks = append(blocks, cur)
	}
	// Re-attach header lines (sample, date) that sit just before a repeated
	// summary block to the block they introduce.
	for i := 1; i < len(blocks); i++ {
		prev := blocks[i-1]
		cut := len(prev)
		for cut > 0 {
			l := prev[cut-1]
			if strings.TrimSpace(l.text) == "" {
				cut--
				continue
			}
			label, _, ok := splitKeyValue(l.text)
			if !ok {
				break
			}
			base, _ := splitLabelUnit(label)
			k := lookup(summaryAliases, base)
			if k != model.SampleID && k != model.MeasuredAt {
				break
			}
			cut--
		}
		if cut < len(prev) {
			blocks[i] = append(append([]line{}, prev[cut:]...), blocks[i]...)
			blocks[i-1] = prev[:cut]
		}
	}
	return blocks
}

func splitRow(s, delim string) []string {
	t := strings.TrimSpace(s)
	if delim == " " {
		return strings.Fields(t)
	}
	parts := strings.Split(t, delim)
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// detectTableFormat chooses the delimiter and decimal separator that make
// the longest run of consistent all-numeric rows.
func detectTableFormat(lines []line) (string, string) {
	best, bestDelim, bestDec := 0, "", "."
	for _, delim := range []string{"\t", ";", ",", " "} {
		for _, dec := range []string{".", ","} {
			if delim == "," && dec == "," {
				continue
			}
			run, width := 0, 0
			for _, l := range lines {
				cells := splitRow(l.text, delim)
				ok := len(cells) >= 2
				for _, c := range cells {
					if _, isNum := parseNumber(c, dec); !isNum {
						ok = false
						break
					}
				}
				if ok && (width == 0 || len(cells) == width) {
					run++
					width = len(cells)
					if run > best {
						best, bestDelim, bestDec = run, delim, dec
					}
				} else {
					run, width = 0, 0
					if ok {
						run, width = 1, len(cells)
					}
				}
			}
		}
	}
	if best < 3 {
		return "", "."
	}
	return bestDelim, bestDec
}

type table struct {
	model.Distribution
	headerLine int
}

func findTable(blk []line, delim, dec string) *table {
	if delim == "" {
		return nil
	}
	start, end, width := -1, -1, 0
	bestStart, bestEnd := -1, -1
	for i, l := range blk {
		cells := splitRow(l.text, delim)
		ok := len(cells) >= 2
		for _, c := range cells {
			if _, isNum := parseNumber(c, dec); !isNum {
				ok = false
				break
			}
		}
		if ok && (start < 0 || len(cells) == width) {
			if start < 0 {
				start, width = i, len(cells)
			}
			end = i
			if end-start > bestEnd-bestStart {
				bestStart, bestEnd = start, end
			}
		} else if ok {
			start, end, width = i, i, len(cells)
		} else {
			start, end, width = -1, -1, 0
		}
	}
	if bestStart < 0 || bestEnd-bestStart+1 < 3 {
		return nil
	}
	width = len(splitRow(blk[bestStart].text, delim))
	t := &table{}
	t.FirstLine, t.LastLine = blk[bestStart].n, blk[bestEnd].n
	// Header: nearest non-empty line above with the same number of cells.
	var header []string
	for i := bestStart - 1; i >= 0 && i >= bestStart-3; i-- {
		if strings.TrimSpace(blk[i].text) == "" {
			continue
		}
		cells := splitRow(blk[i].text, delim)
		if delim == " " {
			cells = splitHeaderWhitespace(blk[i].text, width)
		}
		if len(cells) == width {
			header = cells
			t.headerLine = blk[i].n
		}
		break
	}
	t.Columns = make([]model.Column, width)
	for c := 0; c < width; c++ {
		col := &t.Columns[c]
		if header != nil {
			col.Label = header[c]
			base, unit := splitLabelUnit(header[c])
			col.Unit = unit
			col.Key = lookup(columnAliases, base)
			if col.Key == "" {
				// "Intensity %" style headers.
				if f := strings.Fields(base); len(f) == 2 && unitPattern.MatchString(f[1]) {
					col.Key = lookup(columnAliases, f[0])
					col.Unit = canonicalUnit(f[1])
				}
			}
		} else {
			col.Label = fmt.Sprintf("Column %d", c+1)
		}
	}
	for i := bestStart; i <= bestEnd; i++ {
		cells := splitRow(blk[i].text, delim)
		for c := 0; c < width; c++ {
			v, _ := parseNumber(cells[c], dec)
			t.Columns[c].Values = append(t.Columns[c].Values, v)
			t.Columns[c].Raw = append(t.Columns[c].Raw, cells[c])
		}
	}
	// Each key at most once; a duplicated key is not trusted.
	count := map[string]int{}
	for _, c := range t.Columns {
		if c.Key != "" {
			count[c.Key]++
		}
	}
	for i := range t.Columns {
		if count[t.Columns[i].Key] > 1 {
			t.Columns[i].Key = ""
		}
	}
	return t
}

// splitHeaderWhitespace splits a whitespace-aligned header into width
// labels, keeping multi-word labels such as "Particle Size (nm)" together
// when they are separated by two or more spaces.
func splitHeaderWhitespace(s string, width int) []string {
	parts := regexp.MustCompile(`\s{2,}|\t`).Split(strings.TrimSpace(s), -1)
	if len(parts) == width {
		return parts
	}
	return strings.Fields(s)
}

var dateLayouts = []struct {
	layout string
	tz     bool
}{
	{time.RFC3339, true},
	{"2006-01-02T15:04:05", false},
	{"2006-01-02 15:04:05", false},
	{"2006-01-02 15:04", false},
	{"2006-01-02", false},
	{"2006/01/02 15:04:05", false},
	{"2006/01/02", false},
	{"Jan 2, 2006 3:04:05 PM", false},
	{"January 2, 2006 3:04:05 PM", false},
	{"Jan 2, 2006 15:04:05", false},
	{"Jan 2 2006 15:04:05", false},
	{"Mon Jan 2 15:04:05 2006", false},
	{"2 Jan 2006 15:04:05", false},
	{"Jan 2, 2006", false},
	{"January 2, 2006", false},
}

var numericDate = regexp.MustCompile(`^(\d{1,2})[/.\-](\d{1,2})[/.\-](\d{4})(?:[ T,]+(\d{1,2}):(\d{2})(?::(\d{2}))?\s*([AaPp][Mm])?)?$`)

// parseTimestamp reads a measurement date. Day/month order is never guessed
// when both readings are valid: the timestamp is flagged ambiguous and the
// interface asks the user to confirm.
func parseTimestamp(raw string) (*model.Timestamp, bool) {
	s := strings.TrimSpace(raw)
	for _, l := range dateLayouts {
		if t, err := time.Parse(l.layout, s); err == nil {
			return &model.Timestamp{Time: t.UTC(), Raw: raw, TZKnown: l.tz, Source: "file"}, true
		}
	}
	m := numericDate.FindStringSubmatch(s)
	if m == nil {
		return nil, false
	}
	a, _ := strconv.Atoi(m[1])
	b, _ := strconv.Atoi(m[2])
	y, _ := strconv.Atoi(m[3])
	h, mi, se := 0, 0, 0
	if m[4] != "" {
		h, _ = strconv.Atoi(m[4])
		mi, _ = strconv.Atoi(m[5])
		if m[6] != "" {
			se, _ = strconv.Atoi(m[6])
		}
		if ap := strings.ToLower(m[7]); ap == "pm" && h < 12 {
			h += 12
		} else if ap == "am" && h == 12 {
			h = 0
		}
	}
	month, day := a, b // US order (month first), the instrument software default
	ambiguous := false
	switch {
	case a > 12 && b <= 12:
		month, day = b, a
	case a <= 12 && b <= 12 && a != b:
		ambiguous = true
	case a > 12 && b > 12:
		return nil, false
	}
	t := time.Date(y, time.Month(month), day, h, mi, se, 0, time.UTC)
	if t.Month() != time.Month(month) || t.Day() != day {
		return nil, false
	}
	return &model.Timestamp{Time: t, Raw: raw, TZKnown: false, Ambiguous: ambiguous, Source: "file"}, true
}
