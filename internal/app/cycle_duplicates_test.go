package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/model"
)

func TestCyclePreviewRepeatedIDsMatchesSavedStatistics(t *testing.T) {
	a, _ := portableApp(t)
	if _, _, err := a.CreateAccount("Synthetic duplicate cycle", "correct horse battery staple", false); err != nil {
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
	for i, data := range [][]byte{raw, []byte(strings.Replace(string(raw), "83.125", "91.375", 1))} {
		imported := p.Import("synthetic-duplicate-"+string(rune('a'+i))+".txt", data, false)
		_, ms, err := p.File(imported.FileID)
		if err != nil || len(ms) != 1 {
			t.Fatal("synthetic import failed")
		}
		ids = append(ids, ms[0].ID)
		if i == 0 {
			first = ms[0]
		}
	}
	doc := CycleDoc{Config: cycle.Config{Name: "Synthetic repeated IDs", Start: first.MeasuredAt.Time, StartTZ: first.MeasuredAt.TZKnown, Interval: 1, Unit: cycle.Days, Duration: 1, Params: []string{model.EffectiveDiameter}}, Measurements: []string{ids[0], ids[0], ids[1], ids[0]}}
	preview, err := p.cycleView(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Items) != 2 || len(preview.Assignments) != 2 || preview.Series[model.EffectiveDiameter][0].Summary.N != 2 {
		t.Fatalf("duplicate IDs inflated preview: items=%d assignments=%d summary=%+v", len(preview.Items), len(preview.Assignments), preview.Series[model.EffectiveDiameter][0].Summary)
	}
	saved, err := p.SaveCycle(doc)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := p.CycleDetail(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, want := preview.Series[model.EffectiveDiameter][0].Summary, detail.Series[model.EffectiveDiameter][0].Summary
	if got.N != want.N || got.Mean == nil || want.Mean == nil || *got.Mean != 87.25 || *got.Mean != *want.Mean {
		t.Fatal("preview differs from saved statistics")
	}
	if len(doc.Measurements) != 4 {
		t.Fatal("view mutated caller's measurement list")
	}
	in, err := p.Input(ids)
	if err != nil {
		t.Fatal(err)
	}
	_, as, err := Plan(doc, in)
	if err != nil || len(as) != 2 {
		t.Fatalf("standalone plan includes duplicate IDs: %d %v", len(as), err)
	}
}
