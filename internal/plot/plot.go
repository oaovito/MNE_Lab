// Package plot lays out a computed graph as a display list: a short,
// renderer-neutral list of drawing operations in points (1/72 inch). The
// interface draws the same list on screen that the export engine writes to
// SVG, PDF and raster files, so a preview is exactly what gets exported.
//
// Layout never changes data: values are mapped to coordinates, never
// smoothed, clipped away or rescaled. Zoom only changes the visible window.
package plot

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/oaovito/mne_lab/internal/science/graph"
)

// Units for figure dimensions.
const (
	UnitPx = "px"
	UnitMM = "mm"
	UnitCM = "cm"
	UnitIn = "in"
)

// Limits keep exports usable on modest computers.
const (
	MinDPI       = 72
	MaxDPI       = 1200
	MaxPixels    = 50_000_000 // ~200 MB of RGBA while rendering
	MaxSidePixel = 20_000
)

// Errors (stable identifiers).
var (
	ErrSize     = errors.New("export.invalid_size")
	ErrDPI      = errors.New("export.invalid_dpi")
	ErrTooLarge = errors.New("export.image_too_large")
)

// Spec is the figure configuration chosen in the export dialog.
type Spec struct {
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	Unit       string  `json:"unit"`
	DPI        float64 `json:"dpi"`
	Background string  `json:"background"` // solid or transparent
	BackColor  string  `json:"backColor,omitempty"`
	Title      bool    `json:"title"`
	Legend     bool    `json:"legend"`
	Metadata   bool    `json:"metadata"`
	Grid       bool    `json:"grid"`
	Points     bool    `json:"points"`
	LineWidth  float64 `json:"lineWidth"` // pt
	FontSize   float64 `json:"fontSize"`  // pt
	Margin     float64 `json:"margin"`    // mm
	Scale      float64 `json:"scale"`     // visual scaling of text, lines and markers
	Decimal    string  `json:"decimal,omitempty"`
}

// Presets of the export dialog.
const (
	PresetScreen       = "screen"
	PresetPresentation = "presentation"
	PresetPrint        = "print"
	PresetPublication  = "publication"
	PresetCustom       = "custom"
)

// Preset returns the figure configuration of a preset.
func Preset(name string) Spec {
	s := Spec{Background: "solid", BackColor: "#FFFFFF", Title: true, Legend: true, Metadata: false, Grid: true, Scale: 1, Margin: 4}
	switch name {
	case PresetPresentation:
		s.Width, s.Height, s.Unit, s.DPI = 1920, 1080, UnitPx, 144
		s.FontSize, s.LineWidth = 13, 2
	case PresetPrint:
		s.Width, s.Height, s.Unit, s.DPI = 180, 120, UnitMM, 300
		s.FontSize, s.LineWidth, s.Metadata = 9, 1.25, true
	case PresetPublication:
		s.Width, s.Height, s.Unit, s.DPI = 140, 95, UnitMM, 600
		s.FontSize, s.LineWidth, s.Title, s.Margin = 8, 1, false, 3
	default: // screen and custom start from the screen preset
		s.Width, s.Height, s.Unit, s.DPI = 1600, 1000, UnitPx, 144
		s.FontSize, s.LineWidth = 10, 1.5
	}
	return s
}

// SizePt returns the physical size in points.
func (s Spec) SizePt() (float64, float64) {
	f := 1.0
	switch s.Unit {
	case UnitMM:
		f = 72 / 25.4
	case UnitCM:
		f = 72 / 2.54
	case UnitIn:
		f = 72
	default:
		dpi := s.DPI
		if dpi <= 0 {
			dpi = 96
		}
		f = 72 / dpi
	}
	return s.Width * f, s.Height * f
}

// Pixels returns the raster size at the configured DPI.
func (s Spec) Pixels() (int, int) {
	w, h := s.SizePt()
	if s.Unit == UnitPx || s.Unit == "" {
		return int(math.Round(s.Width)), int(math.Round(s.Height))
	}
	return int(math.Round(w / 72 * s.DPI)), int(math.Round(h / 72 * s.DPI))
}

// Validate checks dimensions and resolution.
func (s Spec) Validate(raster bool) error {
	w, h := s.SizePt()
	if !(w >= 72 && h >= 54) || w > 72*100 || h > 72*100 || math.IsNaN(w) || math.IsNaN(h) {
		return ErrSize
	}
	if s.DPI < MinDPI || s.DPI > MaxDPI {
		return ErrDPI
	}
	if raster {
		pw, ph := s.Pixels()
		if pw > MaxSidePixel || ph > MaxSidePixel || pw*ph > MaxPixels {
			return ErrTooLarge
		}
	}
	return nil
}

