// Package series lists every plottable series of the imported scientific
// files with a stable identifier and its metadata.
//
// A SeriesID is derived only from stable record identifiers (measurement,
// distribution candidate, weighting, parameter, run position), never from
// a label, so renaming or relabeling never changes it and the same data
// always gets the same ID. Listing series reads records and never writes
// them; choosing some series for a graph leaves the others untouched.
package series

import (
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/oaovito/mne_lab/internal/science/design"
	"github.com/oaovito/mne_lab/internal/science/model"
)

// Version identifies the catalog contract in provenance records.
const Version = "series-catalog/1"

// Kinds of series.
const (
	KindScalar  = "scalar"  // one summary value of a measurement
	KindCurve   = "curve"   // one weighting of one distribution candidate
	KindReading = "reading" // one recognized column of one instrument run row
)

// Measurement types.
const (
	TypeSummary      = "dls_summary"
	TypeDistribution = "dls_distribution"
	TypeRunReading   = "dls_run_reading"
)

// TableDistribution names the single table of a plain export, which has no
// candidate identifier of its own.
const TableDistribution = "table"

// Params are the summary parameters offered as series, in display order.
var Params = []string{model.EffectiveDiameter, model.Polydispersity, model.CountRate, model.AverageCountRate, model.BaselineIndex}

var ErrID = errors.New("series.invalid_id")

func esc(s string) string { return url.PathEscape(s) }

// ScalarID identifies a summary value of a measurement.
func ScalarID(measurementID, param string) string {
	return "m:" + esc(measurementID) + "/p:" + esc(param)
}

// CurveID identifies one weighting of one distribution candidate.
func CurveID(measurementID, distributionID, weighting string) string {
	return "m:" + esc(measurementID) + "/d:" + esc(distributionID) + "/w:" + esc(weighting)
}

// ReadingID identifies a recognized column of a run row by its position.
func ReadingID(measurementID string, run int, param string) string {
	return "m:" + esc(measurementID) + "/r:" + strconv.Itoa(run) + "/p:" + esc(param)
}

// Ref is a parsed SeriesID.
type Ref struct {
	Kind          string `json:"kind"`
	MeasurementID string `json:"measurementId"`
	Param         string `json:"param,omitempty"`
	Distribution  string `json:"distribution,omitempty"`
	Weighting     string `json:"weighting,omitempty"`
	Run           int    `json:"run,omitempty"`
}

// Parse reads a SeriesID back into its parts.
func Parse(id string) (Ref, error) {
	parts := strings.Split(id, "/")
	get := func(p, prefix string) (string, bool) {
		if !strings.HasPrefix(p, prefix) {
			return "", false
		}
		v, err := url.PathUnescape(strings.TrimPrefix(p, prefix))
		return v, err == nil && v != ""
	}
	if len(parts) < 2 {
		return Ref{}, ErrID
	}
	m, ok := get(parts[0], "m:")
	if !ok {
		return Ref{}, ErrID
	}
	switch {
	case len(parts) == 2:
		p, ok := get(parts[1], "p:")
		if !ok {
			return Ref{}, ErrID
		}
		return Ref{Kind: KindScalar, MeasurementID: m, Param: p}, nil
	case len(parts) == 3 && strings.HasPrefix(parts[1], "d:"):
		d, ok1 := get(parts[1], "d:")
		w, ok2 := get(parts[2], "w:")
		if !ok1 || !ok2 {
			return Ref{}, ErrID
		}
		return Ref{Kind: KindCurve, MeasurementID: m, Distribution: d, Weighting: w}, nil
	case len(parts) == 3 && strings.HasPrefix(parts[1], "r:"):
		r, err := strconv.Atoi(strings.TrimPrefix(parts[1], "r:"))
		p, ok := get(parts[2], "p:")
		if err != nil || r <= 0 || !ok {
			return Ref{}, ErrID
		}
		return Ref{Kind: KindReading, MeasurementID: m, Run: r, Param: p}, nil
	}
	return Ref{}, ErrID
}

// Variable describes an axis quantity with neutral keys; units are those
// the file states (or the person declared), verbatim.
type Variable struct {
	Key    string `json:"key"`
	Label  string `json:"label,omitempty"` // exact source label or header
	Unit   string `json:"unit,omitempty"`
	Points int    `json:"points,omitempty"`
}

// CurveRef names the distribution candidate a curve comes from.
type CurveRef struct {
	ID        string `json:"id"`
	Method    string `json:"method,omitempty"`
	Format    string `json:"format,omitempty"`
	Weighting string `json:"weighting"`
	Selected  bool   `json:"selected"` // the measurement's current choice
}

