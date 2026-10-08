package export

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/model"
)

func countRateExportInput(t *testing.T) (graph.Input, []string, graph.CycleInput) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-count-rate-types.txt"))
	if err != nil {
		t.Fatal(err)
	}
	r := lightscattering.Parse(b)
	if len(r.Measurements) != 1 {
		t.Fatal("synthetic count rates were not recognized")
	}
	hash := sha256.Sum256(b)
	in := graph.Input{Measurements: map[string]model.Measurement{}, Files: map[string]model.SourceFile{
		"synthetic-source": {ID: "synthetic-source", Name: "synthetic-count-rate-types.txt", SHA256: hex.EncodeToString(hash[:])},
	}}
	ids := []string{"synthetic-first", "synthetic-second"}
	for _, id := range ids {
		m := r.Measurements[0]
		m.ID, m.FileID = id, "synthetic-source"
		m.Params = map[string]model.Quantity{}
		for k, q := range r.Measurements[0].Params {
			m.Params[k] = q
		}
		in.Measurements[id] = m
	}
	stamp := r.Measurements[0].MeasuredAt
	cfg := cycle.Config{Name: "Synthetic count rate export", Start: stamp.Time, StartTZ: stamp.TZKnown, Interval: 1, Unit: cycle.Days, Duration: 1}
	points, err := cfg.Points()
	if err != nil {
		t.Fatal(err)
	}
	return in, ids, graph.CycleInput{Config: cfg, Points: points, Assignments: []cycle.Assignment{
		{MeasurementID: ids[0], Point: 0, Status: "confirmed"}, {MeasurementID: ids[1], Point: 0, Status: "confirmed"},
	}}
}

func columnIndex(t *testing.T, table Table, key string) int {
	t.Helper()
	for i, c := range table.Columns {
		if c.Key == key {
			return i
		}
	}
	t.Fatalf("missing exported column %s", key)
	return -1
}

func TestCountRateExportsKeepIndependentValuesUnitsAndPrecision(t *testing.T) {
	in, ids, cy := countRateExportInput(t)
	ds := MeasurementData("Synthetic count rates", ids, in, nil, tr)
	for _, want := range []struct{ key, unit, raw string }{
		{model.CountRate, "kcps", "73.250"}, {model.AverageCountRate, "Mcps", "0.18450"},
	} {
		index := columnIndex(t, ds.Tables[0], want.key)
		if ds.Tables[0].Columns[index].Unit != want.unit || ds.Tables[0].Rows[0][index].String(".") != want.raw {
			t.Fatal("normalized export conflated count rate values or units")
		}
		pd, err := ParameterData(want.key, want.key, in, cy, tr)
		if err != nil {
			t.Fatal(err)
		}
		index = columnIndex(t, pd.Tables[0], want.key)
		if len(pd.Tables[0].Rows) != 2 || pd.Tables[0].Columns[index].Unit != want.unit || pd.Tables[0].Rows[0][index].String(".") != want.raw {
			t.Fatal("cycle export lost count rate precision or units")
		}
		if len(pd.Provenance) != 2 || pd.Provenance[0].SHA256 != in.Files["synthetic-source"].SHA256 || pd.Provenance[0].Parser != lightscattering.Version {
			t.Fatal("cycle export lost source provenance")
		}
		for _, format := range []string{"csv", "json", "xlsx"} {
			if _, err := WriteData(pd, format, DataOptions{}); err != nil {
				t.Fatal(format, err)
			}
		}
	}
}

func TestCountRateStatisticsRejectOnlyAcceptedMixedUnits(t *testing.T) {
	for _, otherUnit := range []string{"kcps", ""} {
		t.Run("otherUnit="+otherUnit, func(t *testing.T) {
			in, ids, cy := countRateExportInput(t)
			q := in.Measurements[ids[1]].Params[model.AverageCountRate]
			q.Unit = otherUnit
			in.Measurements[ids[1]].Params[model.AverageCountRate] = q
			if ds, err := ParameterData("Synthetic mixed units", model.AverageCountRate, in, cy, tr); err != graph.ErrMixedUnits || len(ds.Tables) != 0 {
				t.Fatalf("mixed units produced statistics: %v", err)
			}
			// Per-measurement exports remain valid with explicit unit columns.
			ds := MeasurementData("Synthetic mixed units", ids, in, nil, tr)
			unit := columnIndex(t, ds.Tables[0], model.AverageCountRate+"_unit")
			value := columnIndex(t, ds.Tables[0], model.AverageCountRate)
			if ds.Tables[0].Columns[value].Unit != "" || ds.Tables[0].Rows[0][unit].Text != "Mcps" || ds.Tables[0].Rows[3][unit].Text != otherUnit {
				t.Fatal("measurement export relabeled mixed-unit values")
			}
			for _, excluded := range []cycle.Assignment{
				{MeasurementID: ids[1], Point: -1, Status: "unassigned"},
				{MeasurementID: ids[1], Point: 0, Status: "needs_confirmation"},
				{MeasurementID: ids[1], Point: len(cy.Points), Status: "confirmed"},
			} {
				cy.Assignments[1] = excluded
				ds, err := ParameterData("Synthetic excluded record", model.AverageCountRate, in, cy, tr)
				if err != nil || len(ds.Tables[0].Rows) != 1 || ds.Tables[0].Columns[columnIndex(t, ds.Tables[0], model.AverageCountRate)].Unit != "Mcps" {
					t.Fatalf("unaccepted record altered cycle export: %v", err)
				}
			}
		})
	}
}
