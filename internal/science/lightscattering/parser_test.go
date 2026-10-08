package lightscattering

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/model"
)

func load(t *testing.T, name string) Result {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "lightscattering", name))
	if err != nil {
		t.Fatal(err)
	}
	return Parse(b)
}

func param(t *testing.T, m model.Measurement, key string) model.Quantity {
	t.Helper()
	q, ok := m.Params[key]
	if !ok {
		t.Fatalf("missing %s", key)
	}
	return q
}

func TestTabDelimitedFullExport(t *testing.T) {
	r := load(t, "synthetic-tab.txt")
	if r.Status != "parsed" || len(r.Measurements) != 1 || r.Delimiter != "tab" || r.Decimal != "." {
		t.Fatalf("%+v", r)
	}
	m := r.Measurements[0]
	if m.SampleID != "A1" {
		t.Fatalf("sample %q", m.SampleID)
	}
	ed := param(t, m, model.EffectiveDiameter)
	if ed.Value != 245.31 || ed.Unit != "nm" || ed.Raw != "245.31" || ed.Decimals() != 2 {
		t.Fatalf("effective diameter %+v", ed)
	}
	if q := param(t, m, model.Polydispersity); q.Value != 0.182 || q.Unit != "" {
		t.Fatalf("pdi %+v", q)
	}
	if q := param(t, m, model.CountRate); q.Value != 312.4 || q.Unit != "kcps" {
		t.Fatalf("count rate %+v", q)
	}
	if q := param(t, m, model.BaselineIndex); q.Value != 9.1 {
		t.Fatalf("baseline %+v", q)
	}
	// Unrecognized field kept verbatim, never dropped.
	found := false
	for _, f := range m.Fields {
		if f.Label == "Temperature (C)" && f.Key == "" && f.Unit == "°C" && *f.Num == 25 {
			found = true
		}
	}
	if !found {
		t.Fatal("unrecognized field not preserved")
	}
	d := m.Dist
	if d == nil || len(d.Columns) != 4 {
		t.Fatalf("distribution %+v", d)
	}
	dia, in := d.Column("diameter"), d.Column("intensity")
	if dia.Unit != "nm" || in.Unit != "%" || len(dia.Values) != 50 || d.Column("volume") == nil || d.Column("number") == nil {
		t.Fatalf("columns %+v", d.Columns)
	}
	if dia.Values[0] != 10 || dia.Raw[0] != "10.00" {
		t.Fatalf("first diameter %v", dia.Values[0])
	}
	sum := 0.0
	for _, v := range in.Values {
		sum += v
	}
	if math.Abs(sum-100) > 0.01 {
		t.Fatalf("intensity sum %.4f", sum)
	}
	// 10/05/2026 is a valid date in both orders: flagged, not guessed.
	if m.MeasuredAt == nil || !m.MeasuredAt.Ambiguous {
		t.Fatalf("date must be ambiguous: %+v", m.MeasuredAt)
	}
}

func TestSemicolonDecimalCommaLegacyEncoding(t *testing.T) {
	r := load(t, "synthetic-semicolon-comma.txt")
	if r.Status != "parsed" || r.Encoding != "Windows-1252" || r.Delimiter != "semicolon" || r.Decimal != "," {
		t.Fatalf("%+v", r)
	}
	m := r.Measurements[0]
	if q := param(t, m, model.EffectiveDiameter); q.Value != 251.07 || q.Unit != "nm" {
		t.Fatalf("ed %+v", q)
	}
	if q := param(t, m, model.Polydispersity); q.Value != 0.205 {
		t.Fatalf("pdi %+v", q)
	}
	if q := param(t, m, model.CountRate); q.Value != 298.7 || q.Unit != "kcps" {
		t.Fatalf("cr %+v", q)
	}
	if m.Dist.Column("diameter").Values[1] != 11.51 {
		t.Fatal("decimal comma table misread")
	}
	if m.MeasuredAt == nil || m.MeasuredAt.Time.Format("2006-01-02 15:04") != "2026-10-12 09:05" || m.MeasuredAt.Ambiguous {
		t.Fatalf("separate date and time not combined: %+v", m.MeasuredAt)
	}
	ok := false
	for _, f := range m.Fields {
		if f.Label == "Amostra" && strings.Contains(f.Text, "réplica") {
			ok = true
		}
	}
	if !ok {
		t.Fatal("accented unrecognized field lost")
	}
}

