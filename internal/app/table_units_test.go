package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/store"
)

func TestOpenProfileCompletesUnitsFromRunTable(t *testing.T) {
	a, _ := portableApp(t)
	if _, _, err := a.CreateAccount("Synthetic run table", "correct horse battery staple", false); err != nil {
		t.Fatal(err)
	}
	pv, err := a.CreateProfile(ProfileInput{Username: "Synthetic", StorageMode: "usb_only"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.OpenProfile(pv.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-nanobrook-run-table.txt"))
	if err != nil {
		t.Fatal(err)
	}
	imported := p.Import("synthetic-nanobrook-run-table.txt", b, false)
	f, ms, err := p.File(imported.FileID)
	if err != nil || len(ms) != 1 || ms[0].Params[model.EffectiveDiameter].Unit != "nm" {
		t.Fatalf("new import lacks the table unit: %+v", ms)
	}
	if f.UnitsReviewed != "" {
		t.Fatal("a current import needs no review")
	}

	// The same report as an earlier parser stored it: no unit.
	m := ms[0]
	want := m.Params[model.EffectiveDiameter]
	q := want
	q.Unit, q.UnitOrigin, q.UnitLine = "", "", 0
	m.Params[model.EffectiveDiameter] = q
	m.Parser, f.Parser = "lightscattering-parser/1.5.0", "lightscattering-parser/1.5.0"
	if err := p.St.Update(func(t *store.Tx) error {
		if _, err := t.Put(CollMeasurements, m.ID, m); err != nil {
			return err
		}
		_, err := t.Put(CollFiles, f.ID, f)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.adoptTableUnits(); err != nil {
		t.Fatal(err)
	}
	f, ms, _ = p.File(imported.FileID)
	if got := ms[0].Params[model.EffectiveDiameter]; got != want {
		t.Fatalf("unit not completed: %+v, want %+v", got, want)
	}
	if ms[0].Parser != "lightscattering-parser/1.5.0" || f.Parser != "lightscattering-parser/1.5.0" || f.UnitsReviewed != lightscattering.Version {
		t.Fatal("import provenance rewritten or review not recorded")
	}
	if pd := ms[0].Params[model.Polydispersity]; pd.Unit != "" {
		t.Fatal("a dimensionless value got a unit")
	}
}

func TestCycleViewNeverAveragesDifferentUnits(t *testing.T) {
	a, _ := portableApp(t)
	if _, _, err := a.CreateAccount("Synthetic cycle units", "correct horse battery staple", false); err != nil {
		t.Fatal(err)
	}
	pv, err := a.CreateProfile(ProfileInput{Username: "Synthetic", StorageMode: "usb_only"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.OpenProfile(pv.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-count-rate-types.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	var first model.Measurement
	for i, data := range [][]byte{raw, raw} {
		imported := p.Import("synthetic-units-"+string(rune('a'+i))+".txt", data, true)
		_, ms, err := p.File(imported.FileID)
		if err != nil || len(ms) != 1 {
			t.Fatal("synthetic import failed")
		}
		ids = append(ids, ms[0].ID)
		first = ms[0]
	}
	doc := CycleDoc{Config: cycle.Config{Name: "Synthetic units", Start: first.MeasuredAt.Time, StartTZ: first.MeasuredAt.TZKnown, Interval: 1, Unit: cycle.Days, Duration: 1, Params: []string{model.EffectiveDiameter}}, Measurements: ids}
	v, err := p.cycleView(doc)
	if err != nil || v.Units[model.EffectiveDiameter] != first.Params[model.EffectiveDiameter].Unit || len(v.MixedUnits) != 0 {
		t.Fatalf("shared unit: %v %+v %v", err, v.Units, v.MixedUnits)
	}
	m := first
	q := m.Params[model.EffectiveDiameter]
	q.Unit = "µm"
	m.Params[model.EffectiveDiameter] = q
	if err := p.St.Update(func(t *store.Tx) error { _, err := t.Put(CollMeasurements, m.ID, m); return err }); err != nil {
		t.Fatal(err)
	}
	v, err = p.cycleView(doc)
	if err != nil || len(v.MixedUnits) != 1 || v.Series[model.EffectiveDiameter] != nil {
		t.Fatalf("values in two units were summarized: %v %+v", err, v.Series)
	}
}
