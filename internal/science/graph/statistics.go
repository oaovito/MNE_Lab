package graph

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/model"
)

func number(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

// StatisticalAnnotation binds a visible bracket to a stored adjusted test.
// Label is neutral; the renderer localizes only the precision warning.
type StatisticalAnnotation struct {
	ComparisonID string  `json:"comparisonId"`
	X1           float64 `json:"x1"`
	X2           float64 `json:"x2"`
	Y            float64 `json:"y"`
	Label        string  `json:"label"`
	AdjustedP    float64 `json:"adjustedP"`
	Correction   string  `json:"correction"`
}

func ValidateAnnotations(def Definition, saved analysis.StatisticalAnalysis) error {
	if len(def.Annotations) > 8 || (def.AnnotationStyle != "" && def.AnnotationStyle != "exact" && def.AnnotationStyle != "stars") {
		return analysis.ErrDefinition
	}
	known := map[string]analysis.Comparison{}
	groups := map[string]bool{}
	for _, g := range saved.Results.Groups {
		groups[g.FactorA+"\x00"+g.FactorB] = true
	}
	for _, c := range saved.Results.Comparisons {
		known[c.ID] = c
	}
	seen := map[string]bool{}
	for _, id := range def.Annotations {
		c, ok := known[id]
		if id == "" || !ok || seen[id] || !groups[c.LeftA+"\x00"+c.LeftB] || !groups[c.RightA+"\x00"+c.RightB] || c.LeftA == c.RightA && c.LeftB == c.RightB {
			return analysis.ErrDefinition
		}
		seen[id] = true
	}
	return nil
}

// StatisticalGroups renders the immutable saved analysis, not newly computed
// source values. All raw observations remain visible beside mean/error bars.
func StatisticalGroups(def Definition, saved analysis.StatisticalAnalysis) (Result, error) {
	if len(saved.Results.Groups) == 0 || len(saved.Results.Groups) > 30 {
		return Result{}, ErrNoData
	}
	if def.ErrorBars != "sd" && def.ErrorBars != "sem" && def.ErrorBars != "ci" {
		return Result{}, analysis.ErrDefinition
	}
	if err := ValidateAnnotations(def, saved); err != nil {
		return Result{}, err
	}
	s := saved.Snapshot
	r := Result{Kind: KindStatistical, Title: def.Title, Visual: def.Visual,
		X: Axis{Label: "axis.statistical_group", Scale: "linear", Min: .5, Max: float64(len(saved.Results.Groups)) + .5},
		Y: Axis{Label: "param." + s.Definition.Variable, Unit: s.Unit, Scale: "linear", Min: math.Inf(1), Max: math.Inf(-1)},
		Provenance: Provenance{Engine: saved.Results.Engine, Spec: analysis.CalculationVersion, ComputedAt: saved.Created,
			Statistics: "MNE Lab derived; " + def.ErrorBars + "; alpha=" + number(s.Definition.Alpha), Transformations: []string{"immutable analysis " + saved.ID, "source snapshot " + s.SourceHash}, Rules: []string{"statistics.explicit_design", "statistics.missing_not_zero"}}}
	if saved.SourceChanged {
		r.Warnings = append(r.Warnings, "statistics.source_changed_saved_graph")
	}
	for _, ci := range saved.Results.EffectIntervals {
		r.Provenance.Transformations = append(r.Provenance.Transformations, "effect interval "+ci.Effect+"; "+ci.Method+"; nominal confidence "+number(ci.ConfidenceLevel)+"; "+ci.Status)
	}
	if m := saved.Results.Model; m != nil {
		r.Provenance.Transformations = append(r.Provenance.Transformations,
			"raw group summaries; not fitted mixed-model means",
			"mixed model "+m.Estimation+"; y~"+m.Fixed+"; "+m.Random+"; "+m.ResidualCovariance+"; "+m.Test,
			"random variance "+number(m.RandomVariance)+"; residual variance "+number(m.ResidualVariance)+"; log likelihood "+number(m.LogLikelihood)+"; relative SD boundary tolerance "+number(m.BoundaryTolerance))
	}
	if m := saved.Results.Model; m != nil && m.CoefficientConfidenceLevel > 0 {
		r.Provenance.Transformations = append(r.Provenance.Transformations, "fixed coefficient intervals "+m.CoefficientIntervalMethod+"; nominal level "+number(m.CoefficientConfidenceLevel))
	}
	if s.Definition.Control != "" {
		r.Provenance.Transformations = append(r.Provenance.Transformations, "explicit control "+s.Definition.Control)
		for _, diagnostic := range saved.Results.Diagnostics {
			if strings.HasPrefix(diagnostic.Code, "dunnett_") && diagnostic.Statistic != nil {
				r.Provenance.Transformations = append(r.Provenance.Transformations, diagnostic.Code+" "+number(*diagnostic.Statistic)+"; "+diagnostic.Details)
			}
		}
	}
	for i, g := range saved.Results.Groups {
		x := float64(i + 1)
		label := strings.TrimSpace(g.FactorA + " · " + g.FactorB)
		label = strings.TrimSuffix(label, " ·")
		r.X.Categories = append(r.X.Categories, label)
		raw := Series{ID: "raw-" + number(x), Label: "statistics:individual:" + label, Kind: "individual"}
		for j, v := range g.Values {
			// Display-only jitter separates coincident observations; Y is unchanged.
			raw.X = append(raw.X, x+(float64(j%7)-3)*.025)
			raw.Y = append(raw.Y, v)
			r.Y.Min = math.Min(r.Y.Min, v)
			r.Y.Max = math.Max(r.Y.Max, v)
		}
		mean := Series{ID: "mean-" + number(x), Label: "statistics:mean_" + def.ErrorBars + ":" + label, Kind: "mean", X: []float64{x}, Y: []float64{g.Mean}, N: []int{g.N}}
		if def.ErrorBars == "ci" {
			mean.Label += " · " + number(100*(1-s.Definition.Alpha)) + "%"
		}
		var width *float64
		switch def.ErrorBars {
		case "sd":
			width = g.SD
		case "sem":
			width = g.SEM
		case "ci":
			if g.CIUpper != nil {
				v := *g.CIUpper - g.Mean
				width = &v
			}
		}
		if width != nil {
			mean.Err = []float64{*width}
			r.Y.Min = math.Min(r.Y.Min, g.Mean-*width)
			r.Y.Max = math.Max(r.Y.Max, g.Mean+*width)
		} else {
			mean.Label = "statistics:mean_only:" + label
		}
		for _, series := range []*Series{&raw, &mean} {
			if st, ok := def.Series[series.ID]; ok {
				series.Hidden, series.Color = st.Hidden, st.Color
				if st.Label != "" {
					series.Label = st.Label
				}
			}
		}
		r.Series = append(r.Series, raw, mean)
	}
	if len(def.Annotations) > 0 {
		r.AnnotationStyle = def.AnnotationStyle
		positions := map[string]float64{}
		for i, g := range saved.Results.Groups {
			positions[g.FactorA+"\x00"+g.FactorB] = float64(i + 1)
		}
		comparisons := map[string]analysis.Comparison{}
		for _, c := range saved.Results.Comparisons {
			comparisons[c.ID] = c
		}
		span := r.Y.Max - r.Y.Min
		if span <= 0 {
			span = math.Max(1, math.Abs(r.Y.Max)*.1)
		}
		top := r.Y.Max
		for i, id := range def.Annotations {
			c := comparisons[id]
			label := "p(adj)=" + strconv.FormatFloat(c.AdjustedP, 'g', 6, 64)
			if c.AdjustedP == 0 {
				label = "statistics.below_precision"
			}
			if def.AnnotationStyle == "stars" {
				label = "ns"
				for _, threshold := range []float64{.05, .01, .001, .0001} {
					if c.AdjustedP < threshold {
						if label == "ns" {
							label = ""
						}
						label += "*"
					}
				}
			}
			x1, x2 := positions[c.LeftA+"\x00"+c.LeftB], positions[c.RightA+"\x00"+c.RightB]
			if x1 > x2 {
				x1, x2 = x2, x1
			}
			y := top + span*.12*float64(i+1)
			r.Annotations = append(r.Annotations, StatisticalAnnotation{ComparisonID: id, X1: x1, X2: x2, Y: y, Label: label, AdjustedP: c.AdjustedP, Correction: c.Correction})
			r.Provenance.Transformations = append(r.Provenance.Transformations, fmt.Sprintf("annotation %s: p(adj)=%s; %s", id, number(c.AdjustedP), c.Correction))
		}
		r.Y.Max = top + span*.12*float64(len(def.Annotations)+1)
		if def.AnnotationStyle == "stars" {
			r.Provenance.Transformations = append(r.Provenance.Transformations, "stars use adjusted p: * <0.05; ** <0.01; *** <0.001; **** <0.0001; ns >=0.05; display thresholds do not change analysis alpha")
		}
	}
	in := Input{Files: map[string]model.SourceFile{}, Measurements: map[string]model.Measurement{}}
	for _, src := range s.Sources {
		in.Files[src.File.ID] = src.File
		in.Measurements[src.Measurement.ID] = src.Measurement
		r.Provenance.Sources = append(r.Provenance.Sources, in.Source(src.Measurement))
	}
	return r, nil
}
