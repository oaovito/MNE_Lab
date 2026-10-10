package lightscattering

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/oaovito/mne_lab/internal/science/model"
)

const MappingVersion = "dls-column-reader/1.0.0"
const WorkbookMappingVersion = "dls-workbook-column-reader/1.0.1"
const WorkbookMappingSpec = "dls-user-workbook-mapping/1-unvalidated"

// MappingIdentity keeps existing CSV recipes/receipts version-bound.
func MappingIdentity(m *model.ColumnMapping) (string, string) {
	if m != nil && m.Schema == 2 {
		return WorkbookMappingVersion, WorkbookMappingSpec
	}
	return MappingVersion, MappingSpec
}

const MappingSpec = "dls-user-column-mapping/1-unvalidated"

var ErrColumnMapping = errors.New("ls.invalid_column_mapping")
var ErrMappedNumber = errors.New("ls.invalid_mapped_number")
var mappingNumber = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)

func normalizeMapping(in *model.ImportSelection) (*model.ImportSelection, error) {
	m := in.Mapping
	if len(in.Sheets) != 0 || m.Module != "lightscattering" || (m.Decimal != "dot" && m.Decimal != "comma") || len(m.Columns) == 0 || len(m.Columns) > 5 {
		return nil, ErrColumnMapping
	}
	maxColumn := maxTextColumns
	switch m.Schema {
	case 1:
		if m.Sheet != "" || m.HeaderRow != 0 || m.FirstRow != 0 || m.LastRow != 0 || (m.Delimiter != "comma" && m.Delimiter != "semicolon" && m.Delimiter != "tab") || m.HeaderRecord < 1 || m.FirstRecord <= m.HeaderRecord || m.LastRecord < m.FirstRecord || m.LastRecord > maxTextLines {
			return nil, ErrColumnMapping
		}
	case 2:
		maxColumn = 16384
		if m.Sheet == "" || len(m.Sheet) > 128 || !utf8.ValidString(m.Sheet) || strings.IndexFunc(m.Sheet, unicode.IsControl) >= 0 || m.Delimiter != "" || m.HeaderRecord != 0 || m.FirstRecord != 0 || m.LastRecord != 0 || m.HeaderRow < 1 || m.FirstRow <= m.HeaderRow || m.LastRow < m.FirstRow || m.LastRow > maxWorkbookRows {
			return nil, ErrColumnMapping
		}
	default:
		return nil, ErrColumnMapping
	}
	if m.SampleColumn < 0 || m.SampleColumn > maxColumn {
		return nil, ErrColumnMapping
	}
	out := in.Clone()
	cols, keys := map[int]bool{}, map[string]bool{}
	if m.SampleColumn != 0 {
		cols[m.SampleColumn] = true
	}
	for _, c := range m.Columns {
		switch c.Key {
		case model.EffectiveDiameter, model.Polydispersity, model.CountRate, model.AverageCountRate, model.BaselineIndex:
		default:
			return nil, ErrColumnMapping
		}
		if c.Column < 1 || c.Column > maxColumn || cols[c.Column] || keys[c.Key] || len(c.Unit) > 64 || !utf8.ValidString(c.Unit) || strings.TrimSpace(c.Unit) != c.Unit || strings.IndexFunc(c.Unit, unicode.IsControl) >= 0 {
			return nil, ErrColumnMapping
		}
		cols[c.Column], keys[c.Key] = true, true
	}
	sort.Slice(out.Mapping.Columns, func(i, j int) bool { return out.Mapping.Columns[i].Column < out.Mapping.Columns[j].Column })
	return out, nil
}

func mappingFailure(err error, encoding string) Result {
	return Result{Status: "failed", Error: err.Error(), Parser: MappingVersion, Spec: MappingSpec, Encoding: encoding, Measurements: []model.Measurement{}, Recognized: []string{}}
}

