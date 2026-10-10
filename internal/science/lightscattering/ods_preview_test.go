package lightscattering

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/oaovito/mne_lab/internal/science/model"
)

func odsFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../../testdata/ods-preview/invented.ods")
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func rewriteODS(t *testing.T, data []byte, part string, change func(string) string, extraName, extraBody string) []byte {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		if f.Name == part {
			b = []byte(change(string(b)))
		}
		h := &zip.FileHeader{Name: f.Name, Method: zip.Store}
		dst, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = dst.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if extraName != "" {
		dst, err := w.Create(extraName)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(dst, extraBody); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestODSLiteralPreviewIndependentFixture(t *testing.T) {
	data := odsFixture(t)
	original := bytes.Clone(data)
	result, format, p := InspectFileSelection("invented.ods", data, nil)
	if format != "ods" || result.Status != "failed" || result.Error != ErrUnrecognized.Error() || len(result.Measurements) != 0 || len(result.Recognized) != 0 || p == nil || p.Error != "" || !p.Truncated || len(p.Tables) != 2 {
		t.Fatal("literal cells gained scientific meaning or unavailable preview", result.Error)
	}
	a, b := p.Tables[0], p.Tables[1]
	if result.Parser != ODSReaderVersion || result.Spec != "" {
		t.Fatal("literal ODS borrowed DLS scientific provenance")
	}
	if a.Sheet != "Invented literal cells" || a.RowCount != 5 || a.ColumnCount != 6 || b.RowCount != 21 || b.ColumnCount != 22 || len(b.Rows) != 20 || len(b.Columns) != 20 {
		t.Fatal("repeat dimensions or preview bounds changed")
	}
	for row := 1; row <= 2; row++ {
		cells := a.Rows[row].Cells
		if cells[0].Address != []string{"A2", "A3"}[row-1] || cells[0].Value != "1,2300" || cells[0].SourceValue == nil || *cells[0].SourceValue != "1.2300" || cells[0].ValueType != "float" {
			t.Fatal("display was confused with declared precision")
		}
		for column := 1; column <= 2; column++ {
			if cells[column].Value != "50%" || *cells[column].SourceValue != "0.50" || cells[column].ValueType != "percentage" {
				t.Fatal("percentage was rescaled")
			}
		}
		if *cells[3].SourceValue != "2026-01-02T03:04:05" || cells[3].Value != "02/01/2026" || *cells[4].SourceValue != "false" || *cells[5].SourceValue != "PT1H2M3S" {
			t.Fatal("date/boolean/duration was reinterpreted")
		}
	}
	if a.Rows[3].Cells[0].Value != "alpha beta  gamma\tdelta\nend\nsecond" {
		t.Fatalf("ODF whitespace: %q", a.Rows[3].Cells[0].Value)
	}
	if a.Rows[4].Cells[0].SourceValue != nil || a.Rows[4].Cells[1].SourceValue == nil || *a.Rows[4].Cells[1].SourceValue != "" || a.Rows[4].Cells[2].Value != "0.0000" {
		t.Fatal("missing/empty/zero conflated")
	}
	if len(b.Rows[0].Cells[0].Value) > previewCellBytes || !utf8.ValidString(b.Rows[0].Cells[0].Value) || !bytes.Equal(data, original) {
		t.Fatal("cell bytes or original changed")
	}
	for _, part := range []string{"content.xml", "META-INF/manifest.xml"} {
		bom := rewriteODS(t, data, part, func(s string) string { return "\ufeff" + s }, "", "")
		_, _, preview := InspectFileSelection("invented.ods", bom, nil)
		if !reflect.DeepEqual(preview, p) {
			t.Fatal("legal UTF-8 BOM changed literal preview")
		}
	}
	// Content-first inspection works through a generic ZIP name, but the same
	// source never becomes a scientific import just because its cells look familiar.
	other, format, table := InspectFileSelection("invented.zip", data, nil)
	if format != "ods" || !reflect.DeepEqual(other, result) || !reflect.DeepEqual(table, p) {
		t.Fatal("container identity depends on extension")
	}
	parsed, format := ParseFileSelection("invented.xlsx", data, nil)
	if parsed.Status != "failed" || parsed.Error != ErrSpreadsheetUnsupported.Error() || format != "ods" {
		t.Fatal("ODS entered XLSX/scientific parsing")
	}
	selection := &model.ImportSelection{Sheets: []model.SheetSelection{{Name: a.Sheet}}}
	selected, _, table := InspectFileSelection("invented.ods", data, selection)
	if table != nil || selected.Error != ErrImportSelection.Error() {
		t.Fatal("unsupported ODS recipe was silently ignored")
	}
}

func TestODSRejectsUnsafeMalformedAndUnsupportedSources(t *testing.T) {
	data := odsFixture(t)
	tests := []struct {
		name, part, old, new, extraName, extraBody string
		want                                       error
	}{
		{name: "formula with cached value", part: "content.xml", old: "office:value=\"1.2300\"", new: "table:formula=\"of:=1+1\" office:value=\"1.2300\"", want: ErrSpreadsheetFormula},
		{name: "formula beyond preview", part: "content.xml", old: "table:number-columns-repeated=\"21\"", new: "table:formula=\"of:=1+1\" table:number-columns-repeated=\"21\"", want: ErrSpreadsheetFormula},
		{name: "formula in other XML", extraName: "styles.xml", extraBody: "<x xmlns:t=\"" + odfTable + "\"><y t:formula=\"of:=1\"/></x>", want: ErrSpreadsheetFormula},
		{name: "script", part: "content.xml", old: "<office:body>", new: "<office:scripts/><office:body>", want: ErrSpreadsheetUnsupported},
		{name: "macro file", extraName: "Basic/Module.xml", extraBody: "<invented/>", want: ErrSpreadsheetUnsupported},
		{name: "external link", extraName: "settings.xml", extraBody: "<x xmlns:l=\"http://www.w3.org/1999/xlink\" l:href=\"https://example.invalid/data\"/>", want: ErrSpreadsheetUnsupported},
		{name: "external table", part: "content.xml", old: "<office:spreadsheet>", new: "<office:spreadsheet><table:table-source/>", want: ErrSpreadsheetUnsupported},
		{name: "encrypted part", part: "META-INF/manifest.xml", old: "</manifest:manifest>", new: "<manifest:encryption-data/></manifest:manifest>", want: ErrSpreadsheetUnsupported},
		{name: "zero repeat", part: "content.xml", old: "number-rows-repeated=\"2\"", new: "number-rows-repeated=\"0\"", want: ErrSpreadsheetLimit},
		{name: "fractional repeat", part: "content.xml", old: "number-rows-repeated=\"2\"", new: "number-rows-repeated=\"2.5\"", want: ErrSpreadsheetLimit},
		{name: "rows limit", part: "content.xml", old: "number-rows-repeated=\"2\"", new: "number-rows-repeated=\"100001\"", want: ErrSpreadsheetLimit},
		{name: "column repeat overflow", part: "content.xml", old: "number-columns-repeated=\"2\"", new: "number-columns-repeated=\"999999999999999999999999999\"", want: ErrSpreadsheetLimit},
		{name: "columns limit", part: "content.xml", old: "number-columns-repeated=\"21\"", new: "number-columns-repeated=\"16385\"", want: ErrSpreadsheetLimit},
		{name: "cells limit", part: "content.xml", old: "number-rows-repeated=\"21\"", new: "number-rows-repeated=\"50000\"", want: ErrSpreadsheetLimit},
		{name: "explicit space expansion", part: "content.xml", old: "text:c=\"2\"", new: "text:c=\"999999999\"", want: ErrSpreadsheetLimit},
		{name: "duplicate content part", extraName: "content.xml", extraBody: "<invented/>", want: ErrSpreadsheet},
		{name: "traversal", extraName: "../invented.xml", extraBody: "<invented/>", want: ErrSpreadsheet},
		{name: "absolute path", extraName: "/invented.xml", extraBody: "<invented/>", want: ErrSpreadsheet},
		{name: "directive", part: "content.xml", old: "<office:body>", new: "<!DOCTYPE invented><office:body>", want: ErrSpreadsheetUnsupported},
		{name: "extra XML root", part: "content.xml", old: "</office:document-content>", new: "</office:document-content><invented/>", want: ErrSpreadsheet},
		{name: "duplicate XML attribute", part: "content.xml", old: "office:value=\"1.2300\"", new: "office:value=\"1.2300\" office:value=\"9\"", want: ErrSpreadsheet},
		{name: "malformed XML", part: "content.xml", old: "</office:body>", new: "</invented:body>", want: ErrSpreadsheet},
		{name: "unsupported version", part: "content.xml", old: "office:version=\"1.3\"", new: "office:version=\"1.2\"", want: ErrSpreadsheetUnsupported},
		{name: "missing declared float", part: "content.xml", old: "office:value=\"1.2300\"", new: "", want: ErrSpreadsheet},
		{name: "unsupported currency", part: "content.xml", old: "office:value-type=\"float\"", new: "office:value-type=\"currency\"", want: ErrSpreadsheetUnsupported},
		{name: "merged cells", part: "content.xml", old: "office:value=\"1.2300\"", new: "table:number-columns-spanned=\"2\" office:value=\"1.2300\"", want: ErrSpreadsheetUnsupported},
		{name: "unsupported rich content", part: "content.xml", old: "<text:span>", new: "<text:unknown>", want: ErrSpreadsheet},
		{name: "wrong package MIME", part: "mimetype", old: odsMIME, new: "application/vnd.oasis.opendocument.text", want: ErrSpreadsheetUnsupported},
		{name: "wrong manifest MIME", part: "META-INF/manifest.xml", old: odsMIME, new: "application/vnd.oasis.opendocument.text", want: ErrSpreadsheet},
		{name: "wrong manifest namespace", part: "META-INF/manifest.xml", old: odfManifest, new: "urn:invented", want: ErrSpreadsheet},
		{name: "missing content declaration", part: "META-INF/manifest.xml", old: "manifest:full-path=\"content.xml\"", new: "manifest:full-path=\"unknown.xml\"", want: ErrSpreadsheet},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bad := rewriteODS(t, data, tc.part, func(s string) string { return strings.Replace(s, tc.old, tc.new, 1) }, tc.extraName, tc.extraBody)
			result, _, p := InspectFileSelection("invented.ods", bad, nil)
			if p != nil || result.Status != "failed" || result.Error != tc.want.Error() {
				t.Fatalf("unsafe source previewed: %v %s", p != nil, result.Error)
			}
		})
	}
}

