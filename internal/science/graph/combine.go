package graph

// Combine builds a new graph definition from two or more saved ones and
// the data they reference, never from their images (addendum B §15-19,
// A §20-22, §35). The parents are only read: the new definition copies
// their data references and records one Derived From entry per parent
// with the hash of the stored parent, so a later change of a parent can be
// told and is never followed silently (decision D9).
//
// Graphs are combined only when they measure the same variable on a
// compatible X domain, in the same unit. When units differ but convert to
// a common one with an exact factor (decision D6), the conversion is
// offered and applied for display only; otherwise the combination is
// refused, never mixed silently.

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/science/units"
)

// RelationCombine is the lineage relation of a combined graph.
const RelationCombine = "combine"

// MaxCombine bounds how many graphs one combination reads.
const MaxCombine = 8

// Problems that block a combination (stable identifiers).
const (
	CombineCount     = "graph.combine_count"
	CombineKind      = "graph.combine_kind"
	CombineVariable  = "graph.combine_variable"
	CombineDesign    = "graph.combine_design"
	CombineXDomain   = "graph.combine_x_domain"
	CombineUnits     = "graph.combine_units"
	CombineNoData    = "graph.combine_no_data"
	CombineTrashed   = "graph.combine_trashed"
	CombineUnitFixed = "graph.combine_display_unit_invalid"
	// Notes that do not block it.
	CombineErrorBarsNote = "graph.combine_error_bars_first"
	CombineAnalysisNote  = "graph.combine_annotations_dropped"
	CombineConvertedNote = "graph.combine_converted"
)

// ErrCombine refuses an incompatible combination.
var ErrCombine = errors.New("graph.combine_incompatible")

// CombineCheck is the compatibility verdict of a combination.
type CombineCheck struct {
	Compatible bool     `json:"compatible"`
	Kind       string   `json:"kind,omitempty"`
	Variable   string   `json:"variable,omitempty"`
	Units      []string `json:"units"`
	XUnits     []string `json:"xUnits,omitempty"`
	Problems   []string `json:"problems"`
	Notes      []string `json:"notes"`
	// Conversions lists the units every parent converts to with an exact
	// factor, offered when the units differ; DisplayUnit is the one applied.
	Conversions []string `json:"conversions,omitempty"`
	DisplayUnit string   `json:"displayUnit,omitempty"`
}

// combinable kinds: their definitions reference data a merged definition
// can draw (selections of series, or a design with conditions).
func combinable(kind string) bool {
	return kind == KindDesignTime || kind == KindSeriesValues || kind == KindDistribution
}

func variableOf(def Definition) string {
	if def.Kind == KindDistribution {
		if def.Weighting == "" {
			return "intensity"
		}
		return def.Weighting
	}
	return def.Param
}

// storedUnit is the unit a computed graph's values are stored in.
func storedUnit(r Result) string {
	if r.Y.Conversion != nil && !strings.Contains(r.Y.Conversion.From, ",") {
		return r.Y.Conversion.From
	}
	return r.Y.Unit
}

// exactTargets lists the units every one of from converts to exactly.
func exactTargets(from []string) []string {
	var out []string
	for _, u := range units.System() {
		ok := len(from) > 0
		for _, f := range from {
			if _, err := units.Ratio(f, u.Symbol); err != nil {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, u.Symbol)
		}
	}
	return out
}

// CheckCombine decides whether the parents can be combined. results are the
// parents' computed graphs, in the same order; displayUnit is the exact
// conversion the person accepted ("" for none).
func CheckCombine(parents []Definition, results []Result, displayUnit string) CombineCheck {
	ck := CombineCheck{Units: []string{}, Problems: []string{}, Notes: []string{}}
	problem := func(p string) {
		if !contains(ck.Problems, p) {
			ck.Problems = append(ck.Problems, p)
		}
	}
	if len(parents) < 2 || len(parents) > MaxCombine || len(results) != len(parents) {
		problem(CombineCount)
		return ck
	}
	ck.Kind, ck.Variable = parents[0].Kind, variableOf(parents[0])
	for i, p := range parents {
		if p.Trashed {
			problem(CombineTrashed)
		}
		if !combinable(p.Kind) || p.Kind != ck.Kind {
			problem(CombineKind)
		}
		if variableOf(p) != ck.Variable {
			problem(CombineVariable)
		}
		if p.Kind == KindDesignTime && p.DesignID != parents[0].DesignID {
			// Another design (or revision) has its own units, conditions and
			// time points: the X domains are not the same.
			problem(CombineDesign)
		}
		if p.ErrorBars != parents[0].ErrorBars && !contains(ck.Notes, CombineErrorBarsNote) {
			ck.Notes = append(ck.Notes, CombineErrorBarsNote)
		}
		if (p.AnalysisID != "" || len(p.Annotations) > 0) && !contains(ck.Notes, CombineAnalysisNote) {
			ck.Notes = append(ck.Notes, CombineAnalysisNote)
		}
		r := results[i]
		if len(r.Series) == 0 {
			problem(CombineNoData)
			continue
		}
		if u := storedUnit(r); !contains(ck.Units, u) {
			ck.Units = append(ck.Units, u)
		}
		if r.Y.Conversion != nil && strings.Contains(r.Y.Conversion.From, ",") {
			for _, u := range strings.Split(r.Y.Conversion.From, ", ") {
				if !contains(ck.Units, u) {
					ck.Units = append(ck.Units, u)
				}
			}
		}
		if p.Kind == KindDistribution {
			if !contains(ck.XUnits, r.X.Unit) {
				ck.XUnits = append(ck.XUnits, r.X.Unit)
			}
			if r.X.Label != results[0].X.Label {
				problem(CombineXDomain)
			}
		}
	}
	if len(ck.XUnits) > 1 {
		problem(CombineXDomain)
	}
	sort.Strings(ck.Units)
	if len(ck.Units) > 1 {
		// Units differ: an exact display conversion is offered where the
		// combined kind can draw it; distributions never convert.
		if ck.Kind != KindDistribution {
			ck.Conversions = exactTargets(ck.Units)
		}
		switch {
		case displayUnit == "":
			problem(CombineUnits)
		case !contains(ck.Conversions, displayUnit):
			problem(CombineUnitFixed)
		default:
			ck.DisplayUnit = displayUnit
			ck.Notes = append(ck.Notes, CombineConvertedNote)
		}
	} else if displayUnit != "" && len(ck.Units) == 1 {
		if _, err := units.Ratio(ck.Units[0], displayUnit); err != nil || ck.Kind == KindDistribution {
			problem(CombineUnitFixed)
		} else {
			ck.DisplayUnit = displayUnit
		}
	}
	ck.Compatible = len(ck.Problems) == 0
	return ck
}

