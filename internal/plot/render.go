package plot

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fogleman/gg"
	"github.com/go-pdf/fpdf"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

// SVG renders a figure as a standalone SVG document. Physical size is kept
// in the unit the user chose so vector editors import it at the right size.
func SVG(f Figure, spec Spec) []byte {
	var b bytes.Buffer
	w, h := svgSize(f, spec)
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`+"\n")
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s" font-family="%s">`+"\n", w, h, num(f.Width), num(f.Height), esc(FontFamily))
	fmt.Fprintf(&b, "<title>%s</title>\n", esc(f.Title))
	if f.Desc != "" {
		fmt.Fprintf(&b, "<desc>%s</desc>\n", esc(f.Desc))
	}
	clip := 0
	open := false
	for _, op := range f.Ops {
		switch op.K {
		case "rect":
			fmt.Fprintf(&b, `<rect x="%s" y="%s" width="%s" height="%s"%s/>`+"\n", num(op.X), num(op.Y), num(op.W), num(op.H), paint(op))
		case "line":
			fmt.Fprintf(&b, `<line x1="%s" y1="%s" x2="%s" y2="%s"%s/>`+"\n", num(op.X), num(op.Y), num(op.X2), num(op.Y2), paint(op))
		case "poly":
			var p strings.Builder
			for i := 0; i+1 < len(op.Pts); i += 2 {
				if i > 0 {
					p.WriteByte(' ')
				}
				p.WriteString(num(op.Pts[i]) + "," + num(op.Pts[i+1]))
			}
			dash := ""
			if len(op.Dash) > 0 {
				var d []string
				for _, v := range op.Dash {
					d = append(d, num(v))
				}
				dash = ` stroke-dasharray="` + strings.Join(d, " ") + `"`
			}
			fmt.Fprintf(&b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="%s"%s stroke-linejoin="round" stroke-linecap="round"/>`+"\n", p.String(), op.Stroke, num(op.SW), dash)
		case "circle":
			fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="%s"%s/>`+"\n", num(op.X), num(op.Y), num(op.R), paint(op))
		case "text":
			weight := ""
			if op.Font == FontSansBold {
				weight = ` font-weight="600"`
			}
			rot := ""
			if op.Rotate != 0 {
				rot = fmt.Sprintf(` transform="rotate(%s %s %s)"`, num(op.Rotate), num(op.X), num(op.Y))
			}
			anchor := ""
			if op.Anchor == "middle" || op.Anchor == "end" {
				anchor = ` text-anchor="` + op.Anchor + `"`
			}
			fmt.Fprintf(&b, `<text x="%s" y="%s" font-size="%s"%s%s fill="%s"%s>%s</text>`+"\n", num(op.X), num(op.Y), num(op.Size), weight, anchor, op.Fill, rot, esc(Normalize(op.Text)))
		case "clip":
			clip++
			fmt.Fprintf(&b, `<clipPath id="plot-clip-%d"><rect x="%s" y="%s" width="%s" height="%s"/></clipPath><g clip-path="url(#plot-clip-%d)">`+"\n", clip, num(op.X), num(op.Y), num(op.W), num(op.H), clip)
			open = true
		case "unclip":
			if open {
				b.WriteString("</g>\n")
				open = false
			}
		}
	}
	if open {
		b.WriteString("</g>\n")
	}
	b.WriteString("</svg>\n")
	return b.Bytes()
}

func svgSize(f Figure, s Spec) (string, string) {
	switch s.Unit {
	case UnitMM, UnitCM, UnitIn:
		return num(s.Width) + s.Unit, num(s.Height) + s.Unit
	}
	pw, ph := s.Pixels()
	return strconv.Itoa(pw), strconv.Itoa(ph)
}

func paint(op Op) string {
	var b strings.Builder
	if op.Fill != "" {
		b.WriteString(` fill="` + op.Fill + `"`)
		if op.Opacity > 0 && op.Opacity < 1 {
			b.WriteString(` fill-opacity="` + num(op.Opacity) + `"`)
		}
	} else {
		b.WriteString(` fill="none"`)
	}
	if op.Stroke != "" {
		b.WriteString(` stroke="` + op.Stroke + `" stroke-width="` + num(op.SW) + `"`)
		if len(op.Dash) > 0 {
			var d []string
			for _, v := range op.Dash {
				d = append(d, num(v))
			}
			b.WriteString(` stroke-dasharray="` + strings.Join(d, " ") + `" stroke-linecap="round"`)
		}
	}
	return b.String()
}

