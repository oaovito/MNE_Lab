package lightscattering

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

const odsMIME = "application/vnd.oasis.opendocument.spreadsheet"

// ODSReaderVersion identifies literal interoperability, without a scientific specification.
const ODSReaderVersion = "ods-literal-reader/1.0.0"
const odfOffice = "urn:oasis:names:tc:opendocument:xmlns:office:1.0"
const odfTable = "urn:oasis:names:tc:opendocument:xmlns:table:1.0"
const odfText = "urn:oasis:names:tc:opendocument:xmlns:text:1.0"
const odfManifest = "urn:oasis:names:tc:opendocument:xmlns:manifest:1.0"

// IsODSPackage identifies the actual MIME part, not an extension or arbitrary
// XML field. It never establishes scientific compatibility or package safety.
func IsODSPackage(data []byte) bool {
	if len(data) > MaxFileSize {
		return false
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(z.File) == 0 || z.File[0].Name != "mimetype" {
		return false
	}
	r, err := z.File[0].Open()
	if err != nil {
		return false
	}
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, int64(len(odsMIME)+1)))
	return err == nil && string(b) == odsMIME
}

func odsFailure(err error) Result {
	return Result{Status: "failed", Error: err.Error(), Parser: ODSReaderVersion, Encoding: "ODF 1.3 XML / ZIP"}
}

func odfAttr(el xml.StartElement, ns, name string) (string, bool) {
	for _, a := range el.Attr {
		if a.Name.Space == ns && a.Name.Local == name {
			return a.Value, true
		}
	}
	return "", false
}

func odfRepeat(el xml.StartElement, ns, name string, limit int) (int, error) {
	value, ok := odfAttr(el, ns, name)
	if !ok {
		return 1, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 || n > limit {
		return 0, ErrSpreadsheetLimit
	}
	return n, nil
}

// validateODFXML checks every XML part, including data outside the preview.
// XML directives, formulas/cached formula values, scripts, external sources
// and encrypted payloads are rejected; nothing is executed or fetched.
func validateODFXML(data []byte) error {
	d := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})))
	depth, tokens, roots := 0, 0, 0
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ErrSpreadsheet
		}
		tokens++
		if tokens > 2000000 {
			return ErrSpreadsheetLimit
		}
		switch el := token.(type) {
		case xml.Directive:
			return ErrSpreadsheetUnsupported
		case xml.StartElement:
			if depth == 0 {
				roots++
				if roots != 1 {
					return ErrSpreadsheet
				}
			}
			depth++
			if depth > 128 {
				return ErrSpreadsheetLimit
			}
			if el.Name.Space == odfOffice && el.Name.Local == "scripts" || el.Name.Space == odfManifest && el.Name.Local == "encryption-data" || strings.Contains(el.Name.Space, ":script:") {
				return ErrSpreadsheetUnsupported
			}
			if el.Name.Space == odfTable {
				switch el.Name.Local {
				case "cell-range-source", "table-source", "dde-link", "tracked-changes":
					return ErrSpreadsheetUnsupported
				}
			}
			attributes := map[xml.Name]bool{}
			for _, a := range el.Attr {
				if attributes[a.Name] {
					return ErrSpreadsheet
				}
				attributes[a.Name] = true
				if a.Name.Space == odfTable && a.Name.Local == "formula" {
					return ErrSpreadsheetFormula
				}
				if a.Name.Space == "http://www.w3.org/1999/xlink" && a.Name.Local == "href" && a.Value != "" && !strings.HasPrefix(a.Value, "#") {
					return ErrSpreadsheetUnsupported
				}
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(el)) != "" {
				return ErrSpreadsheet
			}
		}
	}
	if depth != 0 || roots != 1 {
		return ErrSpreadsheet
	}
	return nil
}

