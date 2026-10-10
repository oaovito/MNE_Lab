package export

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/xuri/excelize/v2"
)

func TestProducerOriginExportRetainsDeclarationAndUnknown(t *testing.T) {
	in, ids, _ := countRateExportInput(t)
	m := in.Measurements[ids[0]]
	f := in.Files[m.FileID]
	f.ProducerOrigin = &model.ProducerOrigin{Kind: "user_created_table", Declaration: "user_declared", Software: "Invented spreadsheet", Evidence: "=untrusted formula-looking note"}
	in.Files[m.FileID] = f
	ds := MeasurementData("Invented producer export", ids[:1], in, nil, tr)
	b, err := WriteData(ds, "json", DataOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Provenance []graph.Source `json:"provenance"`
	}
	if json.Unmarshal(b, &decoded) != nil || decoded.Provenance[0].ProducerOrigin.Evidence != f.ProducerOrigin.Evidence {
		t.Fatal("JSON lost declaration")
	}
	b, err = WriteData(ds, "txt", DataOptions{})
	if err != nil || !strings.Contains(string(b), `"declaration":"user_declared"`) || !strings.Contains(string(b), f.ProducerOrigin.Software) {
		t.Fatal("TXT lost declaration", err)
	}
	b, err = WriteData(ds, "xlsx", DataOptions{})
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	cell, err := book.GetCellValue("Provenance", "R2")
	var declared model.ProducerOrigin
	if err != nil || json.Unmarshal([]byte(cell), &declared) != nil || declared.Evidence != f.ProducerOrigin.Evidence {
		t.Fatal("XLSX lost declaration", err)
	}
	if formula, _ := book.GetCellFormula("Provenance", "R2"); formula != "" {
		t.Fatal("note became executable formula")
	}
	f.ProducerOrigin = nil
	in.Files[m.FileID] = f
	unknown := MeasurementData("Unknown origin", ids[:1], in, nil, tr).Provenance[0].ProducerOrigin
	if unknown == nil || unknown.Kind != "unknown" || unknown.Declaration != "unknown" || unknown.Vendor != "" {
		t.Fatal("legacy export guessed producer")
	}
}
