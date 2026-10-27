package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/model"
)

// The golden file holds invented values only, written by
// scripts/statistics_paired_oracle.py from independent Python libraries.
type pairedGolden struct {
	Cases []struct {
		Name  string `json:"name"`
		Input struct {
			Values  []float64 `json:"values"`
			FactorA []string  `json:"factorA"`
			FactorB []string  `json:"factorB"`
			UnitID  []string  `json:"unitId"`
		} `json:"input"`
		Expected struct {
			ExperimentalUnits int                     `json:"experimentalUnits"`
			Measurements      int                     `json:"measurements"`
			Cells             int                     `json:"cells"`
			Terms             []analysis.Term         `json:"terms"`
			Groups            []analysis.Group        `json:"groups"`
			Comparisons       []analysis.Comparison   `json:"comparisons"`
			SimpleEffects     []analysis.SimpleEffect `json:"simpleEffects"`
			Corrections       map[string]float64      `json:"corrections"`
			Residuals         []float64               `json:"residuals"`
			Model             *analysis.MixedModel    `json:"model"`
		} `json:"expected"`
	} `json:"cases"`
}

func loadPairedGolden(t *testing.T) pairedGolden {
	t.Helper()
	raw, err := os.ReadFile("../../web/tests/fixtures/statistics-paired-golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var g pairedGolden
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	return g
}

// pairedFixture imports one synthetic measurement per golden row and maps the
// invented unit labels to experimental units created in this Profile.
func pairedFixture(t *testing.T, p *Profile, g pairedGolden, name, method, postHoc, adjustment string) (analysis.Definition, map[string]string, int) {
	t.Helper()
	index := -1
	for i, c := range g.Cases {
		if c.Name == name {
			index = i
		}
	}
	if index < 0 {
		t.Fatal("golden case missing", name)
	}
	in := g.Cases[index].Input
	d := analysis.Definition{Schema: 1, Module: "lightscattering", Title: "Synthetic paired design", Variable: model.EffectiveDiameter, FactorAName: "Condition", FactorBName: "Time", Structure: "repeated", StructureReviewed: true, Method: method, Alpha: .05, PostHoc: postHoc, Adjustment: adjustment, Design: analysis.DesignWithinWithin}
	if method == "repeated" {
		d.SphericityCorrection = "GG"
	}
	units := map[string]string{}
	for i, v := range in.Values {
		if units[in.UnitID[i]] == "" {
			u, err := p.CreateExperimentalUnit("Invented " + in.UnitID[i])
			if err != nil {
				t.Fatal(err)
			}
			units[in.UnitID[i]] = u.ID
		}
		text := fmt.Sprintf("Sample ID: invented-%s-%d\nEffective Diameter (nm): %s\n", name, i, strconv.FormatFloat(v, 'g', -1, 64))
		imp := p.Import(fmt.Sprintf("synthetic-paired-%s-%d.txt", name, i), []byte(text), false)
		if imp.Error != "" || imp.Measurements != 1 {
			t.Fatal("synthetic import failed", imp)
		}
		f, _, err := p.File(imp.FileID)
		if err != nil {
			t.Fatal(err)
		}
		d.Observations = append(d.Observations, analysis.ObservationDefinition{MeasurementID: f.Measurements[0], FactorA: in.FactorA[i], FactorB: in.FactorB[i], UnitID: units[in.UnitID[i]]})
	}
	return d, units, index
}

// pairedResults rebuilds the browser result contract from the oracle values.
func pairedResults(g pairedGolden, index int, d analysis.Definition, units map[string]string) analysis.Results {
	e := g.Cases[index].Expected
	raw, _ := json.Marshal(e)
	var copyOf struct {
		Terms         []analysis.Term         `json:"terms"`
		Groups        []analysis.Group        `json:"groups"`
		Comparisons   []analysis.Comparison   `json:"comparisons"`
		SimpleEffects []analysis.SimpleEffect `json:"simpleEffects"`
		Model         *analysis.MixedModel    `json:"model"`
	}
	json.Unmarshal(raw, &copyOf)
	r := analysis.Results{Engine: analysis.ExpectedEngine(d), Calculation: analysis.CalculationVersion, Method: d.Method, Design: analysis.DesignWithinWithin, Terms: copyOf.Terms, Groups: copyOf.Groups, Comparisons: copyOf.Comparisons, SimpleEffects: copyOf.SimpleEffects, Residuals: e.Residuals}
	r.QQObserved, r.QQTheoretical = make([]float64, len(e.Residuals)), make([]float64, len(e.Residuals))
	for i := range r.Groups {
		for j, u := range r.Groups[i].Units {
			r.Groups[i].Units[j] = units[u]
		}
	}
	correction := map[string]string{"holm": "Holm; paired t; individual CI", "bonferroni": "Bonferroni; paired t; individual CI"}[d.Adjustment]
	for i := range r.Comparisons {
		r.Comparisons[i].ID = fmt.Sprintf("comparison-%d", i+1)
		r.Comparisons[i].Correction = correction
	}
	if d.Method == "repeated" {
		r.SSType = "repeated_within_within_strata"
		r.Corrections = map[string]float64{}
		for k, v := range e.Corrections {
			r.Corrections[k] = v
		}
	} else {
		r.SSType = "not_applicable_marginal_Wald_F"
		m := copyOf.Model
		m.Family, m.Fixed, m.Estimation, m.Random = "nested_random_intercepts", "A*B", "REML", "1|unit/A"
		m.ResidualCovariance, m.Test, m.BoundaryTolerance = "homoscedastic conditional errors", "marginal Wald F; sum contrasts; nlme containment denominator df", 1e-4
		m.CoefficientConfidenceLevel, m.CoefficientIntervalMethod = 1-d.Alpha, "nlme::intervals.lme fixed; Student t; conditional GLS; approximate; individual"
		r.Model = m
	}
	return r
}

// Addendum B 60: 3 units x 2 conditions x 4 times are 24 measurements of
// 3 independent experimental units, never 24 subjects.
func TestPairedDesignCountsUnitsNotMeasurements(t *testing.T) {
	_, _, p := inspectionProfile(t)
	g := loadPairedGolden(t)
	d, units, index := pairedFixture(t, p, g, "owner_3x2x4_repeated_gg_holm", "repeated", "simple_effects", "holm")
	s, err := p.PrepareAnalysis(d)
	if err != nil {
		t.Fatal(err)
	}
	if s.Design.ExperimentalUnits != 3 || s.Design.N != 24 || s.Design.UnitCells != 24 || s.Design.TechnicalReadingCells != 0 || !s.Design.CompleteRepeated || s.Design.Recommended != "repeated" {
		t.Fatal("paired design misread", s.Design)
	}
	if s.Design.ExperimentalUnits == s.Design.N {
		t.Fatal("measurements counted as subjects")
	}
	r := pairedResults(g, index, d, units)
	saved, err := p.SaveAnalysis(d, s.Receipt, r, "")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := p.Analysis(saved.ID)
	if err != nil || restored.SourceChanged || !reflect.DeepEqual(restored.Results.Terms, r.Terms) || !reflect.DeepEqual(restored.Results.Groups, r.Groups) || restored.Snapshot.Definition.Design != analysis.DesignWithinWithin {
		t.Fatal("paired analysis not preserved", err)
	}
	for _, group := range restored.Results.Groups {
		if group.N != 3 {
			t.Fatal("cell n is not the unit count", group)
		}
	}
	twentyFour, sixteen := 24., 16.
	for name, mutate := range map[string]func(*analysis.Results){
		"group counted as measurements":  func(v *analysis.Results) { v.Groups[0].N = 24 },
		"pseudoreplicated A df":          func(v *analysis.Results) { v.Terms[1].DF = 23 },
		"denominator missing":            func(v *analysis.Results) { v.Terms[1].DenominatorDF = nil },
		"epsilon above one":              func(v *analysis.Results) { x := 1.5; v.Terms[len(v.Terms)-1].Epsilon = &x },
		"corrected df not epsilon times": func(v *analysis.Results) { v.Terms[len(v.Terms)-1].DF = 2.5 },
		"unknown term":                   func(v *analysis.Results) { v.Terms[1].Source = "residual" },
		"missing interaction":            func(v *analysis.Results) { v.Terms = v.Terms[:5] },
		"value changed":                  func(v *analysis.Results) { v.Groups[0].Values[0]++ },
		"unit swapped": func(v *analysis.Results) {
			v.Groups[0].Units[0], v.Groups[0].Units[1] = v.Groups[0].Units[1], v.Groups[0].Units[0]
		},
		"wrong correction":              func(v *analysis.Results) { v.Comparisons[0].Correction = "Tukey HSD" },
		"unknown family":                func(v *analysis.Results) { v.Comparisons[0].Family = "pooled" },
		"pairs counted as measurements": func(v *analysis.Results) { v.Comparisons[0].N = 24 },
		"duplicate comparison":          func(v *analysis.Results) { v.Comparisons[1] = v.Comparisons[0]; v.Comparisons[1].ID = "comparison-x" },
		"adjusted below raw p":          func(v *analysis.Results) { v.Comparisons[0].AdjustedP = 0 },
		"simple effect lost terms":      func(v *analysis.Results) { v.SimpleEffects[0].Terms = v.SimpleEffects[0].Terms[:1] },
		"residuals of measurements":     func(v *analysis.Results) { v.Residuals = append(v.Residuals, 0) },
		"design not echoed":             func(v *analysis.Results) { v.Design = "" },
		"mixed model on classical":      func(v *analysis.Results) { v.Model = &analysis.MixedModel{} },
		"engine":                        func(v *analysis.Results) { v.Engine = analysis.EngineVersion },
		"independent df 24":             func(v *analysis.Results) { v.Terms[1].DenominatorDF = &twentyFour; v.Terms[1].DF = 24 },
		"independent df 16":             func(v *analysis.Results) { v.Terms[1].DF = sixteen },
	} {
		raw, _ := json.Marshal(r)
		var bad analysis.Results
		json.Unmarshal(raw, &bad)
		mutate(&bad)
		if validateAnalysisResults(s, bad) == nil {
			t.Fatal("paired contract accepted:", name)
		}
	}
	// Declaring the 24 measurements independent is refused: the pairing is
	// the reviewed design, and shared units are never independent subjects.
	independent := d
	independent.Structure = "independent"
	if _, err := p.PrepareAnalysis(independent); !errors.Is(err, analysis.ErrStructure) {
		t.Fatal("paired design accepted as independent", err)
	}
	legacy := d
	legacy.Design, legacy.PostHoc, legacy.Adjustment, legacy.Method, legacy.Structure, legacy.SphericityCorrection = "", "none", "", "two_way", "independent", ""
	if _, err := p.PrepareAnalysis(legacy); !errors.Is(err, analysis.ErrStructure) {
		t.Fatal("pseudoreplicated two-way accepted", err)
	}
	legacy.Method, legacy.Structure, legacy.SphericityCorrection = "repeated", "repeated", "GG"
	if _, err := p.PrepareAnalysis(legacy); !errors.Is(err, analysis.ErrDesign) {
		t.Fatal("between-unit condition accepted for paired units", err)
	}
}

func TestPairedTechnicalReadingsIncompleteAndMixedContract(t *testing.T) {
	_, _, p := inspectionProfile(t)
	g := loadPairedGolden(t)
	d, units, index := pairedFixture(t, p, g, "technical_readings_repeated_gg_holm", "repeated", "simple_effects", "holm")
	s, err := p.PrepareAnalysis(d)
	if err != nil {
		t.Fatal(err)
	}
	// Addendum B 62: readings in a cell never add experimental units.
	if s.Design.ExperimentalUnits != 3 || s.Design.N != 28 || s.Design.UnitCells != 24 || s.Design.TechnicalReadingCells != 3 {
		t.Fatal("technical readings miscounted", s.Design)
	}
	warned := false
	for _, w := range s.Design.Warnings {
		warned = warned || w == "statistics.technical_readings_averaged"
	}
	if !warned {
		t.Fatal("averaging of technical readings not stated")
	}
	r := pairedResults(g, index, d, units)
	if _, err := p.SaveAnalysis(d, s.Receipt, r, ""); err != nil {
		t.Fatal(err)
	}
	bad := pairedResults(g, index, d, units)
	for i := range bad.Groups {
		for j := range bad.Groups[i].Readings {
			bad.Groups[i].Readings[j] = 1
		}
	}
	if validateAnalysisResults(s, bad) == nil {
		t.Fatal("reading counts not bound to sources")
	}

	d, units, index = pairedFixture(t, p, g, "six_units_incomplete_mixed", "mixed", "none", "")
	s, err = p.PrepareAnalysis(d)
	if err != nil || s.Design.CompleteRepeated || s.Design.Recommended != "mixed" || s.Design.ExperimentalUnits != 6 {
		t.Fatal("incomplete mixed paired design rejected", err, s.Design)
	}
	r = pairedResults(g, index, d, units)
	if _, err := p.SaveAnalysis(d, s.Receipt, r, ""); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*analysis.Results){
		"ML instead of REML":        func(v *analysis.Results) { v.Model.Estimation = "ML" },
		"single random intercept":   func(v *analysis.Results) { v.Model.Random = "1|unit" },
		"boundary component":        func(v *analysis.Results) { v.Model.RandomComponents[1].Variance = 1e-12 },
		"missing component":         func(v *analysis.Results) { v.Model.RandomComponents = v.Model.RandomComponents[:1] },
		"classical SS on mixed":     func(v *analysis.Results) { x := 1.; v.Terms[0].SS = &x },
		"coefficient without df":    func(v *analysis.Results) { v.Model.FixedCoefficients[0].DF = nil },
		"sphericity on mixed":       func(v *analysis.Results) { v.Corrections = map[string]float64{"GG:B": .5} },
		"numerator df not levels-1": func(v *analysis.Results) { v.Terms[1].DF = 4 },
	} {
		raw, _ := json.Marshal(r)
		var bad analysis.Results
		json.Unmarshal(raw, &bad)
		mutate(&bad)
		if validateAnalysisResults(s, bad) == nil {
			t.Fatal("mixed paired contract accepted:", name)
		}
	}
	for _, change := range []func(*analysis.Definition){
		func(v *analysis.Definition) { v.Method, v.SphericityCorrection = "repeated", "GG" },
		func(v *analysis.Definition) { v.PostHoc, v.Adjustment = "simple_effects", "holm" },
	} {
		def := d
		change(&def)
		if _, err := p.PrepareAnalysis(def); !errors.Is(err, analysis.ErrDesign) {
			t.Fatal("incomplete paired design accepted for a complete-only procedure", err)
		}
	}
}

