// Package graph is the Graph Engine: it turns a reproducible graph
// definition plus normalized measurements into a scientific representation
// (series, axes, statistics, provenance). Rendering and file formats are
// separate concerns (the interface renders; the export engine serializes).
//
// The engine never alters values for presentation: no smoothing, no
// clipping, no silent unit conversion, no hidden outlier removal.
package graph

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/reference"
	"github.com/oaovito/mne_lab/internal/science/stats"
)

// EngineVersion identifies the engine in provenance.
const EngineVersion = "graph-engine/1.0.0"

// Graph kinds.
const (
	KindDistribution   = "dls_distribution"
	KindParameterTime  = "parameter_time"
	KindDistributionAt = "dls_by_time"
)

// Errors (stable identifiers).
var (
	ErrNoData         = errors.New("graph.no_data")
	ErrMixedUnits     = errors.New("graph.mixed_units")
	ErrNoWeighting    = errors.New("graph.weighting_not_in_file")
	ErrNoDistribution = errors.New("graph.no_distribution")
	ErrNoParameter    = errors.New("graph.parameter_not_in_files")
)

// SeriesStyle is per-series visual configuration (never scientific data).
type SeriesStyle struct {
	Label  string `json:"label,omitempty"`
	Color  string `json:"color,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
}

// Visual is the visual configuration of a graph.
type Visual struct {
	Legend    bool    `json:"legend"`
	Metadata  bool    `json:"metadata"`
	Grid      bool    `json:"grid"`
	Points    bool    `json:"points"`
	LineWidth float64 `json:"lineWidth"`
	FontScale float64 `json:"fontScale"`
	// View is a zoom window; it changes what is shown, never the data.
	View *Range `json:"view,omitempty"`
}

// Range is an axis window.
type Range struct {
	XMin, XMax, YMin, YMax float64
}

// Definition is a saved, reproducible graph.
type Definition struct {
	Schema       int                    `json:"schema"`
	ID           string                 `json:"id"`
	Title        string                 `json:"title"`
	Kind         string                 `json:"kind"`
	Measurements []string               `json:"measurements"`
	Weighting    string                 `json:"weighting,omitempty"`
	XScale       string                 `json:"xScale,omitempty"` // auto, log, linear
	CycleID      string                 `json:"cycleId,omitempty"`
	Param        string                 `json:"param,omitempty"`
	Series       map[string]SeriesStyle `json:"series,omitempty"`
	Visual       Visual                 `json:"visual"`
	Tags         []string               `json:"tags,omitempty"`
	Notes        string                 `json:"notes,omitempty"`
	Created      time.Time              `json:"created"`
	Updated      time.Time              `json:"updated"`
	Trashed      bool                   `json:"trashed,omitempty"`
}

// DefaultVisual is the visual configuration of a new graph.
func DefaultVisual() Visual {
	return Visual{Legend: true, Metadata: true, Grid: true, LineWidth: 1.75, FontScale: 1}
}

// Axis describes one axis.
type Axis struct {
	Label string  `json:"label"` // neutral key, localized by the interface
	Unit  string  `json:"unit"`
	Scale string  `json:"scale"` // log or linear
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
}

// Series is one plotted series.
type Series struct {
	ID            string            `json:"id"`
	Label         string            `json:"label"`
	MeasurementID string            `json:"measurementId,omitempty"`
	X             []float64         `json:"x"`
	Y             []float64         `json:"y"`
	Err           []float64         `json:"err,omitempty"` // SD at each point when n≥2
	N             []int             `json:"n,omitempty"`
	Kind          string            `json:"kind"` // line, mean, individual
	Meta          map[string]string `json:"meta,omitempty"`
	Hidden        bool              `json:"hidden,omitempty"`
	Color         string            `json:"color,omitempty"`
}

// Source is one input of a graph.
type Source struct {
	MeasurementID string `json:"measurementId"`
	FileID        string `json:"fileId"`
	FileName      string `json:"fileName"`
	SHA256        string `json:"sha256"`
	Parser        string `json:"parser"`
	Spec          string `json:"spec"`
	SampleID      string `json:"sampleId,omitempty"`
	MeasuredAt    string `json:"measuredAt,omitempty"`
	Lines         string `json:"lines,omitempty"`
}

// Provenance documents how a result was produced.
type Provenance struct {
	Engine          string    `json:"engine"`
	Spec            string    `json:"spec"`
	Rules           []string  `json:"rules"`
	Sources         []Source  `json:"sources"`
	Transformations []string  `json:"transformations"`
	Statistics      string    `json:"statistics,omitempty"`
	ComputedAt      time.Time `json:"computedAt"`
}

// Result is the computed scientific representation.
type Result struct {
	Kind       string     `json:"kind"`
	Title      string     `json:"title"`
	X          Axis       `json:"x"`
	Y          Axis       `json:"y"`
	Series     []Series   `json:"series"`
	Gaps       []float64  `json:"gaps,omitempty"`
	Provenance Provenance `json:"provenance"`
	Warnings   []string   `json:"warnings,omitempty"`
	Visual     Visual     `json:"visual"`
	Weightings []string   `json:"weightings,omitempty"`
}

// Input bundles what the engine needs.
type Input struct {
	Measurements map[string]model.Measurement
	Files        map[string]model.SourceFile
}

func (in Input) source(m model.Measurement) Source {
	f := in.Files[m.FileID]
	s := Source{MeasurementID: m.ID, FileID: m.FileID, FileName: f.Name, SHA256: f.SHA256, Parser: m.Parser, Spec: m.Spec, SampleID: m.SampleID}
	if m.MeasuredAt != nil {
		s.MeasuredAt = m.MeasuredAt.Time.Format(time.RFC3339)
	}
	if m.Dist != nil {
		s.Lines = fmt.Sprintf("%d-%d", m.Dist.FirstLine, m.Dist.LastLine)
	}
	return s
}

func seriesLabel(def Definition, m model.Measurement, idx int) string {
	if st, ok := def.Series[m.ID]; ok && st.Label != "" {
		return st.Label
	}
	if m.Label != "" {
		return m.Label
	}
	label := m.SampleID
	if label == "" {
		label = fmt.Sprintf("#%d", idx+1)
	}
	if m.Replicate > 0 {
		label += fmt.Sprintf(" · R%d", m.Replicate)
	}
	return label
}

// distinctLabels keeps automatic legend entries apart: equal labels get the
// measurement date and, if still equal, a running number. Labels the user
// typed are left as they are.
func distinctLabels(def Definition, series []Series, ms []model.Measurement) {
	groups := map[string][]int{}
	for i, s := range series {
		if st, ok := def.Series[ms[i].ID]; ok && st.Label != "" {
			continue
		}
		groups[s.Label] = append(groups[s.Label], i)
	}
	for _, idx := range groups {
		if len(idx) < 2 {
			continue
		}
		dates := map[string]bool{}
		for _, i := range idx {
			if at := ms[i].MeasuredAt; at != nil {
				dates[at.Time.Format("2006-01-02")] = true
			}
		}
		if len(dates) > 1 {
			for _, i := range idx {
				if at := ms[i].MeasuredAt; at != nil {
					series[i].Label += " · " + at.Time.Format("2006-01-02")
				}
			}
		}
		seen := map[string]int{}
		for _, i := range idx {
			seen[series[i].Label]++
		}
		count := map[string]int{}
		for _, i := range idx {
			l := series[i].Label
			if seen[l] > 1 {
				count[l]++
				series[i].Label = fmt.Sprintf("%s (%d)", l, count[l])
			}
		}
	}
}

func meta(m model.Measurement) map[string]string {
	out := map[string]string{}
	for _, k := range []string{model.EffectiveDiameter, model.Polydispersity, model.CountRate, model.BaselineIndex} {
		if q, ok := m.Params[k]; ok {
			out[k] = q.Raw
			if q.Unit != "" {
				out[k+".unit"] = q.Unit
			}
		}
	}
	if m.SampleID != "" {
		out["sample"] = m.SampleID
	}
	if m.MeasuredAt != nil {
		out["measuredAt"] = m.MeasuredAt.Time.Format(time.RFC3339)
		if !m.MeasuredAt.TZKnown {
			out["measuredAt.tz"] = "unknown"
		}
	}
	return out
}

// Distribution computes a particle size distribution graph from one or
// more measurements (one series per measurement, overlaid).
func Distribution(def Definition, in Input) (Result, error) {
	w := def.Weighting
	if w == "" {
		w = "intensity"
	}
	res := Result{Kind: KindDistribution, Title: def.Title, Visual: def.Visual,
		Provenance: Provenance{Engine: EngineVersion, Spec: reference.LightScatteringSpecID, Rules: []string{"ls.graph.axes", "ls.weighting", "ls.units"},
			Transformations: []string{"none: values plotted exactly as read from the file"}, ComputedAt: time.Now().UTC()}}
	res.X = Axis{Label: "axis.hydrodynamic_diameter"}
	res.Y = Axis{Label: "axis." + w}
	allPositive := true
	xu, yu := "", ""
	first := true
	weightSet := map[string]bool{}
	xmin, xmax, ymin, ymax := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
	var skipErr error
	var used []model.Measurement
	for i, id := range def.Measurements {
		m, ok := in.Measurements[id]
		if !ok {
			res.Warnings = append(res.Warnings, "graph.missing_measurement:"+id)
			continue
		}
		// A measurement without what this graph needs is left out with a
		// note; the graph fails only when nothing can be drawn.
		var dc, wc *model.Column
		if m.Dist != nil {
			for _, k := range m.Dist.Weightings() {
				weightSet[k] = true
			}
			dc, wc = m.Dist.Column("diameter"), m.Dist.Column(w)
		}
		if dc == nil {
			skipErr = ErrNoDistribution
			res.Warnings = append(res.Warnings, "graph.skipped_no_distribution:"+seriesLabel(def, m, i))
			continue
		}
		if wc == nil {
			if skipErr == nil {
				skipErr = ErrNoWeighting
			}
			res.Warnings = append(res.Warnings, "graph.skipped_no_weighting:"+seriesLabel(def, m, i))
			continue
		}
		if first {
			xu, yu, first = dc.Unit, wc.Unit, false
		} else if dc.Unit != xu || wc.Unit != yu {
			return res, ErrMixedUnits
		}
		s := Series{ID: m.ID, MeasurementID: m.ID, Label: seriesLabel(def, m, i), Kind: "line", Meta: meta(m),
			X: append([]float64(nil), dc.Values...), Y: append([]float64(nil), wc.Values...)}
		if st, ok := def.Series[m.ID]; ok {
			s.Hidden, s.Color = st.Hidden, st.Color
		}
		for j := range s.X {
			if s.X[j] <= 0 {
				allPositive = false
			}
			xmin, xmax = math.Min(xmin, s.X[j]), math.Max(xmax, s.X[j])
			ymin, ymax = math.Min(ymin, s.Y[j]), math.Max(ymax, s.Y[j])
		}
		res.Series = append(res.Series, s)
		used = append(used, m)
		res.Provenance.Sources = append(res.Provenance.Sources, in.source(m))
	}
	distinctLabels(def, res.Series, used)
	if len(res.Series) == 0 {
		if skipErr != nil {
			return res, skipErr
		}
		return res, ErrNoData
	}
	for _, k := range []string{"intensity", "volume", "number"} {
		if weightSet[k] {
			res.Weightings = append(res.Weightings, k)
		}
	}
	res.X.Unit, res.Y.Unit = xu, yu
	if xu == "" {
		res.Warnings = append(res.Warnings, "graph.x_unit_not_stated")
	}
	if yu == "" {
		res.Warnings = append(res.Warnings, "graph.y_unit_not_stated")
	}
	res.X.Scale = scale(def.XScale, allPositive, &res)
	res.X.Min, res.X.Max = xmin, xmax
	res.Y.Min, res.Y.Max = math.Min(0, ymin), ymax
	return res, nil
}

func scale(want string, allPositive bool, res *Result) string {
	switch want {
	case "linear":
		return "linear"
	case "log":
		if allPositive {
			return "log"
		}
		res.Warnings = append(res.Warnings, "graph.log_needs_positive_values")
		return "linear"
	}
	if allPositive {
		return "log"
	}
	return "linear"
}

// CycleInput adds the cycle needed by temporal graphs.
type CycleInput struct {
	Config      cycle.Config
	Points      []cycle.Point
	Assignments []cycle.Assignment
}

// ParameterTime computes a parameter (Effective Diameter, PDI, Count Rate,
// BaseLine Index) over a cycle: individual replicate values plus mean ± SD
// where n ≥ 2. Missing points are reported as gaps.
func ParameterTime(def Definition, in Input, cy CycleInput) (Result, error) {
	res := Result{Kind: KindParameterTime, Title: def.Title, Visual: def.Visual,
		Provenance: Provenance{Engine: EngineVersion, Spec: reference.LightScatteringSpecID, Rules: []string{"ls.stats", "ls.cycles", "ls.units"},
			Statistics: "mean and sample standard deviation (n−1), n ≥ 2", ComputedAt: time.Now().UTC(),
			Transformations: []string{"replicates grouped by cycle point; individual values kept"}}}
	res.X = Axis{Label: "axis.time." + string(cy.Config.Unit), Unit: string(cy.Config.Unit), Scale: "linear"}
	res.Y = Axis{Label: "param." + def.Param, Scale: "linear"}
	values := map[string]float64{}
	unit, haveUnit := "", false
	for _, a := range cy.Assignments {
		m, ok := in.Measurements[a.MeasurementID]
		if !ok {
			continue
		}
		q, ok := m.Params[def.Param]
		if !ok {
			continue
		}
		if !haveUnit {
			unit, haveUnit = q.Unit, true
		} else if q.Unit != unit {
			return res, ErrMixedUnits
		}
		values[m.ID] = q.Value
	}
	if len(values) == 0 {
		return res, ErrNoParameter
	}
	res.Y.Unit = unit
	series := cycle.Series(cy.Points, cy.Assignments, values)
	ind := Series{ID: "individual", Label: "series.individual", Kind: "individual"}
	mean := Series{ID: "mean", Label: "series.mean_sd", Kind: "mean"}
	ymin, ymax := math.Inf(1), math.Inf(-1)
	used := map[string]bool{}
	for _, pr := range series {
		x := float64(pr.Offset)
		if pr.Missing {
			res.Gaps = append(res.Gaps, x)
			continue
		}
		for _, v := range pr.Summary.Values {
			ind.X, ind.Y = append(ind.X, x), append(ind.Y, v)
			ymin, ymax = math.Min(ymin, v), math.Max(ymax, v)
		}
		for _, id := range pr.Sources {
			used[id] = true
		}
		if pr.Summary.Mean != nil {
			mean.X = append(mean.X, x)
			mean.Y = append(mean.Y, *pr.Summary.Mean)
			mean.Err = append(mean.Err, *pr.Summary.SD)
			mean.N = append(mean.N, pr.Summary.N)
			ymin = math.Min(ymin, *pr.Summary.Mean-*pr.Summary.SD)
			ymax = math.Max(ymax, *pr.Summary.Mean+*pr.Summary.SD)
		}
	}
	for _, se := range []*Series{&ind, &mean} {
		if st, ok := def.Series[se.ID]; ok {
			se.Hidden, se.Color = st.Hidden, st.Color
		}
	}
	res.Series = append(res.Series, ind)
	if len(mean.X) > 0 {
		res.Series = append(res.Series, mean)
	} else {
		res.Warnings = append(res.Warnings, "graph.not_enough_replicates_for_statistics")
	}
	ids := make([]string, 0, len(used))
	for id := range used {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		res.Provenance.Sources = append(res.Provenance.Sources, in.source(in.Measurements[id]))
	}
	res.X.Min = 0
	res.X.Max = float64(cy.Config.Duration)
	res.Y.Min, res.Y.Max = ymin, ymax
	return res, nil
}

// DistributionAtPoints overlays the distributions of a cycle, one series
// per measurement, labeled by its cycle point. Distributions are never
// averaged.
func DistributionAtPoints(def Definition, in Input, cy CycleInput) (Result, error) {
	var ids []string
	labels := map[string]string{}
	// Points in time order, replicates in order within a point.
	as := append([]cycle.Assignment(nil), cy.Assignments...)
	sort.SliceStable(as, func(i, j int) bool {
		if as[i].Point != as[j].Point {
			return as[i].Point < as[j].Point
		}
		return as[i].Replicate < as[j].Replicate
	})
	for _, a := range as {
		if a.Point >= 0 && a.Point < len(cy.Points) && (a.Status == "auto" || a.Status == "confirmed") {
			if m, ok := in.Measurements[a.MeasurementID]; ok && m.Dist != nil {
				ids = append(ids, a.MeasurementID)
				labels[a.MeasurementID] = fmt.Sprintf("%s:%d", cy.Config.Unit, cy.Points[a.Point].Offset)
				if a.Replicate > 0 {
					labels[a.MeasurementID] += fmt.Sprintf(":%d", a.Replicate)
				}
			}
		}
	}
	d := def
	d.Measurements = ids
	if d.Series == nil {
		d.Series = map[string]SeriesStyle{}
	}
	for id, l := range labels {
		st := d.Series[id]
		if st.Label == "" {
			st.Label = "point:" + l
		}
		d.Series[id] = st
	}
	res, err := Distribution(d, in)
	res.Kind = KindDistributionAt
	res.Provenance.Rules = append(res.Provenance.Rules, "ls.cycles")
	return res, err
}

// Stats summarizes one parameter for a set of measurements (graph metadata
// and the "compare" view).
func Stats(in Input, ids []string, param string) (stats.Summary, string, error) {
	var vs []float64
	unit, have := "", false
	for _, id := range ids {
		m, ok := in.Measurements[id]
		if !ok {
			continue
		}
		q, ok := m.Params[param]
		if !ok {
			continue
		}
		if have && q.Unit != unit {
			return stats.Summary{}, "", ErrMixedUnits
		}
		unit, have = q.Unit, true
		vs = append(vs, q.Value)
	}
	return stats.Describe(vs), unit, nil
}
