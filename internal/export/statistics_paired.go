package export

import (
	"fmt"
	"strings"

	"github.com/oaovito/mne_lab/internal/science/analysis"
)

// unitName is the name a unit had when the analysis was taken, with its
// identifier when no name was recorded.
func unitName(a analysis.StatisticalAnalysis, id string) string {
	if l := a.Snapshot.UnitLabels[id]; l != "" {
		return l
	}
	if d := a.Snapshot.ExperimentalDesign; d != nil {
		if l := d.UnitLabel(id); l != "" {
			return l
		}
	}
	return id
}

// designMeta states where the structure came from: the layout, the saved
// design revision, the experimental units by name and the factor levels.
func designMeta(a analysis.StatisticalAnalysis) []KV {
	def, layout := a.Snapshot.Definition, a.Snapshot.Design
	out := []KV{}
	if def.Design == analysis.DesignWithinWithin {
		out = append(out, KV{Key: "layout", Label: "Design layout", Value: "within_within: condition (A) and time (B) both within each experimental unit"})
	}
	if d := a.Snapshot.ExperimentalDesign; d != nil {
		out = append(out, KV{Key: "designId", Label: "Experimental design revision ID", Value: d.ID}, KV{Key: "designName", Label: "Experimental design", Value: d.Name}, KV{Key: "designRevision", Label: "Design revision", Value: fmt.Sprint(d.Revision)})
	}
	if def.Design == analysis.DesignWithinWithin || a.Snapshot.ExperimentalDesign != nil {
		names := []string{}
		seen := map[string]bool{}
		for _, o := range a.Snapshot.Observations {
			if o.UnitID != "" && !seen[o.UnitID] && o.ExcludeReason == "" && !o.Missing {
				seen[o.UnitID] = true
				names = append(names, unitName(a, o.UnitID))
			}
		}
		out = append(out, KV{Key: "experimentalUnits", Label: "Independent experimental units (N)", Value: fmt.Sprint(layout.ExperimentalUnits)}, KV{Key: "unitNames", Label: "Experimental units", Value: strings.Join(names, "; ")},
			KV{Key: "factorA", Label: "Factor A (" + def.FactorAName + ") levels", Value: strings.Join(layout.LevelsA, "; ")}, KV{Key: "factorB", Label: "Factor B (" + def.FactorBName + ") levels", Value: strings.Join(layout.LevelsB, "; ")})
	}
	if def.Design == analysis.DesignWithinWithin {
		out = append(out, KV{Key: "technicalReadingCells", Label: "Unit cells with several technical readings (averaged, never counted as N)", Value: fmt.Sprint(layout.TechnicalReadingCells)})
		if def.Adjustment != "" {
			out = append(out, KV{Key: "adjustment", Label: "Simple-effects multiplicity correction", Value: def.Adjustment})
		}
	}
	return out
}

// designTable lists the design revision the analysis used: one row per
// unit × condition × time cell, with the unit name and its readings.
func designTable(a analysis.StatisticalAnalysis) Table {
	t := statisticalTable("design", "Experimental design", "field", "value")
	for _, kv := range designMeta(a) {
		t.Rows = append(t.Rows, []Cell{text(kv.Key), text(kv.Value)})
	}
	if d := a.Snapshot.ExperimentalDesign; d != nil {
		for _, c := range d.Conditions {
			v := c.Label
			if c.DerivedFrom != "" {
				v += " derived from " + c.DerivedFrom
				if c.Process != "" {
					v += " (" + c.Process + ")"
				}
			}
			t.Rows = append(t.Rows, []Cell{text("condition"), text(v)})
		}
		for _, tl := range d.Times {
			v := tl.Label
			if tl.Offset != nil {
				v += fmt.Sprintf(" (%g %s)", *tl.Offset, tl.Unit)
			}
			t.Rows = append(t.Rows, []Cell{text("time"), text(v)})
		}
		for _, p := range d.Pairs {
			t.Rows = append(t.Rows, []Cell{text("pair"), text(unitName(a, p.UnitID) + ": " + p.From + " ↔ " + p.To)})
		}
	}
	return t
}