func TestTwoMeasurementsInOneFile(t *testing.T) {
	r := load(t, "synthetic-two-runs.txt")
	if len(r.Measurements) != 2 || r.Delimiter != "whitespace" {
		t.Fatalf("%d measurements, delim %s", len(r.Measurements), r.Delimiter)
	}
	a, b := r.Measurements[0], r.Measurements[1]
	if param(t, a, model.EffectiveDiameter).Value != 239.8 || param(t, b, model.EffectiveDiameter).Value != 243.1 {
		t.Fatal("runs mixed up")
	}
	if a.MeasuredAt.Time.Minute() != 0 || b.MeasuredAt.Time.Minute() != 4 {
		t.Fatal("dates not attached to their runs")
	}
	if a.MeasuredAt.Ambiguous {
		t.Fatal("10/19/2026 is not ambiguous")
	}
	if q := param(t, b, model.CountRate); q.Unit != "kcps" {
		t.Fatalf("unit from header label lost: %+v", q)
	}
	if a.Dist == nil || b.Dist == nil || len(a.Dist.Column("intensity").Values) != 50 {
		t.Fatal("each run needs its own distribution")
	}
}

func TestMissingFieldsAreAbsentNotZero(t *testing.T) {
	r := load(t, "synthetic-missing-fields.txt")
	if r.Status != "partial" {
		t.Fatalf("status %s", r.Status)
	}
	m := r.Measurements[0]
	if _, ok := m.Params[model.BaselineIndex]; ok {
		t.Fatal("absent BaseLine Index must not appear")
	}
	if m.MeasuredAt != nil {
		t.Fatal("absent date must not be invented")
	}
	if m.Dist != nil {
		t.Fatal("absent distribution must not be invented")
	}
	joined := strings.Join(r.Warnings, " ")
	for _, w := range []string{"ls.no_distribution", "ls.no_measurement_date", "ls.unit_missing:count_rate"} {
		if !strings.Contains(joined, w) {
			t.Fatalf("warning %s missing: %v", w, r.Warnings)
		}
	}
}

func TestMalformedRejected(t *testing.T) {
	for _, c := range []struct {
		data []byte
		err  error
	}{
		{nil, ErrEmpty},
		{[]byte{0x89, 'P', 'N', 'G', 0, 0, 0, 1}, ErrBinary},
		{[]byte("just some notes\nno data here\n"), ErrUnrecognized},
	} {
		r := Parse(c.data)
		if r.Status != "failed" || r.Error != c.err.Error() {
			t.Fatalf("%q: %+v", c.data, r)
		}
	}
	if r := load(t, "malformed-random.txt"); r.Status != "failed" {
		t.Fatalf("random text accepted: %+v", r)
	}
}

func TestAmbiguousDate(t *testing.T) {
	r := load(t, "synthetic-ambiguous-date.txt")
	if ts := r.Measurements[0].MeasuredAt; ts == nil || !ts.Ambiguous || ts.Confirmed {
		t.Fatalf("%+v", ts)
	}
	if ts, ok := parseTimestamp("25/04/2026"); !ok || ts.Ambiguous || ts.Time.Month() != 4 || ts.Time.Day() != 25 {
		t.Fatalf("unambiguous day-first: %+v", ts)
	}
	if ts, ok := parseTimestamp("2026-10-05T14:00:00-03:00"); !ok || !ts.TZKnown || ts.Time.Hour() != 17 {
		t.Fatalf("tz: %+v", ts)
	}
	if _, ok := parseTimestamp("31/31/2026"); ok {
		t.Fatal("invalid date accepted")
	}
}

func TestScalarDecimalComma(t *testing.T) {
	for in, want := range map[string]float64{"0,182": 0.182, "312,45": 312.45, "1234,5": 1234.5} {
		if v, ok := parseScalar(in, "."); !ok || v != want {
			t.Fatalf("%s → %v %v", in, v, ok)
		}
	}
	if _, ok := parseScalar("1,234", "."); ok {
		t.Fatal("1,234 is ambiguous (thousands or decimal) and must not be guessed")
	}
}

func TestUTF16(t *testing.T) {
	src := "Effective Diameter (nm): 200.5\nPolydispersity: 0.1\n"
	b := []byte{0xFF, 0xFE}
	for _, r := range src {
		b = append(b, byte(r), 0)
	}
	r := Parse(b)
	if r.Encoding != "UTF-16" || r.Measurements[0].Params[model.EffectiveDiameter].Value != 200.5 {
		t.Fatalf("%+v", r)
	}
}

