package app

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/units"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
	"github.com/oaovito/mne_lab/internal/version"
)

// Measurement-unit registry (units of measure such as nm or kcps; not the
// experimental units of the statistics module).
//
// CollUnitDefs holds custom unit definitions and, under the system unit's
// stable ID, a person's choices about a system unit (favourite, hidden).
// System symbols and dimensions always come from the code. CollUnitReplacements
// keeps the provenance of every Replace Usage Then Delete. Recently used
// units are machine-local (collUnitRecent), so choosing a unit on two
// computers never creates a sync conflict.
const collUnitRecent = "_unit_recent"

var (
	ErrUnitNotFound        = errors.New("units.not_found")
	ErrUnitExists          = errors.New("units.symbol_exists")
	ErrUnitSystem          = errors.New("units.system_unit")
	ErrUnitInUse           = errors.New("units.in_use")
	ErrUnitReplaceBlocked  = errors.New("units.replace_blocked")
	ErrUnitReplacement     = errors.New("units.invalid_replacement")
	ErrUnitLimit           = errors.New("units.limit_reached")
	ErrUnitReplacementSelf = errors.New("units.replacement_is_same")
)

// UnitDefinition is a stored registry record.
type UnitDefinition struct {
	ID         string          `json:"id"`
	Symbol     string          `json:"symbol,omitempty"`
	Name       string          `json:"name,omitempty"`
	Dimension  units.Dimension `json:"dimension,omitempty"`
	System     bool            `json:"system"`
	Archived   bool            `json:"archived,omitempty"`
	ArchivedAt *time.Time      `json:"archivedAt,omitempty"`
	Favorite   bool            `json:"favorite,omitempty"`
	Created    time.Time       `json:"created"`
	Updated    time.Time       `json:"updated"`
}

// UnitView is a registry entry as the interface sees it.
type UnitView struct {
	units.Unit
	Archived   bool       `json:"archived"`
	ArchivedAt *time.Time `json:"archivedAt,omitempty"`
	Favorite   bool       `json:"favorite"`
	LastUsed   *time.Time `json:"lastUsed,omitempty"`
	Created    *time.Time `json:"created,omitempty"`
	// Uses counts the objects that use the symbol (when asked for).
	Uses *int `json:"uses,omitempty"`
}

// UnitUse is one object that uses a unit symbol.
type UnitUse struct {
	Kind   string   `json:"kind"` // measurement, import_recipe, graph, cycle, analysis
	ID     string   `json:"id"`
	Label  string   `json:"label,omitempty"`
	Fields []string `json:"fields,omitempty"`
	// Origin: user_assigned (a column mapping or recipe), source_file (the
	// file states it), saved_snapshot (an immutable saved analysis) or
	// follows_measurements (a graph or cycle holds no unit of its own).
	Origin      string `json:"origin"`
	Replaceable bool   `json:"replaceable"`
}

// UnitUsage is the usage scan of one unit.
type UnitUsage struct {
	Unit        UnitView       `json:"unit"`
	Uses        []UnitUse      `json:"uses"`
	Counts      map[string]int `json:"counts"`
	InUse       bool           `json:"inUse"`
	Replaceable bool           `json:"replaceable"`
}

// UnitDeleteResult answers a delete request. A used custom unit is never
// deleted destructively: the answer then lists the usage and the options.
type UnitDeleteResult struct {
	Deleted bool       `json:"deleted"`
	Usage   *UnitUsage `json:"usage,omitempty"`
	Options []string   `json:"options,omitempty"`
}

// UnitChange is one rewritten user assignment.
type UnitChange struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// UnitReplacement is the provenance of a Replace Usage Then Delete.
type UnitReplacement struct {
	ID          string         `json:"id"`
	At          time.Time      `json:"at"`
	Unit        UnitDefinition `json:"unit"`
	Replacement units.Unit     `json:"replacement"`
	Changes     []UnitChange   `json:"changes"`
	AppVersion  string         `json:"appVersion"`
	// Note: values are not converted; only the symbol of user-assigned
	// units changes. Units stated by source files are never rewritten.
	Rule string `json:"rule"`
}

// UnitQuery filters the registry list.
type UnitQuery struct {
	Search          string
	Dimension       units.Dimension
	IncludeArchived bool
	Usage           bool
}

