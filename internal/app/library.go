package app

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/plot"
	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/design"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/series"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
)

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

// Errors (stable identifiers).
var (
	ErrGraphKind          = errors.New("graph.invalid_kind")
	ErrNoCycle            = errors.New("graph.cycle_required")
	ErrDistributionChoice = errors.New("ls.invalid_distribution_choice")
	ErrRenderCancelled    = errors.New("graph.render_cancelled")
	ErrGraphTrashed       = errors.New("graph.in_trash")
)

// ---- Graph Library ----

// SaveGraph creates or updates a reproducible graph definition (files,
// series, parameters, scale and visual configuration — never just an image).
func (p *Profile) SaveGraph(def graph.Definition) (graph.Definition, error) {
	if err := normalizeSelection(&def); err != nil {
		return def, err
	}
	switch def.Kind {
	case graph.KindStatistical:
		if def.AnalysisID == "" {
			return def, analysis.ErrDefinition
		}
		if def.ErrorBars == "" {
			def.ErrorBars = "sd"
		}
		if def.ErrorBars != "sd" && def.ErrorBars != "sem" && def.ErrorBars != "ci" {
			return def, analysis.ErrDefinition
		}
	case graph.KindDistribution, graph.KindSeriesValues:
		if len(def.Measurements) == 0 {
			return def, graph.ErrNoData
		}
	case graph.KindParameterTime, graph.KindDistributionAt:
		if def.CycleID == "" {
			return def, ErrNoCycle
		}
	case graph.KindDesignTime, graph.KindPairedCondition:
		if def.DesignID == "" {
			return def, graph.ErrDesignRequired
		}
		if !graph.DesignVariable(def.Param) {
			return def, graph.ErrVariable
		}
		if def.ErrorBars == "" {
			def.ErrorBars = "sd"
		}
		if err := cleanDesignGraph(&def); err != nil {
			return def, err
		}
	default:
		return def, ErrGraphKind
	}
	now := time.Now().UTC()
	def.Schema = graph.DefinitionSchema
	def.Title = clean(def.Title, 160)
	def.Subtitle = clean(def.Subtitle, 200)
	def.XLabel, def.YLabel = clean(def.XLabel, 120), clean(def.YLabel, 120)
	def.DisplayUnit = clean(def.DisplayUnit, 40)
	def.Notes = clean(def.Notes, 4000)
	def.Tags = cleanTags(def.Tags)
	cleanDisplay(&def)
	if def.Visual.LineWidth == 0 {
		def.Visual = graph.DefaultVisual()
	}
	err := p.St.Update(func(t *store.Tx) error {
		if def.Kind == graph.KindStatistical {
			var saved analysis.StatisticalAnalysis
			if _, err := t.Get(CollAnalyses, def.AnalysisID, &saved); err != nil {
				return err
			}
			if len(saved.Results.Groups) > 30 {
				return analysis.ErrDefinition
			}
			if err := graph.ValidateAnnotations(def, saved); err != nil {
				return err
			}
			def.Param = saved.Snapshot.Definition.Variable
			def.CycleID = saved.Snapshot.Definition.CycleID
			def.Measurements = []string{}
			for _, o := range saved.Snapshot.Observations {
				def.Measurements = append(def.Measurements, o.MeasurementID)
			}
		}
		if def.Kind == graph.KindDesignTime || def.Kind == graph.KindPairedCondition {
			if err := checkDesignGraphTx(t, &def); err != nil {
				return err
			}
		}
		// Lineage is written only by the server (Duplicate); a client can
		// neither invent nor drop it.
		def.DerivedFrom = nil
		if def.ID == "" {
			def.ID, def.Created = secure.NewID(), now
		} else {
			var old graph.Definition
			if _, err := t.Get(CollGraphs, def.ID, &old); err == nil {
				def.Created = old.Created
				def.DerivedFrom = old.DerivedFrom
				if def.Distributions == nil && len(def.SeriesIDs) == 0 {
					def.Distributions = old.Distributions
				}
			}
		}
		if def.Kind == graph.KindDistribution && len(def.SeriesIDs) > 0 {
			// The pins follow the selected curves, so readers of schema 1
			// see the same candidates; a missing measurement is left for
			// the graph to report.
			pins := map[string]string{}
			for _, id := range def.SeriesIDs {
				ref, _ := series.Parse(id)
				var m model.Measurement
				if _, err := t.Get(CollMeasurements, ref.MeasurementID, &m); err != nil || len(m.Distributions) == 0 {
					continue
				}
				if _, ok := pins[ref.MeasurementID]; !ok {
					pins[ref.MeasurementID] = ref.Distribution
				}
			}
			def.Distributions = pins
		} else if def.Kind == graph.KindDistribution || def.Kind == graph.KindDistributionAt {
			ids := def.Measurements
			if def.Kind == graph.KindDistributionAt {
				var cy CycleDoc
				if _, err := t.Get(CollCycles, def.CycleID, &cy); err != nil {
					return err
				}
				ids = cy.Measurements
			}
			pins := map[string]string{}
			for _, id := range ids {
				var m model.Measurement
				if _, err := t.Get(CollMeasurements, id, &m); err != nil {
					return err
				}
				if len(m.Distributions) == 0 {
					continue
				}
				choice := def.Distributions[id]
				if choice == "" {
					choice = m.DistributionID
				}
				found := false
				for _, d := range m.Distributions {
					if d.ID == choice {
						found = true
						break
					}
				}
				if !found {
					return ErrDistributionChoice
				}
				pins[id] = choice
			}
			def.Distributions = pins
		}
		snapshotVersions(t, &def)
		def.Updated = now
		_, err := t.Put(CollGraphs, def.ID, def)
		return err
	})
	return def, err
}

