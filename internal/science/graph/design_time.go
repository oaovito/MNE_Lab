package graph

// Design graphs: a parameter of a saved experimental design over its time
// points (condition × time), and the paired view of the conditions at one
// time point. The experimental unit is the observation: technical readings
// of a unit in one condition and time are averaged first (decision D4) and
// never add to n, which always counts experimental units. Means and error
// bars describe the units' values; a confidence interval is drawn only from
// a saved statistical analysis (decision D8). Horizontal offsets that keep
// conditions and units apart are display only; values are never changed.

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/design"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/reference"
	"github.com/oaovito/mne_lab/internal/science/units"
)

// Errors of design graphs (stable identifiers).
var (
	ErrDesignRequired   = errors.New("graph.design_required")
	ErrUnknownCondition = errors.New("graph.unknown_condition")
	ErrPairedConditions = errors.New("graph.paired_needs_two_conditions")
	ErrUnknownTime      = errors.New("graph.unknown_time")
	ErrVariable         = errors.New("graph.invalid_variable")
)

// DesignRef identifies the design revision a graph was computed from.
type DesignRef struct {
	ID       string `json:"id"`
	Root     string `json:"root"`
	Revision int    `json:"revision"`
	Name     string `json:"name"`
}

// DesignInput is what a design graph reads besides the measurements.
type DesignInput struct {
	Design design.Design
	// UnitLabels are the current names of the experimental units; the
	// revision's own names are used for units not listed.
	UnitLabels map[string]string
	// Members limits the measurements to those of the graph's groups (nil:
	// every measurement of the design).
	Members map[string]bool
	// Analysis is the saved statistical analysis linked to the graph, when
	// it is compatible; it supplies confidence intervals and annotations.
	Analysis *analysis.StatisticalAnalysis
}

// DesignVariable reports whether a parameter can be drawn by a design graph.
func DesignVariable(param string) bool {
	switch param {
	case model.EffectiveDiameter, model.Polydispersity, model.CountRate, model.AverageCountRate, model.BaselineIndex:
		return true
	}
	return false
}

// designCell is one unit × condition × time cell with its readings.
type designCell struct {
	unit, cond, time string
	values           []float64
	ids              []string
	experimental     int
}

// unitValue is the value of one experimental unit in one cell: the mean of
// its readings (one value when there is a single reading).
func (c *designCell) unitValue() float64 {
	sum := 0.0
	for _, v := range c.values {
		sum += v
	}
	return sum / float64(len(c.values))
}

type designData struct {
	conds     []string
	times     []design.TimeLevel
	units     []string // unit IDs in declaration order, only those with values
	labels    map[string]string
	cells     map[[3]string]*designCell
	unitOf    map[string]string // condition → stored unit of the parameter
	factor    map[string]float64
	converted bool
	used      []string // measurement IDs, sorted
	averaged  int      // cells whose unit value averages several readings
	readings  int
	mapped    bool
	warnings  []string
}

