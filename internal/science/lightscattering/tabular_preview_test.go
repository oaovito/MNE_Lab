package lightscattering

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/model"
)

func TestLiteralDelimitedPreviewPreservesPositionsAndUnknownUnits(t *testing.T) {
	b := []byte("unknown,value,notes\r\n  invented  ,1.2300,\"first\nsecond\"\r\nextra,2.4500\r\n")
	r, _, p := InspectFileSelection("invented.csv", b, nil)
	if r.Status != "failed" || p == nil || p.Error != "" || p.Truncated || len(p.Tables) != 1 {
		t.Fatal("literal unknown headers cannot be inspected", r.Error)
	}
	table := p.Tables[0]
	if table.RowCount != 3 || table.ColumnCount != 3 || table.Rows[1].Cells[0].Value != "  invented  " || table.Rows[1].Cells[1].Value != "1.2300" || table.Rows[1].Cells[2].Value != "first\nsecond" || table.Rows[1].LastLine != 3 || table.Rows[2].Line != 4 {
		t.Fatal("literal cells or physical lines changed", table)
	}
	if len(r.Measurements) != 0 {
		t.Fatal("unknown units or scientific values inferred")
	}
	_, _, malformed := InspectFileSelection("invented.csv", []byte("unknown,value\n\"unterminated,2"), nil)
	if malformed == nil || malformed.Error != ErrDelimitedTable.Error() || len(malformed.Tables) != 0 {
		t.Fatal("malformed quotes silently repaired")
	}
}

func TestLiteralPreviewDoesNotChangeScientificParsingAndBounds(t *testing.T) {
	b := []byte("Sample ID,Effective Diameter (nm)\nInvented,12.3400\n")
	want, wantFormat := ParseFileSelection("invented.csv", b, nil)
	got, format, p := InspectFileSelection("invented.csv", b, nil)
	if format != wantFormat || !reflect.DeepEqual(want, got) || p == nil {
		t.Fatal("preview changed scientific parsing")
	}
	long := []byte("unknown,value\n" + strings.Repeat("invented,1.23400\n", 30))
	_, _, p = InspectFileSelection("invented.csv", long, nil)
	if p == nil || !p.Truncated || p.Tables[0].RowCount != 31 || len(p.Tables[0].Rows) != previewRowLimit {
		t.Fatal("preview row bounds lost")
	}
	_, _, p = InspectFileSelection("invented.csv", []byte("unknown,value\n"+strings.Repeat("é", 200)+",1\n"), nil)
	if p == nil || !p.Truncated || len(p.Tables[0].Rows[1].Cells[0].Value) > previewCellBytes {
		t.Fatal("preview cell bounds lost")
	}
}

func TestLiteralWorkbookSelectionKeepsPhysicalAddressesAndValidation(t *testing.T) {
	b := workbookFixture(t, "Invented A", "Invented B")
	original := bytes.Clone(b)
	s := &model.ImportSelection{Sheets: []model.SheetSelection{{Name: "Invented B", Range: "B2:B3"}}}
	r, _, p := InspectFileSelection("invented.xlsx", b, s)
	want, _ := ParseFileSelection("invented.xlsx", b, s)
	if !reflect.DeepEqual(want, r) || p == nil || len(p.Tables) != 1 || p.Tables[0].Sheet != "Invented B" || p.Tables[0].Columns[0].Index != 2 || p.Tables[0].Rows[0].Cells[0].Address != "B2" || p.Tables[0].Rows[0].Cells[0].Value != "234.125" || !bytes.Equal(original, b) {
		t.Fatal("selected source addresses/values/parse or original changed")
	}
	if _, _, p := InspectFileSelection("invented.xlsm", b, nil); p != nil {
		t.Fatal("unsupported workbook bypassed validation")
	}
}
