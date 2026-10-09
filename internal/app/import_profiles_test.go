package app

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/xuri/excelize/v2"
)

func selectionReview(t *testing.T, p *Profile) ImportInspection {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	f.SetCellValue("Sheet1", "A2", "Effective Diameter (nm)")
	f.SetCellValue("Sheet1", "B2", "123.450")
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	review, err := p.InspectImportSelection("synthetic.xlsx", b.Bytes(), &model.ImportSelection{Sheets: []model.SheetSelection{{Name: "Sheet1", Range: "A2:B2"}}})
	if err != nil || review.Receipt == "" {
		t.Fatal("missing synthetic worksheet review")
	}
	return review
}

func TestSavedImportProfileIsolationAndPersistence(t *testing.T) {
	a, c, p := inspectionProfile(t)
	review := selectionReview(t, p)
	headers := map[string]string{"X-Account-ID": p.acct.ID(), "X-Profile-ID": p.Entry.ID}
	before, err := p.St.Stats()
	if err != nil {
		t.Fatal(err)
	}
	var saved ImportProfile
	code, out := c.do("POST", "/api/import/profiles", map[string]any{"name": "Reviewed selection", "receipt": review.Receipt}, headers)
	if code != 200 || json.Unmarshal(out, &saved) != nil || saved.ID == "" || !sameImportSelection(saved.Selection, review.Selection) {
		t.Fatal("reviewed selection could not be saved")
	}
	after, err := p.St.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if after.Records != before.Records+1 || after.ByColl[CollFiles] != before.ByColl[CollFiles] || after.ByColl[CollMeasurements] != before.ByColl[CollMeasurements] || after.Blobs != before.Blobs {
		t.Fatal("saving a selection imported scientific data")
	}
	fingerprint := profileFingerprint(t, p)
	if _, err := p.SaveImportProfile("reviewed SELECTION", review.Receipt); !errors.Is(err, ErrImportProfileExists) {
		t.Fatal("duplicate name overwrote saved selection")
	}
	if !reflect.DeepEqual(fingerprint, profileFingerprint(t, p)) {
		t.Fatal("duplicate save changed persistent state")
	}
	firstID := p.Entry.ID
	var other ProfileView
	c.json("POST", "/api/profiles", map[string]any{"username": "Other synthetic", "storageMode": "usb_only"}, &other)
	c.json("POST", "/api/profiles/"+other.ID+"/open", map[string]any{}, nil)
	q, err := a.Profile()
	if err != nil {
		t.Fatal(err)
	}
	others, err := q.ImportProfiles()
	if err != nil || len(others) != 0 {
		t.Fatal("saved selections leaked into another profile")
	}
	if code, _ := c.do("POST", "/api/import/profiles", map[string]any{"name": "Wrong scope", "receipt": review.Receipt}, headers); code < 400 {
		t.Fatal("stale scope saved recipe into new profile")
	}
	headers["X-Profile-ID"] = other.ID
	if code, _ := c.do("POST", "/api/import/profiles", map[string]any{"name": "Wrong receipt", "receipt": review.Receipt}, headers); code < 400 {
		t.Fatal("cross-profile receipt saved recipe")
	}
	if code, _ := c.do("DELETE", "/api/import/profiles/"+saved.ID, nil, headers); code < 400 {
		t.Fatal("cross-profile recipe deletion succeeded")
	}
	c.json("POST", "/api/profiles/"+firstID+"/open", map[string]any{}, nil)
	reopened, err := a.Profile()
	if err != nil {
		t.Fatal(err)
	}
	recipes, err := reopened.ImportProfiles()
	if err != nil || len(recipes) != 1 || recipes[0].ID != saved.ID || recipes[0].Selection.Sheets[0].Range != "A2:B2" {
		t.Fatal("saved selection lost on profile reopen")
	}
	headers["X-Profile-ID"] = firstID
	if code, _ := c.do("DELETE", "/api/import/profiles/"+saved.ID, nil, headers); code != 204 {
		t.Fatal("recipe deletion failed")
	}
	recipes, err = reopened.ImportProfiles()
	if err != nil || len(recipes) != 0 {
		t.Fatal("deleted selection remains visible")
	}
}

func TestSavedImportProfilesRequireReviewedRecipeAndAtomicNames(t *testing.T) {
	_, _, p := inspectionProfile(t)
	before := profileFingerprint(t, p)
	for _, receipt := range []string{"", "invalid"} {
		if _, err := p.SaveImportProfile("Synthetic", receipt); !errors.Is(err, ErrImportReview) {
			t.Fatal("saved unreviewed recipe")
		}
	}
	txt, err := p.InspectImport("synthetic.txt", []byte("Effective Diameter (nm): 123.450\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.SaveImportProfile("Synthetic", txt.Receipt); !errors.Is(err, ErrImportProfileFormat) {
		t.Fatal("unsupported text recipe accepted")
	}
	review := selectionReview(t, p)
	for _, name := range []string{"", " \n", "Invalid\x00name"} {
		if _, err := p.SaveImportProfile(name, review.Receipt); !errors.Is(err, ErrImportProfileName) {
			t.Fatal("invalid name accepted")
		}
	}
	if !reflect.DeepEqual(before, profileFingerprint(t, p)) {
		t.Fatal("rejected saves changed profile")
	}
	start := make(chan struct{})
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { <-start; _, err := p.SaveImportProfile("Same title", review.Receipt); results <- err }()
	}
	close(start)
	saved, duplicates := 0, 0
	for i := 0; i < 8; i++ {
		err := <-results
		if err == nil {
			saved++
		} else if errors.Is(err, ErrImportProfileExists) {
			duplicates++
		} else {
			t.Fatal(err)
		}
	}
	recipes, err := p.ImportProfiles()
	if err != nil || saved != 1 || duplicates != 7 || len(recipes) != 1 {
		t.Fatal("concurrent saves duplicated/overwrote recipe")
	}
	// Returned decoded values are snapshots; editing them cannot alter storage.
	recipes[0].Selection.Sheets[0].Range = "A1:B1"
	again, err := p.ImportProfiles()
	if err != nil || again[0].Selection.Sheets[0].Range != "A2:B2" {
		t.Fatal("recipe read aliases persistent store")
	}
}
