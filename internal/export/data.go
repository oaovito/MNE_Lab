package export

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
)

// Translator returns localized text for a key ({name} placeholders).
type Translator func(key string, kv ...string) string

// Cell is one table value. Numbers keep the decimals they were written
// with in the source file, so nothing is rounded or padded on export.
type Cell struct {
	Num  *float64 `json:"-"`
	Text string   `json:"-"`
	Dec  int      `json:"-"` // decimals; -1 means shortest exact form
}

// Column is a table column.
type Column struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Unit  string `json:"unit,omitempty"`
}

// Header returns "Label (unit)".
func (c Column) Header() string {
	if c.Unit == "" {
		return c.Label
	}
	return c.Label + " (" + c.Unit + ")"
}

// Table is a named table.
type Table struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
	Rows    [][]Cell `json:"-"`
}

// KV is a metadata entry.
type KV struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
}

// DataSet is everything a data export contains. Tables[0] is the main
// table (the only one in single-table formats such as CSV).
type DataSet struct {
	Title      string         `json:"title"`
	Tables     []Table        `json:"tables"`
	Meta       []KV           `json:"metadata"`
	Provenance []graph.Source `json:"provenance"`
	Rules      []string       `json:"rules,omitempty"`
	Engine     string         `json:"engine,omitempty"`
	Created    time.Time      `json:"created"`
	Notes      []string       `json:"notes,omitempty"`
	t          Translator
}

func (ds DataSet) tr(key, fallback string) string {
	if ds.t == nil {
		return fallback
	}
	if v := ds.t(key); v != "" && v != key {
		return v
	}
	return fallback
}

func num(v float64, dec int) Cell { return Cell{Num: &v, Dec: dec} }
func text(s string) Cell          { return Cell{Text: s} }

// Decimals counts the decimals of a number as written ("12,50" → 2);
// -1 when the text uses an exponent or is not a plain number.
func Decimals(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "eE") {
		return -1
	}
	i := strings.LastIndexAny(raw, ".,")
	if i < 0 {
		return 0
	}
	// "1,234" style thousands separators were resolved by the parser; a
	// group of exactly three digits after the only separator is still
	// decimals here because the parser stored the value it decided on.
	n := 0
	for _, c := range raw[i+1:] {
		if c < '0' || c > '9' {
			return -1
		}
		n++
	}
	return n
}

