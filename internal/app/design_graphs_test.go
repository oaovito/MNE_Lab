package app

// Design graphs, groups, Combine, Graph → Analyze annotations and exports
// through the profile (addendum B §15-24, §39-47, §59, §61). Every value is
// synthetic (the invented golden cases of the paired statistics tests).

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/export"
	"github.com/oaovito/mne_lab/internal/plot"
	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/design"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/series"
	"github.com/oaovito/mne_lab/internal/science/units"
	"github.com/oaovito/mne_lab/internal/store"
)

func cycleConfigForTest() cycle.Config {
	return cycle.Config{Name: "Invented group cycle", Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), StartTZ: true, Interval: 7, Unit: cycle.Days, Duration: 21, Params: []string{model.EffectiveDiameter}}
}

// designWithAnalysis imports the invented 3 units × 2 conditions × 4 times
// case, saves its design and a saved paired analysis bound to it.
func designWithAnalysis(t *testing.T, p *Profile) (DesignView, analysis.StatisticalAnalysis) {
	t.Helper()
	g := loadPairedGolden(t)
	d, units, index := pairedFixture(t, p, g, "owner_3x2x4_repeated_gg_holm", "repeated", "simple_effects", "holm")
	draft := design.Design{Name: "Synthetic paired stability",
		Conditions: []design.Condition{{Label: "F-SR"}, {Label: "C-SR", DerivedFrom: "F-SR", Process: "centrifugation"}}}
	for _, day := range []string{"day 00", "day 07", "day 14", "day 21"} {
		draft.Times = append(draft.Times, design.TimeLevel{Label: day})
	}
	seen := map[string]bool{}
	for _, o := range d.Observations {
		if !seen[o.UnitID] {
			seen[o.UnitID] = true
			draft.Units = append(draft.Units, design.Unit{ID: o.UnitID})
		}
		draft.Assignments = append(draft.Assignments, design.Assignment{MeasurementID: o.MeasurementID, UnitID: o.UnitID, Condition: o.FactorA, Time: o.FactorB})
	}
	draft.Pairs = design.DerivedPairs(draft)
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
	return v, saved
}

