package plot

import (
	"bytes"
	"compress/zlib"
	"encoding/xml"
	"fmt"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/science/graph"
)

// shapesFigure has one large marker of every shape, each in its own colour.
func shapesFigure() (Figure, Spec, []string) {
	spec := Spec{Width: 600, Height: 120, Unit: UnitPx, DPI: 72, Background: "solid", BackColor: "#FFFFFF"}
	f := Figure{Width: 600, Height: 120, Title: "shapes", Ops: []Op{{K: "rect", W: 600, H: 120, Fill: "#FFFFFF", Role: "background"}}}
	colors := []string{"#2C7FB8", "#883717", "#549D94", "#143A52", "#C89F5A", "#61666D"}
	for i, shape := range Markers {
		f.Ops = append(f.Ops, markerOp(shape, 50+float64(i)*100, 60, 22, colors[i], 0, "", 3, "point"))
	}
	return f, spec, colors
}

func TestEveryRendererDrawsEveryShape(t *testing.T) {
	f, spec, colors := shapesFigure()

	// SVG: a circle, four polygons and the two strokes of the cross.
	svg := string(SVG(f, spec))
	if err := xmlValid(svg); err != nil {
		t.Fatalf("invalid SVG: %v", err)
	}
	if n := strings.Count(svg, "<circle"); n != 1 {
		t.Fatalf("SVG circles: %d", n)
	}
	if n := strings.Count(svg, "<polygon"); n != 4 {
		t.Fatalf("SVG polygons: %d", n)
	}
	if !regexp.MustCompile(`<path d="M[^"]+M[^"]+" fill="none" stroke="#61666D"`).MatchString(svg) {
		t.Fatal("SVG cross missing")
	}
	for _, c := range colors {
		if !strings.Contains(svg, c) {
			t.Fatalf("SVG lacks colour %s", c)
		}
	}

	// Raster: every marker's centre has its own colour, and the corners of
	// the square (outside the circle's radius) are painted while the
	// circle's are not.
	im, err := Raster(f, spec)
	if err != nil {
		t.Fatal(err)
	}
	near := func(got color.Color, hex string) bool {
		r, g, b, _ := got.RGBA()
		w := ParseColor(hex)
		return math.Abs(float64(r>>8)-float64(w.R)) < 24 && math.Abs(float64(g>>8)-float64(w.G)) < 24 && math.Abs(float64(b>>8)-float64(w.B)) < 24
	}
	for i, c := range colors {
		x := 50 + i*100
		if !near(im.At(x, 60), c) {
			t.Fatalf("raster %s: centre is %v, want %s", Markers[i], im.At(x, 60), c)
		}
	}
	if !near(im.At(50+100+17, 60+17), colors[1]) || near(im.At(50+17, 60+17), colors[0]) {
		t.Fatal("raster square and circle are not told apart")
	}
	// Triangle points up, the inverted triangle down.
	if !near(im.At(270, 72), colors[2]) || near(im.At(270, 40), colors[2]) {
		t.Fatal("raster triangle shape")
	}
	if !near(im.At(470, 48), colors[4]) || near(im.At(470, 80), colors[4]) {
		t.Fatal("raster inverted triangle shape")
	}
	// The cross is two strokes: painted on its diagonal, empty on its axis.
	if !near(im.At(550+12, 60+12), colors[5]) || near(im.At(550+15, 60), colors[5]) {
		t.Fatal("raster cross shape")
	}

	// PDF: one circle (Bézier curves), four filled polygons and a cross of
	// two stroked lines.
	pdf, err := PDF(f, spec, time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	content := pdfContent(t, pdf)
	if n := strings.Count(content, " c\nf\n"); n != 1 {
		t.Fatalf("PDF circles: %d", n)
	}
	if n := strings.Count(content, " l \nf\n"); n != 4 {
		t.Fatalf("PDF filled polygons: %d", n)
	}
	if n := strings.Count(content, " l S\n"); n != 2 {
		t.Fatalf("PDF cross strokes: %d", n)
	}
	for _, c := range colors[:5] {
		p := ParseColor(c)
		if !strings.Contains(content, fmt.Sprintf("%.3f %.3f %.3f rg", float64(p.R)/255, float64(p.G)/255, float64(p.B)/255)) {
			t.Fatalf("PDF lacks fill %s", c)
		}
	}
}

// pdfContent returns the decompressed content streams of a PDF.
func pdfContent(t *testing.T, pdf []byte) string {
	t.Helper()
	var out strings.Builder
	re := regexp.MustCompile(`(?s)/Filter /FlateDecode[^>]*>>\s*stream\r?\n(.*?)endstream`)
	for _, m := range re.FindAllSubmatch(pdf, -1) {
		r, err := zlib.NewReader(bytes.NewReader(m[1]))
		if err != nil {
			continue
		}
		b, _ := io.ReadAll(r)
		if bytes.Contains(b, []byte(" re")) || bytes.Contains(b, []byte(" m ")) {
			out.Write(b)
		}
	}
	return out.String()
}

func xmlValid(s string) error {
	d := xml.NewDecoder(strings.NewReader(s))
	for {
		if _, err := d.Token(); err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
	}
}

// curves returns a distribution result with n curves.
func curves(n int, label func(int) string) graph.Result {
	r := distribution()
	base := r.Series[0]
	r.Series = nil
	for k := 0; k < n; k++ {
		s := base
		s.ID, s.Label = fmt.Sprintf("m%02d", k), label(k)
		s.Y = make([]float64, len(base.X))
		for i, d := range base.X {
			s.Y[i] = 18 * math.Exp(-math.Pow(math.Log10(d)-2.0-0.02*float64(k), 2)/0.04)
		}
		r.Series = append(r.Series, s)
	}
	return r
}

type drawn struct {
	stroke string
	dash   string
	shape  string
}

// drawnSeries reads, for each visible series, its line colour and pattern
// and the shape of the markers drawn on it, from the display list.
func drawnSeries(f Figure) []drawn {
	var out []drawn
	for _, op := range f.Ops {
		switch {
		case op.K == "poly" && op.Role == "series":
			out = append(out, drawn{stroke: op.Stroke, dash: fmt.Sprint(op.Dash)})
		case op.K == "marker" && (op.Role == "marker" || op.Role == "point") && len(out) > 0:
			last := &out[len(out)-1]
			if last.stroke == op.Fill || last.stroke == op.Stroke {
				last.shape = op.Shape
			}
		}
	}
	return out
}

func TestSeriesNeverDifferOnlyInColour(t *testing.T) {
	for _, n := range []int{2, 3, 6, 7, 12, 20, 36} {
		res := curves(n, func(k int) string { return fmt.Sprintf("S%d", k) })
		f := Layout(res, Preset(PresetScreen), tr)
		d := drawnSeries(f)
		if len(d) != n {
			t.Fatalf("%d curves: %d drawn", n, len(d))
		}
		seen := map[string]int{}
		for i, s := range d {
			key := s.dash + "|" + s.shape
			if j, ok := seen[key]; ok {
				t.Fatalf("%d curves: curves %d and %d differ only in colour (%s)", n, j, i, key)
			}
			seen[key] = i
		}
		// The first six differ in line pattern alone, without markers.
		if n <= len(LineStyles) {
			for _, s := range d {
				if s.shape != "" {
					t.Fatalf("%d curves: markers added although the line patterns differ", n)
				}
			}
		}
		// The legend shows the same pattern and marker as the curve.
		var legend []drawn
		for _, op := range f.Ops {
			if op.Role == "legend" && op.K == "line" {
				legend = append(legend, drawn{dash: fmt.Sprint(op.Dash)})
			}
			if op.Role == "legend" && op.K == "marker" {
				legend[len(legend)-1].shape = op.Shape
			}
		}
		if len(legend) == 0 {
			t.Fatalf("%d curves: no legend", n)
		}
		for i, l := range legend {
			if l.shape != d[i].shape || len(l.dash) == 2 != (len(d[i].dash) == 2) {
				t.Fatalf("%d curves: legend entry %d is %+v, curve is %+v", n, i, l, d[i])
			}
		}
	}

	// Means of a temporal graph with several series: markers and patterns.
	res := parameterTime()
	mean := res.Series[1]
	m2 := mean
	m2.ID, m2.Label = "mean2", "Condition B"
	m2.Y = append([]float64(nil), mean.Y...)
	for i := range m2.Y {
		m2.Y[i] += 5
	}
	res.Series = append(res.Series, m2)
	f := Layout(res, Preset(PresetScreen), tr)
	shapes := map[string]bool{}
	for _, op := range f.Ops {
		if op.K == "marker" && op.Role == "point" && op.Opacity == 0 {
			shapes[op.Shape] = true
		}
	}
	if len(shapes) != 2 {
		t.Fatalf("two mean series share a marker: %v", shapes)
	}
	if res.Series[1].LineStyle == res.Series[2].LineStyle {
		t.Fatal("two mean series share a line style")
	}

	// Saved choices win over the defaults.
	res = curves(2, func(k int) string { return fmt.Sprintf("S%d", k) })
	res.Series[0].Marker, res.Series[0].LineStyle = MarkerDiamond, LineDotted
	res.Series[1].LineStyle = LineDotted
	f = Layout(res, Preset(PresetScreen), tr)
	d := drawnSeries(f)
	if d[0].shape != MarkerDiamond || d[0].dash != fmt.Sprint(dashFor(LineDotted, Preset(PresetScreen).LineWidth)) || d[1].shape == "" || d[1].shape == MarkerDiamond {
		t.Fatalf("saved styles: %+v", d)
	}
	// Unknown names fall back to the defaults.
	res = curves(2, func(k int) string { return fmt.Sprintf("S%d", k) })
	res.Series[0].Marker, res.Series[0].LineStyle = "star", "wavy"
	if d := drawnSeries(Layout(res, Preset(PresetScreen), tr)); d[0].dash != "[]" || d[0].shape != "" {
		t.Fatalf("unknown style names: %+v", d)
	}
}

func TestPaletteGrayscaleAndColourVision(t *testing.T) {
	L := make([]float64, len(Palette))
	for i, c := range Palette {
		L[i] = Lightness(c)
		if c != strings.ToUpper(c) || len(c) != 7 {
			t.Fatalf("palette colour %q", c)
		}
		// Visible lines on white paper.
		if contrast := 1.05 / (Luminance(c) + 0.05); contrast < 2.2 {
			t.Fatalf("%s: contrast %.2f on white", c, contrast)
		}
	}
	if Palette[0] != "#2C7FB8" || Palette[3] != "#143A52" {
		t.Fatal("the owner's colours lead the palette")
	}
	// Grayscale: the first six differ by at least 7 L*, neighbours by at
	// least 10, and no two of the ten by less than 2.5.
	for i := range L {
		for j := i + 1; j < len(L); j++ {
			d := math.Abs(L[i] - L[j])
			switch {
			case i < 6 && j < 6 && d < 7:
				t.Fatalf("%s and %s: %.1f L* apart in grayscale", Palette[i], Palette[j], d)
			case j == i+1 && d < 10:
				t.Fatalf("neighbours %s and %s: %.1f L* apart", Palette[i], Palette[j], d)
			case d < 2.5:
				t.Fatalf("%s and %s: %.1f L* apart", Palette[i], Palette[j], d)
			}
		}
	}
	// Colour-vision deficiency: simulated protanopia, deuteranopia and
	// tritanopia (Machado et al. 2009, full severity) keep every pair of the
	// ten apart by at least 8 ΔE (CIE76).
	sims := map[string][3][3]float64{
		"none": {{1, 0, 0}, {0, 1, 0}, {0, 0, 1}},
		"prot": {{0.152286, 1.052583, -0.204868}, {0.114503, 0.786281, 0.099216}, {-0.003882, -0.048116, 1.051998}},
		"deut": {{0.367322, 0.860646, -0.227968}, {0.280085, 0.672501, 0.047413}, {-0.011820, 0.042940, 0.968881}},
		"trit": {{1.255528, -0.076749, -0.178779}, {-0.078411, 0.930809, 0.147602}, {0.004733, 0.691367, 0.303900}},
	}
	for name, m := range sims {
		for i := range Palette {
			for j := i + 1; j < len(Palette); j++ {
				if d := deltaE(simLab(Palette[i], m), simLab(Palette[j], m)); d < 8 {
					t.Fatalf("%s: %s and %s only %.1f ΔE apart", name, Palette[i], Palette[j], d)
				}
			}
		}
	}
	// Generated colours past the palette: distinct, restrained (no
	// saturated or fluorescent greens) and visible on white.
	seen := map[string]bool{}
	for n := 0; n < 60; n++ {
		c := SeriesColor(n)
		if seen[c] {
			t.Fatalf("colour %d repeats %s", n, c)
		}
		seen[c] = true
		if contrast := 1.05 / (Luminance(c) + 0.05); contrast < 1.9 {
			t.Fatalf("generated %s: contrast %.2f", c, contrast)
		}
		p := ParseColor(c)
		if int(p.G) > int(p.R)+40 && int(p.G) > int(p.B)+20 {
			t.Fatalf("generated %s is green", c)
		}
	}
}

func simLab(hex string, m [3][3]float64) [3]float64 {
	c := ParseColor(hex)
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	in := [3]float64{lin(c.R), lin(c.G), lin(c.B)}
	var rgb [3]float64
	for i := range rgb {
		rgb[i] = math.Max(0, math.Min(1, m[i][0]*in[0]+m[i][1]*in[1]+m[i][2]*in[2]))
	}
	X := 0.4124*rgb[0] + 0.3576*rgb[1] + 0.1805*rgb[2]
	Y := 0.2126*rgb[0] + 0.7152*rgb[1] + 0.0722*rgb[2]
	Z := 0.0193*rgb[0] + 0.1192*rgb[1] + 0.9505*rgb[2]
	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return 7.787*t + 16.0/116
	}
	fx, fy, fz := f(X/0.95047), f(Y), f(Z/1.08883)
	return [3]float64{116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)}
}