type unitTx interface {
	List(string) ([]store.Record, error)
	Get(string, string, any) (store.Record, error)
}

// unitRecords loads stored definitions keyed by ID.
func unitRecords(tx unitTx) (map[string]UnitDefinition, error) {
	out := map[string]UnitDefinition{}
	rows, err := tx.List(CollUnitDefs)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		var d UnitDefinition
		if decodeRecord(r, &d) == nil && d.ID != "" {
			out[d.ID] = d
		}
	}
	return out, nil
}

func unitRecent(tx unitTx) map[string]time.Time {
	out := map[string]time.Time{}
	rows, _ := tx.List(collUnitRecent)
	for _, r := range rows {
		var v struct {
			At time.Time `json:"at"`
		}
		if decodeRecord(r, &v) == nil {
			out[r.ID] = v.At
		}
	}
	return out
}

// unitViews merges the system table with stored records. A system
// record contributes only the person's choices; symbol and dimension of a
// system unit always come from the code.
func unitViews(tx unitTx) ([]UnitView, error) {
	recs, err := unitRecords(tx)
	if err != nil {
		return nil, err
	}
	recent := unitRecent(tx)
	view := func(u units.Unit, d UnitDefinition, stored bool) UnitView {
		v := UnitView{Unit: u}
		if stored {
			v.Archived, v.ArchivedAt, v.Favorite = d.Archived, d.ArchivedAt, d.Favorite
			if !u.System {
				c := d.Created
				v.Created = &c
			}
		}
		if at, ok := recent[u.ID]; ok {
			v.LastUsed = &at
		}
		return v
	}
	out := []UnitView{}
	for _, u := range units.System() {
		d, ok := recs[u.ID]
		out = append(out, view(u, d, ok))
	}
	custom := []UnitView{}
	for _, d := range recs {
		if d.System {
			continue
		}
		custom = append(custom, view(units.Unit{ID: d.ID, Symbol: d.Symbol, Name: d.Name, Dimension: d.Dimension}, d, true))
	}
	sort.Slice(custom, func(i, j int) bool {
		a, b := strings.ToLower(custom[i].Symbol), strings.ToLower(custom[j].Symbol)
		if a == b {
			return custom[i].Symbol < custom[j].Symbol
		}
		return a < b
	})
	return append(out, custom...), nil
}

func findUnit(tx unitTx, id string) (UnitView, error) {
	all, err := unitViews(tx)
	if err != nil {
		return UnitView{}, err
	}
	for _, v := range all {
		if v.ID == id {
			return v, nil
		}
	}
	return UnitView{}, ErrUnitNotFound
}

func (p *Profile) unitScope() error {
	if p.closed.Load() || p.app.exiting.Load() {
		return ErrImportScope
	}
	return nil
}

// MeasurementUnits lists the registry. Archived (hidden) units are left out
// unless asked for; they stay valid for the data that uses them.
func (p *Profile) MeasurementUnits(q UnitQuery) ([]UnitView, error) {
	if err := p.unitScope(); err != nil {
		return nil, err
	}
	var out []UnitView
	err := p.St.View(func(tx *store.Tx) error {
		all, err := unitViews(tx)
		if err != nil {
			return err
		}
		var counts map[string]int
		if q.Usage {
			idx, err := unitUsageIndex(tx)
			if err != nil {
				return err
			}
			counts = map[string]int{}
			for sym, uses := range idx {
				counts[sym] = len(uses)
			}
		}
		search := strings.ToLower(strings.TrimSpace(q.Search))
		out = []UnitView{}
		for _, v := range all {
			if v.Archived && !q.IncludeArchived || q.Dimension != "" && v.Dimension != q.Dimension {
				continue
			}
			if search != "" && !strings.Contains(strings.ToLower(v.Symbol), search) && !strings.Contains(strings.ToLower(v.Name), search) {
				continue
			}
			if counts != nil {
				n := counts[v.Symbol]
				v.Uses = &n
			}
			out = append(out, v)
		}
		return nil
	})
	return out, err
}

