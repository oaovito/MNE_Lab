package export

// Golden data of a design graph export. Every value is synthetic.

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/design"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
)

var updateGolden = flag.Bool("update-design-golden", false, "rewrite testdata/design-export")

func designFixture() (graph.Input, design.Design) {
	in := graph.Input{Measurements: map[string]model.Measurement{}, Files: map[string]model.SourceFile{"f1": {ID: "f1", Name: "synthetic-paired.txt", SHA256: "sum"}}}
	d := design.Design{ID: "rev1", Root: "rev1", Revision: 1, Name: "Synthetic paired",
		Units:      []design.Unit{{ID: "unit-a", Label: "A"}, {ID: "unit-b", Label: "B"}, {ID: "unit-c", Label: "C"}},
		Conditions: []design.Condition{{Label: "F-SR"}, {Label: "C-SR", DerivedFrom: "F-SR"}},
		Times:      []design.TimeLevel{{Label: "Day 0"}, {Label: "Day 7"}}}
	values := map[string][]string{
		"A": {"120.1", "124.3", "100.2", "103.4"},
		"B": {"118.7", "121.0", "98.6", "101.9"},
		"C": {"122.4", "126.8", "103.1", "105.5"},
	}
	n := 0
	add := func(unit, cond, tm, raw, kind string) {
		n++
		id := "m" + strconv.Itoa(n)
		v, _ := strconv.ParseFloat(raw, 64)
		in.Measurements[id] = model.Measurement{ID: id, FileID: "f1", SampleID: cond + " " + unit, Params: map[string]model.Quantity{model.EffectiveDiameter: {Value: v, Raw: raw, Unit: "nm"}}}
		d.Assignments = append(d.Assignments, design.Assignment{MeasurementID: id, UnitID: "unit-" + string(rune('a'+unit[0]-'A')), Condition: cond, Time: tm, ReplicateKind: kind})
	}
	for _, u := range []string{"A", "B", "C"} {
		add(u, "F-SR", "Day 0", values[u][0], design.ReplicateExperimental)
		add(u, "F-SR", "Day 7", values[u][1], design.ReplicateExperimental)
		add(u, "C-SR", "Day 0", values[u][2], design.ReplicateExperimental)
		add(u, "C-SR", "Day 7", values[u][3], design.ReplicateExperimental)
	}
	// A second technical reading of unit A: averaged, never counted.
	add("A", "F-SR", "Day 0", "121.9", design.ReplicateTechnical)
	return in, d
}

func TestDesignExportGolden(t *testing.T) {
	in, d := designFixture()
	def := graph.Definition{Kind: graph.KindDesignTime, DesignID: d.ID, Param: model.EffectiveDiameter}
	tab, err := graph.DesignTables(def, in, graph.DesignInput{Design: d})
	if err != nil {
		t.Fatal(err)
	}
	T := func(key string, kv ...string) string { return key }
	ds := DesignData("Graph 3", tab, in, Translator(T))
	if len(ds.Tables) != 3 || len(ds.Provenance) != 13 {
		t.Fatalf("tables %d, sources %d", len(ds.Tables), len(ds.Provenance))
	}
	for _, tb := range ds.Tables {
		one := ds
		one.Tables = []Table{tb}
		b, err := WriteData(one, "csv", DataOptions{})
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join("..", "..", "testdata", "design-export", "graph3-"+tb.ID+".csv")
		if *updateGolden {
			if err := os.WriteFile(path, b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != string(want) {
			t.Fatalf("%s differs from the golden file:\n%s", tb.ID, b)
		}
	}
	// The unit value of A at F-SR, Day 0 is the mean of its two readings
	// and n stays 3 experimental units.
	if tab.Units[0].Readings != 2 || tab.Units[0].Value != (120.1+121.9)/2 || tab.Summary[0].N != 3 {
		t.Fatalf("units: %+v summary: %+v", tab.Units[0], tab.Summary[0])
	}
}
