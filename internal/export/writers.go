package export

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// DataOptions configures data serialization.
type DataOptions struct {
	// Decimal is "." (default, portable) or "," (spreadsheet programs in
	// comma-decimal locales; CSV then uses ";" between columns).
	Decimal string `json:"decimal,omitempty"`
}

// WriteData serializes a data set in a data format.
func WriteData(ds DataSet, format string, o DataOptions) ([]byte, error) {
	switch format {
	case "csv":
		sep := ','
		if o.Decimal == "," {
			sep = ';'
		}
		return delimited(ds.Tables[0], sep, o.Decimal, true)
	case "tsv":
		return delimited(ds.Tables[0], '\t', o.Decimal, false)
	case "txt":
		return plainText(ds, o.Decimal), nil
	case "json":
		return dataJSON(ds)
	case "xlsx":
		return workbook(ds)
	}
	return nil, ErrFormat
}

func delimited(t Table, sep rune, dec string, bom bool) ([]byte, error) {
	var b bytes.Buffer
	if bom {
		b.WriteString("\xef\xbb\xbf") // lets spreadsheet programs detect UTF-8
	}
	w := csv.NewWriter(&b)
	w.Comma = sep
	w.UseCRLF = true
	head := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		head[i] = c.Header()
	}
	if err := w.Write(head); err != nil {
		return nil, err
	}
	rec := make([]string, len(t.Columns))
	for _, r := range t.Rows {
		for i := range rec {
			rec[i] = ""
			if i < len(r) {
				rec[i] = sanitizeCell(r[i].String(dec), r[i].Num != nil)
			}
		}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

// sanitizeCell keeps text from being run as a formula when a CSV file is
// opened in a spreadsheet program. Numbers are never altered.
func sanitizeCell(s string, isNum bool) string {
	if isNum || s == "" {
		return s
	}
	switch s[0] {
	case '=', '@', '\t', '\r':
		return "'" + s
	case '+', '-':
		if strings.ContainsAny(s, "(!") {
			return "'" + s
		}
	}
	return s
}

func plainText(ds DataSet, dec string) []byte {
	var b strings.Builder
	b.WriteString(ds.Title + "\r\n")
	b.WriteString(strings.Repeat("=", max(8, len([]rune(ds.Title)))) + "\r\n\r\n")
	for _, kv := range ds.Meta {
		if kv.Value != "" {
			fmt.Fprintf(&b, "%s: %s\r\n", kv.Label, kv.Value)
		}
	}
	for _, t := range ds.Tables {
		b.WriteString("\r\n" + t.Name + "\r\n")
		b.WriteString(strings.Repeat("-", max(8, len([]rune(t.Name)))) + "\r\n")
		widths := make([]int, len(t.Columns))
		cells := make([][]string, 0, len(t.Rows)+1)
		head := make([]string, len(t.Columns))
		for i, c := range t.Columns {
			head[i] = c.Header()
		}
		cells = append(cells, head)
		for _, r := range t.Rows {
			row := make([]string, len(t.Columns))
			for i := range row {
				if i < len(r) {
					row[i] = r[i].String(dec)
				}
			}
			cells = append(cells, row)
		}
		for _, r := range cells {
			for i, c := range r {
				widths[i] = max(widths[i], len([]rune(c)))
			}
		}
		for _, r := range cells {
			for i, c := range r {
				if i > 0 {
					b.WriteString("  ")
				}
				b.WriteString(c)
				if i < len(r)-1 {
					b.WriteString(strings.Repeat(" ", widths[i]-len([]rune(c))))
				}
			}
			b.WriteString("\r\n")
		}
	}
	if len(ds.Provenance) > 0 {
		p := ds.tr("sheet.provenance", "Provenance")
		b.WriteString("\r\n" + p + "\r\n" + strings.Repeat("-", len([]rune(p))) + "\r\n")
		for _, s := range ds.Provenance {
			fmt.Fprintf(&b, "%s  sha256:%s  %s  %s  lines %s\r\n", s.FileName, s.SHA256, s.Parser, s.Spec, s.Lines)
		}
	}
	return []byte(b.String())
}

type jsonTable struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

func dataJSON(ds DataSet) ([]byte, error) {
	out := struct {
		Format     string `json:"format"`
		Title      string `json:"title"`
		Metadata   map[string]string
		Tables     []jsonTable `json:"tables"`
		Provenance any         `json:"provenance"`
		Rules      []string    `json:"rules,omitempty"`
	}{Format: "mnelab-data/1", Title: ds.Title, Metadata: map[string]string{}, Provenance: ds.Provenance, Rules: ds.Rules}
	for _, kv := range ds.Meta {
		out.Metadata[kv.Key] = kv.Value
	}
	for _, t := range ds.Tables {
		jt := jsonTable{ID: t.ID, Name: t.Name, Columns: t.Columns, Rows: make([][]any, 0, len(t.Rows))}
		for _, r := range t.Rows {
			row := make([]any, len(r))
			for i, c := range r {
				switch {
				case c.Num != nil:
					row[i] = json.Number(FormatValue(*c.Num, c.Dec, "."))
				case c.Text == "":
					row[i] = nil
				default:
					row[i] = c.Text
				}
			}
			jt.Rows = append(jt.Rows, row)
		}
		out.Tables = append(out.Tables, jt)
	}
	return json.MarshalIndent(out, "", "  ")
}

func sheetName(s string, used map[string]bool) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return '_'
		}
		return r
	}, s)
	if r := []rune(s); len(r) > 31 {
		s = string(r[:31])
	}
	if s == "" {
		s = "Sheet"
	}
	base := s
	for i := 2; used[s]; i++ {
		s = fmt.Sprintf("%s %d", base, i)
	}
	used[s] = true
	return s
}

