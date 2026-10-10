package lightscattering

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/xuri/excelize/v2"
)

const previewTableLimit = 10
const previewRowLimit = 20
const previewColumnLimit = 20
const previewCellBytes = 256

var ErrDelimitedTable = errors.New("ls.invalid_delimited_table")

// TabularPreview contains literal decoded cells, never scientific mappings.
// It is transient and bounded independently of the complete validated source.
type TabularPreview struct {
	Schema    int            `json:"schema"`
	Format    string         `json:"format"`
	Delimiter string         `json:"delimiter,omitempty"`
	Tables    []PreviewTable `json:"tables,omitempty"`
	Truncated bool           `json:"truncated"`
	Error     string         `json:"error,omitempty"`
}

type PreviewTable struct {
	Sheet       string          `json:"sheet,omitempty"`
	Range       string          `json:"range,omitempty"`
	RowCount    int             `json:"rowCount"`
	ColumnCount int             `json:"columnCount"`
	Columns     []PreviewColumn `json:"columns,omitempty"`
	Rows        []PreviewRow    `json:"rows,omitempty"`
}

type PreviewColumn struct {
	Index int    `json:"index"` // one-based physical column, not a scientific key
	Label string `json:"label"`
}

type PreviewRow struct {
	Line     int           `json:"line"`
	LastLine int           `json:"lastLine"`
	Cells    []PreviewCell `json:"cells"`
}

type PreviewCell struct {
	Column     int    `json:"column"`
	Line       int    `json:"line"`
	ByteColumn int    `json:"byteColumn,omitempty"` // CSV position in the decoded source
	Address    string `json:"address,omitempty"`    // original XLSX cell address
	Value      string `json:"value"`
}

func (p *TabularPreview) value(s string) string {
	if len(s) <= previewCellBytes {
		return s
	}
	end := previewCellBytes
	for end > 0 && !utf8.ValidString(s[:end]) {
		end--
	}
	p.Truncated = true
	return s[:end]
}

func (p *TabularPreview) workbookRow(t *PreviewTable, row []string, line int, bounds [4]int) {
	if bounds[1] != 0 && (line < bounds[1] || line > bounds[3]) {
		return
	}
	start, end := 0, len(row)
	if bounds[0] != 0 {
		start = bounds[0] - 1
		if end > bounds[2] {
			end = bounds[2]
		}
	}
	width := end - start
	if width < 0 {
		width = 0
	}
	if width > t.ColumnCount {
		t.ColumnCount = width
	}
	t.RowCount++
	if len(t.Rows) >= previewRowLimit {
		p.Truncated = true
		return
	}
	out := PreviewRow{Line: line, LastLine: line, Cells: []PreviewCell{}}
	if width > previewColumnLimit {
		end = start + previewColumnLimit
		p.Truncated = true
	}
	for column := start; column < end; column++ {
		address, _ := excelize.CoordinatesToCellName(column+1, line)
		out.Cells = append(out.Cells, PreviewCell{Column: column + 1, Line: line, Address: address, Value: p.value(row[column])})
	}
	t.Rows = append(t.Rows, out)
	// Later rows can extend the visible column inventory, including empty cells.
	count := t.ColumnCount
	if count > previewColumnLimit {
		count = previewColumnLimit
	}
	for len(t.Columns) < count {
		column := start + len(t.Columns) + 1
		label, _ := excelize.ColumnNumberToName(column)
		t.Columns = append(t.Columns, PreviewColumn{Index: column, Label: label})
	}
}

// InspectFileSelection follows the same validation and interpretation as
// ParseFileSelection, while retaining a bounded table view for manual review.
// Preview values never affect the scientific parser or the signed recipe.
func InspectFileSelection(name string, data []byte, selection *model.ImportSelection) (Result, string, *TabularPreview) {
	normalized, err := NormalizeSelection(selection)
	if err != nil {
		return spreadsheetFailure(err), "", nil
	}
	ext := strings.ToLower(filepath.Ext(name))
	if len(data) <= MaxFileSize && ext != ".xls" && ext != ".ods" && ext != ".xlsm" && !bytes.HasPrefix(data, []byte{0xd0, 0xcf, 0x11, 0xe0}) && (ext == ".xlsx" || bytes.HasPrefix(data, []byte("PK\x03\x04"))) {
		preview := &TabularPreview{Schema: 1, Format: "xlsx"}
		result := parseXLSXSelectionPreview(data, normalized, preview)
		if result.Status == "failed" && result.Error != ErrUnrecognized.Error() {
			return result, "xlsx", nil
		}
		return result, "xlsx", preview
	}
	result, format := ParseFileSelection(name, data, normalized)
	if (result.Status == "failed" && result.Error != ErrUnrecognized.Error()) || format != "nanobrook-text" {
		return result, format, nil
	}
	separator, tableFormat := rune(0), "csv"
	switch result.Delimiter {
	case "comma":
		separator = ','
	case "semicolon":
		separator = ';'
	case "tab":
		separator, tableFormat = '\t', "tsv"
	}
	// Explicit conventional extensions permit inspection of unknown headers;
	// they do not establish scientific compatibility or missing units.
	if separator == 0 {
		switch ext {
		case ".csv":
			separator = ','
		case ".tsv":
			separator, tableFormat = '\t', "tsv"
		default:
			return result, format, nil
		}
	}
	preview := &TabularPreview{Schema: 1, Format: tableFormat, Delimiter: delimName(string(separator))}
	text, _, err := decode(data)
	if err != nil {
		return result, format, nil
	}
	// This is the decoded logical value; CSV quoting is not displayed as data.
	// Unlike scientific tokenization, leading/trailing spaces remain literal.
	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma, reader.FieldsPerRecord = separator, -1
	if !withinCellLimits(text, byte(separator)) {
		preview.Error = ErrTextLimit.Error()
		return result, format, preview
	}
	table := PreviewTable{}
	for {
		values, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			preview.Error, preview.Tables = ErrDelimitedTable.Error(), nil
			return result, format, preview
		}
		table.RowCount++
		if len(values) > table.ColumnCount {
			table.ColumnCount = len(values)
		}
		if len(values) > previewColumnLimit {
			preview.Truncated = true
		}
		if len(table.Rows) >= previewRowLimit {
			preview.Truncated = true
			continue
		}
		line, _ := reader.FieldPos(0)
		out := PreviewRow{Line: line, LastLine: line, Cells: []PreviewCell{}}
		for column, value := range values {
			cellLine, byteColumn := reader.FieldPos(column)
			last := cellLine + strings.Count(value, "\n")
			if last > out.LastLine {
				out.LastLine = last
			}
			if column < previewColumnLimit {
				out.Cells = append(out.Cells, PreviewCell{Column: column + 1, Line: cellLine, ByteColumn: byteColumn, Value: preview.value(value)})
			}
		}
		table.Rows = append(table.Rows, out)
	}
	count := table.ColumnCount
	if count > previewColumnLimit {
		count = previewColumnLimit
	}
	for i := 1; i <= count; i++ {
		label, _ := excelize.ColumnNumberToName(i)
		table.Columns = append(table.Columns, PreviewColumn{Index: i, Label: label})
	}
	preview.Tables = []PreviewTable{table}
	return result, format, preview
}
