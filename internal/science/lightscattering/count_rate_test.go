package lightscattering

import (
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/xuri/excelize/v2"
)

func TestCurrentAndAverageCountRateAreIndependent(t *testing.T) {
	r := load(t, "synthetic-count-rate-types.txt")
	if r.Status != "parsed" || len(r.Measurements) != 1 {
		t.Fatalf("unexpected parse: %s, %d measurements", r.Status, len(r.Measurements))
	}
	m := r.Measurements[0]
	for _, want := range []struct {
		key string
		q   model.Quantity
	}{
		{model.CountRate, model.Quantity{Value: 73.250, Raw: "73.250", Unit: "kcps", Label: "Current Count Rate", Line: 4}},
		{model.AverageCountRate, model.Quantity{Value: 0.18450, Raw: "0.18450", Unit: "Mcps", Label: "Average Count Rate", Line: 5}},
	} {
		if got := param(t, m, want.key); got != want.q {
			t.Errorf("%s: got %+v, want %+v", want.key, got, want.q)
		}
		found := false
		for _, f := range m.Fields {
			if f.Key == want.key && f.Label == want.q.Label && f.Line == want.q.Line && f.Unit == want.q.Unit && f.Num != nil && *f.Num == want.q.Value {
				found = true
			}
		}
		if !found {
			t.Errorf("recognized field provenance missing for %s", want.key)
		}
	}
}

func TestAverageOnlyDoesNotInventCurrentCountRate(t *testing.T) {
	for _, label := range []string{"Average Count Rate", "Avg. Count Rate"} {
		t.Run(label, func(t *testing.T) {
			r := Parse([]byte("Sample ID: SYNTHETIC-AVERAGE-ONLY\n" + label + " (kcps): 41.625\n"))
			if len(r.Measurements) != 1 {
				t.Fatalf("measurements: %d", len(r.Measurements))
			}
			m := r.Measurements[0]
			if _, ok := m.Params[model.CountRate]; ok {
				t.Fatal("average must not populate the current count rate")
			}
			q := param(t, m, model.AverageCountRate)
			if q.Value != 41.625 || q.Raw != "41.625" || q.Unit != "kcps" || q.Label != label+" (kcps)" || q.Line != 2 {
				t.Fatalf("average: %+v", q)
			}
		})
	}
}

func TestAverageCountRateMissingUnitIsNotInferred(t *testing.T) {
	r := Parse([]byte("Sample ID: SYNTHETIC-UNKNOWN-UNIT\nAverage Count Rate: 0.12500\n"))
	if len(r.Measurements) != 1 {
		t.Fatalf("measurements: %d", len(r.Measurements))
	}
	q := param(t, r.Measurements[0], model.AverageCountRate)
	if q.Unit != "" || q.Raw != "0.12500" {
		t.Fatalf("missing unit or precision rewritten: %+v", q)
	}
	if !strings.Contains(strings.Join(r.Warnings, " "), "ls.unit_missing:average_count_rate:") {
		t.Fatalf("missing unit warning absent: %v", r.Warnings)
	}
}

func TestXLSXCurrentAndAverageCountRatePreserveSourceRows(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	rows := map[string][]any{
		"A1": {"Sample ID", "SYNTHETIC-COUNT-RATE-WORKBOOK"},
		"A3": {"Current Count Rate (kcps)", "73.250"},
		"A5": {"Average Count Rate (Mcps)", "0.18450"},
	}
	for cell, row := range rows {
		if err := f.SetSheetRow("Sheet1", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	r, format := ParseFile("synthetic-count-rates.xlsx", b.Bytes())
	if format != "xlsx" || len(r.Measurements) != 1 {
		t.Fatalf("format %q, measurements %d", format, len(r.Measurements))
	}
	m := r.Measurements[0]
	current, average := param(t, m, model.CountRate), param(t, m, model.AverageCountRate)
	if m.SourceSheet != "Sheet1" || current.Line != 3 || average.Line != 5 || current.Raw != "73.250" || average.Raw != "0.18450" || current.Unit != "kcps" || average.Unit != "Mcps" {
		t.Fatalf("source rows or quantities changed: sheet %q, current %+v, average %+v", m.SourceSheet, current, average)
	}
}
