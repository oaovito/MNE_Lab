package lightscattering

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/model"
)

func nanoFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "lightscattering", "synthetic-nanobrook-methods.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestNanoBrookKeepsMethodsAndRepresentations(t *testing.T) {
	r := Parse(nanoFixture(t))
	if r.Status != "partial" || len(r.Measurements) != 1 {
		t.Fatalf("%+v", r)
	}
	m := r.Measurements[0]
	if m.Dist != nil || m.DistributionID != "" {
		t.Fatal("an algorithm was silently selected")
	}
	for _, warning := range r.Warnings {
		if strings.HasPrefix(warning, "ls.no_distribution:") {
			t.Fatal("available alternatives were reported as missing data")
		}
	}
	if m.SampleID != "SYNTHETIC-METHOD-REVIEW" || m.MeasuredAt == nil || m.MeasuredAt.Time.Hour() != 14 {
		t.Fatal("instrument identification or time lost")
	}
	if len(m.Distributions) != 4 {
		t.Fatalf("alternatives: %d, warnings: %v", len(m.Distributions), r.Warnings)
	}
	for i, d := range m.Distributions {
		if d.ID == "" || d.Column("diameter").Unit != "nm" || d.Column("intensity").Unit != "" {
			t.Fatalf("units/provenance invented: %+v", d)
		}
		if len(d.SourceLines) != len(d.Columns[0].Values) || len(d.Columns[0].Values) != len(d.Columns[1].Values) {
			t.Fatal("point/source associations lost")
		}
		if i < 2 && d.Format != "report" || i >= 2 && d.Format != "spreadsheet" {
			t.Fatal("representations mixed")
		}
		for _, f := range d.Auxiliary {
			if f.Key != "" || f.Label != "C(d)" {
				t.Fatal("auxiliary column was reinterpreted")
			}
		}
	}
	if !reflect.DeepEqual(m.Distributions[0].Columns[0].Values, []float64{20, 40, 80, 160, 320}) {
		t.Fatal("wrapped groups read in row order")
	}
	if !reflect.DeepEqual(m.Distributions[0].SourceLines, []int{14, 15, 16, 14, 15}) {
		t.Fatal("flattened source rows incorrect")
	}
	if m.Distributions[2].Columns[1].Raw[0] != "0.0213" || m.Distributions[0].Columns[1].Raw[0] != "0.02" {
		t.Fatal("rounded report substituted for precise spreadsheet data")
	}
	if len(m.Distributions[3].Auxiliary) != 3 || len(m.Distributions[3].Columns[0].Values) != 4 {
		t.Fatal("missing auxiliary value replaced with zero or known values discarded")
	}
}

func TestNanoBrookDamagedCandidateIsNotReducedToFragment(t *testing.T) {
	text := string(nanoFixture(t))
	for _, change := range [][2]string{{"40,0.0634,12", "40,missing,12"}, {"40 0.06 12 | 320 0.01 100", "40 0.06 | 320 0.01 100"}, {"20 0.02 1 | 160 0.05 99", "20 0.02 1 |"}} {
		r := Parse([]byte(strings.Replace(text, change[0], change[1], 1)))
		if len(r.Measurements) != 1 || len(r.Measurements[0].Distributions) != 3 || r.Measurements[0].Dist != nil {
			t.Fatalf("candidate damage accepted: %+v", r)
		}
		found := false
		for _, w := range r.Warnings {
			if strings.HasPrefix(w, "ls.nanobrook_invalid_distribution:") {
				found = true
			}
		}
		if !found {
			t.Fatal("damage was not reported")
		}
	}
}

func TestNanoBrookUnlabeledSpreadsheetRequiresMatchingHeader(t *testing.T) {
	text := "**** Brookhaven Instruments Corp.****\nEffective Diameter: 77\n**** Multimodal Size Distribution Results/Spreadsheet Format ****\n10,1,25\n20,2,75\n30,0,100\n"
	r := Parse([]byte(text))
	if len(r.Measurements) != 1 || len(r.Measurements[0].Distributions) != 0 || r.Measurements[0].Dist != nil {
		t.Fatal("unlabeled CSV column meanings guessed")
	}
	if !strings.Contains(strings.Join(r.Warnings, "|"), "ls.nanobrook_missing_header:") {
		t.Fatal("missing header not reported")
	}
}

func TestNanoBrookMarkerNeedsInstrumentIdentity(t *testing.T) {
	r := Parse([]byte(strings.Replace(string(nanoFixture(t)), "Brookhaven Instruments Corp.", "Unidentified export", 1)))
	if len(r.Measurements) != 1 || len(r.Measurements[0].Distributions) != 0 {
		t.Fatal("generic numeric data promoted to native instrument format")
	}
}

// A text file is recorded as text: its name or content never establishes
// the instrument that produced it, and a failed file is not labeled either.
func TestTextFormatNamesNoProducer(t *testing.T) {
	for name, data := range map[string]string{
		"generic.txt": "Effective Diameter (nm): 200.5\n",
		"failed.csv":  "\x00\x01\x02",
	} {
		if _, format := ParseFile(name, []byte(data)); format != "text" {
			t.Fatalf("%s: format %q", name, format)
		}
	}
}

func TestNanoBrookUnitFromRunTable(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "lightscattering", "synthetic-nanobrook-run-table.txt"))
	if err != nil {
		t.Fatal(err)
	}
	r := Parse(b)
	if len(r.Measurements) != 1 {
		t.Fatalf("%+v", r)
	}
	q := r.Measurements[0].Params[model.EffectiveDiameter]
	if q.Raw != "87.1" || q.Unit != "nm" || q.UnitOrigin != "file_table_header" || q.Line != 13 || q.UnitLine != 17 {
		t.Fatalf("effective diameter: %+v", q)
	}
	for _, k := range []string{model.Polydispersity, model.BaselineIndex} {
		if u := r.Measurements[0].Params[k].Unit; u != "" {
			t.Fatalf("%s got unit %q the file never states", k, u)
		}
	}
	if q := r.Measurements[0].Params[model.AverageCountRate]; q.Unit != "kcps" || q.UnitOrigin != "" {
		t.Fatalf("stated unit changed: %+v", q)
	}
	for _, w := range r.Warnings {
		if strings.HasPrefix(w, "ls.unit_missing:"+model.EffectiveDiameter) {
			t.Fatal("unit still reported missing")
		}
	}

	// A Combined row with another value is not the same number: no unit.
	other := strings.Replace(string(b), "Combined     87.1", "Combined     87.4", 1)
	q = Parse([]byte(other)).Measurements[0].Params[model.EffectiveDiameter]
	if q.Unit != "" || q.UnitOrigin != "" {
		t.Fatalf("unit adopted from a different value: %+v", q)
	}
}
