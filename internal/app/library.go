package app

import (
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/oaovito/mne_lab/internal/plot"
	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
)

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

// Errors (stable identifiers).
var (
	ErrGraphKind = errors.New("graph.invalid_kind")
	ErrNoCycle   = errors.New("graph.cycle_required")
)

// ---- Graph Library ----

// SaveGraph creates or updates a reproducible graph definition (files,
// series, parameters, scale and visual configuration — never just an image).
func (p *Profile) SaveGraph(def graph.Definition) (graph.Definition, error) {
	switch def.Kind {
	case graph.KindDistribution:
		if len(def.Measurements) == 0 {
			return def, graph.ErrNoData
		}
	case graph.KindParameterTime, graph.KindDistributionAt:
		if def.CycleID == "" {
			return def, ErrNoCycle
		}
	default:
		return def, ErrGraphKind
	}
	now := time.Now().UTC()
	def.Schema = 1
	def.Title = clean(def.Title, 160)
	def.Notes = clean(def.Notes, 4000)
	def.Tags = cleanTags(def.Tags)
	if def.Visual.LineWidth == 0 {
		def.Visual = graph.DefaultVisual()
	}
	err := p.St.Update(func(t *store.Tx) error {
		if def.ID == "" {
			def.ID, def.Created = secure.NewID(), now
		} else {
			var old graph.Definition
			if _, err := t.Get(CollGraphs, def.ID, &old); err == nil {
				def.Created = old.Created
			}
		}
		def.Updated = now
		_, err := t.Put(CollGraphs, def.ID, def)
		return err
	})
	return def, err
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
	switch def.Kind {
	case graph.KindDistribution:
		in, err := p.Input(def.Measurements)
		if err != nil {
			return graph.Result{}, err
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
	}
	return graph.Result{}, ErrGraphKind
}

// Render lays out a graph for the screen or an export preview.
func (p *Profile) Render(def graph.Definition, spec plot.Spec) (plot.Figure, graph.Result, error) {
	res, err := p.Compute(def)
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
	Tags         []string            `json:"tags,omitempty"`
	Notes        string              `json:"notes,omitempty"`
	Created      time.Time           `json:"created"`
	Updated      time.Time           `json:"updated"`
	Trashed      bool                `json:"trashed,omitempty"`
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
	Files       []string                       `json:"files"`
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
	if len(doc.Config.Params) == 0 {
		doc.Config.Params = []string{model.EffectiveDiameter, model.Polydispersity}
	}
	seen := map[string]bool{}
	var ms []string
	for _, id := range doc.Measurements {
		if !seen[id] {
			seen[id] = true
			ms = append(ms, id)
		}
	}
	doc.Measurements = ms
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
	for _, id := range doc.Measurements {
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
	for _, param := range doc.Config.Params {
		values := map[string]float64{}
		for id, m := range in.Measurements {
			if q, ok := m.Params[param]; ok {
				values[id] = q.Value
			}
		}
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
}

// FileRel lists what uses a file.
type FileRel struct {
	Graphs []string `json:"graphs"`
	Cycles []string `json:"cycles"`
}

// GraphRel lists what a graph uses.
type GraphRel struct {
	Files []string `json:"files"`
	Cycle string   `json:"cycle,omitempty"`
}

// CycleRel lists what a cycle uses and what uses it.
type CycleRel struct {
	Files  []string `json:"files"`
	Graphs []string `json:"graphs"`
}

// Relations computes the relation index (trashed objects included, so the
// trash can warn too).
func (p *Profile) Relations() (Relations, error) {
	r := Relations{Files: map[string]FileRel{}, Graphs: map[string]GraphRel{}, Cycles: map[string]CycleRel{}}
	fileOf := map[string]string{}
	err := p.St.View(func(t *store.Tx) error {
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
		r.Graphs[g.ID] = gr
		for _, f := range gr.Files {
			addFile(f, func(fr *FileRel) { fr.Graphs = append(fr.Graphs, g.ID) })
		}
	}
	return r, nil
}