func TestODSBoundedRepeatTextAndPackageIntegrity(t *testing.T) {
	data := odsFixture(t)
	b := rewriteODS(t, data, "content.xml", func(s string) string {
		a := strings.Index(s, "<office:spreadsheet>") + len("<office:spreadsheet>")
		z := strings.Index(s, "</office:spreadsheet>")
		return s[:a] + `<table:table table:name="Invented expansion"><table:table-column/><table:table-row table:number-rows-repeated="1000"><table:table-cell table:number-columns-repeated="1000" office:value-type="string"><text:p>` + strings.Repeat("x", 100) + `</text:p></table:table-cell></table:table-row></table:table>` + s[z:]
	}, "", "")
	r, _, p := InspectFileSelection("invented.ods", b, nil)
	if p != nil || r.Error != ErrSpreadsheetLimit.Error() {
		t.Fatal("repeated text bypassed expansion bound")
	}
	stored := rewriteODS(t, data, "", func(s string) string { return s }, "", "")
	z, _ := zip.NewReader(bytes.NewReader(stored), int64(len(stored)))
	for _, f := range z.File {
		if f.Name == "content.xml" {
			offset, _ := f.DataOffset()
			stored[offset] ^= 1
			break
		}
	}
	r, _, p = InspectFileSelection("invented.ods", stored, nil)
	if p != nil || r.Error != ErrSpreadsheet.Error() {
		t.Fatal("damaged CRC accepted")
	}
	// A table after the spreadsheet is closed is not part of that worksheet.
	b = rewriteODS(t, data, "content.xml", func(s string) string {
		return strings.Replace(s, "</office:spreadsheet>", `</office:spreadsheet><table:table table:name="Outside"><table:table-row><table:table-cell/></table:table-row></table:table>`, 1)
	}, "", "")
	r, _, p = InspectFileSelection("invented.ods", b, nil)
	if p != nil || r.Error != ErrSpreadsheetUnsupported.Error() {
		t.Fatal("table outside spreadsheet accepted")
	}
}