func unitLabel(din DesignInput, id string) string {
	if l := din.UnitLabels[id]; l != "" {
		return l
	}
	if l := din.Design.UnitLabel(id); l != "" {
		return l
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// collectDesign gathers the unit values of a design for one parameter.
func collectDesign(def Definition, in Input, din DesignInput, res *Result) (*designData, error) {
	d := din.Design
	if d.ID == "" {
		return nil, ErrDesignRequired
	}
	if !DesignVariable(def.Param) {
		return nil, ErrVariable
	}
	dd := &designData{labels: map[string]string{}, cells: map[[3]string]*designCell{}, unitOf: map[string]string{}, factor: map[string]float64{}}
	if len(def.Conditions) > 0 {
		for _, c := range def.Conditions {
			found := false
			for _, dc := range d.Conditions {
				found = found || dc.Label == c
			}
			if !found {
				return nil, ErrUnknownCondition
			}
			if !contains(dd.conds, c) {
				dd.conds = append(dd.conds, c)
			}
		}
	}
	allConds := len(dd.conds) == 0
	used := map[string]bool{}
	missingUnit := 0
	for _, a := range d.Assignments {
		if din.Members != nil && !din.Members[a.MeasurementID] {
			continue
		}
		if a.Condition == "" || a.Time == "" || (!allConds && !contains(dd.conds, a.Condition)) {
			continue
		}
		if a.UnitID == "" {
			// A measurement without an experimental unit cannot be counted.
			missingUnit++
			continue
		}
		m, ok := in.Measurements[a.MeasurementID]
		if !ok {
			res.Warnings = append(res.Warnings, "graph.missing_measurement:"+a.MeasurementID)
			continue
		}
		q, ok := m.Params[def.Param]
		if !ok {
			res.Warnings = append(res.Warnings, "graph.design_value_missing:"+firstNonEmpty(m.Label, m.SampleID, a.MeasurementID))
			continue
		}
		if u, seen := dd.unitOf[a.Condition]; seen && u != q.Unit {
			return nil, ErrMixedUnits
		}
		dd.unitOf[a.Condition] = q.Unit
		dd.mapped = dd.mapped || q.UnitOrigin == "user_mapping"
		k := [3]string{a.UnitID, a.Condition, a.Time}
		c := dd.cells[k]
		if c == nil {
			c = &designCell{unit: a.UnitID, cond: a.Condition, time: a.Time}
			dd.cells[k] = c
		}
		c.values = append(c.values, q.Value)
		c.ids = append(c.ids, m.ID)
		if a.ReplicateKind != design.ReplicateTechnical {
			c.experimental++
		}
		used[m.ID] = true
		dd.readings++
	}
	if missingUnit > 0 {
		res.Warnings = append(res.Warnings, "graph.design_without_unit:"+strconv.Itoa(missingUnit))
	}
	if allConds {
		for _, c := range d.Conditions {
			if _, ok := dd.unitOf[c.Label]; ok {
				dd.conds = append(dd.conds, c.Label)
			}
		}
	}
	for _, tl := range d.Times {
		for k := range dd.cells {
			if k[2] == tl.Label && contains(dd.conds, k[1]) {
				dd.times = append(dd.times, tl)
				break
			}
		}
	}
	for _, u := range d.Units {
		for k := range dd.cells {
			if k[0] == u.ID {
				dd.units = append(dd.units, u.ID)
				dd.labels[u.ID] = unitLabel(din, u.ID)
				break
			}
		}
	}
	if len(dd.cells) == 0 || len(dd.conds) == 0 {
		return nil, ErrNoParameter
	}
	for _, c := range dd.cells {
		if len(c.values) > 1 {
			dd.averaged++
		}
		if c.experimental > 1 {
			res.Warnings = append(res.Warnings, "graph.design_several_experimental:"+dd.labels[c.unit]+" · "+c.cond+" · "+c.time)
		}
	}
	sort.Strings(res.Warnings)
	// Conditions may state different units only when every one converts to
	// the display unit with an exact factor (offered by Combine); values
	// are never mixed silently.
	first, same := "", true
	for i, c := range dd.conds {
		if i == 0 {
			first = dd.unitOf[c]
		} else if dd.unitOf[c] != first {
			same = false
		}
	}
	for _, c := range dd.conds {
		dd.factor[c] = 1
	}
	if !same {
		if def.DisplayUnit == "" {
			return nil, ErrMixedUnits
		}
		for _, c := range dd.conds {
			r, err := units.Ratio(dd.unitOf[c], def.DisplayUnit)
			if err != nil {
				return nil, ErrMixedUnits
			}
			f, _ := r.Float64()
			dd.factor[c] = f
		}
		dd.converted = true
	}
	for id := range used {
		dd.used = append(dd.used, id)
	}
	sort.Strings(dd.used)
	return dd, nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// timeAxis places the time points: on a numeric axis at their offsets when
// every point states an offset in the same unit, otherwise as categories.
func timeAxis(times []design.TimeLevel) (Axis, map[string]float64) {
	pos := map[string]float64{}
	numeric := len(times) > 0
	unit := ""
	for i, tl := range times {
		if tl.Offset == nil || tl.Unit == "" || (i > 0 && tl.Unit != unit) {
			numeric = false
		}
		unit = tl.Unit
	}
	if numeric {
		ax := Axis{Label: "axis.time." + unit, Unit: unit, Scale: "linear", Min: math.Inf(1), Max: math.Inf(-1)}
		for _, tl := range times {
			pos[tl.Label] = *tl.Offset
			ax.Ticks = append(ax.Ticks, *tl.Offset)
			ax.Min, ax.Max = math.Min(ax.Min, *tl.Offset), math.Max(ax.Max, *tl.Offset)
		}
		sort.Float64s(ax.Ticks)
		return ax, pos
	}
	ax := Axis{Label: "axis.design_time", Scale: "linear", Min: .5, Max: float64(len(times)) + .5}
	for i, tl := range times {
		pos[tl.Label] = float64(i + 1)
		ax.Categories = append(ax.Categories, tl.Label)
	}
	return ax, pos
}

// errorKind resolves the requested error bars: SD by default, SEM on
// request, a confidence interval only with a linked saved analysis.
func errorKind(def Definition, din DesignInput, res *Result) string {
	switch def.ErrorBars {
	case "sem":
		return "sem"
	case "ci":
		if din.Analysis != nil {
			return "ci"
		}
		res.Warnings = append(res.Warnings, "graph.ci_needs_saved_analysis")
	}
	return "sd"
}

// analysisCI returns the half width of the saved analysis' confidence
// interval of a condition × time group.
func analysisCI(a *analysis.StatisticalAnalysis, cond, tm string) (float64, bool) {
	if a == nil {
		return 0, false
	}
	for _, g := range a.Results.Groups {
		if g.FactorA == cond && g.FactorB == tm && g.CIUpper != nil {
			return *g.CIUpper - g.Mean, true
		}
	}
	return 0, false
}

// cellSummary describes the unit values of one condition × time.
type cellSummary struct {
	values []float64
	units  []string
	mean   float64
	bar    float64
	n      int
}

func summarize(dd *designData, cond, tm, bars string, din DesignInput) (cellSummary, bool) {
	var s cellSummary
	f := dd.factor[cond]
	for _, u := range dd.units {
		if c := dd.cells[[3]string{u, cond, tm}]; c != nil {
			s.values = append(s.values, c.unitValue()*f)
			s.units = append(s.units, u)
		}
	}
	s.n = len(s.values)
	if s.n == 0 {
		return s, false
	}
	for _, v := range s.values {
		s.mean += v
	}
	s.mean /= float64(s.n)
	if s.n >= 2 {
		ss := 0.0
		for _, v := range s.values {
			ss += (v - s.mean) * (v - s.mean)
		}
		sd := math.Sqrt(ss / float64(s.n-1))
		switch bars {
		case "sem":
			s.bar = sd / math.Sqrt(float64(s.n))
		case "ci":
			if h, ok := analysisCI(din.Analysis, cond, tm); ok {
				s.bar = h * f
			}
		default:
			s.bar = sd
		}
	}
	return s, true
}

// designProvenance starts the result of a design graph.
func designResult(kind string, def Definition, din DesignInput, dd *designData, bars string) Result {
	d := din.Design
	res := Result{Kind: kind, Title: def.Title, Visual: def.Visual, ErrorBars: bars,
		Design:     &DesignRef{ID: d.ID, Root: d.Root, Revision: d.Revision, Name: d.Name},
		Provenance: Provenance{Engine: EngineVersion, Spec: reference.LightScatteringSpecID, Rules: []string{"ls.design", "ls.stats", "ls.units"}, ComputedAt: time.Now().UTC()}}
	switch bars {
	case "sem":
		res.Provenance.Statistics = "mean and standard error of the mean (sample SD (n−1) / √n) of experimental unit values; n = experimental units"
	case "ci":
		res.Provenance.Statistics = "mean of experimental unit values; confidence interval from saved analysis " + din.Analysis.ID
	default:
		res.Provenance.Statistics = "mean and sample standard deviation (n−1) of experimental unit values; n = experimental units, n ≥ 2"
	}
	res.Provenance.Transformations = append(res.Provenance.Transformations,
		fmt.Sprintf("experimental design %s revision %d (%s, %s)", d.ID, d.Revision, d.Name, design.Version),
		fmt.Sprintf("technical readings averaged within each experimental unit × condition × time: %d of %d cells (%d readings); readings never count as n", dd.averaged, len(dd.cells), dd.readings))
	var names []string
	for _, u := range dd.units {
		names = append(names, dd.labels[u]+"="+u)
	}
	res.Provenance.Transformations = append(res.Provenance.Transformations, "experimental units: "+strings.Join(names, ", "))
	return res
}

func finishDesign(res *Result, def Definition, in Input, dd *designData) {
	if dd.mapped {
		res.Warnings = append(res.Warnings, "ls.user_mapping_unvalidated")
	}
	for _, id := range dd.used {
		res.Provenance.Sources = append(res.Provenance.Sources, in.Source(in.Measurements[id]))
	}
	res.Y.Label, res.Y.Scale = "param."+def.Param, "linear"
	if dd.converted {
		var from, factors []string
		for _, c := range dd.conds {
			from = append(from, dd.unitOf[c])
			r, _ := units.Ratio(dd.unitOf[c], def.DisplayUnit)
			factors = append(factors, c+": "+r.RatString())
			res.Provenance.Transformations = append(res.Provenance.Transformations,
				"display unit "+dd.unitOf[c]+" → "+def.DisplayUnit+" for "+c+" (exact factor "+r.RatString()+"); stored values and units unchanged")
		}
		res.Y.Unit = def.DisplayUnit
		res.Y.Conversion = &AxisConversion{From: strings.Join(uniqueStrings(from), ", "), To: def.DisplayUnit, Factor: strings.Join(factors, "; ")}
	} else {
		res.Y.Unit = dd.unitOf[dd.conds[0]]
	}
	if res.Y.Unit == "" && NeedsUnit(def.Param) {
		res.Warnings = append(res.Warnings, "graph.y_unit_not_stated")
	}
}

func uniqueStrings(s []string) []string {
	var out []string
	for _, v := range s {
		if !contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func styled(def Definition, s *Series) {
	if st, ok := def.Series[s.ID]; ok {
		s.Hidden, s.Color, s.Marker, s.LineStyle = st.Hidden, st.Color, st.Marker, st.LineStyle
		if st.Label != "" {
			s.Label = st.Label
		}
	}
}

// DesignTime draws a parameter of a design over its time points: for each
// condition the mean of the experimental units with its error bar, the
// units' own values (shown by default) and, on request, each unit's thin
// trajectory over time.
func DesignTime(def Definition, in Input, din DesignInput) (Result, error) {
	var pre Result
	dd, err := collectDesign(def, in, din, &pre)
	if err != nil {
		return pre, err
	}
	bars := errorKind(def, din, &pre)
	res := designResult(KindDesignTime, def, din, dd, bars)
	res.Warnings = append(res.Warnings, pre.Warnings...)
	ax, pos := timeAxis(dd.times)
	res.X = ax
	span := ax.Max - ax.Min
	if len(ax.Categories) > 0 || span <= 0 {
		span = math.Max(span, float64(len(dd.times)))
	}
	// Display-only offsets: conditions side by side around each time point,
	// units side by side within a condition.
	step := span * 0.022
	if len(ax.Categories) > 0 {
		step = 0.1
	}
	k := float64(len(dd.conds))
	ymin, ymax := math.Inf(1), math.Inf(-1)
	nmin, nmax := math.MaxInt, 0
	for ci, cond := range dd.conds {
		off := (float64(ci) - (k-1)/2) * step * 2
		mean := Series{ID: "mean:" + cond, Label: "design:mean_" + bars + ":" + cond, Kind: "mean", Meta: map[string]string{"condition": cond}}
		ind := Series{ID: "units:" + cond, Label: "design:units:" + cond, Kind: "individual", Meta: map[string]string{"condition": cond}}
		traj := map[string]*Series{}
		for _, u := range dd.units {
			traj[u] = &Series{ID: "trajectory:" + cond + ":" + u, Label: "design:trajectory:" + cond, Kind: "trajectory", Meta: map[string]string{"condition": cond, "unit": dd.labels[u], "unitId": u}, PointLabels: []string{dd.labels[u]}}
		}
		for _, tl := range dd.times {
			x := pos[tl.Label] + off
			s, ok := summarize(dd, cond, tl.Label, bars, din)
			if !ok {
				mean.Gaps = append(mean.Gaps, x)
				for _, t := range traj {
					t.Gaps = append(t.Gaps, x)
				}
				res.Warnings = append(res.Warnings, "graph.design_no_units:"+cond+" · "+tl.Label)
				continue
			}
			nmin, nmax = min(nmin, s.n), max(nmax, s.n)
			n := float64(len(s.values))
			for i, v := range s.values {
				xj := x + (float64(i)-(n-1)/2)*step*0.32
				ind.X, ind.Y = append(ind.X, xj), append(ind.Y, v)
				ind.PointLabels = append(ind.PointLabels, dd.labels[s.units[i]])
				t := traj[s.units[i]]
				t.X, t.Y = append(t.X, xj), append(t.Y, v)
				ymin, ymax = math.Min(ymin, v), math.Max(ymax, v)
			}
			for _, u := range dd.units {
				if !contains(s.units, u) {
					traj[u].Gaps = append(traj[u].Gaps, x)
				}
			}
			mean.X, mean.Y, mean.N = append(mean.X, x), append(mean.Y, s.mean), append(mean.N, s.n)
			mean.Err = append(mean.Err, s.bar)
			ymin, ymax = math.Min(ymin, s.mean-s.bar), math.Max(ymax, s.mean+s.bar)
		}
		if bars == "ci" && din.Analysis != nil {
			mean.Label += " · " + number(100*(1-din.Analysis.Snapshot.Definition.Alpha)) + "%"
		}
		if dd.converted {
			mean.Meta["unit.original"] = dd.unitOf[cond]
			ind.Meta["unit.original"] = dd.unitOf[cond]
		}
		styled(def, &mean)
		styled(def, &ind)
		res.Series = append(res.Series, mean, ind)
		if def.UnitTrajectories {
			for _, u := range dd.units {
				if t := traj[u]; len(t.X) > 1 {
					styled(def, t)
					res.Series = append(res.Series, *t)
				}
			}
		}
	}
	if nmax > 0 && nmax < 2 {
		res.Warnings = append(res.Warnings, "graph.not_enough_replicates_for_statistics")
	}
	res.Provenance.Transformations = append(res.Provenance.Transformations,
		fmt.Sprintf("n per condition and time = experimental units with a value: %d to %d", nmin, nmax),
		"display-only horizontal offsets separate conditions and units at each time point; values unchanged")
	if len(ax.Categories) == 0 {
		pad := math.Max(span*0.06, step*4)
		res.X.Min, res.X.Max = ax.Min-pad, ax.Max+pad
	}
	res.Y.Min, res.Y.Max = ymin, ymax
	finishDesign(&res, def, in, dd)
	xOf := func(cond, tm string) (float64, bool) {
		ci, p, ok := indexOf(dd.conds, cond), 0.0, false
		if p, ok = pos[tm]; ci < 0 || !ok || !dd.has(cond, tm) {
			return 0, false
		}
		return p + (float64(ci)-(k-1)/2)*step*2, true
	}
	if err := designAnnotations(def, din, xOf, &res); err != nil {
		return res, err
	}
	return res, nil
}

func indexOf(list []string, v string) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return -1
}

// has reports whether some unit has a value for a condition and time.
func (dd *designData) has(cond, tm string) bool {
	for _, u := range dd.units {
		if dd.cells[[3]string{u, cond, tm}] != nil {
			return true
		}
	}
	return false
}

// PairedConditionAt returns the time point a paired view shows: the one
// chosen, or the first time point of the design.
func PairedConditionAt(def Definition, d design.Design) string {
	if def.PairedAt != "" {
		return def.PairedAt
	}
	if len(d.Times) > 0 {
		return d.Times[0].Label
	}
	return ""
}

// PairedCondition draws, at one time point, each experimental unit's
// values under the conditions joined by a line (F-SR A ↔ C-SR A), beside the
// mean and error bar of each condition.
func PairedCondition(def Definition, in Input, din DesignInput) (Result, error) {
	var pre Result
	at := PairedConditionAt(def, din.Design)
	if _, ok := din.Design.Time(at); !ok {
		return pre, ErrUnknownTime
	}
	dd, err := collectDesign(def, in, din, &pre)
	if err != nil {
		return pre, err
	}
	if len(dd.conds) < 2 {
		return pre, ErrPairedConditions
	}
	bars := errorKind(def, din, &pre)
	res := designResult(KindPairedCondition, def, din, dd, bars)
	res.Warnings = append(res.Warnings, pre.Warnings...)
	res.X = Axis{Label: "axis.condition", Scale: "linear", Min: .5, Max: float64(len(dd.conds)) + .5, Categories: append([]string{}, dd.conds...)}
	ymin, ymax := math.Inf(1), math.Inf(-1)
	mean := Series{ID: "mean", Label: "design:mean_" + bars + ":", Kind: "mean", NoLine: true, Meta: map[string]string{"time": at}}
	pairs := map[string]bool{}
	for _, p := range din.Design.Pairs {
		pairs[p.UnitID+"\x00"+p.From+"\x00"+p.To] = true
		pairs[p.UnitID+"\x00"+p.To+"\x00"+p.From] = true
	}
	present := false
	for i, cond := range dd.conds {
		s, ok := summarize(dd, cond, at, bars, din)
		if !ok {
			res.Warnings = append(res.Warnings, "graph.design_no_units:"+cond+" · "+at)
			continue
		}
		present = true
		x := float64(i+1) + 0.2
		mean.X, mean.Y, mean.N, mean.Err = append(mean.X, x), append(mean.Y, s.mean), append(mean.N, s.n), append(mean.Err, s.bar)
		ymin, ymax = math.Min(ymin, s.mean-s.bar), math.Max(ymax, s.mean+s.bar)
	}
	if !present {
		return res, ErrNoParameter
	}
	if bars == "ci" && din.Analysis != nil {
		mean.Label += " · " + number(100*(1-din.Analysis.Snapshot.Definition.Alpha)) + "%"
	}
	styled(def, &mean)
	res.Series = append(res.Series, mean)
	for _, u := range dd.units {
		se := Series{ID: "pair:" + u, Label: dd.labels[u], Kind: "paired", Meta: map[string]string{"unit": dd.labels[u], "unitId": u, "time": at}}
		var conds []string
		for i, cond := range dd.conds {
			c := dd.cells[[3]string{u, cond, at}]
			if c == nil {
				se.Gaps = append(se.Gaps, float64(i+1))
				continue
			}
			v := c.unitValue() * dd.factor[cond]
			se.X, se.Y = append(se.X, float64(i+1)), append(se.Y, v)
			se.PointLabels = append(se.PointLabels, dd.labels[u])
			conds = append(conds, cond)
			ymin, ymax = math.Min(ymin, v), math.Max(ymax, v)
		}
		if len(se.X) == 0 {
			continue
		}
		declared := len(conds) > 1
		for i := 1; i < len(conds); i++ {
			declared = declared && pairs[u+"\x00"+conds[i-1]+"\x00"+conds[i]]
		}
		if declared {
			se.Meta["pairing"] = "declared"
		} else if len(conds) > 1 {
			se.Meta["pairing"] = "same_unit"
			res.Warnings = append(res.Warnings, "graph.pair_not_declared:"+dd.labels[u])
		}
		styled(def, &se)
		res.Series = append(res.Series, se)
	}
	res.Provenance.Transformations = append(res.Provenance.Transformations,
		"paired view at "+at+": lines join the values of the same experimental unit; the mean is drawn beside its condition (display-only offset)")
	res.Y.Min, res.Y.Max = ymin, ymax
	finishDesign(&res, def, in, dd)
	xOf := func(cond, tm string) (float64, bool) {
		ci := indexOf(dd.conds, cond)
		if ci < 0 || tm != at || !dd.has(cond, tm) {
			return 0, false
		}
		return float64(ci + 1), true
	}
	if err := designAnnotations(def, din, xOf, &res); err != nil {
		return res, err
	}
	return res, nil
}
