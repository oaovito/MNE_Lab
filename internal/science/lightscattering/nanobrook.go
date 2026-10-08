package lightscattering

import (
	"fmt"
	"strings"

	"github.com/oaovito/mne_lab/internal/science/model"
)

type nanoSection struct {
	method, format string
	header         line
	lines          []line
}

// nanoMarker recognizes documented instrument headings, never numeric data
// alone. Reports can mix aligned tables and comma-separated spreadsheet rows.
func nanoMarker(l line) (method, format string, ok bool) {
	s := strings.TrimSpace(l.text)
	if !strings.HasPrefix(s, "****") || !strings.HasSuffix(s, "****") {
		return
	}
	s = strings.ToLower(strings.Join(strings.Fields(strings.Trim(s, "* ")), " "))
	format = "report"
	if strings.HasSuffix(s, "/spreadsheet format") {
		format = "spreadsheet"
		s = strings.TrimSuffix(s, "/spreadsheet format")
	}
	switch s {
	case "lognormal size distribution results":
		method = "lognormal"
	case "multimodal size distribution results":
		method = "multimodal"
	default:
		return "", "", false
	}
	return method, format, true
}

func nanoHeader(text string) (groups int, ok bool) {
	parts := strings.Split(text, "|")
	if len(parts) > maxTextColumns/3 {
		return 0, false
	}
	for _, part := range parts {
		cells := strings.Fields(part)
		if len(cells) != 3 || normalize(cells[0]) != "d nm" || normalize(cells[1]) != "g d" || normalize(cells[2]) != "c d" {
			return 0, false
		}
	}
	return len(parts), len(parts) > 0
}

// nanoBrookDistributions keeps algorithm and representation alternatives.
// G(d) is retained as the instrument's relative intensity, without assuming
// percent, sum normalization or conversion to another weighting. C(d) stays
// an unmapped auxiliary field. A user must select a candidate before plotting.
func nanoBrookDistributions(blk []line) ([]model.Distribution, map[int]bool, []string, bool) {
	var sections []nanoSection
	current := -1
	for _, l := range blk {
		if method, format, ok := nanoMarker(l); ok {
			sections = append(sections, nanoSection{method: method, format: format, header: l})
			current = len(sections) - 1
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(l.text), "****") {
			current = -1
		}
		if current >= 0 {
			sections[current].lines = append(sections[current].lines, l)
		}
	}
	if len(sections) == 0 {
		return nil, nil, nil, false
	}
	consumed := map[int]bool{}
	headers := map[string]bool{}
	for _, s := range sections {
		if s.format == "report" {
			for _, l := range s.lines {
				if _, ok := nanoHeader(l.text); ok {
					headers[s.method] = true
				}
			}
		}
	}
	var out []model.Distribution
	var warnings []string
	for _, s := range sections {
		d := model.Distribution{ID: fmt.Sprintf("%s-%s-%d", s.method, s.format, s.header.n), Method: s.method, Format: s.format,
			Columns: []model.Column{{Key: "diameter", Label: "d(nm)", Unit: "nm"}, {Key: "intensity", Label: "G(d)"}}}
		var rows []nanoRow
		badLine := 0
		if s.format == "spreadsheet" {
			if !headers[s.method] {
				warnings = append(warnings, fmt.Sprintf("ls.nanobrook_missing_header:%d", s.header.n))
				continue
			}
			for _, l := range s.lines {
				if strings.TrimSpace(l.text) == "" {
					continue
				}
				cells := splitRow(l.text, ",")
				if len(cells) < 2 || len(cells) > 3 {
					badLine = l.n
					break
				}
				rows = append(rows, nanoRow{cells: cells, line: l.n})
			}
		} else {
			groups := 0
			var columns [][]nanoRow
			var ended []bool
			for _, l := range s.lines {
				if groups == 0 {
					if n, ok := nanoHeader(l.text); ok {
						groups = n
						columns = make([][]nanoRow, n)
						ended = make([]bool, n)
						consumed[l.n] = true
					}
					continue
				}
				text := strings.TrimSpace(l.text)
				if text == "" || strings.Trim(text, "- ") == "" {
					continue
				}
				parts := strings.Split(l.text, "|")
				if len(parts) > groups {
					badLine = l.n
					break
				}
				for i := 0; i < groups; i++ {
					var cells []string
					if i < len(parts) {
						cells = strings.Fields(parts[i])
					}
					if len(cells) == 0 {
						ended[i] = true
						continue
					}
					if ended[i] || len(cells) != 3 {
						badLine = l.n
						break
					}
					columns[i] = append(columns[i], nanoRow{cells: cells, line: l.n})
				}
				if badLine > 0 {
					break
				}
			}
			if groups == 0 {
				badLine = s.header.n
			}
			for _, column := range columns {
				rows = append(rows, column...)
			}
		}
		if badLine == 0 && len(rows) > 0 {
			for _, row := range rows {
				for i := 0; i < 2; i++ {
					v, ok := parseNumber(row.cells[i], ".")
					if !ok {
						badLine = row.line
						break
					}
					d.Columns[i].Raw = append(d.Columns[i].Raw, row.cells[i])
					d.Columns[i].Values = append(d.Columns[i].Values, v)
				}
				if badLine > 0 {
					break
				}
				if len(row.cells) == 3 && strings.TrimSpace(row.cells[2]) != "" {
					f := model.Field{Label: "C(d)", Text: row.cells[2], Line: row.line}
					if v, ok := parseNumber(f.Text, "."); ok {
						f.Num = &v
					}
					d.Auxiliary = append(d.Auxiliary, f)
				}
				d.SourceLines = append(d.SourceLines, row.line)
				if d.FirstLine == 0 || row.line < d.FirstLine {
					d.FirstLine = row.line
				}
				if row.line > d.LastLine {
					d.LastLine = row.line
				}
			}
		}
		if badLine > 0 || len(rows) == 0 {
			if badLine == 0 {
				badLine = s.header.n
			}
			warnings = append(warnings, fmt.Sprintf("ls.nanobrook_invalid_distribution:%d", badLine))
			continue
		}
		for _, l := range s.lines {
			if l.n >= d.FirstLine && l.n <= d.LastLine {
				consumed[l.n] = true
			}
		}
		out = append(out, d)
	}
	return out, consumed, warnings, true
}

type nanoRow struct {
	cells []string
	line  int
}