func num(v float64) string {
	s := strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
	if s == "-0" {
		return "0"
	}
	return s
}

func esc(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '&':
			b.WriteString("&amp;")
		case '"':
			b.WriteString("&quot;")
		default:
			if r < 0x20 && r != '\t' && r != '\n' {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ParseColor reads #RRGGBB.
func ParseColor(hex string) color.NRGBA {
	c := color.NRGBA{A: 255}
	if len(hex) == 7 && hex[0] == '#' {
		v, err := strconv.ParseUint(hex[1:], 16, 32)
		if err == nil {
			c.R, c.G, c.B = uint8(v>>16), uint8(v>>8), uint8(v)
		}
	}
	return c
}

type faceKey struct {
	face string
	px   float64
}

var (
	ttMu    sync.Mutex
	ttFonts = map[string]*opentype.Font{}
	ttFaces = map[faceKey]font.Face{}
)

func rasterFace(face string, px float64) font.Face {
	ttMu.Lock()
	defer ttMu.Unlock()
	px = math.Round(px*4) / 4
	k := faceKey{face, px}
	if f, ok := ttFaces[k]; ok {
		return f
	}
	tf, ok := ttFonts[face]
	if !ok {
		var err error
		tf, err = opentype.Parse(FontBytes(face))
		if err != nil {
			panic("plot: invalid bundled font " + face)
		}
		ttFonts[face] = tf
	}
	f, err := opentype.NewFace(tf, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		panic("plot: font face " + face)
	}
	if len(ttFaces) > 64 {
		ttFaces = map[faceKey]font.Face{}
	}
	ttFaces[k] = f
	return f
}

// Raster renders a figure to pixels at the spec's size and resolution.
// Fonts are rasterized at the target resolution, never scaled bitmaps.
func Raster(f Figure, spec Spec) (*image.RGBA, error) {
	if err := spec.Validate(true); err != nil {
		return nil, err
	}
	pw, ph := spec.Pixels()
	k := float64(pw) / f.Width
	dc := gg.NewContext(pw, ph)
	dc.SetLineCapRound()
	dc.SetLineJoinRound()
	set := func(hex string, alpha float64) {
		c := ParseColor(hex)
		if alpha > 0 && alpha < 1 {
			c.A = uint8(math.Round(alpha * 255))
		}
		dc.SetColor(c)
	}
	dash := func(d []float64) {
		if len(d) == 0 {
			dc.SetDash()
			return
		}
		s := make([]float64, len(d))
		for i, v := range d {
			s[i] = v * k
		}
		dc.SetDash(s...)
	}
	stroke := func(op Op) {
		set(op.Stroke, 0)
		dc.SetLineWidth(math.Max(op.SW*k, 0.5))
		dash(op.Dash)
		dc.Stroke()
		dc.SetDash()
	}
	for _, op := range f.Ops {
		switch op.K {
		case "rect":
			dc.DrawRectangle(op.X*k, op.Y*k, op.W*k, op.H*k)
			if op.Fill != "" {
				set(op.Fill, op.Opacity)
				if op.Stroke != "" {
					dc.FillPreserve()
				} else {
					dc.Fill()
				}
			}
			if op.Stroke != "" {
				stroke(op)
			}
			dc.ClearPath()
		case "line":
			dc.DrawLine(op.X*k, op.Y*k, op.X2*k, op.Y2*k)
			stroke(op)
		case "poly":
			for i := 0; i+1 < len(op.Pts); i += 2 {
				if i == 0 {
					dc.MoveTo(op.Pts[i]*k, op.Pts[i+1]*k)
				} else {
					dc.LineTo(op.Pts[i]*k, op.Pts[i+1]*k)
				}
			}
			stroke(op)
		case "circle":
			dc.DrawCircle(op.X*k, op.Y*k, op.R*k)
			if op.Fill != "" {
				set(op.Fill, op.Opacity)
				if op.Stroke != "" {
					dc.FillPreserve()
				} else {
					dc.Fill()
				}
			}
			if op.Stroke != "" {
				stroke(op)
			}
			dc.ClearPath()
		case "text":
			face := op.Font
			if face == "" {
				face = FontSans
			}
			dc.SetFontFace(rasterFace(face, op.Size*k))
			set(op.Fill, 0)
			ax := 0.0
			switch op.Anchor {
			case "middle":
				ax = 0.5
			case "end":
				ax = 1
			}
			s := Normalize(op.Text)
			if op.Rotate != 0 {
				dc.Push()
				dc.RotateAbout(gg.Radians(op.Rotate), op.X*k, op.Y*k)
				dc.DrawStringAnchored(s, op.X*k, op.Y*k, ax, 0)
				dc.Pop()
			} else {
				dc.DrawStringAnchored(s, op.X*k, op.Y*k, ax, 0)
			}
		case "clip":
			dc.DrawRectangle(op.X*k, op.Y*k, op.W*k, op.H*k)
			dc.Clip()
		case "unclip":
			dc.ResetClip()
		}
	}
	im, ok := dc.Image().(*image.RGBA)
	if !ok {
		return nil, fmt.Errorf("plot: unexpected image type")
	}
	return im, nil
}

// PDF renders a figure as a vector PDF with the bundled fonts embedded.
func PDF(f Figure, spec Spec, created time.Time) ([]byte, error) {
	pdf := fpdf.NewCustom(&fpdf.InitType{UnitStr: "pt", Size: fpdf.SizeType{Wd: f.Width, Ht: f.Height}})
	pdf.SetMargins(0, 0, 0)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddUTF8FontFromBytes("inter", "", FontBytes(FontSans))
	pdf.AddUTF8FontFromBytes("inter", "B", FontBytes(FontSansBold))
	pdf.AddUTF8FontFromBytes("mono", "", FontBytes(FontMono))
	pdf.SetTitle(f.Title, true)
	pdf.SetCreator("MNE Lab", true)
	if f.Desc != "" {
		pdf.SetSubject(f.Desc, true)
	}
	if !created.IsZero() {
		pdf.SetCreationDate(created)
		pdf.SetModificationDate(created)
	}
	pdf.AddPageFormat("P", fpdf.SizeType{Wd: f.Width, Ht: f.Height})
	pdf.SetLineCapStyle("round")
	pdf.SetLineJoinStyle("round")
	rgb := func(hex string) (int, int, int) {
		c := ParseColor(hex)
		return int(c.R), int(c.G), int(c.B)
	}
	setStroke := func(op Op) {
		pdf.SetDrawColor(rgb(op.Stroke))
		pdf.SetLineWidth(op.SW)
		if len(op.Dash) > 0 {
			pdf.SetDashPattern(op.Dash, 0)
		} else {
			pdf.SetDashPattern([]float64{}, 0)
		}
	}
	fill := func(op Op, draw func(style string)) {
		if op.Fill != "" {
			pdf.SetFillColor(rgb(op.Fill))
			if op.Opacity > 0 && op.Opacity < 1 {
				pdf.SetAlpha(op.Opacity, "Normal")
				draw("F")
				pdf.SetAlpha(1, "Normal")
			} else {
				draw("F")
			}
		}
		if op.Stroke != "" {
			setStroke(op)
			draw("D")
		}
	}
	for _, op := range f.Ops {
		switch op.K {
		case "rect":
			fill(op, func(st string) { pdf.Rect(op.X, op.Y, op.W, op.H, st) })
		case "line":
			setStroke(op)
			pdf.Line(op.X, op.Y, op.X2, op.Y2)
		case "poly":
			setStroke(op)
			for i := 0; i+1 < len(op.Pts); i += 2 {
				if i == 0 {
					pdf.MoveTo(op.Pts[i], op.Pts[i+1])
				} else {
					pdf.LineTo(op.Pts[i], op.Pts[i+1])
				}
			}
			pdf.DrawPath("D")
		case "circle":
			fill(op, func(st string) { pdf.Circle(op.X, op.Y, op.R, st) })
		case "text":
			family, style := "inter", ""
			switch op.Font {
			case FontSansBold:
				style = "B"
			case FontMono:
				family = "mono"
			}
			pdf.SetFont(family, style, op.Size)
			pdf.SetTextColor(rgb(op.Fill))
			s := Normalize(op.Text)
			w := pdf.GetStringWidth(s)
			dx := 0.0
			switch op.Anchor {
			case "middle":
				dx = -w / 2
			case "end":
				dx = -w
			}
			if op.Rotate != 0 {
				pdf.TransformBegin()
				pdf.TransformRotate(-op.Rotate, op.X, op.Y)
				pdf.Text(op.X+dx, op.Y, s)
				pdf.TransformEnd()
			} else {
				pdf.Text(op.X+dx, op.Y, s)
			}
		case "clip":
			pdf.ClipRect(op.X, op.Y, op.W, op.H, false)
		case "unclip":
			pdf.ClipEnd()
		}
	}
	var b bytes.Buffer
	if err := pdf.Output(&b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
