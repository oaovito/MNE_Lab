package lightscattering

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"io"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/xuri/excelize/v2"
)

func workbookFixture(t *testing.T, sheets ...string) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", sheets[0])
	for i, sheet := range sheets {
		if i > 0 {
			f.NewSheet(sheet)
		}
		rows := [][]any{
			{"Sample ID", "Synthetic " + sheet},
			{"Effective Diameter (nm)", 234.125},
			{"Date", "2026-09-18 14:30:00"},
			{"Diameter (nm)", "Intensity (%)"},
			{50.0, 10.5}, {100.0, 50.0}, {200.0, 39.5},
		}
		for row, values := range rows {
			cell, _ := excelize.CoordinatesToCellName(1, row+1)
			if err := f.SetSheetRow(sheet, cell, &values); err != nil {
				t.Fatal(err)
			}
		}
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestXLSXScientificImportPreservesSheetsAndValues(t *testing.T) {
	r, format := ParseFile("synthetic.xlsx", workbookFixture(t, "Replicate A", "Replicate B"))
	if r.Status != "parsed" || format != "xlsx" || len(r.Measurements) != 2 {
		t.Fatalf("unexpected result: %+v, format %q", r, format)
	}
	for i, m := range r.Measurements {
		wantSheet := []string{"Replicate A", "Replicate B"}[i]
		if m.SourceSheet != wantSheet || m.Params[model.EffectiveDiameter].Value != 234.125 || m.Params[model.EffectiveDiameter].Line != 2 {
			t.Fatalf("source/value provenance: %+v", m)
		}
		if m.Params[model.EffectiveDiameter].Raw != "234.125" || m.Params[model.EffectiveDiameter].Unit != "nm" || m.Dist == nil || m.Dist.FirstLine != 5 || len(m.Dist.Column("diameter").Values) != 3 {
			t.Fatalf("raw values/units/rows changed: %+v", m)
		}
		if _, exists := m.Params[model.CountRate]; exists {
			t.Fatal("missing value was invented")
		}
	}
}

func TestXLSXRejectsFormulasAndUnsupportedContainers(t *testing.T) {
	f, err := excelize.OpenReader(bytes.NewReader(workbookFixture(t, "Synthetic")))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.SetCellFormula("Synthetic", "B2", "=1+2"); err != nil {
		t.Fatal(err)
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	r, _ := ParseFile("synthetic.xlsx", b.Bytes())
	if r.Status != "failed" || r.Error != ErrSpreadsheetFormula.Error() || len(r.Measurements) != 0 {
		t.Fatalf("formula accepted: %+v", r)
	}
	for _, name := range []string{"synthetic.xls", "synthetic.ods", "synthetic.xlsm"} {
		r, _ = ParseFile(name, b.Bytes())
		if r.Error != ErrSpreadsheetUnsupported.Error() {
			t.Fatalf("%s: %+v", name, r)
		}
	}
	r, _ = ParseFile("synthetic.xlsx", []byte("not an Excel workbook"))
	if r.Error != ErrSpreadsheet.Error() {
		t.Fatalf("invalid workbook: %+v", r)
	}
}

func TestXLSXDecompressionLimitAndUnknownSheets(t *testing.T) {
	var bomb bytes.Buffer
	z := zip.NewWriter(&bomb)
	w, _ := z.Create("xl/workbook.xml")
	w.Write([]byte("<workbook/>"))
	w, _ = z.Create("large.xml")
	chunk := bytes.Repeat([]byte("x"), 1<<20)
	for i := 0; i < 65; i++ {
		w.Write(chunk)
	}
	z.Close()
	r, _ := ParseFile("synthetic.xlsx", bomb.Bytes())
	if r.Error != ErrSpreadsheetLimit.Error() {
		t.Fatalf("zip size limit not applied: %+v", r)
	}
	f, err := excelize.OpenReader(bytes.NewReader(workbookFixture(t, "Synthetic")))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.NewSheet("Notes")
	f.SetCellValue("Notes", "A1", "Synthetic unrelated notes")
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	r, _ = ParseFile("synthetic.xlsx", b.Bytes())
	if r.Status != "partial" || len(r.Measurements) != 1 || !strings.Contains(strings.Join(r.Warnings, ";"), "ls.spreadsheet_sheet_unrecognized:Notes") {
		t.Fatalf("unrecognized sheet silently ignored: %+v", r)
	}
}

func workbookRowsFixture(t *testing.T, rows [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for i, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetSheetRow("Sheet1", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	b, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestXLSXWorkbookMultilineRowProvenance(t *testing.T) {
	data := workbookRowsFixture(t, [][]any{
		{"Notes", "first line\nsecond line"},
		{"Effective Diameter (nm)", 123.456},
		{"Date", "2026-09-18 14:30:00"},
		{"Diameter (nm)", "Intensity (%)"},
		{10, 10}, {20, 20}, {30, 70},
	})
	r, _ := ParseFile("synthetic.xlsx", data)
	if len(r.Measurements) != 1 {
		t.Fatalf("measurement count %d", len(r.Measurements))
	}
	m := r.Measurements[0]
	if q := m.Params[model.EffectiveDiameter]; q.Line != 2 || m.Dist.FirstLine != 5 {
		t.Fatalf("worksheet rows lost: scalar line=%d, first distribution line=%d", q.Line, m.Dist.FirstLine)
	}
}

func TestXLSXFormulaAlternateWorksheetPath(t *testing.T) {
	for _, mode := range []string{"custom", "backslashes", "extensionless"} {
		t.Run(mode, func(t *testing.T) {
			data := workbookRowsFixture(t, [][]any{
				{"Effective Diameter (nm)", 123.456},
				{"Date", "2026-09-18 14:30:00"},
				{"Diameter (nm)", "Intensity (%)"},
				{10, 10}, {20, 20}, {30, 70},
			})
			zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			zw := zip.NewWriter(&out)
			for _, item := range zr.File {
				rd, err := item.Open()
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(rd)
				rd.Close()
				if err != nil {
					t.Fatal(err)
				}
				name := item.Name
				if name == "xl/worksheets/sheet1.xml" {
					body = bytes.Replace(body, []byte("<v>123.456</v>"), []byte("<f>1+2</f><v>123.456</v>"), 1)
					if mode == "custom" {
						name = "xl/custom.xml"
					} else if mode == "extensionless" {
						name = "xl/custom"
					} else {
						name = strings.ReplaceAll(name, "/", "\\")
					}
				}
				if mode == "custom" {
					body = bytes.ReplaceAll(body, []byte("worksheets/sheet1.xml"), []byte("custom.xml"))
				} else if mode == "extensionless" {
					body = bytes.ReplaceAll(body, []byte("worksheets/sheet1.xml"), []byte("custom"))
				}
				wr, err := zw.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := wr.Write(body); err != nil {
					t.Fatal(err)
				}
			}
			if err := zw.Close(); err != nil {
				t.Fatal(err)
			}
			r, _ := ParseFile("synthetic.xlsx", out.Bytes())
			if r.Status != "failed" || r.Error != ErrSpreadsheetFormula.Error() {
				t.Fatalf("formula cache accepted through %s worksheet path: status=%q, measurements=%d", mode, r.Status, len(r.Measurements))
			}
		})
	}
}

func TestXLSXExpandedSharedStringLimit(t *testing.T) {
	row := make([]any, 1100)
	for i := range row {
		row[i] = strings.Repeat("x", 32767)
	}
	data := workbookRowsFixture(t, [][]any{row})
	r, _ := ParseFile("synthetic.xlsx", data)
	if r.Error != ErrSpreadsheetLimit.Error() {
		t.Fatalf("expanded worksheet byte limit not checked before writing final row: status=%q, error=%q, warnings=%v", r.Status, r.Error, r.Warnings)
	}
}

func TestXLSXWorkbookCRRowProvenance(t *testing.T) {
	data := workbookRowsFixture(t, [][]any{
		{"Notes", "first line\rsecond line"},
		{"Effective Diameter (nm)", 123.456},
	})
	r, _ := ParseFile("synthetic.xlsx", data)
	if len(r.Measurements) != 1 {
		t.Fatalf("measurement count %d", len(r.Measurements))
	}
	if q := r.Measurements[0].Params[model.EffectiveDiameter]; q.Line != 2 {
		t.Fatalf("bare CR shifted worksheet scalar row to %d", q.Line)
	}
}

func TestXLSXWorkbookTextStringWriterBound(t *testing.T) {
	var b workbookText
	w := csv.NewWriter(&b)
	w.Write([]string{strings.Repeat("x", MaxFileSize+1)})
	w.Flush()
	if b.Len() > MaxFileSize {
		t.Fatalf("CSV StringWriter bypassed memory bound: buffer contains %d bytes", b.Len())
	}
}

func TestXLSXWorkbookBooleanNotScientificNumber(t *testing.T) {
	data := workbookRowsFixture(t, [][]any{
		{"Effective Diameter (nm)", true},
		{"Date", "2026-09-18 14:30:00"},
		{"Diameter (nm)", "Intensity (%)"},
		{10, 10}, {20, 20}, {30, 70},
	})
	r, _ := ParseFile("synthetic.xlsx", data)
	if len(r.Measurements) > 0 {
		if q, ok := r.Measurements[0].Params[model.EffectiveDiameter]; ok {
			t.Fatalf("boolean TRUE invented effective diameter %g %s", q.Value, q.Unit)
		}
	}
}

func TestXLSXExplicitSheetRangeSelection(t *testing.T) {
	data := workbookFixture(t, "Replicate A", "Replicate B")
	selection := &model.ImportSelection{Sheets: []model.SheetSelection{{Name: "Replicate B", Range: "a2:b2"}}}
	normalized, err := NormalizeSelection(selection)
	if err != nil || normalized.Sheets[0].Range != "A2:B2" || selection.Sheets[0].Range != "a2:b2" {
		t.Fatal("selection normalization changed caller")
	}
	r, format := ParseFileSelection("synthetic.xlsx", data, normalized)
	if format != "xlsx" || r.Status == "failed" || len(r.Measurements) != 1 || len(r.Sheets) != 2 {
		t.Fatalf("selection failed: %+v", r)
	}
	m := r.Measurements[0]
	if m.SourceSheet != "Replicate B" || m.SourceRange != "A2:B2" || m.Params[model.EffectiveDiameter].Line != 2 || m.Params[model.EffectiveDiameter].Raw != "234.125" || m.Dist != nil || m.SampleID != "" {
		t.Fatalf("selection invented metadata/changed source: %+v", m)
	}
	for _, info := range r.Sheets {
		if info.Rows != 7 || info.Columns != 2 || !info.Compatible || info.Selected != (info.Name == "Replicate B") {
			t.Fatalf("wrong full sheet inventory: %+v", info)
		}
	}
	// Starting below row 1 still preserves physical source rows, including
	// multiline cells and non-A starting columns.
	f := excelize.NewFile()
	defer f.Close()
	f.SetCellValue("Sheet1", "C4", "Effective Diameter (nm)")
	f.SetCellValue("Sheet1", "D4", "321.500")
	f.SetCellValue("Sheet1", "C5", "Synthetic unknown label")
	f.SetCellValue("Sheet1", "D5", "two\nlines")
	b, _ := f.WriteToBuffer()
	r, _ = ParseFileSelection("synthetic.xlsx", b.Bytes(), &model.ImportSelection{Sheets: []model.SheetSelection{{Name: "Sheet1", Range: "C4:D5"}}})
	if r.Status == "failed" || r.Measurements[0].Params[model.EffectiveDiameter].Line != 4 || r.Measurements[0].Params[model.EffectiveDiameter].Raw != "321.500" || r.Measurements[0].Fields[1].Line != 5 {
		t.Fatalf("non-A range lost original rows: %+v", r)
	}
}

func TestXLSXSelectionCannotBypassValidation(t *testing.T) {
	data := workbookFixture(t, "Replicate A", "Replicate B")
	invalid := []*model.ImportSelection{
		{Sheets: []model.SheetSelection{}},
		{Sheets: []model.SheetSelection{{Name: "Missing"}}},
		{Sheets: []model.SheetSelection{{Name: "Replicate A"}, {Name: "Replicate A"}}},
	}
	for _, r := range []string{"A0:B1", "B2:A1", "A2:B1", "A1", "Sheet1!A1:B2", "$A$1:B2", "A1:B100001", "A1:XFE2"} {
		invalid = append(invalid, &model.ImportSelection{Sheets: []model.SheetSelection{{Name: "Replicate A", Range: r}}})
	}
	for _, selection := range invalid {
		r, _ := ParseFileSelection("synthetic.xlsx", data, selection)
		if r.Error != ErrImportSelection.Error() || len(r.Measurements) != 0 {
			t.Fatalf("invalid selection accepted: %+v, %+v", selection, r)
		}
	}
	r, _ := ParseFileSelection("synthetic.txt", []byte("Effective Diameter (nm): 123"), &model.ImportSelection{Sheets: []model.SheetSelection{{Name: "Replicate A"}}})
	if r.Error != ErrImportSelection.Error() {
		t.Fatal("workbook selection applied to plain text")
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.SetCellFormula("Replicate A", "B2", "=1+2")
	b, _ := f.WriteToBuffer()
	r, _ = ParseFileSelection("synthetic.xlsx", b.Bytes(), &model.ImportSelection{Sheets: []model.SheetSelection{{Name: "Replicate B"}}})
	if r.Error != ErrSpreadsheetFormula.Error() || len(r.Measurements) != 0 {
		t.Fatal("formula in excluded sheet bypassed safety")
	}
}
