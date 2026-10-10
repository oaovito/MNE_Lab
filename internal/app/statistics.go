package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
	"github.com/oaovito/mne_lab/internal/version"
)

func (p *Profile) ExperimentalUnits() ([]analysis.ExperimentalUnit, error) {
	units := []analysis.ExperimentalUnit{}
	err := p.St.View(func(tx *store.Tx) error {
		rows, err := tx.List(CollUnits)
		if err != nil {
			return err
		}
		for _, row := range rows {
			var unit analysis.ExperimentalUnit
			if err := decodeRecord(row, &unit); err != nil {
				return err
			}
			units = append(units, unit)
		}
		return nil
	})
	sort.Slice(units, func(i, j int) bool { return units[i].Label < units[j].Label })
	return units, err
}

func (p *Profile) CreateExperimentalUnit(label string) (analysis.ExperimentalUnit, error) {
	label = strings.TrimSpace(label)
	if label == "" || !analysis.ValidText(label, 120) {
		return analysis.ExperimentalUnit{}, analysis.ErrDefinition
	}
	unit := analysis.ExperimentalUnit{ID: secure.NewID(), Label: label, Created: time.Now().UTC()}
	done := p.app.begin("statistical-analysis")
	defer done()
	err := p.St.Update(func(tx *store.Tx) error {
		if p.closed.Load() || p.app.exiting.Load() {
			return ErrImportScope
		}
		_, err := tx.Put(CollUnits, unit.ID, unit)
		return err
	})
	return unit, err
}

func sourceHash(snapshot analysis.Snapshot) (string, error) {
	snapshot.Receipt, snapshot.SourceHash, snapshot.AccountID, snapshot.ProfileID = "", "", "", ""
	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return secure.HashHex(data), nil
}

func (p *Profile) analysisSnapshot(def analysis.Definition) (analysis.Snapshot, error) {
	var out analysis.Snapshot
	err := p.St.View(func(tx *store.Tx) error { var err error; out, err = p.analysisSnapshotTx(tx, def); return err })
	return out, err
}

func (p *Profile) analysisSnapshotTx(tx *store.Tx, def analysis.Definition) (analysis.Snapshot, error) {
	if err := def.Validate(); err != nil {
		return analysis.Snapshot{}, err
	}
	if p.closed.Load() || p.app.exiting.Load() {
		return analysis.Snapshot{}, ErrImportScope
	}
	out := analysis.Snapshot{Definition: def, Observations: []analysis.Observation{}, Sources: []analysis.Source{}, AccountID: p.acct.ID(), ProfileID: p.Entry.ID}
	unitSeen := false
	err := func() error {
		allowed := map[string]bool{}
		if def.CycleID != "" {
			var cy CycleDoc
			if _, err := tx.Get(CollCycles, def.CycleID, &cy); err != nil {
				return err
			}
			if cy.Trashed {
				return analysis.ErrSource
			}
			for _, id := range cy.Measurements {
				allowed[id] = true
			}
			out.CycleSource, _ = json.Marshal(cy)
		}
		for _, row := range def.Observations {
			if def.CycleID != "" && !allowed[row.MeasurementID] {
				return analysis.ErrSource
			}
			var m model.Measurement
			var f model.SourceFile
			if _, err := tx.Get(CollMeasurements, row.MeasurementID, &m); err != nil {
				return err
			}
			if _, err := tx.Get(CollFiles, m.FileID, &f); err != nil {
				return err
			}
			if f.Trashed {
				return analysis.ErrSource
			}
			if f.Module != def.Module {
				return analysis.ErrDefinition
			}
			if row.UnitID != "" {
				if _, err := tx.GetRecord(CollUnits, row.UnitID); err != nil {
					return err
				}
			}
			o := analysis.Observation{ObservationDefinition: row, FileID: m.FileID, SampleID: m.SampleID, Replicate: m.Replicate}
			q, ok := m.Params[def.Variable]
			o.Missing = !ok
			if ok {
				if math.IsNaN(q.Value) || math.IsInf(q.Value, 0) {
					return analysis.ErrNumeric
				}
				o.Quantity = &q
				if row.ExcludeReason == "" {
					if unitSeen && out.Unit != q.Unit {
						return analysis.ErrUnits
					}
					out.Unit = q.Unit
					unitSeen = true
				}
			}
			// Only the dependent quantity and its semantic identity enter this
			// source snapshot. Curves are neither analysed nor copied as bins.
			m.Params = map[string]model.Quantity{}
			if ok {
				m.Params[def.Variable] = q
			}
			m.Fields, m.Dist, m.Distributions = nil, nil, nil
			// This snapshot already names its selected measurements. Repeating
			// the entire file's membership for every row would grow quadratically.
			f.Measurements = nil
			out.Observations = append(out.Observations, o)
			out.Sources = append(out.Sources, analysis.Source{Measurement: m, File: f})
		}
		return nil
	}()
	if err != nil {
		return analysis.Snapshot{}, err
	}
	design, err := detectAnalysisDesign(out)
	if err != nil {
		return analysis.Snapshot{}, err
	}
	out.Design = design
	out.SourceHash, err = sourceHash(out)
	return out, err
}