// cleanDesignGraph bounds the design fields of a definition.
func cleanDesignGraph(def *graph.Definition) error {
	var conds []string
	for _, c := range def.Conditions {
		if c = clean(c, 80); c != "" && !contains(conds, c) {
			conds = append(conds, c)
		}
	}
	if len(conds) > 50 {
		return analysis.ErrDefinition
	}
	def.Conditions = conds
	def.PairedAt = clean(def.PairedAt, 80)
	var groups []string
	for _, g := range def.Groups {
		if g = clean(g, 80); g != "" && !contains(groups, g) {
			groups = append(groups, g)
		}
	}
	if len(groups) > 20 {
		return analysis.ErrDefinition
	}
	def.Groups = groups
	// Selection fields of other kinds never apply to a design graph.
	def.SeriesIDs, def.Distributions, def.CycleID, def.Weighting = nil, nil, "", ""
	if def.AnalysisID == "" {
		def.Annotations, def.AnnotationStyle = nil, ""
		if def.ErrorBars == "ci" {
			// A confidence interval is drawn only from a saved analysis.
			def.ErrorBars = "sd"
		}
	}
	return nil
}

// designOfGraphTx loads the design revision a graph names.
func designOfGraphTx(t *store.Tx, id string) (design.Design, error) {
	all, err := allDesignsTx(t)
	if err != nil {
		return design.Design{}, err
	}
	hist := design.History(all, id)
	if len(hist) == 0 {
		return design.Design{}, design.ErrNotFound
	}
	return hist[0], nil
}

// checkDesignGraphTx verifies a design graph against its design, groups
// and linked analysis, and derives the measurements it reads (references
// for relations, exports and the source snapshot).
func checkDesignGraphTx(t *store.Tx, def *graph.Definition) error {
	d, err := designOfGraphTx(t, def.DesignID)
	if err != nil {
		return err
	}
	labels := map[string]bool{}
	for _, c := range d.Conditions {
		labels[c.Label] = true
	}
	for _, c := range def.Conditions {
		if !labels[c] {
			return graph.ErrUnknownCondition
		}
	}
	if def.Kind == graph.KindPairedCondition {
		if _, ok := d.Time(graph.PairedConditionAt(*def, d)); !ok {
			return graph.ErrUnknownTime
		}
	} else {
		def.PairedAt = ""
	}
	members, err := groupMembersTx(t, def.Groups)
	if err != nil {
		return err
	}
	def.Measurements = []string{}
	for _, a := range d.Assignments {
		if members != nil && !members[a.MeasurementID] {
			continue
		}
		if len(def.Conditions) > 0 && !contains(def.Conditions, a.Condition) {
			continue
		}
		def.Measurements = append(def.Measurements, a.MeasurementID)
	}
	if def.AnalysisID != "" {
		var saved analysis.StatisticalAnalysis
		if _, err := t.Get(CollAnalyses, def.AnalysisID, &saved); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return errors.New("statistics.not_found")
			}
			return err
		}
		if err := graph.CompatibleAnalysis(*def, saved, d); err != nil {
			return err
		}
		if err := graph.ValidateDesignAnnotations(*def, saved, d); err != nil {
			return err
		}
	}
	return nil
}