// FormatValue writes a number with the given decimals and separator.
func FormatValue(v float64, dec int, sep string) string {
	var s string
	switch {
	case math.IsNaN(v) || math.IsInf(v, 0):
		return ""
	case dec >= 0:
		s = strconv.FormatFloat(v, 'f', dec, 64)
	case v == 0 || (math.Abs(v) >= 1e-6 && math.Abs(v) < 1e15):
		s = strconv.FormatFloat(v, 'f', -1, 64)
	default:
		s = strconv.FormatFloat(v, 'g', -1, 64)
	}
	if sep == "," {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

// String renders a cell for text formats.
func (c Cell) String(sep string) string {
	if c.Num != nil {
		return FormatValue(*c.Num, c.Dec, sep)
	}
	return c.Text
}

// FormatTime writes a measurement time; times without a known time zone
// are written as the wall-clock reading, without inventing an offset.
func FormatTime(ts *model.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.ISOTime()
}

var paramKeys = []string{model.EffectiveDiameter, model.Polydispersity, model.CountRate, model.AverageCountRate, model.BaselineIndex}

// commonUnit returns the unit shared by all values of a parameter, or ""
// with mixed=true when they differ (each value then keeps its own unit).
func commonUnit(ms []model.Measurement, key string) (string, bool) {
	unit, seen := "", false
	for _, m := range ms {
		q, ok := m.Params[key]
		if !ok {
			continue
		}
		if !seen {
			unit, seen = q.Unit, true
		} else if q.Unit != unit {
			return "", true
		}
	}
	return unit, false
}

// PointLabels maps measurement identifiers to their cycle point.
type PointLabels map[string]cycle.Point

// MeasurementData builds the data of measurements: a long table with one
// row per distribution bin carrying every parameter and identifier, and a
// summary table with one row per measurement.
func MeasurementData(title string, ids []string, in graph.Input, points PointLabels, T Translator) DataSet {
	ds := DataSet{Title: title, Created: time.Now().UTC(), Engine: graph.EngineVersion, t: T}
	var ms []model.Measurement
	for _, id := range ids {
		if m, ok := in.Measurements[id]; ok {
			ms = append(ms, m)
		}
	}
	// Weightings present in any measurement, in canonical order, with units.
	type wcol struct{ key, unit string }
	var wcols []wcol
	xunit := ""
	for _, k := range []string{"intensity", "volume", "number"} {
		for _, m := range ms {
			if c := m.Dist.Column(k); c != nil {
				wcols = append(wcols, wcol{k, c.Unit})
				break
			}
		}
	}
	for _, m := range ms {
		if c := m.Dist.Column("diameter"); c != nil {
			xunit = c.Unit
			break
		}
	}
	units := map[string]string{}
	mixed := map[string]bool{}
	for _, k := range paramKeys {
		units[k], mixed[k] = commonUnit(ms, k)
	}
	idCols := []Column{
		{Key: "sample_id", Label: T("col.sample_id")},
		{Key: "replicate", Label: T("col.replicate")},
		{Key: "label", Label: T("col.label")},
		{Key: "experiment", Label: T("col.experiment")},
		{Key: "condition", Label: T("col.condition")},
	}
	if len(points) > 0 {
		idCols = append(idCols, Column{Key: "cycle_point", Label: T("col.cycle_point")}, Column{Key: "cycle_offset", Label: T("col.cycle_offset")})
	}
	idCols = append(idCols,
		Column{Key: "measured_at", Label: T("col.measured_at")},
		Column{Key: "source_file", Label: T("col.source_file")},
		Column{Key: "measurement_index", Label: T("col.measurement_index")})
	var paramCols []Column
	for _, k := range paramKeys {
		paramCols = append(paramCols, Column{Key: k, Label: T("param." + k), Unit: units[k]})
		if mixed[k] {
			paramCols = append(paramCols, Column{Key: k + "_unit", Label: T("param."+k) + " · " + T("col.unit")})
		}
	}
	idCells := func(m model.Measurement) []Cell {
		f := in.Files[m.FileID]
		rep := text("")
		if m.Replicate > 0 {
			rep = num(float64(m.Replicate), 0)
		}
		row := []Cell{text(m.SampleID), rep, text(m.Label), text(m.Experiment), text(m.Condition)}
		if len(points) > 0 {
			if p, ok := points[m.ID]; ok {
				row = append(row, text(PointName(p, T)), num(float64(p.Offset), 0))
			} else {
				row = append(row, text(""), text(""))
			}
		}
		return append(row, text(FormatTime(m.MeasuredAt)), text(f.Name), num(float64(m.Index+1), 0))
	}
	paramCells := func(m model.Measurement) []Cell {
		var row []Cell
		for _, k := range paramKeys {
			q, ok := m.Params[k]
			if ok {
				row = append(row, num(q.Value, Decimals(q.Raw)))
			} else {
				row = append(row, text(""))
			}
			if mixed[k] {
				row = append(row, text(q.Unit))
			}
		}
		return row
	}

	long := Table{ID: "data", Name: T("sheet.data")}
	long.Columns = append(long.Columns, idCols...)
	long.Columns = append(long.Columns, Column{Key: "diameter", Label: T("col.diameter"), Unit: xunit})
	for _, w := range wcols {
		long.Columns = append(long.Columns, Column{Key: w.key, Label: T("axis." + w.key), Unit: w.unit})
	}
	long.Columns = append(long.Columns, paramCols...)

	summary := Table{ID: "measurements", Name: T("sheet.measurements")}
	summary.Columns = append(append(append(summary.Columns, idCols...), paramCols...), Column{Key: "bins", Label: T("col.bins")})

	for _, m := range ms {
		ic, pc := idCells(m), paramCells(m)
		bins := 0
		if d := m.Dist.Column("diameter"); d != nil {
			bins = len(d.Values)
			for i, v := range d.Values {
				row := append(append([]Cell{}, ic...), num(v, rawDec(d.Raw, i)))
				for _, w := range wcols {
					c := m.Dist.Column(w.key)
					if c != nil && i < len(c.Values) {
						row = append(row, num(c.Values[i], rawDec(c.Raw, i)))
					} else {
						row = append(row, text(""))
					}
				}
				long.Rows = append(long.Rows, append(row, pc...))
			}
		} else {
			// A measurement without a distribution still exports its parameters.
			row := append(append([]Cell{}, ic...), text(""))
			for range wcols {
				row = append(row, text(""))
			}
			long.Rows = append(long.Rows, append(row, pc...))
		}
		summary.Rows = append(summary.Rows, append(append(append([]Cell{}, ic...), pc...), num(float64(bins), 0)))
		ds.Provenance = append(ds.Provenance, sourceOf(in, m))
	}
	ds.Tables = []Table{long, summary}
	ds.Meta = append(ds.Meta, KV{"measurements", T("meta.measurements"), strconv.Itoa(len(ms))})
	return ds
}

func rawDec(raw []string, i int) int {
	if i < len(raw) {
		return Decimals(raw[i])
	}
	return -1
}

func sourceOf(in graph.Input, m model.Measurement) graph.Source {
	return in.Source(m)
}

// PointName localizes a cycle point ("Week 4").
func PointName(p cycle.Point, T Translator) string {
	return T("point."+string(p.Unit), "n", strconv.Itoa(p.Offset))
}

// ParameterData builds the data of a parameter-over-time graph: every
// individual value with its cycle point, plus a statistics table (n, mean,
// sample SD, min, max per point; mean and SD only when n ≥ 2).
// Accepted assignments must share a unit; values are never converted.
func ParameterData(title, param string, in graph.Input, cy graph.CycleInput, T Translator) (DataSet, error) {
	ds := DataSet{Title: title, Created: time.Now().UTC(), Engine: graph.EngineVersion, t: T,
		Rules: []string{"ls.stats", "ls.cycles", "ls.units"}}
	if param == model.AverageCountRate {
		ds.Rules = append(ds.Rules, "ls.count-rate-mean")
	}
	values := map[string]float64{}
	unit, unitSeen := "", false
	for _, a := range cy.Assignments {
		if a.Point < 0 || a.Point >= len(cy.Points) || (a.Status != "auto" && a.Status != "confirmed") {
			continue
		}
		if m, ok := in.Measurements[a.MeasurementID]; ok {
			if q, ok := m.Params[param]; ok {
				if unitSeen && q.Unit != unit {
					return DataSet{}, graph.ErrMixedUnits
				}
				values[m.ID] = q.Value
				unit, unitSeen = q.Unit, true
			}
		}
	}
	series := cycle.Series(cy.Points, cy.Assignments, values)
	data := Table{ID: "data", Name: T("sheet.data"), Columns: []Column{
		{Key: "cycle_point", Label: T("col.cycle_point")},
		{Key: "cycle_offset", Label: T("col.cycle_offset")},
		{Key: "planned_at", Label: T("col.planned_at")},
		{Key: "sample_id", Label: T("col.sample_id")},
		{Key: "replicate", Label: T("col.replicate")},
		{Key: "measured_at", Label: T("col.measured_at")},
		{Key: param, Label: T("param." + param), Unit: unit},
		{Key: "association", Label: T("col.association")},
		{Key: "source_file", Label: T("col.source_file")},
		{Key: "n", Label: T("col.n")},
		{Key: "mean", Label: T("col.mean"), Unit: unit},
		{Key: "sd", Label: T("col.sd"), Unit: unit},
	}}
	st := Table{ID: "statistics", Name: T("sheet.statistics"), Columns: []Column{
		{Key: "cycle_point", Label: T("col.cycle_point")},
		{Key: "cycle_offset", Label: T("col.cycle_offset")},
		{Key: "planned_at", Label: T("col.planned_at")},
		{Key: "n", Label: T("col.n")},
		{Key: "mean", Label: T("col.mean"), Unit: unit},
		{Key: "sd", Label: T("col.sd"), Unit: unit},
		{Key: "min", Label: T("col.min"), Unit: unit},
		{Key: "max", Label: T("col.max"), Unit: unit},
		{Key: "status", Label: T("col.status")},
	}}
	status := map[string]string{}
	replicate := map[string]int{}
	for _, a := range cy.Assignments {
		status[a.MeasurementID] = a.Status
		replicate[a.MeasurementID] = a.Replicate
	}
	opt := func(p *float64) Cell {
		if p == nil {
			return text("")
		}
		return num(*p, -1)
	}
	used := map[string]bool{}
	for i, pr := range series {
		p := cy.Points[i]
		pointCells := []Cell{text(PointName(p, T)), num(float64(p.Offset), 0), text(p.Target.Format("2006-01-02"))}
		s := pr.Summary
		stRow := append(append([]Cell{}, pointCells...), num(float64(s.N), 0), opt(s.Mean), opt(s.SD), opt(s.Min), opt(s.Max))
		if pr.Missing {
			stRow = append(stRow, text(T("cycle.status.missing")))
		} else {
			stRow = append(stRow, text(""))
		}
		st.Rows = append(st.Rows, stRow)
		for _, id := range pr.Sources {
			m := in.Measurements[id]
			q := m.Params[param]
			rep := text("")
			if r := max(m.Replicate, replicate[id]); r > 0 {
				rep = num(float64(r), 0)
			}
			row := append(append([]Cell{}, pointCells...), text(m.SampleID), rep, text(FormatTime(m.MeasuredAt)),
				num(q.Value, Decimals(q.Raw)), text(T("cycle.status."+status[id])), text(in.Files[m.FileID].Name),
				num(float64(s.N), 0), opt(s.Mean), opt(s.SD))
			data.Rows = append(data.Rows, row)
			if !used[id] {
				used[id] = true
				ds.Provenance = append(ds.Provenance, sourceOf(in, m))
			}
		}
	}
	sort.Slice(ds.Provenance, func(i, j int) bool { return ds.Provenance[i].MeasurementID < ds.Provenance[j].MeasurementID })
	ds.Tables = []Table{data, st}
	c := cy.Config
	ds.Meta = append(ds.Meta,
		KV{"parameter", T("meta.parameter"), T("param." + param)},
		KV{"cycle", T("meta.cycle"), c.Name},
		KV{"cycle_start", T("meta.cycle_start"), c.Start.Format(time.RFC3339)},
		KV{"cycle_interval", T("meta.cycle_interval"), fmt.Sprintf("%d %s", c.Interval, T("unit."+string(c.Unit)))},
		KV{"cycle_duration", T("meta.cycle_duration"), fmt.Sprintf("%d %s", c.Duration, T("unit."+string(c.Unit)))},
		KV{"statistics", T("meta.statistics"), T("plot.meta.stats")},
	)
	return ds, nil
}

// AddCommonMeta appends the application and export metadata.
func (ds *DataSet) AddCommonMeta(T Translator, app string, schema int, extra ...KV) {
	ds.Meta = append([]KV{
		{"title", T("meta.title"), ds.Title},
		{"exported_at", T("meta.exported_at"), ds.Created.Format(time.RFC3339)},
		{"software", T("meta.software"), app},
		{"schema", T("meta.schema"), strconv.Itoa(schema)},
		{"engine", T("meta.engine"), ds.Engine},
	}, append(extra, ds.Meta...)...)
}
