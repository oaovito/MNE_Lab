package graph

// Schema 2 of the graph definition: selection by SeriesID, display options
// of the Graph Builder and lineage. Everything here is additive: a schema 1
// definition never reaches these paths and draws as it always did.
//
// Extension points for the next phase (groups, combine, temporal design
// graphs, Graph → Analyze) are new optional fields on Definition and new
// Relation values on Lineage; a combined graph will record one Lineage per
// parent with RelationCombine.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/reference"
	"github.com/oaovito/mne_lab/internal/science/series"
	"github.com/oaovito/mne_lab/internal/science/units"
)

// DefinitionSchema is the schema written by this engine.
const DefinitionSchema = 2

// Lineage relations.
const RelationDuplicate = "duplicate"

// MaxSeries bounds a selection so a definition stays a reference list.
const MaxSeries = 500

// ErrInvalidSeries rejects a selection with an identifier that is not a
// SeriesID, or with too many series.
var ErrInvalidSeries = errors.New("graph.invalid_series")

// Lineage records the graph a definition was derived from.
type Lineage struct {
	GraphID  string `json:"graphId"`
	Relation string `json:"relation"`
	Title    string `json:"title,omitempty"`
	// DefinitionHash is the SHA-256 of the parent's stored definition when
	// the copy was made, so a later change of the parent can be told.
	DefinitionHash string    `json:"definitionHash"`
	At             time.Time `json:"at"`
}

