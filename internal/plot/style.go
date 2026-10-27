package plot

import (
	"fmt"
	"math"

	"github.com/oaovito/mne_lab/internal/science/graph"
)

// Series styling: colours, marker shapes and line patterns. Series are never
// told apart by colour alone: series that would otherwise differ only in
// colour also get distinct line patterns or marker shapes, so figures stay
// readable in grayscale print and for colour-vision deficiencies.

// Legend modes (graph.Visual.LegendMode). The default lists the visible
// series.
const (
	LegendVisible  = "visible"
	LegendAll      = "all"      // hidden series too, muted
	LegendSelected = "selected" // only graph.Visual.LegendSeries
)

// Marker shapes.
const (
	MarkerCircle       = "circle"
	MarkerSquare       = "square"
	MarkerTriangle     = "triangle"
	MarkerDiamond      = "diamond"
	MarkerTriangleDown = "triangle_down"
	MarkerCross        = "cross"
	// MarkerNone draws a curve without identity markers (only for curves;
	// points that are data, such as means and replicates, are always drawn).
	MarkerNone = "none"
)

// Markers is the default order of marker shapes.
var Markers = []string{MarkerCircle, MarkerSquare, MarkerTriangle, MarkerDiamond, MarkerTriangleDown, MarkerCross}

// Line styles.
const (
	LineSolid      = "solid"
	LineDashed     = "dashed"
	LineDotted     = "dotted"
	LineDashDot    = "dashdot"
	LineLongDash   = "longdash"
	LineDashDotDot = "dashdotdot"
)

// LineStyles is the default order of line patterns.
var LineStyles = []string{LineSolid, LineDashed, LineDotted, LineDashDot, LineLongDash, LineDashDotDot}

// Dash patterns in multiples of the line width. Lines have round caps,
// which add one width to every dash, so dots are nearly zero long and gaps
// leave room for the caps.
var dashPatterns = map[string][]float64{
	LineSolid:      nil,
	LineDashed:     {3.2, 2.4},
	LineDotted:     {0.1, 2.4},
	LineDashDot:    {3.2, 2.2, 0.1, 2.2},
	LineLongDash:   {6.5, 2.6},
	LineDashDotDot: {3.2, 2.0, 0.1, 2.0, 0.1, 2.0},
}

// Palette is the default series palette: the owner's blue (#2C7FB8) and
// navy (#143A52) with muted teal, restrained rust, ochre, graphite and other
// desaturated colours. The order alternates light and dark so that
// neighbouring series also differ in grayscale; the first six differ by at
// least 7 L* from each other, and every colour keeps at least 2.2:1 contrast
// on white paper. No saturated greens.
var Palette = []string{
	"#2C7FB8", // owner blue
	"#883717", // restrained rust
	"#549D94", // muted teal
	"#143A52", // owner navy
	"#C89F5A", // muted ochre
	"#61666D", // graphite
	"#9A74A2", // muted plum
	"#72323A", // wine
	"#92B3C6", // pale steel
	"#545183", // slate indigo
}

// SeriesColor returns the default colour of the n-th series. Past the
// palette, colours are generated with perceptual spacing (golden-angle hues
// at moderate chroma in OKLCH, alternating lightness, no green band), so
// any number of series gets distinct, restrained colours.
func SeriesColor(n int) string {
	if n < 0 {
		n = 0
	}
	if n < len(Palette) {
		return Palette[n]
	}
	k := n - len(Palette)
	h := math.Mod(250+float64(k)*137.50776, 360)
	// Skip the yellow-green to green band (no fluorescent greens).
	if h >= 95 && h < 180 {
		h = math.Mod(h+85, 360)
	}
	lightness := []float64{0.50, 0.64, 0.42, 0.57, 0.70}[k%5]
	return oklchHex(lightness, 0.085, h)
}

// oklchHex converts an OKLCH colour to #RRGGBB, clipping to sRGB.
func oklchHex(l, c, h float64) string {
	a, b := c*math.Cos(h*math.Pi/180), c*math.Sin(h*math.Pi/180)
	l_ := l + 0.3963377774*a + 0.2158037573*b
	m_ := l - 0.1055613458*a - 0.0638541728*b
	s_ := l - 0.0894841775*a - 1.2914855480*b
	L, M, S := l_*l_*l_, m_*m_*m_, s_*s_*s_
	r := 4.0767416621*L - 3.3077115913*M + 0.2309699292*S
	g := -1.2684380046*L + 2.6097574011*M - 0.3413193965*S
	bb := -0.0041960863*L - 0.7034186147*M + 1.7076127010*S
	enc := func(x float64) int {
		x = math.Max(0, math.Min(1, x))
		if x <= 0.0031308 {
			x *= 12.92
		} else {
			x = 1.055*math.Pow(x, 1/2.4) - 0.055
		}
		return int(math.Round(x * 255))
	}
	return fmt.Sprintf("#%02X%02X%02X", enc(r), enc(g), enc(bb))
}

