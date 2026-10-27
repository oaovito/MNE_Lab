package app

import (
	"errors"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/design"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/module"
	"github.com/oaovito/mne_lab/internal/science/series"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
)

// ErrDesignMeasurement refuses a design that references a measurement the
// profile does not hold (or one of another scientific module).
var ErrDesignMeasurement = errors.New("design.unknown_measurement")

// DesignView is a design revision with what the review screen needs.
type DesignView struct {
	Design     design.Design     `json:"design"`
	Structure  design.Structure  `json:"structure"`
	UnitLabels map[string]string `json:"unitLabels"`
	Head       bool              `json:"head"`
	Heads      int               `json:"heads"` // heads of this design (more than one after diverging edits)
	History    []design.Design   `json:"history,omitempty"`
}

func allDesignsTx(t *store.Tx) ([]design.Design, error) {
	recs, err := t.List(CollDesigns)
	if err != nil {
		return nil, err
	}
	out := []design.Design{}
	for _, r := range recs {
		var d design.Design
		if decodeRecord(r, &d) == nil && d.ID != "" {
			out = append(out, d)
		}
	}
	return out, nil
}

func unitLabelsTx(t *store.Tx) (map[string]string, error) {
	recs, err := t.List(CollUnits)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, r := range recs {
		var u analysis.ExperimentalUnit
		if decodeRecord(r, &u) == nil {
			out[u.ID] = u.Label
		}
	}
	return out, nil
}

func measurementInfoTx(t *store.Tx, d design.Design) map[string]design.MeasurementInfo {
	out := map[string]design.MeasurementInfo{}
	for _, a := range d.Assignments {
		var m model.Measurement
		if _, err := t.Get(CollMeasurements, a.MeasurementID, &m); err == nil {
			out[m.ID] = design.MeasurementInfo{SampleID: m.SampleID}
		}
	}
	return out
}

// Designs lists the current revision of every design (each head).
func (p *Profile) Designs() ([]DesignView, error) {
	out := []DesignView{}
	err := p.St.View(func(t *store.Tx) error {
		all, err := allDesignsTx(t)
		if err != nil {
			return err
		}
		labels, err := unitLabelsTx(t)
		if err != nil {
			return err
		}
		heads := design.Heads(all)
		count := map[string]int{}
		for _, h := range heads {
			count[h.Root]++
		}
		for _, h := range heads {
			out = append(out, DesignView{Design: h, Structure: design.Detect(h, measurementInfoTx(t, h)), UnitLabels: labels, Head: true, Heads: count[h.Root]})
		}
		return nil
	})
	return out, err
}

// Design returns one revision with its history.
func (p *Profile) Design(id string) (DesignView, error) {
	var v DesignView
	err := p.St.View(func(t *store.Tx) error {
		all, err := allDesignsTx(t)
		if err != nil {
			return err
		}
		hist := design.History(all, id)
		if len(hist) == 0 {
			return design.ErrNotFound
		}
		labels, err := unitLabelsTx(t)
		if err != nil {
			return err
		}
		v = DesignView{Design: hist[0], Structure: design.Detect(hist[0], measurementInfoTx(t, hist[0])), UnitLabels: labels, History: hist}
		for _, h := range design.Heads(all) {
			if h.Root == hist[0].Root {
				v.Heads++
				v.Head = v.Head || h.ID == id
			}
		}
		return nil
	})
	return v, err
}

// checkDesignTx verifies that every unit and measurement of a draft exists
// in this profile; unit labels are snapshotted from the profile records.
func checkDesignTx(t *store.Tx, d *design.Design) error {
	labels, err := unitLabelsTx(t)
	if err != nil {
		return err
	}
	for i, u := range d.Units {
		l, ok := labels[u.ID]
		if !ok {
			return design.ErrUnit
		}
		d.Units[i].Label = l
	}
	for _, a := range d.Assignments {
		var m model.Measurement
		if _, err := t.Get(CollMeasurements, a.MeasurementID, &m); err != nil {
			return ErrDesignMeasurement
		}
		var f model.SourceFile
		if _, err := t.Get(CollFiles, m.FileID, &f); err != nil || (f.Module != "" && f.Module != module.LightScattering) {
			return ErrDesignMeasurement
		}
	}
	if d.CycleID != "" {
		if _, err := t.GetRecord(CollCycles, d.CycleID); err != nil {
			return design.ErrInvalid
		}
	}
	return nil
}

// SaveDesign stores a draft as a new immutable revision. previousID names
// the revision that was edited; it must still be current, so an edit made
// meanwhile elsewhere is never overwritten.
func (p *Profile) SaveDesign(draft design.Design, previousID string) (DesignView, error) {
	done := p.app.begin("design")
	defer done()
	var saved design.Design
	err := p.St.Update(func(t *store.Tx) error {
		if p.closed.Load() || p.app.exiting.Load() {
			return ErrImportScope
		}
		d := design.Normalize(draft)
		if err := checkDesignTx(t, &d); err != nil {
			return err
		}
		var prev *design.Design
		if previousID != "" {
			all, err := allDesignsTx(t)
			if err != nil {
				return err
			}
			var found *design.Design
			for i := range all {
				if all[i].ID == previousID {
					found = &all[i]
				}
				if all[i].PreviousID == previousID {
					return design.ErrConflict
				}
			}
			if found == nil {
				return design.ErrNotFound
			}
			prev = found
		}
		next, err := design.Revise(prev, d, secure.NewID(), time.Now())
		if err != nil {
			return err
		}
		saved = next
		_, err = t.Put(CollDesigns, next.ID, next)
		return err
	})
	if err != nil {
		return DesignView{}, err
	}
	return p.Design(saved.ID)
}

