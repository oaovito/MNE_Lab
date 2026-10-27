package plot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/model"
)

// Definitions saved before selection by SeriesID (schema 1) must keep
// drawing exactly what they drew. The golden file was written by the
// engine as it was before the v2 fields existed (MNELAB_UPDATE_GOLDEN=1
// rewrites it; never do so to make a difference go away). Only the engine
// version and the computation time are left out of the comparison.

func compatInput(t *testing.T) (graph.Input, []string) {
	in := graph.Input{Measurements: map[string]model.Measurement{}, Files: map[string]model.SourceFile{}}
	var ids []string
	for fi, name := range []string{"synthetic-tab.txt", "synthetic-two-runs.txt", "synthetic-nanobrook-methods.txt"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", name))
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

type compatCase struct {
	Name   string          `json:"name"`
	Result json.RawMessage `json:"result"`
	Figure json.RawMessage `json:"figure"`
}

func compatResults(t *testing.T) []compatCase {
	in, ids := compatInput(t)
	var out []compatCase
	add := func(name string, res graph.Result, err error) {
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		res.Provenance.Engine = ""
		res.Provenance.ComputedAt = time.Time{}
		spec := Preset(PresetScreen)
		spec.Width, spec.Height, spec.DPI = 900, 560, 72
		spec.Legend, spec.Metadata, spec.Grid = res.Visual.Legend, res.Visual.Metadata, res.Visual.Grid
		fig := Layout(res, spec, tr)
		fig.Desc = ""
		rb, _ := json.Marshal(res)
		fb, _ := json.Marshal(fig)
		out = append(out, compatCase{name, rb, fb})
	}
	v := graph.DefaultVisual()
	r, err := graph.Distribution(graph.Definition{Schema: 1, Kind: graph.KindDistribution, Title: "v1 overlay", Measurements: ids, Visual: v}, in)
	add("distribution", r, err)
	styled := graph.Definition{Schema: 1, Kind: graph.KindDistribution, Measurements: ids[:3], Weighting: "volume", XScale: "linear", Visual: v,
		Series: map[string]graph.SeriesStyle{ids[0]: {Label: "Mine", Color: "#143A52"}, ids[1]: {Hidden: true}, ids[2]: {Marker: "square", LineStyle: "dashed"}}}
	styled.Visual.Points = true
	styled.Visual.LegendMode = "all"
	r, err = graph.Distribution(styled, in)
	add("distribution-styled", r, err)

	var dated []string
	for _, id := range ids {
		if in.Measurements[id].MeasuredAt != nil {
			dated = append(dated, id)
		}
	}
	start := in.Measurements[dated[0]].MeasuredAt.Time
	for _, id := range dated {
		if at := in.Measurements[id].MeasuredAt.Time; at.Before(start) {
			start = at
		}
	}
	cfg := cycle.Config{Name: "c", Start: start.Truncate(24 * time.Hour).Add(10 * time.Hour), Interval: 7, Unit: cycle.Days, Duration: 21}
	pts, _ := cfg.Points()
	var cands []cycle.Candidate
	for _, id := range dated {
		cands = append(cands, cycle.Candidate{ID: id, MeasuredAt: in.Measurements[id].MeasuredAt})
	}
	cy := graph.CycleInput{Config: cfg, Points: pts, Assignments: cycle.Associate(cfg, pts, cands)}
	r, err = graph.ParameterTime(graph.Definition{Schema: 1, Kind: graph.KindParameterTime, Param: model.EffectiveDiameter, Visual: v}, in, cy)
	add("parameter-time", r, err)
	r, err = graph.DistributionAtPoints(graph.Definition{Schema: 1, Kind: graph.KindDistributionAt, Visual: v}, in, cy)
	add("distribution-at-points", r, err)
	return out
}

func TestV1DefinitionsRenderIdentically(t *testing.T) {
	got := compatResults(t)
	file := filepath.Join("testdata", "v1-graphs.golden.json")
	if os.Getenv("MNELAB_UPDATE_GOLDEN") == "1" {
		b, _ := json.MarshalIndent(got, "", " ")
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(file, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var want []compatCase
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) != len(got) {
		t.Fatalf("%d cases, want %d", len(got), len(want))
	}
	for i := range want {
		if want[i].Name != got[i].Name {
			t.Fatalf("case %d is %s, want %s", i, got[i].Name, want[i].Name)
		}
		for _, part := range []struct {
			name      string
			want, got json.RawMessage
		}{{"result", want[i].Result, got[i].Result}, {"figure", want[i].Figure, got[i].Figure}} {
			var a, b any
			json.Unmarshal(part.want, &a)
			json.Unmarshal(part.got, &b)
			wa, _ := json.Marshal(a)
			gb, _ := json.Marshal(b)
			if string(wa) != string(gb) {
				t.Errorf("%s: the %s of a v1 definition changed", want[i].Name, part.name)
			}
		}
	}
}
