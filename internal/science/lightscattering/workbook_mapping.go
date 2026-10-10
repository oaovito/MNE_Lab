package lightscattering

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/xuri/excelize/v2"
)

// A workbook mapping reads the complete bounded source, including excluded
// sheets. Formulas/macros are rejected before any quantities are constructed.
// No formatting, header recognition, unit conversion or date inference applies.
func parseMappedWorkbook(name string, data []byte, m *model.ColumnMapping, inspect bool) (Result, string, *TabularPreview) {
	fail := func(err error) (Result, string, *TabularPreview) {
		r := mappingFailure(err, "OOXML")
		r.Parser, r.Spec = WorkbookMappingVersion, WorkbookMappingSpec
		return r, "xlsx", nil
	}
	if !strings.EqualFold(filepath.Ext(name), ".xlsx") || IsODSPackage(data) {
		return fail(ErrColumnMapping)
	}
	if len(data) > MaxFileSize {
		return fail(ErrTooLarge)
	}
	if err := validateMappedWorkbook(data); err != nil {
		return fail(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{RawCellValue: true, UnzipSizeLimit: maxWorkbookBytes, UnzipXMLSizeLimit: maxWorkbookBytes})
	if err != nil {
		return fail(ErrSpreadsheet)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) > 256 {
		return fail(ErrSpreadsheetLimit)
	}
	found := false
	for _, sheet := range sheets {
		found = found || sheet == m.Sheet
	}
	if !found {
		return fail(ErrColumnMapping)
	}
	result := Result{Status: "partial", Parser: WorkbookMappingVersion, Spec: WorkbookMappingSpec, Encoding: "OOXML", Decimal: m.Decimal, Measurements: []model.Measurement{}, Recognized: []string{}, Warnings: []string{"ls.user_mapping_unvalidated"}}
	preview := &TabularPreview{Schema: 1, Format: "xlsx"}
	cells, totalRows := 0, 0
	have := map[string]bool{}
	for _, sheet := range sheets {
		selected := sheet == m.Sheet
		info := SheetInfo{Name: sheet, Selected: selected}
		var table *PreviewTable
		if inspect && selected {
			preview.Tables = append(preview.Tables, PreviewTable{Sheet: sheet})
			table = &preview.Tables[len(preview.Tables)-1]
		}
		rows, err := f.Rows(sheet)
		if err != nil {
			return fail(ErrSpreadsheet)
		}
		var headers []string
		n := 0
		readErr := func() error {
			for rows.Next() {
				n++
				totalRows++
				if totalRows > maxWorkbookRows {
					return ErrSpreadsheetLimit
				}
				values, err := rows.Columns(excelize.Options{RawCellValue: true})
				if err != nil {
					return ErrSpreadsheet
				}
				cells += len(values)
				if cells > maxWorkbookCells || len(values) > 16384 {
					return ErrSpreadsheetLimit
				}
				info.Rows = n
				if len(values) > info.Columns {
					info.Columns = len(values)
				}
				if len(info.Headers) == 0 && len(values) > 0 {
					end := len(values)
					if end > 20 {
						end = 20
					}
					info.Headers = append([]string(nil), values[:end]...)
				}
				if table != nil {
					preview.workbookRow(table, values, n, [4]int{})
				}
				if !selected {
					continue
				}
				if n == m.HeaderRow {
					headers = append([]string{}, values...)
					if m.SampleColumn > len(headers) {
						return ErrColumnMapping
					}
					for _, c := range m.Columns {
						if c.Column > len(headers) {
							return ErrColumnMapping
						}
					}
				}
				if n < m.FirstRow || n > m.LastRow {
					continue
				}
				if headers == nil {
					return ErrColumnMapping
				}
				lastColumn := m.SampleColumn
				for _, c := range m.Columns {
					if c.Column > lastColumn {
						lastColumn = c.Column
					}
				}
				first, _ := excelize.CoordinatesToCellName(1, n)
				last, _ := excelize.CoordinatesToCellName(lastColumn, n)
				measurement := model.Measurement{Index: len(result.Measurements), SourceSheet: sheet, SourceRange: first + ":" + last, Params: map[string]model.Quantity{}, Fields: []model.Field{}, Parser: WorkbookMappingVersion, Spec: WorkbookMappingSpec}
				if m.SampleColumn > 0 && m.SampleColumn <= len(values) {
					measurement.SampleID = values[m.SampleColumn-1]
				}
				for i, raw := range values {
					label := ""
					if i < len(headers) {
						label = headers[i]
					}
					measurement.Fields = append(measurement.Fields, model.Field{Label: label, Text: raw, Line: n})
				}
				for _, c := range m.Columns {
					if c.Column > len(values) || strings.TrimSpace(values[c.Column-1]) == "" {
						continue
					}
					address, _ := excelize.CoordinatesToCellName(c.Column, n)
					kind, err := f.GetCellType(sheet, address)
					if err != nil {
						return ErrSpreadsheet
					}
					decimal := m.Decimal
					switch kind {
					case excelize.CellTypeNumber, excelize.CellTypeUnset:
						decimal = "dot" // OOXML numerical values are locale-independent.
					case excelize.CellTypeInlineString, excelize.CellTypeSharedString:
					default:
						return ErrMappedNumber // Boolean, date, error or formula/string-result.
					}
					raw := values[c.Column-1]
					value, err := mappedNumber(raw, decimal)
					if err != nil {
						return err
					}
					measurement.Params[c.Key] = model.Quantity{Value: value, Raw: raw, Unit: c.Unit, UnitOrigin: "user_mapping", SourceColumn: c.Column, Label: headers[c.Column-1], Line: n}
					measurement.Fields[c.Column-1].Key = c.Key
					measurement.Fields[c.Column-1].Num = &value
					have[c.Key] = true
				}
				result.Measurements = append(result.Measurements, measurement)
			}
			return rows.Error()
		}()
		rows.Close()
		if readErr != nil {
			return fail(readErr)
		}
		if selected && n < m.LastRow {
			return fail(ErrColumnMapping)
		}
		info.Compatible = selected && len(have) > 0
		result.Sheets = append(result.Sheets, info)
	}
	if len(have) == 0 {
		return fail(ErrUnrecognized)
	}
	for key := range have {
		result.Recognized = append(result.Recognized, key)
	}
	sort.Strings(result.Recognized)
	if !inspect {
		return result, "xlsx", nil
	}
	return result, "xlsx", preview
}

// Require unambiguous physical coordinates before the OOXML library can
// normalize cells. Check all parts, including sheets excluded by the recipe.
// Merged/linked workbooks and rows without explicit coordinates are outside
// this reader's supported subset; no stored-value fallback is attempted.
func validateMappedWorkbook(data []byte) error {
	if err := validateWorkbook(data); err != nil {
		return err
	}
	z, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	for _, part := range z.File {
		reader, err := part.Open()
		if err != nil {
			return ErrSpreadsheet
		}
		body, err := io.ReadAll(io.LimitReader(reader, maxWorkbookBytes+1))
		reader.Close()
		if err != nil {
			return ErrSpreadsheet
		}
		body = bytes.TrimSpace(bytes.TrimPrefix(body, []byte{0xef, 0xbb, 0xbf}))
		if !bytes.HasPrefix(body, []byte("<")) {
			continue
		}
		d := xml.NewDecoder(bytes.NewReader(body))
		worksheet, row, previousRow, previousColumn := false, 0, 0, 0
		for {
			token, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return ErrSpreadsheet
			}
			switch el := token.(type) {
			case xml.Directive:
				return ErrSpreadsheetUnsupported
			case xml.StartElement:
				if el.Name.Local == "worksheet" {
					worksheet = true
				}
				if el.Name.Local == "mergeCell" || el.Name.Local == "externalLink" {
					return ErrSpreadsheetUnsupported
				}
				for _, a := range el.Attr {
					if a.Name.Local == "TargetMode" && strings.EqualFold(a.Value, "External") {
						return ErrSpreadsheetUnsupported
					}
				}
				if !worksheet {
					continue
				}
				attr := func(name string) string {
					for _, a := range el.Attr {
						if a.Name.Local == name {
							return a.Value
						}
					}
					return ""
				}
				if el.Name.Local == "row" {
					if row != 0 {
						return ErrSpreadsheet
					}
					n, err := strconv.Atoi(attr("r"))
					if err != nil || n <= previousRow {
						return ErrSpreadsheet
					}
					if n > maxWorkbookRows {
						return ErrSpreadsheetLimit
					}
					row, previousRow, previousColumn = n, n, 0
				}
				if el.Name.Local == "c" {
					column, cellRow, err := excelize.CellNameToCoordinates(attr("r"))
					if err != nil || row == 0 || cellRow != row || column <= previousColumn {
						return ErrSpreadsheet
					}
					if column > 16384 {
						return ErrSpreadsheetLimit
					}
					previousColumn = column
				}
			case xml.EndElement:
				if worksheet && el.Name.Local == "row" {
					row = 0
				}
			}
		}
	}
	return nil
}
