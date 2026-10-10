package app

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/export"
	"github.com/oaovito/mne_lab/internal/plot"
	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/store"
)

func statisticalFixture(t *testing.T, p *Profile) analysis.Definition {
	t.Helper()
	d := analysis.Definition{Schema: 1, Module: "lightscattering", Title: "Synthetic analysis", Variable: model.EffectiveDiameter, Structure: "independent", StructureReviewed: true, Method: "one_way", Alpha: .05, PostHoc: "none"}
	for i, v := range []float64{1, 2, 3, 4, 5, 6} {
		imp := p.Import(fmt.Sprintf("synthetic-stat-%d.txt", i), []byte(fmt.Sprintf("Sample ID: invented-%d\nEffective Diameter (nm): %.3f\n", i, v)), false)
		if imp.Error != "" || imp.Measurements != 1 {
			t.Fatal("synthetic import failed", imp)
		}
		group := "A"
		if i >= 3 {
			group = "B"
		}
		f, _, err := p.File(imp.FileID)
		if err != nil {
			t.Fatal(err)
		}
		d.Observations = append(d.Observations, analysis.ObservationDefinition{MeasurementID: f.Measurements[0], FactorA: group})
	}
	return d
}

func TestAnalysisRepeatedUnitDesignAndWorkerPolicy(t *testing.T) {
	_, c, p := inspectionProfile(t)
	d := statisticalFixture(t, p)
	d.Method, d.Structure = "repeated", "repeated"
	units := []string{}
	for i := 0; i < 3; i++ {
		u, e := p.CreateExperimentalUnit(fmt.Sprintf("Physical unit %d", i))
		if e != nil {
			t.Fatal(e)
		}
		units = append(units, u.ID)
	}
	for i := range d.Observations {
		d.Observations[i].FactorA = "A"
		d.Observations[i].FactorB = fmt.Sprintf("t%d", i/3)
		d.Observations[i].UnitID = units[i%3]
	}
	s, err := p.PrepareAnalysis(d)
	if err != nil || !s.Design.CompleteRepeated || s.Design.ExperimentalUnits != 3 {
		t.Fatal("complete repeated design rejected", err)
	}
	d.Observations = d.Observations[:5]
	if _, err := p.PrepareAnalysis(d); !errors.Is(err, analysis.ErrDesign) {
		t.Fatal("incomplete repeated design silently fit", err)
	}
	for _, path := range []string{"/", "/statistics-engine/webr-worker.js", "/statistics-engine/no-packages/PACKAGES"} {
		res, err := c.hc.Get(c.base + path)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		policy := res.Header.Get("Content-Security-Policy")
		if path == "/" && strings.Contains(policy, "unsafe-eval") {
			t.Fatal("main UI evaluation policy weakened")
		}
		if strings.HasSuffix(path, "webr-worker.js") && (!strings.Contains(policy, "script-src 'self' 'unsafe-eval'") || !strings.Contains(policy, "connect-src 'self'")) {
			t.Fatal("worker policy missing local restriction")
		}
		if strings.Contains(path, "no-packages") && res.StatusCode != 404 {
			t.Fatal("missing runtime dependency fell back to HTML")
		}
	}
}

// Persistence tests exercise the browser result contract. Numerical validity
// has separate independent SciPy/statsmodels/Pingouin tests in web/tests.
func statisticalContract(s analysis.Snapshot) analysis.Results {
	ss, ess, f, p := 13.5, 4., 13.5, .021311641128756727
	ms := ess / 4
	r := analysis.Results{Engine: analysis.EngineVersion, Calculation: analysis.CalculationVersion, Method: s.Definition.Method, Terms: []analysis.Term{{Source: "A", SS: &ss, DF: 1, MS: &ss, F: &f, P: &p}, {Source: "residual", SS: &ess, DF: 4, MS: &ms}}, Residuals: []float64{-1, 0, 1, -1, 0, 1}, QQTheoretical: []float64{-1.282, -.643, -.202, .202, .643, 1.282}, QQObserved: []float64{-1, -1, 0, 0, 1, 1}, Groups: []analysis.Group{{FactorA: "A", N: 3, Values: []float64{1, 2, 3}, Mean: 2}, {FactorA: "B", N: 3, Values: []float64{4, 5, 6}, Mean: 5}}}
	return r
}

