// Package reference records the scientific specifications that rules in
// MNE Lab are derived from, so provenance can name the exact version used.
//
// Documentation ≠ experimental data: these entries describe sources of
// rules and are never treated as measurements. Nothing here is sent
// anywhere; it exists for traceability inside the project and in exported
// research packages.
package reference

// LightScatteringSpecID is the specification version applied by the
// LIGHTSCATTERING parser and graph rules.
const LightScatteringSpecID = "ls-spec/0.1-provisional"

// Source is one specification source.
type Source struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Kind     string `json:"kind"` // owner-documentation, manufacturer, literature, decision
	SHA256   string `json:"sha256,omitempty"`
	Revision string `json:"revision,omitempty"`
	Added    string `json:"added,omitempty"`
	Status   string `json:"status"`
	Notes    string `json:"notes,omitempty"`
}

// Rule ties an implemented behavior to the source(s) it comes from.
type Rule struct {
	ID      string   `json:"id"`
	Summary string   `json:"summary"`
	Sources []string `json:"sources"`
}

// Sources lists every specification source known to this build.
var Sources = []Source{
	{ID: "owner-pdf-ls-zeta", Title: "Documentação de Dados Lightscattering e Zeta.pdf", Kind: "owner-documentation",
		Status: "pending", Notes: "Named by the product specification as the official scientific reference. Not yet incorporated: checksum, date and revision will be recorded when the document is added."},
	{ID: "product-spec-ls", Title: "MNE Lab product specification, LIGHTSCATTERING sections", Kind: "owner-documentation",
		Status: "incorporated", Added: "2026-10-06", Notes: "Field names, axis conventions (Hydrodynamic Diameter nm / Intensity %, log X), cycles 1–99 Hours/Days/Weeks/Months/Years, replicates."},
	{ID: "brookhaven-manual", Title: "Instruction Manual for the NanoBrook Series, Rev 1.3", Kind: "manufacturer",
		Status: "consulted", Notes: "Confirms the parameter names Effective Diameter, Polydispersity, Baseline Index and Count Rate; does not document the text export layout."},
	{ID: "decision-parser", Title: "Tolerant parser decisions", Kind: "decision", Status: "incorporated",
		Notes: "Delimiter and decimal separator detection; day/month order never guessed; absent field never zero; unknown fields kept verbatim."},
}

// Rules lists implemented scientific rules and their sources.
var Rules = []Rule{
	{"ls.fields", "Recognize Effective Diameter, Polydispersity, Current Count Rate, BaseLine Index, Diameter/Particle Size, Intensity (and Volume/Number when present)", []string{"product-spec-ls", "brookhaven-manual"}},
	{"ls.units", "Keep units as stated in the file; never assume a unit", []string{"product-spec-ls"}},
	{"ls.graph.axes", "Default DLS graph: X Hydrodynamic Diameter (nm), Y Intensity (%), logarithmic X when all diameters are positive", []string{"product-spec-ls"}},
	{"ls.weighting", "Intensity, Volume and Number are shown only when present; no conversion between weightings", []string{"product-spec-ls"}},
	{"ls.stats", "Mean and sample standard deviation (n−1) only with at least two observations; individual values always kept", []string{"product-spec-ls", "decision-parser"}},
	{"ls.cycles", "Cycle points at start + k·interval up to the duration; months and years use calendar arithmetic", []string{"product-spec-ls"}},
	{"ls.dates", "Measurement time is never replaced by import time; ambiguous dates require confirmation", []string{"product-spec-ls", "decision-parser"}},
}
