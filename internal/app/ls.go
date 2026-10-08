package app

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
)

// Errors (stable identifiers).
var (
	ErrFileTooLarge = errors.New("ls.file_too_large")
	ErrEmptyFile    = errors.New("ls.empty_file")
	ErrNotTrashed   = errors.New("ls.not_in_trash")
)

// ImportResult reports one imported file.
type ImportResult struct {
	Name         string   `json:"name"`
	Status       string   `json:"status"` // parsed, partial, failed, duplicate
	FileID       string   `json:"fileId,omitempty"`
	Existing     string   `json:"existing,omitempty"` // identical file already in the library
	Measurements int      `json:"measurements"`
	Recognized   []string `json:"recognized,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
	Error        string   `json:"error,omitempty"`
	NeedsDate    bool     `json:"needsDate,omitempty"`
}

// MeasurementSummary is a compact view for libraries and pickers.
type MeasurementSummary struct {
	ID             string                    `json:"id"`
	FileID         string                    `json:"fileId"`
	SourceRange    string                    `json:"sourceRange,omitempty"`
	SourceSheet    string                    `json:"sourceSheet,omitempty"`
	Index          int                       `json:"index"`
	SampleID       string                    `json:"sampleId,omitempty"`
	MeasuredAt     *model.Timestamp          `json:"measuredAt,omitempty"`
	Params         map[string]model.Quantity `json:"params"`
	Weightings     []string                  `json:"weightings,omitempty"`
	Bins           int                       `json:"bins"`
	DistributionID string                    `json:"distributionId,omitempty"`
	Label          string                    `json:"label,omitempty"`
	Replicate      int                       `json:"replicate,omitempty"`
	Condition      string                    `json:"condition,omitempty"`
	Experiment     string                    `json:"experiment,omitempty"`
	Group          string                    `json:"group,omitempty"`
	Tags           []string                  `json:"tags,omitempty"`
	Notes          string                    `json:"notes,omitempty"`
}

// FileView is a library entry.
type FileView struct {
	model.SourceFile
	Items []MeasurementSummary `json:"items"`
}

func summarize(m model.Measurement) MeasurementSummary {
	s := MeasurementSummary{ID: m.ID, FileID: m.FileID, SourceSheet: m.SourceSheet, SourceRange: m.SourceRange, Index: m.Index, SampleID: m.SampleID, MeasuredAt: m.MeasuredAt, Params: map[string]model.Quantity{},
		Label: m.Label, Replicate: m.Replicate, Condition: m.Condition, Experiment: m.Experiment, Group: m.Group, Tags: m.Tags, Notes: m.Notes}
	s.DistributionID = m.DistributionID
	for _, k := range []string{model.EffectiveDiameter, model.Polydispersity, model.CountRate, model.AverageCountRate, model.BaselineIndex} {
		if q, ok := m.Params[k]; ok {
			s.Params[k] = q
		}
	}
	if m.Dist != nil {
		s.Weightings = m.Dist.Weightings()
		if d := m.Dist.Column("diameter"); d != nil {
			s.Bins = len(d.Values)
		}
	}
	return s
}

// Import adds a file to the library: the original bytes are kept exactly,
// the parser output is stored as normalized data, and an identical file
// already in the library is reported instead of duplicated.
func (p *Profile) Import(name string, data []byte, force bool) ImportResult {
	return p.importSelected(name, data, force, nil)
}

func (p *Profile) importSelected(name string, data []byte, force bool, selection *model.ImportSelection) ImportResult {
	done := p.app.begin("import")
	defer done()
	res := ImportResult{Name: name}
	if len(data) == 0 {
		res.Status, res.Error = "failed", ErrEmptyFile.Error()
		return res
	}
	if len(data) > lightscattering.MaxFileSize {
		res.Status, res.Error = "failed", ErrFileTooLarge.Error()
		return res
	}
	sum := secure.HashHex(data)
	if !force {
		var dup string
		p.St.View(func(t *store.Tx) error {
			recs, _ := t.List(CollFiles)
			for _, r := range recs {
				var f model.SourceFile
				if decodeRecord(r, &f) == nil && f.SHA256 == sum && !f.Trashed && sameImportSelection(f.ImportSelection, selection) {
					dup = f.ID
				}
			}
			return nil
		})
		if dup != "" {
			res.Status, res.Existing = "duplicate", dup
			return res
		}
	}
	r, format := lightscattering.ParseFileSelection(name, data, selection)
	f := model.SourceFile{ID: secure.NewID(), Name: name, Format: format, Size: int64(len(data)), SHA256: sum, ImportedAt: time.Now().UTC(),
		Module: "lightscattering", Parser: r.Parser, Spec: r.Spec, Status: r.Status, Encoding: r.Encoding, Delimiter: r.Delimiter, Decimal: r.Decimal,
		Warnings: r.Warnings, Error: r.Error, Origin: "import", ImportSelection: selection}
	var ms []model.Measurement
	for i, m := range r.Measurements {
		m.ID = secure.NewID()
		m.FileID = f.ID
		m.Index = i
		ms = append(ms, m)
		f.Measurements = append(f.Measurements, m.ID)
		if m.MeasuredAt == nil || m.MeasuredAt.Ambiguous {
			res.NeedsDate = true
		}
	}
	var savedDuplicate string
	err := p.St.Update(func(t *store.Tx) error {
		// Recheck within the write transaction: concurrent confirmations of the
		// same original/recipe must not create two scientific datasets.
		if !force {
			records, err := t.List(CollFiles)
			if err != nil {
				return err
			}
			for _, record := range records {
				var existing model.SourceFile
				if decodeRecord(record, &existing) == nil && !existing.Trashed && existing.SHA256 == sum && sameImportSelection(existing.ImportSelection, selection) {
					savedDuplicate = existing.ID
					return nil
				}
			}
		}
		id, err := t.PutBlob(data)
		if err != nil {
			return err
		}
		f.BlobID = id
		for _, m := range ms {
			if _, err := t.Put(CollMeasurements, m.ID, m); err != nil {
				return err
			}
		}
		_, err = t.Put(CollFiles, f.ID, f)
		return err
	})
	if err != nil {
		res.Status, res.Error = "failed", "ls.save_failed"
		return res
	}
	if savedDuplicate != "" {
		res.Status, res.Existing = "duplicate", savedDuplicate
		return res
	}
	res.Status, res.FileID, res.Measurements, res.Recognized, res.Warnings, res.Error = r.Status, f.ID, len(ms), r.Recognized, r.Warnings, r.Error
	return res
}

func decodeRecord(r store.Record, v any) error { return jsonUnmarshal(r.Data, v) }

// Files lists the library (trashed files only when asked).
func (p *Profile) Files(trash bool) ([]FileView, error) {
	var out []FileView
	err := p.St.View(func(t *store.Tx) error {
		ms := map[string]model.Measurement{}
		recs, err := t.List(CollMeasurements)
		if err != nil {
			return err
		}
		for _, r := range recs {
			var m model.Measurement
			if decodeRecord(r, &m) == nil {
				ms[m.ID] = m
			}
		}
		frs, err := t.List(CollFiles)
		if err != nil {
			return err
		}
		for _, r := range frs {
			var f model.SourceFile
			if decodeRecord(r, &f) != nil || f.Trashed != trash {
				continue
			}
			v := FileView{SourceFile: f}
			for _, id := range f.Measurements {
				if m, ok := ms[id]; ok {
					v.Items = append(v.Items, summarize(m))
				}
			}
			out = append(out, v)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].ImportedAt.After(out[j].ImportedAt) })
	return out, err
}

// File returns one file with its full measurements.
func (p *Profile) File(id string) (model.SourceFile, []model.Measurement, error) {
	var f model.SourceFile
	var ms []model.Measurement
	err := p.St.View(func(t *store.Tx) error {
		if _, err := t.Get(CollFiles, id, &f); err != nil {
			return err
		}
		for _, mid := range f.Measurements {
			var m model.Measurement
			if _, err := t.Get(CollMeasurements, mid, &m); err == nil {
				ms = append(ms, m)
			}
		}
		return nil
	})
	return f, ms, err
}

// Original returns the exact bytes that were imported.
func (p *Profile) Original(id string) (model.SourceFile, []byte, error) {
	var f model.SourceFile
	var b []byte
	err := p.St.View(func(t *store.Tx) error {
		if _, err := t.Get(CollFiles, id, &f); err != nil {
			return err
		}
		var err error
		b, err = t.GetBlob(f.BlobID)
		return err
	})
	if err == nil && secure.HashHex(b) != f.SHA256 {
		return f, nil, errors.New("ls.original_checksum_mismatch")
	}
	return f, b, err
}

// FilePatch edits the person's organization of a file (never its data).
type FilePatch struct {
	Experiment *string   `json:"experiment"`
	Group      *string   `json:"group"`
	Tags       *[]string `json:"tags"`
	Notes      *string   `json:"notes"`
}

// UpdateFile applies a patch.
func (p *Profile) UpdateFile(id string, patch FilePatch) (model.SourceFile, error) {
	var f model.SourceFile
	err := p.St.Update(func(t *store.Tx) error {
		if _, err := t.Get(CollFiles, id, &f); err != nil {
			return err
		}
		if patch.Experiment != nil {
			f.Experiment = clean(*patch.Experiment, 120)
		}
		if patch.Group != nil {
			f.Group = clean(*patch.Group, 120)
		}
		if patch.Tags != nil {
			f.Tags = cleanTags(*patch.Tags)
		}
		if patch.Notes != nil {
			f.Notes = clean(*patch.Notes, 4000)
		}
		_, err := t.Put(CollFiles, id, f)
		return err
	})
	return f, err
}

// MeasurementPatch edits identification and organization. A corrected
// date is recorded as the person's decision; the file keeps its original.
type MeasurementPatch struct {
	DistributionID *string    `json:"distributionId"`
	Label          *string    `json:"label"`
	SampleID       *string    `json:"sampleId"`
	Replicate      *int       `json:"replicate"`
	Condition      *string    `json:"condition"`
	Experiment     *string    `json:"experiment"`
	Group          *string    `json:"group"`
	Tags           *[]string  `json:"tags"`
	Notes          *string    `json:"notes"`
	MeasuredAt     *time.Time `json:"measuredAt"`
	ConfirmDate    bool       `json:"confirmDate"`
}

// UpdateMeasurement applies a patch.
func (p *Profile) UpdateMeasurement(id string, patch MeasurementPatch) (MeasurementSummary, error) {
	var m model.Measurement
	err := p.St.Update(func(t *store.Tx) error {
		if _, err := t.Get(CollMeasurements, id, &m); err != nil {
			return err
		}
		if patch.DistributionID != nil {
			found := false
			for i := range m.Distributions {
				if m.Distributions[i].ID == *patch.DistributionID {
					d := m.Distributions[i]
					m.Dist, m.DistributionID = &d, d.ID
					now := time.Now().UTC()
					m.DistributionSelectedAt = &now
					found = true
					break
				}
			}
			if !found {
				return ErrDistributionChoice
			}
		}
		if patch.Label != nil {
			m.Label = clean(*patch.Label, 120)
		}
		if patch.SampleID != nil {
			m.SampleID = clean(*patch.SampleID, 120)
		}
		if patch.Replicate != nil && *patch.Replicate >= 0 && *patch.Replicate < 1000 {
			m.Replicate = *patch.Replicate
		}
		if patch.Condition != nil {
			m.Condition = clean(*patch.Condition, 120)
		}
		if patch.Experiment != nil {
			m.Experiment = clean(*patch.Experiment, 120)
		}
		if patch.Group != nil {
			m.Group = clean(*patch.Group, 120)
		}
		if patch.Tags != nil {
			m.Tags = cleanTags(*patch.Tags)
		}
		if patch.Notes != nil {
			m.Notes = clean(*patch.Notes, 4000)
		}
		switch {
		case patch.MeasuredAt != nil:
			raw := ""
			if m.MeasuredAt != nil {
				raw = m.MeasuredAt.Raw
			}
			m.MeasuredAt = &model.Timestamp{Time: patch.MeasuredAt.UTC(), Raw: raw, TZKnown: false, Confirmed: true, Source: "user"}
		case patch.ConfirmDate && m.MeasuredAt != nil:
			m.MeasuredAt.Confirmed = true
		}
		_, err := t.Put(CollMeasurements, id, m)
		return err
	})
	return summarize(m), err
}

// TrashFile moves a file to the trash (recoverable).
func (p *Profile) TrashFile(id string, trashed bool) error {
	return p.St.Update(func(t *store.Tx) error {
		var f model.SourceFile
		if _, err := t.Get(CollFiles, id, &f); err != nil {
			return err
		}
		f.Trashed = trashed
		_, err := t.Put(CollFiles, id, f)
		return err
	})
}

// DeleteFile removes a trashed file permanently, including its original
// bytes when no other library entry uses them.
func (p *Profile) DeleteFile(id string) error {
	return p.St.Update(func(t *store.Tx) error {
		var f model.SourceFile
		if _, err := t.Get(CollFiles, id, &f); err != nil {
			return err
		}
		if !f.Trashed {
			return ErrNotTrashed
		}
		for _, mid := range f.Measurements {
			if _, err := t.GetRecord(CollMeasurements, mid); err == nil {
				if err := t.Delete(CollMeasurements, mid); err != nil {
					return err
				}
			}
		}
		if err := t.Delete(CollFiles, id); err != nil {
			return err
		}
		shared := false
		recs, _ := t.List(CollFiles)
		for _, r := range recs {
			var o model.SourceFile
			if decodeRecord(r, &o) == nil && o.BlobID == f.BlobID {
				shared = true
			}
		}
		if !shared && f.BlobID != "" && t.HasBlob(f.BlobID) {
			if _, err := t.Put(store.CollBlobGone, f.BlobID, map[string]any{"at": time.Now().UTC()}); err != nil {
				return err
			}
			return t.DeleteBlob(f.BlobID)
		}
		return nil
	})
}

// Input loads what the graph engine needs for a set of measurements.
func (p *Profile) Input(ids []string) (graph.Input, error) {
	in := graph.Input{Measurements: map[string]model.Measurement{}, Files: map[string]model.SourceFile{}}
	err := p.St.View(func(t *store.Tx) error {
		for _, id := range ids {
			var m model.Measurement
			if _, err := t.Get(CollMeasurements, id, &m); err != nil {
				continue
			}
			in.Measurements[id] = m
			if _, ok := in.Files[m.FileID]; !ok {
				var f model.SourceFile
				if _, err := t.Get(CollFiles, m.FileID, &f); err == nil {
					in.Files[f.ID] = f
				}
			}
		}
		return nil
	})
	return in, err
}

func clean(s string, max int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s))
	if r := []rune(s); len(r) > max {
		s = string(r[:max])
	}
	return s
}

func cleanTags(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range in {
		t = clean(t, 40)
		if t != "" && !seen[strings.ToLower(t)] && len(out) < 20 {
			seen[strings.ToLower(t)] = true
			out = append(out, t)
		}
	}
	return out
}
