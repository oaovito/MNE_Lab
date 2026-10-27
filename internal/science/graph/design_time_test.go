package graph

// Design graphs (condition × time and the paired view), combination and
// statistical annotations. All values are synthetic (invented for tests).

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/design"
	"github.com/oaovito/mne_lab/internal/science/model"
)

func fp(v float64) *float64 { return &v }

// Synthetic effective diameters (nm) of units A, B, C under F-SR and C-SR
// at days 0, 7, 14 and 21.
var synthetic = map[string]map[string][]float64{
	"A": {"F-SR": {120, 124, 130, 135}, "C-SR": {100, 103, 108, 112}},
	"B": {"F-SR": {118, 121, 128, 133}, "C-SR": {98, 101, 106, 111}},
	"C": {"F-SR": {122, 126, 131, 138}, "C-SR": {103, 105, 110, 115}},
}

var dayLabels = []string{"Day 0", "Day 7", "Day 14", "Day 21"}

// pairedDesign is the 3 units × 2 conditions × 4 times design with one
// measurement per cell (24 measurements).
func pairedDesign(unit string) (Input, design.Design) {
	in := Input{Measurements: map[string]model.Measurement{}, Files: map[string]model.SourceFile{"f1": {ID: "f1", Name: "synthetic.txt", SHA256: "sum"}}}
	d := design.Design{ID: "rev1", Root: "rev1", Revision: 1, Name: "Synthetic stability", Version: design.Version,
		Units:      []design.Unit{{ID: "uA", Label: "A"}, {ID: "uB", Label: "B"}, {ID: "uC", Label: "C"}},
		Conditions: []design.Condition{{Label: "F-SR"}, {Label: "C-SR", DerivedFrom: "F-SR"}}}
	for i, l := range dayLabels {
		d.Times = append(d.Times, design.TimeLevel{Label: l, Offset: fp(float64(7 * i)), Unit: "days"})
	}
	for _, u := range []string{"A", "B", "C"} {
		for _, c := range []string{"F-SR", "C-SR"} {
			for ti, tl := range dayLabels {
				id := c + "-" + u + "-" + strconv.Itoa(ti)
				v := synthetic[u][c][ti]
				in.Measurements[id] = model.Measurement{ID: id, FileID: "f1", SampleID: c + " " + u,
					Params: map[string]model.Quantity{model.EffectiveDiameter: {Value: v, Raw: strconv.FormatFloat(v, 'f', -1, 64), Unit: unit}, model.Polydispersity: {Value: .1, Raw: "0.1"}}}
				d.Assignments = append(d.Assignments, design.Assignment{MeasurementID: id, UnitID: "u" + u, Condition: c, Time: tl, ReplicateKind: design.ReplicateExperimental})
			}
		}
		d.Pairs = append(d.Pairs, design.Pair{UnitID: "u" + u, From: "F-SR", To: "C-SR"})
	}
	return in, d
}

func seriesByID(res Result, id string) (Series, bool) {
	for _, s := range res.Series {
		if s.ID == id {
			return s, true
		}
	}
	return Series{}, false
}

func sampleSD(v []float64) float64 {
	m := 0.0
	for _, x := range v {
		m += x
	}
	m /= float64(len(v))
	ss := 0.0
	for _, x := range v {
		ss += (x - m) * (x - m)
	}
	return math.Sqrt(ss / float64(len(v)-1))
}

