package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/oaovito/mne_lab/internal/provider"
	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/design"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/series"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
	"github.com/oaovito/mne_lab/internal/syncer"
)

var pairedTimes = []float64{0, 7, 14, 21}

// importPaired imports the synthetic owner design: formulations A, B and C,
// each as F-SR and its centrifuged C-SR aliquot at days 0, 7, 14 and 21.
// Every value is invented. It returns measurement IDs by file name.
func importPaired(t *testing.T, p *Profile) map[string]string {
	t.Helper()
	out := map[string]string{}
	n := 0
	for _, unit := range []string{"A", "B", "C"} {
		for _, cond := range []string{"F-SR", "C-SR"} {
			for _, day := range pairedTimes {
				n++
				name := fmt.Sprintf("%s %s day %g.txt", cond, unit, day)
				data := fmt.Sprintf("Sample ID: Synthetic %s %s\nMeasurement Date: 2026-09-%02d 10:%02d:00\nEffective Diameter (nm): %.2f\nPolydispersity: 0.%03d\n", cond, unit, 1+int(day), n, 150+float64(n)*1.7, 100+n)
				r := p.Import(name, []byte(data), false)
				if r.Error != "" || r.Measurements != 1 {
					t.Fatal("synthetic import failed", r)
				}
				f, _, err := p.File(r.FileID)
				if err != nil {
					t.Fatal(err)
				}
				out[name] = f.Measurements[0]
			}
		}
	}
	return out
}