func odsContent(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, ErrEmpty
	}
	if len(data) > MaxFileSize {
		return nil, ErrTooLarge
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ErrSpreadsheet
	}
	if len(z.File) == 0 || len(z.File) > 4096 {
		return nil, ErrSpreadsheetLimit
	}
	first := z.File[0]
	offset, offsetErr := first.DataOffset()
	if offsetErr != nil || offset != int64(30+len("mimetype")) || first.Name != "mimetype" || first.Method != zip.Store || len(first.Extra) != 0 {
		return nil, ErrSpreadsheetUnsupported
	}
	seen := map[string]bool{}
	var total uint64
	var content []byte
	manifest, contentEntry := false, false
	for _, f := range z.File {
		name := f.Name
		if name == "" || name == "." || strings.Contains(name, "\\") || strings.HasPrefix(name, "/") || path.Clean(name) != strings.TrimSuffix(name, "/") || name == ".." || strings.HasPrefix(name, "../") || seen[name] {
			return nil, ErrSpreadsheet
		}
		seen[name] = true
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "basic/") || strings.HasPrefix(lower, "scripts/") {
			return nil, ErrSpreadsheetUnsupported
		}
		if f.UncompressedSize64 > maxWorkbookBytes || total > maxWorkbookBytes-f.UncompressedSize64 {
			return nil, ErrSpreadsheetLimit
		}
		total += f.UncompressedSize64
		r, err := f.Open()
		if err != nil {
			return nil, ErrSpreadsheet
		}
		raw, err := io.ReadAll(io.LimitReader(r, maxWorkbookBytes+1))
		r.Close()
		if err != nil {
			return nil, ErrSpreadsheet
		}
		if len(raw) > maxWorkbookBytes {
			return nil, ErrSpreadsheetLimit
		}
		if uint64(len(raw)) != f.UncompressedSize64 {
			return nil, ErrSpreadsheet
		}
		if name == "mimetype" && string(raw) != odsMIME {
			return nil, ErrSpreadsheetUnsupported
		}
		if strings.HasSuffix(lower, ".xml") || bytes.HasPrefix(bytes.TrimSpace(raw), []byte("<")) {
			if err := validateODFXML(raw); err != nil {
				return nil, err
			}
		}
		if name == "content.xml" {
			content = raw
		}
		if name == "META-INF/manifest.xml" {
			var root struct {
				XMLName xml.Name
				Entries []struct {
					Path  string `xml:"urn:oasis:names:tc:opendocument:xmlns:manifest:1.0 full-path,attr"`
					Media string `xml:"urn:oasis:names:tc:opendocument:xmlns:manifest:1.0 media-type,attr"`
				} `xml:"urn:oasis:names:tc:opendocument:xmlns:manifest:1.0 file-entry"`
			}
			if xml.Unmarshal(raw, &root) != nil || root.XMLName != (xml.Name{Space: odfManifest, Local: "manifest"}) {
				return nil, ErrSpreadsheet
			}
			paths := map[string]bool{}
			for _, entry := range root.Entries {
				if paths[entry.Path] {
					return nil, ErrSpreadsheet
				}
				paths[entry.Path] = true
				if entry.Path == "content.xml" {
					contentEntry = entry.Media == "text/xml"
				}
				if entry.Path == "/" {
					if manifest || entry.Media != odsMIME {
						return nil, ErrSpreadsheet
					}
					manifest = true
				}
			}
		}
	}
	if len(content) == 0 || !manifest || !contentEntry {
		return nil, ErrSpreadsheet
	}
	return content, nil
}

// odfParagraph implements ODF 1.3 Part 3 §6.1.2 for the supported plain text
// subset. Explicit s/tab/line-break elements preserve their stated whitespace.
type odfParagraph struct {
	text    strings.Builder
	pending bool
	started bool
}

func (p *odfParagraph) chars(s string) {
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			p.pending = p.started
			continue
		}
		if p.pending {
			p.text.WriteByte(' ')
			p.pending = false
		}
		p.text.WriteRune(r)
		p.started = true
	}
}
func (p *odfParagraph) explicit(s string) {
	if p.pending {
		p.text.WriteByte(' ')
		p.pending = false
	}
	p.text.WriteString(s)
	p.started = true
}