func workbook(ds DataSet) ([]byte, error) {
	f := excelize.NewFile()
	defer f.Close()
	f.SetDocProps(&excelize.DocProperties{Title: ds.Title, Creator: "MNE Lab", LastModifiedBy: "MNE Lab",
		Created: ds.Created.Format("2006-01-02T15:04:05Z"), Modified: ds.Created.Format("2006-01-02T15:04:05Z")})
	f.SetAppProps(&excelize.AppProperties{Application: "MNE Lab"})
	head, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EEF1F5"}},
		Border: []excelize.Border{{Type: "bottom", Color: "9AA3AF", Style: 1}}})
	numStyles := map[int]int{}
	numStyle := func(dec int) int {
		if id, ok := numStyles[dec]; ok {
			return id
		}
		code := "General"
		if dec == 0 {
			code = "0"
		} else if dec > 0 {
			code = "0." + strings.Repeat("0", min(dec, 15))
		}
		id, _ := f.NewStyle(&excelize.Style{CustomNumFmt: &code})
		numStyles[dec] = id
		return id
	}
	used := map[string]bool{}
	first := true
	for _, t := range ds.Tables {
		name := sheetName(t.Name, used)
		if first {
			f.SetSheetName("Sheet1", name)
			first = false
		} else {
			f.NewSheet(name)
		}
		sw, err := f.NewStreamWriter(name)
		if err != nil {
			return nil, err
		}
		widths := make([]int, len(t.Columns))
		hrow := make([]any, len(t.Columns))
		for i, c := range t.Columns {
			hrow[i] = excelize.Cell{StyleID: head, Value: c.Header()}
			widths[i] = len([]rune(c.Header()))
		}
		for i, r := range t.Rows {
			for j, c := range r {
				if j < len(widths) {
					widths[j] = max(widths[j], len([]rune(c.String("."))))
				}
			}
			if i > 200 {
				break
			}
		}
		for i, w := range widths {
			sw.SetColWidth(i+1, i+1, float64(min(max(w, 6), 48))+2)
		}
		sw.SetPanes(&excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
		if err := sw.SetRow("A1", hrow); err != nil {
			return nil, err
		}
		for i, r := range t.Rows {
			row := make([]any, len(r))
			for j, c := range r {
				if c.Num != nil {
					row[j] = excelize.Cell{StyleID: numStyle(c.Dec), Value: *c.Num}
				} else {
					row[j] = c.Text
				}
			}
			cell, _ := excelize.CoordinatesToCellName(1, i+2)
			if err := sw.SetRow(cell, row); err != nil {
				return nil, err
			}
		}
		if err := sw.Flush(); err != nil {
			return nil, err
		}
	}
	// Metadata and provenance sheets.
	meta := sheetName(ds.tr("sheet.metadata", "Metadata"), used)
	f.NewSheet(meta)
	f.SetCellValue(meta, "A1", ds.tr("col.field", "Field"))
	f.SetCellValue(meta, "B1", ds.tr("col.value", "Value"))
	f.SetCellStyle(meta, "A1", "B1", head)
	for i, kv := range ds.Meta {
		f.SetCellValue(meta, fmt.Sprintf("A%d", i+2), kv.Label)
		f.SetCellValue(meta, fmt.Sprintf("B%d", i+2), kv.Value)
	}
	f.SetColWidth(meta, "A", "A", 28)
	f.SetColWidth(meta, "B", "B", 60)
	if len(ds.Provenance) > 0 {
		prov := sheetName(ds.tr("sheet.provenance", "Provenance"), used)
		f.NewSheet(prov)
		cols := []string{ds.tr("col.source_file", "File"), "SHA-256", ds.tr("col.parser", "Parser"), ds.tr("col.spec", "Specification"),
			ds.tr("col.measurement", "Measurement"), ds.tr("col.sample_id", "Sample"), ds.tr("col.measured_at", "Measured at"), ds.tr("col.lines", "Lines")}
		for i, c := range cols {
			cell, _ := excelize.CoordinatesToCellName(i+1, 1)
			f.SetCellValue(prov, cell, c)
		}
		end, _ := excelize.CoordinatesToCellName(len(cols), 1)
		f.SetCellStyle(prov, "A1", end, head)
		for i, s := range ds.Provenance {
			vals := []string{s.FileName, s.SHA256, s.Parser, s.Spec, s.MeasurementID, s.SampleID, s.MeasuredAt, s.Lines}
			for j, v := range vals {
				cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
				f.SetCellValue(prov, cell, v)
			}
		}
		f.SetColWidth(prov, "A", "A", 32)
		f.SetColWidth(prov, "B", "B", 66)
		f.SetColWidth(prov, "C", "H", 22)
	}
	f.SetActiveSheet(0)
	var b bytes.Buffer
	if err := f.Write(&b); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