// CreateMeasurementUnit adds a custom unit. The symbol is kept exactly as
// written and must not repeat any registered spelling (system or custom,
// archived included); a different spelling is a different unit.
func (p *Profile) CreateMeasurementUnit(symbol, name string, dim units.Dimension) (UnitView, error) {
	name = strings.TrimSpace(name)
	switch {
	case !units.ValidSymbol(symbol):
		return UnitView{}, units.ErrSymbol
	case !units.ValidName(name):
		return UnitView{}, units.ErrName
	case !units.ValidDimension(dim):
		return UnitView{}, units.ErrDimension
	}
	if known, ok := units.DimensionOf(symbol); ok && known != dim {
		return UnitView{}, units.ErrIncompatible
	}
	if err := p.unitScope(); err != nil {
		return UnitView{}, err
	}
	now := time.Now().UTC()
	d := UnitDefinition{ID: secure.NewID(), Symbol: symbol, Name: name, Dimension: dim, Created: now, Updated: now}
	done := p.app.begin("measurement-units")
	defer done()
	err := p.St.Update(func(tx *store.Tx) error {
		all, err := unitViews(tx)
		if err != nil {
			return err
		}
		if len(all) >= 500 {
			return ErrUnitLimit
		}
		for _, v := range all {
			if v.Symbol == symbol {
				return ErrUnitExists
			}
		}
		_, err = tx.Put(CollUnitDefs, d.ID, d)
		return err
	})
	if err != nil {
		return UnitView{}, err
	}
	c := d.Created
	return UnitView{Unit: units.Unit{ID: d.ID, Symbol: d.Symbol, Name: d.Name, Dimension: d.Dimension}, Created: &c}, nil
}

// updateUnitState changes a person's choice about a unit. For a system unit
// only the choices are stored, under its stable ID.
func (p *Profile) updateUnitState(id string, fn func(d *UnitDefinition)) (UnitView, error) {
	if err := p.unitScope(); err != nil {
		return UnitView{}, err
	}
	done := p.app.begin("measurement-units")
	defer done()
	var out UnitView
	err := p.St.Update(func(tx *store.Tx) error {
		var d UnitDefinition
		if sys, ok := units.SystemByID(id); ok {
			if _, err := tx.Get(CollUnitDefs, id, &d); err != nil {
				d = UnitDefinition{ID: sys.ID, System: true, Created: time.Now().UTC()}
			}
		} else if _, err := tx.Get(CollUnitDefs, id, &d); err != nil || d.System {
			return ErrUnitNotFound
		}
		fn(&d)
		d.Updated = time.Now().UTC()
		if _, err := tx.Put(CollUnitDefs, id, d); err != nil {
			return err
		}
		v, err := findUnit(tx, id)
		out = v
		return err
	})
	return out, err
}

// ArchiveMeasurementUnit hides a unit from new selections. Data, recipes,
// graphs and analyses that use it keep it and still show it.
func (p *Profile) ArchiveMeasurementUnit(id string, archived bool) (UnitView, error) {
	return p.updateUnitState(id, func(d *UnitDefinition) {
		d.Archived = archived
		if archived {
			now := time.Now().UTC()
			d.ArchivedAt = &now
		} else {
			d.ArchivedAt = nil
		}
	})
}

// FavoriteMeasurementUnit marks or unmarks a unit as a favourite.
func (p *Profile) FavoriteMeasurementUnit(id string, favorite bool) (UnitView, error) {
	return p.updateUnitState(id, func(d *UnitDefinition) { d.Favorite = favorite })
}

// MarkMeasurementUnitUsed records, on this computer only, that a unit was
// chosen, for the "recently used" list of the selector.
func (p *Profile) MarkMeasurementUnitUsed(id string) error {
	if err := p.unitScope(); err != nil {
		return err
	}
	return p.St.Update(func(tx *store.Tx) error {
		if _, err := findUnit(tx, id); err != nil {
			return err
		}
		_, err := tx.Put(collUnitRecent, id, map[string]time.Time{"at": time.Now().UTC()})
		return err
	})
}