func odfCell(d *xml.Decoder, start xml.StartElement, p *TabularPreview, textBudget *int) (PreviewCell, error) {
	reserve := func(n int) error {
		if n > *textBudget {
			return ErrSpreadsheetLimit
		}
		*textBudget -= n
		return nil
	}
	var out PreviewCell
	valueType, _ := odfAttr(start, odfOffice, "value-type")
	out.ValueType = valueType
	attr := ""
	switch valueType {
	case "", "void":
	case "string":
		attr = "string-value"
	case "float", "percentage":
		attr = "value"
	case "boolean":
		attr = "boolean-value"
	case "date":
		attr = "date-value"
	case "time":
		attr = "time-value"
	default:
		return out, ErrSpreadsheetUnsupported
	}
	if attr != "" {
		value, ok := odfAttr(start, odfOffice, attr)
		if ok {
			if err := reserve(len(value)); err != nil {
				return out, err
			}
			value = p.value(value)
			out.SourceValue = &value
		} else if valueType != "string" {
			return out, ErrSpreadsheet
		}
	}
	for _, name := range []string{"number-columns-spanned", "number-rows-spanned", "number-matrix-columns-spanned", "number-matrix-rows-spanned"} {
		n, err := odfRepeat(start, odfTable, name, 1)
		if err != nil || n != 1 {
			return out, ErrSpreadsheetUnsupported
		}
	}
	var paragraphs []string
	var paragraph *odfParagraph
	depth := 1
	for depth > 0 {
		token, err := d.Token()
		if err != nil {
			return out, ErrSpreadsheet
		}
		switch el := token.(type) {
		case xml.StartElement:
			depth++
			if el.Name.Space != odfText {
				return out, ErrSpreadsheetUnsupported
			}
			switch el.Name.Local {
			case "p":
				if paragraph != nil || depth != 2 {
					return out, ErrSpreadsheet
				}
				paragraph = &odfParagraph{}
			case "span":
				if paragraph == nil {
					return out, ErrSpreadsheet
				}
			case "s":
				if paragraph == nil {
					return out, ErrSpreadsheet
				}
				n, err := odfRepeat(el, odfText, "c", maxWorkbookBytes)
				if err != nil {
					return out, err
				}
				if err := reserve(n); err != nil {
					return out, err
				}
				paragraph.explicit(strings.Repeat(" ", n))
			case "tab", "line-break":
				if paragraph == nil {
					return out, ErrSpreadsheet
				}
				s := "\t"
				if el.Name.Local == "line-break" {
					s = "\n"
				}
				if err := reserve(1); err != nil {
					return out, err
				}
				paragraph.explicit(s)
			default:
				return out, ErrSpreadsheetUnsupported
			}
			if el.Name.Local == "s" || el.Name.Local == "tab" || el.Name.Local == "line-break" {
				token, err := d.Token()
				end, ok := token.(xml.EndElement)
				if err != nil || !ok || end.Name != el.Name {
					return out, ErrSpreadsheet
				}
				depth--
			}
		case xml.EndElement:
			if el.Name.Space == odfText && el.Name.Local == "p" {
				if len(paragraphs) > 0 {
					if err := reserve(1); err != nil {
						return out, err
					}
				}
				paragraphs = append(paragraphs, paragraph.text.String())
				paragraph = nil
			}
			depth--
		case xml.CharData:
			if paragraph != nil {
				if err := reserve(len(el)); err != nil {
					return out, err
				}
				paragraph.chars(string(el))
			} else if strings.TrimSpace(string(el)) != "" {
				return out, ErrSpreadsheet
			}
		}
	}
	if len(paragraphs) > 0 {
		out.Value = p.value(strings.Join(paragraphs, "\n"))
	} else if out.SourceValue != nil {
		out.Value = *out.SourceValue
	}
	return out, nil
}

