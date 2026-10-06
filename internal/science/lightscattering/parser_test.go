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
