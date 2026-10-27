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
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/reference"
	"github.com/oaovito/mne_lab/internal/science/stats"
)

// EngineVersion identifies the engine in provenance.
const EngineVersion = "graph-engine/1.3.0"

// Graph kinds.
const (
	KindDistribution   = "dls_distribution"
	KindParameterTime  = "parameter_time"
	KindDistributionAt = "dls_by_time"
	KindStatistical    = "statistical_groups"
	// KindSeriesValues draws the values of chosen summary or instrument
	// reading series side by side, one category per series.
	KindSeriesValues = "series_values"
	// KindDesignTime draws a parameter of an experimental design over its
	// time points, one mean ± error bar per condition, with n counted in
	// experimental units (condition × time graphs).
	KindDesignTime = "design_time"
	// KindPairedCondition joins each experimental unit's values under the
	// conditions of the design at one time point (paired view).
	KindPairedCondition = "paired_condition"
)

// Errors (stable identifiers).
var (
	ErrNoData         = errors.New("graph.no_data")
	ErrMixedUnits     = errors.New("graph.mixed_units")
	ErrNoWeighting    = errors.New("graph.weighting_not_in_file")
	ErrNoDistribution = errors.New("graph.no_distribution")
	ErrNoParameter    = errors.New("graph.parameter_not_in_files")
	ErrMixedVariables = errors.New("graph.mixed_variables")
	ErrSeriesKind     = errors.New("graph.series_kind_not_supported")
)

