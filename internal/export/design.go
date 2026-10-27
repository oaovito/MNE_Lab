package export

import (
	"sort"
	"time"

	"github.com/oaovito/mne_lab/internal/science/graph"
)

// DesignData is the data set of a design graph (condition × time or the
// paired view): the readings with their experimental unit, condition, time
// point and replicate kind; each unit's value per cell (technical readings
// averaged, never counted as n); and the condition × time summary over
// experimental units. Values are in their stored units, exactly as read.
// errorBars is what the graph's error bars showed (sd, sem or ci); the
// statistics note names it, never a fixed "Mean ± SD".
func DesignData(title string, tab graph.DesignTable, in graph.Input, T Translator, errorBars string) DataSet {
	ds := DataSet{Title: title, Created: time.Now().UTC(), Engine: graph.EngineVersion, t: T, Rules: []string{"ls.design", "ls.stats", "ls.units"}}
	unit := ""
	if len(tab.Units) > 0 {
		unit = tab.Units[0].StoredUnit
	}
	mixed := false
	for _, u := range tab.Units {
		mixed = mixed || u.StoredUnit != unit
	}
	valueUnit := unit
	if mixed {
		valueUnit = ""
	}
	readings := Table{ID: "data", Name: T("sheet.data"), Columns: []Column{
		{Key: "experimental_unit", Label: T("col.experimental_unit")},
		{Key: "unit_id", Label: T("col.unit_id")},
		{Key: "condition", Label: T("col.condition")},
		{Key: "time_level", Label: T("col.time_level")},
		{Key: "replicate_kind", Label: T("col.replicate_kind")},
		{Key: "sample_id", Label: T("col.sample_id")},
		{Key: "measured_at", Label: T("col.measured_at")},
		{Key: tab.Param, Label: T("param." + tab.Param), Unit: valueUnit},
		{Key: "unit", Label: T("col.unit")},
		{Key: "source_file", Label: T("col.source_file")},
		{Key: "measurement_id", Label: T("col.measurement")},
	}}
	used := map[string]bool{}
	for _, r := range tab.Readings {
		m := in.Measurements[r.MeasurementID]
		readings.Rows = append(readings.Rows, []Cell{text(r.Unit), text(r.UnitID), text(r.Condition), text(r.Time), text(T("design.replicate." + r.ReplicateKind)),
			text(m.SampleID), text(FormatTime(m.MeasuredAt)), num(r.Value, Decimals(r.Raw)), text(r.StoredUnit), text(in.Files[m.FileID].Name), text(r.MeasurementID)})
		if !used[m.ID] && m.ID != "" {
			used[m.ID] = true
			ds.Provenance = append(ds.Provenance, sourceOf(in, m))
		}
	}
	units := Table{ID: "units", Name: T("sheet.units"), Columns: []Column{
		{Key: "experimental_unit", Label: T("col.experimental_unit")},
		{Key: "unit_id", Label: T("col.unit_id")},
		{Key: "condition", Label: T("col.condition")},
		{Key: "time_level", Label: T("col.time_level")},
		{Key: "n_readings", Label: T("col.n_readings")},
		{Key: "unit_value", Label: T("col.unit_value"), Unit: valueUnit},
		{Key: "unit", Label: T("col.unit")},
	}}
	for _, u := range tab.Units {
		units.Rows = append(units.Rows, []Cell{text(u.Unit), text(u.UnitID), text(u.Condition), text(u.Time), num(float64(u.Readings), 0), num(u.Value, -1), text(u.StoredUnit)})
	}
	opt := func(p *float64) Cell {
		if p == nil {
			return text("")
		}
		return num(*p, -1)
	}
	st := Table{ID: "statistics", Name: T("sheet.statistics"), Columns: []Column{
		{Key: "condition", Label: T("col.condition")},
		{Key: "time_level", Label: T("col.time_level")},
		{Key: "n", Label: T("col.n_units")},
		{Key: "mean", Label: T("col.mean"), Unit: valueUnit},
		{Key: "sd", Label: T("col.sd"), Unit: valueUnit},
		{Key: "sem", Label: T("col.sem"), Unit: valueUnit},
		{Key: "unit", Label: T("col.unit")},
	}}
	for _, s := range tab.Summary {
		st.Rows = append(st.Rows, []Cell{text(s.Condition), text(s.Time), num(float64(s.N), 0), num(s.Mean, -1), opt(s.SD), opt(s.SEM), text(s.StoredUnit)})
	}
	sort.Slice(ds.Provenance, func(i, j int) bool { return ds.Provenance[i].MeasurementID < ds.Provenance[j].MeasurementID })
	ds.Tables = []Table{readings, units, st}
	ds.Meta = append(ds.Meta,
		KV{"parameter", T("meta.parameter"), T("param." + tab.Param)},
		KV{"statistics", T("meta.statistics"), T(designStatsKey(errorBars))},
		KV{"n", T("meta.n"), T("meta.n_units")},
	)
	return ds
}

// designStatsKey names the error bars a design graph drew (SD by default).
func designStatsKey(kind string) string {
	switch kind {
	case "sem":
		return "plot.meta.design_stats_sem"
	case "ci":
		return "plot.meta.design_stats_ci"
	}
	return "plot.meta.design_stats_sd"
}
