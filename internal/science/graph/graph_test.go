package graph

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/model"
)

func input(t *testing.T, files ...string) (Input, []string) {
	in := Input{Measurements: map[string]model.Measurement{}, Files: map[string]model.SourceFile{}}
	var ids []string
	for fi, name := range files {
		b, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "lightscattering", name))
		if err != nil {
			t.Fatal(err)
		}
		r := lightscattering.Parse(b)
		fid := "f" + string(rune('a'+fi))
		in.Files[fid] = model.SourceFile{ID: fid, Name: name, SHA256: "sum-" + name}
		for i, m := range r.Measurements {
			m.ID = fid + string(rune('0'+i))
			m.FileID = fid
			in.Measurements[m.ID] = m
			ids = append(ids, m.ID)
		}
	}
	return in, ids
}

func TestSingleAndMultiFileDistribution(t *testing.T) {
	in, ids := input(t, "synthetic-tab.txt", "synthetic-two-runs.txt")
	one, err := Distribution(Definition{Measurements: ids[:1]}, in)
	if err != nil {
		t.Fatal(err)
	}
	if one.X.Scale != "log" || one.X.Unit != "nm" || one.Y.Unit != "%" || one.Y.Label != "axis.intensity" || len(one.Series) != 1 {
		t.Fatalf("single: %+v", one)
	}
	if len(one.Weightings) != 3 {
		t.Fatalf("weightings: %v", one.Weightings)
	}
	multi, err := Distribution(Definition{Measurements: ids}, in)
	if err != nil || len(multi.Series) != 3 || len(multi.Provenance.Sources) != 3 {
		t.Fatalf("multi: %v %d", err, len(multi.Series))
	}
	// Values are plotted exactly as read.
	m := in.Measurements[ids[0]]
	for i, v := range multi.Series[0].Y {
		if v != m.Dist.Column("intensity").Values[i] {
			t.Fatal("plotted values differ from the file")
		}
	}
	if multi.Series[0].Meta["effective_diameter"] != "245.31" {
		t.Fatalf("meta: %v", multi.Series[0].Meta)
	}
	if _, err := Distribution(Definition{Measurements: ids[1:2], Weighting: "volume"}, in); err != ErrNoWeighting {
		t.Fatalf("absent weighting: %v", err)
	}
	if r, _ := Distribution(Definition{Measurements: ids[:1], XScale: "linear"}, in); r.X.Scale != "linear" {
		t.Fatal("explicit linear scale ignored")
	}
}

func TestMeasurementWithoutDistributionIsLeftOut(t *testing.T) {
	in, ids := input(t, "synthetic-tab.txt", "synthetic-missing-fields.txt")
	r, err := Distribution(Definition{Measurements: ids}, in)
	if err != nil || len(r.Series) != 1 {
		t.Fatalf("expected one series and a note, got %v %d", err, len(r.Series))
	}
	if len(r.Warnings) == 0 || r.Warnings[len(r.Warnings)-1] != "graph.skipped_no_distribution:"+seriesLabel(Definition{}, in.Measurements[ids[1]], 1) {
		t.Fatalf("warnings: %v", r.Warnings)
	}
	if _, err := Distribution(Definition{Measurements: ids[1:]}, in); err != ErrNoDistribution {
		t.Fatalf("nothing to draw must still fail: %v", err)
	}
}

func TestMixedUnitsRefused(t *testing.T) {
	in, ids := input(t, "synthetic-tab.txt", "synthetic-semicolon-comma.txt")
	m := in.Measurements[ids[1]]
	m.Dist.Columns[0].Unit = "µm"
	in.Measurements[ids[1]] = m
	if _, err := Distribution(Definition{Measurements: ids}, in); err != ErrMixedUnits {
		t.Fatalf("mixed units must be refused, got %v", err)
	}
}

func TestParameterTime(t *testing.T) {
	in, ids := input(t, "synthetic-two-runs.txt", "synthetic-tab.txt")
	start := in.Measurements[ids[0]].MeasuredAt.Time
	cfg := cycle.Config{Name: "c", Start: start, Interval: 1, Unit: cycle.Days, Duration: 2}
	pts, _ := cfg.Points()
	as := cycle.Associate(cfg, pts, []cycle.Candidate{
		{ID: ids[0], MeasuredAt: in.Measurements[ids[0]].MeasuredAt},
		{ID: ids[1], MeasuredAt: in.Measurements[ids[1]].MeasuredAt},
	})
	r, err := ParameterTime(Definition{Param: model.EffectiveDiameter}, in, CycleInput{cfg, pts, as})
	if err != nil {
		t.Fatal(err)
	}
	if r.Y.Unit != "nm" || len(r.Series) != 2 || r.Series[1].Kind != "mean" {
		t.Fatalf("%+v", r)
	}
	mean := r.Series[1]
	if mean.N[0] != 2 || mean.Y[0] != (239.8+243.1)/2 {
		t.Fatalf("mean %+v", mean)
	}
	if len(r.Gaps) != 2 {
		t.Fatalf("days 1 and 2 have no measurement: gaps %v", r.Gaps)
	}
	_ = time.Now
}

func TestEqualLabelsAreTold(t *testing.T) {
	mk := func(id string, day int) model.Measurement {
		at := &model.Timestamp{Time: time.Date(2026, 9, day, 10, 0, 0, 0, time.UTC)}
		return model.Measurement{ID: id, SampleID: "F-A1", MeasuredAt: at}
	}
	ms := []model.Measurement{mk("a", 1), mk("b", 8), mk("c", 8), mk("d", 15)}
	series := []Series{{Label: "F-A1"}, {Label: "F-A1"}, {Label: "F-A1"}, {Label: "Mine"}}
	def := Definition{Series: map[string]SeriesStyle{"d": {Label: "Mine"}}}
	distinctLabels(def, series, ms)
	want := []string{"F-A1 · 2026-09-01", "F-A1 · 2026-09-08 (1)", "F-A1 · 2026-09-08 (2)", "Mine"}
	for i, s := range series {
		if s.Label != want[i] {
			t.Fatalf("label %d = %q, want %q", i, s.Label, want[i])
		}
	}
}
