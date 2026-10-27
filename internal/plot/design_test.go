package plot

// Layout of design graphs. Values are synthetic.

import (
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/graph"
)

func designResult() graph.Result {
	x := []float64{0, 7, 14, 21}
	mean := func(cond string, off float64, y []float64) graph.Series {
		xs := make([]float64, len(x))
		for i := range x {
			xs[i] = x[i] + off
		}
		return graph.Series{ID: "mean:" + cond, Label: "design:mean_sd:" + cond, Kind: "mean", X: xs, Y: y, Err: []float64{2, 2, 3, 3}, N: []int{3, 3, 3, 3}, Meta: map[string]string{"condition": cond}}
	}
	units := func(cond string) graph.Series {
		return graph.Series{ID: "units:" + cond, Label: "design:units:" + cond, Kind: "individual", X: []float64{0, 0.2, 0.4}, Y: []float64{100, 101, 102}, PointLabels: []string{"A", "B", "C"}, Meta: map[string]string{"condition": cond}}
	}
	return graph.Result{Kind: graph.KindDesignTime, ErrorBars: "sd", Visual: graph.DefaultVisual(),
		Design: &graph.DesignRef{ID: "d1", Name: "Synthetic", Revision: 2},
		X:      graph.Axis{Label: "axis.time.days", Unit: "days", Min: -2, Max: 23, Ticks: x},
		Y:      graph.Axis{Label: "param.effective_diameter", Unit: "nm", Min: 95, Max: 140},
		Series: []graph.Series{
			mean("F-SR", -.4, []float64{120, 124, 130, 135}), units("F-SR"),
			mean("C-SR", .4, []float64{100, 103, 108, 112}), units("C-SR"),
			{ID: "trajectory:F-SR:uA", Label: "design:trajectory:F-SR", Kind: "trajectory", X: x, Y: []float64{120, 124, 130, 135}, PointLabels: []string{"A"}, Meta: map[string]string{"condition": "F-SR"}},
		}}
}

func TestDesignTimeLayout(t *testing.T) {
	spec := Preset(PresetScreen)
	spec.Legend, spec.Metadata = true, true
	fig := Layout(designResult(), spec, tr)
	var legend, meta, ticks, labels []string
	for _, op := range fig.Ops {
		switch op.Role {
		case "legend":
			if op.K == "text" {
				legend = append(legend, op.Text)
			}
		case "metadata":
			meta = append(meta, op.Text)
		case "tick":
			ticks = append(ticks, op.Text)
		case "point-label":
			labels = append(labels, op.Text)
		}
	}
	all := strings.Join(legend, "|")
	if !strings.Contains(all, "series.design_mean_sd · F-SR") || !strings.Contains(all, "series.design_mean_sd · C-SR") || strings.Contains(all, "trajectory") {
		t.Fatalf("legend: %v", legend)
	}
	m := strings.Join(meta, "|")
	if !strings.Contains(m, "plot.meta.design_stats_sd") || !strings.Contains(m, "plot.meta.design_n") || !strings.Contains(m, "plot.meta.design_trajectories") {
		t.Fatalf("metadata must name the error bars and n: %v", meta)
	}
	if !strings.Contains(strings.Join(ticks, ","), "0,7,14,21") {
		t.Fatalf("time ticks at the design's time points: %v", ticks)
	}
	if len(labels) != 1 || labels[0] != "A" {
		t.Fatalf("trajectory end label: %v", labels)
	}
	// Conditions differ in marker and line style, never in colour alone.
	looks := assignStyles(designResult(), 1, false)
	if looks[0].marker == looks[2].marker || looks[0].line == looks[2].line || looks[0].color == looks[2].color {
		t.Fatalf("conditions: %+v %+v", looks[0], looks[2])
	}
	// A unit's points take its condition's colour and marker.
	if looks[1].color != looks[0].color || looks[1].marker != looks[0].marker || looks[4].line != LineSolid {
		t.Fatalf("units and trajectories: %+v %+v %+v", looks[0], looks[1], looks[4])
	}
}

func TestPairedConditionLayout(t *testing.T) {
	res := graph.Result{Kind: graph.KindPairedCondition, ErrorBars: "sd", Visual: graph.DefaultVisual(),
		X: graph.Axis{Label: "axis.condition", Min: .5, Max: 2.5, Categories: []string{"F-SR", "C-SR"}},
		Y: graph.Axis{Label: "param.effective_diameter", Unit: "nm", Min: 95, Max: 130},
		Series: []graph.Series{
			{ID: "mean", Label: "design:mean_sd:", Kind: "mean", NoLine: true, X: []float64{1.2, 2.2}, Y: []float64{124, 103}, Err: []float64{2, 2}, N: []int{3, 3}, Meta: map[string]string{"time": "Day 7"}},
			{ID: "pair:uA", Label: "A", Kind: "paired", X: []float64{1, 2}, Y: []float64{124, 103}, PointLabels: []string{"A", "A"}},
			{ID: "pair:uB", Label: "B", Kind: "paired", X: []float64{1, 2}, Y: []float64{121, 101}, PointLabels: []string{"B", "B"}},
		}}
	spec := Preset(PresetScreen)
	spec.Legend, spec.Metadata, spec.Title = true, true, true
	fig := Layout(res, spec, tr)
	var sub string
	polys, labels := 0, 0
	for _, op := range fig.Ops {
		switch {
		case op.Role == "subtitle":
			sub = op.Text
		case op.Role == "series" && op.K == "poly":
			polys++
		case op.Role == "point-label":
			labels++
		}
	}
	if sub != "plot.paired_at" || polys != 2 || labels != 2 {
		t.Fatalf("paired layout: subtitle %q, lines %d, labels %d", sub, polys, labels)
	}
	looks := assignStyles(res, 1, false)
	if looks[1].marker == looks[2].marker || looks[1].color == looks[2].color {
		t.Fatalf("units must differ in marker and colour: %+v %+v", looks[1], looks[2])
	}
}

func TestAnnotationLabelStaysInsidePlot(t *testing.T) {
	if x := labelX(10, 100, 50, 500); x != 100 {
		t.Fatalf("left edge: %v", x)
	}
	if x := labelX(490, 100, 50, 500); x != 450 {
		t.Fatalf("right edge: %v", x)
	}
	if x := labelX(200, 100, 50, 500); x != 200 {
		t.Fatalf("inside: %v", x)
	}
}