func TestQuotedTabularRecords(t *testing.T) {
	for _, tc := range []struct {
		name, separator, decimal, number, delimiter string
	}{
		{"csv", ",", ".", "200.50", "comma"},
		{"csv_decimal_comma", ",", ",", "200,50", "comma"},
		{"tsv", "\t", ".", "200.50", "tab"},
		{"semicolon", ";", ",", "200,50", "semicolon"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			quoted := func(cells ...string) string {
				for i := range cells {
					cells[i] = `"` + strings.ReplaceAll(cells[i], `"`, `""`) + `"`
				}
				return strings.Join(cells, tc.separator) + "\n"
			}
			src := quoted("Sample ID", "Synthetic "+tc.separator+` "sample"`) +
				quoted("Date", "2026-10-19 09:05:00") +
				quoted("Effective Diameter (nm)", tc.number) +
				quoted("Polydispersity", "0"+tc.decimal+"12") +
				quoted("Notes", "first line\nsecond line") +
				quoted("Diameter\n(nm)", "Intensity (%)") +
				quoted("10"+tc.decimal+"00", "20"+tc.decimal+"0") +
				quoted("20"+tc.decimal+"00", "30"+tc.decimal+"0") +
				quoted("30"+tc.decimal+"00", "50"+tc.decimal+"0")
			r := Parse([]byte(src))
			if r.Status != "parsed" || len(r.Measurements) != 1 || r.Delimiter != tc.delimiter || r.Decimal != tc.decimal {
				t.Fatalf("%+v", r)
			}
			m := r.Measurements[0]
			if q := param(t, m, model.EffectiveDiameter); q.Value != 200.5 || q.Raw != tc.number || q.Unit != "nm" || q.Line != 3 {
				t.Fatalf("quantity %+v", q)
			}
			if m.SampleID != "Synthetic "+tc.separator+` "sample"` || m.MeasuredAt == nil || m.MeasuredAt.Time.Minute() != 5 {
				t.Fatalf("metadata %+v", m)
			}
			if m.Dist.FirstLine != 9 || m.Dist.LastLine != 11 || m.Dist.Column("diameter").Values[2] != 30 || m.Dist.Column("intensity").Values[2] != 50 {
				t.Fatalf("distribution %+v", m.Dist)
			}
			found := false
			for _, f := range m.Fields {
				if f.Label == "Notes" && f.Text == "first line\nsecond line" && f.Line == 5 {
					found = true
				}
			}
			if !found {
				t.Fatal("quoted multiline unknown field was not retained")
			}
		})
	}
}

func TestTabularSummaryWithoutDistribution(t *testing.T) {
	for _, separator := range []string{",", ";", "\t"} {
		src := strings.ReplaceAll("Effective Diameter (nm)|200.50\nPolydispersity|0.12\nDate|2026-10-19 09:05:00\n", "|", separator)
		r := Parse([]byte(src))
		if r.Status != "partial" || len(r.Measurements) != 1 || r.Measurements[0].Dist != nil || r.Measurements[0].MeasuredAt == nil {
			t.Fatalf("separator %q: %+v", separator, r)
		}
		if q := param(t, r.Measurements[0], model.EffectiveDiameter); q.Value != 200.5 || q.Raw != "200.50" {
			t.Fatalf("quantity %+v", q)
		}
	}
}

func TestIncompleteDistributionIsNeverTruncated(t *testing.T) {
	for _, bad := range []string{"50,missing", "50,", "50", "50,20,30", `"50",bad"quote`, "missing,50", "NaN,50", "bad:5,50", "Notes: invalid row", "Polydispersity,0.12"} {
		t.Run(bad, func(t *testing.T) {
			src := "Effective Diameter (nm): 200.5\nDiameter (nm),Intensity (%)\n10,10\n20,20\n30,30\n40,40\n" + bad + "\n60,60\n70,70\n80,80\n"
			r := Parse([]byte(src))
			if r.Status != "partial" || len(r.Measurements) != 1 || r.Measurements[0].Dist != nil {
				t.Fatalf("damaged table must not be plotted: %+v", r)
			}
			if !strings.Contains(strings.Join(r.Warnings, " "), "ls.incomplete_distribution:7") {
				t.Fatalf("missing row warning: %v", r.Warnings)
			}
			if q := param(t, r.Measurements[0], model.EffectiveDiameter); q.Value != 200.5 {
				t.Fatal("independent scalar lost")
			}
		})
	}
}

