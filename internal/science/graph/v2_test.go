package graph

import (
	"math"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/model"
)

func pairCycle(t *testing.T) (Input, CycleInput) {
	in, ids := input(t, "synthetic-two-runs.txt", "synthetic-tab.txt")
	start := in.Measurements[ids[0]].MeasuredAt.Time
	cfg := cycle.Config{Name: "c", Start: start, Interval: 1, Unit: cycle.Days, Duration: 2}
	pts, _ := cfg.Points()
	as := cycle.Associate(cfg, pts, []cycle.Candidate{{ID: ids[0], MeasuredAt: in.Measurements[ids[0]].MeasuredAt}, {ID: ids[1], MeasuredAt: in.Measurements[ids[1]].MeasuredAt}})
	return in, CycleInput{cfg, pts, as}
}

func TestParameterTimeErrorBarsAreNamed(t *testing.T) {
	in, cy := pairCycle(t)
	sd, err := ParameterTime(Definition{Param: model.EffectiveDiameter}, in, cy)
	if err != nil {
		t.Fatal(err)
	}
	sem, err := ParameterTime(Definition{Param: model.EffectiveDiameter, ErrorBars: "sem"}, in, cy)
	if err != nil {
		t.Fatal(err)
	}
	if sem.ErrorBars != "sem" || sem.Series[1].Label != "series.mean_sem" || math.Abs(sem.Series[1].Err[0]-sd.Series[1].Err[0]/math.Sqrt(2)) > 1e-12 {
		t.Fatalf("%+v", sem.Series[1])
	}
	ci, err := ParameterTime(Definition{Param: model.EffectiveDiameter, ErrorBars: "ci"}, in, cy)
	if err != nil || ci.ErrorBars != "sd" || ci.Series[1].Err[0] != sd.Series[1].Err[0] || !strings.Contains(strings.Join(ci.Warnings, " "), "graph.ci_needs_saved_analysis") {
		t.Fatalf("a CI is never computed here: %v %+v %v", err, ci.Series[1], ci.Warnings)
	}
	no := false
	def := Definition{Param: model.EffectiveDiameter, ShowIndividual: &no, ConnectMeans: &no, FocusSeries: []string{"mean"}}
	Finish(def, &sd)
	if !sd.Series[0].Hidden || !sd.Series[1].NoLine || !sd.Series[0].Muted || sd.Series[1].Muted {
		t.Fatalf("%+v", sd.Series)
	}
	if len(sd.Series[0].Y) == 0 {
		t.Fatal("hiding individual values must not drop them")
	}
}
