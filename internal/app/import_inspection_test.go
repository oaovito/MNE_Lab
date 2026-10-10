package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/xuri/excelize/v2"
)

func inspectionProfile(t *testing.T) (*App, *client, *Profile) {
	t.Helper()
	a, c := portableApp(t)
	res, err := c.hc.Get(a.srv.LaunchURL("/"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	c.json("POST", "/api/account/create", map[string]any{"name": "Synthetic inspection", "passphrase": "correct horse battery staple"}, nil)
	var pv ProfileView
	c.json("POST", "/api/profiles", map[string]any{"username": "Synthetic", "storageMode": "usb_only"}, &pv)
	c.json("POST", "/api/profiles/"+pv.ID+"/open", map[string]any{}, nil)
	p, err := a.Profile()
	if err != nil {
		t.Fatal(err)
	}
	return a, c, p
}

func profileFingerprint(t *testing.T, p *Profile) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(p.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[path] = secure.HashHex(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestLiteralUnknownTableIsReadOnlyAndCannotBeConfirmed(t *testing.T) {
	_, _, p := inspectionProfile(t)
	before := profileFingerprint(t, p)
	b := []byte("unknown,value\nInvented,12.3400\n")
	r, err := p.InspectImport("invented.csv", b)
	if err != nil || r.Tabular == nil || r.Receipt != "" || r.Result.Status != "failed" || r.Measurements != 0 {
		t.Fatal("unknown scientific table misrepresented", err)
	}
	if _, err := p.ConfirmImport("invented.csv", b, r.Receipt, false); err == nil {
		t.Fatal("unknown table confirmed without scientific mapping")
	}
	if !reflect.DeepEqual(before, profileFingerprint(t, p)) {
		t.Fatal("literal preview persisted scientific or profile data")
	}
}

func TestODSLiteralInspectionIsScopedReadOnlyAndCannotBeConfirmed(t *testing.T) {
	_, c, p := inspectionProfile(t)
	before := profileFingerprint(t, p)
	data, err := os.ReadFile("../../testdata/ods-preview/invented.ods")
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"X-File-Name": "invented.ods", "X-Account-ID": p.acct.ID(), "X-Profile-ID": p.Entry.ID}
	code, body := c.do("POST", "/api/import/inspect", data, headers)
	var review ImportInspection
	if code != 200 || json.Unmarshal(body, &review) != nil || review.Tabular == nil || review.Format != "ods" || review.Module != "" || review.Result.Module != "" || review.Result.Parser != "ods-literal-reader/1.0.0" || review.Result.Spec != "" || review.Receipt != "" || review.Measurements != 0 || review.Result.Status != "failed" {
		t.Fatalf("literal ODS inspection gained scientific/import authority: status %d", code)
	}
	cell := review.Tabular.Tables[0].Rows[1].Cells[0]
	if cell.Value != "1,2300" || cell.SourceValue == nil || *cell.SourceValue != "1.2300" || cell.Address != "A2" {
		t.Fatal("HTTP preview changed declared precision or source address")
	}
	headers["X-Import-Receipt"] = "forged"
	code, _ = c.do("POST", "/api/import/confirm", data, headers)
	if code < 400 {
		t.Fatal("literal ODS confirmed with forged receipt")
	}
	wrong := map[string]string{"X-File-Name": "invented.ods", "X-Account-ID": p.acct.ID(), "X-Profile-ID": "other"}
	code, _ = c.do("POST", "/api/import/inspect", data, wrong)
	if code < 400 {
		t.Fatal("ODS preview crossed profile scope")
	}
	if !reflect.DeepEqual(before, profileFingerprint(t, p)) {
		t.Fatal("ODS inspection/rejected import changed profile or queue")
	}
	// Existing programmatic import can archive unsupported originals. It must
	// retain a failed scientific status, without inventing measurements.
	if result := p.Import("invented.ods", data, false); result.Status != "failed" || result.Measurements != 0 {
		t.Fatal("legacy archive manufactured ODS scientific data")
	}
}

func TestImportInspectionHasNoPersistentEffects(t *testing.T) {
	_, c, p := inspectionProfile(t)
	before := profileFingerprint(t, p)
	stats, err := p.St.Stats()
	if err != nil {
		t.Fatal(err)
	}
	headers := map[string]string{"X-File-Name": "synthetic.txt", "X-Account-ID": p.acct.ID(), "X-Profile-ID": p.Entry.ID}
	original := []byte("Sample ID: Synthetic\nEffective Diameter (nm): 123.450\nAverage Count Rate (kcps): 73.250\n")
	var review ImportInspection
	code, out := c.do("POST", "/api/import/inspect", original, headers)
	if code != 200 || json.Unmarshal(out, &review) != nil || review.Receipt == "" || review.Measurements != 1 {
		t.Fatalf("inspection failed: status %d", code)
	}
	if review.Result.Measurements[0].Params["effective_diameter"].Raw != "123.450" {
		t.Fatal("preview precision changed")
	}
	// Invalid and canceled previews must be equally read-only.
	code, out = c.do("POST", "/api/import/inspect", []byte("Unrecognized report"), headers)
	var invalid ImportInspection
	if code != 200 || json.Unmarshal(out, &invalid) != nil || invalid.Receipt != "" || invalid.Result.Status != "failed" {
		t.Fatal("invalid preview accepted")
	}
	for _, changed := range []string{"X-Account-ID", "X-Profile-ID"} {
		bad := map[string]string{}
		for k, v := range headers {
			bad[k] = v
		}
		bad[changed] = "different"
		if code, _ := c.do("POST", "/api/import/inspect", original, bad); code < 400 {
			t.Fatal("wrong scope accepted")
		}
	}
	for _, changed := range []string{"bytes", "name", "receipt"} {
		bad := map[string]string{}
		for k, v := range headers {
			bad[k] = v
		}
		bad["X-Import-Receipt"] = review.Receipt
		data := original
		switch changed {
		case "bytes":
			data = append(bytes.Clone(original), 'x')
		case "name":
			bad["X-File-Name"] = "other.txt"
		case "receipt":
			bad["X-Import-Receipt"] = "invalid"
		}
		if code, _ := c.do("POST", "/api/import/confirm", data, bad); code < 400 {
			t.Fatalf("changed %s accepted", changed)
		}
	}
	afterStats, err := p.St.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stats, afterStats) || !reflect.DeepEqual(before, profileFingerprint(t, p)) {
		t.Fatal("inspection/rejected confirmation changed database, blobs, queue or profile files")
	}
	headers["X-Import-Receipt"] = review.Receipt
	code, out = c.do("POST", "/api/import/confirm", original, headers)
	var imported ImportResult
	if code != 200 || json.Unmarshal(out, &imported) != nil || imported.FileID == "" {
		t.Fatal("reviewed import failed")
	}
	f, data, err := p.Original(imported.FileID)
	if err != nil || !bytes.Equal(original, data) || f.SHA256 != review.SHA256 {
		t.Fatal("confirmation changed original")
	}
	before = profileFingerprint(t, p)
	dup, err := p.InspectImport("synthetic.txt", original)
	if err != nil || dup.Existing != f.ID || dup.Receipt == "" {
		t.Fatal("duplicate not detected read-only")
	}
	repeated, err := p.ConfirmImport("synthetic.txt", original, review.Receipt, false)
	if err != nil || repeated.Status != "duplicate" || !reflect.DeepEqual(before, profileFingerprint(t, p)) {
		t.Fatal("replayed confirmation duplicated persisted data")
	}
}

func TestImportReviewScopeExpiryAndVersions(t *testing.T) {
	_, _, p := inspectionProfile(t)
	data := []byte("Effective Diameter (nm): 123.450\n")
	review, err := p.InspectImport("synthetic.txt", data)
	if err != nil || review.Receipt == "" {
		t.Fatal("missing review")
	}
	encoded := strings.Split(review.Receipt, ".")[0]
	payload, _ := base64.RawURLEncoding.DecodeString(encoded)
	var claim importClaim
	if err := json.Unmarshal(payload, &claim); err != nil {
		t.Fatal(err)
	}
	before := profileFingerprint(t, p)
	cases := []struct {
		name   string
		change func(*importClaim)
		want   error
	}{
		{"account", func(c *importClaim) { c.Account = "other" }, ErrImportScope},
		{"profile", func(c *importClaim) { c.Profile = "other" }, ErrImportScope},
		{"expiry", func(c *importClaim) { c.Expires = time.Now().Add(-time.Second).Unix() }, ErrImportReview},
		{"version", func(c *importClaim) { c.Version = 2 }, ErrImportReview},
		{"parser", func(c *importClaim) { c.Parser = "obsolete" }, ErrImportChanged},
		{"spec", func(c *importClaim) { c.Spec = "obsolete" }, ErrImportChanged},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := claim
			tc.change(&changed)
			b, _ := json.Marshal(changed)
			receipt := base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(p.importMAC(b))
			if _, err := p.ConfirmImport("synthetic.txt", data, receipt, false); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
	other := &Profile{app: &App{importSecret: secure.Token()}}
	if _, err := other.ConfirmImport("synthetic.txt", data, review.Receipt, false); !errors.Is(err, ErrImportReview) {
		t.Fatal("review survived another process secret")
	}
	if !reflect.DeepEqual(before, profileFingerprint(t, p)) {
		t.Fatal("rejected review changed profile")
	}
}

func TestImportPreviewBoundsDoNotTruncateSavedOriginal(t *testing.T) {
	_, _, p := inspectionProfile(t)
	var original strings.Builder
	for i := 0; i < 12; i++ {
		original.WriteString("Sample ID: Synthetic\nEffective Diameter (nm): 123.450\n\n")
	}
	data := []byte(original.String())
	review, err := p.InspectImport("synthetic.txt", data)
	if err != nil || review.Measurements != 12 || len(review.Result.Measurements) != 10 || !review.PreviewTruncated {
		t.Fatalf("unbounded/wrong preview: count %d, err %v", review.Measurements, err)
	}
	imported, err := p.ConfirmImport("synthetic.txt", data, review.Receipt, false)
	if err != nil || imported.Measurements != 12 {
		t.Fatalf("full import truncated: %+v %v", imported, err)
	}
	_, saved, err := p.Original(imported.FileID)
	if err != nil || !bytes.Equal(saved, data) {
		t.Fatal("original truncated")
	}
}

func TestReviewedSelectionIsBoundAndPersistsProvenance(t *testing.T) {
	_, c, p := inspectionProfile(t)
	f := excelize.NewFile()
	defer f.Close()
	f.SetCellValue("Sheet1", "A2", "Effective Diameter (nm)")
	f.SetCellValue("Sheet1", "B2", "123.450")
	f.NewSheet("Other")
	f.SetCellValue("Other", "A1", "Effective Diameter (nm)")
	f.SetCellValue("Other", "B1", "321.500")
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	data := b.Bytes()
	selection := &model.ImportSelection{Sheets: []model.SheetSelection{{Name: "Sheet1", Range: "A2:B2"}}}
	headers := map[string]string{"X-File-Name": "synthetic.xlsx", "X-Account-ID": p.acct.ID(), "X-Profile-ID": p.Entry.ID}
	encoded, _ := json.Marshal(selection)
	headers["X-Import-Selection"] = url.QueryEscape(string(encoded))
	code, out := c.do("POST", "/api/import/inspect", data, headers)
	var review ImportInspection
	if code != 200 || json.Unmarshal(out, &review) != nil || review.Receipt == "" || review.Measurements != 1 {
		t.Fatalf("inspection failed: %d", code)
	}
	before := profileFingerprint(t, p)
	headers["X-Import-Receipt"] = review.Receipt
	savedHeader := headers["X-Import-Selection"]
	delete(headers, "X-Import-Selection")
	if code, _ := c.do("POST", "/api/import/confirm", data, headers); code < 400 {
		t.Fatal("removed selection accepted")
	}
	headers["X-Import-Selection"] = url.QueryEscape(`{"sheets":[{"name":"Other"}]}`)
	if code, _ := c.do("POST", "/api/import/confirm", data, headers); code < 400 {
		t.Fatal("changed selection accepted")
	}
	headers["X-Import-Selection"] = "malformed"
	if code, _ := c.do("POST", "/api/import/inspect", data, headers); code < 400 {
		t.Fatal("malformed selection accepted")
	}
	if !reflect.DeepEqual(before, profileFingerprint(t, p)) {
		t.Fatal("rejected selections persisted")
	}
	headers["X-Import-Selection"] = savedHeader
	code, out = c.do("POST", "/api/import/confirm", data, headers)
	var imported ImportResult
	if code != 200 || json.Unmarshal(out, &imported) != nil || imported.Measurements != 1 || imported.FileID == "" {
		t.Fatalf("confirmation failed: %d", code)
	}
	file, ms, err := p.File(imported.FileID)
	if err != nil || len(ms) != 1 || ms[0].SourceRange != "A2:B2" || ms[0].SourceSheet != "Sheet1" || ms[0].Params[model.EffectiveDiameter].Line != 2 || !sameImportSelection(file.ImportSelection, selection) {
		t.Fatal("selection provenance lost")
	}
	_, original, err := p.Original(file.ID)
	if err != nil || !bytes.Equal(original, data) {
		t.Fatal("selected import altered workbook")
	}
	in := graph.Input{Files: map[string]model.SourceFile{file.ID: file}}
	source := in.Source(ms[0])
	if source.SourceRange != "A2:B2" || !sameImportSelection(source.ImportSelection, selection) {
		t.Fatal("graph/export source lost selection")
	}
	source.ImportSelection.Sheets[0].Range = "A1:B1"
	if file.ImportSelection.Sheets[0].Range != "A2:B2" {
		t.Fatal("source snapshot aliases file recipe")
	}
	// A distinct recipe on identical original bytes creates a separate dataset.
	all, err := p.InspectImport("synthetic.xlsx", data)
	if err != nil || all.Existing != "" || all.Measurements != 2 {
		t.Fatal("different recipe incorrectly considered duplicate")
	}
	importedAll, err := p.ConfirmImport("synthetic.xlsx", data, all.Receipt, false)
	if err != nil || importedAll.Measurements != 2 || importedAll.FileID == "" {
		t.Fatal("whole workbook recipe failed")
	}
	again, err := p.InspectImportSelection("synthetic.xlsx", data, selection)
	if err != nil || again.Existing != file.ID {
		t.Fatal("same original and recipe not detected duplicate")
	}
}

func TestConcurrentReviewedImportsDeduplicateAtomically(t *testing.T) {
	_, _, p := inspectionProfile(t)
	data := []byte("Effective Diameter (nm): 123.450\n")
	review, err := p.InspectImport("synthetic.txt", data)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan ImportResult, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() {
			<-start
			r, err := p.ConfirmImport("synthetic.txt", data, review.Receipt, false)
			results <- r
			errs <- err
		}()
	}
	close(start)
	saved, duplicates := 0, 0
	for i := 0; i < 8; i++ {
		r := <-results
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		if r.FileID != "" {
			saved++
		}
		if r.Status == "duplicate" {
			duplicates++
		}
	}
	files, err := p.Files(false)
	if err != nil {
		t.Fatal(err)
	}
	if saved != 1 || duplicates != 7 || len(files) != 1 || len(files[0].Items) != 1 {
		t.Fatalf("concurrent reviews saved %d files and %d duplicates", saved, duplicates)
	}
}

func TestImportPreviewBoundsLargeMetadataWithoutChangingOriginal(t *testing.T) {
	_, _, p := inspectionProfile(t)
	sample := strings.Repeat("á", 5000)
	data := []byte("Sample ID: " + sample + "\nEffective Diameter (nm): 123.450\nUnknown synthetic label: " + sample + "\n")
	review, err := p.InspectImport("synthetic.txt", data)
	if err != nil || !review.PreviewTruncated || len(review.Result.Measurements[0].SampleID) > 2048 || !utf8.ValidString(review.Result.Measurements[0].SampleID) {
		t.Fatal("large/multibyte metadata preview was not bounded safely")
	}
	for _, f := range review.Result.Measurements[0].Fields {
		if len(f.Text) > 2048 || !utf8.ValidString(f.Text) {
			t.Fatal("large field preview was not bounded")
		}
	}
	imported, err := p.ConfirmImport("synthetic.txt", data, review.Receipt, false)
	if err != nil || imported.FileID == "" {
		t.Fatal("bounded review confirmation failed")
	}
	_, ms, err := p.File(imported.FileID)
	if err != nil || ms[0].SampleID != sample {
		t.Fatal("preview truncated persisted sample metadata")
	}
	_, saved, err := p.Original(imported.FileID)
	if err != nil || !bytes.Equal(data, saved) {
		t.Fatal("large metadata original altered")
	}
}