func TestMultipleDistributionsRequireReview(t *testing.T) {
	src := "Effective Diameter (nm): 200.5\nDiameter (nm),Intensity (%)\n10,10\n20,20\n30,30\n40,40\n\nDiameter (nm),Intensity (%)\n50,50\n60,60\n70,70\n"
	r := Parse([]byte(src))
	if r.Status != "partial" || len(r.Measurements) != 1 || r.Measurements[0].Dist != nil || !strings.Contains(strings.Join(r.Warnings, " "), "ls.multiple_distributions") {
		t.Fatalf("independent tables silently selected or merged: %+v", r)
	}
	// A damaged distribution alone must not invent a measurement or values.
	r = Parse([]byte("Diameter (nm),Intensity (%)\n10,10\n20,missing\n30,30\n40,40\n50,50\n"))
	if r.Status != "failed" || len(r.Measurements) != 0 || !strings.Contains(strings.Join(r.Warnings, " "), "ls.incomplete_distribution:3") {
		t.Fatalf("%+v", r)
	}
}

func TestInvalidClockIsNotNormalized(t *testing.T) {
	for _, raw := range []string{
		"19/10/2026 09:75:00", "19/10/2026 09:00:75", "19/10/2026 24:00:00",
		"19/10/2026 00:15:00 AM", "19/10/2026 13:15:00 PM",
	} {
		if ts, ok := parseTimestamp(raw); ok {
			t.Fatalf("invalid source clock %q became %+v", raw, ts)
		}
	}
	for raw, want := range map[string]string{
		"19/10/2026 12:15:00 AM": "2026-10-19 00:15:00",
		"19/10/2026 12:15:00 PM": "2026-10-19 12:15:00",
		"19/10/2026 11:59:59 PM": "2026-10-19 23:59:59",
	} {
		if ts, ok := parseTimestamp(raw); !ok || ts.Time.Format("2006-01-02 15:04:05") != want {
			t.Fatalf("valid source clock %q: %+v", raw, ts)
		}
	}
}

func TestTextStructureLimits(t *testing.T) {
	for _, src := range []string{
		strings.Repeat("\n", maxTextLines+1),
		"Effective Diameter (nm),200.5\n" + strings.Repeat(",", maxTextColumns) + "\n",
		"Effective Diameter (nm): 200.5\n" + strings.Repeat("1 ", maxTextColumns+1) + "\n",
		"Effective Diameter (nm)\t200.5\n" + strings.Repeat("\t", maxTextColumns) + "\n",
		"Effective Diameter (nm),200.5\n" + strings.Repeat(strings.Repeat("1,", 10)+"1\n", maxTextCells/11+1),
	} {
		if len(src) >= MaxFileSize {
			t.Fatal("the structure test must remain below the byte-size limit")
		}
		r := Parse([]byte(src))
		if r.Status != "failed" || r.Error != ErrTextLimit.Error() || len(r.Measurements) != 0 {
			t.Fatalf("unsafe text structure was accepted: %+v", r)
		}
	}
	// Delimiters inside a quoted value are text rather than thousands of
	// columns. Quoted line breaks also retain their original field meaning.
	src := "Notes,\"" + strings.Repeat(",", maxTextColumns+1) + "\"\nEffective Diameter (nm),200.50\n"
	r := Parse([]byte(src))
	if r.Status != "partial" || len(r.Measurements) != 1 {
		t.Fatalf("quoted content was counted as structure: %+v", r)
	}
	// An exactly full cell budget must not count the ending newline as an
	// additional empty record.
	if !withinCellLimits(strings.Repeat("1,1,1,1,1,1,1,1,1,1\n", maxTextCells/10), ',') {
		t.Fatal("a trailing newline consumed an extra cell")
	}
}

func TestWrongDelimiterLimitDoesNotRejectTextValues(t *testing.T) {
	note := strings.Repeat("note,", maxTextColumns+100)
	src := "Effective Diameter (nm)\t200.5\nDate\t2026-10-19 09:05:00\nNotes\t" + note +
		"\nDiameter (nm)\tIntensity (%)\n10\t10\n20\t20\n30\t70\n"
	r := Parse([]byte(src))
	if r.Status != "parsed" || r.Delimiter != "tab" || len(r.Measurements) != 1 || r.Measurements[0].Dist == nil {
		t.Fatalf("a bounded TSV was rejected by the comma candidate: %+v", r)
	}
	found := false
	for _, f := range r.Measurements[0].Fields {
		if f.Label == "Notes" && f.Text == note {
			found = true
		}
	}
	if !found {
		t.Fatal("the note lost its commas")
	}
	// The fallback recognizes exactly two tab-separated fields without
	// allocating an array for every separator of an incompatible record.
	if _, _, ok := splitKeyValue("Notes\t" + strings.Repeat("value\t", maxTextColumns+100)); ok {
		t.Fatal("a multi-column record was treated as one scalar")
	}
}