func (s Spec) normalized() Spec {
	if s.Scale <= 0 {
		s.Scale = 1
	}
	if s.FontSize <= 0 {
		s.FontSize = 10
	}
	if s.LineWidth <= 0 {
		s.LineWidth = 1.5
	}
	if s.Margin < 0 {
		s.Margin = 0
	}
	if s.Decimal != "," {
		s.Decimal = "."
	}
	if s.BackColor == "" {
		s.BackColor = "#FFFFFF"
	}
	return s
}

// Op is one drawing operation. Coordinates are points from the top-left.
type Op struct {
	K       string    `json:"k"` // rect, line, poly, circle, text, clip, unclip
	X       float64   `json:"x,omitempty"`
	Y       float64   `json:"y,omitempty"`
	W       float64   `json:"w,omitempty"`
	H       float64   `json:"h,omitempty"`
	X2      float64   `json:"x2,omitempty"`
	Y2      float64   `json:"y2,omitempty"`
	R       float64   `json:"r,omitempty"`
	Pts     []float64 `json:"pts,omitempty"`
	Fill    string    `json:"fill,omitempty"`
	Stroke  string    `json:"stroke,omitempty"`
	SW      float64   `json:"sw,omitempty"`
	Dash    []float64 `json:"dash,omitempty"`
	Opacity float64   `json:"op,omitempty"` // fill opacity; 0 means opaque
	Text    string    `json:"t,omitempty"`
	Size    float64   `json:"size,omitempty"`
	Font    string    `json:"font,omitempty"`
	Anchor  string    `json:"anchor,omitempty"` // start, middle, end
	Rotate  float64   `json:"rot,omitempty"`    // degrees, around (X, Y)
	Role    string    `json:"role,omitempty"`   // semantic hint for the interface
}

