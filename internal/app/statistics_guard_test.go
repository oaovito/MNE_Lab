package app

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/plot"
	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/design"
	"github.com/oaovito/mne_lab/internal/science/model"
)

// designDefinition is what the editor builds from a saved design revision:
// unit, condition and time of every observation come from the design.
func designDefinition(v DesignView) analysis.Definition {
	d := analysis.Definition{Schema: 1, Module: "lightscattering", Title: "Synthetic paired design", Variable: model.EffectiveDiameter, FactorAName: "Condition", FactorBName: "Time",
		Structure: "repeated", StructureReviewed: true, Method: "repeated", SphericityCorrection: "GG", Alpha: .05, PostHoc: "simple_effects", Adjustment: "holm", Design: analysis.DesignWithinWithin, DesignID: v.Design.ID}
	for _, a := range v.Design.Assignments {
		d.Observations = append(d.Observations, analysis.ObservationDefinition{MeasurementID: a.MeasurementID, FactorA: a.Condition, FactorB: a.Time, UnitID: a.UnitID})
	}
	return d
}

func hasWarning(d analysis.Design, code string) bool {
	for _, w := range d.Warnings {
		if w == code {
			return true
		}
	}
	return false
}

// Addendum B §37-38 and A §14: the structure comes from the saved design
// revision, the revision enters the snapshot, and any reading that makes
// one preparation several subjects is refused.
func TestPseudoreplicationGuardFollowsTheSavedDesign(t *testing.T) {
	_, _, p := inspectionProfile(t)
	v := pairedDesign(t, p, importPaired(t, p))
	def := designDefinition(v)
	s, err := p.PrepareAnalysis(def)
	if err != nil {
		t.Fatal(err)
	}
	if s.ExperimentalDesign == nil || s.ExperimentalDesign.ID != v.Design.ID || s.ExperimentalDesign.Revision != 1 || len(s.DesignHeads) != 1 || s.DesignHeads[0] != v.Design.ID {
		t.Fatal("design revision not recorded in the snapshot", s.ExperimentalDesign, s.DesignHeads)
	}
	names := []string{}
	for _, l := range s.UnitLabels {
		names = append(names, l)
	}
	sort.Strings(names)
	if s.Design.ExperimentalUnits != 3 || s.Design.N != 24 || strings.Join(names, ",") != "A,B,C" {
		t.Fatal("units by name", s.Design, names)
	}

	// 24 measurements declared independent: refused with the owner's message,
	// in the paired layout and in the original two-way layout, with or
	// without unit identities on the observations (the design links them).
	independent := def
	independent.Structure = "independent"
	if _, err := p.PrepareAnalysis(independent); !errors.Is(err, analysis.ErrPseudoreplication) {
		t.Fatal("paired observations accepted as independent", err)
	}
	legacy := def
	legacy.Design, legacy.DesignID, legacy.Structure, legacy.Method, legacy.PostHoc, legacy.Adjustment, legacy.SphericityCorrection = "", "", "independent", "two_way", "none", "", ""
	legacy.Observations = append([]analysis.ObservationDefinition{}, def.Observations...)
	if _, err := p.PrepareAnalysis(legacy); !errors.Is(err, analysis.ErrPseudoreplication) || err.Error() != "statistics.pseudoreplication" {
		t.Fatal("24 measurements with shared units accepted as independent", err)
	}
	for i := range legacy.Observations {
		legacy.Observations[i].UnitID = ""
	}
	if _, err := p.PrepareAnalysis(legacy); !errors.Is(err, analysis.ErrPseudoreplication) {
		t.Fatal("design-linked measurements accepted as independent without unit IDs", err)
	}

	// A F-SR and A C-SR as two different subjects although the design says
	// they are one preparation: refused.
	fsr, err := p.CreateExperimentalUnit("A F-SR")
	if err != nil {
		t.Fatal(err)
	}
	csr, err := p.CreateExperimentalUnit("A C-SR")
	if err != nil {
		t.Fatal(err)
	}
	split := def
	split.DesignID = ""
	split.Observations = append([]analysis.ObservationDefinition{}, def.Observations...)
	for i, o := range split.Observations {
		if v.UnitLabels[o.UnitID] == "A" {
			split.Observations[i].UnitID = map[string]string{"F-SR": fsr.ID, "C-SR": csr.ID}[o.FactorA]
		}
	}
	if _, err := p.PrepareAnalysis(split); !errors.Is(err, analysis.ErrPseudoreplication) {
		t.Fatal("one design unit split into two subjects", err)
	}
	split.Design, split.PostHoc, split.Adjustment = "", "none", ""
	if _, err := p.PrepareAnalysis(split); !errors.Is(err, analysis.ErrPseudoreplication) {
		t.Fatal("one design unit split into two subjects in the original layout", err)
	}

	// The original layout with condition between units cannot hold units
	// observed under both conditions: the paired design is suggested.
	between := def
	between.Design, between.DesignID, between.PostHoc, between.Adjustment = "", "", "none", ""
	if _, err := p.PrepareAnalysis(between); !errors.Is(err, analysis.ErrPairedDetected) || !errors.Is(err, analysis.ErrDesign) {
		t.Fatal("paired longitudinal design not detected", err)
	}

	// The definition must follow the revision; the person reviews, never rebuilds.
	moved := def
	moved.Observations = append([]analysis.ObservationDefinition{}, def.Observations...)
	moved.Observations[0].FactorB = moved.Observations[1].FactorB + " "
	if _, err := p.PrepareAnalysis(moved); !errors.Is(err, analysis.ErrDesignMismatch) {
		t.Fatal("observation that differs from the design accepted", err)
	}
	moved.Observations = def.Observations[1:]
	if _, err := p.PrepareAnalysis(moved); !errors.Is(err, analysis.ErrDesignMismatch) {
		t.Fatal("observation left out of the design accepted", err)
	}
	excluded := def
	excluded.Observations = append([]analysis.ObservationDefinition{}, def.Observations...)
	excluded.Observations[0].ExcludeReason = "Synthetic exclusion"
	excluded.PostHoc, excluded.Adjustment, excluded.Method, excluded.SphericityCorrection = "none", "", "mixed", ""
	if _, err := p.PrepareAnalysis(excluded); err != nil {
		t.Fatal("exclusion with a reason refused", err)
	}
	missing := def
	missing.DesignID = "no-such-design"
	if _, err := p.PrepareAnalysis(missing); !errors.Is(err, design.ErrNotFound) {
		t.Fatal(err)
	}

	// A new revision of the design marks analyses of the old one changed.
	next := v.Design
	next.Notes = "Synthetic note"
	if _, err := p.SaveDesign(next, v.Design.ID); err != nil {
		t.Fatal(err)
	}
	again, err := p.PrepareAnalysis(def)
	if err != nil {
		t.Fatal(err)
	}
	if again.SourceHash == s.SourceHash || again.ExperimentalDesign.Revision != 1 || again.DesignHeads[0] == v.Design.ID {
		t.Fatal("a later design revision does not change the source snapshot")
	}
	// Renaming a unit is presentation only: the source hash is unchanged.
	for id, l := range v.UnitLabels {
		if l == "B" {
			if _, err := p.RenameExperimentalUnit(id, "B renamed"); err != nil {
				t.Fatal(err)
			}
		}
	}
	renamed, err := p.PrepareAnalysis(def)
	if err != nil || renamed.SourceHash != again.SourceHash {
		t.Fatal("unit name entered the source hash", err)
	}
}

