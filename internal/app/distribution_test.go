package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/graph"
)

func TestDistributionReviewPinsSavedGraphMethod(t *testing.T) {
	a, c := portableApp(t)
	r, err := c.hc.Get(a.srv.LaunchURL("/"))
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	c.json("POST", "/api/account/create", map[string]any{"name": "Synthetic Lab", "passphrase": "correct horse battery staple"}, nil)
	var pv ProfileView
	c.json("POST", "/api/profiles", map[string]any{"username": "Synthetic", "storageMode": "usb_only"}, &pv)
	c.json("POST", "/api/profiles/"+pv.ID+"/open", map[string]any{}, nil)
	p, err := a.Profile()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-nanobrook-methods.txt"))
	if err != nil {
		t.Fatal(err)
	}
	imported := p.Import("synthetic-methods.txt", b, false)
	_, ms, err := p.File(imported.FileID)
	if err != nil || len(ms) != 1 {
		t.Fatalf("import: %+v, %v", imported, err)
	}
	m := ms[0]
	def := graph.Definition{Kind: graph.KindDistribution, Measurements: []string{m.ID}, Visual: graph.DefaultVisual()}
	if _, err = p.SaveGraph(def); !errors.Is(err, ErrDistributionChoice) {
		t.Fatalf("unreviewed graph: %v", err)
	}
	bad, label := "does-not-exist", "must-not-be-written"
	if _, err = p.UpdateMeasurement(m.ID, MeasurementPatch{DistributionID: &bad, Label: &label}); !errors.Is(err, ErrDistributionChoice) {
		t.Fatalf("invalid selection: %v", err)
	}
	_, after, _ := p.File(imported.FileID)
	if after[0].Label != "" || after[0].Dist != nil {
		t.Fatal("invalid choice partially changed the measurement")
	}
	first := m.Distributions[0].ID
	if _, err = p.UpdateMeasurement(m.ID, MeasurementPatch{DistributionID: &first}); err != nil {
		t.Fatal(err)
	}
	saved, err := p.SaveGraph(def)
	if err != nil || saved.Distributions[m.ID] != first {
		t.Fatalf("pin: %+v, %v", saved, err)
	}
	old, err := p.Compute(saved)
	if err != nil {
		t.Fatal(err)
	}
	second := m.Distributions[1].ID
	if _, err = p.UpdateMeasurement(m.ID, MeasurementPatch{DistributionID: &second}); err != nil {
		t.Fatal(err)
	}
	got, err := p.Compute(saved)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Series[0].Y, old.Series[0].Y) || got.Provenance.Sources[0].Method != "lognormal" || got.Provenance.Sources[0].DistributionID != first {
		t.Fatal("changing library choice changed a saved graph")
	}
	live, err := p.Compute(def)
	if err != nil || len(live.Series[0].Y) != 4 {
		t.Fatal("new graph did not use the confirmed method")
	}
	saved.Distributions = nil
	updated, err := p.SaveGraph(saved)
	if err != nil || updated.Distributions[m.ID] != first {
		t.Fatal("editing a saved graph dropped its method pin")
	}
	_, original, err := p.Original(imported.FileID)
	if err != nil || !bytes.Equal(original, b) {
		t.Fatal("review modified the stored original")
	}
	badDef := saved
	badDef.Distributions = map[string]string{m.ID: "invalid"}
	if _, err = p.Compute(badDef); !errors.Is(err, graph.ErrNoDistribution) {
		t.Fatalf("invalid graph pin fell back: %v", err)
	}
}
