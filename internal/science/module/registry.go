// Package module describes capabilities; a new parser does not inherit another
// module's scientific validation by sharing infrastructure or drawing a graph.
package module

const LightScattering = "lightscattering"
const Zeta = "zeta"
const NTA = "nta"
const UnknownMalvern = "unknown_malvern_dts"
const UnknownCompound = "unknown_compound"

type Capability struct {
	ID                   string   `json:"id"`
	Implementation       string   `json:"implementation"`
	ScientificValidation string   `json:"scientificValidation"`
	Blockers             []string `json:"blockers"`
}

func Capabilities() []Capability {
	return []Capability{
		{"statistical_analysis", "IMPLEMENTED / TESTED (documented scope)", "VALIDATED (documented scope)", []string{"Effect intervals beyond documented population eta squared in fixed one-factor models remain unavailable; Mixed is limited to documented ML random-intercept models; Dunnett to documented one-way independent models"}},
		{LightScattering, "IMPLEMENTED / CONTINUING", "PARTIAL", []string{"finish documented instrument-field and quantitative-reference validation"}},
		{Zeta, "IMPLEMENTATION ALLOWED", "PENDING", []string{"official potential/mobility/quality field documentation", "real native exports with corresponding trusted results", "reproducible field/unit/numeric comparisons"}},
		{NTA, "ARCHITECTURE / PREPARATION ALLOWED", "PENDING", []string{"official NanoSight export/software schema", "real tracking and summary/distribution exports with trusted references", "independent numeric comparisons"}},
		{UnknownMalvern, "PARTIAL: CONTAINER / RECORD METADATA", "UNVALIDATED", []string{"version-aware scientific field decoding and authoritative semantics", "matching official summary/distribution exports"}},
		{UnknownCompound, "PARTIAL: CONTAINER PRESERVATION", "UNVALIDATED", []string{"identify actual record schema and scientific module before normalization"}},
	}
}
