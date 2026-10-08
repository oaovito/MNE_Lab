package lightscattering

import (
	"errors"
	"sort"
	"strings"

	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/xuri/excelize/v2"
)

var ErrImportSelection = errors.New("ls.invalid_import_selection")

// NormalizeSelection validates and copies the explicit recipe. nil means all
// sheets; an empty explicit selection never silently imports the whole book.
func NormalizeSelection(in *model.ImportSelection) (*model.ImportSelection, error) {
	if in == nil {
		return nil, nil
	}
	if len(in.Sheets) == 0 || len(in.Sheets) > 256 {
		return nil, ErrImportSelection
	}
	out := &model.ImportSelection{}
	seen := map[string]bool{}
	for _, s := range in.Sheets {
		if s.Name == "" || len(s.Name) > 128 || seen[s.Name] {
			return nil, ErrImportSelection
		}
		seen[s.Name] = true
		s.Range = strings.TrimSpace(s.Range)
		if s.Range != "" {
			bounds, err := selectionBounds(s.Range)
			if err != nil {
				return nil, err
			}
			a, _ := excelize.CoordinatesToCellName(bounds[0], bounds[1])
			b, _ := excelize.CoordinatesToCellName(bounds[2], bounds[3])
			s.Range = a + ":" + b
		}
		out.Sheets = append(out.Sheets, s)
	}
	sort.Slice(out.Sheets, func(i, j int) bool { return out.Sheets[i].Name < out.Sheets[j].Name })
	return out, nil
}

func selectionBounds(r string) ([4]int, error) {
	var out [4]int
	parts := strings.Split(strings.ToUpper(r), ":")
	if len(parts) != 2 {
		return out, ErrImportSelection
	}
	for i, p := range parts {
		col, row, err := excelize.CellNameToCoordinates(p)
		// Only plain A1 references; no formulas, sheet names, absolute references
		// or values beyond the bounded reader's supported row extent.
		if err != nil || strings.ContainsAny(p, "$ !") || row > maxWorkbookRows || col > 16384 {
			return out, ErrImportSelection
		}
		out[i*2], out[i*2+1] = col, row
	}
	if out[0] > out[2] || out[1] > out[3] {
		return out, ErrImportSelection
	}
	return out, nil
}