// Lineage of one parent of a combination.
func combineLineage(p Definition, hash string, at time.Time) Lineage {
	return Lineage{GraphID: p.ID, Relation: RelationCombine, Title: p.Title, DefinitionHash: hash, At: at}
}

// Combine builds the new definition. hashes are the SHA-256 of the stored
// parent records, in the same order. The caller checks compatibility first.
func Combine(parents []Definition, hashes []string, title string, ck CombineCheck, id string, now time.Time) (Definition, error) {
	if !ck.Compatible || len(parents) != len(hashes) || len(parents) < 2 {
		return Definition{}, ErrCombine
	}
	first := parents[0]
	c := Definition{Schema: DefinitionSchema, ID: id, Kind: first.Kind, Param: first.Param, Weighting: first.Weighting, XScale: first.XScale,
		YScale: first.YScale, ErrorBars: first.ErrorBars, ShowIndividual: first.ShowIndividual, ConnectMeans: first.ConnectMeans,
		UnitTrajectories: first.UnitTrajectories, DesignID: first.DesignID, Visual: first.Visual, Created: now, Updated: now}
	c.Visual.View = nil
	c.Visual.LegendSeries = nil
	c.DisplayUnit = first.DisplayUnit
	if ck.DisplayUnit != "" {
		c.DisplayUnit = ck.DisplayUnit
	}
	if title = strings.TrimSpace(title); title != "" {
		c.Title = title
	} else {
		var names []string
		for _, p := range parents {
			names = append(names, firstNonEmpty(p.Title, p.ID))
		}
		c.Title = strings.Join(names, " vs ")
	}
	allConditions, allGroups := false, false
	seenSeries, seenM := map[string]bool{}, map[string]bool{}
	for i, p := range parents {
		c.DerivedFrom = append(c.DerivedFrom, combineLineage(p, hashes[i], now))
		if len(p.Conditions) == 0 {
			allConditions = true
		}
		if len(p.Groups) == 0 {
			allGroups = true
		}
		for _, x := range p.Conditions {
			if !contains(c.Conditions, x) {
				c.Conditions = append(c.Conditions, x)
			}
		}
		for _, x := range p.Groups {
			if !contains(c.Groups, x) {
				c.Groups = append(c.Groups, x)
			}
		}
		for _, x := range p.SeriesIDs {
			if !seenSeries[x] {
				seenSeries[x] = true
				c.SeriesIDs = append(c.SeriesIDs, x)
			}
		}
		for _, x := range p.Measurements {
			if !seenM[x] {
				seenM[x] = true
				c.Measurements = append(c.Measurements, x)
			}
		}
		for k, v := range p.Distributions {
			if c.Distributions == nil {
				c.Distributions = map[string]string{}
			}
			if _, ok := c.Distributions[k]; !ok {
				c.Distributions[k] = v
			}
		}
		for k, v := range p.Series {
			if c.Series == nil {
				c.Series = map[string]SeriesStyle{}
			}
			if _, ok := c.Series[k]; !ok {
				c.Series[k] = v
			}
		}
		for _, t := range p.Tags {
			if !contains(c.Tags, t) {
				c.Tags = append(c.Tags, t)
			}
		}
	}
	if allConditions {
		c.Conditions = nil
	}
	if allGroups {
		c.Groups = nil
	}
	if c.Measurements == nil {
		c.Measurements = []string{}
	}
	return c, nil
}
