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
	for _, method := range []string{"mixed", "dunnett"} {
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