func TestDunnettExplicitControlAndSavedContract(t *testing.T) {
	_, _, p := inspectionProfile(t)
	d := statisticalFixture(t, p)
	d.PostHoc = "dunnett"
	if _, err := p.PrepareAnalysis(d); !errors.Is(err, analysis.ErrControl) {
		t.Fatal("implicit control accepted", err)
	}
	d.Control = "absent"
	if _, err := p.PrepareAnalysis(d); !errors.Is(err, analysis.ErrControl) {
		t.Fatal("unknown control accepted", err)
	}
	d.Control = "A"
	s, err := p.PrepareAnalysis(d)
	if err != nil {
		t.Fatal(err)
	}
	r := statisticalContract(s)
	r.Engine = analysis.ExpectedEngine(d)
	lower, upper, accuracy := .732, 5.268, 0.
	r.Comparisons = []analysis.Comparison{{ID: "comparison-1", LeftA: "A", RightA: "B", Contrast: "B-A", Difference: 3, Lower: &lower, Upper: &upper, AdjustedP: .021311641128756727, Correction: "Dunnett two-sided single-step (multivariate t)"}}
	r.Diagnostics = []analysis.Diagnostic{{Code: "dunnett_integration", Statistic: &accuracy}, {Code: "dunnett_quantile", Statistic: &accuracy}, {Code: "dunnett_confidence_integration", Statistic: &accuracy}}
	if _, err = p.SaveAnalysis(d, s.Receipt, r, ""); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []func(*analysis.Results){
		func(r *analysis.Results) { r.Engine = analysis.EngineVersion },
		func(r *analysis.Results) { r.Comparisons = nil },
		func(r *analysis.Results) { r.Comparisons[0].LeftA = "B"; r.Comparisons[0].RightA = "A" },
		func(r *analysis.Results) { r.Comparisons[0].Lower = nil },
		func(r *analysis.Results) { r.Diagnostics = nil },
	} {
		bad := r
		bad.Comparisons = append([]analysis.Comparison{}, r.Comparisons...)
		mutation(&bad)
		if _, err = p.SaveAnalysis(d, s.Receipt, bad, ""); err == nil {
			t.Fatal("unbound/incomplete Dunnett result accepted")
		}
	}
	d.Method = "welch"
	if _, err = p.PrepareAnalysis(d); !errors.Is(err, analysis.ErrDesign) {
		t.Fatal("Dunnett silently substituted for Welch", err)
	}
}