// The complete source is read and bounded. The preview never supplies imported
// values. There is no unit recognition/conversion, date or experimental-unit
// inference, missing-value imputation, formula evaluation or automatic mapping.
func parseMappedDelimited(name string, data []byte, m *model.ColumnMapping, inspect bool) (Result, string, *TabularPreview) {
	format := "csv"
	if m.Delimiter == "tab" {
		format = "tsv"
	}
	fail := func(err error) (Result, string, *TabularPreview) { return mappingFailure(err, ""), format, nil }
	ext := strings.ToLower(filepath.Ext(name))
	if (ext != ".csv" && ext != ".tsv") || bytes.HasPrefix(data, []byte("PK\x03\x04")) || bytes.HasPrefix(data, []byte{0xd0, 0xcf, 0x11, 0xe0}) || IsODSPackage(data) {
		return fail(ErrColumnMapping)
	}
	if len(data) > MaxFileSize {
		return fail(ErrTooLarge)
	}
	text, encoding, err := decode(data)
	if err != nil {
		return fail(err)
	}
	sep := ','
	if m.Delimiter == "semicolon" {
		sep = ';'
	} else if m.Delimiter == "tab" {
		sep = '\t'
	}
	if !withinCellLimits(text, byte(sep)) {
		return fail(ErrTextLimit)
	}
	r := csv.NewReader(strings.NewReader(text))
	r.Comma = sep
	r.FieldsPerRecord = -1
	result := Result{Status: "partial", Parser: MappingVersion, Spec: MappingSpec, Encoding: encoding, Delimiter: m.Delimiter, Decimal: m.Decimal, Measurements: []model.Measurement{}, Recognized: []string{}, Warnings: []string{"ls.user_mapping_unvalidated"}}
	preview := &TabularPreview{Schema: 1, Format: format, Delimiter: m.Delimiter}
	table := PreviewTable{}
	var headers []string
	record, cells := 0, 0
	for {
		values, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return fail(ErrDelimitedTable)
		}
		record++
		cells += len(values)
		if record > maxTextLines || cells > maxTextCells || len(values) > maxTextColumns {
			return fail(ErrTextLimit)
		}
		if inspect {
			table.RowCount++
			if len(values) > table.ColumnCount {
				table.ColumnCount = len(values)
			}
			if len(table.Rows) < previewRowLimit {
				line, _ := r.FieldPos(0)
				row := PreviewRow{Line: line, LastLine: line, Cells: []PreviewCell{}}
				for i, v := range values {
					line, col := r.FieldPos(i)
					last := line + strings.Count(v, "\n")
					if last > row.LastLine {
						row.LastLine = last
					}
					if i < previewColumnLimit {
						row.Cells = append(row.Cells, PreviewCell{Column: i + 1, Line: line, ByteColumn: col, Value: preview.value(v)})
					} else {
						preview.Truncated = true
					}
				}
				table.Rows = append(table.Rows, row)
			} else {
				preview.Truncated = true
			}
		}
		if record == m.HeaderRecord {
			headers = append([]string(nil), values...)
			if m.SampleColumn > len(headers) {
				return fail(ErrColumnMapping)
			}
			for _, c := range m.Columns {
				if c.Column > len(headers) {
					return fail(ErrColumnMapping)
				}
			}
		}
		if record < m.FirstRecord || record > m.LastRecord {
			continue
		}
		if headers == nil {
			return fail(ErrColumnMapping)
		}
		measurement := model.Measurement{Index: len(result.Measurements), SourceRange: "record:" + strconv.Itoa(record), Params: map[string]model.Quantity{}, Fields: []model.Field{}, Parser: MappingVersion, Spec: MappingSpec}
		if m.SampleColumn > 0 && m.SampleColumn <= len(values) {
			measurement.SampleID = values[m.SampleColumn-1]
		}
		for i, v := range values {
			cellLine, _ := r.FieldPos(i)
			label := ""
			if i < len(headers) {
				label = headers[i]
			}
			measurement.Fields = append(measurement.Fields, model.Field{Label: label, Text: v, Line: cellLine})
		}
		for _, c := range m.Columns {
			if c.Column > len(values) || strings.TrimSpace(values[c.Column-1]) == "" {
				continue
			}
			raw := values[c.Column-1]
			value, e := mappedNumber(raw, m.Decimal)
			if e != nil {
				return fail(e)
			}
			cellLine, _ := r.FieldPos(c.Column - 1)
			measurement.Params[c.Key] = model.Quantity{Value: value, Raw: raw, Unit: c.Unit, UnitOrigin: "user_mapping", SourceColumn: c.Column, Label: headers[c.Column-1], Line: cellLine}
			field := &measurement.Fields[c.Column-1]
			field.Key = c.Key
			field.Num = &value
		}
		// Rows with no assigned numeric values are kept as absent observations.
		result.Measurements = append(result.Measurements, measurement)
	}
	if record < m.LastRecord {
		return fail(ErrColumnMapping)
	}
	have := map[string]bool{}
	for _, measurement := range result.Measurements {
		for key := range measurement.Params {
			have[key] = true
		}
	}
	if len(have) == 0 {
		return fail(ErrUnrecognized)
	}
	for key := range have {
		result.Recognized = append(result.Recognized, key)
	}
	sort.Strings(result.Recognized)
	if !inspect {
		return result, format, nil
	}
	count := table.ColumnCount
	if count > previewColumnLimit {
		count = previewColumnLimit
		preview.Truncated = true
	}
	for i := 1; i <= count; i++ {
		table.Columns = append(table.Columns, PreviewColumn{Index: i, Label: strconv.Itoa(i)})
	}
	preview.Tables = []PreviewTable{table}
	return result, format, preview
}

// Text decimals are declared by the user; OOXML numeric cells use the
// format's dot decimal syntax regardless of a workbook's display locale.
func mappedNumber(raw, decimal string) (float64, error) {
	token := strings.TrimSpace(raw)
	if decimal == "comma" {
		if strings.Contains(token, ".") {
			return 0, ErrMappedNumber
		}
		token = strings.ReplaceAll(token, ",", ".")
	}
	if !mappingNumber.MatchString(token) {
		return 0, ErrMappedNumber
	}
	value, err := strconv.ParseFloat(token, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, ErrMappedNumber
	}
	mantissa := strings.Split(strings.ToLower(token), "e")[0]
	if value == 0 && strings.ContainsAny(mantissa, "123456789") {
		return 0, ErrMappedNumber
	}
	return value, nil
}
