package plot

import (
	"bytes"
	"encoding/xml"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/science/graph"
)

func tr(key string, kv ...string) string {
	s := map[string]string{
		"axis.hydrodynamic_diameter":     "Hydrodynamic diameter",
		"axis.intensity":                 "Intensity",
		"axis.time.weeks":                "Time (weeks)",
		"param.effective_diameter":       "Effective Diameter",
		"param.short.effective_diameter": "Eff. diam.",
		"param.short.polydispersity":     "PDI",
		"series.individual":              "Replicates",
		"series.mean_sd":                 "Mean ± SD",
		"point.weeks":                    "Week {n}",
		"plot.more":                      "+{n} more",
		"plot.meta.stats":                "Mean ± sample SD (n−1)",
		"plot.meta.gaps":                 "No data at: {points}",
		"plot.meta.sources":              "{n} measurements",
		"graph.kind.dls_distribution":    "Size distribution",
	}[key]
	if s == "" {
		s = key
	}
	for i := 0; i+1 < len(kv); i += 2 {
		s = strings.ReplaceAll(s, "{"+kv[i]+"}", kv[i+1])
	}
	return s
}

func distribution() graph.Result {
	r := graph.Result{Kind: graph.KindDistribution, Title: "A1 stability, day 0",
		X:          graph.Axis{Label: "axis.hydrodynamic_diameter", Unit: "nm", Scale: "log", Min: 1.2, Max: 8000},
		Y:          graph.Axis{Label: "axis.intensity", Unit: "%", Scale: "linear", Min: 0, Max: 18.4},
		Provenance: graph.Provenance{Engine: graph.EngineVersion, Spec: "ls-spec", Sources: []graph.Source{{FileName: "a.txt", SHA256: "abc"}}},
		Visual:     graph.DefaultVisual()}
	for k := 0; k < 3; k++ {
		s := graph.Series{ID: string(rune('a' + k)), Label: []string{"A1 · R1", "A1 · R2", "B2 with a rather long sample name"}[k], Kind: "line",
			Meta: map[string]string{"effective_diameter": "245.31", "effective_diameter.unit": "nm", "polydispersity": "0.213", "measuredAt": "2026-10-05T14:02:00Z"}}
		for i := 0; i < 70; i++ {
			d := math.Pow(10, 0.08+float64(i)*0.056)
			s.X = append(s.X, d)
			s.Y = append(s.Y, 18*math.Exp(-math.Pow(math.Log10(d)-2.3-0.1*float64(k), 2)/0.04))
		}
		r.Series = append(r.Series, s)
	}
	return r
}

func parameterTime() graph.Result {
	mean, sd := 4.0, 0.3
	r := graph.Result{Kind: graph.KindParameterTime,
		X:    graph.Axis{Label: "axis.time.weeks", Unit: "weeks", Scale: "linear", Min: 0, Max: 4},
		Y:    graph.Axis{Label: "param.effective_diameter", Unit: "nm", Scale: "linear", Min: 230, Max: 262},
		Gaps: []float64{2}, Visual: graph.DefaultVisual()}
	ind := graph.Series{ID: "individual", Label: "series.individual", Kind: "individual"}
	mn := graph.Series{ID: "mean", Label: "series.mean_sd", Kind: "mean"}
	for _, w := range []float64{0, 1, 3, 4} {
		for k := 0; k < 3; k++ {
			ind.X, ind.Y = append(ind.X, w), append(ind.Y, 240+w*mean+float64(k-1)*sd*10)
		}
		mn.X, mn.Y, mn.Err, mn.N = append(mn.X, w), append(mn.Y, 240+w*mean), append(mn.Err, sd*10), append(mn.N, 3)
	}
	r.Series = []graph.Series{ind, mn}
	return r
}

func checkFigure(t *testing.T, f Figure) {
	t.Helper()
	inside := true
	for _, op := range f.Ops {
		for _, v := range append([]float64{op.X, op.Y, op.X2, op.Y2, op.W, op.H, op.R}, op.Pts...) {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("non-finite coordinate in %+v", op)
			}
		}
		if op.K == "clip" {
			inside = false
		}
		if op.K == "unclip" {
			inside = true
		}
		if inside && op.K == "text" && op.Rotate == 0 {
			w := TextWidth(op.Font, op.Text, op.Size)
			x0 := op.X
			switch op.Anchor {
			case "middle":
				x0 -= w / 2
			case "end":
				x0 -= w
			}
			if x0 < -0.5 || x0+w > f.Width+0.5 || op.Y > f.Height+0.5 || op.Y-op.Size < -0.5 {
				t.Fatalf("text outside the figure: %q at %.1f,%.1f (w %.1f) in %.0fx%.0f", op.Text, x0, op.Y, w, f.Width, f.Height)
			}
		}
	}
}

