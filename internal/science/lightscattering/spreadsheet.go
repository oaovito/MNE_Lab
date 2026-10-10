package lightscattering

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/xuri/excelize/v2"
)

const maxWorkbookBytes = 64 << 20
const maxWorkbookRows = 100000
const maxWorkbookCells = 1000000

var (
	ErrSpreadsheet            = errors.New("ls.invalid_spreadsheet")
	ErrSpreadsheetFormula     = errors.New("ls.spreadsheet_formula")
	ErrSpreadsheetLimit       = errors.New("ls.spreadsheet_limit")
	ErrSpreadsheetUnsupported = errors.New("ls.unsupported_spreadsheet")
)

// ParseFile detects a supported container without changing the original bytes.
// Spreadsheet cells use the same scientific field rules as text exports.
func ParseFile(name string, data []byte) (Result, string) {
	return ParseFileSelection(name, data, nil)
}

// ParseFileSelection applies only explicit sheet/range choices. All workbook
// parts are still checked, including sheets outside the selected subset.
func ParseFileSelection(name string, data []byte, selection *model.ImportSelection) (Result, string) {
	selection, err := NormalizeSelection(selection)
	if err != nil {
		return spreadsheetFailure(err), ""
	}
	ext := strings.ToLower(filepath.Ext(name))
	if len(data) > MaxFileSize {
		return spreadsheetFailure(ErrTooLarge), ""
	}
	if ext == ".ods" || IsODSPackage(data) {
		return odsFailure(ErrSpreadsheetUnsupported), "ods"
	}
	if ext == ".xls" || ext == ".xlsm" || bytes.HasPrefix(data, []byte{0xd0, 0xcf, 0x11, 0xe0}) {
		return spreadsheetFailure(ErrSpreadsheetUnsupported), ""
	}
	if ext == ".xlsx" || bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return parseXLSXSelection(data, selection), "xlsx"
	}
	if selection != nil {
		return spreadsheetFailure(ErrImportSelection), "nanobrook-text"
	}
	return Parse(data), "nanobrook-text"
}

func spreadsheetFailure(err error) Result {
	return Result{Status: "failed", Error: err.Error(), Parser: Version, Spec: Parse(nil).Spec}
}

// validateWorkbook bounds decompression and rejects formulas/macros rather
// than evaluating them or trusting a cached result of unknown provenance.
func validateWorkbook(data []byte) error {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ErrSpreadsheet
	}
	if len(z.File) > 4096 {
		return ErrSpreadsheetLimit
	}
	var total uint64
	workbook := false
	seen := map[string]bool{}
	for _, f := range z.File {
		name := path.Clean(strings.ReplaceAll(f.Name, "\\", "/"))
		if seen[name] {
			return ErrSpreadsheet
		}
		seen[name] = true
		if f.UncompressedSize64 > maxWorkbookBytes || total > maxWorkbookBytes-f.UncompressedSize64 {
			return ErrSpreadsheetLimit
		}
		total += f.UncompressedSize64
		if name == "xl/workbook.xml" {
			workbook = true
		}
		if strings.Contains(strings.ToLower(name), "vbaproject") {
			return ErrSpreadsheetUnsupported
		}
		// Relationships may legally point to nonstandard worksheet paths.
		// Inspect every XML part rather than assuming a directory layout.
		r, err := f.Open()
		if err != nil {
			return ErrSpreadsheet
		}
		// Content types can identify XML without a filename suffix.
		body, err := io.ReadAll(io.LimitReader(r, maxWorkbookBytes+1))
		r.Close()
		if err != nil {
			return ErrSpreadsheet
		}
		trimmed := bytes.TrimSpace(bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf}))
		if !strings.HasSuffix(strings.ToLower(name), ".xml") && !bytes.HasPrefix(trimmed, []byte("<")) {
			continue
		}
		d := xml.NewDecoder(bytes.NewReader(trimmed))
		for {
			t, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return ErrSpreadsheet
			}
			if el, ok := t.(xml.StartElement); ok && el.Name.Local == "f" {
				return ErrSpreadsheetFormula
			}
		}
	}
	if !workbook {
		return ErrSpreadsheet
	}
	return nil
}

type workbookText struct{ buffer bytes.Buffer }