func deltaE(a, b [3]float64) float64 {
	return math.Sqrt((a[0]-b[0])*(a[0]-b[0]) + (a[1]-b[1])*(a[1]-b[1]) + (a[2]-b[2])*(a[2]-b[2]))
}

func TestLegendStaysBoundedWithFortySeries(t *testing.T) {
	out := os.Getenv("MNELAB_PLOT_OUT")
	labels := map[string]func(int) string{
		"short": func(k int) string { return fmt.Sprintf("S%02d", k) },
		"long": func(k int) string {
			return fmt.Sprintf("F-SR unit A day %d, replicate reading %d of the series", k*7, k)
		},
	}
	for name, label := range labels {
		for _, preset := range []string{PresetScreen, PresetPresentation, PresetPrint, PresetPublication} {
			for _, mode := range []string{"", LegendVisible, LegendAll, LegendSelected} {
				res := curves(40, label)
				res.Visual.LegendMode = mode
				res.Series[3].Hidden = true
				if mode == LegendSelected {
					res.Visual.LegendSeries = []string{"m00", "m05", "m03"}
				}
				spec := Preset(preset)
				f := Layout(res, spec, tr)
				checkFigure(t, f)
				where := fmt.Sprintf("%s/%s/%q", name, preset, mode)
				p := f.Map.Plot
				if p.W < f.Width*0.6 || p.H < f.Height*0.45 {
					t.Fatalf("%s: plot area %.0f×%.0f of %.0f×%.0f", where, p.W, p.H, f.Width, f.Height)
				}
				listed, more := 0, ""
				for _, op := range f.Ops {
					if op.Role != "legend" {
						continue
					}
					if op.X < -0.5 || op.Y < -0.5 || op.X > f.Width+0.5 || op.Y > f.Height+0.5 {
						t.Fatalf("%s: legend outside the figure: %+v", where, op)
					}
					inPlot := op.X > p.X+1 && op.X < p.X+p.W-1 && op.Y > p.Y+1 && op.Y < p.Y+p.H-1
					if inPlot {
						t.Fatalf("%s: legend over the plot: %+v", where, op)
					}
					if op.K == "text" {
						if strings.HasPrefix(op.Text, "+") {
							more = op.Text
						} else {
							listed++
						}
					}
				}
				total := 39
				if mode == LegendAll {
					total = 40
				}
				switch mode {
				case LegendSelected:
					// m03 is hidden: only m00 and m05 are listed.
					if listed != 2 || more != "+37 more" {
						t.Fatalf("%s: %d listed, %q", where, listed, more)
					}
				default:
					if more == "" || listed+atoi(strings.Fields(strings.TrimPrefix(more, "+"))[0]) != total {
						t.Fatalf("%s: %d listed and %q, want %d in all", where, listed, more, total)
					}
				}
				if out != "" && preset == PresetScreen {
					im, err := Raster(f, spec)
					if err == nil {
						var b bytes.Buffer
						png.Encode(&b, im)
						os.WriteFile(filepath.Join(out, fmt.Sprintf("legend40-%s-%s.png", name, strings.Trim(fmt.Sprintf("%q", mode), `"`)+"x")), b.Bytes(), 0o644)
					}
				}
			}
		}
	}
	// "all" lists hidden series muted.
	res := curves(3, labels["short"])
	res.Series[1].Hidden = true
	res.Visual.LegendMode = LegendAll
	f := Layout(res, Preset(PresetScreen), tr)
	muted := false
	for _, op := range f.Ops {
		if op.Role == "legend" && op.K == "text" && op.Text == "S01" && op.Fill == inkMuted {
			muted = true
		}
	}
	if !muted {
		t.Fatal("hidden series not listed muted in the \"all\" legend")
	}
}