func measurementBytes(t *testing.T, p *Profile) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	if err := p.St.View(func(tx *store.Tx) error {
		recs, err := tx.List(CollMeasurements)
		for _, r := range recs {
			out[r.ID] = append([]byte(nil), r.Data...)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

// pairedDesign builds the design through the confirm-a-suggestion flow:
// declare levels, ask for proposals, confirm them (creating the units the
// proposals name), then save.
func pairedDesign(t *testing.T, p *Profile, ids map[string]string) DesignView {
	t.Helper()
	draft := design.Design{Name: "Synthetic F-SR / C-SR stability",
		Conditions: []design.Condition{{Label: "F-SR"}, {Label: "C-SR", DerivedFrom: "F-SR", Process: "centrifugation"}}}
	for _, d := range pairedTimes {
		v := d
		draft.Times = append(draft.Times, design.TimeLevel{Label: fmt.Sprintf("Day %g", d), Offset: &v, Unit: "days"})
	}
	var all []string
	for _, id := range ids {
		all = append(all, id)
	}
	before, _ := p.Designs()
	sugg, err := p.SuggestDesign(draft, all)
	if err != nil || len(sugg) != 24 {
		t.Fatalf("suggestions %d %v", len(sugg), err)
	}
	if after, _ := p.Designs(); len(after) != len(before) {
		t.Fatal("a suggestion was stored without confirmation")
	}
	units := map[string]string{}
	for _, s := range sugg {
		if !s.NewUnit || s.Condition == "" || s.Time == "" {
			t.Fatalf("proposal %+v", s)
		}
		if units[s.UnitLabel] == "" {
			u, err := p.CreateExperimentalUnit(s.UnitLabel)
			if err != nil {
				t.Fatal(err)
			}
			units[s.UnitLabel] = u.ID
			draft.Units = append(draft.Units, design.Unit{ID: u.ID})
		}
		draft.Assignments = append(draft.Assignments, design.Assignment{MeasurementID: s.MeasurementID, UnitID: units[s.UnitLabel], Condition: s.Condition, Time: s.Time})
	}
	if len(units) != 3 {
		t.Fatalf("units proposed: %v", units)
	}
	draft.Pairs = design.DerivedPairs(draft)
	draft.Source = design.SourceSuggestion
	v, err := p.SaveDesign(draft, "")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func checkPairs(t *testing.T, where string, s design.Structure, labels map[string]string) {
	t.Helper()
	if s.ExperimentalUnits != 3 || s.Measurements != 24 || len(s.Pairs) != 3 {
		t.Fatalf("%s: units %d measurements %d pairs %d", where, s.ExperimentalUnits, s.Measurements, len(s.Pairs))
	}
	got := map[string]bool{}
	for _, p := range s.Pairs {
		if p.From != "F-SR" || p.To != "C-SR" || !p.Complete || len(p.Times) != 4 {
			t.Fatalf("%s: pair %+v", where, p)
		}
		got[labels[p.UnitID]] = true
	}
	if !got["A"] || !got["B"] || !got["C"] {
		t.Fatalf("%s: pairs %v", where, got)
	}
}

// B§60: 24 measurements, 3 independent units; the analysis engine refuses
// to treat the 24 as independent subjects.
func TestDesignTwentyFourMeasurementsThreeUnits(t *testing.T) {
	_, _, p := inspectionProfile(t)
	ids := importPaired(t, p)
	raw := measurementBytes(t, p)
	v := pairedDesign(t, p, ids)
	s := v.Structure
	if s.Measurements != 24 || s.ExperimentalUnits != 3 || s.Observations != 24 || s.Detected != design.StructurePairedLongitudinal || s.Recommended != design.RecommendConditionTime {
		t.Fatalf("%+v", s)
	}
	if v.Design.Source != design.SourceSuggestion || v.Design.Revision != 1 || len(v.Design.Changes) == 0 {
		t.Fatalf("%+v", v.Design)
	}
	// Saving a design never touches a measurement record.
	after := measurementBytes(t, p)
	for id, b := range raw {
		if !bytes.Equal(after[id], b) {
			t.Fatalf("measurement %s changed", id)
		}
	}
	// Observations taken from the design: declaring them independent is the
	// pseudoreplication the addendum forbids, and it is refused.
	def := analysis.Definition{Schema: 1, Module: "lightscattering", Title: "Synthetic", Variable: model.EffectiveDiameter, FactorAName: "Condition", FactorBName: "Time",
		Structure: "independent", StructureReviewed: true, Method: "two_way", Alpha: .05, PostHoc: "none"}
	for _, a := range v.Design.Assignments {
		def.Observations = append(def.Observations, analysis.ObservationDefinition{MeasurementID: a.MeasurementID, FactorA: a.Condition, FactorB: a.Time, UnitID: a.UnitID})
	}
	if snap, err := p.PrepareAnalysis(def); !errors.Is(err, analysis.ErrStructure) {
		t.Fatalf("24 measurements of 3 units accepted as independent subjects: n=%d units=%d err=%v", snap.Design.N, snap.Design.ExperimentalUnits, err)
	}
}

// B§61: the pairing F-SR X ↔ C-SR X survives save, close and reopen, and sync.
func TestDesignPairingSurvivesReopenAndSync(t *testing.T) {
	a, c, p := inspectionProfile(t)
	ids := importPaired(t, p)
	v := pairedDesign(t, p, ids)
	checkPairs(t, "saved", v.Structure, v.UnitLabels)
	profileID := p.Entry.ID

	// Close and reopen the profile.
	c.json("POST", "/api/profile/close", map[string]any{}, nil)
	c.json("POST", "/api/profiles/"+profileID+"/open", map[string]any{}, nil)
	p2, err := a.Profile()
	if err != nil || p2 == p {
		t.Fatal("profile not reopened", err)
	}
	r, err := p2.Design(v.Design.ID)
	if err != nil {
		t.Fatal(err)
	}
	checkPairs(t, "reopened", r.Structure, r.UnitLabels)

	// A second revision keeps the pairs and records exactly what changed.
	draft := r.Design
	draft.Assignments = append([]design.Assignment{}, r.Design.Assignments...)
	draft.Assignments[0].Note = "Re-read label on the vial"
	draft.Source, draft.Reason = design.SourceUser, "Note added"
	rev, err := p2.SaveDesign(draft, r.Design.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rev.Design.Revision != 2 || len(rev.Design.Changes) != 1 || rev.Design.Changes[0].Field != "note" || len(rev.History) != 2 {
		t.Fatalf("%+v", rev.Design.Changes)
	}
	checkPairs(t, "revised", rev.Structure, rev.UnitLabels)
	if _, err := p2.SaveDesign(draft, r.Design.ID); !errors.Is(err, design.ErrConflict) {
		t.Fatal("an edit of a replaced revision was accepted", err)
	}

	// Sync: copy the profile's records into one machine's store, sync it
	// through a cloud folder and read the design on another machine.
	cloud := provider.NewFolder(provider.Google, t.TempDir())
	pdk := secure.NewKey()
	open := func(dev string) *store.Store {
		st, err := store.Open(filepath.Join(t.TempDir(), "p.db"), pdk, store.Options{DeviceID: dev, Queue: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { st.Close() })
		return st
	}
	stA, stB := open("A"), open("B")
	if err := p2.St.View(func(src *store.Tx) error {
		return stA.Update(func(dst *store.Tx) error {
			for _, coll := range []string{CollFiles, CollMeasurements, CollUnits, CollDesigns, CollReadings} {
				recs, err := src.List(coll)
				if err != nil {
					return err
				}
				for _, rec := range recs {
					var v any
					if err := decodeRecord(rec, &v); err != nil {
						return err
					}
					if _, err := dst.Put(coll, rec.ID, v); err != nil {
						return err
					}
				}
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := syncer.New(stA, pdk, cloud, "acc", "prof", nil).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := syncer.New(stB, pdk, cloud, "acc", "prof", nil).Run(ctx); err != nil {
		t.Fatal(err)
	}
	pb := &Profile{St: stB}
	synced, err := pb.Design(rev.Design.ID)
	if err != nil {
		t.Fatal(err)
	}
	checkPairs(t, "synced", synced.Structure, synced.UnitLabels)
	if len(synced.History) != 2 || synced.History[1].ID != v.Design.ID {
		t.Fatal("history lost in sync")
	}
	// The series catalog on the other machine resolves the same design.
	cat, err := pb.Series(SeriesQuery{DesignID: rev.Design.ID})
	if err != nil {
		t.Fatal(err)
	}
	units := map[string]bool{}
	scalars := 0
	for _, s := range cat.Series {
		if s.Kind == series.KindScalar && s.Y.Key == model.EffectiveDiameter {
			scalars++
			units[s.ExperimentalUnit] = true
			if s.Condition == "" || s.Time == nil || s.Provenance.DesignRevision != 2 {
				t.Fatalf("%+v", s)
			}
		}
	}
	if scalars != 24 || len(units) != 3 {
		t.Fatalf("catalog: %d series, units %v", scalars, units)
	}
}

// B§62: unit A with twelve instrument readings is one experimental unit.
func TestDesignTwelveInstrumentReadingsOneUnit(t *testing.T) {
	_, _, p := inspectionProfile(t)
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-nanobrook-twelve-runs.txt"))
	if err != nil {
		t.Fatal(err)
	}
	r := p.Import("synthetic twelve runs.txt", data, false)
	if r.Measurements != 1 {
		t.Fatal(r)
	}
	f, _, _ := p.File(r.FileID)
	u, _ := p.CreateExperimentalUnit("A")
	v, err := p.SaveDesign(design.Design{Name: "Synthetic", Units: []design.Unit{{ID: u.ID}}, Conditions: []design.Condition{{Label: "F-SR"}}, Times: []design.TimeLevel{{Label: "Day 7"}},
		Assignments: []design.Assignment{{MeasurementID: f.Measurements[0], UnitID: u.ID, Condition: "F-SR", Time: "Day 7"}}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if v.Structure.ExperimentalUnits != 1 {
		t.Fatalf("%+v", v.Structure)
	}
	cat, err := p.Series(SeriesQuery{DesignID: v.Design.ID})
	if err != nil {
		t.Fatal(err)
	}
	runs := map[int]bool{}
	for _, s := range cat.Series {
		if s.Kind == series.KindReading {
			runs[s.Reading.Index] = true
			if s.ReplicateKind != design.ReplicateInstrumentReading || s.ExperimentalUnitID != u.ID || s.Reading.Parser == "" {
				t.Fatalf("%+v", s)
			}
		}
	}
	if len(runs) != 12 {
		t.Fatalf("%d readings", len(runs))
	}
}

func TestRunReadingsAreAdoptedForEarlierImports(t *testing.T) {
	_, _, p := inspectionProfile(t)
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-nanobrook-run-table.txt"))
	if err != nil {
		t.Fatal(err)
	}
	r := p.Import("synthetic run table.txt", data, false)
	var stored model.FileReadings
	p.St.View(func(tx *store.Tx) error { stored, _ = readingsTx(tx, r.FileID); return nil })
	if len(stored.Tables) != 1 || len(stored.Tables[0].Rows) != 2 || stored.Tables[0].MeasurementID == "" {
		t.Fatalf("%+v", stored)
	}
	// As if imported by an earlier build: no reading record.
	p.St.Update(func(tx *store.Tx) error { return tx.Delete(CollReadings, r.FileID) })
	raw := measurementBytes(t, p)
	if err := p.adoptRunReadings(); err != nil {
		t.Fatal(err)
	}
	var adopted model.FileReadings
	p.St.View(func(tx *store.Tx) error { adopted, _ = readingsTx(tx, r.FileID); return nil })
	if a, b := mustJSON(t, adopted), mustJSON(t, stored); a != b {
		t.Fatalf("adopted %+v", adopted)
	}
	for id, b := range measurementBytes(t, p) {
		if !bytes.Equal(raw[id], b) {
			t.Fatal("adoption changed a measurement")
		}
	}
	// Deleting the file deletes its readings.
	p.TrashFile(r.FileID, true)
	if err := p.DeleteFile(r.FileID); err != nil {
		t.Fatal(err)
	}
	p.St.View(func(tx *store.Tx) error {
		if _, ok := readingsTx(tx, r.FileID); ok {
			t.Fatal("readings outlived their file")
		}
		return nil
	})
}

func TestDesignAPIScopeAndRefusals(t *testing.T) {
	_, c, p := inspectionProfile(t)
	if code, _ := c.do("GET", "/api/series", nil, nil); code < 400 {
		t.Fatal("series catalog served without the account and profile scope")
	}
	h := map[string]string{"X-Account-ID": p.acct.ID(), "X-Profile-ID": p.Entry.ID}
	if code, body := c.do("GET", "/api/series", nil, h); code != 200 {
		t.Fatal(code, string(body))
	}
	if _, err := p.SaveDesign(design.Design{Name: "Bad", Assignments: []design.Assignment{{MeasurementID: "missing"}}}, ""); !errors.Is(err, ErrDesignMeasurement) {
		t.Fatal(err)
	}
	if _, err := p.SaveDesign(design.Design{Name: "Bad", Units: []design.Unit{{ID: "missing"}}}, ""); !errors.Is(err, design.ErrUnit) {
		t.Fatal(err)
	}
	u, _ := p.CreateExperimentalUnit("A")
	if _, err := p.RenameExperimentalUnit(u.ID, "  "); err == nil {
		t.Fatal("empty unit name accepted")
	}
	if r, err := p.RenameExperimentalUnit(u.ID, "Formulation A"); err != nil || r.ID != u.ID || r.Label != "Formulation A" {
		t.Fatal(r, err)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
