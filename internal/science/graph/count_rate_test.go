package graph

import (
	"testing"

	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/model"
)

func countRateCycle(t *testing.T, in Input, ids []string) CycleInput {
	t.Helper()
	cfg := cycle.Config{Name: "Synthetic count rate cycle", Start: in.Measurements[ids[0]].MeasuredAt.Time, StartTZ: in.Measurements[ids[0]].MeasuredAt.TZKnown, Interval: 1, Unit: cycle.Days, Duration: 1}
	pts, err := cfg.Points()
	if err != nil {
		t.Fatal(err)
	}
	var cands []cycle.Candidate
	for _, id := range ids {
		cands = append(cands, cycle.Candidate{ID: id, MeasuredAt: in.Measurements[id].MeasuredAt})
	}
	return CycleInput{Config: cfg, Points: pts, Assignments: cycle.Associate(cfg, pts, cands)}
}

func TestCountRateParametersUseIndependentCycleValues(t *testing.T) {
	in, ids := input(t, "synthetic-count-rate-types.txt", "synthetic-count-rate-types.txt")
	cy := countRateCycle(t, in, ids)
	for _, want := range []struct {
		key, unit string
		value     float64
	}{
		{model.CountRate, "kcps", 73.250},
		{model.AverageCountRate, "Mcps", 0.18450},
	} {
		r, err := ParameterTime(Definition{Param: want.key}, in, cy)
		if err != nil {
			t.Fatal(err)
		}
		if r.Y.Label != "param."+want.key || r.Y.Unit != want.unit || len(r.Series) != 2 || len(r.Series[0].Y) != 2 || r.Series[0].Y[0] != want.value || r.Series[0].Y[1] != want.value || r.Series[1].Y[0] != want.value || r.Series[1].N[0] != 2 || r.Series[1].Err[0] != 0 {
			t.Fatalf("%s: incorrect independent values or statistics: %+v", want.key, r)
		}
		if len(r.Provenance.Sources) != 2 || len(r.Gaps) != 1 {
			t.Fatalf("%s: provenance or missing point lost", want.key)
		}
	}
	r, err := Distribution(Definition{Measurements: ids[:1]}, in)
	if err != nil {
		t.Fatal(err)
	}
	meta := r.Series[0].Meta
	if meta[model.CountRate] != "73.250" || meta[model.CountRate+".unit"] != "kcps" || meta[model.AverageCountRate] != "0.18450" || meta[model.AverageCountRate+".unit"] != "Mcps" {
		t.Fatalf("independent graph metadata lost: %v", meta)
	}
}

func TestAverageCountRateGraphRejectsMixedAcceptedUnits(t *testing.T) {
	for _, unit := range []string{"kcps", ""} {
		t.Run("unit="+unit, func(t *testing.T) {
			in, ids := input(t, "synthetic-count-rate-types.txt", "synthetic-count-rate-types.txt")
			m := in.Measurements[ids[1]]
			q := m.Params[model.AverageCountRate]
			q.Unit = unit
			m.Params[model.AverageCountRate] = q
			cy := countRateCycle(t, in, ids)
			if _, err := ParameterTime(Definition{Param: model.AverageCountRate}, in, cy); err != ErrMixedUnits {
				t.Fatalf("mixed accepted units: %v", err)
			}
			for _, excluded := range []cycle.Assignment{
				{MeasurementID: ids[1], Point: -1, Status: "unassigned"},
				{MeasurementID: ids[1], Point: 0, Status: "needs_confirmation"},
				{MeasurementID: ids[1], Point: len(cy.Points), Status: "confirmed"},
			} {
				cy.Assignments[1] = excluded
				r, err := ParameterTime(Definition{Param: model.AverageCountRate}, in, cy)
				if err != nil || r.Y.Unit != "Mcps" || len(r.Series[0].Y) != 1 || r.Series[0].Y[0] != 0.18450 {
					t.Fatalf("excluded mixed measurement affected graph: %+v, %v", r, err)
				}
			}
		})
	}
}