func atoi(s string) int {
	n := 0
	fmt.Sscan(s, &n)
	return n
}

func TestDemoFigures(t *testing.T) {
	out := os.Getenv("MNELAB_PLOT_OUT")
	if out == "" {
		t.Skip("set MNELAB_PLOT_OUT to write demo figures")
	}
	for _, n := range []int{2, 4, 6, 8, 12} {
		res := curves(n, func(k int) string { return fmt.Sprintf("Series %d", k+1) })
		spec := Preset(PresetScreen)
		spec.Width, spec.Height = 1100, 640
		f := Layout(res, spec, tr)
		im, err := Raster(f, spec)
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		png.Encode(&b, im)
		os.WriteFile(filepath.Join(out, fmt.Sprintf("curves-%02d.png", n)), b.Bytes(), 0o644)
		os.WriteFile(filepath.Join(out, fmt.Sprintf("curves-%02d.svg", n)), SVG(f, spec), 0o644)
		pdf, _ := PDF(f, spec, time.Time{})
		os.WriteFile(filepath.Join(out, fmt.Sprintf("curves-%02d.pdf", n)), pdf, 0o644)
	}
	f, spec, _ := shapesFigure()
	im, _ := Raster(f, spec)
	var b bytes.Buffer
	png.Encode(&b, im)
	os.WriteFile(filepath.Join(out, "shapes.png"), b.Bytes(), 0o644)
}