// pairedTables replaces the classical tables of the paired Condition × Time
// design: every term with the denominator df R returned, its error stratum
// and sphericity epsilon; the repeated-measures ANOVA of time within each
// condition; paired simple effects with their unadjusted intervals; and the
// observation of each experimental unit in each cell.
func pairedTables(a analysis.StatisticalAnalysis, u string) []Table {
	r := a.Results
	classical := r.Model == nil
	terms := func(id, name string, list []analysis.Term, context string) Table {
		t := statisticalTable(id, name, "context", "source", "SS", "df", "denominator_df", "MS", "F", "p", "epsilon", "eta_squared", "partial_eta_squared", "generalized_eta_squared")
		for _, x := range list {
			t.Rows = append(t.Rows, []Cell{text(context), text(x.Source), optional(x.SS), num(x.DF, -1), optional(x.DenominatorDF), optional(x.MS), optional(x.F), optional(x.P), optional(x.Epsilon), optional(x.EtaSquared), optional(x.PartialEtaSquared), optional(x.GeneralizedEtaSquared)})
		}
		withUnit(&t, u, true, "SS", "MS")
		return t
	}
	out := []Table{}
	if classical {
		out = append(out, terms("anova", "Repeated-measures ANOVA (Condition, Time, Condition × Time)", r.Terms, ""))
	}
	if len(r.SimpleEffects) > 0 {
		t := terms("simple_effects", "Repeated-measures ANOVA of time within each condition", nil, "")
		for _, e := range r.SimpleEffects {
			t.Rows = append(t.Rows, terms("", "", e.Terms, e.Context).Rows...)
		}
		out = append(out, t)
	}
	posthoc := statisticalTable("posthoc", "Simple effects (paired t)", "comparison_id", "family", "context", "left_a", "left_b", "right_a", "right_b", "contrast", "difference", "unadjusted_CI_lower", "unadjusted_CI_upper", "t", "df", "p", "adjusted_p", "correction", "pairs")
	for _, c := range r.Comparisons {
		posthoc.Rows = append(posthoc.Rows, []Cell{text(c.ID), text(c.Family), text(c.Context), text(c.LeftA), text(c.LeftB), text(c.RightA), text(c.RightB), text(c.Contrast), num(c.Difference, -1), optional(c.Lower), optional(c.Upper), optional(c.Statistic), optional(c.DF), optional(c.P), num(c.AdjustedP, -1), text(c.Correction), num(float64(c.N), 0)})
	}
	withUnit(&posthoc, u, false, "difference", "unadjusted_CI_lower", "unadjusted_CI_upper")
	out = append(out, posthoc)
	units := statisticalTable("unit_observations", "Individual experimental unit observations", "factor_a", "factor_b", "unit_id", "unit_label", "value", "technical_readings")
	for _, g := range r.Groups {
		for i, v := range g.Values {
			id, readings := "", 1
			if i < len(g.Units) {
				id = g.Units[i]
			}
			if i < len(g.Readings) {
				readings = g.Readings[i]
			}
			units.Rows = append(units.Rows, []Cell{text(g.FactorA), text(g.FactorB), text(id), text(unitName(a, id)), num(v, -1), num(float64(readings), 0)})
		}
	}
	withUnit(&units, u, false, "value")
	out = append(out, units)
	if m := r.Model; m != nil && len(m.RandomComponents) > 0 {
		rc := statisticalTable("random_components", "Random components", "level", "variance")
		for _, c := range m.RandomComponents {
			rc.Rows = append(rc.Rows, []Cell{text(c.Level), num(c.Variance, -1)})
		}
		rc.Rows = append(rc.Rows, []Cell{text("residual"), num(m.ResidualVariance, -1)})
		withUnit(&rc, u, true, "variance")
		out = append(out, rc)
	}
	return out
}