func TestDesignTimeCountsExperimentalUnits(t *testing.T) {
	in, d := pairedDesign("nm")
	def := Definition{Kind: KindDesignTime, DesignID: d.ID, Param: model.EffectiveDiameter, Conditions: []string{"F-SR"}}
	res, err := DesignTime(def, in, DesignInput{Design: d})
	if err != nil {
		t.Fatal(err)
	}
	mean, ok := seriesByID(res, "mean:F-SR")
	if !ok || len(mean.Y) != 4 {
		t.Fatalf("mean series: %+v", res.Series)
	}
	for ti := range dayLabels {
		vals := []float64{synthetic["A"]["F-SR"][ti], synthetic["B"]["F-SR"][ti], synthetic["C"]["F-SR"][ti]}
		if mean.N[ti] != 3 {
			t.Fatalf("n at %d = %d, want 3 experimental units", ti, mean.N[ti])
		}
		if math.Abs(mean.Err[ti]-sampleSD(vals)) > 1e-12 {
			t.Fatalf("SD at %d = %v, want sample SD %v", ti, mean.Err[ti], sampleSD(vals))
		}
	}
	if res.ErrorBars != "sd" || res.Y.Unit != "nm" || res.X.Unit != "days" || len(res.X.Ticks) != 4 || res.X.Ticks[3] != 21 {
		t.Fatalf("axes or error bars: %+v %+v %q", res.X, res.Y, res.ErrorBars)
	}
	// Individual unit values are drawn by default, each named by its unit.
	units, ok := seriesByID(res, "units:F-SR")
	if !ok || units.Hidden || len(units.Y) != 12 || units.PointLabels[0] != "A" {
		t.Fatalf("individual points: %+v", units)
	}
	if res.Design == nil || res.Design.ID != "rev1" || res.Design.Revision != 1 {
		t.Fatalf("design ref: %+v", res.Design)
	}
	// Graph 2 (C-SR) on its own.
	def.Conditions = []string{"C-SR"}
	res2, err := DesignTime(def, in, DesignInput{Design: d})
	if err != nil || len(res2.Series) != 2 || res2.Series[0].ID != "mean:C-SR" {
		t.Fatalf("graph 2: %v %+v", err, res2.Series)
	}
}

func TestTechnicalReadingsNeverCountAsN(t *testing.T) {
	in := Input{Measurements: map[string]model.Measurement{}, Files: map[string]model.SourceFile{}}
	d := design.Design{ID: "r", Root: "r", Revision: 1, Name: "Readings", Units: []design.Unit{{ID: "uA", Label: "A"}},
		Conditions: []design.Condition{{Label: "F-SR"}}, Times: []design.TimeLevel{{Label: "Day 0"}}}
	sum := 0.0
	for i := 0; i < 12; i++ {
		id := "m" + strconv.Itoa(i)
		v := 100 + float64(i)
		sum += v
		in.Measurements[id] = model.Measurement{ID: id, Params: map[string]model.Quantity{model.EffectiveDiameter: {Value: v, Raw: strconv.Itoa(int(v)), Unit: "nm"}}}
		kind := design.ReplicateTechnical
		if i == 0 {
			kind = design.ReplicateExperimental
		}
		d.Assignments = append(d.Assignments, design.Assignment{MeasurementID: id, UnitID: "uA", Condition: "F-SR", Time: "Day 0", ReplicateKind: kind})
	}
	res, err := DesignTime(Definition{Kind: KindDesignTime, DesignID: "r", Param: model.EffectiveDiameter}, in, DesignInput{Design: d})
	if err != nil {
		t.Fatal(err)
	}
	mean, _ := seriesByID(res, "mean:F-SR")
	if len(mean.N) != 1 || mean.N[0] != 1 || mean.Y[0] != sum/12 || mean.Err[0] != 0 {
		t.Fatalf("12 readings of one unit must give n = 1 and their mean: %+v", mean)
	}
	if !contains(res.Warnings, "graph.not_enough_replicates_for_statistics") {
		t.Fatalf("warnings: %v", res.Warnings)
	}
	tab, err := DesignTables(Definition{Kind: KindDesignTime, DesignID: "r", Param: model.EffectiveDiameter}, in, DesignInput{Design: d})
	if err != nil || len(tab.Readings) != 12 || len(tab.Units) != 1 || tab.Units[0].Readings != 12 || tab.Summary[0].N != 1 || tab.Summary[0].SD != nil {
		t.Fatalf("table: %v %+v", err, tab.Summary)
	}
}

func TestDesignTimeErrorBarsAndTrajectories(t *testing.T) {
	in, d := pairedDesign("nm")
	def := Definition{Kind: KindDesignTime, DesignID: d.ID, Param: model.EffectiveDiameter, ErrorBars: "ci", UnitTrajectories: true}
	res, err := DesignTime(def, in, DesignInput{Design: d})
	if err != nil {
		t.Fatal(err)
	}
	// A confidence interval needs a saved analysis: SD is drawn and said.
	if res.ErrorBars != "sd" || !contains(res.Warnings, "graph.ci_needs_saved_analysis") {
		t.Fatalf("ci without analysis: %q %v", res.ErrorBars, res.Warnings)
	}
	n := 0
	for _, s := range res.Series {
		if s.Kind == "trajectory" {
			n++
			if len(s.X) != 4 {
				t.Fatalf("trajectory: %+v", s)
			}
		}
	}
	if n != 6 {
		t.Fatalf("trajectories: %d, want 3 units × 2 conditions", n)
	}
	def.ErrorBars = "sem"
	res, _ = DesignTime(def, in, DesignInput{Design: d})
	mean, _ := seriesByID(res, "mean:F-SR")
	vals := []float64{120, 118, 122}
	if res.ErrorBars != "sem" || math.Abs(mean.Err[0]-sampleSD(vals)/math.Sqrt(3)) > 1e-12 {
		t.Fatalf("sem: %v", mean.Err)
	}
}

