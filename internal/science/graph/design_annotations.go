package graph

// Statistical annotations of design graphs (condition × time and paired
// views, combined or not). Every bracket, star, adjusted p-value and
// estimated difference comes from a comparison stored in a saved analysis
// of the same design and variable; nothing is recalculated to draw it.

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/design"
)

// ErrAnalysisIncompatible refuses linking a graph to a saved analysis of
// another design or another variable.
var ErrAnalysisIncompatible = errors.New("graph.analysis_incompatible")

// MaxDesignAnnotations bounds the brackets of one design graph.
const MaxDesignAnnotations = 12

// Annotation styles: exact adjusted p, stars from the adjusted p, or the
// estimated difference with its stored interval and adjusted p.
const (
	AnnotationExact  = "exact"
	AnnotationStars  = "stars"
	AnnotationEffect = "effect"
)

// CompatibleAnalysis reports whether a saved analysis can supply intervals
// and annotations to a design graph: same variable and the same design
// (any revision of it).
func CompatibleAnalysis(def Definition, saved analysis.StatisticalAnalysis, d design.Design) error {
	ed := saved.Snapshot.ExperimentalDesign
	if saved.Snapshot.Definition.Variable != def.Param || ed == nil || ed.Root != d.Root {
		return ErrAnalysisIncompatible
	}
	return nil
}

// ValidateDesignAnnotations checks the annotations of a design graph
// against its linked saved analysis.
func ValidateDesignAnnotations(def Definition, saved analysis.StatisticalAnalysis, d design.Design) error {
	if len(def.Annotations) > MaxDesignAnnotations || (def.AnnotationStyle != "" && def.AnnotationStyle != AnnotationExact && def.AnnotationStyle != AnnotationStars && def.AnnotationStyle != AnnotationEffect) {
		return analysis.ErrDefinition
	}
	if err := CompatibleAnalysis(def, saved, d); err != nil {
		return err
	}
	conds := def.Conditions
	if len(conds) == 0 {
		for _, c := range d.Conditions {
			conds = append(conds, c.Label)
		}
	}
	times := map[string]bool{}
	for _, tl := range d.Times {
		times[tl.Label] = true
	}
	at := PairedConditionAt(def, d)
	known := map[string]analysis.Comparison{}
	for _, c := range saved.Results.Comparisons {
		known[c.ID] = c
	}
	seen := map[string]bool{}
	for _, id := range def.Annotations {
		c, ok := known[id]
		if id == "" || !ok || seen[id] || !contains(conds, c.LeftA) || !contains(conds, c.RightA) || !times[c.LeftB] || !times[c.RightB] || (c.LeftA == c.RightA && c.LeftB == c.RightB) {
			return analysis.ErrDefinition
		}
		if def.Kind == KindPairedCondition && (c.LeftB != at || c.RightB != at || c.LeftA == c.RightA) {
			return analysis.ErrDefinition
		}
		seen[id] = true
	}
	return nil
}

func g4(v float64) string { return strconv.FormatFloat(v, 'g', 4, 64) }

// effectLabel is the text of an effect bracket: the stored difference, its
// interval and the adjusted p, in the unit the axis shows.
func effectLabel(diff float64, lower, upper *float64, p float64, unit string) string {
	if p == 0 {
		// The precision note replaces the whole label.
		return "statistics.below_precision"
	}
	s := "Δ " + g4(diff)
	if unit != "" {
		s += " " + unit
	}
	if lower != nil && upper != nil {
		s += " [" + g4(*lower) + ", " + g4(*upper) + "]"
	}
	return s + " · p(adj)=" + strconv.FormatFloat(p, 'g', 6, 64)
}

// annotationLabel is the text of a bracket, from stored values only.
func annotationLabel(style string, c analysis.Comparison, unit string) string {
	p := "p(adj)=" + strconv.FormatFloat(c.AdjustedP, 'g', 6, 64)
	if c.AdjustedP == 0 {
		p = "statistics.below_precision"
	}
	switch style {
	case AnnotationStars:
		label := "ns"
		for _, threshold := range []float64{.05, .01, .001, .0001} {
			if c.AdjustedP < threshold {
				if label == "ns" {
					label = ""
				}
				label += "*"
			}
		}
		return label
	case AnnotationEffect:
		return effectLabel(c.Difference, c.Lower, c.Upper, c.AdjustedP, unit)
	}
	return p
}