func detectAnalysisDesign(snapshot analysis.Snapshot) (analysis.Design, error) {
	d := analysis.Design{Balanced: true, CompleteRepeated: true, Warnings: []string{}}
	a, b, units := map[string]int{}, map[string]int{}, map[string]bool{}
	cells, unitTime, unitGroup := map[string]int{}, map[string]bool{}, map[string]string{}
	def := snapshot.Definition
	for _, o := range snapshot.Observations {
		if o.ExcludeReason != "" {
			d.Excluded++
			continue
		}
		if o.Missing {
			d.Missing++
			continue
		}
		d.N++
		a[o.FactorA]++
		b[o.FactorB]++
		cells[o.FactorA+"\x00"+o.FactorB]++
		if o.UnitID != "" {
			if def.Structure == "independent" && units[o.UnitID] {
				return d, analysis.ErrStructure
			}
			units[o.UnitID] = true
			key := o.UnitID + "\x00" + o.FactorB
			if unitTime[key] && def.Structure == "repeated" {
				return d, analysis.ErrDesign
			}
			unitTime[key] = true
			if old, ok := unitGroup[o.UnitID]; ok && old != o.FactorA {
				return d, analysis.ErrDesign
			}
			unitGroup[o.UnitID] = o.FactorA
		} else if def.Structure == "repeated" {
			return d, analysis.ErrStructure
		}
	}
	for name := range a {
		d.LevelsA = append(d.LevelsA, name)
	}
	for name := range b {
		d.LevelsB = append(d.LevelsB, name)
	}
	sort.Strings(d.LevelsA)
	sort.Strings(d.LevelsB)
	if def.PostHoc == "dunnett" {
		if a[def.Control] == 0 {
			return d, analysis.ErrControl
		}
		if len(a) > 9 {
			return d, analysis.ErrFamily
		}
	}
	d.ExperimentalUnits = len(units)
	if len(cells) > 1000 {
		return d, analysis.ErrDefinition
	}
	count := 0
	for _, n := range cells {
		if count == 0 {
			count = n
		}
		if count != n {
			d.Balanced = false
		}
	}
	for _, unit := range func() []string {
		v := []string{}
		for id := range units {
			v = append(v, id)
		}
		return v
	}() {
		for _, level := range d.LevelsB {
			if !unitTime[unit+"\x00"+level] {
				d.CompleteRepeated = false
			}
		}
	}
	if d.N < 3 {
		return d, analysis.ErrDesign
	}
	if d.Missing > 0 {
		d.Warnings = append(d.Warnings, "statistics.missing_not_zero")
	}
	if d.ExperimentalUnits < d.N && def.Structure == "independent" {
		d.Warnings = append(d.Warnings, "statistics.unidentified_units_reviewed")
	}
	if snapshot.Unit == "" {
		d.Warnings = append(d.Warnings, "statistics.units_absent_not_inferred")
	}
	if def.Structure == "repeated" {
		d.Recommended = "repeated"
		if (!d.CompleteRepeated || d.Missing > 0) && def.Method != "mixed" {
			d.Recommended = "mixed"
			d.Warnings = append(d.Warnings, "statistics.mixed_selection_required")
			return d, analysis.ErrDesign
		}
		if len(d.LevelsB) < 2 || d.ExperimentalUnits < 2 {
			return d, analysis.ErrDesign
		}
		// The supported classical RM implementation requires complete,
		// balanced between-subject groups. Other structures belong to the
		// separately validated mixed-effects engine, not a silent fallback.
		if (def.Method != "mixed" && !d.Balanced) || d.ExperimentalUnits <= len(a) {
			return d, analysis.ErrDesign
		}
		if len(a) > 1 {
			counts := map[string]int{}
			for _, group := range unitGroup {
				counts[group]++
			}
			for _, n := range counts {
				if n < 2 {
					return d, analysis.ErrDesign
				}
			}
		}
		if def.Method == "mixed" {
			counts := map[string]int{}
			for _, o := range snapshot.Observations {
				if !o.Missing && o.ExcludeReason == "" {
					counts[o.UnitID]++
				}
			}
			for _, n := range counts {
				if n < 2 {
					return d, analysis.ErrDesign
				}
			}
			d.Recommended = "mixed"
			if d.ExperimentalUnits < 3 || len(cells) != len(a)*len(b) || d.N-d.ExperimentalUnits-len(a)*(len(b)-1) <= 0 {
				return d, analysis.ErrDesign
			}
		}
	} else if len(d.LevelsB) > 1 && len(d.LevelsA) > 1 {
		d.Recommended = "two_way"
	} else {
		d.Recommended = "one_way"
	}
	if def.Method == "one_way" || def.Method == "welch" {
		if len(a) < 2 {
			return d, analysis.ErrDesign
		}
		for _, n := range a {
			if n < 2 {
				return d, analysis.ErrDesign
			}
		}
	}
	if def.Method == "two_way" {
		if len(a) < 2 || len(b) < 2 || len(cells) != len(a)*len(b) || d.N <= len(cells) {
			return d, analysis.ErrDesign
		}
	}
	return d, nil
}