func TestDesignTimeRefusesMixedUnits(t *testing.T) {
	in, d := pairedDesign("nm")
	for id, m := range in.Measurements {
		if strings.HasPrefix(id, "C-SR") {
			q := m.Params[model.EffectiveDiameter]
			q.Unit, q.Value = "µm", q.Value/1000
			m.Params[model.EffectiveDiameter] = q
			in.Measurements[id] = m
		}
	}
	def := Definition{Kind: KindDesignTime, DesignID: d.ID, Param: model.EffectiveDiameter}
	if _, err := DesignTime(def, in, DesignInput{Design: d}); err != ErrMixedUnits {
		t.Fatalf("mixed units must be refused, got %v", err)
	}
	def.DisplayUnit = "nm"
	res, err := DesignTime(def, in, DesignInput{Design: d})
	if err != nil || res.Y.Unit != "nm" || res.Y.Conversion == nil {
		t.Fatalf("exact conversion: %v %+v", err, res.Y)
	}
	mean, _ := seriesByID(res, "mean:C-SR")
	if math.Abs(mean.Y[0]-(100+98+103)/3.0) > 1e-9 {
		t.Fatalf("converted mean: %v", mean.Y)
	}
}

func TestPairedConditionJoinsUnits(t *testing.T) {
	in, d := pairedDesign("nm")
	def := Definition{Kind: KindPairedCondition, DesignID: d.ID, Param: model.EffectiveDiameter, PairedAt: "Day 7"}
	res, err := PairedCondition(def, in, DesignInput{Design: d})
	if err != nil {
		t.Fatal(err)
	}
	pair, ok := seriesByID(res, "pair:uA")
	if !ok || len(pair.Y) != 2 || pair.Y[0] != 124 || pair.Y[1] != 103 || pair.Meta["pairing"] != "declared" {
		t.Fatalf("A pair: %+v", pair)
	}
	if strings.Join(res.X.Categories, ",") != "F-SR,C-SR" {
		t.Fatalf("categories: %v", res.X.Categories)
	}
	d.Pairs = nil
	res, _ = PairedCondition(def, in, DesignInput{Design: d})
	if !contains(res.Warnings, "graph.pair_not_declared:A") {
		t.Fatalf("undeclared pairs must be said: %v", res.Warnings)
	}
	def.PairedAt = "Day 99"
	if _, err := PairedCondition(def, in, DesignInput{Design: d}); err != ErrUnknownTime {
		t.Fatalf("unknown time: %v", err)
	}
	def.PairedAt, def.Conditions = "Day 0", []string{"F-SR"}
	if _, err := PairedCondition(def, in, DesignInput{Design: d}); err != ErrPairedConditions {
		t.Fatalf("one condition: %v", err)
	}
}

func savedAnalysis(d design.Design) analysis.StatisticalAnalysis {
	a := analysis.StatisticalAnalysis{ID: "an1"}
	a.Snapshot.Definition.Variable = model.EffectiveDiameter
	a.Snapshot.Definition.Alpha = .05
	a.Snapshot.ExperimentalDesign = &d
	a.Results.Engine = "test engine"
	a.Results.Groups = []analysis.Group{{FactorA: "F-SR", FactorB: "Day 0", N: 3, Mean: 120, CIUpper: fp(125)}}
	a.Results.Comparisons = []analysis.Comparison{
		{ID: "comparison-1", LeftA: "F-SR", LeftB: "Day 0", RightA: "C-SR", RightB: "Day 0", Contrast: "C-SR-F-SR", Context: "B=Day 0", Family: "A_within_B",
			Difference: -19.6667, Lower: fp(-22.5), Upper: fp(-16.8), AdjustedP: 0.0123456, Correction: "Holm; paired t; individual CI"},
		{ID: "comparison-2", LeftA: "F-SR", LeftB: "Day 0", RightA: "F-SR", RightB: "Day 21", Contrast: "Day 21-Day 0", Context: "A=F-SR", Family: "B_within_A",
			Difference: 15.3333, AdjustedP: 0.0004, Correction: "Holm; paired t; individual CI"},
	}
	return a
}