// ReviewDesign computes the structure of an unsaved draft without storing it.
func (p *Profile) ReviewDesign(draft design.Design) (design.Structure, error) {
	d := design.Normalize(draft)
	if d.Name == "" {
		d.Name = "draft"
	}
	if err := design.Validate(d); err != nil {
		return design.Structure{}, err
	}
	var s design.Structure
	err := p.St.View(func(t *store.Tx) error {
		s = design.Detect(d, measurementInfoTx(t, d))
		return nil
	})
	return s, err
}

// SuggestDesign proposes conditions, units and times from the names of the
// given measurements. Nothing is stored or applied.
func (p *Profile) SuggestDesign(draft design.Design, ids []string) ([]design.Suggestion, error) {
	d := design.Normalize(draft)
	var cands []design.Candidate
	err := p.St.View(func(t *store.Tx) error {
		labels, err := unitLabelsTx(t)
		if err != nil {
			return err
		}
		for i, u := range d.Units {
			if l, ok := labels[u.ID]; ok {
				d.Units[i].Label = l
			}
		}
		if len(ids) > design.MaxAssignments {
			return design.ErrLimit
		}
		for _, id := range ids {
			var m model.Measurement
			if _, err := t.Get(CollMeasurements, id, &m); err != nil {
				return ErrDesignMeasurement
			}
			var f model.SourceFile
			t.Get(CollFiles, m.FileID, &f)
			cands = append(cands, design.Candidate{MeasurementID: m.ID, FileName: f.Name, SampleID: m.SampleID, Label: m.Label})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return design.Suggest(d, cands), nil
}

// RenameExperimentalUnit changes the visible name of a unit. Its identity,
// and therefore every design and analysis that references it, is unchanged;
// design revisions keep the name they were saved with.
func (p *Profile) RenameExperimentalUnit(id, label string) (analysis.ExperimentalUnit, error) {
	label = strings.TrimSpace(label)
	var u analysis.ExperimentalUnit
	if label == "" || !analysis.ValidText(label, 120) {
		return u, analysis.ErrDefinition
	}
	done := p.app.begin("design")
	defer done()
	err := p.St.Update(func(t *store.Tx) error {
		if _, err := t.Get(CollUnits, id, &u); err != nil {
			return err
		}
		u.Label = label
		_, err := t.Put(CollUnits, id, u)
		return err
	})
	return u, err
}

// SeriesQuery selects catalog entries.
type SeriesQuery struct {
	FileID   string
	DesignID string
	Q        string
}

// SeriesCatalog is the response of the series catalog.
type SeriesCatalog struct {
	Version string          `json:"version"`
	Design  *design.Design  `json:"design,omitempty"`
	Series  []series.Series `json:"series"`
}

// Series lists the series of the library (files in the trash excluded),
// resolving unit, condition, time and replicate kind from a design when
// one is named. It only reads.
func (p *Profile) Series(q SeriesQuery) (SeriesCatalog, error) {
	out := SeriesCatalog{Version: series.Version, Series: []series.Series{}}
	err := p.St.View(func(t *store.Tx) error {
		in := series.Input{Measurements: map[string]model.Measurement{}, Readings: map[string]model.FileReadings{}}
		var only map[string]bool
		if q.DesignID != "" {
			all, err := allDesignsTx(t)
			if err != nil {
				return err
			}
			hist := design.History(all, q.DesignID)
			if len(hist) == 0 {
				return design.ErrNotFound
			}
			d := hist[0]
			in.Design, out.Design = &d, &d
			only = map[string]bool{}
			for _, a := range d.Assignments {
				only[a.MeasurementID] = true
			}
		}
		labels, err := unitLabelsTx(t)
		if err != nil {
			return err
		}
		in.UnitLabels = labels
		frs, err := t.List(CollFiles)
		if err != nil {
			return err
		}
		for _, r := range frs {
			var f model.SourceFile
			if decodeRecord(r, &f) != nil || f.Trashed || (q.FileID != "" && f.ID != q.FileID) || (f.Module != "" && f.Module != module.LightScattering) {
				continue
			}
			keep := f
			keep.Measurements = nil
			for _, id := range f.Measurements {
				if only != nil && !only[id] {
					continue
				}
				var m model.Measurement
				if _, err := t.Get(CollMeasurements, id, &m); err == nil {
					in.Measurements[id] = m
					keep.Measurements = append(keep.Measurements, id)
				}
			}
			if len(keep.Measurements) == 0 {
				continue
			}
			in.Files = append(in.Files, keep)
			if fr, ok := readingsTx(t, f.ID); ok {
				in.Readings[f.ID] = fr
			}
		}
		out.Series = series.Filter(series.Build(in), q.Q)
		return nil
	})
	return out, err
}
