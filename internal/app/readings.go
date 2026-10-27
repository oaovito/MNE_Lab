package app

import (
	"strings"

	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/store"
)

// fileReadings binds the runs tables of a parse result to the stored
// measurements of one file. Measurements are matched by their position in
// the file, exactly as the import numbers them.
func fileReadings(f model.SourceFile, tables []model.ReadingTable, ids []string) model.FileReadings {
	out := model.FileReadings{FileID: f.ID, Parser: lightscattering.Version, Spec: f.Spec, Tables: []model.ReadingTable{}}
	for _, tb := range tables {
		if tb.Measurement < 0 || tb.Measurement >= len(ids) {
			continue
		}
		tb.MeasurementID = ids[tb.Measurement]
		out.Tables = append(out.Tables, tb)
	}
	return out
}

// adoptRunReadings reads the runs tables of reports imported before the
// parser kept their run rows. It never edits a measurement or the file
// record: the readings go to their own collection, and only when the
// current parser reads the same summary values on the same lines of the
// unchanged original. The record has no timestamp, so two machines that do
// this for the same synchronized file write the same content.
func (p *Profile) adoptRunReadings() error {
	return p.St.Update(func(t *store.Tx) error {
		recs, err := t.List(CollFiles)
		if err != nil {
			return err
		}
		for _, r := range recs {
			var f model.SourceFile
			if decodeRecord(r, &f) != nil || !strings.HasPrefix(f.Parser, "lightscattering-parser/") || f.ImportSelection != nil || f.BlobID == "" || len(f.Measurements) == 0 {
				continue
			}
			if _, err := t.Get(CollReadings, f.ID, nil); err == nil {
				continue
			}
			data, err := t.GetBlob(f.BlobID)
			if err != nil {
				continue
			}
			parsed := lightscattering.Parse(data)
			ids := make([]string, len(parsed.Measurements))
			for _, id := range f.Measurements {
				var m model.Measurement
				if _, err := t.Get(CollMeasurements, id, &m); err != nil || m.Index < 0 || m.Index >= len(parsed.Measurements) {
					continue
				}
				if sameSummary(m, parsed.Measurements[m.Index]) {
					ids[m.Index] = m.ID
				}
			}
			var tables []model.ReadingTable
			for _, tb := range parsed.Readings {
				if tb.Measurement >= 0 && tb.Measurement < len(ids) && ids[tb.Measurement] != "" {
					tables = append(tables, tb)
				}
			}
			if _, err := t.Put(CollReadings, f.ID, fileReadings(f, tables, ids)); err != nil {
				return err
			}
		}
		return nil
	})
}

// sameSummary reports whether two reads of a report agree on every stored
// summary value, its text and its line.
func sameSummary(stored, now model.Measurement) bool {
	if len(stored.Params) != len(now.Params) {
		return false
	}
	for k, q := range stored.Params {
		n, ok := now.Params[k]
		if !ok || n.Raw != q.Raw || n.Line != q.Line {
			return false
		}
	}
	return true
}

// Readings returns the instrument readings stored for one file.
func readingsTx(t *store.Tx, fileID string) (model.FileReadings, bool) {
	var fr model.FileReadings
	if _, err := t.Get(CollReadings, fileID, &fr); err != nil {
		return fr, false
	}
	return fr, true
}
