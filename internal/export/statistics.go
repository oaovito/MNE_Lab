package export

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/go-pdf/fpdf"
	"github.com/oaovito/mne_lab/internal/plot"
	"github.com/oaovito/mne_lab/internal/science/analysis"
)

func statisticalTable(id, name string, keys ...string) Table {
	t := Table{ID: id, Name: name, Rows: [][]Cell{}}
	for _, key := range keys {
		t.Columns = append(t.Columns, Column{Key: key, Label: key})
	}
	return t
}
func optional(v *float64) Cell {
	if v == nil {
		return text("")
	}
	return num(*v, -1)
}

// StatisticalData uses machine precision. Rounding belongs only to display.
// CSV/TSV writers export one table at a time; XLSX retains every named sheet.
func StatisticalData(a analysis.StatisticalAnalysis) DataSet {
	d := DataSet{Title: a.Snapshot.Definition.Title, Engine: a.Results.Engine, Created: a.Created,
		Notes: append(append([]string{}, a.Results.Warnings...), a.Snapshot.Design.Warnings...),
		Meta:  []KV{{Key: "analysis", Label: "Analysis ID", Value: a.ID}, {Key: "module", Label: "Scientific module", Value: a.Snapshot.Definition.Module}, {Key: "sourceHash", Label: "Source snapshot SHA-256", Value: a.Snapshot.SourceHash}, {Key: "method", Label: "Method", Value: a.Results.Method}, {Key: "calculation", Label: "Calculation version", Value: a.Results.Calculation}, {Key: "resultOrigin", Label: "Result origin", Value: a.ResultOrigin}}}
	if a.Snapshot.Definition.Control != "" {
		d.Meta = append(d.Meta, KV{Key: "control", Label: "Control group", Value: a.Snapshot.Definition.Control})
	}
	an := statisticalTable("anova", "ANOVA", "source", "SS", "df", "MS", "F", "p", "eta_squared", "partial_eta_squared", "omega_squared")
	effects := statisticalTable("effects", "Effect sizes", "source", "eta_squared", "partial_eta_squared", "omega_squared")
	for _, r := range a.Results.Terms {
		an.Rows = append(an.Rows, []Cell{text(r.Source), optional(r.SS), num(r.DF, -1), optional(r.MS), optional(r.F), optional(r.P), optional(r.EtaSquared), optional(r.PartialEtaSquared), optional(r.OmegaSquared)})
		effects.Rows = append(effects.Rows, []Cell{text(r.Source), optional(r.EtaSquared), optional(r.PartialEtaSquared), optional(r.OmegaSquared)})
	}
	groups := statisticalTable("groups", "Group summaries", "factor_a", "factor_b", "n", "mean", "SD", "SEM", "CI_lower", "CI_upper")
	for _, r := range a.Results.Groups {
		groups.Rows = append(groups.Rows, []Cell{text(r.FactorA), text(r.FactorB), num(float64(r.N), 0), num(r.Mean, -1), optional(r.SD), optional(r.SEM), optional(r.CILower), optional(r.CIUpper)})
	}
	observations := statisticalTable("observations", "Observations", "measurement_id", "file_id", "sample_id", "factor_a", "factor_b", "unit_id", "value", "raw", "unit", "missing", "exclude_reason")
	for _, o := range a.Snapshot.Observations {
		v, raw, unit := text(""), "", ""
		if o.Quantity != nil {
			v = num(o.Quantity.Value, -1)
			raw = o.Quantity.Raw
			unit = o.Quantity.Unit
		}
		observations.Rows = append(observations.Rows, []Cell{text(o.MeasurementID), text(o.FileID), text(o.SampleID), text(o.FactorA), text(o.FactorB), text(o.UnitID), v, text(raw), text(unit), text(fmt.Sprint(o.Missing)), text(o.ExcludeReason)})
	}
	posthoc := statisticalTable("posthoc", "Multiple comparisons", "comparison_id", "left_a", "left_b", "right_a", "right_b", "contrast", "context", "difference", "CI_lower", "CI_upper", "adjusted_p", "correction")
	for _, c := range a.Results.Comparisons {
		posthoc.Rows = append(posthoc.Rows, []Cell{text(c.ID), text(c.LeftA), text(c.LeftB), text(c.RightA), text(c.RightB), text(c.Contrast), text(c.Context), num(c.Difference, -1), optional(c.Lower), optional(c.Upper), num(c.AdjustedP, -1), text(c.Correction)})
	}
	diagnostics := statisticalTable("diagnostics", "Diagnostics", "code", "statistic", "p", "details")
	for _, r := range a.Results.Diagnostics {
		diagnostics.Rows = append(diagnostics.Rows, []Cell{text(r.Code), optional(r.Statistic), optional(r.P), text(r.Details)})
	}
	residuals := statisticalTable("residuals", "Residuals and QQ", "row", "residual", "normal_quantile", "ordered_residual")
	for i, v := range a.Results.Residuals {
		residuals.Rows = append(residuals.Rows, []Cell{num(float64(i+1), 0), num(v, -1), num(a.Results.QQTheoretical[i], -1), num(a.Results.QQObserved[i], -1)})
	}
	corrections := statisticalTable("corrections", "Sphericity corrections", "correction", "epsilon")
	for _, k := range []string{"GG", "HF", "applied"} {
		if v, ok := a.Results.Corrections[k]; ok {
			corrections.Rows = append(corrections.Rows, []Cell{text(k), num(v, -1)})
		}
	}
	d.Tables = []Table{an, groups, observations, posthoc, diagnostics, residuals, effects, corrections}
	if m := a.Results.Model; m != nil {
		tests := statisticalTable("anova", "Marginal Wald F tests", "source", "df", "denominator_df", "F", "p")
		for _, r := range a.Results.Terms {
			tests.Rows = append(tests.Rows, []Cell{text(r.Source), num(r.DF, -1), optional(r.DenominatorDF), optional(r.F), optional(r.P)})
		}
		model := statisticalTable("mixed_model", "Mixed model", "family", "fixed", "estimation", "random", "residual_covariance", "test", "levels_a", "levels_b", "random_variance", "residual_variance", "log_likelihood", "boundary_tolerance")
		la, _ := json.Marshal(m.LevelsA)
		lb, _ := json.Marshal(m.LevelsB)
		model.Rows = append(model.Rows, []Cell{text(m.Family), text(m.Fixed), text(m.Estimation), text(m.Random), text(m.ResidualCovariance), text(m.Test), text(string(la)), text(string(lb)), num(m.RandomVariance, -1), num(m.ResidualVariance, -1), num(m.LogLikelihood, -1), num(m.BoundaryTolerance, -1)})
		coefficients := statisticalTable("coefficients", "Fixed coefficients (sum contrasts)", "coefficient", "estimate", "conditional_GLS_SE")
		for _, v := range m.FixedCoefficients {
			coefficients.Rows = append(coefficients.Rows, []Cell{text(v.Name), num(v.Estimate, -1), num(v.SE, -1)})
		}
		// All adapters iterate these tables: no mixed terms are presented as
		// classical sums of squares, effect sizes or sphericity corrections.
		d.Tables = []Table{tests, groups, observations, diagnostics, residuals, model, coefficients}
	}
	return d
}

