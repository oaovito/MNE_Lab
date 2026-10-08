package app

import (
	"bytes"
	"testing"

	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/xuri/excelize/v2"
)

func TestSpreadsheetImportKeepsExactOriginal(t *testing.T) {
	a, c := portableApp(t)
	res, err := c.hc.Get(a.srv.LaunchURL("/"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	c.json("POST", "/api/account/create", map[string]any{"name": "Synthetic Lab", "passphrase": "correct horse battery staple"}, nil)
	var pv ProfileView
	c.json("POST", "/api/profiles", map[string]any{"username": "Synthetic", "storageMode": "usb_only"}, &pv)
	c.json("POST", "/api/profiles/"+pv.ID+"/open", map[string]any{}, nil)
	f := excelize.NewFile()
	defer f.Close()
	f.SetCellValue("Sheet1", "A1", "Effective Diameter (nm)")
	f.SetCellValue("Sheet1", "B1", 123.456)
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	var imported ImportResult
	code, out := c.do("POST", "/api/files", b.Bytes(), map[string]string{"X-File-Name": "synthetic.xlsx"})
	if code >= 300 {
		t.Fatalf("import: %d", code)
	}
	if err := jsonUnmarshal(out, &imported); err != nil {
		t.Fatal(err)
	}
	p, err := a.Profile()
	if err != nil {
		t.Fatal(err)
	}
	file, original, err := p.Original(imported.FileID)
	if err != nil {
		t.Fatal(err)
	}
	if file.Format != "xlsx" || file.SHA256 != secure.HashHex(b.Bytes()) || !bytes.Equal(original, b.Bytes()) {
		t.Fatal("original workbook bytes or format provenance changed")
	}
	_, measurements, err := p.File(imported.FileID)
	if err != nil || len(measurements) != 1 || measurements[0].SourceSheet != "Sheet1" {
		t.Fatalf("sheet provenance lost: %v", err)
	}
	duplicate := p.Import("second-name.xlsx", b.Bytes(), false)
	if duplicate.Status != "duplicate" || duplicate.Existing != file.ID {
		t.Fatalf("duplicate original not detected: %+v", duplicate)
	}
}