func TestPairedDefinitionValidation(t *testing.T) {
	base := analysis.Definition{Schema: 1, Module: "lightscattering", Title: "Synthetic", Variable: model.EffectiveDiameter, Structure: "repeated", StructureReviewed: true, Method: "repeated", Alpha: .05, PostHoc: "none", SphericityCorrection: "GG", Design: analysis.DesignWithinWithin,
		Observations: []analysis.ObservationDefinition{{MeasurementID: "m1", FactorA: "F-SR", FactorB: "day 0"}, {MeasurementID: "m2", FactorA: "C-SR", FactorB: "day 0"}}}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		change func(*analysis.Definition)
		want   error
	}{
		"unknown design":            {func(v *analysis.Definition) { v.Design = "nested" }, analysis.ErrDefinition},
		"independent":               {func(v *analysis.Definition) { v.Structure = "independent" }, analysis.ErrStructure},
		"one-way":                   {func(v *analysis.Definition) { v.Method = "one_way" }, analysis.ErrDesign},
		"two-way":                   {func(v *analysis.Definition) { v.Method = "two_way" }, analysis.ErrDesign},
		"tukey":                     {func(v *analysis.Definition) { v.PostHoc = "tukey" }, analysis.ErrDesign},
		"adjustment without tests":  {func(v *analysis.Definition) { v.Adjustment = "holm" }, analysis.ErrMethod},
		"simple effects unadjusted": {func(v *analysis.Definition) { v.PostHoc = "simple_effects" }, analysis.ErrDefinition},
		"unknown adjustment":        {func(v *analysis.Definition) { v.PostHoc, v.Adjustment = "simple_effects", "none" }, analysis.ErrDefinition},
		"mixed with sphericity":     {func(v *analysis.Definition) { v.Method = "mixed" }, analysis.ErrDesign},
		"missing time":              {func(v *analysis.Definition) { v.Observations[0].FactorB = "" }, analysis.ErrDesign},
		"effect CI":                 {func(v *analysis.Definition) { v.EffectCI = true }, analysis.ErrDesign},
	} {
		def := base
		def.Observations = append([]analysis.ObservationDefinition{}, base.Observations...)
		c.change(&def)
		if err := def.Validate(); !errors.Is(err, c.want) {
			t.Fatal(name, err)
		}
	}
	ok := base
	ok.PostHoc, ok.Adjustment = "simple_effects", "bonferroni"
	if ok.Validate() != nil || analysis.ExpectedEngine(ok) != "webR/0.6.0; R/4.6.0; paired-rm/1; paired-simple-effects/1" {
		t.Fatal("paired simple effects rejected")
	}
	ok.Method, ok.SphericityCorrection = "mixed", ""
	if ok.Validate() != nil || analysis.ExpectedEngine(ok) != "webR/0.6.0; R/4.6.0; nlme/3.1-169; mixed-within2/1; paired-simple-effects/1" {
		t.Fatal("paired mixed rejected")
	}
	legacy := base
	legacy.Design, legacy.Adjustment, legacy.PostHoc = "", "holm", "none"
	if err := legacy.Validate(); !errors.Is(err, analysis.ErrMethod) {
		t.Fatal("adjustment accepted outside the paired design", err)
	}
	legacy.Adjustment, legacy.PostHoc = "", "simple_effects"
	if err := legacy.Validate(); !errors.Is(err, analysis.ErrMethod) {
		t.Fatal("simple effects accepted outside the paired design", err)
	}
}