func (b *workbookText) Len() int      { return b.buffer.Len() }
func (b *workbookText) Bytes() []byte { return b.buffer.Bytes() }

func (b *workbookText) Write(p []byte) (int, error) {
	if len(p) > MaxFileSize-b.Len() {
		return 0, ErrSpreadsheetLimit
	}
	return b.buffer.Write(p)
}

type sheetRowSpan struct{ first, last, row int }

func restoreSheetRows(m *model.Measurement, spans []sheetRowSpan) {
	row := func(line int) int { return sourceSheetRow(spans, line) }
	for i := range m.Fields {
		m.Fields[i].Line = row(m.Fields[i].Line)
	}
	for key, q := range m.Params {
		q.Line = row(q.Line)
		m.Params[key] = q
	}
	if m.Dist != nil {
		m.Dist.FirstLine = row(m.Dist.FirstLine)
		m.Dist.LastLine = row(m.Dist.LastLine)
	}
	for i := range m.Distributions {
		d := &m.Distributions[i]
		d.FirstLine, d.LastLine = row(d.FirstLine), row(d.LastLine)
		for j := range d.SourceLines {
			d.SourceLines[j] = row(d.SourceLines[j])
		}
		for j := range d.Auxiliary {
			d.Auxiliary[j].Line = row(d.Auxiliary[j].Line)
		}
	}
}

func sourceSheetRow(spans []sheetRowSpan, line int) int {
	i := sort.Search(len(spans), func(i int) bool { return spans[i].last >= line })
	if i < len(spans) {
		return spans[i].row
	}
	return line
}

func parseXLSX(data []byte) Result { return parseXLSXSelection(data, nil) }
func parseXLSXSelection(data []byte, selection *model.ImportSelection) Result {
	return parseXLSXSelectionPreview(data, selection, nil)
}