// unitUsageIndex scans, in one pass, every object that uses a unit symbol:
// measurements (parameters, unmapped fields, distribution columns),
// import recipes, graphs and cycles (through their measurements) and saved
// analyses (their immutable snapshots).
func unitUsageIndex(tx unitTx) (map[string][]UnitUse, error) {
	idx := map[string][]UnitUse{}
	add := func(sym string, u UnitUse) {
		if sym != "" {
			idx[sym] = append(idx[sym], u)
		}
	}
	files := map[string]string{}
	if rows, err := tx.List(CollFiles); err == nil {
		for _, r := range rows {
			var f model.SourceFile
			if decodeRecord(r, &f) == nil {
				files[f.ID] = f.Name
			}
		}
	} else {
		return nil, err
	}
	// symbols used by each measurement, for graphs and cycles.
	measured := map[string]map[string]bool{}
	rows, err := tx.List(CollMeasurements)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		var m model.Measurement
		if decodeRecord(r, &m) != nil {
			continue
		}
		label := files[m.FileID]
		if s := firstNonEmpty(m.Label, m.SampleID); s != "" {
			label = strings.TrimPrefix(label+" · "+s, " · ")
		}
		user, file := map[string][]string{}, map[string][]string{}
		for k, q := range m.Params {
			if q.Unit == "" {
				continue
			}
			if q.UnitOrigin == "user_mapping" {
				user[q.Unit] = append(user[q.Unit], k)
			} else {
				file[q.Unit] = append(file[q.Unit], k)
			}
		}
		for _, f := range m.Fields {
			if f.Unit != "" && f.Key == "" {
				file[f.Unit] = append(file[f.Unit], f.Label)
			}
		}
		dists := m.Distributions
		if m.Dist != nil {
			dists = append([]model.Distribution{*m.Dist}, dists...)
		}
		for _, d := range dists {
			for _, c := range d.Columns {
				if c.Unit != "" {
					file[c.Unit] = append(file[c.Unit], "distribution."+firstNonEmpty(c.Key, c.Label))
				}
			}
		}
		syms := map[string]bool{}
		for sym, fields := range user {
			add(sym, UnitUse{Kind: "measurement", ID: m.ID, Label: label, Fields: uniqueSorted(fields), Origin: "user_assigned", Replaceable: true})
			syms[sym] = true
		}
		for sym, fields := range file {
			add(sym, UnitUse{Kind: "measurement", ID: m.ID, Label: label, Fields: uniqueSorted(fields), Origin: "source_file"})
			syms[sym] = true
		}
		measured[m.ID] = syms
	}
	if rows, err = tx.List(CollImportProfiles); err != nil {
		return nil, err
	}
	for _, r := range rows {
		var ip ImportProfile
		if decodeRecord(r, &ip) != nil || ip.Selection == nil || ip.Selection.Mapping == nil {
			continue
		}
		fields := map[string][]string{}
		for _, c := range ip.Selection.Mapping.Columns {
			if c.Unit != "" {
				fields[c.Unit] = append(fields[c.Unit], c.Key)
			}
		}
		for sym, f := range fields {
			add(sym, UnitUse{Kind: "import_recipe", ID: ip.ID, Label: ip.Name, Fields: uniqueSorted(f), Origin: "user_assigned", Replaceable: true})
		}
	}
	cycles := map[string][]string{}
	if rows, err = tx.List(CollCycles); err != nil {
		return nil, err
	}
	for _, r := range rows {
		var c CycleDoc
		if decodeRecord(r, &c) != nil {
			continue
		}
		cycles[c.ID] = c.Measurements
		for sym := range symbolsOf(measured, c.Measurements) {
			add(sym, UnitUse{Kind: "cycle", ID: c.ID, Label: c.Config.Name, Origin: "follows_measurements", Replaceable: true})
		}
	}
	analyses := map[string]map[string]bool{}
	if rows, err = tx.List(CollAnalyses); err != nil {
		return nil, err
	}
	for _, r := range rows {
		var a analysis.StatisticalAnalysis
		if decodeRecord(r, &a) != nil {
			continue
		}
		syms := map[string]bool{}
		if a.Snapshot.Unit != "" {
			syms[a.Snapshot.Unit] = true
		}
		for _, o := range a.Snapshot.Observations {
			if o.Quantity != nil && o.Quantity.Unit != "" {
				syms[o.Quantity.Unit] = true
			}
		}
		for _, s := range a.Snapshot.Sources {
			for _, q := range s.Measurement.Params {
				if q.Unit != "" {
					syms[q.Unit] = true
				}
			}
		}
		analyses[a.ID] = syms
		for sym := range syms {
			add(sym, UnitUse{Kind: "analysis", ID: a.ID, Label: a.Snapshot.Definition.Title, Origin: "saved_snapshot"})
		}
	}
	if rows, err = tx.List(CollGraphs); err != nil {
		return nil, err
	}
	for _, r := range rows {
		var g graph.Definition
		if decodeRecord(r, &g) != nil {
			continue
		}
		ids := append([]string{}, g.Measurements...)
		if g.CycleID != "" {
			ids = append(ids, cycles[g.CycleID]...)
		}
		syms := symbolsOf(measured, ids)
		for sym := range analyses[g.AnalysisID] {
			syms[sym] = true
		}
		for sym := range syms {
			add(sym, UnitUse{Kind: "graph", ID: g.ID, Label: g.Title, Origin: "follows_measurements", Replaceable: true})
		}
	}
	return idx, nil
}