// designAnnotations places the brackets of the stored comparisons above the
// values they compare. xOf gives the drawn position of a condition × time.
func designAnnotations(def Definition, din DesignInput, xOf func(cond, tm string) (float64, bool), res *Result) error {
	a := din.Analysis
	if a == nil {
		if len(def.Annotations) > 0 {
			res.Warnings = append(res.Warnings, "graph.annotations_need_analysis")
		}
		return nil
	}
	if a.SourceChanged {
		res.Warnings = append(res.Warnings, "statistics.source_changed_saved_graph")
	}
	res.Provenance.Transformations = append(res.Provenance.Transformations, "linked saved analysis "+a.ID+" ("+a.Results.Engine+"); values shown from it are never recalculated")
	if len(def.Annotations) == 0 {
		return nil
	}
	if res.Y.Conversion != nil && strings.Contains(res.Y.Conversion.Factor, ":") {
		// Conditions in different stored units: the analysis could not
		// compare them, so no bracket is drawn.
		res.Warnings = append(res.Warnings, "graph.annotations_mixed_units")
		return nil
	}
	if err := ValidateDesignAnnotations(def, *a, din.Design); err != nil {
		return err
	}
	comparisons := map[string]analysis.Comparison{}
	for _, c := range a.Results.Comparisons {
		comparisons[c.ID] = c
	}
	span := res.Y.Max - res.Y.Min
	if span <= 0 || math.IsInf(span, 0) || math.IsNaN(span) {
		span = math.Max(1, math.Abs(res.Y.Max)*.1)
	}
	gap := span * .1
	type placed struct{ x1, x2, y float64 }
	var done []placed
	top := res.Y.Max
	style := def.AnnotationStyle
	if style == "" {
		style = AnnotationExact
	}
	res.AnnotationStyle = style
	for _, id := range def.Annotations {
		c := comparisons[id]
		x1, ok1 := xOf(c.LeftA, c.LeftB)
		x2, ok2 := xOf(c.RightA, c.RightB)
		if !ok1 || !ok2 {
			res.Warnings = append(res.Warnings, "graph.annotation_not_drawn:"+c.Contrast)
			continue
		}
		if x1 > x2 {
			x1, x2 = x2, x1
		}
		// The bracket sits above the values and error bars it spans.
		w := math.Max((x2-x1)*.25, (res.X.Max-res.X.Min)*.02)
		local := math.Inf(-1)
		for _, s := range res.Series {
			for i := range s.X {
				if s.X[i] < x1-w || s.X[i] > x2+w {
					continue
				}
				v := s.Y[i]
				if i < len(s.Err) {
					v += s.Err[i]
				}
				local = math.Max(local, v)
			}
		}
		if math.IsInf(local, -1) {
			local = res.Y.Max
		}
		y := local + gap
		for moved := true; moved; {
			moved = false
			for _, p := range done {
				if x1 <= p.x2+w && x2 >= p.x1-w && math.Abs(y-p.y) < gap {
					y, moved = p.y+gap, true
				}
			}
		}
		done = append(done, placed{x1, x2, y})
		top = math.Max(top, y)
		an := StatisticalAnnotation{ComparisonID: id, X1: x1, X2: x2, Y: y, Label: annotationLabel(style, c, res.Y.Unit), AdjustedP: c.AdjustedP, Correction: c.Correction,
			AnalysisID: a.ID, Contrast: c.Contrast, Context: c.Context, Family: c.Family}
		if style == AnnotationEffect {
			d := c.Difference
			an.Difference, an.Lower, an.Upper = &d, c.Lower, c.Upper
		}
		res.Annotations = append(res.Annotations, an)
		res.Provenance.Transformations = append(res.Provenance.Transformations,
			fmt.Sprintf("annotation %s: %s %s; difference %s; p(adj)=%s; %s; from saved analysis %s", id, c.Contrast, c.Context, number(c.Difference), number(c.AdjustedP), c.Correction, a.ID))
	}
	if len(res.Annotations) > 0 {
		res.Y.Max = top + gap*.7
	}
	if style == AnnotationStars {
		res.Provenance.Transformations = append(res.Provenance.Transformations, "stars use adjusted p: * <0.05; ** <0.01; *** <0.001; **** <0.0001; ns >=0.05; display thresholds do not change analysis alpha")
	}
	return nil
}
