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
const LightScatteringSpecID = "ls-spec/0.3-provisional"

// Source is one specification source.
type Source struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Kind     string `json:"kind"` // owner-documentation, manufacturer, literature, decision
	Filename string `json:"filename,omitempty"`
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
	{ID: "owner-pdf-ls-zeta", Title: "Documentação de Dados Lightscattering e Zeta", Kind: "owner-documentation",
		Filename: "Documentacao_de_Dados_Lightscattering_e_Zeta_LATEST.pdf", SHA256: "ddc16ccba54d8e00545a0b665a942f92d15e3d5f6cba833b48ac6bbba835ea87",
		Revision: "Package LATEST; document has no stated revision", Added: "2026-10-08", Status: "consulted",
		Notes: "Initial private review confirms field names and DLS graph conventions. Export layout, algorithm selection, G(d)/C(d) meaning and scientific explanations remain under validation. The document stays outside product artifacts; Zeta is outside the current module scope."},
	{ID: "owner-pdf-ls-zeta-previous", Title: "Documentação de Dados Lightscattering e Zeta (previous)", Kind: "owner-documentation",
		Filename: "Documentacao_de_Dados_Lightscattering_e_Zeta_PREVIOUS.pdf", SHA256: "8cb6b7190d4839166b60bcd06d6cfe8249259c8a7d198cc8712712208870eaab",
		Revision: "Package PREVIOUS; document has no stated revision", Added: "2026-10-08", Status: "consulted",
		Notes: "Retained privately for comparison with the latest reference; not a source of experimental measurements."},
	{ID: "product-spec-ls", Title: "MNE Lab product specification, LIGHTSCATTERING sections", Kind: "owner-documentation",
		Status: "incorporated", Added: "2026-10-06", Notes: "Field names, axis conventions (Hydrodynamic Diameter nm / Intensity %, log X), cycles 1–99 Hours/Days/Weeks/Months/Years, replicates."},
	{ID: "brookhaven-manual", Title: "Instruction Manual for the NanoBrook Series, Rev 1.3", Kind: "manufacturer",
		Status: "consulted", Notes: "Confirms the parameter names Effective Diameter, Polydispersity, Baseline Index and Count Rate; does not document the text export layout."},
	{ID: "decision-parser", Title: "Tolerant parser decisions", Kind: "decision", Status: "incorporated",
		Notes: "Delimiter and decimal separator detection; day/month order never guessed; absent field never zero; unknown fields kept verbatim."},
}

// Rules lists implemented scientific rules and their sources.
var Rules = []Rule{
	{"ls.methods", "Preserve Lognormal/Multimodal and report/spreadsheet alternatives; require explicit selection and pin saved graph methods", []string{"owner-pdf-ls-zeta", "decision-parser"}},
	{"ls.native-columns", "Instrument d(nm) is diameter in nm; G(d) is relative intensity without assumed percentage normalization; keep C(d) as unmapped auxiliary data", []string{"owner-pdf-ls-zeta", "decision-parser"}},
	{"ls.count-rate-mean", "Keep explicitly labeled Average Count Rate separate from Current Count Rate, with source units, raw precision and source line; do not infer a missing unit or normalize mixed units", []string{"decision-parser"}},
	{"ls.time-zone-comparison", "Never automatically associate a known-zone instant with an unknown-zone wall clock; require explicit cycle-point confirmation without rewriting source time", []string{"decision-parser"}},
	{"ls.fields", "Recognize Effective Diameter, Polydispersity, Current Count Rate, BaseLine Index, Diameter/Particle Size, Intensity (and Volume/Number when present)", []string{"product-spec-ls", "brookhaven-manual"}},
	{"ls.units", "Keep units as stated in the file; never assume a unit", []string{"product-spec-ls"}},
	{"ls.graph.axes", "Default DLS graph: X Hydrodynamic Diameter (nm), Y Intensity (%), logarithmic X when all diameters are positive", []string{"product-spec-ls"}},
	{"ls.weighting", "Intensity, Volume and Number are shown only when present; no conversion between weightings", []string{"product-spec-ls"}},
	{"ls.stats", "Mean and sample standard deviation (n−1) only with at least two observations; individual values always kept", []string{"product-spec-ls", "decision-parser"}},
	{"ls.cycles", "Cycle points at start + k·interval up to the duration; months and years use calendar arithmetic", []string{"product-spec-ls"}},
	{"ls.dates", "Measurement time is never replaced by import time; ambiguous dates require confirmation", []string{"product-spec-ls", "decision-parser"}},
}