// ReadingRef names the run row a reading comes from.
type ReadingRef struct {
	Index      int                `json:"index"`
	Run        string             `json:"run"`
	Line       int                `json:"line"`
	Header     string             `json:"header"`
	HeaderLine int                `json:"headerLine"`
	Value      model.ReadingValue `json:"value"`
	Parser     string             `json:"parser"`
}

// Provenance traces a series to its original file and organization.
type Provenance struct {
	Catalog        string `json:"catalog"`
	FileSHA256     string `json:"fileSha256"`
	SourceSheet    string `json:"sourceSheet,omitempty"`
	SourceRange    string `json:"sourceRange,omitempty"`
	FirstLine      int    `json:"firstLine,omitempty"`
	LastLine       int    `json:"lastLine,omitempty"`
	UnitOrigin     string `json:"unitOrigin,omitempty"`
	UnitLine       int    `json:"unitLine,omitempty"`
	Organization   string `json:"organization,omitempty"` // design or measurement
	DesignID       string `json:"designId,omitempty"`
	DesignRoot     string `json:"designRoot,omitempty"`
	DesignRevision int    `json:"designRevision,omitempty"`
}

// Series is one entry of the catalog, with the metadata addendum B §4 asks
// for whenever it is available.
type Series struct {
	ID                 string            `json:"id"`
	Kind               string            `json:"kind"`
	FileID             string            `json:"fileId"`
	FileName           string            `json:"fileName"`
	MeasurementID      string            `json:"measurementId"`
	MeasurementIndex   int               `json:"measurementIndex"`
	Sample             string            `json:"sample,omitempty"`
	Label              string            `json:"label,omitempty"` // the person's label; display only
	ExperimentalUnitID string            `json:"experimentalUnitId,omitempty"`
	ExperimentalUnit   string            `json:"experimentalUnit,omitempty"`
	Condition          string            `json:"condition,omitempty"`
	Time               *design.TimeLevel `json:"time,omitempty"`
	Replicate          int               `json:"replicate,omitempty"`
	ReplicateKind      string            `json:"replicateKind,omitempty"`
	MeasuredAt         *model.Timestamp  `json:"measuredAt,omitempty"`
	MeasurementType    string            `json:"measurementType"`
	X                  *Variable         `json:"x,omitempty"`
	Y                  Variable          `json:"y"`
	Value              *model.Quantity   `json:"value,omitempty"`
	Distribution       *CurveRef         `json:"distribution,omitempty"`
	Reading            *ReadingRef       `json:"reading,omitempty"`
	Parser             string            `json:"parser"`
	Spec               string            `json:"spec"`
	Provenance         Provenance        `json:"provenance"`
}

// Input is everything the catalog reads. Measurements are those of Files;
// Readings and Design are optional.
type Input struct {
	Files        []model.SourceFile
	Measurements map[string]model.Measurement
	Readings     map[string]model.FileReadings // by file ID
	Design       *design.Design
	UnitLabels   map[string]string // experimental unit ID → current label
}

// Build lists the series of the input in a stable order: by file name and
// ID, measurement position, then summary values, curves and readings.
func Build(in Input) []Series {
	files := append([]model.SourceFile{}, in.Files...)
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Name != files[j].Name {
			return files[i].Name < files[j].Name
		}
		return files[i].ID < files[j].ID
	})
	out := []Series{}
	for _, f := range files {
		var ms []model.Measurement
		for _, id := range f.Measurements {
			if m, ok := in.Measurements[id]; ok {
				ms = append(ms, m)
			}
		}
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].Index < ms[j].Index })
		readings := map[string]model.ReadingTable{}
		readingsParser := ""
		if fr, ok := in.Readings[f.ID]; ok {
			readingsParser = fr.Parser
			for _, tb := range fr.Tables {
				readings[tb.MeasurementID] = tb
			}
		}
		for _, m := range ms {
			base := in.base(f, m)
			for _, k := range Params {
				q, ok := m.Params[k]
				if !ok {
					continue
				}
				s := base
				s.ID, s.Kind, s.MeasurementType = ScalarID(m.ID, k), KindScalar, TypeSummary
				q2 := q
				s.Value = &q2
				s.Y = Variable{Key: k, Label: q.Label, Unit: q.Unit, Points: 1}
				s.Provenance.FirstLine, s.Provenance.LastLine = q.Line, q.Line
				s.Provenance.UnitOrigin, s.Provenance.UnitLine = q.UnitOrigin, q.UnitLine
				out = append(out, s)
			}
			for _, d := range candidates(m) {
				x := d.Column("diameter")
				for _, w := range d.Weightings() {
					y := d.Column(w)
					dist := d.ID
					if dist == "" {
						dist = TableDistribution
					}
					s := base
					s.ID, s.Kind, s.MeasurementType = CurveID(m.ID, dist, w), KindCurve, TypeDistribution
					s.Distribution = &CurveRef{ID: dist, Method: d.Method, Format: d.Format, Weighting: w, Selected: m.Dist != nil && (m.Dist.ID == d.ID)}
					if x != nil {
						s.X = &Variable{Key: "diameter", Label: x.Label, Unit: x.Unit, Points: len(x.Values)}
					}
					s.Y = Variable{Key: w, Label: y.Label, Unit: y.Unit, Points: len(y.Values)}
					s.Provenance.FirstLine, s.Provenance.LastLine = d.FirstLine, d.LastLine
					out = append(out, s)
				}
			}
			if tb, ok := readings[m.ID]; ok {
				for _, row := range tb.Rows {
					for c, col := range tb.Columns {
						if col.Key == "" || c >= len(row.Values) {
							continue
						}
						s := base
						s.ID, s.Kind, s.MeasurementType = ReadingID(m.ID, row.Index, col.Key), KindReading, TypeRunReading
						// A run row is a reading of the same measurement, never
						// another experimental unit or replicate.
						s.ReplicateKind = design.ReplicateInstrumentReading
						s.Replicate = 0
						s.Y = Variable{Key: col.Key, Label: col.Header, Unit: col.Unit, Points: 1}
						s.Reading = &ReadingRef{Index: row.Index, Run: row.Run, Line: row.Line, Header: col.Header, HeaderLine: tb.HeaderLine, Value: row.Values[c], Parser: readingsParser}
						s.Provenance.FirstLine, s.Provenance.LastLine, s.Provenance.UnitLine = row.Line, row.Line, 0
						if col.Unit != "" {
							s.Provenance.UnitOrigin, s.Provenance.UnitLine = "file_table_header", tb.HeaderLine
						}
						out = append(out, s)
					}
				}
			}
		}
	}
	return out
}