type analysisClaim struct {
	Account string `json:"account"`
	Profile string `json:"profile"`
	Hash    string `json:"hash"`
	Expires int64  `json:"expires"`
}

func (p *Profile) analysisMAC(data []byte) []byte {
	mac := hmac.New(sha256.New, []byte(p.app.importSecret))
	mac.Write([]byte("mnelab.statistics/1\x00"))
	mac.Write(data)
	return mac.Sum(nil)
}
func (p *Profile) PrepareAnalysis(def analysis.Definition) (analysis.Snapshot, error) {
	out, err := p.analysisSnapshot(def)
	if err != nil {
		return out, err
	}
	claim := analysisClaim{Account: out.AccountID, Profile: out.ProfileID, Hash: out.SourceHash, Expires: time.Now().Add(30 * time.Minute).Unix()}
	data, _ := json.Marshal(claim)
	out.Receipt = base64.RawURLEncoding.EncodeToString(data) + "." + base64.RawURLEncoding.EncodeToString(p.analysisMAC(data))
	return out, nil
}

func validateAnalysisResults(snapshot analysis.Snapshot, result analysis.Results) error {
	minimumTerms := 2
	if snapshot.Definition.Method == "mixed" {
		minimumTerms = 1
	}
	if result.Engine != analysis.ExpectedEngine(snapshot.Definition) || result.Calculation != analysis.CalculationVersion || result.Method != snapshot.Definition.Method || len(result.Terms) < minimumTerms || len(result.Terms) > 40 || len(result.Groups) > 1000 || len(result.Comparisons) > 10000 || len(result.Residuals) != snapshot.Design.N || len(result.QQObserved) != len(result.Residuals) || len(result.QQTheoretical) != len(result.Residuals) {
		return analysis.ErrDefinition
	}
	if snapshot.Definition.Method == "mixed" {
		m := result.Model
		if result.SSType != "not_applicable_marginal_Wald_F" {
			return analysis.ErrDefinition
		}
		if m == nil || m.Family != "random_intercept" || m.Estimation != "ML" || m.Random != "1|unit" || m.ResidualCovariance != "homoscedastic conditional errors" || m.Test != "marginal Wald F; sum contrasts; adjustSigma=TRUE" || m.BoundaryTolerance != 1e-4 || !(m.RandomVariance > 0) || !(m.ResidualVariance > 0) || math.Sqrt(m.RandomVariance/m.ResidualVariance) < m.BoundaryTolerance || len(m.FixedCoefficients) != len(snapshot.Design.LevelsA)*len(snapshot.Design.LevelsB) || len(result.Comparisons) != 0 || len(result.Corrections) != 0 {
			return analysis.ErrDefinition
		}
		if len(snapshot.Design.LevelsA) > 1 && m.Fixed != "A*B" || len(snapshot.Design.LevelsA) == 1 && m.Fixed != "B" {
			return analysis.ErrDefinition
		}
		for _, pair := range [][2][]string{{m.LevelsA, snapshot.Design.LevelsA}, {m.LevelsB, snapshot.Design.LevelsB}} {
			got := append([]string{}, pair[0]...)
			sort.Strings(got)
			if len(got) != len(pair[1]) {
				return analysis.ErrDefinition
			}
			for i, v := range got {
				if v != pair[1][i] {
					return analysis.ErrDefinition
				}
			}
		}
		wantCoefficients := map[string]bool{"(Intercept)": true}
		for b := 1; b < len(m.LevelsB); b++ {
			wantCoefficients[fmt.Sprintf("B%d", b)] = true
		}
		for a := 1; a < len(m.LevelsA); a++ {
			wantCoefficients[fmt.Sprintf("A%d", a)] = true
			for b := 1; b < len(m.LevelsB); b++ {
				wantCoefficients[fmt.Sprintf("A%d:B%d", a, b)] = true
			}
		}
		for _, v := range m.FixedCoefficients {
			if !wantCoefficients[v.Name] || !(v.SE > 0) {
				return analysis.ErrDefinition
			}
			delete(wantCoefficients, v.Name)
		}
		if len(wantCoefficients) != 0 {
			return analysis.ErrDefinition
		}
		wantTerms := map[string]bool{"B": true}
		if len(snapshot.Design.LevelsA) > 1 {
			wantTerms["A"] = true
			wantTerms["A:B"] = true
		}
		for _, t := range result.Terms {
			if !wantTerms[t.Source] || t.DenominatorDF == nil || !(*t.DenominatorDF > 0) || t.F == nil || t.P == nil || t.SS != nil || t.MS != nil || t.EtaSquared != nil || t.PartialEtaSquared != nil || t.OmegaSquared != nil {
				return analysis.ErrDefinition
			}
			numerator := (len(m.LevelsA) - 1) * (len(m.LevelsB) - 1)
			denominator := snapshot.Design.N - snapshot.Design.ExperimentalUnits - len(m.LevelsA)*(len(m.LevelsB)-1)
			if t.Source == "A" {
				numerator = len(m.LevelsA) - 1
				denominator = snapshot.Design.ExperimentalUnits - len(m.LevelsA)
			}
			if t.Source == "B" {
				numerator = len(m.LevelsB) - 1
			}
			if t.DF != float64(numerator) || *t.DenominatorDF != float64(denominator) || !(*t.F >= 0) {
				return analysis.ErrDefinition
			}
			delete(wantTerms, t.Source)
		}
		if len(wantTerms) != 0 {
			return analysis.ErrDefinition
		}
		for _, v := range m.FixedCoefficients {
			if !analysis.ValidText(v.Name, 120) || !(v.SE > 0) {
				return analysis.ErrDefinition
			}
		}
	} else if result.Model != nil {
		return analysis.ErrDefinition
	}
	if !snapshot.Definition.EffectCI {
		if len(result.EffectIntervals) != 0 {
			return analysis.ErrDefinition
		}
	} else {
		if len(result.EffectIntervals) != 1 {
			return analysis.ErrDefinition
		}
		ci := result.EffectIntervals[0]
		if ci.Source != "A" || ci.Effect != "population_eta_squared" || ci.ConfidenceLevel != 1-snapshot.Definition.Alpha || ci.Method != "MBESS 5.0.1 ci.pvaf / conf.limits.ncf" {
			return analysis.ErrDefinition
		}
		if ci.Lower != nil && !(*ci.Lower >= 0 && *ci.Lower < 1) || ci.Upper != nil && !(*ci.Upper >= 0 && *ci.Upper < 1) {
			return analysis.ErrNumeric
		}
		if ci.LowerAtBoundary != (ci.Lower != nil && *ci.Lower == 0) {
			return analysis.ErrDefinition
		}
		switch ci.Status {
		case "available":
			if ci.Lower == nil || ci.Upper == nil || *ci.Lower > *ci.Upper {
				return analysis.ErrDefinition
			}
			precision, coverage := false, false
			for _, v := range result.Diagnostics {
				if v.Code == "effect_ci_precision" && v.Statistic != nil && *v.Statistic >= 0 && *v.Statistic <= 2e-9 {
					precision = true
				}
				if v.Code == "effect_ci_tail_coverage" && v.Statistic != nil && *v.Statistic >= ci.ConfidenceLevel-2e-9 && *v.Statistic <= 1 {
					coverage = true
				}
			}
			if !precision || !coverage {
				return analysis.ErrDefinition
			}
		case "not_estimable":
			if ci.Upper != nil {
				return analysis.ErrDefinition
			}
			found := false
			for _, v := range result.Warnings {
				if v == "statistics.effect_ci_not_estimable" {
					found = true
				}
			}
			if !found {
				return analysis.ErrDefinition
			}
		default:
			return analysis.ErrDefinition
		}
	}
	// Group values must be the reviewed source quantities, in source order.
	// This binds the result contract to data without claiming an attestation
	// of browser calculations; independent numerical tests validate the engine.
	expected := map[string][]float64{}
	for _, o := range snapshot.Observations {
		if !o.Missing && o.ExcludeReason == "" {
			key := o.FactorA + "\x00" + o.FactorB
			expected[key] = append(expected[key], o.Quantity.Value)
		}
	}
	if len(result.Groups) != len(expected) {
		return analysis.ErrDefinition
	}
	for _, group := range result.Groups {
		key := group.FactorA + "\x00" + group.FactorB
		values, ok := expected[key]
		if !ok || group.N != len(values) || len(group.Values) != len(values) {
			return analysis.ErrDefinition
		}
		for i, v := range values {
			if group.Values[i] != v {
				return analysis.ErrSource
			}
		}
		delete(expected, key)
	}
	for _, term := range result.Terms {
		if !(term.DF > 0) || math.IsInf(term.DF, 0) || math.IsNaN(term.DF) {
			return analysis.ErrNumeric
		}
		if term.P != nil && !(*term.P >= 0 && *term.P <= 1) {
			return analysis.ErrNumeric
		}
	}
	comparisonIDs := map[string]bool{}
	groupIDs := map[string]bool{}
	for _, g := range result.Groups {
		groupIDs[g.FactorA+"\x00"+g.FactorB] = true
	}
	for _, comparison := range result.Comparisons {
		if !(comparison.AdjustedP >= 0 && comparison.AdjustedP <= 1) || comparison.Correction == "" {
			return analysis.ErrNumeric
		}
		if comparison.ID == "" || !analysis.ValidText(comparison.ID, 96) || comparisonIDs[comparison.ID] || !groupIDs[comparison.LeftA+"\x00"+comparison.LeftB] || !groupIDs[comparison.RightA+"\x00"+comparison.RightB] || comparison.LeftA == comparison.RightA && comparison.LeftB == comparison.RightB {
			return analysis.ErrDefinition
		}
		comparisonIDs[comparison.ID] = true
	}
	if snapshot.Definition.PostHoc == "dunnett" {
		if len(result.Comparisons) != len(snapshot.Design.LevelsA)-1 {
			return analysis.ErrDefinition
		}
		treatments := map[string]bool{}
		for _, c := range result.Comparisons {
			if c.LeftA != snapshot.Definition.Control || c.LeftB != "" || c.RightB != "" || treatments[c.RightA] || c.Lower == nil || c.Upper == nil || *c.Lower > c.Difference || *c.Upper < c.Difference || c.Correction != "Dunnett two-sided single-step (multivariate t)" {
				return analysis.ErrDefinition
			}
			treatments[c.RightA] = true
		}
		precision := map[string]bool{}
		for _, diagnostic := range result.Diagnostics {
			limit := 1e-5
			if diagnostic.Code == "dunnett_quantile" {
				limit = 3e-5
			}
			if diagnostic.Statistic != nil && *diagnostic.Statistic >= 0 && *diagnostic.Statistic <= limit {
				precision[diagnostic.Code] = true
			}
		}
		if !precision["dunnett_integration"] || !precision["dunnett_quantile"] || !precision["dunnett_confidence_integration"] {
			return analysis.ErrDefinition
		}
	}
	if _, err := json.Marshal(result); err != nil {
		return analysis.ErrNumeric
	}
	return nil
}