func storedGraph(t *testing.T, p *Profile, id string) []byte {
	t.Helper()
	var out []byte
	if err := p.St.View(func(tx *store.Tx) error {
		r, err := tx.GetRecord(CollGraphs, id)
		out = append([]byte(nil), r.Data...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

// Addendum B §59: Graph 1 (F-SR) and Graph 2 (C-SR) combine into Graph 3,
// a new definition that records both parents; 1 and 2 stay byte-identical.
func TestCombineCreatesGraphThreeAndKeepsParents(t *testing.T) {
	_, _, p := inspectionProfile(t)
	v, _ := designWithAnalysis(t, p)
	g1, err := p.SaveGraph(graph.Definition{Title: "Graph 1 · F-SR", Kind: graph.KindDesignTime, DesignID: v.Design.ID, Param: model.EffectiveDiameter, Conditions: []string{"F-SR"}})
	if err != nil {
		t.Fatal(err)
	}
	if g1.ErrorBars != "sd" || len(g1.Measurements) != 12 || len(g1.SourceFiles) != 12 {
		t.Fatalf("graph 1: bars %q, measurements %d, files %d", g1.ErrorBars, len(g1.Measurements), len(g1.SourceFiles))
	}
	g2, err := p.SaveGraph(graph.Definition{Title: "Graph 2 · C-SR", Kind: graph.KindDesignTime, DesignID: v.Design.ID, Param: model.EffectiveDiameter, Conditions: []string{"C-SR"}})
	if err != nil {
		t.Fatal(err)
	}
	r1, err := p.Compute(g1)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range r1.Series {
		if s.Kind == "mean" {
			for _, n := range s.N {
				if n != 3 {
					t.Fatalf("n counts experimental units: %v", s.N)
				}
			}
		}
	}
	b1, b2 := storedGraph(t, p, g1.ID), storedGraph(t, p, g2.ID)
	ck, err := p.CheckCombine([]string{g1.ID, g2.ID}, "")
	if err != nil || !ck.Compatible {
		t.Fatalf("check: %v %+v", err, ck)
	}
	g3, _, err := p.CombineGraphs([]string{g1.ID, g2.ID}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if g3.ID == g1.ID || g3.ID == g2.ID || len(g3.DerivedFrom) != 2 || g3.DerivedFrom[0].GraphID != g1.ID || g3.DerivedFrom[1].GraphID != g2.ID ||
		g3.DerivedFrom[0].DefinitionHash != graph.Hash(b1) || g3.DerivedFrom[1].DefinitionHash != graph.Hash(b2) || len(g3.Measurements) != 24 {
		t.Fatalf("graph 3: %+v", g3)
	}
	if !bytes.Equal(storedGraph(t, p, g1.ID), b1) || !bytes.Equal(storedGraph(t, p, g2.ID), b2) {
		t.Fatal("combining changed a parent")
	}
	r3, err := p.Compute(g3)
	if err != nil {
		t.Fatal(err)
	}
	plot.Layout(r3, plot.Preset(plot.PresetScreen), p.app.T)
	var means []graph.Series
	for _, s := range r3.Series {
		if s.Kind == "mean" {
			means = append(means, s)
		}
	}
	if len(means) != 2 || means[0].Marker == means[1].Marker || means[0].LineStyle == means[1].LineStyle {
		t.Fatalf("conditions must differ in marker and line: %+v", means)
	}
	// Editing Graph 3 never touches 1 or 2; the lineage is kept.
	g3.Title, g3.UnitTrajectories = "Graph 3 · F-SR vs C-SR", true
	g3.DerivedFrom = nil
	g3, err = p.SaveGraph(g3)
	if err != nil || len(g3.DerivedFrom) != 2 {
		t.Fatalf("edit: %v %+v", err, g3.DerivedFrom)
	}
	if !bytes.Equal(storedGraph(t, p, g1.ID), b1) || !bytes.Equal(storedGraph(t, p, g2.ID), b2) {
		t.Fatal("editing graph 3 changed a parent")
	}
	// Lineage both ways; a later edit of a parent is told, never followed.
	rel, err := p.Relations()
	if err != nil {
		t.Fatal(err)
	}
	if !contains(rel.Graphs[g1.ID].Derived, g3.ID) || !contains(rel.Graphs[g2.ID].Derived, g3.ID) || len(rel.Graphs[g3.ID].DerivedFrom) != 2 ||
		len(rel.Graphs[g3.ID].ChangedParents) != 0 || rel.Graphs[g3.ID].Design != v.Design.ID {
		t.Fatalf("relations: %+v %+v", rel.Graphs[g1.ID], rel.Graphs[g3.ID])
	}
	g1.Title = "Graph 1 renamed"
	if _, err := p.SaveGraph(g1); err != nil {
		t.Fatal(err)
	}
	rel, _ = p.Relations()
	if strings.Join(rel.Graphs[g3.ID].ChangedParents, ",") != g1.ID {
		t.Fatalf("changed parent: %v", rel.Graphs[g3.ID].ChangedParents)
	}
	again, _ := p.Graph(g3.ID)
	if !strings.Contains(strings.Join(again.Conditions, ","), "C-SR") || again.Title != "Graph 3 · F-SR vs C-SR" {
		t.Fatalf("a parent's edit must not propagate: %+v", again)
	}
}

func TestCombineRefusesIncompatibleGraphs(t *testing.T) {
	_, _, p := inspectionProfile(t)
	v, _ := designWithAnalysis(t, p)
	ed, err := p.SaveGraph(graph.Definition{Title: "ED", Kind: graph.KindDesignTime, DesignID: v.Design.ID, Param: model.EffectiveDiameter})
	if err != nil {
		t.Fatal(err)
	}
	paired, err := p.SaveGraph(graph.Definition{Title: "Paired", Kind: graph.KindPairedCondition, DesignID: v.Design.ID, Param: model.EffectiveDiameter, PairedAt: "day 07"})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, a := range v.Design.Assignments {
		ids[a.MeasurementID] = true
	}
	var sel []string
	for id := range ids {
		sel = append(sel, series.ScalarID(id, model.EffectiveDiameter))
		if len(sel) == 2 {
			break
		}
	}
	values, err := p.SaveGraph(graph.Definition{Title: "Values", Kind: graph.KindSeriesValues, SeriesIDs: sel})
	if err != nil {
		t.Fatal(err)
	}
	for name, pair := range map[string][]string{"kind": {ed.ID, paired.ID}, "series and design": {ed.ID, values.ID}} {
		before := storedGraph(t, p, pair[0])
		ck, err := p.CheckCombine(pair, "")
		if err != nil || ck.Compatible || !contains(ck.Problems, graph.CombineKind) {
			t.Fatalf("%s: %v %+v", name, err, ck)
		}
		if _, _, err := p.CombineGraphs(pair, "", ""); !errors.Is(err, graph.ErrCombine) {
			t.Fatalf("%s must be refused: %v", name, err)
		}
		if !bytes.Equal(before, storedGraph(t, p, pair[0])) {
			t.Fatal("a refused combination changed a graph")
		}
	}
	all, _ := p.Graphs(false)
	if len(all) != 3 {
		t.Fatalf("a refused combination created a graph: %d", len(all))
	}
}

// Addendum B §61: the pairing declared in the design reaches the paired
// view and the saved analysis; annotations quote the saved comparisons.
func TestPairingThroughGraphAndAnalysis(t *testing.T) {
	_, _, p := inspectionProfile(t)
	v, saved := designWithAnalysis(t, p)
	if saved.Snapshot.ExperimentalDesign == nil || saved.Snapshot.ExperimentalDesign.ID != v.Design.ID || len(saved.Snapshot.ExperimentalDesign.Pairs) != 3 {
		t.Fatal("analysis must keep the design and its pairs")
	}
	pv, err := p.SaveGraph(graph.Definition{Title: "Paired day 7", Kind: graph.KindPairedCondition, DesignID: v.Design.ID, Param: model.EffectiveDiameter, PairedAt: "day 07"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Compute(pv)
	if err != nil {
		t.Fatal(err)
	}
	pairs := 0
	for _, s := range res.Series {
		if s.Kind == "paired" {
			pairs++
			if s.Meta["pairing"] != "declared" || len(s.Y) != 2 {
				t.Fatalf("pair: %+v", s)
			}
		}
	}
	if pairs != 3 {
		t.Fatalf("pairs %d", pairs)
	}
	// Graph → Analyze → annotate: only comparisons of the saved analysis.
	var ids []string
	for _, c := range saved.Results.Comparisons {
		if c.Family == "A_within_B" {
			ids = append(ids, c.ID)
		}
	}
	def := graph.Definition{Title: "Annotated", Kind: graph.KindDesignTime, DesignID: v.Design.ID, Param: model.EffectiveDiameter, AnalysisID: saved.ID, Annotations: ids, ErrorBars: "ci"}
	def, err = p.SaveGraph(def)
	if err != nil {
		t.Fatal(err)
	}
	res, err = p.Compute(def)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Annotations) != len(ids) || res.ErrorBars != "ci" {
		t.Fatalf("annotations %d of %d, bars %q, warnings %v", len(res.Annotations), len(ids), res.ErrorBars, res.Warnings)
	}
	stored := map[string]analysis.Comparison{}
	for _, c := range saved.Results.Comparisons {
		stored[c.ID] = c
	}
	for _, a := range res.Annotations {
		c := stored[a.ComparisonID]
		if a.AdjustedP != c.AdjustedP || a.Correction != c.Correction || a.AnalysisID != saved.ID || a.Contrast != c.Contrast {
			t.Fatalf("annotation must quote the saved comparison: %+v %+v", a, c)
		}
	}
	bad := def
	bad.Annotations = []string{"comparison-999"}
	if _, err := p.SaveGraph(bad); !errors.Is(err, analysis.ErrDefinition) {
		t.Fatalf("unknown comparison: %v", err)
	}
	pdi := def
	pdi.ID, pdi.Param, pdi.Annotations = "", model.Polydispersity, nil
	if _, err := p.SaveGraph(pdi); !errors.Is(err, graph.ErrAnalysisIncompatible) {
		t.Fatalf("analysis of another variable: %v", err)
	}
	// Without an analysis a confidence interval is never drawn.
	plain := graph.Definition{Title: "No analysis", Kind: graph.KindDesignTime, DesignID: v.Design.ID, Param: model.EffectiveDiameter, ErrorBars: "ci", Annotations: ids}
	plain, err = p.SaveGraph(plain)
	if err != nil || plain.ErrorBars != "sd" || len(plain.Annotations) != 0 {
		t.Fatalf("plain: %v %q %v", err, plain.ErrorBars, plain.Annotations)
	}
}

func TestGroupsStoreReferencesOnly(t *testing.T) {
	_, _, p := inspectionProfile(t)
	v, _ := designWithAnalysis(t, p)
	var fsr []string
	for _, a := range v.Design.Assignments {
		if a.Condition == "F-SR" {
			fsr = append(fsr, a.MeasurementID)
		}
	}
	before := measurementBytes(t, p)
	g, err := p.SaveGroup(Group{Name: "F-SR Triplicate", Measurements: append(fsr, fsr[0])})
	if err != nil {
		t.Fatal(err)
	}
	if g.Kind != GroupKindGroup || len(g.Measurements) != 12 {
		t.Fatalf("group: %+v", g)
	}
	var raw map[string]any
	if err := p.St.View(func(tx *store.Tx) error {
		r, err := tx.GetRecord(CollGroups, g.ID)
		if err == nil {
			err = json.Unmarshal(r.Data, &raw)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for k := range raw {
		if !contains([]string{"schema", "id", "name", "kind", "measurements", "created", "updated"}, k) {
			t.Fatalf("a group stores references only, found %q", k)
		}
	}
	if _, err := p.SaveGroup(Group{Name: "Bad", Measurements: []string{"not-a-measurement"}}); !errors.Is(err, ErrGroupMember) {
		t.Fatalf("unknown member: %v", err)
	}
	sel, err := p.SaveGroup(Group{Name: "Day 14 comparison", SeriesIDs: []string{series.ScalarID(fsr[0], model.EffectiveDiameter), series.ScalarID(fsr[1], model.EffectiveDiameter)}})
	if err != nil || sel.Kind != GroupKindSelection || len(sel.Measurements) != 2 {
		t.Fatalf("selection: %v %+v", err, sel)
	}
	// A group builds a graph: the design graph reads only its members.
	gr, err := p.SaveGraph(graph.Definition{Title: "From group", Kind: graph.KindDesignTime, DesignID: v.Design.ID, Param: model.EffectiveDiameter, Groups: []string{g.ID}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Compute(gr)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Series {
		if s.Meta["condition"] == "C-SR" {
			t.Fatal("the group holds F-SR only")
		}
	}
	// A group builds a cycle and is named by it.
	cy, err := p.SaveCycle(CycleDoc{Config: cycleConfigForTest(), Measurements: g.Measurements, GroupID: g.ID})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := p.Relations()
	if err != nil {
		t.Fatal(err)
	}
	gr2 := rel.Groups[g.ID]
	if !contains(gr2.Graphs, gr.ID) || !contains(gr2.Cycles, cy.ID) || len(gr2.Files) != 12 {
		t.Fatalf("group relations: %+v", gr2)
	}
	// Deleting a source file warns through Relations.
	f := gr2.Files[0]
	if !contains(rel.Files[f].Groups, g.ID) {
		t.Fatalf("file relations: %+v", rel.Files[f])
	}
	if err := p.DeleteGroup(g.ID); !errors.Is(err, ErrGroupInUse) {
		t.Fatalf("a group read by a graph must not be deleted: %v", err)
	}
	if err := p.DeleteGroup(sel.ID); err != nil {
		t.Fatal(err)
	}
	if after := measurementBytes(t, p); len(after) != len(before) {
		t.Fatal("groups changed measurements")
	} else {
		for k, b := range before {
			if !bytes.Equal(b, after[k]) {
				t.Fatal("groups changed a measurement")
			}
		}
	}
	// A group exports its members as tables.
	outs, _, err := p.groupOutputs(g.ID, ExportRequest{Formats: []string{"csv"}}, false)
	if err != nil || len(outs) != 1 || strings.Count(string(outs[0].data), "\r\n") != 13 {
		t.Fatalf("group export: %v %d", err, len(outs))
	}
	// Groups live in a synced collection.
	if strings.HasPrefix(CollGroups, "_") {
		t.Fatal("groups must sync")
	}
}

func TestDesignGraphExportsAndPackage(t *testing.T) {
	a, _, p := inspectionProfile(t)
	v, saved := designWithAnalysis(t, p)
	grp, _ := p.SaveGroup(Group{Name: "All", Measurements: func() []string {
		var ids []string
		for _, x := range v.Design.Assignments {
			ids = append(ids, x.MeasurementID)
		}
		return ids
	}()})
	g1, _ := p.SaveGraph(graph.Definition{Title: "Graph 1", Kind: graph.KindDesignTime, DesignID: v.Design.ID, Param: model.EffectiveDiameter, Conditions: []string{"F-SR"}, Groups: []string{grp.ID}})
	g2, _ := p.SaveGraph(graph.Definition{Title: "Graph 2", Kind: graph.KindDesignTime, DesignID: v.Design.ID, Param: model.EffectiveDiameter, Conditions: []string{"C-SR"}})
	g3, _, err := p.CombineGraphs([]string{g1.ID, g2.ID}, "Graph 3", "")
	if err != nil {
		t.Fatal(err)
	}
	g3.AnalysisID = saved.ID
	g3.Annotations = []string{saved.Results.Comparisons[0].ID}
	if g3, err = p.SaveGraph(g3); err != nil {
		t.Fatal(err)
	}
	outs, _, err := p.graphOutputs(g3, []string{"csv", "json", "svg"}, plot.Preset(plot.PresetScreen), export.DataOptions{}, false)
	if err != nil || len(outs) != 3 {
		t.Fatalf("outputs: %v %d", err, len(outs))
	}
	csv := string(outs[0].data)
	if strings.Count(csv, "\r\n") != 25 || !strings.Contains(csv, "day 21") || !strings.Contains(csv, "C-SR") {
		t.Fatalf("csv: %s", csv)
	}
	var js struct {
		Tables []struct {
			ID string `json:"id"`
		} `json:"tables"`
		Metadata map[string]string `json:"Metadata"`
	}
	if err := json.Unmarshal(outs[1].data, &js); err != nil || len(js.Tables) != 3 {
		t.Fatalf("json: %v %s", err, outs[1].data[:200])
	}
	for _, k := range []string{"design", "design_revision", "analysis", "derived_from_1", "derived_from_2", "error_bars"} {
		if js.Metadata[k] == "" {
			t.Fatalf("metadata %q missing: %v", k, js.Metadata)
		}
	}
	if !strings.Contains(string(outs[2].data), "<svg") {
		t.Fatal("figure")
	}
	// The research package carries the design with its pairs, the groups,
	// the lineage and the linked analysis.
	dir := t.TempDir()
	res, err := a.Export(ExportRequest{Items: []ExportItem{{Kind: export.KindGraph, ID: g3.ID}}, Formats: []string{"package"}, Destination: dir})
	if err != nil || len(res.Saved) != 1 {
		t.Fatalf("package: %v %+v", err, res)
	}
	b, err := os.ReadFile(filepath.Join(dir, res.Saved[0].Name))
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name[strings.Index(f.Name, "/")+1:]] = f
	}
	for _, want := range []string{"design/design.json", "graphs/definitions.json", "provenance/provenance.json", "manifest/manifest.json"} {
		if files[want] == nil {
			var names []string
			for n := range files {
				names = append(names, n)
			}
			t.Fatalf("package lacks %s: %v", want, names)
		}
	}
	rc, _ := files["design/design.json"].Open()
	var dj struct {
		Design design.Design `json:"design"`
		Pairs  []design.Pair `json:"pairs"`
	}
	json.NewDecoder(rc).Decode(&dj)
	rc.Close()
	if dj.Design.ID != v.Design.ID || len(dj.Pairs) != 3 {
		t.Fatalf("pairing must be readable back: %+v", dj.Pairs)
	}
	originals, analyses := 0, 0
	for n := range files {
		if strings.HasPrefix(n, export.DirOriginals+"/") {
			originals++
		}
		if strings.HasPrefix(n, "statistics/analyses/"+saved.ID+"/") {
			analyses++
		}
	}
	if originals != 24 || analyses == 0 {
		t.Fatalf("originals %d, analysis files %d", originals, analyses)
	}
}

func TestUnitUsageCoversDisplayUnitsAndReadings(t *testing.T) {
	_, _, p := inspectionProfile(t)
	custom, err := p.CreateMeasurementUnit("nmx", "Invented length", units.Length)
	if err != nil {
		t.Fatal(err)
	}
	v, _ := designWithAnalysis(t, p)
	g, err := p.SaveGraph(graph.Definition{Title: "Display", Kind: graph.KindDesignTime, DesignID: v.Design.ID, Param: model.EffectiveDiameter, DisplayUnit: "nmx"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := p.MeasurementUnitUsage(custom.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, use := range u.Uses {
		if use.Kind == "graph" && use.ID == g.ID && contains(use.Fields, "displayUnit") && use.Origin == "user_assigned" && use.Replaceable {
			found = true
		}
	}
	if !found || !u.Replaceable {
		t.Fatalf("display unit usage: %+v", u)
	}
	target, err := p.CreateMeasurementUnit("nmy", "Invented length 2", units.Length)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := p.ReplaceMeasurementUnit(custom.ID, target.ID)
	if err != nil || len(rec.Changes) != 1 || rec.Changes[0].Field != "displayUnit" {
		t.Fatalf("replace: %v %+v", err, rec)
	}
	if after, _ := p.Graph(g.ID); after.DisplayUnit != "nmy" {
		t.Fatalf("display unit not replaced: %q", after.DisplayUnit)
	}
	// Units written in a runs table header are stated by the source file.
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-nanobrook-run-table.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if r := p.Import("synthetic-nanobrook-run-table.txt", b, false); r.Error != "" {
		t.Fatal(r.Error)
	}
	var idx map[string][]UnitUse
	p.St.View(func(tx *store.Tx) error {
		idx, err = unitUsageIndex(tx)
		return err
	})
	readings := 0
	for _, uses := range idx {
		for _, use := range uses {
			if use.Kind == "reading" {
				readings++
				if use.Origin != "source_file" || use.Replaceable {
					t.Fatalf("reading use: %+v", use)
				}
			}
		}
	}
	if readings == 0 {
		t.Fatal("units stated by reading tables are not scanned")
	}
}
