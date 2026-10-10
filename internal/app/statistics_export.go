package app

import (
	"encoding/json"

	"github.com/oaovito/mne_lab/internal/export"
	"github.com/oaovito/mne_lab/internal/plot"
	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
)

func analysisInput(a analysis.StatisticalAnalysis) graph.Input {
	in := graph.Input{Measurements: map[string]model.Measurement{}, Files: map[string]model.SourceFile{}}
	for _, s := range a.Snapshot.Sources {
		in.Measurements[s.Measurement.ID] = s.Measurement
		in.Files[s.File.ID] = s.File
	}
	return in
}
func (p *Profile) analysisOutputs(id string, req ExportRequest, spec plot.Spec, multi bool) ([]output, string, error) {
	a, err := p.Analysis(id)
	if err != nil {
		return nil, "", err
	}
	// Local account/profile identifiers and ephemeral receipts do not belong in
	// a transferable scientific export. Experimental/source identities remain.
	a.Snapshot.AccountID, a.Snapshot.ProfileID, a.Snapshot.Receipt = "", "", ""
	base := export.Suggest(export.NameInfo{Title: a.Snapshot.Definition.Title, Module: "LIGHTSCATTERING", Subject: "Statistics"})
	prefix := ""
	if multi {
		prefix = base + "/"
	}
	d := export.StatisticalData(a)
	graphs, err := p.Graphs(false)
	if err != nil {
		return nil, "", err
	}
	annotationGraphs := []graph.Definition{}
	for _, g := range graphs {
		if g.Kind == graph.KindStatistical && g.AnalysisID == a.ID {
			annotationGraphs = append(annotationGraphs, g)
		}
	}
	outs := []output{}
	addJSON := func(name, role string, value any) error {
		b, e := json.MarshalIndent(value, "", "  ")
		if e == nil {
			outs = append(outs, output{name: prefix + name, role: role, data: append(b, '\n')})
		}
		return e
	}
	for _, format := range req.Formats {
		if !export.Compatible(export.KindAnalysis, format) {
			return nil, "", export.ErrFormat
		}
		switch format {
		case "json":
			if e := addJSON(base+".json", "statistics", a); e != nil {
				return nil, "", e
			}
		case "pdf":
			b, e := export.StatisticalReport(a)
			if e != nil {
				return nil, "", e
			}
			outs = append(outs, output{name: prefix + base + ".pdf", role: "statistics", data: b})
		case "package":
			for _, entry := range []struct {
				path  string
				value any
			}{
				{"statistics/analysis-definitions/definition.json", a.Snapshot.Definition},
				{"statistics/anova-tables/anova.json", a.Results.Terms},
				{"statistics/posthoc-tests/comparisons.json", a.Results.Comparisons},
				{"statistics/diagnostics/diagnostics.json", map[string]any{"tests": a.Results.Diagnostics, "residuals": a.Results.Residuals, "qqTheoretical": a.Results.QQTheoretical, "qqObserved": a.Results.QQObserved, "sphericity": a.Results.Corrections, "warnings": d.Notes}},
				{"statistics/effect-sizes/effects.json", a.Results.Terms},
				{"statistics/graph-annotations/annotations.json", map[string]any{"graphs": annotationGraphs, "availableComparisons": a.Results.Comparisons}},
				{"statistics/analysis-provenance/snapshot.json", a.Snapshot},
				{"statistics/analysis-summary/analysis.json", a},
			} {
				if e := addJSON(entry.path, "statistics", entry.value); e != nil {
					return nil, "", e
				}
			}
			b, e := export.StatisticalReport(a)
			if e != nil {
				return nil, "", e
			}
			outs = append(outs, output{name: prefix + "statistics/analysis-summary/report.pdf", role: "statistics", data: b})
			b, e = export.WriteData(d, "xlsx", req.Data)
			if e != nil {
				return nil, "", e
			}
			outs = append(outs, output{name: prefix + "statistics/analysis-summary/results.xlsx", role: "statistics", data: b})
			b, e = export.WriteData(d, "json", req.Data)
			if e != nil {
				return nil, "", e
			}
			outs = append(outs, output{name: prefix + "statistics/effect-sizes/all-tables.json", role: "statistics", data: b})
		case "xlsx":
			b, e := export.WriteData(d, format, req.Data)
			if e != nil {
				return nil, "", e
			}
			outs = append(outs, output{name: prefix + base + export.Formats[format].Ext, role: "statistics", data: b})
		default:
			// Every table gets its own delimited file; a CSV never silently loses
			// post-hoc, observations, diagnostic or effect-size tables.
			for _, table := range d.Tables {
				single := d
				single.Tables = []export.Table{table}
				b, e := export.WriteData(single, format, req.Data)
				if e != nil {
					return nil, "", e
				}
				outs = append(outs, output{name: prefix + base + "_" + table.ID + export.Formats[format].Ext, role: "statistics", data: b})
			}
		}
	}
	_ = spec // Reports use fixed paginated A4 rather than a figure-size preference.
	return outs, base, nil
}
