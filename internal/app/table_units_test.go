package app

import (
	"os"
	"path/filepath"
	"testing"

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