func parseXLSXSelectionPreview(data []byte, selection *model.ImportSelection, preview *TabularPreview) Result {
	if len(data) == 0 {
		return spreadsheetFailure(ErrEmpty)
	}
	if err := validateWorkbook(data); err != nil {
		return spreadsheetFailure(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{
		RawCellValue: true, UnzipSizeLimit: maxWorkbookBytes, UnzipXMLSizeLimit: maxWorkbookBytes,
	})
	if err != nil {
		return spreadsheetFailure(ErrSpreadsheet)
	}
	defer f.Close()
	selections := map[string]model.SheetSelection{}
	known := map[string]bool{}
	for _, name := range f.GetSheetList() {
		known[name] = true
	}
	if selection != nil {
		for _, s := range selection.Sheets {
			if !known[s.Name] {
				return spreadsheetFailure(ErrImportSelection)
			}
			selections[s.Name] = s
		}
	}
	res := spreadsheetFailure(ErrUnrecognized)
	res.Encoding, res.Delimiter = "OOXML", "tab"
	recognized := map[string]bool{}
	cells := 0
	totalRows := 0
	for _, sheet := range f.GetSheetList() {
		choice, selected := selections[sheet]
		if selection == nil {
			selected = true
		}
		info := SheetInfo{Name: sheet, Selected: selected}
		var bounds [4]int
		if choice.Range != "" {
			bounds, _ = selectionBounds(choice.Range)
		}
		var table *PreviewTable
		if preview != nil && selected {
			if len(preview.Tables) < previewTableLimit {
				preview.Tables = append(preview.Tables, PreviewTable{Sheet: sheet, Range: choice.Range})
				table = &preview.Tables[len(preview.Tables)-1]
			} else {
				preview.Truncated = true
			}
		}
		rows, err := f.Rows(sheet)
		if err != nil {
			return spreadsheetFailure(ErrSpreadsheet)
		}
		var text workbookText
		w := csv.NewWriter(&text)
		w.Comma = '\t'
		n := 0
		physicalLine := 1
		var spans []sheetRowSpan
		for rows.Next() {
			n++
			totalRows++
			if totalRows > maxWorkbookRows {
				rows.Close()
				return spreadsheetFailure(ErrSpreadsheetLimit)
			}
			row, err := rows.Columns(excelize.Options{RawCellValue: true})
			if err != nil {
				rows.Close()
				return spreadsheetFailure(ErrSpreadsheet)
			}
			cells += len(row)
			if cells > maxWorkbookCells || text.Len() > MaxFileSize {
				rows.Close()
				return spreadsheetFailure(ErrSpreadsheetLimit)
			}
			// Remove padding cells without changing meaningful values.
			for len(row) > 0 && row[len(row)-1] == "" {
				row = row[:len(row)-1]
			}
			if table != nil {
				preview.workbookRow(table, row, n, bounds)
			}
			for column, value := range row {
				if value != "0" && value != "1" {
					continue
				}
				cell, _ := excelize.CoordinatesToCellName(column+1, n)
				kind, err := f.GetCellType(sheet, cell)
				if err != nil {
					rows.Close()
					return spreadsheetFailure(ErrSpreadsheet)
				}
				if kind == excelize.CellTypeBool {
					if value == "1" {
						row[column] = "TRUE"
					} else {
						row[column] = "FALSE"
					}
				}
			}
			info.Rows = n
			if len(row) > info.Columns {
				info.Columns = len(row)
			}
			if len(info.Headers) == 0 && len(row) > 0 {
				info.Headers = append([]string(nil), row...)
				if len(info.Headers) > 20 {
					info.Headers = info.Headers[:20]
				}
			}
			if choice.Range != "" {
				if n < bounds[1] || n > bounds[3] {
					continue
				}
				start, end := bounds[0]-1, bounds[2]
				if start >= len(row) {
					row = nil
				} else {
					if end > len(row) {
						end = len(row)
					}
					row = row[start:end]
				}
			}
			previous := text.Len()
			if err := w.Write(row); err != nil {
				rows.Close()
				return spreadsheetFailure(ErrSpreadsheetLimit)
			}
			w.Flush()
			if w.Error() != nil {
				rows.Close()
				return spreadsheetFailure(ErrSpreadsheetLimit)
			}
			serialized := text.Bytes()[previous:]
			lines := bytes.Count(serialized, []byte("\n")) + bytes.Count(serialized, []byte("\r")) - bytes.Count(serialized, []byte("\r\n"))
			spans = append(spans, sheetRowSpan{physicalLine, physicalLine + lines - 1, n})
			physicalLine += lines
			if physicalLine > maxWorkbookRows+1 {
				rows.Close()
				return spreadsheetFailure(ErrSpreadsheetLimit)
			}
		}
		rowErr := rows.Error()
		rows.Close()
		w.Flush()
		if rowErr != nil || w.Error() != nil {
			return spreadsheetFailure(ErrSpreadsheet)
		}
		r := Parse(text.Bytes())
		info.Compatible = r.Status != "failed"
		res.Sheets = append(res.Sheets, info)
		if !selected {
			continue
		}
		for _, warning := range r.Warnings {
			for _, code := range []string{"ls.ambiguous_number:", "ls.incomplete_distribution:"} {
				if strings.HasPrefix(warning, code) {
					if line, err := strconv.Atoi(strings.TrimPrefix(warning, code)); err == nil {
						warning = code + strconv.Itoa(sourceSheetRow(spans, line))
					}
				}
			}
			res.Warnings = append(res.Warnings, "ls.spreadsheet_sheet_warning:"+sheet+":"+warning)
		}
		if r.Status == "failed" {
			res.Warnings = append(res.Warnings, "ls.spreadsheet_sheet_unrecognized:"+sheet)
			continue
		}
		if res.Status == "failed" {
			res.Status, res.Error = r.Status, ""
		}
		if r.Status == "partial" {
			res.Status = "partial"
		}
		for _, m := range r.Measurements {
			restoreSheetRows(&m, spans)
			m.SourceSheet = sheet
			m.SourceRange = choice.Range
			m.Index = len(res.Measurements)
			res.Measurements = append(res.Measurements, m)
		}
		for _, key := range r.Recognized {
			recognized[key] = true
		}
		if res.Decimal == "" {
			res.Decimal = r.Decimal
		} else if res.Decimal != r.Decimal {
			res.Decimal = "mixed"
		}
	}
	for key := range recognized {
		res.Recognized = append(res.Recognized, key)
	}
	sort.Strings(res.Recognized)
	if len(res.Warnings) > 0 && res.Status == "parsed" {
		res.Status = "partial"
	}
	return res
}
