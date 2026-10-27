package units

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/model"
)

// The system list is built from the parser, never invented: every entry
// must be a spelling the parser's unit pattern or value-suffix list
// recognizes, and every alternative of that pattern must be represented.
func TestSystemUnitsAreParserSpellings(t *testing.T) {
	src, err := os.ReadFile("../lightscattering/parser.go")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile("unitPattern = regexp.MustCompile\\(`\\(\\?i\\)\\^\\(([^`]*)\\)\\$`\\)").FindSubmatch(src)
	if m == nil {
		t.Fatal("parser unit pattern not found")
	}
	pattern := regexp.MustCompile(`(?i)^(` + string(m[1]) + `)$`)
	suffixes := regexp.MustCompile(`for _, u := range \[\]string\{([^}]*)\}`).FindSubmatch(src)
	if suffixes == nil {
		t.Fatal("parser suffix list not found")
	}
	suffix := map[string]bool{}
	for _, s := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(suffixes[1], -1) {
		suffix[string(s[1])] = true
	}
	seen := map[string]bool{}
	ids := map[string]bool{}
	for _, u := range System() {
		if !pattern.MatchString(u.Symbol) && !suffix[u.Symbol] {
			t.Errorf("system unit %q is not a parser spelling", u.Symbol)
		}
		if seen[u.Symbol] || ids[u.ID] || !strings.HasPrefix(u.ID, "sys.") || !u.System || !ValidDimension(u.Dimension) {
			t.Errorf("bad system entry %+v", u)
		}
		seen[u.Symbol], ids[u.ID] = true, true
	}
	for s := range suffix {
		if !seen[s] {
			t.Errorf("parser suffix %q missing from the system units", s)
		}
	}
	for _, alt := range strings.Split(string(m[1]), "|") {
		alt = strings.ReplaceAll(alt, `\.`, ".")
		found := false
		for s := range seen {
			if strings.EqualFold(s, alt) {
				found = true
			}
		}
		if !found {
			t.Errorf("parser spelling %q missing from the system units", alt)
		}
	}
}

func TestSpellingVariantsStayDistinct(t *testing.T) {
	for _, pair := range [][2]string{{"um", "µm"}, {"μm", "µm"}, {"um", "nm"}, {"μm", "nm"}, {"Kcps", "kcps"}, {"mcps", "Mcps"}, {"mcps", "cps"}, {"C", "°C"}} {
		if _, err := Ratio(pair[0], pair[1]); err == nil {
			t.Errorf("%s converted to %s", pair[0], pair[1])
		}
	}
	a, _ := SystemBySymbol("µm")
	b, _ := SystemBySymbol("μm")
	c, _ := SystemBySymbol("um")
	if a.ID == b.ID || a.ID == c.ID || b.ID == c.ID {
		t.Fatal("variants merged")
	}
}

func TestExactConversionsOnlyForD6Pairs(t *testing.T) {
	cases := []struct {
		from, to string
		in, want float64
		factor   string
	}{
		{"nm", "µm", 239.8, 0.2398, "1/1000"},
		{"µm", "nm", 0.2, 200, "1000"},
		{"nm", "m", 250, 2.5e-7, "1/1000000000"},
		{"kcps", "cps", 305.2, 305200, "1000"},
		{"cps", "Mcps", 1234567, 1.234567, "1/1000000"},
		{"Mcps", "kcps", 0.1845, 184.5, "1000"},
		{"s", "min", 90, 1.5, "1/60"},
		{"h", "d", 36, 1.5, "1/24"},
		{"d", "s", 2, 172800, "86400"},
		{"nm", "nm", 51.3, 51.3, "1"},
	}
	for _, c := range cases {
		got, err := Display(c.in, "", c.from, c.to)
		if err != nil || got.DisplayValue != c.want || got.Factor != c.factor || got.OriginalValue != c.in || got.OriginalUnit != c.from || got.DisplayUnit != c.to || got.Converted != (c.from != c.to) {
			t.Errorf("%v %s→%s: %+v %v", c.in, c.from, c.to, got, err)
		}
	}
	for _, pair := range [][2]string{{"nm", "mm"}, {"ms", "s"}, {"°C", "K"}, {"mV", "mV2"}, {"nm", "Å"}} {
		if _, err := Ratio(pair[0], pair[1]); !errors.Is(err, ErrNoExactFactor) {
			t.Errorf("%s→%s: %v", pair[0], pair[1], err)
		}
	}
}