// StatisticalReport is a paginated, real PDF, using the existing bundled font
// and PDF library. It lists every table row; large results are never truncated.
func StatisticalReport(a analysis.StatisticalAnalysis) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(12, 12, 12)
	pdf.SetAutoPageBreak(true, 12)
	pdf.AddUTF8FontFromBytes("inter", "", plot.FontBytes(plot.FontSans))
	pdf.AddUTF8FontFromBytes("inter", "B", plot.FontBytes(plot.FontSansBold))
	pdf.SetTitle(a.Snapshot.Definition.Title, true)
	pdf.SetCreator("MNE Lab", true)
	pdf.SetCreationDate(a.Created)
	pdf.SetModificationDate(a.Created)
	pdf.AddPage()
	pdf.SetFont("inter", "B", 14)
	pdf.MultiCell(0, 7, a.Snapshot.Definition.Title, "", "L", false)
	pdf.SetFont("inter", "", 9)
	pdf.MultiCell(0, 5, fmt.Sprintf("MNE Lab derived analysis | %s | %s\n%s | %s\nalpha=%g | variable=%s | unit=%s\nSource snapshot SHA-256: %s", a.Snapshot.Definition.Module, a.Results.Method, a.Results.Engine, a.Results.Calculation, a.Snapshot.Definition.Alpha, a.Snapshot.Definition.Variable, a.Snapshot.Unit, a.Snapshot.SourceHash), "", "L", false)
	if a.SourceChanged {
		pdf.MultiCell(0, 5, "Sources changed after this analysis was saved. Results retain the original snapshot.", "", "L", false)
	}
	d := StatisticalData(a)
	for _, table := range d.Tables {
		pdf.Ln(3)
		pdf.SetFont("inter", "B", 10)
		pdf.MultiCell(0, 5, table.Name, "", "L", false)
		pdf.SetFont("inter", "", 8)
		for _, row := range table.Rows {
			var line bytes.Buffer
			for i, c := range row {
				if i > 0 {
					line.WriteString(" | ")
				}
				line.WriteString(table.Columns[i].Key + "=")
				if c.Num != nil {
					if *c.Num == 0 && (table.Columns[i].Key == "p" || table.Columns[i].Key == "adjusted_p") {
						line.WriteString("below engine precision")
					} else {
						line.WriteString(FormatValue(*c.Num, c.Dec, "."))
					}
				} else {
					line.WriteString(c.Text)
				}
			}
			pdf.MultiCell(0, 4, line.String(), "", "L", false)
		}
	}
	for _, warning := range d.Notes {
		pdf.MultiCell(0, 5, warning, "", "L", false)
	}
	var b bytes.Buffer
	err := pdf.Output(&b)
	return b.Bytes(), err
}
