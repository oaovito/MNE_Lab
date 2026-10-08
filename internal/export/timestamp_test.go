package export

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/xuri/excelize/v2"
)

func TestExportTimestampProvenancePreservesFlagsAndNativeSource(t *testing.T) {
	in, ids, _ := countRateExportInput(t)
	m := in.Measurements[ids[0]]
	m.SourceSheet = "Synthetic source sheet"
	m.SourceRange = "C4:D20"
	m.MeasuredAt.Ambiguous, m.MeasuredAt.Confirmed = true, true
	m.Dist.ID, m.Dist.Method, m.Dist.Format = "synthetic-candidate", "lognormal", "spreadsheet"
	in.Measurements[m.ID] = m
	ds := MeasurementData("Synthetic timestamp export", ids[:1], in, nil, tr)
	s := ds.Provenance[0]
	if s.MeasuredTimestamp == nil || *s.MeasuredTimestamp != *m.MeasuredAt || s.MeasuredAt != FormatTime(m.MeasuredAt) || s.SourceSheet != m.SourceSheet || s.SourceRange != m.SourceRange || s.DistributionID != m.Dist.ID || s.Method != m.Dist.Method || s.Format != m.Dist.Format {
		t.Fatal("normalized export lost source context")
	}
	b, err := dataJSON(ds)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Provenance []graph.Source `json:"provenance"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	ts := out.Provenance[0].MeasuredTimestamp
	if ts == nil || ts.TZKnown || !ts.Ambiguous || !ts.Confirmed || ts.Raw != m.MeasuredAt.Raw {
		t.Fatal("JSON lost raw or explicit unknown zone")
	}
	b, err = WriteData(ds, "xlsx", DataOptions{})
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for cell, want := range map[string]string{"G2": s.MeasuredAt, "I2": ts.Raw, "J2": "FALSE", "K2": "TRUE", "L2": "TRUE", "M2": "file", "N2": m.SourceSheet, "O2": m.Dist.Method, "P2": m.Dist.Format, "Q2": m.SourceRange} {
		got, err := f.GetCellValue("Provenance", cell)
		if err != nil || got != want {
			t.Fatalf("%s: got %q want %q: %v", cell, got, want, err)
		}
	}
	textBytes, err := WriteData(ds, "txt", DataOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if text := string(textBytes); !strings.Contains(text, `"tzKnown":false`) || !strings.Contains(text, ts.Raw) {
		t.Fatal("TXT provenance lost time semantics")
	}
}