func TestDesignAnnotationsComeFromSavedAnalysis(t *testing.T) {
	in, d := pairedDesign("nm")
	a := savedAnalysis(d)
	def := Definition{Kind: KindDesignTime, DesignID: d.ID, Param: model.EffectiveDiameter, AnalysisID: "an1", Annotations: []string{"comparison-1", "comparison-2"}}
	res, err := DesignTime(def, in, DesignInput{Design: d, Analysis: &a})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Annotations) != 2 {
		t.Fatalf("annotations: %+v %v", res.Annotations, res.Warnings)
	}
	first := res.Annotations[0]
	if first.ComparisonID != "comparison-1" || first.AdjustedP != 0.0123456 || first.Label != "p(adj)=0.0123456" || first.AnalysisID != "an1" || first.Contrast != "C-SR-F-SR" {
		t.Fatalf("annotation must carry the stored values: %+v", first)
	}
	if first.Y <= 125 || res.Y.Max < first.Y {
		t.Fatalf("bracket above the values: %+v %v", first, res.Y.Max)
	}
	def.AnnotationStyle = AnnotationStars
	res, _ = DesignTime(def, in, DesignInput{Design: d, Analysis: &a})
	if res.Annotations[0].Label != "*" || res.Annotations[1].Label != "***" {
		t.Fatalf("stars: %q %q", res.Annotations[0].Label, res.Annotations[1].Label)
	}
	def.AnnotationStyle = AnnotationEffect
	res, _ = DesignTime(def, in, DesignInput{Design: d, Analysis: &a})
	if res.Annotations[0].Label != "Δ -19.67 nm [-22.5, -16.8] · p(adj)=0.0123456" {
		t.Fatalf("effect: %q", res.Annotations[0].Label)
	}
	// Shown in µm, the stored difference and interval convert exactly.
	def.DisplayUnit = "µm"
	Finish(def, &res)
	if res.Annotations[0].Label != "Δ -0.01967 µm [-0.0225, -0.0168] · p(adj)=0.0123456" {
		t.Fatalf("converted effect: %q", res.Annotations[0].Label)
	}
	// A comparison the analysis does not hold is refused.
	def.DisplayUnit, def.Annotations = "", []string{"comparison-9"}
	if _, err := DesignTime(def, in, DesignInput{Design: d, Analysis: &a}); err != analysis.ErrDefinition {
		t.Fatalf("unknown comparison: %v", err)
	}
	// Another design or another variable is incompatible.
	other := d
	other.Root = "other"
	if err := CompatibleAnalysis(def, a, other); err != ErrAnalysisIncompatible {
		t.Fatalf("other design: %v", err)
	}
	def.Param = model.Polydispersity
	if err := CompatibleAnalysis(def, a, d); err != ErrAnalysisIncompatible {
		t.Fatalf("other variable: %v", err)
	}
	// The confidence interval is the saved one.
	def = Definition{Kind: KindDesignTime, DesignID: d.ID, Param: model.EffectiveDiameter, AnalysisID: "an1", ErrorBars: "ci", Conditions: []string{"F-SR"}}
	res, _ = DesignTime(def, in, DesignInput{Design: d, Analysis: &a})
	mean, _ := seriesByID(res, "mean:F-SR")
	if res.ErrorBars != "ci" || mean.Err[0] != 5 || mean.Err[1] != 0 {
		t.Fatalf("saved CI: %v", mean.Err)
	}
	// Without a linked analysis, requested annotations are not drawn.
	def.Annotations = []string{"comparison-1"}
	res, _ = DesignTime(def, in, DesignInput{Design: d})
	if len(res.Annotations) != 0 || !contains(res.Warnings, "graph.annotations_need_analysis") {
		t.Fatalf("no analysis: %+v %v", res.Annotations, res.Warnings)
	}
}

