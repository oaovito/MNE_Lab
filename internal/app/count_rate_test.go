package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/model"
)

func TestCountRateLibrarySummaryPreservesBothParameters(t *testing.T) {
	a, _ := portableApp(t)
	if _, _, err := a.CreateAccount("Synthetic count rates", "correct horse battery staple", false); err != nil {
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
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-count-rate-types.txt"))
	if err != nil {
		t.Fatal(err)
	}
	imported := p.Import("synthetic-count-rate-types.txt", b, false)
	_, measurements, err := p.File(imported.FileID)
	if err != nil || len(measurements) != 1 {
		t.Fatal("count rate import failed")
	}
	summary := summarize(measurements[0])
	for _, key := range []string{model.CountRate, model.AverageCountRate} {
		if got, ok := summary.Params[key]; !ok || got != measurements[0].Params[key] {
			t.Fatalf("library omitted or changed %s", key)
		}
	}
	_, original, err := p.Original(imported.FileID)
	if err != nil || !bytes.Equal(original, b) {
		t.Fatal("count rate recognition changed original bytes")
	}
}