func importSynthetic(t *testing.T, p *Profile, name, sample string, value float64) string {
	t.Helper()
	r := p.Import(name, []byte(fmt.Sprintf("Sample ID: %s\nEffective Diameter (nm): %.3f\n", sample, value)), false)
	if r.Error != "" || r.Measurements != 1 {
		t.Fatal("synthetic import failed", r)
	}
	f, _, err := p.File(r.FileID)
	if err != nil {
		t.Fatal(err)
	}
	return f.Measurements[0]
}

// Names and sample identifications are weaker evidence than a saved design:
// they warn, with the owner's message, and never refuse by themselves.
func TestPseudoreplicationWarningsWithoutADesign(t *testing.T) {
	_, _, p := inspectionProfile(t)
	// Distinct unit IDs that look like one preparation per condition.
	d := analysis.Definition{Schema: 1, Module: "lightscattering", Title: "Synthetic", Variable: model.EffectiveDiameter, FactorAName: "Condition", FactorBName: "Time", Structure: "repeated", StructureReviewed: true, Method: "repeated", SphericityCorrection: "GG", Alpha: .05, PostHoc: "none"}
	n := 0
	for _, unit := range []string{"A", "B"} {
		for _, cond := range []string{"F-SR", "C-SR"} {
			u, err := p.CreateExperimentalUnit(cond + " " + unit)
			if err != nil {
				t.Fatal(err)
			}
			for _, day := range []string{"day 0", "day 7"} {
				n++
				id := importSynthetic(t, p, fmt.Sprintf("synthetic-guard-%d.txt", n), fmt.Sprintf("Synthetic guard %d", n), 100+float64(n*n%7))
				d.Observations = append(d.Observations, analysis.ObservationDefinition{MeasurementID: id, FactorA: cond, FactorB: day, UnitID: u.ID})
			}
		}
	}
	s, err := p.PrepareAnalysis(d)
	if err != nil {
		t.Fatal(err)
	}
	if !hasWarning(s.Design, "statistics.pseudoreplication") {
		t.Fatal("units split by condition not warned", s.Design.Warnings)
	}

	// Repeated sample IDs without unit IDs, declared independent.
	ind := analysis.Definition{Schema: 1, Module: "lightscattering", Title: "Synthetic", Variable: model.EffectiveDiameter, FactorAName: "Group", Structure: "independent", StructureReviewed: true, Method: "one_way", Alpha: .05, PostHoc: "none"}
	for i := 0; i < 6; i++ {
		id := importSynthetic(t, p, fmt.Sprintf("synthetic-sample-%d.txt", i), fmt.Sprintf("Synthetic sample %d", i/2), 50+float64(i)+float64(i%2)*3)
		ind.Observations = append(ind.Observations, analysis.ObservationDefinition{MeasurementID: id, FactorA: []string{"G1", "G2"}[i%2]})
	}
	s, err = p.PrepareAnalysis(ind)
	if err != nil {
		t.Fatal(err)
	}
	if !hasWarning(s.Design, "statistics.repeated_samples_without_units") || hasWarning(s.Design, "statistics.pseudoreplication") {
		t.Fatal("repeated sample IDs without units not warned", s.Design.Warnings)
	}
	// Unique sample IDs: no warning.
	ind.Observations = ind.Observations[:0]
	for i := 0; i < 6; i++ {
		id := importSynthetic(t, p, fmt.Sprintf("synthetic-unique-%d.txt", i), fmt.Sprintf("Synthetic unique %d", i), 60+float64(i)+float64(i%2)*3)
		ind.Observations = append(ind.Observations, analysis.ObservationDefinition{MeasurementID: id, FactorA: []string{"G1", "G2"}[i%2]})
	}
	if s, err = p.PrepareAnalysis(ind); err != nil || hasWarning(s.Design, "statistics.repeated_samples_without_units") {
		t.Fatal("unique samples warned", err, s.Design.Warnings)
	}
}