// Hit links a drawn point back to its data for tooltips.
type Hit struct {
	Series int     `json:"s"`
	Index  int     `json:"i"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
}

// Rect is a rectangle in points.
type Rect struct {
	X, Y, W, H float64
}

// Mapping converts data values to plot coordinates (for the interface's
// zoom and tooltips).
type Mapping struct {
	Plot   Rect    `json:"plot"`
	XMin   float64 `json:"xMin"`
	XMax   float64 `json:"xMax"`
	YMin   float64 `json:"yMin"`
	YMax   float64 `json:"yMax"`
	XScale string  `json:"xScale"`
	YScale string  `json:"yScale"`
}

// Figure is a laid-out graph.
type Figure struct {
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	Background string  `json:"background,omitempty"` // empty when transparent
	Ops        []Op    `json:"ops"`
	Hits       []Hit   `json:"hits,omitempty"`
	Map        Mapping `json:"map"`
	Title      string  `json:"title"`
	Desc       string  `json:"desc,omitempty"`
}

// Translator returns localized text for a key. Pairs in kv replace {name}.
type Translator func(key string, kv ...string) string

// Colors of figures (high-contrast, color-blind friendly order).
var Palette = []string{"#0072B2", "#D55E00", "#009E73", "#CC79A7", "#E69F00", "#56B4E9", "#7F3C8D", "#3B3B3B", "#B8860B", "#11A579"}

const (
	inkText  = "#1D232B"
	inkMuted = "#59606B"
	inkAxis  = "#4A5361"
	inkGrid  = "#E3E7ED"
	inkMinor = "#F0F2F5"
	inkGap   = "#B4BAC4"
	inkIndiv = "#7C8696"
)

type axisInfo struct {
	min, max float64
	log      bool
	ticks    []float64
	minor    []float64
	labels   []string
}

// Layout produces the display list of a computed graph.
func Layout(res graph.Result, spec Spec, T Translator) Figure {
	s := spec.normalized()
	W, H := s.SizePt()
	fs := s.FontSize * s.Scale
	lw := s.LineWidth * s.Scale
	m := s.Margin * 72 / 25.4
	fig := Figure{Width: W, Height: H, Title: graphTitle(res, T)}
	if s.Background != "transparent" {
		fig.Background = s.BackColor
		fig.Ops = append(fig.Ops, Op{K: "rect", X: 0, Y: 0, W: W, H: H, Fill: s.BackColor, Role: "background"})
	}
	top, bottom, left, right := m, H-m, m, W-m

	// Title.
	if s.Title && fig.Title != "" {
		ts := fs * 1.25
		fig.Ops = append(fig.Ops, Op{K: "text", X: left, Y: top + ts, Text: fit(fig.Title, FontSansBold, ts, right-left), Size: ts, Font: FontSansBold, Fill: inkText, Anchor: "start", Role: "title"})
		top += ts * 1.7
	}

	// Series (visible only) and their colors.
	type entry struct {
		idx   int
		label string
		color string
		kind  string
		dash  []float64
	}
	var entries []entry
	// Replicates of the same cycle point share a color and differ by line
	// pattern, so many curves stay readable.
	dashes := make([][]float64, len(res.Series))
	groupColor := map[string]string{}
	groupSeen := map[string]int{}
	ci := 0
	for i, se := range res.Series {
		c := se.Color
		group := replicateGroup(se)
		if group != "" {
			if n := groupSeen[group]; n > 0 {
				dashes[i] = replicateDash(n, lw)
			}
			groupSeen[group]++
		}
		switch {
		case c != "":
		case se.Kind == "individual":
			c = inkIndiv
		case group != "" && groupColor[group] != "":
			c = groupColor[group]
		default:
			c = Palette[ci%len(Palette)]
			ci++
		}
		if group != "" && groupColor[group] == "" {
			groupColor[group] = c
		}
		res.Series[i].Color = c
		if !se.Hidden {
			entries = append(entries, entry{i, SeriesLabel(se.Label, T), c, se.Kind, dashes[i]})
		}
	}

	// Comparison labels must remain interpretable even when optional metadata
	// is hidden. Wrap the correction/threshold note instead of truncating it.
	if len(res.Annotations) > 0 {
		seen := map[string]bool{}
		corrections := []string{}
		for _, a := range res.Annotations {
			if !seen[a.Correction] {
				seen[a.Correction] = true
				corrections = append(corrections, a.Correction)
			}
		}
		notes := []string{T("plot.stat.annotations", "correction", strings.Join(corrections, "; "))}
		if res.AnnotationStyle == "stars" {
			notes = append(notes, T("plot.stat.stars"))
		}
		nfs := fs * .72
		lines := []string{}
		for _, note := range notes {
			line := ""
			for _, word := range strings.Fields(note) {
				next := strings.TrimSpace(line + " " + word)
				if line != "" && TextWidth(FontSans, next, nfs) > right-left {
					lines = append(lines, line)
					line = word
				} else {
					line = next
				}
			}
			if line != "" {
				lines = append(lines, line)
			}
		}
		for i := len(lines) - 1; i >= 0; i-- {
			fig.Ops = append(fig.Ops, Op{K: "text", X: left, Y: bottom, Text: lines[i], Size: nfs, Font: FontSans, Fill: inkMuted, Anchor: "start", Role: "statistical-annotation-note"})
			bottom -= nfs * 1.4
		}
		bottom -= nfs * .5
	}
	// Metadata block at the bottom.
	if s.Metadata {
		ms := fs * 0.82
		lines := metadataLines(res, T, s.Decimal)
		maxLines := 6
		if len(lines) > maxLines {
			more := len(lines) - (maxLines - 1)
			lines = append(lines[:maxLines-1], T("plot.more", "n", strconv.Itoa(more)))
		}
		for i := len(lines) - 1; i >= 0; i-- {
			fig.Ops = append(fig.Ops, Op{K: "text", X: left, Y: bottom, Text: fit(lines[i], FontSans, ms, right-left), Size: ms, Font: FontSans, Fill: inkMuted, Anchor: "start", Role: "metadata"})
			bottom -= ms * 1.4
		}
		if len(lines) > 0 {
			bottom -= ms * 0.5
		}
	}

	// Legend: a right column when there is room, otherwise rows below.
	sw := fs * 1.8
	if s.Legend && len(entries) > 0 {
		maxw := 0.0
		for _, e := range entries {
			maxw = math.Max(maxw, TextWidth(FontSans, e.label, fs))
		}
		colW := sw + fs*0.5 + maxw
		if colW < (right-left)*0.28 && W > 300 {
			x := right - colW
			y := top + fs
			rowH := fs * 1.5
			maxRows := int((bottom - top - fs*3) / rowH)
			for i, e := range entries {
				if i >= maxRows-1 && len(entries) > maxRows {
					fig.Ops = append(fig.Ops, Op{K: "text", X: x, Y: y, Text: T("plot.more", "n", strconv.Itoa(len(entries)-i)), Size: fs, Font: FontSans, Fill: inkMuted, Anchor: "start", Role: "legend"})
					break
				}
				fig.Ops = append(fig.Ops, legendSwatch(x, y-fs*0.32, sw, e.color, e.kind, e.dash, lw, fs)...)
				fig.Ops = append(fig.Ops, Op{K: "text", X: x + sw + fs*0.5, Y: y, Text: e.label, Size: fs, Font: FontSans, Fill: inkText, Anchor: "start", Role: "legend"})
				y += rowH
			}
			right = x - fs*1.2
		} else {
			// Rows, laid out bottom-up after computing line breaks.
			var rows [][]int
			var row []int
			used := 0.0
			for i, e := range entries {
				w := sw + fs*0.5 + math.Min(TextWidth(FontSans, e.label, fs), (right-left)*0.6) + fs*1.4
				if used+w > right-left && len(row) > 0 {
					rows, row, used = append(rows, row), nil, 0
				}
				row = append(row, i)
				used += w
			}
			rows = append(rows, row)
			more := -1 // entry replaced by "+N more" when rows are cut
			if len(rows) > 3 {
				rows = rows[:3]
				last := rows[2]
				more = last[len(last)-1]
			}
			rowH := fs * 1.5
			y := bottom - rowH*float64(len(rows)-1)
			for _, r := range rows {
				x := left
				for _, i := range r {
					if i == more {
						fig.Ops = append(fig.Ops, Op{K: "text", X: x, Y: y, Text: T("plot.more", "n", strconv.Itoa(len(entries)-i)), Size: fs, Font: FontSans, Fill: inkMuted, Anchor: "start", Role: "legend"})
						break
					}
					e := entries[i]
					label := fit(e.label, FontSans, fs, (right-left)*0.6)
					fig.Ops = append(fig.Ops, legendSwatch(x, y-fs*0.32, sw, e.color, e.kind, e.dash, lw, fs)...)
					fig.Ops = append(fig.Ops, Op{K: "text", X: x + sw + fs*0.5, Y: y, Text: label, Size: fs, Font: FontSans, Fill: inkText, Anchor: "start", Role: "legend"})
					x += sw + fs*0.5 + TextWidth(FontSans, label, fs) + fs*1.4
				}
				y += rowH
			}
			bottom -= rowH*float64(len(rows)) + fs*0.4
		}
	}

	// Axes.
	view := res.Visual.View
	xa := buildAxis(res.X, view, true, res.Kind == graph.KindParameterTime, s.Decimal)
	ya := buildAxis(res.Y, view, false, false, s.Decimal)
	tick := fs * 0.45
	yLabelW := 0.0
	for _, l := range ya.labels {
		yLabelW = math.Max(yLabelW, TextWidth(FontSans, l, fs*0.92))
	}
	pl := left + fs*1.5 + yLabelW + tick + fs*0.4
	pr := right - TextWidth(FontSans, lastOr(xa.labels), fs*0.92)/2
	pt := top + fs*0.5
	pb := bottom - (fs*0.92 + tick + fs*0.5) - fs*1.5
	if pr-pl < 40 || pb-pt < 30 { // tiny figures: keep something drawable
		pl, pr, pt, pb = left, right, top, bottom
	}
	fig.Map = Mapping{Plot: Rect{pl, pt, pr - pl, pb - pt}, XMin: xa.min, XMax: xa.max, YMin: ya.min, YMax: ya.max, XScale: scaleName(xa.log), YScale: scaleName(ya.log)}
	mx := func(v float64) float64 { return pl + frac(v, xa)*(pr-pl) }
	my := func(v float64) float64 { return pb - frac(v, ya)*(pb-pt) }

	// Grid.
	if s.Grid {
		for _, v := range xa.minor {
			fig.Ops = append(fig.Ops, Op{K: "line", X: mx(v), Y: pt, X2: mx(v), Y2: pb, Stroke: inkMinor, SW: 0.5 * s.Scale, Role: "grid"})
		}
		for _, v := range xa.ticks {
			fig.Ops = append(fig.Ops, Op{K: "line", X: mx(v), Y: pt, X2: mx(v), Y2: pb, Stroke: inkGrid, SW: 0.6 * s.Scale, Role: "grid"})
		}
		for _, v := range ya.ticks {
			fig.Ops = append(fig.Ops, Op{K: "line", X: pl, Y: my(v), X2: pr, Y2: my(v), Stroke: inkGrid, SW: 0.6 * s.Scale, Role: "grid"})
		}
	}

	// Missing cycle points are shown, never interpolated.
	for _, g := range res.Gaps {
		if g >= xa.min && g <= xa.max {
			fig.Ops = append(fig.Ops, Op{K: "line", X: mx(g), Y: pt, X2: mx(g), Y2: pb, Stroke: inkGap, SW: 0.8 * s.Scale, Dash: []float64{3 * s.Scale, 3 * s.Scale}, Role: "gap"})
		}
	}

	// Data, clipped to the plot area (matters only when zoomed).
	fig.Ops = append(fig.Ops, Op{K: "clip", X: pl, Y: pt, W: pr - pl, H: pb - pt})
	gapBetween := func(a, b float64) bool {
		for _, g := range res.Gaps {
			if g > a && g < b {
				return true
			}
		}
		return false
	}
	for si, se := range res.Series {
		if se.Hidden {
			continue
		}
		c := se.Color
		switch se.Kind {
		case "individual":
			r := fs * 0.26 * 1.0
			for i := range se.X {
				x, y := mx(se.X[i]), my(se.Y[i])
				fig.Ops = append(fig.Ops, Op{K: "circle", X: x, Y: y, R: r, Fill: c, Opacity: 0.35, Stroke: c, SW: 0.7 * s.Scale, Role: "point"})
				fig.Hits = append(fig.Hits, Hit{si, i, x, y})
			}
		case "mean":
			var pts []float64
			flush := func() {
				if len(pts) >= 4 {
					fig.Ops = append(fig.Ops, Op{K: "poly", Pts: pts, Stroke: c, SW: lw, Role: "series"})
				}
				pts = nil
			}
			for i := range se.X {
				if i > 0 && gapBetween(se.X[i-1], se.X[i]) {
					flush()
				}
				pts = append(pts, mx(se.X[i]), my(se.Y[i]))
			}
			flush()
			cap := fs * 0.35
			for i := range se.X {
				x := mx(se.X[i])
				if i < len(se.Err) && se.Err[i] > 0 {
					y1, y2 := my(se.Y[i]-se.Err[i]), my(se.Y[i]+se.Err[i])
					fig.Ops = append(fig.Ops,
						Op{K: "line", X: x, Y: y1, X2: x, Y2: y2, Stroke: c, SW: lw * 0.8, Role: "error"},
						Op{K: "line", X: x - cap, Y: y1, X2: x + cap, Y2: y1, Stroke: c, SW: lw * 0.8, Role: "error"},
						Op{K: "line", X: x - cap, Y: y2, X2: x + cap, Y2: y2, Stroke: c, SW: lw * 0.8, Role: "error"})
				}
				y := my(se.Y[i])
				fig.Ops = append(fig.Ops, Op{K: "circle", X: x, Y: y, R: fs * 0.3, Fill: c, Role: "point"})
				fig.Hits = append(fig.Hits, Hit{si, i, x, y})
			}
		default:
			pts := make([]float64, 0, len(se.X)*2)
			for i := range se.X {
				if xa.log && se.X[i] <= 0 {
					continue
				}
				pts = append(pts, mx(se.X[i]), my(se.Y[i]))
			}
			if len(pts) >= 4 {
				fig.Ops = append(fig.Ops, Op{K: "poly", Pts: pts, Stroke: c, SW: lw, Dash: dashes[si], Role: "series"})
			}
			for i := 0; i+1 < len(pts); i += 2 {
				if s.Points || res.Visual.Points {
					fig.Ops = append(fig.Ops, Op{K: "circle", X: pts[i], Y: pts[i+1], R: lw * 1.3, Fill: c, Role: "point"})
				}
			}
			j := 0
			for i := range se.X {
				if xa.log && se.X[i] <= 0 {
					continue
				}
				fig.Hits = append(fig.Hits, Hit{si, i, pts[j], pts[j+1]})
				j += 2
			}
		}
	}
	for _, annotation := range res.Annotations {
		x1, x2, y := mx(annotation.X1), mx(annotation.X2), my(annotation.Y)
		label := annotation.Label
		if label == "statistics.below_precision" {
			label = T("stat.p_underflow")
		}
		fig.Ops = append(fig.Ops,
			Op{K: "line", X: x1, Y: y + fs*.35, X2: x1, Y2: y, Stroke: inkAxis, SW: .8 * s.Scale, Role: "statistical-annotation"},
			Op{K: "line", X: x1, Y: y, X2: x2, Y2: y, Stroke: inkAxis, SW: .8 * s.Scale, Role: "statistical-annotation"},
			Op{K: "line", X: x2, Y: y, X2: x2, Y2: y + fs*.35, Stroke: inkAxis, SW: .8 * s.Scale, Role: "statistical-annotation"},
			Op{K: "text", X: (x1 + x2) / 2, Y: y - fs*.35, Text: label, Size: fs * .85, Font: FontSans, Fill: inkAxis, Anchor: "middle", Role: "statistical-annotation"})
	}
	fig.Ops = append(fig.Ops, Op{K: "unclip"})

	// Axis lines, ticks and labels.
	aw := 0.8 * s.Scale
	fig.Ops = append(fig.Ops,
		Op{K: "line", X: pl, Y: pb, X2: pr, Y2: pb, Stroke: inkAxis, SW: aw, Role: "axis"},
		Op{K: "line", X: pl, Y: pt, X2: pl, Y2: pb, Stroke: inkAxis, SW: aw, Role: "axis"})
	tfs := fs * 0.92
	for i, v := range xa.ticks {
		x := mx(v)
		fig.Ops = append(fig.Ops, Op{K: "line", X: x, Y: pb, X2: x, Y2: pb + tick, Stroke: inkAxis, SW: aw, Role: "axis"},
			Op{K: "text", X: x, Y: pb + tick + tfs*1.05, Text: xa.labels[i], Size: tfs, Font: FontSans, Fill: inkMuted, Anchor: "middle", Role: "tick"})
	}
	for _, v := range xa.minor {
		x := mx(v)
		fig.Ops = append(fig.Ops, Op{K: "line", X: x, Y: pb, X2: x, Y2: pb + tick*0.55, Stroke: inkAxis, SW: aw * 0.8, Role: "axis"})
	}
	for i, v := range ya.ticks {
		y := my(v)
		fig.Ops = append(fig.Ops, Op{K: "line", X: pl - tick, Y: y, X2: pl, Y2: y, Stroke: inkAxis, SW: aw, Role: "axis"},
			Op{K: "text", X: pl - tick - fs*0.3, Y: y + tfs*0.35, Text: ya.labels[i], Size: tfs, Font: FontSans, Fill: inkMuted, Anchor: "end", Role: "tick"})
	}
	xl, yl := AxisLabel(res.X, T), AxisLabel(res.Y, T)
	fig.Ops = append(fig.Ops,
		Op{K: "text", X: (pl + pr) / 2, Y: pb + tick + tfs*1.05 + fs*1.45, Text: fit(xl, FontSans, fs, pr-pl), Size: fs, Font: FontSans, Fill: inkText, Anchor: "middle", Role: "axis-label"},
		Op{K: "text", X: left + fs*0.95, Y: (pt + pb) / 2, Text: fit(yl, FontSans, fs, pb-pt), Size: fs, Font: FontSans, Fill: inkText, Anchor: "middle", Rotate: -90, Role: "axis-label"})
	if s.Metadata {
		fig.Desc = description(res)
	}
	return fig
}

func scaleName(log bool) string {
	if log {
		return "log"
	}
	return "linear"
}

func lastOr(l []string) string {
	if len(l) == 0 {
		return ""
	}
	return l[len(l)-1]
}

// replicateGroup is the cycle point of a replicate curve ("" for others).
func replicateGroup(se graph.Series) string {
	if se.Kind != "line" && se.Kind != "" {
		return ""
	}
	parts := strings.Split(se.Label, ":")
	if len(parts) != 4 || parts[0] != "point" {
		return ""
	}
	return strings.Join(parts[:3], ":")
}

// replicateDash is the line pattern of the n-th replicate after the first.
// Lengths follow the line width; round caps add one width to each dash.
func replicateDash(n int, lw float64) []float64 {
	patterns := [][]float64{{3, 2.5}, {0.6, 2}, {3, 2.2, 0.6, 2.2}}
	p := patterns[(n-1)%len(patterns)]
	out := make([]float64, len(p))
	for i, v := range p {
		out[i] = v * lw
	}
	return out
}

func legendSwatch(x, y, w float64, color, kind string, dash []float64, lw, fs float64) []Op {
	switch kind {
	case "individual":
		return []Op{{K: "circle", X: x + w/2, Y: y, R: fs * 0.26, Fill: color, Opacity: 0.35, Stroke: color, SW: 0.7, Role: "legend"}}
	case "mean":
		return []Op{{K: "line", X: x, Y: y, X2: x + w, Y2: y, Stroke: color, SW: lw, Role: "legend"},
			{K: "line", X: x + w/2, Y: y - fs*0.35, X2: x + w/2, Y2: y + fs*0.35, Stroke: color, SW: lw * 0.8, Role: "legend"},
			{K: "circle", X: x + w/2, Y: y, R: fs * 0.3, Fill: color, Role: "legend"}}
	}
	return []Op{{K: "line", X: x, Y: y, X2: x + w, Y2: y, Stroke: color, SW: math.Max(lw, 1.5), Dash: dash, Role: "legend"}}
}

// fit shortens text with an ellipsis so it fits a width.
func fit(s, face string, size, maxW float64) string {
	if maxW <= 0 || TextWidth(face, s, size) <= maxW {
		return s
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[:len(r)-1]
		if TextWidth(face, string(r)+"…", size) <= maxW {
			return strings.TrimRight(string(r), " ") + "…"
		}
	}
	return "…"
}

func frac(v float64, a axisInfo) float64 {
	if a.log {
		if v <= 0 {
			return 0
		}
		return (math.Log10(v) - math.Log10(a.min)) / (math.Log10(a.max) - math.Log10(a.min))
	}
	if a.max == a.min {
		return 0.5
	}
	return (v - a.min) / (a.max - a.min)
}

func buildAxis(ax graph.Axis, view *graph.Range, isX, integer bool, dec string) axisInfo {
	lo, hi := ax.Min, ax.Max
	fixed := false
	if view != nil {
		if isX && view.XMax > view.XMin {
			lo, hi, fixed = view.XMin, view.XMax, true
		}
		if !isX && view.YMax > view.YMin {
			lo, hi, fixed = view.YMin, view.YMax, true
		}
	}
	if math.IsInf(lo, 0) || math.IsInf(hi, 0) || math.IsNaN(lo) || math.IsNaN(hi) {
		lo, hi = 0, 1
	}
	a := axisInfo{log: ax.Scale == "log" && lo > 0}
	if isX && len(ax.Categories) > 0 {
		a.min, a.max = lo, hi
		for i, label := range ax.Categories {
			x := float64(i + 1)
			if x >= lo && x <= hi {
				a.ticks = append(a.ticks, x)
				a.labels = append(a.labels, label)
			}
		}
		return a
	}
	if a.log {
		if hi <= lo {
			hi = lo * 10
		}
		if !fixed {
			lo = math.Pow(10, math.Floor(math.Log10(lo)+1e-9))
			hi = math.Pow(10, math.Ceil(math.Log10(hi)-1e-9))
			if hi <= lo {
				hi = lo * 10
			}
		}
		a.min, a.max = lo, hi
		k0, k1 := int(math.Floor(math.Log10(lo)+1e-9)), int(math.Ceil(math.Log10(hi)-1e-9))
		decades := k1 - k0
		for k := k0; k <= k1; k++ {
			base := math.Pow(10, float64(k))
			for d := 1; d <= 9; d++ {
				v := base * float64(d)
				if v < lo*(1-1e-9) || v > hi*(1+1e-9) {
					continue
				}
				labeled := d == 1 || (decades <= 1 && (d == 2 || d == 5))
				if labeled {
					a.ticks = append(a.ticks, v)
					a.labels = append(a.labels, formatTick(v, logDecimals(v), dec))
				} else {
					a.minor = append(a.minor, v)
				}
			}
		}
		return a
	}
	if hi <= lo {
		pad := math.Max(math.Abs(lo)*0.1, 1)
		lo, hi = lo-pad, hi+pad
	}
	if !fixed && !isX && ax.Label != "" && !strings.HasPrefix(ax.Label, "axis.intensity") && !strings.HasPrefix(ax.Label, "axis.volume") && !strings.HasPrefix(ax.Label, "axis.number") {
		pad := (hi - lo) * 0.08
		lo, hi = lo-pad, hi+pad
		if ax.Min >= 0 && lo < 0 {
			lo = 0
		}
	}
	step := niceStep((hi - lo) / 5)
	if integer && step < 1 {
		step = 1
	}
	tlo, thi := lo, hi
	switch {
	case fixed:
	case integer:
		// Time axes keep exact cycle bounds with a little room so points
		// at the first and last time are not cut by the axes.
		pad := (hi - lo) * 0.04
		lo, hi = lo-pad, hi+pad
	default:
		lo = math.Floor(lo/step+1e-9) * step
		hi = math.Ceil(hi/step-1e-9) * step
		tlo, thi = lo, hi
	}
	if fixed {
		tlo, thi = lo, hi
	}
	a.min, a.max = lo, hi
	d := stepDecimals(step)
	for v := math.Ceil(tlo/step-1e-9) * step; v <= thi+step*1e-9; v += step {
		if math.Abs(v) < step*1e-9 {
			v = 0
		}
		a.ticks = append(a.ticks, v)
		a.labels = append(a.labels, formatTick(v, d, dec))
	}
	return a
}

func niceStep(raw float64) float64 {
	if raw <= 0 || math.IsNaN(raw) {
		return 1
	}
	p := math.Pow(10, math.Floor(math.Log10(raw)))
	f := raw / p
	switch {
	case f <= 1:
		return p
	case f <= 2:
		return 2 * p
	case f <= 2.5:
		return 2.5 * p
	case f <= 5:
		return 5 * p
	}
	return 10 * p
}

func stepDecimals(step float64) int {
	for d := 0; d <= 10; d++ {
		v := step * math.Pow(10, float64(d))
		if math.Abs(v-math.Round(v)) < 1e-6 {
			return d
		}
	}
	return 10
}

func logDecimals(v float64) int {
	if v >= 1 {
		return 0
	}
	return stepDecimals(v)
}

func formatTick(v float64, decimals int, dec string) string {
	if math.Abs(v) >= 1e6 || (v != 0 && math.Abs(v) < 1e-4) {
		s := strconv.FormatFloat(v, 'g', 3, 64)
		return strings.Replace(s, "e+0", "e", 1)
	}
	return FormatNumber(v, decimals, dec)
}

// FormatNumber formats a number with a decimal separator and a true minus.
func FormatNumber(v float64, decimals int, dec string) string {
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	if dec == "," {
		s = strings.Replace(s, ".", ",", 1)
	}
	if strings.HasPrefix(s, "-") {
		s = "−" + s[1:]
	}
	return s
}

// AxisLabel returns the localized label of an axis with its unit.
func AxisLabel(a graph.Axis, T Translator) string {
	l := T(a.Label)
	if strings.HasPrefix(a.Label, "axis.time.") || a.Unit == "" {
		return l
	}
	return fmt.Sprintf("%s (%s)", l, Normalize(a.Unit))
}

// SeriesLabel localizes generated series labels.
func SeriesLabel(label string, T Translator) string {
	switch {
	case strings.HasPrefix(label, "statistics:"):
		parts := strings.SplitN(label, ":", 3)
		keys := map[string]string{"individual": "series.stat_individual", "mean_sd": "series.stat_mean_sd", "mean_sem": "series.stat_mean_sem", "mean_ci": "series.stat_mean_ci", "mean_only": "series.stat_mean_only"}
		if len(parts) == 3 && keys[parts[1]] != "" {
			return T(keys[parts[1]]) + " · " + parts[2]
		}
	case strings.HasPrefix(label, "series."):
		return T(label)
	case strings.HasPrefix(label, "point:"):
		parts := strings.Split(label, ":") // point:<unit>:<offset>[:<replicate>]
		if len(parts) == 3 {
			return T("point."+parts[1], "n", parts[2])
		}
		if len(parts) == 4 {
			return T("point."+parts[1], "n", parts[2]) + " · R" + parts[3]
		}
	}
	return label
}

func graphTitle(res graph.Result, T Translator) string {
	if res.Title != "" {
		return res.Title
	}
	switch res.Kind {
	case graph.KindParameterTime:
		return T(res.Y.Label)
	default:
		return T("graph.kind." + res.Kind)
	}
}

var paramOrder = []string{"effective_diameter", "polydispersity", "count_rate", "average_count_rate", "baseline_index"}

func metadataLines(res graph.Result, T Translator, dec string) []string {
	var out []string
	switch res.Kind {
	case graph.KindParameterTime:
		out = append(out, T("plot.meta.stats"))
		if len(res.Gaps) > 0 {
			var g []string
			for _, x := range res.Gaps {
				g = append(g, T("point."+res.X.Unit, "n", FormatNumber(x, 0, dec)))
			}
			out = append(out, T("plot.meta.gaps", "points", strings.Join(g, ", ")))
		}
		out = append(out, T("plot.meta.sources", "n", strconv.Itoa(len(res.Provenance.Sources))))
	default:
		for _, se := range res.Series {
			if se.Hidden {
				continue
			}
			parts := []string{SeriesLabel(se.Label, T)}
			for _, k := range paramOrder {
				if raw, ok := se.Meta[k]; ok {
					v := raw
					if dec == "," {
						v = strings.Replace(v, ".", ",", 1)
					} else {
						v = strings.Replace(v, ",", ".", 1)
					}
					if u := se.Meta[k+".unit"]; u != "" {
						v += " " + Normalize(u)
					}
					parts = append(parts, T("param.short."+k)+" "+v)
				}
			}
			if t, ok := se.Meta["measuredAt"]; ok && len(t) >= 16 {
				parts = append(parts, strings.Replace(t[:16], "T", " ", 1))
			}
			out = append(out, strings.Join(parts, " · "))
		}
	}
	return out
}

func description(res graph.Result) string {
	var b strings.Builder
	b.WriteString(res.Provenance.Engine + "; " + res.Provenance.Spec)
	for _, src := range res.Provenance.Sources {
		b.WriteString("; " + src.FileName + " sha256:" + src.SHA256)
	}
	for _, t := range res.Provenance.Transformations {
		b.WriteString("; " + t)
	}
	return b.String()
}
