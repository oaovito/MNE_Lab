package plot

import (
	"strings"
	"testing"
)

func countRole(f Figure, role string) (n int, texts []string) {
	for _, op := range f.Ops {
		if op.Role == role {
			n++
			texts = append(texts, op.Text)
		}
	}
	return
}

// Error bars are never left without a description: with the legend and the
// metadata off, a note names them.
func TestErrorBarsAlwaysNamed(t *testing.T) {
	s := Preset(PresetScreen)
	s.Legend, s.Metadata = false, false
	r := parameterTime()
	f := Layout(r, s, tr)
	if n, texts := countRole(f, "error-note"); n != 1 || texts[0] != "plot.error_bars.sd" {
		t.Fatalf("%d %v", n, texts)
	}
	r = parameterTime()
	r.ErrorBars = "sem"
	if _, texts := countRole(Layout(r, s, tr), "error-note"); len(texts) != 1 || texts[0] != "plot.error_bars.sem" {
		t.Fatalf("%v", texts)
	}
	s.Legend = true
	if n, _ := countRole(Layout(parameterTime(), s, tr), "error-note"); n != 0 {
		t.Fatal("the legend already names them")
	}
}

func TestFocusSubtitleNotesAndMeanLine(t *testing.T) {
	s := Preset(PresetScreen)
	r := distribution()
	r.Subtitle = "Synthetic subtitle"
	r.Series[1].Muted = true
	r.X.Title = "My diameter"
	f := Layout(r, s, tr)
	if n, texts := countRole(f, "subtitle"); n != 1 || texts[0] != "Synthetic subtitle" {
		t.Fatalf("subtitle %v", texts)
	}
	_, legend := countRole(f, "legend")
	if strings.Contains(strings.Join(legend, "|"), r.Series[1].Label) {
		t.Fatal("a muted series is listed in the legend")
	}
	_, labels := countRole(f, "axis-label")
	if !strings.HasPrefix(labels[0], "My diameter (nm)") {
		t.Fatalf("axis title %v", labels)
	}
	p := parameterTime()
	lines := func(f Figure) int {
		n := 0
		for _, op := range f.Ops {
			if op.K == "poly" && op.Role == "series" {
				n++
			}
		}
		return n
	}
	with := lines(Layout(p, s, tr))
	for i := range p.Series {
		if p.Series[i].Kind == "mean" {
			p.Series[i].NoLine = true
		}
	}
	if without := lines(Layout(p, s, tr)); without >= with {
		t.Fatalf("the mean line is still drawn: %d → %d", with, without)
	}
}