// Exports of a paired analysis taken from a design: every format carries
// the within-condition RM ANOVA, the simple effects, the design revision,
// the unit names and the factor levels; the package carries the design.
func TestPairedDesignExports(t *testing.T) {
	_, _, p := inspectionProfile(t)
	g := loadPairedGolden(t)
	d, units, index := pairedFixture(t, p, g, "owner_3x2x4_repeated_gg_holm", "repeated", "simple_effects", "holm")
	draft := design.Design{Name: "Synthetic export design"}
	seenA, seenB := map[string]bool{}, map[string]bool{}
	for label, id := range units {
		_ = label
		draft.Units = append(draft.Units, design.Unit{ID: id})
	}
	sort.Slice(draft.Units, func(i, j int) bool { return draft.Units[i].ID < draft.Units[j].ID })
	for _, o := range d.Observations {
		if !seenA[o.FactorA] {
			seenA[o.FactorA] = true
			draft.Conditions = append(draft.Conditions, design.Condition{Label: o.FactorA})
		}
		if !seenB[o.FactorB] {
			seenB[o.FactorB] = true
			draft.Times = append(draft.Times, design.TimeLevel{Label: o.FactorB})
		}
		draft.Assignments = append(draft.Assignments, design.Assignment{MeasurementID: o.MeasurementID, UnitID: o.UnitID, Condition: o.FactorA, Time: o.FactorB})
	}
	draft.Pairs = []design.Pair{}
	for _, u := range draft.Units {
		draft.Pairs = append(draft.Pairs, design.Pair{UnitID: u.ID, From: "F-SR", To: "C-SR"})
	}
	v, err := p.SaveDesign(draft, "")
	if err != nil {
		t.Fatal(err)
	}
	d.DesignID = v.Design.ID
	s, err := p.PrepareAnalysis(d)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := p.SaveAnalysis(d, s.Receipt, pairedResults(g, index, d, units), "")
	if err != nil {
		t.Fatal(err)
	}
	outs, _, err := p.analysisOutputs(saved.ID, ExportRequest{Formats: []string{"package", "csv", "txt", "json", "pdf", "xlsx"}}, plot.Preset(plot.PresetScreen), false)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, o := range outs {
		files[o.name] = o.data
	}
	find := func(suffix string) []byte {
		for name, b := range files {
			if strings.HasSuffix(name, suffix) {
				return b
			}
		}
		t.Fatal("missing export file", suffix)
		return nil
	}
	for _, format := range []string{".csv", ".txt"} {
		simple := string(find("_simple_effects" + format))
		if !strings.Contains(simple, "A=") || !strings.Contains(simple, "residual_unit:B") {
			t.Fatal("within-condition RM ANOVA missing from", format, simple)
		}
		posthoc := string(find("_posthoc" + format))
		if !strings.Contains(posthoc, "unadjusted_CI_lower") || !strings.Contains(posthoc, "A_within_B") || !strings.Contains(posthoc, "Holm; paired t; individual CI") {
			t.Fatal("simple effects missing from", format)
		}
		des := string(find("_design" + format))
		for _, want := range []string{v.Design.ID, "Synthetic export design", "Invented unit-A", "F-SR", "C-SR", "day 21", "within_within"} {
			if !strings.Contains(des, want) {
				t.Fatal("design export lacks", want, format)
			}
		}
		obs := string(find("_unit_observations" + format))
		if !strings.Contains(obs, "Invented unit-B") {
			t.Fatal("unit observations lack unit names", format)
		}
	}
	rows, err := csv.NewReader(bytes.NewReader(find("_observations.csv"))).ReadAll()
	if err != nil || !strings.Contains(strings.Join(rows[0], ","), "unit_label") {
		t.Fatal("observations lack the unit label", err)
	}
	var full analysis.StatisticalAnalysis
	whole := []byte{}
	for name, b := range files {
		if !strings.Contains(name, "/") && strings.HasSuffix(name, ".json") {
			whole = b
		}
	}
	if err := json.Unmarshal(whole, &full); err != nil || full.Snapshot.ExperimentalDesign == nil || full.Snapshot.ExperimentalDesign.Revision != 1 || len(full.Results.SimpleEffects) != 2 {
		t.Fatal("JSON export lacks the design or simple effects", err)
	}
	var packaged struct {
		Design design.Design `json:"design"`
	}
	if err := json.Unmarshal(find("statistics/analysis-definitions/experimental-design.json"), &packaged); err != nil || packaged.Design.ID != v.Design.ID || len(packaged.Design.Pairs) != 3 || len(packaged.Design.Assignments) != 24 {
		t.Fatal("package lacks the design JSON", err)
	}
	if !bytes.Contains(find("statistics/posthoc-tests/simple-effects.json"), []byte(`"withinCondition"`)) {
		t.Fatal("package lacks the simple effects")
	}
	pdf := find(".pdf")
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatal("no PDF")
	}
	x := find(".xlsx")
	zr, err := zip.NewReader(bytes.NewReader(x), int64(len(x)))
	if err != nil {
		t.Fatal(err)
	}
	sheets := 0
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "xl/worksheets/") {
			sheets++
		}
	}
	if sheets < 11 {
		t.Fatal("XLSX lacks the paired tables", sheets)
	}
}