// Luminance returns the relative luminance (WCAG) of a #RRGGBB colour, the
// value grayscale printing keeps.
func Luminance(hex string) float64 {
	c := ParseColor(hex)
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
}

// Lightness returns the CIE L* (0 to 100) of a #RRGGBB colour.
func Lightness(hex string) float64 {
	y := Luminance(hex)
	if y > 216.0/24389 {
		return 116*math.Cbrt(y) - 16
	}
	return y * 24389 / 27
}

func validMarker(m string) bool {
	if m == MarkerNone {
		return true
	}
	for _, v := range Markers {
		if v == m {
			return true
		}
	}
	return false
}

func validLineStyle(s string) bool {
	_, ok := dashPatterns[s]
	return ok
}

// dashFor returns the dash pattern of a line style at a line width.
func dashFor(style string, lw float64) []float64 {
	p := dashPatterns[style]
	if len(p) == 0 {
		return nil
	}
	out := make([]float64, len(p))
	for i, v := range p {
		out[i] = v * lw
	}
	return out
}

// styleIndex gives the k-th series of a kind a line style and marker so that
// the first len(LineStyles)*len(Markers) series all differ in the pair, and
// the first six differ in both.
func styleIndex(k int) (line, marker string) {
	return LineStyles[k%len(LineStyles)], Markers[(k+k/len(LineStyles))%len(Markers)]
}

// markerOp returns the drawing operation of one marker. Shapes have about
// the same visual weight as a circle of radius r; their outline points are
// computed here so every renderer draws the same geometry.
func markerOp(shape string, x, y, r float64, fill string, opacity float64, stroke string, sw float64, role string) Op {
	op := Op{K: "marker", Shape: shape, X: x, Y: y, R: r, Fill: fill, Opacity: opacity, Stroke: stroke, SW: sw, Role: role}
	switch shape {
	case MarkerSquare:
		d := r * 0.89
		op.Pts = []float64{x - d, y - d, x + d, y - d, x + d, y + d, x - d, y + d}
	case MarkerDiamond:
		d := r * 1.25
		op.Pts = []float64{x, y - d, x + d, y, x, y + d, x - d, y}
	case MarkerTriangle, MarkerTriangleDown:
		// Equilateral, centred on its centroid.
		d := r * 1.35
		s := 1.0
		if shape == MarkerTriangleDown {
			s = -1
		}
		op.Pts = []float64{x, y - s*d, x + d*0.866, y + s*d*0.5, x - d*0.866, y + s*d*0.5}
	case MarkerCross:
		// Two strokes (an ×), drawn in the series colour.
		d := r * 0.95
		op.Pts = []float64{x - d, y - d, x + d, y + d, x - d, y + d, x + d, y - d}
		op.Stroke = fill
		if op.Stroke == "" {
			op.Stroke = stroke
		}
		op.Fill, op.Opacity = "", 0
		op.SW = math.Max(sw, r*0.45)
	default:
		op.Shape = MarkerCircle
	}
	return op
}

// sparseMarkers picks the data points that carry identity markers on a
// curve: about n of them, evenly spaced along the curve as drawn, shifted
// by phase (0 to 1) so curves that overlap do not put markers in the same
// places. Markers sit on real data points only. Points on the bottom edge
// of the plot (empty classes of a distribution) are skipped when the curve
// has others, so markers do not pile up on a shared baseline.
func sparseMarkers(pts []float64, plot Rect, n int, phase float64) []int {
	inside := func(i int) bool {
		x, y := pts[2*i], pts[2*i+1]
		return x >= plot.X-0.01 && x <= plot.X+plot.W+0.01 && y >= plot.Y-0.01 && y <= plot.Y+plot.H+0.01
	}
	floor := plot.Y + plot.H - plot.H*0.02
	count := len(pts) / 2
	var cand []int
	for i := 0; i < count; i++ {
		if inside(i) && pts[2*i+1] < floor {
			cand = append(cand, i)
		}
	}
	if len(cand) == 0 {
		for i := 0; i < count; i++ {
			if inside(i) {
				cand = append(cand, i)
			}
		}
	}
	if len(cand) == 0 || n <= 0 {
		return nil
	}
	if len(cand) <= n {
		return cand
	}
	// Cumulative length along the candidate points.
	cum := make([]float64, len(cand))
	for j := 1; j < len(cand); j++ {
		a, b := cand[j-1], cand[j]
		cum[j] = cum[j-1] + math.Hypot(pts[2*b]-pts[2*a], pts[2*b+1]-pts[2*a+1])
	}
	total := cum[len(cum)-1]
	if total <= 0 {
		return cand[:1]
	}
	var out []int
	j := 0
	for m := 0; m < n; m++ {
		target := (float64(m) + phase) / float64(n) * total
		for j < len(cand)-1 && cum[j] < target {
			j++
		}
		if len(out) == 0 || out[len(out)-1] != cand[j] {
			out = append(out, cand[j])
		}
	}
	return out
}

