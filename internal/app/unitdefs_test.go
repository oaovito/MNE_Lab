package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/units"
	"github.com/oaovito/mne_lab/internal/store"
)

// Synthetic, invented values only.
const unitTestCSV = "sample,size\nA,1.000\nA,2.000\nA,3.000\nB,4.000\nB,5.000\nB,6.000\n"

// importMapped imports the invented CSV with a user-assigned diameter unit
// and saves the reviewed mapping as a recipe.
func importMapped(t *testing.T, p *Profile, name, unit string) (model.SourceFile, []model.Measurement, ImportProfile) {
	t.Helper()
	s := appMapping()
	s.Mapping.LastRecord, s.Mapping.SampleColumn = 7, 1
	s.Mapping.Columns[0].Unit = unit
	review, err := p.InspectImportSelection(name, []byte(unitTestCSV), s)
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := p.SaveImportProfile("Recipe "+name, review.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := p.ConfirmImportSelection(name, []byte(unitTestCSV), review.Receipt, false, s)
	if err != nil {
		t.Fatal(err)
	}
	f, ms, err := p.File(imported.FileID)
	if err != nil || len(ms) != 6 {
		t.Fatal("mapped import", err)
	}
	return f, ms, recipe
}

func cycleGraph(t *testing.T, p *Profile, ms []model.Measurement) graph.Definition {
	t.Helper()
	doc := CycleDoc{Config: cycle.Config{Name: "Invented cycle", Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), StartTZ: true, Interval: 1, Unit: cycle.Days, Duration: 1}, Overrides: map[string]Override{}}
	for _, m := range ms {
		doc.Measurements = append(doc.Measurements, m.ID)
		doc.Overrides[m.ID] = Override{Point: 0}
	}
	cy, err := p.SaveCycle(doc)
	if err != nil {
		t.Fatal(err)
	}
	g, err := p.SaveGraph(graph.Definition{Kind: graph.KindParameterTime, Title: "Invented graph", CycleID: cy.ID, Param: model.EffectiveDiameter})
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func unitBySymbol(t *testing.T, p *Profile, symbol string) UnitView {
	t.Helper()
	all, err := p.MeasurementUnits(UnitQuery{IncludeArchived: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range all {
		if u.Symbol == symbol {
			return u
		}
	}
	t.Fatalf("unit %q not listed", symbol)
	return UnitView{}
}

func listed(list []UnitView, symbol string) bool {
	return slices.ContainsFunc(list, func(u UnitView) bool { return u.Symbol == symbol })
}

func measurementBytes(t *testing.T, p *Profile, id string) []byte {
	t.Helper()
	var data []byte
	if err := p.St.View(func(tx *store.Tx) error {
		r, err := tx.GetRecord(CollMeasurements, id)
		data = r.Data
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMeasurementUnitRegistryAPI(t *testing.T) {
	_, c, p := inspectionProfile(t)
	hdr := map[string]string{"X-Account-ID": p.acct.ID(), "X-Profile-ID": p.Entry.ID}
	call := func(method, path string, body any, v any) int {
		t.Helper()
		code, out := c.do(method, path, body, hdr)
		if v != nil && code < 300 {
			if err := json.Unmarshal(out, v); err != nil {
				t.Fatal(path, err, string(out))
			}
		}
		return code
	}
	if code, _ := c.do("GET", "/api/measurement-units", nil, nil); code != http.StatusBadRequest {
		t.Fatal("unscoped registry request answered", code)
	}
	var list []UnitView
	call("GET", "/api/measurement-units", nil, &list)
	if len(list) != len(units.System()) || !listed(list, "nm") || !listed(list, "µm") || !listed(list, "μm") || !listed(list, "um") {
		t.Fatal("system units", list)
	}
	var created UnitView
	if call("POST", "/api/measurement-units", map[string]any{"symbol": "Å", "name": "Invented length unit", "dimension": "length"}, &created) != 200 || created.System || created.Dimension != units.Length {
		t.Fatal("create", created)
	}
	for _, bad := range []map[string]any{
		{"symbol": "Å", "dimension": "length"},       // exists
		{"symbol": "nm", "dimension": "length"},      // system spelling
		{"symbol": "m", "dimension": "time"},         // known as a length
		{"symbol": " nm2", "dimension": "length"},    // surrounding space
		{"symbol": "x", "dimension": "mass"},         // unknown dimension
		{"symbol": "", "dimension": "dimensionless"}, // empty
	} {
		if code := call("POST", "/api/measurement-units", bad, nil); code < 400 {
			t.Fatal("invalid unit created", bad)
		}
	}
	// A diameter field never offers count-rate units.
	call("GET", "/api/measurement-units?field=effective_diameter", nil, &list)
	for _, u := range list {
		if u.Dimension != units.Length || strings.Contains(u.Symbol, "cps") {
			t.Fatal("incompatible unit offered for diameter", u)
		}
	}
	if !listed(list, "Å") || !listed(list, "nm") {
		t.Fatal("compatible units missing", list)
	}
	call("GET", "/api/measurement-units?q=%C3%A5", nil, &list)
	if len(list) != 1 || list[0].Symbol != "Å" {
		t.Fatal("search", list)
	}
	// System units are never deleted; they can be hidden and shown again.
	if call("DELETE", "/api/measurement-units/sys.kcps", nil, nil) != http.StatusBadRequest {
		t.Fatal("system unit deleted")
	}
	call("POST", "/api/measurement-units/sys.kcps/archive", nil, nil)
	call("GET", "/api/measurement-units", nil, &list)
	if listed(list, "kcps") {
		t.Fatal("hidden system unit still offered")
	}
	call("GET", "/api/measurement-units?archived=1", nil, &list)
	if !listed(list, "kcps") {
		t.Fatal("hidden system unit lost")
	}
	call("POST", "/api/measurement-units/sys.kcps/restore", nil, nil)
	call("POST", "/api/measurement-units/"+created.ID+"/favorite", map[string]bool{"favorite": true}, nil)
	call("POST", "/api/measurement-units/"+created.ID+"/used", nil, nil)
	u := unitBySymbol(t, p, "Å")
	if !u.Favorite || u.LastUsed == nil || unitBySymbol(t, p, "kcps").Archived {
		t.Fatal("favourite/recent/restore", u)
	}
	// Unused custom unit: deleted.
	var del UnitDeleteResult
	if call("DELETE", "/api/measurement-units/"+created.ID, nil, &del) != 200 || !del.Deleted {
		t.Fatal("unused unit not deleted", del)
	}
	call("GET", "/api/measurement-units?archived=1", nil, &list)
	if listed(list, "Å") {
		t.Fatal("deleted unit listed")
	}
}

func TestMeasurementUnitUseArchiveRestoreAndProtectedDelete(t *testing.T) {
	_, _, p := inspectionProfile(t)
	custom, err := p.CreateMeasurementUnit("Å", "", units.Length)
	if err != nil {
		t.Fatal(err)
	}
	file, ms, recipe := importMapped(t, p, "invented-angstrom.csv", "Å")
	g := cycleGraph(t, p, ms)
	usage, err := p.MeasurementUnitUsage(custom.ID)
	if err != nil || !usage.InUse || usage.Counts["measurement"] != 6 || usage.Counts["import_recipe"] != 1 || usage.Counts["cycle"] != 1 || usage.Counts["graph"] != 1 || !usage.Replaceable {
		t.Fatalf("usage %+v %v", usage.Counts, err)
	}
	if !slices.ContainsFunc(usage.Uses, func(u UnitUse) bool { return u.Kind == "import_recipe" && u.ID == recipe.ID }) {
		t.Fatal("recipe use missing")
	}
	// Prevent destructive delete: nothing is deleted, three options are offered.
	res, err := p.DeleteMeasurementUnit(custom.ID)
	if err != nil || res.Deleted || res.Usage == nil || !slices.Equal(res.Options, []string{"archive", "replace_then_delete", "cancel"}) {
		t.Fatal("used unit deleted", res, err)
	}
	unitBySymbol(t, p, "Å")
	// Archive: hidden from new selections, still valid for existing data.
	if _, err := p.ArchiveMeasurementUnit(custom.ID, true); err != nil {
		t.Fatal(err)
	}
	offered, _ := p.MeasurementUnits(UnitQuery{Dimension: units.Length})
	if listed(offered, "Å") {
		t.Fatal("archived unit offered")
	}
	r, err := p.Compute(g)
	if err != nil || r.Y.Unit != "Å" || len(r.Series) == 0 {
		t.Fatal("historical graph lost the archived unit", r.Y, err)
	}
	_, after, _ := p.File(file.ID)
	if after[0].Params[model.EffectiveDiameter].Unit != "Å" {
		t.Fatal("archived unit removed from data")
	}
	// Restore.
	if _, err := p.ArchiveMeasurementUnit(custom.ID, false); err != nil {
		t.Fatal(err)
	}
	offered, _ = p.MeasurementUnits(UnitQuery{Dimension: units.Length})
	if !listed(offered, "Å") {
		t.Fatal("restored unit not offered")
	}
}

func TestMeasurementUnitReplaceUsageThenDelete(t *testing.T) {
	_, _, p := inspectionProfile(t)
	custom, err := p.CreateMeasurementUnit("NM", "File spelling kept as declared", units.Length)
	if err != nil {
		t.Fatal(err)
	}
	file, ms, recipe := importMapped(t, p, "invented-upper.csv", "NM")
	g := cycleGraph(t, p, ms)
	_, original, _ := p.Original(file.ID)
	nm := unitBySymbol(t, p, "nm")
	kcps := unitBySymbol(t, p, "kcps")
	if _, err := p.ReplaceMeasurementUnit(custom.ID, kcps.ID); !errors.Is(err, ErrUnitReplacement) {
		t.Fatal("replacement across dimensions accepted", err)
	}
	if _, err := p.ReplaceMeasurementUnit("sys.nm", kcps.ID); !errors.Is(err, ErrUnitSystem) {
		t.Fatal("system unit replaced", err)
	}
	rec, err := p.ReplaceMeasurementUnit(custom.ID, nm.ID)
	if err != nil || len(rec.Changes) != 7 || rec.Unit.Symbol != "NM" || rec.Replacement.Symbol != "nm" {
		t.Fatal("replace", rec, err)
	}
	_, after, _ := p.File(file.ID)
	for i, m := range after {
		q := m.Params[model.EffectiveDiameter]
		if q.Unit != "nm" || q.UnitOrigin != "user_mapping" || q.Raw != ms[i].Params[model.EffectiveDiameter].Raw || q.Value != ms[i].Params[model.EffectiveDiameter].Value {
			t.Fatal("replacement changed a value or origin", q)
		}
	}
	f2, _, _ := p.File(file.ID)
	if f2.ImportSelection.Mapping.Columns[0].Unit != "NM" {
		t.Fatal("import declaration history rewritten")
	}
	_, original2, _ := p.Original(file.ID)
	if !bytes.Equal(original, original2) {
		t.Fatal("original changed")
	}
	recipes, _ := p.ImportProfiles()
	for _, r := range recipes {
		if r.ID == recipe.ID && r.Selection.Mapping.Columns[0].Unit != "nm" {
			t.Fatal("recipe not replaced")
		}
	}
	if r, err := p.Compute(g); err != nil || r.Y.Unit != "nm" {
		t.Fatal("graph does not follow the replacement", err)
	}
	all, _ := p.MeasurementUnits(UnitQuery{IncludeArchived: true})
	if listed(all, "NM") {
		t.Fatal("replaced unit not deleted")
	}
	provenance, _ := p.MeasurementUnitReplacements()
	if len(provenance) != 1 || provenance[0].ID != rec.ID || provenance[0].Unit.Name != "File spelling kept as declared" {
		t.Fatal("provenance", provenance)
	}
}

func TestMeasurementUnitReplaceRefusesSnapshotsAndSourceFileUnits(t *testing.T) {
	_, _, p := inspectionProfile(t)
	custom, err := p.CreateMeasurementUnit("Å", "", units.Length)
	if err != nil {
		t.Fatal(err)
	}
	_, ms, _ := importMapped(t, p, "invented-snapshot.csv", "Å")
	d := analysis.Definition{Schema: 1, Module: "lightscattering", Title: "Invented analysis", Variable: model.EffectiveDiameter, Structure: "independent", StructureReviewed: true, Method: "one_way", Alpha: .05, PostHoc: "none"}
	for i, m := range ms {
		d.Observations = append(d.Observations, analysis.ObservationDefinition{MeasurementID: m.ID, FactorA: fmt.Sprint("G", i/3)})
	}
	snapshot, err := p.PrepareAnalysis(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.St.Update(func(tx *store.Tx) error {
		_, err := tx.Put(CollAnalyses, "invented-analysis", analysis.StatisticalAnalysis{Schema: 1, ID: "invented-analysis", Snapshot: snapshot, Created: time.Now().UTC()})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	before := measurementBytes(t, p, ms[0].ID)
	nm := unitBySymbol(t, p, "nm")
	if _, err := p.ReplaceMeasurementUnit(custom.ID, nm.ID); !errors.Is(err, ErrUnitReplaceBlocked) {
		t.Fatal("saved analysis snapshot overridden", err)
	}
	if !bytes.Equal(before, measurementBytes(t, p, ms[0].ID)) {
		t.Fatal("refused replacement changed data")
	}
	res, _ := p.DeleteMeasurementUnit(custom.ID)
	if res.Deleted || res.Usage.Counts["analysis"] != 1 || res.Usage.Replaceable {
		t.Fatal("snapshot use not protected", res)
	}
	// A unit a source file states is never rewritten.
	imp := p.Import("invented-statement.txt", []byte("Sample ID: invented\nEffective Diameter (Å): 2.5\n"), false)
	if imp.Error != "" {
		t.Fatal(imp.Error)
	}
	usage, _ := p.MeasurementUnitUsage(custom.ID)
	if !slices.ContainsFunc(usage.Uses, func(u UnitUse) bool { return u.Origin == "source_file" && !u.Replaceable }) {
		t.Fatal("file-stated unit not classified", usage.Uses)
	}
	// Archive remains available and keeps everything readable.
	if _, err := p.ArchiveMeasurementUnit(custom.ID, true); err != nil {
		t.Fatal(err)
	}
	if !unitBySymbol(t, p, "Å").Archived {
		t.Fatal("archive")
	}
}

// Display conversion reads stored data and never writes it.
func TestDisplayConversionLeavesStoredBytesUnchanged(t *testing.T) {
	_, _, p := inspectionProfile(t)
	imp := p.Import("invented-display.txt", []byte("Sample ID: invented\nEffective Diameter (nm): 239.80\nCurrent Count Rate (kcps): 305.2\n"), false)
	if imp.Error != "" {
		t.Fatal(imp.Error)
	}
	f, ms, _ := p.File(imp.FileID)
	before := measurementBytes(t, p, ms[0].ID)
	_, orig, _ := p.Original(f.ID)
	c, err := units.DisplayQuantity(ms[0].Params[model.EffectiveDiameter], "µm")
	if err != nil || c.OriginalRaw != "239.80" || c.OriginalUnit != "nm" || c.DisplayValue != 0.2398 || c.DisplayUnit != "µm" {
		t.Fatal(c, err)
	}
	if _, err := units.DisplayQuantity(ms[0].Params[model.CountRate], "nm"); !errors.Is(err, units.ErrIncompatible) {
		t.Fatal("incompatible conversion", err)
	}
	_, orig2, _ := p.Original(f.ID)
	if !bytes.Equal(before, measurementBytes(t, p, ms[0].ID)) || !bytes.Equal(orig, orig2) {
		t.Fatal("conversion wrote data")
	}
}