// SeriesStyle is per-series visual configuration (never scientific data).
type SeriesStyle struct {
	Label  string `json:"label,omitempty"`
	Color  string `json:"color,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
	// Marker and LineStyle override the default shape and pattern (empty
	// means the default; see the plot package for the names).
	Marker    string `json:"marker,omitempty"`
	LineStyle string `json:"lineStyle,omitempty"`
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
	// LegendMode lists all series, the visible ones (empty, the default) or
	// only LegendSeries ("selected"); series left out are counted as "+N more".
	LegendMode   string   `json:"legendMode,omitempty"`
	LegendSeries []string `json:"legendSeries,omitempty"`
	// Margin around the figure in mm (0 keeps the figure preset's margin)
	// and the publication preset last applied to these settings.
	Margin float64 `json:"margin,omitempty"`
	Preset string  `json:"preset,omitempty"`
}

// Range is an axis window.
type Range struct {
	XMin, XMax, YMin, YMax float64
}

// Definition is a saved, reproducible graph.
type Definition struct {
	Schema          int                    `json:"schema"`
	ID              string                 `json:"id"`
	Title           string                 `json:"title"`
	Kind            string                 `json:"kind"`
	AnalysisID      string                 `json:"analysisId,omitempty"`
	ErrorBars       string                 `json:"errorBars,omitempty"`       // sd, sem or ci; explicit statistical graph choice
	Annotations     []string               `json:"annotations,omitempty"`     // saved analysis comparison IDs, explicitly selected
	AnnotationStyle string                 `json:"annotationStyle,omitempty"` // exact or stars
	Measurements    []string               `json:"measurements"`
	Distributions   map[string]string      `json:"distributions,omitempty"` // measurement -> original distribution candidate
	Weighting       string                 `json:"weighting,omitempty"`
	XScale          string                 `json:"xScale,omitempty"` // auto, log, linear
	CycleID         string                 `json:"cycleId,omitempty"`
	Param           string                 `json:"param,omitempty"`
	Series          map[string]SeriesStyle `json:"series,omitempty"`
	Visual          Visual                 `json:"visual"`
	Tags            []string               `json:"tags,omitempty"`
	Notes           string                 `json:"notes,omitempty"`
	Created         time.Time              `json:"created"`
	Updated         time.Time              `json:"updated"`
	Trashed         bool                   `json:"trashed,omitempty"`

	// Schema 2 fields. All are optional: a definition without them draws
	// exactly as it did under schema 1.

	// SeriesIDs selects series by their stable catalog identifier (never by
	// label). When set, they decide what a selection-based graph draws;
	// Measurements, Distributions, Weighting and Param are then derived
	// from them on save so older readers still find the same data.
	SeriesIDs []string `json:"seriesIds,omitempty"`
	// FocusSeries draws only these series in full; the others are kept in
	// the graph but muted and left out of the legend.
	FocusSeries []string `json:"focusSeries,omitempty"`
	Subtitle    string   `json:"subtitle,omitempty"`
	XLabel      string   `json:"xLabel,omitempty"` // axis titles typed by the person; the unit is always added
	YLabel      string   `json:"yLabel,omitempty"`
	YScale      string   `json:"yScale,omitempty"` // linear (default) or log
	// DisplayUnit shows the Y values in another unit of the same dimension
	// with an exact factor; stored values and units never change.
	DisplayUnit string `json:"displayUnit,omitempty"`
	// ShowIndividual and ConnectMeans default to true when absent.
	ShowIndividual *bool            `json:"showIndividual,omitempty"`
	ConnectMeans   *bool            `json:"connectMeans,omitempty"`
	TextNotes      []TextAnnotation `json:"textNotes,omitempty"`
	// Lineage and the versions in force when the graph was saved.
	DerivedFrom        []Lineage `json:"derivedFrom,omitempty"`
	SourceFiles        []string  `json:"sourceFiles,omitempty"`
	ParserVersions     []string  `json:"parserVersions,omitempty"`
	CalculationVersion string    `json:"calculationVersion,omitempty"`

	// Design graphs (design_time and paired_condition). DesignID names the
	// saved design revision (pinned, so the graph stays reproducible);
	// Conditions are the levels shown, in order (empty: every condition of
	// the design); PairedAt is the time point of the paired view; Groups
	// restrict the measurements to the members of saved groups (references
	// only); UnitTrajectories draws each unit's thin secondary line.
	DesignID         string   `json:"designId,omitempty"`
	Conditions       []string `json:"conditions,omitempty"`
	PairedAt         string   `json:"pairedAt,omitempty"`
	Groups           []string `json:"groups,omitempty"`
	UnitTrajectories bool     `json:"unitTrajectories,omitempty"`
	// GroupSnapshots pin the members of each group in Groups as they were
	// when the graph was saved, so the graph redraws the same measurements
	// after a group is edited (the change is reported, never followed).
	GroupSnapshots []GroupSnapshot `json:"groupSnapshots,omitempty"`
	// Parts are the selections of the parents of a combined design graph:
	// each part draws exactly the conditions and group members its parent
	// drew. When present they replace Conditions and Groups (which then
	// list their union for older readers). Written only by Combine.
	Parts []DesignPart `json:"parts,omitempty"`
}

// GroupSnapshot is the member list of a saved group when a graph was saved.
type GroupSnapshot struct {
	ID           string   `json:"id"`
	Name         string   `json:"name,omitempty"`
	Measurements []string `json:"measurements"`
	// Hash is the SHA-256 of the sorted member list (MembersHash).
	Hash string `json:"hash"`
}

// DesignPart is what one parent of a combined design graph showed.
type DesignPart struct {
	GraphID string `json:"graphId,omitempty"`
	Title   string `json:"title,omitempty"`
	// Conditions are the conditions the parent drew (every condition of the
	// design when it drew all of them).
	Conditions []string `json:"conditions"`
	// Groups restrict the part to the members of these groups, pinned as
	// the parent had them.
	Groups []GroupSnapshot `json:"groups,omitempty"`
}

// DefaultVisual is the visual configuration of a new graph.
func DefaultVisual() Visual {
	return Visual{Legend: true, Metadata: true, Grid: true, LineWidth: 1.75, FontScale: 1}
}

// Axis describes one axis.
type Axis struct {
	Categories []string `json:"categories,omitempty"`
	Label      string   `json:"label"` // neutral key, localized by the interface
	Unit       string   `json:"unit"`
	Scale      string   `json:"scale"` // log or linear
	Min        float64  `json:"min"`
	Max        float64  `json:"max"`
	// Title replaces the localized label with the person's own text.
	Title string `json:"title,omitempty"`
	// Conversion records a display-only unit change of this axis.
	Conversion *AxisConversion `json:"conversion,omitempty"`
	// Ticks places the labelled ticks at these values (the time points of
	// a design) instead of evenly spaced ones.
	Ticks []float64 `json:"ticks,omitempty"`
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
	Marker        string            `json:"marker,omitempty"`
	LineStyle     string            `json:"lineStyle,omitempty"`
	// SeriesID is the catalog identifier the series was selected by.
	SeriesID string `json:"seriesId,omitempty"`
	// Muted series stay in the graph behind the focused ones.
	Muted bool `json:"muted,omitempty"`
	// NoLine draws a mean series without the line between its points.
	NoLine bool `json:"noLine,omitempty"`
	// YOriginal keeps the values in their stored unit when the axis shows
	// them in a display unit.
	YOriginal []float64 `json:"yOriginal,omitempty"`
	// PointLabels names the experimental unit of each point of a series of
	// unit values (design graphs).
	PointLabels []string `json:"pointLabels,omitempty"`
	// Gaps are X positions where this series has no value: its line is
	// interrupted there, never drawn across.
	Gaps []float64 `json:"gaps,omitempty"`
}

// Source is one input of a graph.
type Source struct {
	ProducerOrigin *model.ProducerOrigin     `json:"producerOrigin"`
	Parameters     map[string]model.Quantity `json:"parameters,omitempty"`
	MeasurementID  string                    `json:"measurementId"`
	FileID         string                    `json:"fileId"`
	FileName       string                    `json:"fileName"`
	SHA256         string                    `json:"sha256"`
	Parser         string                    `json:"parser"`
	Spec           string                    `json:"spec"`
	SampleID       string                    `json:"sampleId,omitempty"`
	MeasuredAt     string                    `json:"measuredAt,omitempty"`
	// MeasuredTimestamp preserves raw text and explicit zone/date semantics.
	MeasuredTimestamp *model.Timestamp       `json:"measuredTimestamp,omitempty"`
	Lines             string                 `json:"lines,omitempty"`
	SourceRange       string                 `json:"sourceRange,omitempty"`
	ImportSelection   *model.ImportSelection `json:"importSelection,omitempty"`
	SourceSheet       string                 `json:"sourceSheet,omitempty"`
	DistributionID    string                 `json:"distributionId,omitempty"`
	Method            string                 `json:"method,omitempty"`
	Format            string                 `json:"format,omitempty"`
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
	Kind            string                  `json:"kind"`
	Title           string                  `json:"title"`
	X               Axis                    `json:"x"`
	Y               Axis                    `json:"y"`
	Series          []Series                `json:"series"`
	Gaps            []float64               `json:"gaps,omitempty"`
	Provenance      Provenance              `json:"provenance"`
	Warnings        []string                `json:"warnings,omitempty"`
	Visual          Visual                  `json:"visual"`
	Weightings      []string                `json:"weightings,omitempty"`
	Annotations     []StatisticalAnnotation `json:"annotations,omitempty"`
	AnnotationStyle string                  `json:"annotationStyle,omitempty"`
	Subtitle        string                  `json:"subtitle,omitempty"`
	// ErrorBars names what the error bars show (sd, sem or ci), so they are
	// never drawn without a description.
	ErrorBars string           `json:"errorBars,omitempty"`
	TextNotes []TextAnnotation `json:"textNotes,omitempty"`
	// Design identifies the experimental design revision of a design graph.
	Design *DesignRef `json:"design,omitempty"`
}

// Input bundles what the engine needs. Readings (by file ID) are needed
// only for instrument reading series.
type Input struct {
	Measurements map[string]model.Measurement
	Files        map[string]model.SourceFile
	Readings     map[string]model.FileReadings
}

// Source snapshots provenance without guessing a zone or aliasing source time.
func (in Input) Source(m model.Measurement) Source {
	f := in.Files[m.FileID]
	s := Source{MeasurementID: m.ID, FileID: m.FileID, FileName: f.Name, SHA256: f.SHA256, Parser: m.Parser, Spec: m.Spec, SampleID: m.SampleID}
	s.ProducerOrigin = model.CloneProducerOrigin(f.ProducerOrigin)
	s.SourceSheet, s.SourceRange = m.SourceSheet, m.SourceRange
	for k, q := range m.Params {
		if q.UnitOrigin != "" {
			if s.Parameters == nil {
				s.Parameters = map[string]model.Quantity{}
			}
			s.Parameters[k] = q
		}
	}
	if f.ImportSelection != nil {
		s.ImportSelection = f.ImportSelection.Clone()
	}
	if m.MeasuredAt != nil {
		ts := *m.MeasuredAt
		s.MeasuredTimestamp = &ts
		s.MeasuredAt = ts.ISOTime()
	}
	if m.Dist != nil {
		s.Lines = fmt.Sprintf("%d-%d", m.Dist.FirstLine, m.Dist.LastLine)
		s.DistributionID, s.Method, s.Format = m.Dist.ID, m.Dist.Method, m.Dist.Format
	}
	return s
}

func seriesLabel(def Definition, m model.Measurement, idx int) string {
	return keyedLabel(def, m.ID, m, idx)
}

// keyedLabel is the label of a series whose style is kept under key (the
// measurement ID in schema 1, the SeriesID in schema 2).
func keyedLabel(def Definition, key string, m model.Measurement, idx int) string {
	if st, ok := styleOf(def, key, m.ID); ok && st.Label != "" {
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
		key := ms[i].ID
		if s.SeriesID != "" {
			key = s.SeriesID
		}
		if st, ok := styleOf(def, key, ms[i].ID); ok && st.Label != "" {
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
	for _, k := range []string{model.EffectiveDiameter, model.Polydispersity, model.CountRate, model.AverageCountRate, model.BaselineIndex} {
		if q, ok := m.Params[k]; ok {
			out[k] = q.Raw
			if q.UnitOrigin != "" {
				out[k+".unitOrigin"] = q.UnitOrigin
			}
			if q.SourceColumn > 0 {
				out[k+".sourceColumn"] = fmt.Sprint(q.SourceColumn)
			}
			if q.Unit != "" {
				out[k+".unit"] = q.Unit
			}
		}
	}
	if m.SampleID != "" {
		out["sample"] = m.SampleID
	}
	if m.MeasuredAt != nil {
		out["measuredAt"] = m.MeasuredAt.ISOTime()
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
	nativeUsed := false
	var used []model.Measurement
	items, w, err := distributionItems(def, in, w, &res)
	if err != nil {
		return res, err
	}
	res.Y.Label = "axis." + w
	for i, it := range items {
		m := it.m
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
			res.Warnings = append(res.Warnings, "graph.skipped_no_distribution:"+keyedLabel(def, it.key, m, i))
			continue
		}
		if wc == nil {
			if skipErr == nil {
				skipErr = ErrNoWeighting
			}
			res.Warnings = append(res.Warnings, "graph.skipped_no_weighting:"+keyedLabel(def, it.key, m, i))
			continue
		}
		if m.Dist.Method != "" && !nativeUsed {
			res.Provenance.Rules = append(res.Provenance.Rules, "ls.methods", "ls.native-columns")
			nativeUsed = true
		}
		if first {
			xu, yu, first = dc.Unit, wc.Unit, false
		} else if dc.Unit != xu || wc.Unit != yu {
			return res, ErrMixedUnits
		}
		s := Series{ID: it.key, MeasurementID: m.ID, Label: keyedLabel(def, it.key, m, i), Kind: "line", Meta: meta(m),
			X: append([]float64(nil), dc.Values...), Y: append([]float64(nil), wc.Values...), SeriesID: it.seriesID}
		if st, ok := styleOf(def, it.key, m.ID); ok {
			s.Hidden, s.Color, s.Marker, s.LineStyle = st.Hidden, st.Color, st.Marker, st.LineStyle
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
		res.Provenance.Sources = append(res.Provenance.Sources, in.Source(m))
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

// NeedsUnit reports whether a parameter key (or an axis label such as
// "param.effective_diameter") names a quantity measured in some unit, so an
// empty unit means the file did not state it.
func NeedsUnit(key string) bool {
	key = strings.TrimPrefix(key, "param.")
	switch key {
	case model.EffectiveDiameter, model.CountRate, model.AverageCountRate, "axis.hydrodynamic_diameter":
		return true
	}
	return false
}

// CycleInput adds the cycle needed by temporal graphs.
type CycleInput struct {
	Config      cycle.Config
	Points      []cycle.Point
	Assignments []cycle.Assignment
}

// ParameterTime computes a parameter (Effective Diameter, PDI, current or
// average Count Rate, BaseLine Index) over a cycle: individual values plus mean ± SD
// where n ≥ 2. Missing points are reported as gaps.
func ParameterTime(def Definition, in Input, cy CycleInput) (Result, error) {
	res := Result{Kind: KindParameterTime, Title: def.Title, Visual: def.Visual,
		Provenance: Provenance{Engine: EngineVersion, Spec: reference.LightScatteringSpecID, Rules: []string{"ls.stats", "ls.cycles", "ls.units"},
			Statistics: "mean and sample standard deviation (n−1), n ≥ 2", ComputedAt: time.Now().UTC(),
			Transformations: []string{"replicates grouped by cycle point; individual values kept"}}}
	res.X = Axis{Label: "axis.time." + string(cy.Config.Unit), Unit: string(cy.Config.Unit), Scale: "linear"}
	res.Y = Axis{Label: "param." + def.Param, Scale: "linear"}
	if def.Param == model.AverageCountRate {
		res.Provenance.Rules = append(res.Provenance.Rules, "ls.count-rate-mean")
	}
	values := map[string]float64{}
	unit, haveUnit := "", false
	mapped := false
	for _, a := range cy.Assignments {
		if a.Point < 0 || a.Point >= len(cy.Points) || (a.Status != "auto" && a.Status != "confirmed") {
			continue
		}
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
		mapped = mapped || q.UnitOrigin == "user_mapping"
	}
	if len(values) == 0 {
		return res, ErrNoParameter
	}
	if mapped {
		res.Warnings = append(res.Warnings, "ls.user_mapping_unvalidated")
	}
	res.Y.Unit = unit
	if unit == "" && NeedsUnit(def.Param) {
		res.Warnings = append(res.Warnings, "graph.y_unit_not_stated")
	}
	series := cycle.Series(cy.Points, cy.Assignments, values)
	ind := Series{ID: "individual", Label: "series.individual", Kind: "individual"}
	mean := Series{ID: "mean", Label: "series.mean_sd", Kind: "mean"}
	// SD is the default. A confidence interval needs a saved statistical
	// analysis (decision D8), so it is never computed here. SEM is not
	// drawn either: n at a cycle point counts measurements, which may be
	// technical repeats of one preparation, not experimental units, so
	// SD/√n would understate the uncertainty. The design graph (n =
	// experimental units) offers SEM.
	switch def.ErrorBars {
	case "sd":
		res.ErrorBars = "sd"
	case "sem":
		res.ErrorBars = "sd"
		res.Warnings = append(res.Warnings, "graph.sem_needs_experimental_units")
	case "ci":
		res.ErrorBars = "sd"
		res.Warnings = append(res.Warnings, "graph.ci_needs_saved_analysis")
	}
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
			bar := *pr.Summary.SD
			mean.Err = append(mean.Err, bar)
			mean.N = append(mean.N, pr.Summary.N)
			ymin = math.Min(ymin, *pr.Summary.Mean-bar)
			ymax = math.Max(ymax, *pr.Summary.Mean+bar)
		}
	}
	for _, se := range []*Series{&ind, &mean} {
		if st, ok := def.Series[se.ID]; ok {
			se.Hidden, se.Color, se.Marker, se.LineStyle = st.Hidden, st.Color, st.Marker, st.LineStyle
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
		res.Provenance.Sources = append(res.Provenance.Sources, in.Source(in.Measurements[id]))
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
			if m, ok := in.Measurements[a.MeasurementID]; ok && selectedDistribution(def, m).Dist != nil {
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

// Saved definitions pin a candidate so a later library choice cannot change
// the method behind an existing scientific graph. Invalid pins never fall back.
func selectedDistribution(def Definition, m model.Measurement) model.Measurement {
	if id, pinned := def.Distributions[m.ID]; pinned {
		m.Dist = nil
		for i := range m.Distributions {
			if m.Distributions[i].ID == id {
				m.Dist = &m.Distributions[i]
				break
			}
		}
	}
	return m
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
