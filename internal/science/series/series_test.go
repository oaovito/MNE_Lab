package series

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/design"
	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/model"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "lightscattering", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// input imports parsed files the way the application does: random-looking
// fixed IDs, measurements by position, readings bound to measurement IDs.
func input(t *testing.T, names ...string) Input {
	in := Input{Measurements: map[string]model.Measurement{}, Readings: map[string]model.FileReadings{}}
	for fi, name := range names {
		r := lightscattering.Parse(fixture(t, name))
		f := model.SourceFile{ID: fmt.Sprintf("file-%d", fi), Name: name, SHA256: "invented", Parser: r.Parser, Spec: r.Spec}
		for i, m := range r.Measurements {
			m.ID, m.FileID, m.Index = fmt.Sprintf("meas-%d-%d", fi, i), f.ID, i
			in.Measurements[m.ID] = m
			f.Measurements = append(f.Measurements, m.ID)
		}
		fr := model.FileReadings{FileID: f.ID, Parser: r.Parser}
		for _, tb := range r.Readings {
			tb.MeasurementID = f.Measurements[tb.Measurement]
			fr.Tables = append(fr.Tables, tb)
		}
		in.Readings[f.ID] = fr
		in.Files = append(in.Files, f)
	}
	return in
}

func TestIDsAreDeterministicAndParse(t *testing.T) {
	in := input(t, "synthetic-nanobrook-methods.txt", "synthetic-tab.txt", "synthetic-nanobrook-run-table.txt")
	a, b := Build(in), Build(in)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatal(len(a), len(b))
	}
	seen := map[string]bool{}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Fatalf("not deterministic: %s %s", a[i].ID, b[i].ID)
		}
		if seen[a[i].ID] {
			t.Fatalf("duplicate ID %s", a[i].ID)
		}
		seen[a[i].ID] = true
		ref, err := Parse(a[i].ID)
		if err != nil || ref.Kind != a[i].Kind || ref.MeasurementID != a[i].MeasurementID {
			t.Fatalf("%s: %+v %v", a[i].ID, ref, err)
		}
	}
	for _, bad := range []string{"", "x", "m:a", "m:a/q:b", "m:a/r:0/p:x", "m:a/d:b/x:c", "m:/p:x"} {
		if _, err := Parse(bad); err == nil {
			t.Fatalf("parsed %q", bad)
		}
	}
	// A label, a sample name or a file name never takes part in identity.
	for id, m := range in.Measurements {
		m.Label, m.SampleID = "Renamed", "Renamed sample"
		in.Measurements[id] = m
	}
	in.Files[0].Name = "zzz renamed.txt"
	c := map[string]bool{}
	for _, s := range Build(in) {
		c[s.ID] = true
	}
	for id := range seen {
		if !c[id] {
			t.Fatalf("renaming changed %s", id)
		}
	}
}

func TestCandidatesAndWeightingsAreDistinctSeries(t *testing.T) {
	in := input(t, "synthetic-nanobrook-methods.txt")
	var curves []Series
	for _, s := range Build(in) {
		if s.Kind == KindCurve {
			curves = append(curves, s)
		}
	}
	m := in.Measurements["meas-0-0"]
	want := 0
	for _, d := range m.Distributions {
		want += len(d.Weightings())
	}
	if len(curves) != want || want < 2 {
		t.Fatalf("%d curves, want %d", len(curves), want)
	}
	for _, c := range curves {
		if c.X == nil || c.X.Key != "diameter" || c.X.Unit != "nm" || c.Distribution == nil || c.Distribution.Selected {
			t.Fatalf("%+v", c)
		}
	}
}

func TestTwentySeriesFileHasTwentyIDs(t *testing.T) {
	// Twenty invented runs in one native report: twenty readings of one
	// recognized column, all distinct, none an experimental unit.
	text := "    **** Brookhaven Instruments Corp.****\nSynthetic; every value invented.\nSample Identification: SYNTHETIC-TWENTY\nEffective Diameter: 100.0\n\n Run   Eff. Diam. (nm)\n"
	for i := 1; i <= 20; i++ {
		text += fmt.Sprintf("  %d   %.1f\n", i, 95+float64(i)*0.5)
	}
	text += "Combined   100.0\n\n    **** Lognormal Size Distribution Results ****\nd(nm) G(d) C(d)\n20 0.5 1\n40 0.5 2\n"
	r := lightscattering.Parse([]byte(text))
	if len(r.Measurements) != 1 || len(r.Readings) != 1 {
		t.Fatalf("%+v %+v", r.Warnings, r.Readings)
	}
	m := r.Measurements[0]
	m.ID, m.FileID = "meas-twenty", "file-twenty"
	tb := r.Readings[0]
	tb.MeasurementID = m.ID
	in := Input{Files: []model.SourceFile{{ID: "file-twenty", Name: "twenty.txt", Measurements: []string{m.ID}}}, Measurements: map[string]model.Measurement{m.ID: m},
		Readings: map[string]model.FileReadings{"file-twenty": {FileID: "file-twenty", Tables: []model.ReadingTable{tb}}}}
	ids := map[string]bool{}
	for _, s := range Build(in) {
		if s.Kind != KindReading {
			continue
		}
		ids[s.ID] = true
		if s.ReplicateKind != design.ReplicateInstrumentReading || s.Y.Unit != "nm" || s.Y.Label != "Eff. Diam. (nm)" || s.Provenance.UnitOrigin != "file_table_header" {
			t.Fatalf("%+v", s)
		}
	}
	if len(ids) != 20 {
		t.Fatalf("%d reading IDs", len(ids))
	}
}

// B§62 on the catalog: one report of unit A with twelve run readings is
// still one measurement of one experimental unit.
func TestTwelveReadingsBelongToOneUnit(t *testing.T) {
	in := input(t, "synthetic-nanobrook-twelve-runs.txt")
	d := design.Normalize(design.Design{Name: "Synthetic", Units: []design.Unit{{ID: "unit-a", Label: "A"}}, Conditions: []design.Condition{{Label: "F-SR"}},
		Times: []design.TimeLevel{{Label: "Day 7"}}, Assignments: []design.Assignment{{MeasurementID: "meas-0-0", UnitID: "unit-a", Condition: "F-SR", Time: "Day 7"}}})
	in.Design, in.UnitLabels = &d, map[string]string{"unit-a": "A"}
	runs, units := map[int]bool{}, map[string]bool{}
	for _, s := range Build(in) {
		units[s.ExperimentalUnitID] = true
		if s.Kind == KindReading {
			runs[s.Reading.Index] = true
			if s.ReplicateKind != design.ReplicateInstrumentReading || s.Condition != "F-SR" || s.Time == nil || s.Time.Label != "Day 7" || s.ExperimentalUnit != "A" {
				t.Fatalf("%+v", s)
			}
		}
	}
	if len(runs) != 12 || len(units) != 1 || !units["unit-a"] {
		t.Fatalf("runs %d units %v", len(runs), units)
	}
	if s := design.Detect(d, nil); s.ExperimentalUnits != 1 || s.Measurements != 1 {
		t.Fatalf("%+v", s)
	}
}

func TestCatalogNeverChangesItsInput(t *testing.T) {
	in := input(t, "synthetic-nanobrook-run-table.txt", "synthetic-two-runs.txt")
	before, _ := json.Marshal(in)
	list := Build(in)
	Filter(list, "diam")
	after, _ := json.Marshal(in)
	if string(before) != string(after) {
		t.Fatal("the catalog changed its input")
	}
	if got := Filter(list, "eff. diam"); len(got) == 0 {
		t.Fatal("search found nothing")
	}
}