// TextAnnotation is a note the person placed at data coordinates. It is
// display only and never computed from the data.
type TextAnnotation struct {
	Text string  `json:"text"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

// AxisConversion records a display-only unit change.
type AxisConversion struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Factor string `json:"factor"` // exact, such as "1/1000"
}

// Hash fingerprints a stored definition record.
func Hash(stored []byte) string {
	h := sha256.Sum256(stored)
	return hex.EncodeToString(h[:])
}

// styleOf finds a series style by its key, then by its measurement (styles
// of a schema 1 graph keep applying after its selection is edited).
func styleOf(def Definition, key, measurementID string) (SeriesStyle, bool) {
	if st, ok := def.Series[key]; ok {
		return st, true
	}
	if key != measurementID {
		st, ok := def.Series[measurementID]
		return st, ok
	}
	return SeriesStyle{}, false
}

type distItem struct {
	key, seriesID string
	m             model.Measurement
}

// distributionItems lists what a distribution graph draws: the selected
// curves (schema 2) or the measurements with their pinned candidates
// (schema 1), and the weighting they share.
func distributionItems(def Definition, in Input, w string, res *Result) ([]distItem, string, error) {
	var out []distItem
	if len(def.SeriesIDs) == 0 {
		for _, id := range def.Measurements {
			m, ok := in.Measurements[id]
			if !ok {
				res.Warnings = append(res.Warnings, "graph.missing_measurement:"+id)
				continue
			}
			out = append(out, distItem{key: id, m: selectedDistribution(def, m)})
		}
		return out, w, nil
	}
	weighting := ""
	for _, id := range def.SeriesIDs {
		ref, err := series.Parse(id)
		if err != nil || ref.Kind != series.KindCurve {
			res.Warnings = append(res.Warnings, "graph.series_not_drawable:"+id)
			continue
		}
		m, ok := in.Measurements[ref.MeasurementID]
		if ok {
			m, ok = curveOf(m, ref.Distribution)
		}
		if !ok {
			res.Warnings = append(res.Warnings, "graph.missing_series:"+id)
			continue
		}
		if weighting == "" {
			weighting = ref.Weighting
		} else if ref.Weighting != weighting {
			return nil, w, ErrMixedVariables
		}
		out = append(out, distItem{key: id, seriesID: id, m: m})
	}
	if weighting == "" {
		weighting = w
	}
	return out, weighting, nil
}

// curveOf selects a distribution candidate by its identifier; a plain
// export's single table has none.
func curveOf(m model.Measurement, dist string) (model.Measurement, bool) {
	if dist == series.TableDistribution && len(m.Distributions) == 0 {
		return m, m.Dist != nil
	}
	for i := range m.Distributions {
		if m.Distributions[i].ID == dist {
			m.Dist = &m.Distributions[i]
			return m, true
		}
	}
	return m, false
}

// SelectionMeasurements returns the measurements a selection refers to,
// once each, in selection order.
func SelectionMeasurements(ids []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, id := range ids {
		if ref, err := series.Parse(id); err == nil && !seen[ref.MeasurementID] {
			seen[ref.MeasurementID] = true
			out = append(out, ref.MeasurementID)
		}
	}
	return out
}

// SeriesValues draws the chosen summary values or instrument readings of
// one variable side by side, one category per series. Values are plotted
// exactly as read; nothing is averaged, and readings are never counted as
// replicates.
func SeriesValues(def Definition, in Input) (Result, error) {
	res := Result{Kind: KindSeriesValues, Title: def.Title, Visual: def.Visual,
		Provenance: Provenance{Engine: EngineVersion, Spec: reference.LightScatteringSpecID, Rules: []string{"ls.units", "ls.series"},
			Transformations: []string{"none: values plotted exactly as read from the file; instrument readings are never counted as replicates"}, ComputedAt: time.Now().UTC()}}
	res.X = Axis{Label: "axis.series", Scale: "linear"}
	res.Y = Axis{Scale: "linear"}
	key, unit := "", ""
	mapped := false
	ymin, ymax := math.Inf(1), math.Inf(-1)
	var used []model.Measurement
	sourced := map[string]bool{}
	for _, id := range def.SeriesIDs {
		ref, err := series.Parse(id)
		if err != nil || (ref.Kind != series.KindScalar && ref.Kind != series.KindReading) {
			res.Warnings = append(res.Warnings, "graph.series_not_drawable:"+id)
			continue
		}
		m, ok := in.Measurements[ref.MeasurementID]
		if !ok {
			res.Warnings = append(res.Warnings, "graph.missing_series:"+id)
			continue
		}
		mt := meta(m)
		var v float64
		var u string
		run := ""
		if ref.Kind == series.KindScalar {
			q, ok := m.Params[ref.Param]
			if !ok {
				res.Warnings = append(res.Warnings, "graph.missing_series:"+id)
				continue
			}
			v, u = q.Value, q.Unit
			mapped = mapped || q.UnitOrigin == "user_mapping"
		} else {
			rv, col, row, ok := readingOf(in, m, ref)
			if !ok {
				res.Warnings = append(res.Warnings, "graph.missing_series:"+id)
				continue
			}
			v, u, run = *rv.Value, col.Unit, row.Run
			mt["reading.run"], mt["reading.raw"], mt["reading.line"], mt["reading.header"] = row.Run, rv.Raw, strconv.Itoa(row.Line), col.Header
		}
		if len(res.Series) == 0 {
			key, unit = ref.Param, u
		} else if ref.Param != key {
			return res, ErrMixedVariables
		} else if u != unit {
			return res, ErrMixedUnits
		}
		label := ""
		if st, ok := def.Series[id]; ok && st.Label != "" {
			label = st.Label
		} else {
			label = keyedLabel(def, id, m, len(res.Series))
			if run != "" {
				label = "reading:" + run + ":" + label
			}
		}
		x := float64(len(res.Series) + 1)
		s := Series{ID: id, SeriesID: id, MeasurementID: m.ID, Label: label, Kind: "individual", X: []float64{x}, Y: []float64{v}, Meta: mt}
		if st, ok := def.Series[id]; ok {
			s.Hidden, s.Color, s.Marker, s.LineStyle = st.Hidden, st.Color, st.Marker, st.LineStyle
		}
		ymin, ymax = math.Min(ymin, v), math.Max(ymax, v)
		res.Series = append(res.Series, s)
		used = append(used, m)
		if !sourced[m.ID] {
			sourced[m.ID] = true
			res.Provenance.Sources = append(res.Provenance.Sources, in.Source(m))
		}
	}
	if len(res.Series) == 0 {
		return res, ErrNoData
	}
	distinctLabels(def, res.Series, used)
	for _, s := range res.Series {
		res.X.Categories = append(res.X.Categories, s.Label)
	}
	if mapped {
		res.Warnings = append(res.Warnings, "ls.user_mapping_unvalidated")
	}
	res.Y.Label, res.Y.Unit = "param."+key, unit
	if unit == "" && NeedsUnit(key) {
		res.Warnings = append(res.Warnings, "graph.y_unit_not_stated")
	}
	res.X.Min, res.X.Max = .5, float64(len(res.Series))+.5
	res.Y.Min, res.Y.Max = ymin, ymax
	return res, nil
}

// readingOf finds one numeric cell of a stored runs table.
func readingOf(in Input, m model.Measurement, ref series.Ref) (model.ReadingValue, model.ReadingColumn, model.Reading, bool) {
	fr, ok := in.Readings[m.FileID]
	if !ok {
		return model.ReadingValue{}, model.ReadingColumn{}, model.Reading{}, false
	}
	for _, tb := range fr.Tables {
		if tb.MeasurementID != m.ID {
			continue
		}
		for c, col := range tb.Columns {
			if col.Key != ref.Param {
				continue
			}
			for _, row := range tb.Rows {
				if row.Index == ref.Run && c < len(row.Values) && row.Values[c].Value != nil {
					return row.Values[c], col, row, true
				}
			}
		}
	}
	return model.ReadingValue{}, model.ReadingColumn{}, model.Reading{}, false
}

// MaxTextNotes bounds the person's notes on one graph.
const MaxTextNotes = 20

// Finish applies the schema 2 display options to a computed graph. They
// change how the graph is shown, never the data: hidden or muted series
// stay in the result, and a display unit keeps the stored values next to
// the converted ones.
func Finish(def Definition, res *Result) {
	res.Subtitle = def.Subtitle
	if def.XLabel != "" {
		res.X.Title = def.XLabel
	}
	if def.YLabel != "" {
		res.Y.Title = def.YLabel
	}
	if len(def.FocusSeries) > 0 {
		focus := map[string]bool{}
		for _, id := range def.FocusSeries {
			focus[id] = true
		}
		any := false
		for _, s := range res.Series {
			any = any || focus[s.ID]
		}
		if any {
			for i := range res.Series {
				res.Series[i].Muted = !focus[res.Series[i].ID]
			}
		}
	}
	if def.ShowIndividual != nil && !*def.ShowIndividual && (res.Kind == KindParameterTime || res.Kind == KindStatistical) {
		for i := range res.Series {
			if res.Series[i].Kind == "individual" {
				res.Series[i].Hidden = true
			}
		}
	}
	if def.ConnectMeans != nil && !*def.ConnectMeans {
		for i := range res.Series {
			if res.Series[i].Kind == "mean" {
				res.Series[i].NoLine = true
			}
		}
	}
	if def.DisplayUnit != "" && def.DisplayUnit != res.Y.Unit {
		displayUnit(def.DisplayUnit, res)
	}
	if def.YScale == "log" {
		ok := res.Y.Min > 0
		for _, s := range res.Series {
			for _, y := range s.Y {
				ok = ok && y > 0
			}
		}
		if ok {
			res.Y.Scale = "log"
		} else {
			res.Warnings = append(res.Warnings, "graph.log_needs_positive_values")
		}
	}
	for _, n := range def.TextNotes {
		n.Text = strings.TrimSpace(n.Text)
		if n.Text == "" || math.IsNaN(n.X) || math.IsNaN(n.Y) || math.IsInf(n.X, 0) || math.IsInf(n.Y, 0) || len(res.TextNotes) >= MaxTextNotes {
			continue
		}
		res.TextNotes = append(res.TextNotes, n)
	}
}

// displayUnit shows the Y axis in another unit when an exact factor of the
// same dimension exists (decision D6); otherwise the graph keeps the
// stored unit and says why.
func displayUnit(to string, res *Result) {
	from := res.Y.Unit
	r, err := units.Ratio(from, to)
	if err != nil {
		res.Warnings = append(res.Warnings, "graph.display_unit_not_converted")
		return
	}
	conv := func(v float64) float64 {
		c, err := units.Display(v, "", from, to)
		if err != nil {
			return v
		}
		return c.DisplayValue
	}
	for i := range res.Series {
		s := &res.Series[i]
		s.YOriginal = append([]float64(nil), s.Y...)
		for j := range s.Y {
			s.Y[j] = conv(s.Y[j])
		}
		for j := range s.Err {
			s.Err[j] = conv(s.Err[j])
		}
	}
	for i := range res.Annotations {
		res.Annotations[i].Y = conv(res.Annotations[i].Y)
	}
	res.Y.Min, res.Y.Max = conv(res.Y.Min), conv(res.Y.Max)
	res.Y.Unit = to
	res.Y.Conversion = &AxisConversion{From: from, To: to, Factor: r.RatString()}
	res.Provenance.Transformations = append(res.Provenance.Transformations,
		"display unit "+from+" → "+to+" (exact factor "+r.RatString()+"); stored values and units unchanged")
}