// candidates returns the distribution alternatives of a measurement once
// each: the instrument candidates, or the single table of a plain export.
func candidates(m model.Measurement) []model.Distribution {
	if len(m.Distributions) > 0 {
		return m.Distributions
	}
	if m.Dist != nil {
		return []model.Distribution{*m.Dist}
	}
	return nil
}

func (in Input) base(f model.SourceFile, m model.Measurement) Series {
	s := Series{FileID: f.ID, FileName: f.Name, MeasurementID: m.ID, MeasurementIndex: m.Index, Sample: m.SampleID, Label: m.Label,
		Replicate: m.Replicate, MeasuredAt: m.MeasuredAt, Parser: m.Parser, Spec: m.Spec,
		Provenance: Provenance{Catalog: Version, FileSHA256: f.SHA256, SourceSheet: m.SourceSheet, SourceRange: m.SourceRange}}
	if in.Design != nil {
		if a, ok := in.Design.Assignment(m.ID); ok {
			s.ExperimentalUnitID, s.Condition, s.ReplicateKind = a.UnitID, a.Condition, a.ReplicateKind
			if tl, ok := in.Design.Time(a.Time); ok {
				tl2 := tl
				s.Time = &tl2
			}
			s.Provenance.Organization, s.Provenance.DesignID, s.Provenance.DesignRoot, s.Provenance.DesignRevision = "design", in.Design.ID, in.Design.Root, in.Design.Revision
			s.ExperimentalUnit = in.label(a.UnitID, in.Design.UnitLabel(a.UnitID))
			return s
		}
	}
	s.ExperimentalUnitID, s.Condition = m.ExperimentalUnitID, m.Condition
	s.ExperimentalUnit = in.label(m.ExperimentalUnitID, "")
	if s.ExperimentalUnitID != "" || s.Condition != "" {
		s.Provenance.Organization = "measurement"
	}
	return s
}

func (in Input) label(id, fallback string) string {
	if id == "" {
		return ""
	}
	if l, ok := in.UnitLabels[id]; ok && l != "" {
		return l
	}
	return fallback
}

// Filter keeps the series whose visible metadata contains every word of q
// (case insensitive). Identity never depends on the result.
func Filter(list []Series, q string) []Series {
	words := strings.Fields(strings.ToLower(q))
	if len(words) == 0 {
		return list
	}
	out := []Series{}
	for _, s := range list {
		hay := strings.ToLower(strings.Join([]string{s.ID, s.FileName, s.Sample, s.Label, s.ExperimentalUnit, s.Condition, s.Y.Key, s.Y.Label, timeText(s.Time), s.ReplicateKind, s.Kind}, " "))
		ok := true
		for _, w := range words {
			ok = ok && strings.Contains(hay, w)
		}
		if ok {
			out = append(out, s)
		}
	}
	return out
}

func timeText(t *design.TimeLevel) string {
	if t == nil {
		return ""
	}
	return t.Label
}
