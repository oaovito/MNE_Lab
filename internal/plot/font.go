package plot

import (
	"embed"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Fonts bundled with the application (SIL Open Font License, see the
// *-OFL.txt files next to them). Exports never depend on fonts installed
// on the computer, so a figure looks the same everywhere.
//
//go:embed fonts/*.ttf
var fontFS embed.FS

// Font faces used by figures.
const (
	FontSans     = "sans"
	FontSansBold = "sans-bold"
	FontMono     = "mono"
)

var fontFiles = map[string]string{
	FontSans:     "fonts/Inter-Regular.ttf",
	FontSansBold: "fonts/Inter-SemiBold.ttf",
	FontMono:     "fonts/JetBrainsMono-Regular.ttf",
}

// FontBytes returns the TrueType data of a bundled face.
func FontBytes(face string) []byte {
	b, err := fontFS.ReadFile(fontFiles[face])
	if err != nil {
		panic("plot: missing bundled font " + face)
	}
	return b
}

// FontFamily is the CSS family list used where text stays editable (SVG).
const FontFamily = "Inter, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif"

type metrics struct {
	once    sync.Once
	f       *sfnt.Font
	upem    float64
	mu      sync.Mutex
	advance map[rune]float64 // in em
}

var faces = map[string]*metrics{FontSans: {}, FontSansBold: {}, FontMono: {}}

func (m *metrics) load(face string) {
	m.once.Do(func() {
		f, err := sfnt.Parse(FontBytes(face))
		if err != nil {
			panic("plot: invalid bundled font " + face)
		}
		m.f, m.upem, m.advance = f, float64(f.UnitsPerEm()), map[rune]float64{}
	})
}

// Normalize maps characters the bundled fonts lack to equivalent glyphs
// (Greek mu to the micro sign) so text never renders as missing boxes.
func Normalize(s string) string {
	if !strings.ContainsAny(s, "μ  ") {
		return s
	}
	return strings.NewReplacer("μ", "µ", " ", " ", " ", " ").Replace(s)
}

// TextWidth measures text in points at the given size.
func TextWidth(face, s string, size float64) float64 {
	m := faces[face]
	if m == nil {
		m = faces[FontSans]
		face = FontSans
	}
	m.load(face)
	m.mu.Lock()
	defer m.mu.Unlock()
	var buf sfnt.Buffer
	w := 0.0
	s = Normalize(s)
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		s = s[n:]
		a, ok := m.advance[r]
		if !ok {
			idx, err := m.f.GlyphIndex(&buf, r)
			if err != nil || idx == 0 {
				idx, _ = m.f.GlyphIndex(&buf, '?')
			}
			adv, err := m.f.GlyphAdvance(&buf, idx, fixed.I(int(m.upem)), font.HintingNone)
			if err == nil {
				a = float64(adv) / 64 / m.upem
			}
			m.advance[r] = a
		}
		w += a * size
	}
	return w
}
