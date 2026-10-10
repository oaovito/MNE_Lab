package lightscattering

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/model"
)

func workbookRecipe() *model.ImportSelection {
	return &model.ImportSelection{Mapping: &model.ColumnMapping{Schema: 2, Module: "lightscattering", Sheet: "Raw Data", HeaderRow: 2, FirstRow: 3, LastRow: 28, Decimal: "comma", SampleColumn: 1, Columns: []model.MappedColumn{{Column: 2, Key: model.EffectiveDiameter, Unit: "nm"}, {Column: 3, Key: model.AverageCountRate}, {Column: 4, Key: model.BaselineIndex}}}}
}
func inventedWorkbook(t *testing.T) []byte {
	t.Helper()
	b, e := os.ReadFile("../../../testdata/workbook-mapping/invented.xlsx")
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func rewriteWorkbook(t *testing.T, data []byte, part, old, replacement string) []byte {
	t.Helper()
	z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if e != nil {
		t.Fatal(e)
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, f := range z.File {
		r, _ := f.Open()
		body, _ := io.ReadAll(r)
		r.Close()
		if f.Name == part {
			if !bytes.Contains(body, []byte(old)) {
				t.Fatal("mutation missing")
			}
			body = bytes.Replace(body, []byte(old), []byte(replacement), 1)
		}
		writer, e := w.Create(f.Name)
		if e != nil {
			t.Fatal(e)
		}
		writer.Write(body)
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestWorkbookMappingKeepsXMLValuesCoordinatesAndAbsentRows(t *testing.T) {
	s := workbookRecipe()
	data := inventedWorkbook(t)
	r, format, p := InspectFileSelection("invented.xlsx", data, s)
	if r.Status != "partial" || r.Parser != WorkbookMappingVersion || r.Spec != WorkbookMappingSpec || format != "xlsx" || len(r.Measurements) != 26 || p == nil || !p.Truncated || len(p.Tables[0].Rows) != 20 || p.Tables[0].RowCount != 28 || len(r.Sheets) != 2 {
		t.Fatal("workbook review", r)
	}
	m := r.Measurements[0]
	q := m.Params[model.EffectiveDiameter]
	if q.Raw != "1.2300" || q.Value != 1.23 || q.Unit != "nm" || q.UnitOrigin != "user_mapping" || q.Label != "unknown size (um)" || q.SourceColumn != 2 || q.Line != 3 || m.SourceSheet != "Raw Data" || m.SourceRange != "A3:D3" || m.SampleID != "Synthetic\nsample" || m.Fields[1].Unit != "" {
		t.Fatal("raw/source/declaration", m)
	}
	if m.Params[model.AverageCountRate].Raw != "0.0000" || m.Params[model.BaselineIndex].Raw != "1,2500" || m.Params[model.BaselineIndex].Value != 1.25 || len(r.Measurements[1].Params) != 0 || len(r.Measurements[3].Params) != 0 || r.Measurements[2].Params[model.EffectiveDiameter].Raw != "0.0000" || r.Measurements[25].Params[model.EffectiveDiameter].Raw != "23.0000" {
		t.Fatal("missing/zero/full source")
	}
	for _, m := range r.Measurements {
		if m.MeasuredAt != nil || m.ExperimentalUnitID != "" || m.Dist != nil {
			t.Fatal("invented semantics")
		}
	}
	full, _ := ParseFileSelection("invented.xlsx", data, s)
	a, _ := json.Marshal(r)
	b, _ := json.Marshal(full)
	if !bytes.Equal(a, b) {
		t.Fatal("inspection/full divergence")
	}
}
func TestWorkbookMappingRejectsUnsafeOrAmbiguousSourceAtomically(t *testing.T) {
	data := inventedWorkbook(t)
	cases := []struct{ name, part, old, new string }{
		{"excluded formula", "xl/worksheets/sheet2.xml", "</row>", "<c r=\"B1\"><f>1+1</f><v>2</v></c></row>"},
		{"excluded merges", "xl/worksheets/sheet2.xml", "</worksheet>", "<mergeCells><mergeCell ref=\"A1:B1\"/></mergeCells></worksheet>"},
		{"duplicate row", "xl/worksheets/sheet1.xml", "<row r=\"4\">", "<row r=\"3\">"},
		{"duplicate cell", "xl/worksheets/sheet1.xml", "r=\"C3\"", "r=\"B3\""},
		{"wrong physical row", "xl/worksheets/sheet1.xml", "r=\"C3\"", "r=\"C4\""},
		{"excess extent", "xl/worksheets/sheet2.xml", "r=\"1\"", "r=\"100001\""},
		{"boolean", "xl/worksheets/sheet1.xml", "<c r=\"C3\">", "<c r=\"C3\" t=\"b\">"},
		{"nonfinite", "xl/worksheets/sheet1.xml", "<v>1.2300</v>", "<v>1e309</v>"},
		{"underflow", "xl/worksheets/sheet1.xml", "<v>1.2300</v>", "<v>1e-400</v>"},
		{"text decimal conflict", "xl/worksheets/sheet1.xml", "1,2500", "1.2500"},
		{"external relationship", "xl/_rels/workbook.xml.rels", "Target=\"worksheets/sheet2.xml\"", "Target=\"https://example.invalid/source\" TargetMode=\"External\""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, _, p := InspectFileSelection("invented.xlsx", rewriteWorkbook(t, data, c.part, c.old, c.new), workbookRecipe())
			if r.Status != "failed" || len(r.Measurements) != 0 || p != nil {
				t.Fatal("unsafe source partly imported", r)
			}
		})
	}
	for name, edit := range map[string]func(*model.ColumnMapping){
		"missing sheet": func(m *model.ColumnMapping) { m.Sheet = "Absent" }, "missing last row": func(m *model.ColumnMapping) { m.LastRow = 29 }, "ambiguous coordinates": func(m *model.ColumnMapping) { m.FirstRecord = 3 }, "delimiter": func(m *model.ColumnMapping) { m.Delimiter = "tab" }, "header overlap": func(m *model.ColumnMapping) { m.HeaderRow = 3 }, "column past header": func(m *model.ColumnMapping) { m.Columns[0].Column = 5 }, "unknown schema": func(m *model.ColumnMapping) { m.Schema = 3 },
	} {
		t.Run(name, func(t *testing.T) {
			s := workbookRecipe()
			edit(s.Mapping)
			r, _ := ParseFileSelection("invented.xlsx", data, s)
			if r.Status != "failed" || len(r.Measurements) != 0 {
				t.Fatal("unsafe recipe accepted")
			}
		})
	}
	for _, name := range []string{"invented.csv", "invented.ods", "invented.xls"} {
		r, _ := ParseFileSelection(name, data, workbookRecipe())
		if r.Status != "failed" {
			t.Fatal("schema used for wrong container")
		}
	}
	// A pre-existing CSV schema cannot silently acquire workbook coordinates.
	s := inventedMapping()
	s.Mapping.Sheet = "Raw Data"
	if _, e := NormalizeSelection(s); e == nil {
		t.Fatal("old recipe reinterpreted")
	}
}
func FuzzManualWorkbookMapping(f *testing.F) {
	b, e := os.ReadFile("../../../testdata/workbook-mapping/invented.xlsx")
	if e != nil {
		f.Fatal(e)
	}
	f.Add(b)
	f.Add([]byte("not a workbook"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		r, _, _ := InspectFileSelection("fuzz.xlsx", data, workbookRecipe())
		if r.Status == "failed" && len(r.Measurements) > 0 {
			t.Fatal("partial failure")
		}
		for _, m := range r.Measurements {
			if !strings.HasPrefix(m.SourceRange, "A") {
				t.Fatal("physical source missing")
			}
		}
	})
}

func TestWorkbookMappingBoundsExpandedTextAndStoredLabels(t *testing.T) {
	for _, label := range []string{strings.Repeat("L", 2<<20), strings.Repeat("&lt;", 320<<10)} {
		changed := rewriteWorkbook(t, inventedWorkbook(t), "xl/worksheets/sheet1.xml", "unknown size (um)", label)
		r, _, p := InspectFileSelection("invented.xlsx", changed, workbookRecipe())
		if r.Error != ErrSpreadsheetLimit.Error() || len(r.Measurements) != 0 || p != nil {
			t.Fatal("repeated/JSON-escaped source labels exceeded mapped byte budget without rejection")
		}
	}
	// The logical expansion on an excluded sheet must still be bounded.
	data := rewriteWorkbook(t, inventedWorkbook(t), "xl/sharedStrings.xml", "Synthetic\nsample", strings.Repeat("S", 8<<20))
	var refs strings.Builder
	for _, col := range []string{"B", "C", "D", "E", "F", "G", "H", "I", "J"} {
		refs.WriteString(`<c r="` + col + `1" t="s"><v>0</v></c>`)
	}
	data = rewriteWorkbook(t, data, "xl/worksheets/sheet2.xml", "</row>", refs.String()+"</row>")
	r, _, p := InspectFileSelection("invented.xlsx", data, workbookRecipe())
	if r.Error != ErrSpreadsheetLimit.Error() || len(r.Measurements) != 0 || p != nil {
		t.Fatal("excluded shared-string expansion accepted")
	}
}