func TestLayoutPresetsAndRenderers(t *testing.T) {
	out := os.Getenv("MNELAB_PLOT_OUT")
	for _, preset := range []string{PresetScreen, PresetPresentation, PresetPrint, PresetPublication} {
		for name, res := range map[string]graph.Result{"dist": distribution(), "param": parameterTime()} {
			spec := Preset(preset)
			spec.Metadata = true
			f := Layout(res, spec, tr)
			checkFigure(t, f)
			if len(f.Hits) == 0 {
				t.Fatal("no hit points")
			}
			svg := SVG(f, spec)
			d := xml.NewDecoder(bytes.NewReader(svg))
			for {
				if _, err := d.Token(); err == io.EOF {
					break
				} else if err != nil {
					t.Fatalf("%s/%s: invalid SVG: %v", preset, name, err)
				}
			}
			im, err := Raster(f, spec)
			if err != nil {
				t.Fatal(err)
			}
			pw, ph := spec.Pixels()
			if im.Bounds().Dx() != pw || im.Bounds().Dy() != ph {
				t.Fatalf("raster size %v, want %dx%d", im.Bounds(), pw, ph)
			}
			pdf, err := PDF(f, spec, time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC))
			if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) {
				t.Fatalf("pdf: %v", err)
			}
			if out != "" {
				base := filepath.Join(out, preset+"-"+name)
				os.WriteFile(base+".svg", svg, 0o644)
				os.WriteFile(base+".pdf", pdf, 0o644)
				var b bytes.Buffer
				png.Encode(&b, im)
				os.WriteFile(base+".png", b.Bytes(), 0o644)
			}
		}
	}
}

func TestDataIsNeverAltered(t *testing.T) {
	res := distribution()
	before := append([]float64(nil), res.Series[0].Y...)
	f := Layout(res, Preset(PresetScreen), tr)
	for i := range before {
		if res.Series[0].Y[i] != before[i] {
			t.Fatal("layout modified data")
		}
	}
	// Each drawn point maps back to its exact value.
	m := f.Map
	for _, h := range f.Hits {
		if h.Series != 0 {
			continue
		}
		v := m.YMin + (m.Plot.Y+m.Plot.H-h.Y)/m.Plot.H*(m.YMax-m.YMin)
		if math.Abs(v-res.Series[0].Y[h.Index]) > 1e-9*math.Max(1, math.Abs(v)) {
			t.Fatalf("point %d maps to %v, want %v", h.Index, v, res.Series[0].Y[h.Index])
		}
	}
}

func TestLogTicksAndHiddenSeries(t *testing.T) {
	res := distribution()
	res.Series[1].Hidden = true
	f := Layout(res, Preset(PresetScreen), tr)
	var ticks []string
	for _, op := range f.Ops {
		if op.Role == "tick" && op.Anchor == "middle" {
			ticks = append(ticks, op.Text)
		}
		if op.Role == "legend" && op.Text == "A1 · R2" {
			t.Fatal("hidden series in legend")
		}
	}
	if strings.Join(ticks, " ") != "1 10 100 1000 10000" {
		t.Fatalf("log ticks: %v", ticks)
	}
	if f.Map.XMin != 1 || f.Map.XMax != 10000 {
		t.Fatalf("log bounds %v %v", f.Map.XMin, f.Map.XMax)
	}
	// Gaps break the mean line and are marked.
	pf := Layout(parameterTime(), Preset(PresetScreen), tr)
	polys, gaps := 0, 0
	for _, op := range pf.Ops {
		if op.K == "poly" {
			polys++
		}
		if op.Role == "gap" {
			gaps++
		}
	}
	if polys != 2 || gaps != 1 {
		t.Fatalf("gap handling: %d lines, %d gap markers", polys, gaps)
	}
}

func TestDecimalComma(t *testing.T) {
	res := parameterTime()
	res.Y.Min, res.Y.Max = 0.1, 0.32
	res.Y.Label = "param.polydispersity"
	res.Y.Unit = ""
	f := Layout(res, Spec{Width: 800, Height: 500, Unit: UnitPx, DPI: 96, Decimal: ",", Grid: true}, tr)
	found := false
	for _, op := range f.Ops {
		if op.Role == "tick" && strings.Contains(op.Text, ",") {
			found = true
		}
		if op.Role == "tick" && strings.Contains(op.Text, ".") {
			t.Fatalf("decimal point with comma locale: %q", op.Text)
		}
	}
	if !found {
		t.Fatal("no decimal comma ticks")
	}
}

func TestSpecLimits(t *testing.T) {
	s := Preset(PresetPublication)
	if w, h := s.Pixels(); w != 3307 || h != 2244 {
		t.Fatalf("publication pixels %dx%d", w, h)
	}
	s.DPI = 1200
	s.Width, s.Height = 1000, 1000
	if err := s.Validate(true); err == nil {
		t.Fatal("oversized raster accepted")
	}
	s = Preset(PresetPrint)
	s.DPI = 10
	if s.Validate(false) != ErrDPI {
		t.Fatal("bad dpi accepted")
	}
}

func TestReplicatesShareColor(t *testing.T) {
	res := distribution()
	res.Series[0].Label = "point:weeks:0:1"
	res.Series[1].Label = "point:weeks:0:2"
	res.Series[2].Label = "point:weeks:1:1"
	f := Layout(res, Preset(PresetScreen), tr)
	var series []Op
	for _, op := range f.Ops {
		if op.K == "poly" && op.Role == "series" {
			series = append(series, op)
		}
	}
	if len(series) != 3 {
		t.Fatalf("%d series lines", len(series))
	}
	if series[0].Stroke != series[1].Stroke || series[0].Stroke == series[2].Stroke {
		t.Fatalf("colors: %s %s %s", series[0].Stroke, series[1].Stroke, series[2].Stroke)
	}
	if len(series[0].Dash) != 0 || len(series[1].Dash) == 0 || len(series[2].Dash) != 0 {
		t.Fatalf("patterns: %v %v %v", series[0].Dash, series[1].Dash, series[2].Dash)
	}
	svg := string(SVG(f, Preset(PresetScreen)))
	if !strings.Contains(svg, "stroke-dasharray") {
		t.Fatal("SVG drops the replicate pattern")
	}
}
