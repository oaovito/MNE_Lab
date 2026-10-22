package app

import (
	"strings"

	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/store"
)

// adoptTableUnits completes reports imported before the parser read units
// from the runs table header. Only the unit is added, and only where the
// current parser finds the same value on the same line of the unchanged
// original; every other value, the original parser and the import stay as
// they were.
func (p *Profile) adoptTableUnits() error {
	return p.St.Update(func(t *store.Tx) error {
		recs, err := t.List(CollFiles)
		if err != nil {
			return err
		}
		for _, r := range recs {
			var f model.SourceFile
			if decodeRecord(r, &f) != nil || f.UnitsReviewed == lightscattering.Version || f.Parser == lightscattering.Version ||
				!strings.HasPrefix(f.Parser, "lightscattering-parser/") || f.ImportSelection != nil || f.BlobID == "" {
				continue
			}
			var ms []model.Measurement
			missing := false
			for _, id := range f.Measurements {
				var m model.Measurement
				if _, err := t.Get(CollMeasurements, id, &m); err != nil {
					continue
				}
				for _, q := range m.Params {
					missing = missing || q.Unit == ""
				}
				ms = append(ms, m)
			}
			if !missing {
				continue
			}
			data, err := t.GetBlob(f.BlobID)
			if err != nil {
				continue
			}
			parsed := lightscattering.Parse(data)
			for _, m := range ms {
				if m.Index < 0 || m.Index >= len(parsed.Measurements) {
					continue
				}
				now, changed := parsed.Measurements[m.Index].Params, false
				for k, q := range m.Params {
					n, ok := now[k]
					if ok && q.Unit == "" && n.UnitOrigin == "file_table_header" && n.Raw == q.Raw && n.Line == q.Line {
						q.Unit, q.UnitOrigin, q.UnitLine = n.Unit, n.UnitOrigin, n.UnitLine
						m.Params[k], changed = q, true
					}
				}
				if changed {
					if _, err := t.Put(CollMeasurements, m.ID, m); err != nil {
						return err
					}
				}
			}
			f.UnitsReviewed = lightscattering.Version
			if _, err := t.Put(CollFiles, f.ID, f); err != nil {
				return err
			}
		}
		return nil
	})
}