// normalizeSelection checks a selection by SeriesID and derives the
// schema 1 fields from it (measurements, weighting, parameter), so older
// readers, exports and relations see the same data.
func normalizeSelection(def *graph.Definition) error {
	if len(def.SeriesIDs) == 0 {
		def.SeriesIDs = nil
		if def.Kind == graph.KindSeriesValues {
			return graph.ErrNoData
		}
		return nil
	}
	if def.Kind != graph.KindDistribution && def.Kind != graph.KindSeriesValues {
		return graph.ErrSeriesKind
	}
	seen := map[string]bool{}
	ids := []string{}
	variable := ""
	for _, id := range def.SeriesIDs {
		ref, err := series.Parse(id)
		if err != nil || len(id) > 400 {
			return graph.ErrInvalidSeries
		}
		switch {
		case def.Kind == graph.KindDistribution && ref.Kind != series.KindCurve,
			def.Kind == graph.KindSeriesValues && ref.Kind == series.KindCurve:
			return graph.ErrSeriesKind
		}
		v := ref.Param
		if ref.Kind == series.KindCurve {
			v = ref.Weighting
		}
		if variable == "" {
			variable = v
		} else if v != variable {
			return graph.ErrMixedVariables
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) > graph.MaxSeries {
		return graph.ErrInvalidSeries
	}
	def.SeriesIDs = ids
	def.Measurements = graph.SelectionMeasurements(ids)
	if def.Kind == graph.KindDistribution {
		def.Weighting = variable
	} else {
		def.Param = variable
	}
	keep := func(list []string) []string {
		var out []string
		for _, id := range list {
			if seen[id] {
				out = append(out, id)
			}
		}
		return out
	}
	def.FocusSeries = keep(def.FocusSeries)
	return nil
}

// cleanDisplay bounds the display options of a definition.
func cleanDisplay(def *graph.Definition) {
	if def.YScale != "log" {
		def.YScale = ""
	}
	if def.ErrorBars != "" && def.ErrorBars != "sd" && def.ErrorBars != "sem" && def.ErrorBars != "ci" {
		def.ErrorBars = ""
	}
	if def.Visual.Margin < 0 || def.Visual.Margin > 40 {
		def.Visual.Margin = 0
	}
	def.Visual.Preset = clean(def.Visual.Preset, 40)
	var notes []graph.TextAnnotation
	for _, n := range def.TextNotes {
		if n.Text = clean(n.Text, 200); n.Text != "" && len(notes) < graph.MaxTextNotes {
			notes = append(notes, n)
		}
	}
	def.TextNotes = notes
	if len(def.FocusSeries) > graph.MaxSeries {
		def.FocusSeries = def.FocusSeries[:graph.MaxSeries]
	}
}

// snapshotVersions records the source files and the parser and engine
// versions in force when the graph is saved (addendum B §12).
func snapshotVersions(t *store.Tx, def *graph.Definition) {
	ids := def.Measurements
	def.CalculationVersion = graph.EngineVersion
	switch def.Kind {
	case graph.KindParameterTime, graph.KindDistributionAt:
		var cy CycleDoc
		if _, err := t.Get(CollCycles, def.CycleID, &cy); err == nil {
			ids = cy.Measurements
		}
	case graph.KindStatistical:
		def.CalculationVersion = graph.EngineVersion + "; " + analysis.CalculationVersion
	}
	files, parsers := map[string]bool{}, map[string]bool{}
	def.SourceFiles, def.ParserVersions = nil, nil
	for _, id := range ids {
		var m model.Measurement
		if _, err := t.Get(CollMeasurements, id, &m); err != nil {
			continue
		}
		if !files[m.FileID] {
			files[m.FileID] = true
			def.SourceFiles = append(def.SourceFiles, m.FileID)
		}
		if v := strings.TrimSpace(m.Parser + " " + m.Spec); v != "" && !parsers[v] {
			parsers[v] = true
			def.ParserVersions = append(def.ParserVersions, v)
		}
	}
	sort.Strings(def.ParserVersions)
}

// DuplicateGraph copies a saved graph into a new, independent definition
// that records where it came from. The original record is only read.
func (p *Profile) DuplicateGraph(id, title string) (graph.Definition, error) {
	var out graph.Definition
	err := p.St.Update(func(t *store.Tx) error {
		rec, err := t.Get(CollGraphs, id, &out)
		if err != nil {
			return err
		}
		if out.Trashed {
			return ErrGraphTrashed
		}
		now := time.Now().UTC()
		parent := graph.Lineage{GraphID: out.ID, Relation: graph.RelationDuplicate, Title: out.Title, DefinitionHash: graph.Hash(rec.Data), At: now}
		if title = clean(title, 160); title != "" {
			out.Title = title
		}
		out.ID, out.Created, out.Updated = secure.NewID(), now, now
		out.DerivedFrom = []graph.Lineage{parent}
		out.Schema = graph.DefinitionSchema
		_, err = t.Put(CollGraphs, out.ID, out)
		return err
	})
	return out, err
}

// Graphs lists saved graphs.
func (p *Profile) Graphs(trash bool) ([]graph.Definition, error) {
	var out []graph.Definition
	err := p.St.View(func(t *store.Tx) error {
		recs, err := t.List(CollGraphs)
		for _, r := range recs {
			var d graph.Definition
			if decodeRecord(r, &d) == nil && d.Trashed == trash {
				out = append(out, d)
			}
		}
		return err
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out, err
}

// Graph loads one definition.
func (p *Profile) Graph(id string) (graph.Definition, error) {
	var d graph.Definition
	err := p.St.View(func(t *store.Tx) error {
		_, err := t.Get(CollGraphs, id, &d)
		return err
	})
	return d, err
}

// TrashGraph moves a graph to or from the trash.
func (p *Profile) TrashGraph(id string, trashed bool) error {
	return p.St.Update(func(t *store.Tx) error {
		var d graph.Definition
		if _, err := t.Get(CollGraphs, id, &d); err != nil {
			return err
		}
		d.Trashed = trashed
		_, err := t.Put(CollGraphs, id, d)
		return err
	})
}

// DeleteGraph removes a trashed graph permanently.
func (p *Profile) DeleteGraph(id string) error {
	return p.St.Update(func(t *store.Tx) error {
		var d graph.Definition
		if _, err := t.Get(CollGraphs, id, &d); err != nil {
			return err
		}
		if !d.Trashed {
			return ErrNotTrashed
		}
		return t.Delete(CollGraphs, id)
	})
}

// Compute recalculates a graph from its definition and the current data.
func (p *Profile) Compute(def graph.Definition) (graph.Result, error) {
	return p.ComputeContext(context.Background(), def)
}

// ComputeContext is Compute that gives up as soon as ctx is cancelled (a
// newer preview replaced this one), so superseded work does not pile up.
func (p *Profile) ComputeContext(ctx context.Context, def graph.Definition) (graph.Result, error) {
	if ctx.Err() != nil {
		return graph.Result{}, ErrRenderCancelled
	}
	res, err := p.compute(ctx, def)
	if err != nil {
		return res, err
	}
	if ctx.Err() != nil {
		return graph.Result{}, ErrRenderCancelled
	}
	graph.Finish(def, &res)
	return res, nil
}

func (p *Profile) compute(ctx context.Context, def graph.Definition) (graph.Result, error) {
	switch def.Kind {
	case graph.KindStatistical:
		saved, err := p.Analysis(def.AnalysisID)
		if err != nil {
			return graph.Result{}, err
		}
		return graph.StatisticalGroups(def, saved)
	case graph.KindDistribution, graph.KindSeriesValues:
		ids := def.Measurements
		if len(def.SeriesIDs) > 0 {
			ids = graph.SelectionMeasurements(def.SeriesIDs)
		}
		in, err := p.inputContext(ctx, ids, def.Kind == graph.KindSeriesValues)
		if err != nil {
			return graph.Result{}, err
		}
		if def.Kind == graph.KindSeriesValues {
			return graph.SeriesValues(def, in)
		}
		return graph.Distribution(def, in)
	case graph.KindParameterTime, graph.KindDistributionAt:
		cy, err := p.Cycle(def.CycleID)
		if err != nil {
			return graph.Result{}, err
		}
		in, ci, err := p.cycleInput(cy)
		if err != nil {
			return graph.Result{}, err
		}
		if def.Kind == graph.KindParameterTime {
			return graph.ParameterTime(def, in, ci)
		}
		return graph.DistributionAtPoints(def, in, ci)
	case graph.KindDesignTime, graph.KindPairedCondition:
		din, ids, err := p.designInput(def)
		if err != nil {
			return graph.Result{}, err
		}
		in, err := p.inputContext(ctx, ids, false)
		if err != nil {
			return graph.Result{}, err
		}
		if def.Kind == graph.KindDesignTime {
			return graph.DesignTime(def, in, din)
		}
		return graph.PairedCondition(def, in, din)
	}
	return graph.Result{}, ErrGraphKind
}

// designInput gathers what a design graph reads: the pinned design
// revision, the current unit names, the members of its groups and its
// linked saved analysis (checked to be of the same design and variable).
func (p *Profile) designInput(def graph.Definition) (graph.DesignInput, []string, error) {
	var din graph.DesignInput
	var ids []string
	err := p.St.View(func(t *store.Tx) error {
		d, err := designOfGraphTx(t, def.DesignID)
		if err != nil {
			return err
		}
		din.Design = d
		if din.UnitLabels, err = unitLabelsTx(t); err != nil {
			return err
		}
		if din.Members, err = groupMembersTx(t, def.Groups); err != nil {
			return err
		}
		for _, a := range d.Assignments {
			if din.Members == nil || din.Members[a.MeasurementID] {
				ids = append(ids, a.MeasurementID)
			}
		}
		return nil
	})
	if err != nil || def.AnalysisID == "" {
		return din, ids, err
	}
	saved, err := p.Analysis(def.AnalysisID)
	if err != nil {
		return din, ids, err
	}
	if err := graph.CompatibleAnalysis(def, saved, din.Design); err != nil {
		return din, ids, err
	}
	din.Analysis = &saved
	return din, ids, nil
}

// Render lays out a graph for the screen or an export preview.
func (p *Profile) Render(def graph.Definition, spec plot.Spec) (plot.Figure, graph.Result, error) {
	return p.RenderContext(context.Background(), def, spec)
}

// RenderContext is Render that stops when ctx is cancelled.
func (p *Profile) RenderContext(ctx context.Context, def graph.Definition, spec plot.Spec) (plot.Figure, graph.Result, error) {
	res, err := p.ComputeContext(ctx, def)
	if err != nil {
		return plot.Figure{}, res, err
	}
	if spec.Decimal == "" {
		spec.Decimal = p.app.decimal()
	}
	return plot.Layout(res, spec, p.app.T), res, nil
}

// ---- Cycle Library ----

// CycleDoc is a saved cycle. It references measurements; it never changes
// them. Overrides hold the person's explicit point decisions.
type CycleDoc struct {
	Schema       int                 `json:"schema"`
	ID           string              `json:"id"`
	Config       cycle.Config        `json:"config"`
	Measurements []string            `json:"measurements"`
	Overrides    map[string]Override `json:"overrides,omitempty"`
	// GroupID names the saved group the cycle's measurements were taken
	// from (a reference kept for provenance; the list above is the cycle's).
	GroupID string    `json:"groupId,omitempty"`
	Tags    []string  `json:"tags,omitempty"`
	Notes   string    `json:"notes,omitempty"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
	Trashed bool      `json:"trashed,omitempty"`
}

// Override is a confirmed assignment (Point -1 excludes the measurement).
type Override struct {
	Point     int       `json:"point"`
	Replicate int       `json:"replicate,omitempty"`
	At        time.Time `json:"at"`
}

// CycleView is a cycle with its computed plan and associations.
type CycleView struct {
	CycleDoc
	Points      []cycle.Point                  `json:"points"`
	Assignments []cycle.Assignment             `json:"assignments"`
	Items       []MeasurementSummary           `json:"items"`
	Series      map[string][]cycle.PointResult `json:"series"`
	// Units holds each parameter's unit shared by every assigned
	// measurement ("" when the files state none); MixedUnits lists the
	// parameters whose units differ, which are not summarized.
	Units      map[string]string `json:"units"`
	MixedUnits []string          `json:"mixedUnits,omitempty"`
	Files      []string          `json:"files"`
}

// uniqueMeasurementIDs preserves input order and never edits the caller's
// slice. Repeating a record identifier does not create another observation.
func uniqueMeasurementIDs(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// SaveCycle validates and stores a cycle.
func (p *Profile) SaveCycle(doc CycleDoc) (CycleDoc, error) {
	doc.Config.Name = clean(doc.Config.Name, 120)
	if err := doc.Config.Validate(); err != nil {
		return doc, err
	}
	now := time.Now().UTC()
	doc.Schema = 1
	doc.Tags = cleanTags(doc.Tags)
	doc.Notes = clean(doc.Notes, 4000)
	doc.GroupID = clean(doc.GroupID, 80)
	if len(doc.Config.Params) == 0 {
		doc.Config.Params = []string{model.EffectiveDiameter, model.Polydispersity}
	}
	doc.Measurements = uniqueMeasurementIDs(doc.Measurements)
	err := p.St.Update(func(t *store.Tx) error {
		if doc.ID == "" {
			doc.ID, doc.Created = secure.NewID(), now
		} else {
			var old CycleDoc
			if _, err := t.Get(CollCycles, doc.ID, &old); err == nil {
				doc.Created = old.Created
			}
		}
		doc.Updated = now
		_, err := t.Put(CollCycles, doc.ID, doc)
		return err
	})
	return doc, err
}

// Cycle loads one cycle.
func (p *Profile) Cycle(id string) (CycleDoc, error) {
	var d CycleDoc
	err := p.St.View(func(t *store.Tx) error {
		_, err := t.Get(CollCycles, id, &d)
		return err
	})
	return d, err
}

// Cycles lists saved cycles.
func (p *Profile) Cycles(trash bool) ([]CycleDoc, error) {
	var out []CycleDoc
	err := p.St.View(func(t *store.Tx) error {
		recs, err := t.List(CollCycles)
		for _, r := range recs {
			var d CycleDoc
			if decodeRecord(r, &d) == nil && d.Trashed == trash {
				out = append(out, d)
			}
		}
		return err
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out, err
}

// TrashCycle moves a cycle to or from the trash.
func (p *Profile) TrashCycle(id string, trashed bool) error {
	return p.St.Update(func(t *store.Tx) error {
		var d CycleDoc
		if _, err := t.Get(CollCycles, id, &d); err != nil {
			return err
		}
		d.Trashed = trashed
		_, err := t.Put(CollCycles, id, d)
		return err
	})
}

// DeleteCycle removes a trashed cycle permanently.
func (p *Profile) DeleteCycle(id string) error {
	return p.St.Update(func(t *store.Tx) error {
		var d CycleDoc
		if _, err := t.Get(CollCycles, id, &d); err != nil {
			return err
		}
		if !d.Trashed {
			return ErrNotTrashed
		}
		return t.Delete(CollCycles, id)
	})
}

// Plan computes points and automatic associations, then applies the
// person's confirmed decisions.
func Plan(doc CycleDoc, in graph.Input) ([]cycle.Point, []cycle.Assignment, error) {
	pts, err := doc.Config.Points()
	if err != nil {
		return nil, nil, err
	}
	var cands []cycle.Candidate
	for _, id := range uniqueMeasurementIDs(doc.Measurements) {
		m, ok := in.Measurements[id]
		if !ok {
			continue
		}
		exp := m.Experiment
		if exp == "" {
			exp = in.Files[m.FileID].Experiment
		}
		cands = append(cands, cycle.Candidate{ID: id, SampleID: m.SampleID, Experiment: exp, MeasuredAt: m.MeasuredAt})
	}
	as := cycle.Associate(doc.Config, pts, cands)
	for i := range as {
		if o, ok := doc.Overrides[as[i].MeasurementID]; ok {
			as[i].Point, as[i].Replicate, as[i].Reason = o.Point, o.Replicate, ""
			if o.Point >= 0 && o.Point < len(pts) {
				as[i].Status = "confirmed"
			} else {
				as[i].Point, as[i].Status = -1, "unassigned"
				as[i].Reason = "cycle.excluded"
			}
		}
	}
	// Replicate numbers within each point, in measurement time order.
	byPoint := map[int][]int{}
	for i, a := range as {
		if a.Point >= 0 && (a.Status == "auto" || a.Status == "confirmed") {
			byPoint[a.Point] = append(byPoint[a.Point], i)
		}
	}
	for _, idx := range byPoint {
		sort.SliceStable(idx, func(x, y int) bool {
			mx, my := in.Measurements[as[idx[x]].MeasurementID].MeasuredAt, in.Measurements[as[idx[y]].MeasurementID].MeasuredAt
			if mx == nil || my == nil {
				return false
			}
			return mx.Time.Before(my.Time)
		})
		for n, i := range idx {
			if as[i].Replicate == 0 {
				as[i].Replicate = n + 1
			}
		}
	}
	return pts, as, nil
}

func (p *Profile) cycleInput(doc CycleDoc) (graph.Input, graph.CycleInput, error) {
	in, err := p.Input(doc.Measurements)
	if err != nil {
		return in, graph.CycleInput{}, err
	}
	pts, as, err := Plan(doc, in)
	return in, graph.CycleInput{Config: doc.Config, Points: pts, Assignments: as}, err
}

// CycleDetail returns a cycle with its plan, associations and the series of
// every tracked parameter.
func (p *Profile) CycleDetail(id string) (CycleView, error) {
	doc, err := p.Cycle(id)
	if err != nil {
		return CycleView{}, err
	}
	return p.cycleView(doc)
}

func (p *Profile) cycleView(doc CycleDoc) (CycleView, error) {
	doc.Measurements = uniqueMeasurementIDs(doc.Measurements)
	in, ci, err := p.cycleInput(doc)
	if err != nil {
		return CycleView{}, err
	}
	v := CycleView{CycleDoc: doc, Points: ci.Points, Assignments: ci.Assignments, Series: map[string][]cycle.PointResult{}}
	files := map[string]bool{}
	for _, id := range doc.Measurements {
		if m, ok := in.Measurements[id]; ok {
			v.Items = append(v.Items, summarize(m))
			if !files[m.FileID] {
				files[m.FileID] = true
				v.Files = append(v.Files, m.FileID)
			}
		}
	}
	v.Units = map[string]string{}
	for _, param := range doc.Config.Params {
		values := map[string]float64{}
		unit, seen, mixed := "", false, false
		for id, m := range in.Measurements {
			if q, ok := m.Params[param]; ok {
				values[id] = q.Value
			}
		}
		for _, a := range ci.Assignments {
			if a.Status != "auto" && a.Status != "confirmed" {
				continue
			}
			q, ok := in.Measurements[a.MeasurementID].Params[param]
			if !ok {
				continue
			}
			if !seen {
				unit, seen = q.Unit, true
			} else if q.Unit != unit {
				mixed = true
			}
		}
		if mixed {
			// Values in different units are never averaged together.
			v.MixedUnits = append(v.MixedUnits, param)
			continue
		}
		v.Units[param] = unit
		v.Series[param] = cycle.Series(ci.Points, ci.Assignments, values)
	}
	return v, nil
}

// PreviewCycle shows the points and associations of an unsaved cycle.
func (p *Profile) PreviewCycle(doc CycleDoc) (CycleView, error) {
	if err := doc.Config.Validate(); err != nil {
		return CycleView{}, err
	}
	return p.cycleView(doc)
}

// Assign records an explicit decision for one measurement (point -1
// excludes it from the cycle without touching the measurement).
func (p *Profile) Assign(cycleID, measurementID string, point, replicate int) (CycleView, error) {
	var doc CycleDoc
	err := p.St.Update(func(t *store.Tx) error {
		if _, err := t.Get(CollCycles, cycleID, &doc); err != nil {
			return err
		}
		if doc.Overrides == nil {
			doc.Overrides = map[string]Override{}
		}
		doc.Overrides[measurementID] = Override{Point: point, Replicate: replicate, At: time.Now().UTC()}
		doc.Updated = time.Now().UTC()
		_, err := t.Put(CollCycles, cycleID, doc)
		return err
	})
	if err != nil {
		return CycleView{}, err
	}
	return p.cycleView(doc)
}

// ClearAssignment returns a measurement to the automatic rule.
func (p *Profile) ClearAssignment(cycleID, measurementID string) (CycleView, error) {
	var doc CycleDoc
	err := p.St.Update(func(t *store.Tx) error {
		if _, err := t.Get(CollCycles, cycleID, &doc); err != nil {
			return err
		}
		delete(doc.Overrides, measurementID)
		doc.Updated = time.Now().UTC()
		_, err := t.Put(CollCycles, cycleID, doc)
		return err
	})
	if err != nil {
		return CycleView{}, err
	}
	return p.cycleView(doc)
}

// ---- relations between objects (File → Graphs, Graph → Files + Cycle,
// Cycle → Files + Graphs), also used to warn before deleting ----

// Relations indexes how library objects reference each other.
type Relations struct {
	Files  map[string]FileRel  `json:"files"`
	Graphs map[string]GraphRel `json:"graphs"`
	Cycles map[string]CycleRel `json:"cycles"`
	Groups map[string]GroupRel `json:"groups"`
}

// FileRel lists what uses a file.
type FileRel struct {
	Graphs   []string `json:"graphs"`
	Cycles   []string `json:"cycles"`
	Analyses []string `json:"analyses"`
	Groups   []string `json:"groups,omitempty"`
}

// GroupRel lists the files a group references and what uses the group.
type GroupRel struct {
	Files  []string `json:"files"`
	Graphs []string `json:"graphs"`
	Cycles []string `json:"cycles"`
}

// GraphRel lists what a graph uses.
type GraphRel struct {
	Files []string `json:"files"`
	Cycle string   `json:"cycle,omitempty"`
	// Lineage: the graphs this one was derived from, and those derived
	// from it.
	DerivedFrom []string `json:"derivedFrom,omitempty"`
	Derived     []string `json:"derived,omitempty"`
	// ChangedParents are parents whose stored definition changed since
	// this graph was derived (changes never propagate on their own).
	ChangedParents []string `json:"changedParents,omitempty"`
	Design         string   `json:"design,omitempty"`
	Groups         []string `json:"groups,omitempty"`
	Analysis       string   `json:"analysis,omitempty"`
}

// CycleRel lists what a cycle uses and what uses it.
type CycleRel struct {
	Files    []string `json:"files"`
	Graphs   []string `json:"graphs"`
	Analyses []string `json:"analyses"`
}

// Relations computes the relation index (trashed objects included, so the
// trash can warn too).
func (p *Profile) Relations() (Relations, error) {
	r := Relations{Files: map[string]FileRel{}, Graphs: map[string]GraphRel{}, Cycles: map[string]CycleRel{}, Groups: map[string]GroupRel{}}
	fileOf := map[string]string{}
	graphHash := map[string]string{}
	err := p.St.View(func(t *store.Tx) error {
		gs, err := t.List(CollGraphs)
		if err != nil {
			return err
		}
		for _, rec := range gs {
			var g struct {
				ID string `json:"id"`
			}
			if jsonUnmarshal(rec.Data, &g) == nil {
				graphHash[g.ID] = graph.Hash(rec.Data)
			}
		}
		rs, err := t.List(CollMeasurements)
		if err != nil {
			return err
		}
		for _, rec := range rs {
			var m struct {
				ID     string `json:"id"`
				FileID string `json:"fileId"`
			}
			if jsonUnmarshal(rec.Data, &m) == nil {
				fileOf[m.ID] = m.FileID
			}
		}
		return nil
	})
	if err != nil {
		return r, err
	}
	filesOf := func(ids []string) []string {
		seen := map[string]bool{}
		out := []string{}
		for _, id := range ids {
			if f := fileOf[id]; f != "" && !seen[f] {
				seen[f] = true
				out = append(out, f)
			}
		}
		return out
	}
	addFile := func(fid string, fn func(*FileRel)) {
		fr := r.Files[fid]
		fn(&fr)
		r.Files[fid] = fr
	}
	cycles, err := p.Cycles(true)
	if err != nil {
		return r, err
	}
	live, _ := p.Cycles(false)
	cycles = append(cycles, live...)
	cycleFiles := map[string][]string{}
	for _, c := range cycles {
		if _, done := r.Cycles[c.ID]; done {
			continue
		}
		fs := filesOf(c.Measurements)
		cycleFiles[c.ID] = fs
		r.Cycles[c.ID] = CycleRel{Files: fs, Graphs: []string{}}
		for _, f := range fs {
			addFile(f, func(fr *FileRel) { fr.Cycles = append(fr.Cycles, c.ID) })
		}
	}
	graphs, err := p.Graphs(true)
	if err != nil {
		return r, err
	}
	liveG, _ := p.Graphs(false)
	graphs = append(graphs, liveG...)
	for _, g := range graphs {
		if _, done := r.Graphs[g.ID]; done {
			continue
		}
		gr := GraphRel{Cycle: g.CycleID}
		if g.CycleID != "" {
			gr.Files = cycleFiles[g.CycleID]
			if cr, ok := r.Cycles[g.CycleID]; ok {
				cr.Graphs = append(cr.Graphs, g.ID)
				r.Cycles[g.CycleID] = cr
			}
		} else {
			gr.Files = filesOf(g.Measurements)
		}
		if gr.Files == nil {
			gr.Files = []string{}
		}
		for _, l := range g.DerivedFrom {
			gr.DerivedFrom = append(gr.DerivedFrom, l.GraphID)
			if h, ok := graphHash[l.GraphID]; ok && l.DefinitionHash != "" && h != l.DefinitionHash && !contains(gr.ChangedParents, l.GraphID) {
				gr.ChangedParents = append(gr.ChangedParents, l.GraphID)
			}
		}
		gr.Design, gr.Groups, gr.Analysis = g.DesignID, g.Groups, g.AnalysisID
		r.Graphs[g.ID] = gr
		for _, f := range gr.Files {
			addFile(f, func(fr *FileRel) { fr.Graphs = append(fr.Graphs, g.ID) })
		}
	}
	groups, err := p.Groups()
	if err != nil {
		return r, err
	}
	for _, gp := range groups {
		gr := GroupRel{Files: filesOf(gp.Measurements), Graphs: []string{}, Cycles: []string{}}
		for _, f := range gr.Files {
			addFile(f, func(fr *FileRel) { fr.Groups = append(fr.Groups, gp.ID) })
		}
		r.Groups[gp.ID] = gr
	}
	for _, g := range graphs {
		for _, id := range g.Groups {
			if gr, ok := r.Groups[id]; ok && !contains(gr.Graphs, g.ID) {
				gr.Graphs = append(gr.Graphs, g.ID)
				r.Groups[id] = gr
			}
		}
	}
	for _, c := range cycles {
		if gr, ok := r.Groups[c.GroupID]; ok && !contains(gr.Cycles, c.ID) {
			gr.Cycles = append(gr.Cycles, c.ID)
			r.Groups[c.GroupID] = gr
		}
	}
	for _, g := range graphs {
		for _, parent := range r.Graphs[g.ID].DerivedFrom {
			if pr, ok := r.Graphs[parent]; ok && !contains(pr.Derived, g.ID) {
				pr.Derived = append(pr.Derived, g.ID)
				r.Graphs[parent] = pr
			}
		}
	}
	// Snapshots retain original file identities after source deletion.
	err = p.St.View(func(tx *store.Tx) error {
		rows, err := tx.List(CollAnalyses)
		if err != nil {
			return err
		}
		for _, row := range rows {
			var a analysis.StatisticalAnalysis
			if err := decodeRecord(row, &a); err != nil {
				return err
			}
			seen := map[string]bool{}
			for _, o := range a.Snapshot.Observations {
				if !seen[o.FileID] {
					seen[o.FileID] = true
					addFile(o.FileID, func(fr *FileRel) { fr.Analyses = append(fr.Analyses, a.ID) })
				}
			}
			if id := a.Snapshot.Definition.CycleID; id != "" {
				cr := r.Cycles[id]
				cr.Analyses = append(cr.Analyses, a.ID)
				r.Cycles[id] = cr
			}
		}
		return nil
	})
	return r, err
}

// ---- Combine ----

// ErrCombineChanged refuses a combination when a parent changed while it
// was being checked.
var ErrCombineChanged = errors.New("graph.combine_conflict")

// combineParents loads and computes the graphs to combine. A parent that
// cannot be drawn gives an empty result, which the check reports.
func (p *Profile) combineParents(ids []string) ([]graph.Definition, []graph.Result, error) {
	var defs []graph.Definition
	var results []graph.Result
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if len(defs) >= graph.MaxCombine {
			return nil, nil, graph.ErrCombine
		}
		def, err := p.Graph(id)
		if err != nil {
			return nil, nil, err
		}
		res, err := p.Compute(def)
		if err != nil {
			res = graph.Result{}
		}
		defs = append(defs, def)
		results = append(results, res)
	}
	return defs, results, nil
}

// CheckCombine tells whether saved graphs can be combined and which exact
// display-unit conversions are available when their units differ.
func (p *Profile) CheckCombine(ids []string, displayUnit string) (graph.CombineCheck, error) {
	defs, results, err := p.combineParents(ids)
	if err != nil {
		return graph.CombineCheck{}, err
	}
	return graph.CheckCombine(defs, results, clean(displayUnit, 40)), nil
}

// CombineGraphs creates a new graph from the definitions and data of saved
// graphs (never from their images). The parents are only read; the new
// definition records one Derived From entry per parent with the hash of
// the parent's stored record.
func (p *Profile) CombineGraphs(ids []string, title, displayUnit string) (graph.Definition, graph.CombineCheck, error) {
	defs, results, err := p.combineParents(ids)
	if err != nil {
		return graph.Definition{}, graph.CombineCheck{}, err
	}
	ck := graph.CheckCombine(defs, results, clean(displayUnit, 40))
	if !ck.Compatible {
		return graph.Definition{}, ck, graph.ErrCombine
	}
	var out graph.Definition
	err = p.St.Update(func(t *store.Tx) error {
		hashes := make([]string, len(defs))
		for i, d := range defs {
			var cur graph.Definition
			rec, err := t.Get(CollGraphs, d.ID, &cur)
			if err != nil {
				return err
			}
			if !cur.Updated.Equal(d.Updated) || cur.Trashed {
				return ErrCombineChanged
			}
			hashes[i] = graph.Hash(rec.Data)
		}
		now := time.Now().UTC()
		c, err := graph.Combine(defs, hashes, clean(title, 160), ck, secure.NewID(), now)
		if err != nil {
			return err
		}
		switch c.Kind {
		case graph.KindDesignTime:
			if err := checkDesignGraphTx(t, &c); err != nil {
				return err
			}
		default:
			if err := normalizeSelection(&c); err != nil {
				return err
			}
		}
		snapshotVersions(t, &c)
		out = c
		_, err = t.Put(CollGraphs, c.ID, c)
		return err
	})
	return out, ck, err
}
