package app

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/oaovito/mne_lab/internal/export"
	"github.com/oaovito/mne_lab/internal/plot"
	"github.com/oaovito/mne_lab/internal/science/module"
	"github.com/oaovito/mne_lab/internal/testfixtures"
)

func TestDTSReviewedPartialImportOriginalAndIsolation(t *testing.T) {
	a, _, p := inspectionProfile(t)
	original := testfixtures.CompoundDTS("5.10")
	before := profileFingerprint(t, p)
	// The same content is recognized even when a filename has no DTS suffix.
	review, err := p.InspectImport("invented-container.dat", original)
	if err != nil || review.Result.Status != "partial" || review.Module != module.UnknownMalvern || review.Receipt == "" || review.Measurements != 0 {
		t.Fatal("partial DTS review failed", err)
	}
	if !reflect.DeepEqual(before, profileFingerprint(t, p)) {
		t.Fatal("DTS review wrote profile data")
	}
	imp, err := p.ConfirmImport("invented-container.dat", original, review.Receipt, false)
	if err != nil || imp.Status != "partial" || imp.Measurements != 0 {
		t.Fatal("partial confirmation failed", err)
	}
	f, ms, err := p.File(imp.FileID)
	if err != nil || len(ms) != 0 || f.SourceInfo == nil || f.Module != module.UnknownMalvern || f.SourceInfo.ScientificValidation != "UNVALIDATED" {
		t.Fatal("unsupported scientific quantities created", err)
	}
	_, actual, err := p.Original(f.ID)
	if err != nil || !bytes.Equal(actual, original) {
		t.Fatal("DTS original changed", err)
	}
	outs, _, err := p.produceItem(ExportItem{Kind: export.KindFile, ID: f.ID}, ExportRequest{Formats: []string{"original"}}, plot.Preset(plot.PresetScreen), false)
	if err != nil || len(outs) != 1 || !bytes.Equal(outs[0].data, original) {
		t.Fatal("original export changed bytes", err)
	}
	outs, _, err = p.produceItem(ExportItem{Kind: export.KindFile, ID: f.ID}, ExportRequest{Formats: []string{"json"}}, plot.Preset(plot.PresetScreen), false)
	if err != nil || len(outs) != 1 || outs[0].role != "source_metadata" {
		t.Fatal("metadata export failed", err)
	}
	var meta map[string]any
	if err := json.Unmarshal(outs[0].data, &meta); err != nil || meta["kind"] != "source_metadata" || meta["sourceSHA256"] != f.SHA256 || meta["normalizedMeasurements"] != float64(0) || meta["measuredAt"] != nil || meta["blobId"] != nil {
		t.Fatal("metadata export misrepresented science or provenance", err)
	}
	for _, format := range []string{"xlsx", "csv", "tsv", "txt"} {
		if _, _, err := p.produceItem(ExportItem{Kind: export.KindFile, ID: f.ID}, ExportRequest{Formats: []string{format}}, plot.Preset(plot.PresetScreen), false); err == nil || err.Error() != "dts.no_scientific_results" {
			t.Fatal("unmapped scientific data exported", format, err)
		}
	}
	duplicate, err := p.InspectImport("another-name.dts", original)
	if err != nil || duplicate.Existing != f.ID {
		t.Fatal("DTS duplicate not found", err)
	}
	if b, err := json.Marshal(f.SourceInfo); err != nil || bytes.Contains(b, []byte(`C:\invented`)) {
		t.Fatal("unmapped private path guessed into normal source metadata", err)
	}
	pv, err := a.CreateProfile(ProfileInput{Username: "Other synthetic profile", StorageMode: "usb_only"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := a.OpenProfile(pv.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := other.File(f.ID); err == nil {
		t.Fatal("DTS visible across profiles")
	}
}

func TestDTSCorruptAndUnsupportedOriginalHandling(t *testing.T) {
	_, _, p := inspectionProfile(t)
	if r, err := p.InspectImport("invented.dts", []byte("not a compound file")); err != nil || r.Receipt != "" || r.Result.Status != "failed" {
		t.Fatal("corrupt DTS accepted", err)
	}
	unknown := testfixtures.CompoundDTS("5.10")
	// Valid compound structure, but unsupported record envelope.
	unknown[5120] = 0
	r, err := p.InspectImport("unsupported.dts", unknown)
	if err != nil || r.Result.Status != "partial" || r.Module != module.UnknownCompound || r.Receipt == "" {
		t.Fatal("valid unsupported original discarded", err)
	}
	imp, err := p.ConfirmImport("unsupported.dts", unknown, r.Receipt, false)
	if err != nil || imp.Measurements != 0 {
		t.Fatal("unsupported science normalized", err)
	}
	_, actual, err := p.Original(imp.FileID)
	if err != nil || !bytes.Equal(actual, unknown) {
		t.Fatal("unsupported original not preserved", err)
	}
}