func (p *Profile) SaveAnalysis(def analysis.Definition, receipt string, result analysis.Results, previous string) (analysis.StatisticalAnalysis, error) {
	if len(receipt) > 4096 {
		return analysis.StatisticalAnalysis{}, ErrImportReview
	}
	parts := strings.Split(receipt, ".")
	if len(parts) != 2 {
		return analysis.StatisticalAnalysis{}, ErrImportReview
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return analysis.StatisticalAnalysis{}, ErrImportReview
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, p.analysisMAC(data)) {
		return analysis.StatisticalAnalysis{}, ErrImportReview
	}
	var claim analysisClaim
	if json.Unmarshal(data, &claim) != nil || time.Now().Unix() >= claim.Expires {
		return analysis.StatisticalAnalysis{}, ErrImportReview
	}
	if claim.Account != p.acct.ID() || claim.Profile != p.Entry.ID {
		return analysis.StatisticalAnalysis{}, ErrImportScope
	}
	out, err := p.analysisSnapshot(def)
	if err != nil {
		return analysis.StatisticalAnalysis{}, err
	}
	if claim.Hash != out.SourceHash {
		return analysis.StatisticalAnalysis{}, analysis.ErrSource
	}
	if err := validateAnalysisResults(out, result); err != nil {
		return analysis.StatisticalAnalysis{}, err
	}
	value := analysis.StatisticalAnalysis{Schema: analysis.Schema, ID: secure.NewID(), PreviousID: previous, Created: time.Now().UTC(), Snapshot: out, Results: result, AppVersion: version.Version, ResultOrigin: "local_bundled_browser_worker"}
	done := p.app.begin("statistical-analysis")
	defer done()
	err = p.St.Update(func(tx *store.Tx) error {
		// Close the prepare/save race inside the same transaction that writes
		// the analysis. A source or cycle edit cannot slip between checks.
		current, err := p.analysisSnapshotTx(tx, def)
		if err != nil {
			return err
		}
		if current.SourceHash != claim.Hash {
			return analysis.ErrSource
		}
		value.Snapshot = current
		if previous != "" {
			if _, err := tx.GetRecord(CollAnalyses, previous); err != nil {
				return err
			}
		}
		_, err = tx.Put(CollAnalyses, value.ID, value)
		return err
	})
	return value, err
}