// inspectODS offers a transient literal view only. It never calls the DLS
// scalar/distribution parser, issues a receipt, or produces measurements.
func inspectODS(data []byte) (Result, string, *TabularPreview) {
	fail := func(err error) (Result, string, *TabularPreview) { return odsFailure(err), "ods", nil }
	content, err := odsContent(data)
	if err != nil {
		return fail(err)
	}
	p := &TabularPreview{Schema: 1, Format: "ods"}
	d := xml.NewDecoder(bytes.NewReader(content))
	var table *PreviewTable
	var stack []xml.Name
	var row []PreviewCell
	rowWidth, rowRepeat := 0, 0
	totalRows, totalCells, tables := 0, 0, 0
	names := map[string]bool{}
	root, body, spreadsheet := false, false, false
	bodySeen, spreadsheetSeen := false, false
	textBudget := maxWorkbookBytes
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fail(ErrSpreadsheet)
		}
		switch el := token.(type) {
		case xml.StartElement:
			parent := xml.Name{}
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			stack = append(stack, el.Name)
			if !root {
				version, _ := odfAttr(el, odfOffice, "version")
				if el.Name != (xml.Name{Space: odfOffice, Local: "document-content"}) || version != "1.3" {
					return fail(ErrSpreadsheetUnsupported)
				}
				root = true
				continue
			}
			if el.Name.Space == odfOffice {
				switch el.Name.Local {
				case "body":
					if bodySeen || parent != (xml.Name{Space: odfOffice, Local: "document-content"}) {
						return fail(ErrSpreadsheet)
					}
					body, bodySeen = true, true
				case "spreadsheet":
					if !body || spreadsheetSeen || parent != (xml.Name{Space: odfOffice, Local: "body"}) {
						return fail(ErrSpreadsheet)
					}
					spreadsheet, spreadsheetSeen = true, true
				}
			}
			if el.Name.Space != odfTable {
				continue
			}
			switch el.Name.Local {
			case "table":
				if !spreadsheet || table != nil || parent != (xml.Name{Space: odfOffice, Local: "spreadsheet"}) {
					return fail(ErrSpreadsheetUnsupported)
				}
				name, _ := odfAttr(el, odfTable, "name")
				if name == "" || len(name) > 128 || names[name] {
					return fail(ErrSpreadsheet)
				}
				names[name] = true
				tables++
				if tables > 256 {
					return fail(ErrSpreadsheetLimit)
				}
				table = &PreviewTable{Sheet: name}
			case "table-row":
				if table == nil || rowRepeat != 0 || parent.Space != odfTable || (parent.Local != "table" && parent.Local != "table-header-rows" && parent.Local != "table-rows" && parent.Local != "table-row-group") {
					return fail(ErrSpreadsheet)
				}
				rowRepeat, err = odfRepeat(el, odfTable, "number-rows-repeated", maxWorkbookRows)
				if err != nil {
					return fail(err)
				}
				rowWidth = 0
				row = nil
			case "table-cell":
				if table == nil || rowRepeat == 0 || parent != (xml.Name{Space: odfTable, Local: "table-row"}) {
					return fail(ErrSpreadsheet)
				}
				repeat, err := odfRepeat(el, odfTable, "number-columns-repeated", 16384)
				if err != nil {
					return fail(err)
				}
				if repeat > 16384-rowWidth {
					return fail(ErrSpreadsheetLimit)
				}
				before := textBudget
				cell, err := odfCell(d, el, p, &textBudget)
				if err != nil {
					return fail(err)
				}
				stack = stack[:len(stack)-1] // odfCell consumed its matching end token
				cost := before - textBudget
				copies := int64(repeat)*int64(rowRepeat) - 1
				if cost > 0 && copies > int64(textBudget/cost) {
					return fail(ErrSpreadsheetLimit)
				}
				textBudget -= int(copies) * cost
				for column := rowWidth + 1; column <= rowWidth+repeat && column <= previewColumnLimit; column++ {
					copy := cell
					copy.Column = column
					row = append(row, copy)
				}
				rowWidth += repeat
			case "covered-table-cell":
				return fail(ErrSpreadsheetUnsupported)
			}
		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1] != el.Name {
				return fail(ErrSpreadsheet)
			}
			stack = stack[:len(stack)-1]
			if el.Name.Space == odfOffice {
				if el.Name.Local == "spreadsheet" {
					spreadsheet = false
				}
				if el.Name.Local == "body" {
					body = false
				}
			}
			if el.Name.Space != odfTable {
				continue
			}
			switch el.Name.Local {
			case "table-row":
				if table == nil || rowRepeat == 0 || rowRepeat > maxWorkbookRows-totalRows || rowWidth > 0 && rowRepeat > (maxWorkbookCells-totalCells)/rowWidth {
					return fail(ErrSpreadsheetLimit)
				}
				totalRows += rowRepeat
				totalCells += rowWidth * rowRepeat
				for n := 0; n < rowRepeat && len(table.Rows) < previewRowLimit; n++ {
					line := table.RowCount + n + 1
					cells := append([]PreviewCell(nil), row...)
					for i := range cells {
						cells[i].Line = line
						cells[i].Address, _ = excelize.CoordinatesToCellName(cells[i].Column, line)
					}
					table.Rows = append(table.Rows, PreviewRow{Line: line, LastLine: line, Cells: cells})
				}
				table.RowCount += rowRepeat
				if rowWidth > table.ColumnCount {
					table.ColumnCount = rowWidth
				}
				if table.RowCount > previewRowLimit || table.ColumnCount > previewColumnLimit {
					p.Truncated = true
				}
				rowRepeat = 0
			case "table":
				if table == nil || rowRepeat != 0 {
					return fail(ErrSpreadsheet)
				}
				for n := 1; n <= table.ColumnCount && n <= previewColumnLimit; n++ {
					label, _ := excelize.ColumnNumberToName(n)
					table.Columns = append(table.Columns, PreviewColumn{Index: n, Label: label})
				}
				if len(p.Tables) < previewTableLimit {
					p.Tables = append(p.Tables, *table)
				} else {
					p.Truncated = true
				}
				table = nil
			}
		}
	}
	if !root || !bodySeen || !spreadsheetSeen || len(stack) != 0 || tables == 0 || table != nil || rowRepeat != 0 {
		return fail(ErrSpreadsheet)
	}
	result := odsFailure(ErrUnrecognized)
	return result, "ods", p
}