func TestIncompatibleDimensionsRefused(t *testing.T) {
	for _, pair := range [][2]string{{"nm", "kcps"}, {"kcps", "s"}, {"s", "nm"}, {"mm", "cps"}, {"°C", "nm"}, {"%", "nm"}, {"min", "µm"}} {
		if _, err := Display(1, "1", pair[0], pair[1]); !errors.Is(err, ErrIncompatible) {
			t.Errorf("%s→%s: %v", pair[0], pair[1], err)
		}
	}
	if _, err := Display(1, "1", "", "nm"); !errors.Is(err, ErrNotStated) {
		t.Error("missing unit converted", err)
	}
}

// The display conversion is pure: the quantity, its raw text and its
// serialized bytes are identical after any number of conversions.
func TestRawBytesUnchangedAfterConversion(t *testing.T) {
	q := model.Quantity{Value: 239.8, Raw: "239.80", Unit: "nm", Label: "Effective Diameter (nm)", Line: 3}
	before, _ := json.Marshal(q)
	c, err := DisplayQuantity(q, "µm")
	if err != nil {
		t.Fatal(err)
	}
	if c.OriginalRaw != "239.80" || c.OriginalValue != 239.8 || c.OriginalUnit != "nm" || c.DisplayUnit != "µm" || c.DisplayValue != 0.2398 || c.DisplayDecimals != 5 {
		t.Fatalf("conversion %+v", c)
	}
	if _, err := DisplayQuantity(q, "kcps"); err == nil {
		t.Fatal("incompatible conversion accepted")
	}
	after, _ := json.Marshal(q)
	if !bytes.Equal(before, after) {
		t.Fatal("quantity changed by display conversion")
	}
	c, _ = Display(305.2, "305.2", "kcps", "cps")
	if c.DisplayDecimals != 0 {
		t.Fatal("decimals", c)
	}
	c, _ = Display(90, "90", "s", "min")
	if c.DisplayDecimals != -1 {
		t.Fatal("decimals invented for a non-decimal factor", c)
	}
}

func TestFieldDimensions(t *testing.T) {
	d, ok := FieldDimension(model.EffectiveDiameter)
	if !ok || d != Length {
		t.Fatal("diameter")
	}
	for _, u := range System() {
		if u.Dimension == Length && u.Symbol == "kcps" || (d == u.Dimension && strings.Contains(u.Symbol, "cps")) {
			t.Fatal("count rate offered for diameter", u)
		}
	}
	for _, k := range []string{model.CountRate, model.AverageCountRate} {
		if d, _ := FieldDimension(k); d != CountRate {
			t.Fatal(k)
		}
	}
	for _, k := range []string{model.Polydispersity, model.BaselineIndex} {
		if d, _ := FieldDimension(k); d != Dimensionless {
			t.Fatal(k)
		}
	}
}

func TestValidation(t *testing.T) {
	for _, s := range []string{"", " nm", "nm ", "a\nb", strings.Repeat("x", 65), "\xff"} {
		if ValidSymbol(s) {
			t.Errorf("%q accepted", s)
		}
	}
	for _, s := range []string{"nm", "mPa·s", "counts/s", strings.Repeat("x", 64)} {
		if !ValidSymbol(s) {
			t.Errorf("%q refused", s)
		}
	}
	if !ValidName("") || ValidName(strings.Repeat("n", 121)) || !ValidDimension(Length) || ValidDimension("mass") {
		t.Fatal("name/dimension validation")
	}
}