func (p *Profile) Analyses() ([]analysis.StatisticalAnalysis, error) {
	list := []analysis.StatisticalAnalysis{}
	err := p.St.View(func(tx *store.Tx) error {
		rows, err := tx.List(CollAnalyses)
		if err != nil {
			return err
		}
		for _, row := range rows {
			var value analysis.StatisticalAnalysis
			if err := decodeRecord(row, &value); err != nil {
				return err
			}
			list = append(list, value)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for i := range list {
		current, err := p.analysisSnapshot(list[i].Snapshot.Definition)
		list[i].SourceChanged = err != nil || current.SourceHash != list[i].Snapshot.SourceHash
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Created.After(list[j].Created) })
	return list, nil
}

func (p *Profile) Analysis(id string) (analysis.StatisticalAnalysis, error) {
	var value analysis.StatisticalAnalysis
	err := p.St.View(func(tx *store.Tx) error { _, err := tx.Get(CollAnalyses, id, &value); return err })
	if errors.Is(err, store.ErrNotFound) {
		return value, errors.New("statistics.not_found")
	}
	if err != nil {
		return value, err
	}
	current, err := p.analysisSnapshot(value.Snapshot.Definition)
	value.SourceChanged = err != nil || current.SourceHash != value.Snapshot.SourceHash
	return value, nil
}