func symbolsOf(measured map[string]map[string]bool, ids []string) map[string]bool {
	out := map[string]bool{}
	for _, id := range ids {
		for sym := range measured[id] {
			out[sym] = true
		}
	}
	return out
}

func uniqueSorted(s []string) []string {
	sort.Strings(s)
	out := s[:0]
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func unitUsageTx(tx unitTx, id string) (UnitUsage, error) {
	v, err := findUnit(tx, id)
	if err != nil {
		return UnitUsage{}, err
	}
	idx, err := unitUsageIndex(tx)
	if err != nil {
		return UnitUsage{}, err
	}
	u := UnitUsage{Unit: v, Uses: idx[v.Symbol], Counts: map[string]int{}, Replaceable: true}
	if u.Uses == nil {
		u.Uses = []UnitUse{}
	}
	sort.SliceStable(u.Uses, func(i, j int) bool {
		if u.Uses[i].Kind != u.Uses[j].Kind {
			return u.Uses[i].Kind < u.Uses[j].Kind
		}
		return u.Uses[i].Label < u.Uses[j].Label
	})
	for _, use := range u.Uses {
		u.Counts[use.Kind]++
		u.Replaceable = u.Replaceable && use.Replaceable
	}
	u.InUse = len(u.Uses) > 0
	return u, nil
}

// MeasurementUnitUsage lists every object that uses a unit.
func (p *Profile) MeasurementUnitUsage(id string) (UnitUsage, error) {
	if err := p.unitScope(); err != nil {
		return UnitUsage{}, err
	}
	var out UnitUsage
	err := p.St.View(func(tx *store.Tx) error {
		var err error
		out, err = unitUsageTx(tx, id)
		return err
	})
	return out, err
}

// unitDeleteOptions are offered when a used custom unit cannot be deleted.
var unitDeleteOptions = []string{"archive", "replace_then_delete", "cancel"}

// DeleteMeasurementUnit deletes an unused custom unit. A system unit is
// never deleted (it can be hidden). A used custom unit is not deleted: the
// answer lists its usage with Archive, Replace Usage Then Delete and Cancel.
func (p *Profile) DeleteMeasurementUnit(id string) (UnitDeleteResult, error) {
	if _, ok := units.SystemByID(id); ok {
		return UnitDeleteResult{}, ErrUnitSystem
	}
	if err := p.unitScope(); err != nil {
		return UnitDeleteResult{}, err
	}
	done := p.app.begin("measurement-units")
	defer done()
	var out UnitDeleteResult
	err := p.St.Update(func(tx *store.Tx) error {
		usage, err := unitUsageTx(tx, id)
		if err != nil {
			return err
		}
		if usage.Unit.System {
			return ErrUnitSystem
		}
		if usage.InUse {
			out = UnitDeleteResult{Usage: &usage, Options: unitDeleteOptions}
			return nil
		}
		if err := tx.Delete(CollUnitDefs, id); err != nil {
			return err
		}
		if _, err := tx.GetRecord(collUnitRecent, id); err == nil {
			if err := tx.Delete(collUnitRecent, id); err != nil {
				return err
			}
		}
		out = UnitDeleteResult{Deleted: true}
		return nil
	})
	return out, err
}

// ReplaceMeasurementUnit rewrites every user-assigned use of a custom unit
// (measurement values a column mapping assigned, saved import recipes) to
// another registered unit of the same dimension, then deletes the custom
// unit, in one transaction with a provenance record. Values are not
// converted. Units stated by source files and saved analysis snapshots are
// never rewritten: when any remain, nothing changes and the unit can be
// archived instead.
func (p *Profile) ReplaceMeasurementUnit(id, replacementID string) (UnitReplacement, error) {
	if _, ok := units.SystemByID(id); ok {
		return UnitReplacement{}, ErrUnitSystem
	}
	if id == replacementID {
		return UnitReplacement{}, ErrUnitReplacementSelf
	}
	if err := p.unitScope(); err != nil {
		return UnitReplacement{}, err
	}
	done := p.app.begin("measurement-units")
	defer done()
	var rec UnitReplacement
	err := p.St.Update(func(tx *store.Tx) error {
		var def UnitDefinition
		if _, err := tx.Get(CollUnitDefs, id, &def); err != nil || def.System {
			return ErrUnitNotFound
		}
		target, err := findUnit(tx, replacementID)
		if err != nil {
			return ErrUnitReplacement
		}
		if target.Archived || target.Dimension != def.Dimension || target.Dimension == units.Unclassified {
			return ErrUnitReplacement
		}
		if target.Symbol == def.Symbol {
			return ErrUnitReplacementSelf
		}
		usage, err := unitUsageTx(tx, id)
		if err != nil {
			return err
		}
		if !usage.Replaceable {
			return ErrUnitReplaceBlocked
		}
		from, to := def.Symbol, target.Symbol
		rec = UnitReplacement{ID: secure.NewID(), At: time.Now().UTC(), Unit: def, Replacement: target.Unit, Changes: []UnitChange{}, AppVersion: version.Version,
			Rule: "user-assigned units only; values unchanged; units stated by source files never rewritten"}
		for _, use := range usage.Uses {
			switch use.Kind {
			case "measurement":
				var m model.Measurement
				if _, err := tx.Get(CollMeasurements, use.ID, &m); err != nil {
					return err
				}
				keys := make([]string, 0, len(m.Params))
				for k := range m.Params {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					q := m.Params[k]
					if q.Unit == from && q.UnitOrigin == "user_mapping" {
						q.Unit = to
						m.Params[k] = q
						rec.Changes = append(rec.Changes, UnitChange{Kind: "measurement", ID: m.ID, Field: k, From: from, To: to})
					}
				}
				if _, err := tx.Put(CollMeasurements, m.ID, m); err != nil {
					return err
				}
			case "import_recipe":
				var ip ImportProfile
				if _, err := tx.Get(CollImportProfiles, use.ID, &ip); err != nil {
					return err
				}
				for i, c := range ip.Selection.Mapping.Columns {
					if c.Unit == from {
						ip.Selection.Mapping.Columns[i].Unit = to
						rec.Changes = append(rec.Changes, UnitChange{Kind: "import_recipe", ID: ip.ID, Field: c.Key, From: from, To: to})
					}
				}
				if _, err := tx.Put(CollImportProfiles, ip.ID, ip); err != nil {
					return err
				}
			}
		}
		if _, err := tx.Put(CollUnitReplacements, rec.ID, rec); err != nil {
			return err
		}
		if err := tx.Delete(CollUnitDefs, id); err != nil {
			return err
		}
		if _, err := tx.GetRecord(collUnitRecent, id); err == nil {
			return tx.Delete(collUnitRecent, id)
		}
		return nil
	})
	if err != nil {
		return UnitReplacement{}, err
	}
	return rec, nil
}

// MeasurementUnitReplacements lists the provenance records, newest first.
func (p *Profile) MeasurementUnitReplacements() ([]UnitReplacement, error) {
	if err := p.unitScope(); err != nil {
		return nil, err
	}
	out := []UnitReplacement{}
	err := p.St.View(func(tx *store.Tx) error {
		rows, err := tx.List(CollUnitReplacements)
		if err != nil {
			return err
		}
		for _, r := range rows {
			var rec UnitReplacement
			if decodeRecord(r, &rec) == nil {
				out = append(out, rec)
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out, err
}
