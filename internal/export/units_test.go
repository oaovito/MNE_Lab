package export

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
)

func TestExportsNoteUnitsTheFilesDoNotState(t *testing.T) {
	m := model.Measurement{ID: "m1", Params: map[string]model.Quantity{
		model.EffectiveDiameter: {Value: 87.1, Raw: "87.1"},
		model.Polydispersity:    {Value: 0.1, Raw: "0.1"},
	}}
	ds := MeasurementData("t", []string{"m1"}, graph.Input{Measurements: map[string]model.Measurement{"m1": m}}, nil, tr)
	if len(ds.Notes) != 1 || ds.Notes[0] != NoteUnitNotStated+model.EffectiveDiameter {
		t.Fatalf("notes %v", ds.Notes)
	}
	txt, err := WriteData(ds, "txt", DataOptions{})
	if err != nil || !strings.Contains(string(txt), "Effective Diameter: unit not stated in the source files") {
		t.Fatalf("text export lost the note:\n%s", txt)
	}
	js, _ := WriteData(ds, "json", DataOptions{})
	var out struct{ Notes []string }
	if json.Unmarshal(js, &out) != nil || len(out.Notes) != 1 {
		t.Fatalf("JSON export lost the note: %s", js)
	}
}

func TestMeasurementDataKeepsDifferingDistributionUnitsPerRow(t *testing.T) {
	mk := func(id, unit string) model.Measurement {
		return model.Measurement{ID: id, Dist: &model.Distribution{Columns: []model.Column{
			{Key: "diameter", Unit: unit, Values: []float64{1}, Raw: []string{"1"}},
			{Key: "intensity", Values: []float64{2}, Raw: []string{"2"}},
		}}}
	}
	in := graph.Input{Measurements: map[string]model.Measurement{"a": mk("a", "nm"), "b": mk("b", "µm")}}
	main := MeasurementData("t", []string{"a", "b"}, in, nil, tr).Tables[0]
	var units []string
	for i, c := range main.Columns {
		if c.Key == "diameter" && c.Unit != "" {
			t.Fatalf("one unit %q put over values in two units", c.Unit)
		}
		if c.Key == "diameter_unit" {
			for _, r := range main.Rows {
				units = append(units, r[i].Text)
			}
		}
	}
	if strings.Join(units, ",") != "nm,µm" {
		t.Fatalf("per-row units %v", units)
	}
}

func TestStatisticalDataCarriesTheVariableUnit(t *testing.T) {
	a := analysis.StatisticalAnalysis{Snapshot: analysis.Snapshot{Unit: "nm", Definition: analysis.Definition{Variable: model.EffectiveDiameter}}}
	d := StatisticalData(a)
	units := map[string]string{}
	for _, tb := range d.Tables {
		for _, c := range tb.Columns {
			units[tb.ID+"."+c.Key] = c.Unit
		}
	}
	if units["groups.mean"] != "nm" || units["groups.SD"] != "nm" || units["anova.SS"] != "nm²" || units["anova.F"] != "" {
		t.Fatalf("units %v", units)
	}
	a.Snapshot.Unit = ""
	for _, kv := range StatisticalData(a).Meta {
		if kv.Key == "unit" && kv.Value != "not stated in the source files" {
			t.Fatalf("missing unit shown as %q", kv.Value)
		}
	}
}