func FuzzODSLiteralPreview(f *testing.F) {
	f.Add([]byte("not a zip"))
	b, err := os.ReadFile("../../../testdata/ods-preview/invented.ods")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(b)
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > MaxFileSize {
			return
		}
		result, _, preview := InspectFileSelection("invented.ods", data, nil)
		if len(result.Measurements) != 0 || result.Status != "failed" {
			t.Fatal("ODS manufactured scientific data")
		}
		if preview != nil {
			encoded, err := json.Marshal(preview)
			if err != nil || len(encoded) == 0 {
				t.Fatal("invalid preview")
			}
		}
	})
}

func TestODSPackageAndHiddenStructureBounds(t *testing.T) {
	data := odsFixture(t)
	for _, mode := range []string{"compressed MIME", "MIME not first", "prefixed archive", "declared expansion"} {
		t.Run(mode, func(t *testing.T) {
			z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			files := append([]*zip.File(nil), z.File...)
			if mode == "MIME not first" {
				files[0], files[1] = files[1], files[0]
			}
			var out bytes.Buffer
			w := zip.NewWriter(&out)
			for _, f := range files {
				r, err := f.Open()
				if err != nil {
					t.Fatal(err)
				}
				b, err := io.ReadAll(r)
				r.Close()
				if err != nil {
					t.Fatal(err)
				}
				h := &zip.FileHeader{Name: f.Name, Method: zip.Store}
				if mode == "compressed MIME" && f.Name == "mimetype" {
					h.Method = zip.Deflate
				}
				if mode == "declared expansion" && f.Name == "content.xml" {
					h.UncompressedSize64 = maxWorkbookBytes + 1
					h.CompressedSize64 = 1
					dst, err := w.CreateRaw(h)
					if err != nil {
						t.Fatal(err)
					}
					dst.Write([]byte{0})
					continue
				}
				dst, err := w.CreateHeader(h)
				if err != nil {
					t.Fatal(err)
				}
				dst.Write(b)
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			b := out.Bytes()
			if mode == "prefixed archive" {
				b = append([]byte("MZ"), b...)
			}
			r, _, p := InspectFileSelection("invented.ods", b, nil)
			want := ErrSpreadsheetUnsupported
			if mode == "declared expansion" {
				want = ErrSpreadsheetLimit
			}
			if p != nil || r.Error != want.Error() {
				t.Fatal("invalid package previewed", r.Error)
			}
		})
	}
	nested := rewriteODS(t, data, "", func(s string) string { return s }, "styles.xml", strings.Repeat("<x>", 129)+strings.Repeat("</x>", 129))
	r, _, p := InspectFileSelection("invented.ods", nested, nil)
	if p != nil || r.Error != ErrSpreadsheetLimit.Error() {
		t.Fatal("hidden XML depth bypassed bound")
	}
	duplicates := rewriteODS(t, data, "content.xml", func(s string) string {
		return strings.Replace(s, "Invented bounded cells", "Invented literal cells", 1)
	}, "", "")
	r, _, p = InspectFileSelection("invented.ods", duplicates, nil)
	if p != nil || r.Error != ErrSpreadsheet.Error() {
		t.Fatal("ambiguous worksheet identity accepted")
	}
}