func TestAnalysisReviewPersistenceRevisionsAndScope(t *testing.T) {
	a, c, p := inspectionProfile(t)
	d := statisticalFixture(t, p)
	before := profileFingerprint(t, p)
	s, err := p.PrepareAnalysis(d)
	if err != nil || s.Receipt == "" || s.Design.N != 6 {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, profileFingerprint(t, p)) {
		t.Fatal("review persisted data")
	}
	result := statisticalContract(s)
	altered := result
	altered.Groups = append([]analysis.Group{}, result.Groups...)
	altered.Groups[0].Values = []float64{99, 2, 3}
	if _, err := p.SaveAnalysis(d, s.Receipt, altered, ""); !errors.Is(err, analysis.ErrSource) {
		t.Fatal("fabricated source group accepted", err)
	}
	saved, err := p.SaveAnalysis(d, s.Receipt, result, "")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Snapshot.Receipt != "" || saved.ResultOrigin != "local_bundled_browser_worker" {
		t.Fatal("misleading attestation or persisted receipt")
	}
	headers := map[string]string{"X-Account-ID": p.acct.ID(), "X-Profile-ID": p.Entry.ID}
	if code, _ := c.do("GET", "/api/statistics", nil, nil); code < 400 {
		t.Fatal("unscoped analysis list accepted")
	}
	var listed []analysis.StatisticalAnalysis
	code, b := c.do("GET", "/api/statistics", nil, headers)
	if code != 200 || json.Unmarshal(b, &listed) != nil || len(listed) != 1 {
		t.Fatal("scoped list failed")
	}
	label := "Changed source organization"
	if _, err := p.UpdateMeasurement(d.Observations[0].MeasurementID, MeasurementPatch{Label: &label}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.SaveAnalysis(d, s.Receipt, result, ""); !errors.Is(err, analysis.ErrSource) {
		t.Fatal("stale source receipt accepted", err)
	}
	got, err := p.Analysis(saved.ID)
	if err != nil || !got.SourceChanged || got.Snapshot.SourceHash != saved.Snapshot.SourceHash || !reflect.DeepEqual(got.Results, saved.Results) {
		t.Fatal("saved results mutated or source change hidden", err)
	}
	fresh, err := p.PrepareAnalysis(d)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := p.SaveAnalysis(d, fresh.Receipt, result, saved.ID)
	if err != nil || revision.ID == saved.ID || revision.PreviousID != saved.ID {
		t.Fatal("revision failed", err)
	}
	first := p.Entry.ID
	var other ProfileView
	c.json("POST", "/api/profiles", map[string]any{"username": "Separate statistics", "storageMode": "usb_only"}, &other)
	c.json("POST", "/api/profiles/"+other.ID+"/open", map[string]any{}, nil)
	if code, _ := c.do("GET", "/api/statistics/"+saved.ID, nil, headers); code < 400 {
		t.Fatal("stale scope read analysis")
	}
	headers["X-Profile-ID"] = other.ID
	if code, _ := c.do("GET", "/api/statistics/"+saved.ID, nil, headers); code < 400 {
		t.Fatal("cross-profile analysis leaked")
	}
	q, _ := a.Profile()
	empty, err := q.Analyses()
	if err != nil || len(empty) != 0 {
		t.Fatal("analysis collection leaked", err)
	}
	c.json("POST", "/api/profiles/"+first+"/open", map[string]any{}, nil)
	reopened, _ := a.Profile()
	all, err := reopened.Analyses()
	if err != nil || len(all) != 2 {
		t.Fatal("analyses lost on reopen", err)
	}
}

func TestAnalysisDesignRejectsAmbiguityAndPreservesMissing(t *testing.T) {
	_, _, p := inspectionProfile(t)
	d := statisticalFixture(t, p)
	d.StructureReviewed = false
	if _, err := p.PrepareAnalysis(d); !errors.Is(err, analysis.ErrStructure) {
		t.Fatal(err)
	}
	d.StructureReviewed = true
	unit, err := p.CreateExperimentalUnit("Synthetic physical unit")
	if err != nil {
		t.Fatal(err)
	}
	d.Observations[0].UnitID = unit.ID
	d.Observations[1].UnitID = unit.ID
	if _, err := p.PrepareAnalysis(d); !errors.Is(err, analysis.ErrStructure) {
		t.Fatal("pseudoreplication accepted", err)
	}
	d.Observations[1].UnitID = ""
	d.Observations[0].UnitID = ""
	for _, method := range []string{"dunnett"} {
		d.Method = method
		if _, err := p.PrepareAnalysis(d); !errors.Is(err, analysis.ErrMethod) {
			t.Fatal("unavailable method accepted", err)
		}
	}
	d.Method = "one_way"
	d.Module = "zeta"
	if _, err := p.PrepareAnalysis(d); !errors.Is(err, analysis.ErrDefinition) {
		t.Fatal("cross-module analysis accepted", err)
	}
	d.Module = "lightscattering"
	d.Observations[0].FactorA = "A\x00B"
	if _, err := p.PrepareAnalysis(d); !errors.Is(err, analysis.ErrDefinition) {
		t.Fatal("control label accepted", err)
	}
	d.Observations[0].FactorA = "A"
	id := d.Observations[0].MeasurementID
	if err := p.St.Update(func(tx *store.Tx) error {
		var m model.Measurement
		if _, e := tx.Get(CollMeasurements, id, &m); e != nil {
			return e
		}
		delete(m.Params, model.EffectiveDiameter)
		_, e := tx.Put(CollMeasurements, id, m)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	s, err := p.PrepareAnalysis(d)
	if err != nil || !s.Observations[0].Missing || s.Observations[0].Quantity != nil || s.Design.N != 5 || s.Design.Missing != 1 {
		t.Fatal("missing became zero", err)
	}
	d.Observations[0].ExcludeReason = "Predefined instrument failure"
	s, err = p.PrepareAnalysis(d)
	if err != nil || s.Design.Excluded != 1 || s.Design.Missing != 0 {
		t.Fatal("explicit exclusion lost", err)
	}
}

func TestAnalysisReceiptsExpiryUnitsAndSavedGraphExport(t *testing.T) {
	_, _, p := inspectionProfile(t)
	d := statisticalFixture(t, p)
	s, err := p.PrepareAnalysis(d)
	if err != nil {
		t.Fatal(err)
	}
	var claim analysisClaim
	data, _ := base64.RawURLEncoding.DecodeString(strings.Split(s.Receipt, ".")[0])
	json.Unmarshal(data, &claim)
	claim.Expires = time.Now().Add(-time.Second).Unix()
	data, _ = json.Marshal(claim)
	expired := base64.RawURLEncoding.EncodeToString(data) + "." + base64.RawURLEncoding.EncodeToString(p.analysisMAC(data))
	if _, err := p.SaveAnalysis(d, expired, statisticalContract(s), ""); !errors.Is(err, ErrImportReview) {
		t.Fatal("expired receipt accepted", err)
	}
	saved, err := p.SaveAnalysis(d, s.Receipt, statisticalContract(s), "")
	if err != nil {
		t.Fatal(err)
	}
	g, err := p.SaveGraph(graph.Definition{Kind: graph.KindStatistical, AnalysisID: saved.ID, ErrorBars: "sd", Title: "Saved scientific groups"})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := p.Compute(g)
	if err != nil || len(rendered.X.Categories) != 2 || len(rendered.Series) != 4 || rendered.Provenance.ComputedAt != saved.Created {
		t.Fatal("graph did not preserve immutable result", err)
	}
	outs, _, err := p.analysisOutputs(saved.ID, ExportRequest{Formats: []string{"package", "csv", "json", "pdf"}}, plot.Preset(plot.PresetScreen), false)
	if err != nil {
		t.Fatal(err)
	}
	directories := map[string]bool{}
	csv := 0
	for _, out := range outs {
		if strings.HasSuffix(out.name, ".csv") {
			csv++
		}
		if strings.HasSuffix(out.name, ".pdf") && !strings.HasPrefix(string(out.data), "%PDF-") {
			t.Fatal("invalid PDF")
		}
		if strings.HasPrefix(out.name, "statistics/") {
			directories[strings.Split(out.name, "/")[1]] = true
		}
		if strings.HasSuffix(out.name, ".json") && (strings.Contains(string(out.data), p.acct.ID()) || strings.Contains(string(out.data), p.Entry.ID)) {
			t.Fatal("account scope leaked into scientific export")
		}
	}
	if len(directories) != 8 || csv != 8 {
		t.Fatal("incomplete statistical export", len(directories), csv)
	}
	ds := export.StatisticalData(saved)
	if len(ds.Tables) != 8 {
		t.Fatal("missing tables")
	}
	id := d.Observations[1].MeasurementID
	err = p.St.Update(func(tx *store.Tx) error {
		var m model.Measurement
		if _, e := tx.Get(CollMeasurements, id, &m); e != nil {
			return e
		}
		q := m.Params[d.Variable]
		q.Unit = "um"
		m.Params[d.Variable] = q
		_, e := tx.Put(CollMeasurements, id, m)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.PrepareAnalysis(d); !errors.Is(err, analysis.ErrUnits) {
		t.Fatal("incompatible units combined", err)
	}
	still, err := p.Compute(g)
	if err != nil || !reflect.DeepEqual(still.Series, rendered.Series) {
		t.Fatal("saved graph changed with source", err)
	}
}

func TestAnalysisComparisonBracketsAndCyclePackage(t *testing.T) {
	a, _, p := inspectionProfile(t)
	d := statisticalFixture(t, p)
	ids := []string{}
	for _, o := range d.Observations {
		ids = append(ids, o.MeasurementID)
	}
	cy, err := p.SaveCycle(CycleDoc{Config: cycle.Config{Name: "Synthetic statistics cycle", Start: time.Now().UTC(), Interval: 1, Unit: cycle.Days, Duration: 1, Params: []string{model.EffectiveDiameter}}, Measurements: ids})
	if err != nil {
		t.Fatal(err)
	}
	d.CycleID, d.PostHoc = cy.ID, "tukey"
	s, err := p.PrepareAnalysis(d)
	if err != nil {
		t.Fatal(err)
	}
	r := statisticalContract(s)
	r.Comparisons = []analysis.Comparison{{ID: "comparison-1", LeftA: "A", RightA: "B", Contrast: "B-A", AdjustedP: .021311641128756727, Correction: "Tukey HSD", Difference: 3}}
	saved, err := p.SaveAnalysis(d, s.Receipt, r, "")
	if err != nil {
		t.Fatal(err)
	}
	g := graph.Definition{Kind: graph.KindStatistical, AnalysisID: saved.ID, ErrorBars: "ci", Annotations: []string{"comparison-1"}, AnnotationStyle: "exact", Visual: graph.DefaultVisual()}
	g, err = p.SaveGraph(g)
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Compute(g)
	if err != nil || len(res.Annotations) != 1 || res.Annotations[0].X1 != 1 || res.Annotations[0].X2 != 2 {
		t.Fatal("bracket not bound to explicit pair", err, res.Annotations)
	}
	spec := plot.Preset(plot.PresetScreen)
	fig := plot.Layout(res, spec, a.T)
	if !strings.Contains(string(plot.SVG(fig, spec)), "p(adj)=0.0213116") {
		t.Fatal("SVG omitted adjusted comparison")
	}
	pdf, err := plot.PDF(fig, spec, saved.Created)
	if err != nil || !strings.HasPrefix(string(pdf), "%PDF-") {
		t.Fatal("annotated native PDF invalid", err)
	}
	bad := g
	bad.Annotations = []string{"unknown"}
	if _, err := p.SaveGraph(bad); !errors.Is(err, analysis.ErrDefinition) {
		t.Fatal("unknown bracket accepted", err)
	}
	bad.Annotations = []string{"comparison-1", "comparison-1"}
	if _, err := p.SaveGraph(bad); !errors.Is(err, analysis.ErrDefinition) {
		t.Fatal("duplicated bracket accepted", err)
	}
	g.AnnotationStyle = "stars"
	stars, err := p.Compute(g)
	if err != nil || stars.Annotations[0].Label != "*" {
		t.Fatal("incorrect adjusted significance stars", err)
	}
	spec.Metadata = false
	starSVG := string(plot.SVG(plot.Layout(stars, spec, a.T), spec))
	if !strings.Contains(starSVG, "Tukey HSD") || !strings.Contains(starSVG, "0.0001") {
		t.Fatal("exported stars lost correction/threshold note when metadata hidden")
	}
	outs, _, err := p.analysisOutputs(saved.ID, ExportRequest{Formats: []string{"package"}}, spec, false)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, out := range outs {
		if strings.HasSuffix(out.name, "annotations.json") {
			found = strings.Contains(string(out.data), g.ID) && strings.Contains(string(out.data), "comparison-1")
		}
	}
	if !found {
		t.Fatal("selected graph annotations absent from package")
	}
	outs, _, err = p.cycleOutputs(cy.ID, ExportRequest{Formats: []string{"package"}}, spec, false)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, out := range outs {
		if strings.HasPrefix(out.name, "statistics/analyses/"+saved.ID+"/") {
			count++
		}
	}
	if count < 11 {
		t.Fatal("cycle package omitted saved analysis", count)
	}
	relations, err := p.Relations()
	if err != nil {
		t.Fatal(err)
	}
	if len(relations.Cycles[cy.ID].Analyses) != 1 || len(relations.Files[s.Observations[0].FileID].Analyses) != 1 {
		t.Fatal("analysis source dependencies missing")
	}
}

func TestMixedExplicitUnitsIncompleteDesignAndSavedContract(t *testing.T) {
	_, _, p := inspectionProfile(t)
	d := statisticalFixture(t, p)
	d.Method, d.Structure = "mixed", "repeated"
	// Add a third time point; enough residual DF after one missing observation.
	for i := 0; i < 3; i++ {
		imp := p.Import(fmt.Sprintf("mixed-time3-%d.txt", i), []byte(fmt.Sprintf("Sample ID: invented-mixed-%d\nEffective Diameter (nm): %d\n", i, i+7)), false)
		f, _, err := p.File(imp.FileID)
		if err != nil {
			t.Fatal(err)
		}
		d.Observations = append(d.Observations, analysis.ObservationDefinition{MeasurementID: f.Measurements[0]})
	}
	units := []string{}
	for i := 0; i < 3; i++ {
		u, e := p.CreateExperimentalUnit(fmt.Sprintf("Mixed physical unit %d", i))
		if e != nil {
			t.Fatal(e)
		}
		units = append(units, u.ID)
	}
	for i := range d.Observations {
		d.Observations[i].FactorA = "A"
		d.Observations[i].FactorB = fmt.Sprintf("t%d", i/3)
		d.Observations[i].UnitID = units[i%3]
	}
	d.Observations = d.Observations[:8]
	s, err := p.PrepareAnalysis(d)
	if err != nil || s.Design.CompleteRepeated || s.Design.N != 8 || s.Design.Recommended != "mixed" {
		t.Fatal("incomplete explicit Mixed rejected", err, s.Design)
	}
	// Contract values here are deliberately synthetic; independent actual
	// webR numerical oracles test scientific inference, not this persistence test.
	r := analysis.Results{Engine: analysis.ExpectedEngine(d), Calculation: analysis.CalculationVersion, Method: "mixed", SSType: "not_applicable_marginal_Wald_F", Residuals: make([]float64, 8), QQObserved: make([]float64, 8), QQTheoretical: make([]float64, 8)}
	df, f, pval := 3., 2., .2
	r.Terms = []analysis.Term{{Source: "B", DF: 2, DenominatorDF: &df, F: &f, P: &pval}}
	r.Model = &analysis.MixedModel{Family: "random_intercept", Fixed: "B", Estimation: "ML", Random: "1|unit", ResidualCovariance: "homoscedastic conditional errors", Test: "marginal Wald F; sum contrasts; adjustSigma=TRUE", LevelsA: s.Design.LevelsA, LevelsB: s.Design.LevelsB, RandomVariance: 1, ResidualVariance: 2, LogLikelihood: -10, BoundaryTolerance: 1e-4, FixedCoefficients: []analysis.FixedCoefficient{{Name: "(Intercept)", Estimate: 1, SE: 1}, {Name: "B1", Estimate: 2, SE: 1}, {Name: "B2", Estimate: 3, SE: 1}}}
	r.Model.CoefficientConfidenceLevel = 1 - d.Alpha
	r.Model.CoefficientIntervalMethod = "nlme::intervals.lme fixed; Student t; conditional GLS; approximate; individual"
	for i := range r.Model.FixedCoefficients {
		v := &r.Model.FixedCoefficients[i]
		low, up, df := v.Estimate-1, v.Estimate+1, 3.
		v.Lower = &low
		v.Upper = &up
		v.DF = &df
	}
	groups := map[string]int{}
	for _, o := range s.Observations {
		k := o.FactorB
		i, ok := groups[k]
		if !ok {
			i = len(r.Groups)
			groups[k] = i
			r.Groups = append(r.Groups, analysis.Group{FactorA: o.FactorA, FactorB: k})
		}
		r.Groups[i].Values = append(r.Groups[i].Values, o.Quantity.Value)
		r.Groups[i].N++
	}
	saved, e := p.SaveAnalysis(d, s.Receipt, r, "")
	if e != nil {
		t.Fatal(e)
	}
	restored, e := p.Analysis(saved.ID)
	if e != nil || !reflect.DeepEqual(restored.Results.Model, r.Model) {
		t.Fatal("model lost", e)
	}
	data := export.StatisticalData(saved)
	if len(data.Tables) != 7 || data.Tables[0].Columns[2].Key != "denominator_df" || data.Tables[5].ID != "mixed_model" || data.Tables[6].ID != "coefficients" {
		t.Fatal("mixed export lost model or DF", data.Tables)
	}
	if len(data.Tables[6].Columns) != 6 || data.Tables[6].Columns[3].Key != "coefficient_df" {
		t.Fatal("coefficient CI export fields lost")
	}
	rawLegacy, _ := json.Marshal(saved)
	var legacy analysis.StatisticalAnalysis
	json.Unmarshal(rawLegacy, &legacy)
	legacy.Results.Engine = "webR/0.6.0; R/4.6.0; nlme/3.1-169; mixed-random-intercept/1"
	legacy.Results.Model.CoefficientConfidenceLevel = 0
	legacy.Results.Model.CoefficientIntervalMethod = ""
	for i := range legacy.Results.Model.FixedCoefficients {
		v := &legacy.Results.Model.FixedCoefficients[i]
		v.DF = nil
		v.Lower = nil
		v.Upper = nil
	}
	historical := export.StatisticalData(legacy)
	if len(historical.Tables[6].Columns) != 3 {
		t.Fatal("historical coefficient CIs manufactured")
	}
	oldGraph, e := graph.StatisticalGroups(graph.Definition{Kind: graph.KindStatistical, AnalysisID: legacy.ID, ErrorBars: "sd"}, legacy)
	if e != nil || strings.Contains(strings.Join(oldGraph.Provenance.Transformations, " "), "fixed coefficient intervals") {
		t.Fatal("historical graph interval provenance fabricated", e)
	}
	outs, _, e := p.analysisOutputs(saved.ID, ExportRequest{Formats: []string{"package", "csv", "json", "pdf", "xlsx"}}, plot.Preset(plot.PresetScreen), false)
	if e != nil {
		t.Fatal(e)
	}
	csv, modelFile := 0, false
	for _, o := range outs {
		if strings.HasSuffix(o.name, ".csv") {
			csv++
		}
		if o.name == "statistics/analysis-definitions/mixed-model.json" {
			modelFile = strings.Contains(string(o.data), "randomVariance") && strings.Contains(string(o.data), "fixedCoefficients")
		}
		if o.name == "statistics/effect-sizes/effects.json" && strings.TrimSpace(string(o.data)) != "[]" {
			t.Fatal("Mixed exported fake effect sizes")
		}
		if strings.HasSuffix(o.name, ".json") && (strings.Contains(string(o.data), p.acct.ID()) || strings.Contains(string(o.data), p.Entry.ID)) {
			t.Fatal("local scope leaked")
		}
	}
	if csv != 7 || !modelFile {
		t.Fatal("Mixed exports lost tables/model", csv, modelFile)
	}
	g, e := p.SaveGraph(graph.Definition{Kind: graph.KindStatistical, AnalysisID: saved.ID, ErrorBars: "sd", Title: "Synthetic Mixed raw groups"})
	if e != nil {
		t.Fatal(e)
	}
	fig, e := p.Compute(g)
	if e != nil || fig.Provenance.Engine != r.Engine || !strings.Contains(strings.Join(fig.Provenance.Transformations, " "), "not fitted mixed-model means") {
		t.Fatal("Mixed graph provenance lost", e)
	}
	for _, mutate := range []func(*analysis.Results){
		func(v *analysis.Results) { v.Model = nil },
		func(v *analysis.Results) { v.Model.FixedCoefficients[0].DF = nil },
		func(v *analysis.Results) { v.Model.FixedCoefficients[0].Upper = nil },
		func(v *analysis.Results) { v.Model.CoefficientConfidenceLevel = .9 },

		func(v *analysis.Results) { v.Terms[0].DenominatorDF = nil },
		func(v *analysis.Results) { x := 4.; v.Terms[0].DenominatorDF = &x },
		func(v *analysis.Results) { v.Model.LevelsB = []string{"wrong", "t1", "t2"} },
		func(v *analysis.Results) { v.Model.FixedCoefficients[1].Name = "(Intercept)" },
		func(v *analysis.Results) { v.Model.RandomVariance = 1e-12 },
		func(v *analysis.Results) { v.Terms[0].SS = &f },
		func(v *analysis.Results) { v.Corrections = map[string]float64{"GG": .5} },
	} {
		raw, _ := json.Marshal(r)
		var bad analysis.Results
		json.Unmarshal(raw, &bad)
		mutate(&bad)
		if validateAnalysisResults(s, bad) == nil {
			t.Fatal("unsupported model contract accepted", bad)
		}
	}
	d.Method = "repeated"
	if _, e := p.PrepareAnalysis(d); !errors.Is(e, analysis.ErrDesign) {
		t.Fatal("classical RM accepted incomplete units", e)
	}
	d.Method = "mixed"
	d.SphericityCorrection = "GG"
	if _, e := p.PrepareAnalysis(d); !errors.Is(e, analysis.ErrDesign) {
		t.Fatal("Mixed accepted GG", e)
	}
	d.SphericityCorrection = ""
	d.Structure = "independent"
	if _, e := p.PrepareAnalysis(d); !errors.Is(e, analysis.ErrDesign) {
		t.Fatal("Mixed without repeated review accepted", e)
	}
	d.Structure = "repeated"
	d.Observations[0].UnitID = "unknown"
	if _, e := p.PrepareAnalysis(d); e == nil {
		t.Fatal("unknown physical unit accepted")
	}
}

func TestEffectIntervalsScopedPersistenceExportsAndRejections(t *testing.T) {
	_, _, p := inspectionProfile(t)
	d := statisticalFixture(t, p)
	d.EffectCI = true
	s, e := p.PrepareAnalysis(d)
	if e != nil {
		t.Fatal(e)
	}
	r := statisticalContract(s)
	r.Engine = analysis.ExpectedEngine(d)
	low, up, precision, coverage := .017419935275215046, .8833409901892689, 1e-10, .95
	r.EffectIntervals = []analysis.EffectInterval{{Source: "A", Effect: "population_eta_squared", ConfidenceLevel: 1 - d.Alpha, Method: "MBESS 5.0.1 ci.pvaf / conf.limits.ncf", Status: "available", Lower: &low, Upper: &up}}
	r.Diagnostics = []analysis.Diagnostic{{Code: "effect_ci_precision", Statistic: &precision}, {Code: "effect_ci_tail_coverage", Statistic: &coverage}}
	saved, e := p.SaveAnalysis(d, s.Receipt, r, "")
	if e != nil {
		t.Fatal(e)
	}
	restored, e := p.Analysis(saved.ID)
	if e != nil || !reflect.DeepEqual(restored.Results.EffectIntervals, r.EffectIntervals) {
		t.Fatal("effect interval lost", e)
	}
	outs, _, e := p.analysisOutputs(saved.ID, ExportRequest{Formats: []string{"package", "csv", "xlsx", "pdf", "json"}}, plot.Preset(plot.PresetScreen), false)
	if e != nil {
		t.Fatal(e)
	}
	csv, ciFile := 0, false
	for _, o := range outs {
		if strings.HasSuffix(o.name, ".csv") {
			csv++
		}
		if o.name == "statistics/effect-sizes/intervals.json" {
			ciFile = strings.Contains(string(o.data), "population_eta_squared")
		}
	}
	if csv != 9 || !ciFile {
		t.Fatal("effect interval dropped from exports", csv, ciFile)
	}
	for _, change := range []func(*analysis.Results){
		func(v *analysis.Results) { v.EffectIntervals = nil },
		func(v *analysis.Results) { v.EffectIntervals[0].Effect = "partial_eta_squared" },
		func(v *analysis.Results) { v.EffectIntervals[0].Upper = nil },
		func(v *analysis.Results) { v.EffectIntervals[0].ConfidenceLevel = .9 },
		func(v *analysis.Results) { v.EffectIntervals[0].LowerAtBoundary = true },
		func(v *analysis.Results) { v.Diagnostics = nil },
		func(v *analysis.Results) { v.EffectIntervals[0].Status = "not_estimable" },
	} {
		raw, _ := json.Marshal(r)
		var bad analysis.Results
		json.Unmarshal(raw, &bad)
		change(&bad)
		if validateAnalysisResults(s, bad) == nil {
			t.Fatal("invalid effect interval contract accepted")
		}
	}
	// An unavailable upper bound is honest and leaves the supported ANOVA intact.
	unavailable := r
	unavailable.EffectIntervals = []analysis.EffectInterval{{Source: "A", Effect: "population_eta_squared", ConfidenceLevel: 1 - d.Alpha, Method: "MBESS 5.0.1 ci.pvaf / conf.limits.ncf", Status: "not_estimable"}}
	unavailable.Diagnostics = nil
	unavailable.Warnings = []string{"statistics.effect_ci_not_estimable"}
	if validateAnalysisResults(s, unavailable) != nil {
		t.Fatal("honest unavailable CI rejected")
	}
	changed := d
	changed.EffectCI = false
	if _, e := p.SaveAnalysis(changed, s.Receipt, r, ""); !errors.Is(e, analysis.ErrSource) {
		t.Fatal("CI option outside reviewed receipt accepted", e)
	}
	changed.Method = "welch"
	changed.EffectCI = true
	if _, e := p.PrepareAnalysis(changed); !errors.Is(e, analysis.ErrDesign) {
		t.Fatal("CI silently applied to Welch", e)
	}
}