// look is how one series is drawn.
type look struct {
	color  string
	line   string // line style
	dash   []float64
	marker string // shape, or MarkerNone on a curve
	curve  bool   // a line series (distribution curve)
	every  bool   // a curve with a marker on every point
	sparse bool   // a curve with a few identity markers
}

// shown reports whether the series draws markers: means and individual
// points always do, curves only when they carry markers.
func (l look) shown() bool { return !l.curve || l.every || l.sparse }

// assignStyles gives every series its colour, line style and marker. Saved
// choices (colour, marker, line style) are kept; the rest follow the order
// of the series, counted over hidden series too so hiding one never changes
// the look of the others.
//
//   - Curves (distributions): the k-th curve gets the k-th line style; the
//     replicates of one cycle point share a colour and differ in line style.
//     When two visible curves would share a line style, every curve also
//     gets identity markers (a few per curve, on data points), so no two
//     curves differ only in colour.
//   - Means and individual points: the k-th series gets the k-th marker and
//     line style, except on statistical graphs, where each group already has
//     its own position on the axis.
func assignStyles(res graph.Result, lw float64, points bool) []look {
	looks := make([]look, len(res.Series))
	statistical := res.Kind == graph.KindStatistical
	groupColor := map[string]string{}
	groupIndex := map[string]int{}
	groupSeen := map[string]int{}
	ci, kCurve, kMean, kInd := 0, 0, 0, 0
	for i, se := range res.Series {
		lk := &looks[i]
		group := replicateGroup(se)
		c := se.Color
		switch {
		case c != "":
		case se.Kind == "individual":
			c = inkIndiv
		case group != "" && groupColor[group] != "":
			c = groupColor[group]
		default:
			c = SeriesColor(ci)
			ci++
		}
		if group != "" && groupColor[group] == "" {
			groupColor[group] = c
		}
		lk.color = c
		isCurve := false
		switch se.Kind {
		case "individual":
			lk.line, lk.marker = LineSolid, MarkerCircle
			if !statistical {
				_, lk.marker = styleIndex(kInd)
			}
			kInd++
		case "mean":
			lk.line, lk.marker = LineSolid, MarkerCircle
			if !statistical {
				lk.line, lk.marker = styleIndex(kMean)
			}
			kMean++
		default:
			isCurve = true
			if group != "" {
				gi, ok := groupIndex[group]
				if !ok {
					gi = len(groupIndex)
					groupIndex[group] = gi
				}
				lk.line = LineStyles[groupSeen[group]%len(LineStyles)]
				lk.marker = Markers[gi%len(Markers)]
				groupSeen[group]++
			} else {
				lk.line, lk.marker = styleIndex(kCurve)
				kCurve++
			}
		}
		if validLineStyle(se.LineStyle) {
			lk.line = se.LineStyle
		}
		if validMarker(se.Marker) && (isCurve || se.Marker != MarkerNone) {
			lk.marker = se.Marker
		}
		lk.dash = dashFor(lk.line, lw)
		lk.curve = isCurve
		if isCurve && lk.marker != MarkerNone {
			lk.every = points
			lk.sparse = !points && validMarker(se.Marker)
		}
	}
	// Curves that would differ only in colour get identity markers.
	lines := map[string]bool{}
	collide := false
	for i, se := range res.Series {
		if se.Hidden || !isCurveKind(se.Kind) {
			continue
		}
		if lines[looks[i].line] {
			collide = true
		}
		lines[looks[i].line] = true
	}
	if collide {
		for i, se := range res.Series {
			if isCurveKind(se.Kind) && looks[i].marker != MarkerNone && !looks[i].every {
				looks[i].sparse = true
			}
		}
	}
	return looks
}

func isCurveKind(kind string) bool { return kind != "individual" && kind != "mean" }