func TestCombineDesignGraphs(t *testing.T) {
	in, d := pairedDesign("nm")
	g1 := Definition{ID: "g1", Title: "Graph 1", Kind: KindDesignTime, DesignID: d.ID, Param: model.EffectiveDiameter, Conditions: []string{"F-SR"}, ErrorBars: "sd", Visual: DefaultVisual()}
	g2 := g1
	g2.ID, g2.Title, g2.Conditions = "g2", "Graph 2", []string{"C-SR"}
	r1, _ := DesignTime(g1, in, DesignInput{Design: d})
	r2, _ := DesignTime(g2, in, DesignInput{Design: d})
	ck := CheckCombine([]Definition{g1, g2}, []Result{r1, r2}, "")
	if !ck.Compatible || len(ck.Problems) != 0 {
		t.Fatalf("check: %+v", ck)
	}
	c, err := Combine([]Definition{g1, g2}, []string{"h1", "h2"}, "", ck, "g3", r1.Provenance.ComputedAt)
	if err != nil {
		t.Fatal(err)
	}
	if c.Title != "Graph 1 vs Graph 2" || strings.Join(c.Conditions, ",") != "F-SR,C-SR" || len(c.DerivedFrom) != 2 ||
		c.DerivedFrom[0].GraphID != "g1" || c.DerivedFrom[1].DefinitionHash != "h2" || c.DerivedFrom[0].Relation != RelationCombine {
		t.Fatalf("combined: %+v", c)
	}
	r3, err := DesignTime(c, in, DesignInput{Design: d})
	if err != nil {
		t.Fatal(err)
	}
	m1, _ := seriesByID(r3, "mean:F-SR")
	m2, _ := seriesByID(r3, "mean:C-SR")
	if len(m1.Y) != 4 || len(m2.Y) != 4 || m1.X[0] == m2.X[0] {
		t.Fatalf("both conditions on the shared time axis, side by side: %v %v", m1.X, m2.X)
	}
	// Graphs of another variable, kind or design are refused.
	pdi := g2
	pdi.Param = model.Polydispersity
	rp, _ := DesignTime(pdi, in, DesignInput{Design: d})
	if ck := CheckCombine([]Definition{g1, pdi}, []Result{r1, rp}, ""); ck.Compatible || !contains(ck.Problems, CombineVariable) {
		t.Fatalf("PDI with effective diameter: %+v", ck)
	}
	other := g2
	other.DesignID = "rev2"
	if ck := CheckCombine([]Definition{g1, other}, []Result{r1, r2}, ""); ck.Compatible || !contains(ck.Problems, CombineDesign) {
		t.Fatalf("other design: %+v", ck)
	}
	if ck := CheckCombine([]Definition{g1}, []Result{r1}, ""); ck.Compatible || !contains(ck.Problems, CombineCount) {
		t.Fatalf("one graph: %+v", ck)
	}
	if _, err := Combine([]Definition{g1, pdi}, []string{"a", "b"}, "", CheckCombine([]Definition{g1, pdi}, []Result{r1, rp}, ""), "x", r1.Provenance.ComputedAt); err != ErrCombine {
		t.Fatalf("an incompatible combination must be refused: %v", err)
	}
}

func TestCombineOffersOnlyExactConversion(t *testing.T) {
	inNM, d := pairedDesign("nm")
	inUM, _ := pairedDesign("µm")
	g1 := Definition{ID: "g1", Kind: KindDesignTime, DesignID: d.ID, Param: model.EffectiveDiameter, Conditions: []string{"F-SR"}}
	g2 := g1
	g2.ID, g2.Conditions = "g2", []string{"C-SR"}
	r1, _ := DesignTime(g1, inNM, DesignInput{Design: d})
	r2, _ := DesignTime(g2, inUM, DesignInput{Design: d})
	ck := CheckCombine([]Definition{g1, g2}, []Result{r1, r2}, "")
	if ck.Compatible || !contains(ck.Problems, CombineUnits) || !contains(ck.Conversions, "nm") || !contains(ck.Conversions, "µm") {
		t.Fatalf("different units: %+v", ck)
	}
	if ck := CheckCombine([]Definition{g1, g2}, []Result{r1, r2}, "cps"); ck.Compatible || !contains(ck.Problems, CombineUnitFixed) {
		t.Fatalf("a conversion that is not exact must be refused: %+v", ck)
	}
	ck = CheckCombine([]Definition{g1, g2}, []Result{r1, r2}, "nm")
	if !ck.Compatible || ck.DisplayUnit != "nm" || !contains(ck.Notes, CombineConvertedNote) {
		t.Fatalf("accepted conversion: %+v", ck)
	}
	c, _ := Combine([]Definition{g1, g2}, []string{"a", "b"}, "", ck, "g3", r1.Provenance.ComputedAt)
	if c.DisplayUnit != "nm" {
		t.Fatalf("display unit: %q", c.DisplayUnit)
	}
}
