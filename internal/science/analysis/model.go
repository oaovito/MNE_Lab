// Package analysis defines reproducible analyses of recognized scientific
// quantities. Calculations are delegated to the bundled R statistical engine;
// this package validates design, source identity and the persisted contract.
package analysis

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/oaovito/mne_lab/internal/science/design"
	"github.com/oaovito/mne_lab/internal/science/model"
)

const Schema = 1
const CalculationVersion = "mnelab-statistics/1"
const EngineVersion = "webR/0.6.0; R/4.6.0"

// DesignWithinWithin marks the paired Condition x Time design: factor A
// (condition) and factor B (time) both vary within each experimental unit.
// The empty design keeps the original between-A, within-B behaviour.
const DesignWithinWithin = "within_within"

func ExpectedEngine(d Definition) string {
	engine := EngineVersion
	if d.Design == DesignWithinWithin {
		if d.Method == "mixed" {
			engine += "; nlme/3.1-169; mixed-within2/1"
		} else {
			engine += "; paired-rm/1"
		}
		if d.PostHoc == "simple_effects" {
			engine += "; paired-simple-effects/1"
		}
		return engine
	}
	if d.Method == "mixed" {
		engine += "; nlme/3.1-169; mixed-random-intercept/2"
	}
	if d.PostHoc == "dunnett" {
		engine += "; multcomp/1.4-30; mvtnorm/1.2-4; Dunnett/1"
	}
	if d.EffectCI {
		engine += "; MBESS-source/5.0.1; eta2-CI/1"
	}
	return engine
}

var (
	ErrDefinition = errors.New("statistics.invalid_definition")
	ErrStructure  = errors.New("statistics.review_structure")
	ErrUnits      = errors.New("statistics.incompatible_units")
	ErrNumeric    = errors.New("statistics.invalid_numeric")
	ErrSource     = errors.New("statistics.source_changed")
	ErrMethod     = errors.New("statistics.method_unavailable")
	ErrDesign     = errors.New("statistics.incompatible_design")
	ErrControl    = errors.New("statistics.control_required")
	ErrFamily     = errors.New("statistics.dunnett_family_limit")
	// ErrDesignMismatch refuses observations that do not follow the saved
	// experimental design revision the definition names.
	ErrDesignMismatch = errors.New("statistics.design_mismatch")
	// ErrPseudoreplication refuses treating repeated observations of the
	// same experimental unit as independent subjects (addendum B §37). It is
	// a structure refusal: errors.Is(err, ErrStructure) holds.
	ErrPseudoreplication error = &codedError{"statistics.pseudoreplication", ErrStructure}
	// ErrPairedDetected refuses the original layout (condition between
	// units) when every unit is observed under several conditions and times,
	// and points to the paired Condition x Time design (addendum A §57).
	ErrPairedDetected error = &codedError{"statistics.paired_longitudinal_detected", ErrDesign}
)

// codedError is a stable identifier that is also a more general refusal.
type codedError struct {
	code   string
	parent error
}

func (e *codedError) Error() string { return e.code }
func (e *codedError) Unwrap() error { return e.parent }

// ExperimentalUnit has an identity independent of the visible sample name.
// IDs are created and resolved in the current Profile, never inferred from
// filenames, replicate numbers or dates.
type ExperimentalUnit struct {
	ID      string    `json:"id"`
	Label   string    `json:"label"`
	Created time.Time `json:"created"`
}

type ObservationDefinition struct {
	MeasurementID string `json:"measurementId"`
	FactorA       string `json:"factorA"`
	FactorB       string `json:"factorB,omitempty"`
	UnitID        string `json:"unitId,omitempty"`
	ExcludeReason string `json:"excludeReason,omitempty"`
}

type Definition struct {
	Schema               int     `json:"schema"`
	Module               string  `json:"module"`
	Title                string  `json:"title"`
	CycleID              string  `json:"cycleId,omitempty"`
	Variable             string  `json:"variable"`
	FactorAName          string  `json:"factorAName"`
	FactorBName          string  `json:"factorBName,omitempty"`
	Structure            string  `json:"structure"` // independent or repeated; explicit user decision
	StructureReviewed    bool    `json:"structureReviewed"`
	Method               string  `json:"method"`
	Alpha                float64 `json:"alpha"`
	PostHoc              string  `json:"postHoc"`
	EffectCI             bool    `json:"effectCI,omitempty"`
	Control              string  `json:"control,omitempty"`
	SphericityCorrection string  `json:"sphericityCorrection,omitempty"`
	// Design is empty for the original layout or DesignWithinWithin.
	Design string `json:"design,omitempty"`
	// Adjustment is the multiplicity correction of paired simple effects
	// (holm or bonferroni); empty for every other post-hoc choice.
	Adjustment string `json:"adjustment,omitempty"`
	// DesignID names the saved experimental design revision the unit,
	// condition and time of every observation were taken from; empty when
	// the person organized the observations in the editor.
	DesignID     string                  `json:"designId,omitempty"`
	Observations []ObservationDefinition `json:"observations"`
}

type Observation struct {
	ObservationDefinition
	FileID    string          `json:"fileId"`
	SampleID  string          `json:"sampleId,omitempty"`
	Replicate int             `json:"replicate,omitempty"`
	Quantity  *model.Quantity `json:"quantity,omitempty"`
	Missing   bool            `json:"missing"`
}

type Source struct {
	Measurement model.Measurement `json:"measurement"`
	File        model.SourceFile  `json:"file"`
}

// Snapshot is returned by the core and bound to a scoped, expiring receipt.
// Client-provided numeric columns are never accepted as scientific sources.
type Snapshot struct {
	CycleSource  json.RawMessage `json:"cycleSource,omitempty"`
	Definition   Definition      `json:"definition"`
	Observations []Observation   `json:"observations"`
	Sources      []Source        `json:"sources"`
	Unit         string          `json:"unit"`
	SourceHash   string          `json:"sourceHash"`
	Design       Design          `json:"design"`
	Receipt      string          `json:"receipt,omitempty"`
	AccountID    string          `json:"accountId"`
	ProfileID    string          `json:"profileId"`
	// ExperimentalDesign is the immutable design revision named by
	// Definition.DesignID; DesignHeads are the current revisions of that
	// design, so a later revision marks the analysis Source Changed.
	ExperimentalDesign *design.Design `json:"experimentalDesign,omitempty"`
	DesignHeads        []string       `json:"designHeads,omitempty"`
	// UnitLabels are the names of the experimental units when the snapshot
	// was taken. Names are presentation: they never enter the source hash.
	UnitLabels map[string]string `json:"unitLabels,omitempty"`
}

type Design struct {
	N                 int      `json:"n"`
	Missing           int      `json:"missing"`
	Excluded          int      `json:"excluded"`
	ExperimentalUnits int      `json:"experimentalUnits"`
	LevelsA           []string `json:"levelsA"`
	LevelsB           []string `json:"levelsB"`
	Balanced          bool     `json:"balanced"`
	CompleteRepeated  bool     `json:"completeRepeated"`
	Recommended       string   `json:"recommended"`
	Warnings          []string `json:"warnings"`
	// Paired design only: distinct unit x A x B cells (one observation each
	// after technical readings are averaged) and cells holding several readings.
	UnitCells             int `json:"unitCells,omitempty"`
	TechnicalReadingCells int `json:"technicalReadingCells,omitempty"`
}

type Term struct {
	Source            string   `json:"source"`
	SS                *float64 `json:"ss,omitempty"`
	DF                float64  `json:"df"`
	DenominatorDF     *float64 `json:"denominatorDF,omitempty"`
	MS                *float64 `json:"ms,omitempty"`
	F                 *float64 `json:"f,omitempty"`
	P                 *float64 `json:"p,omitempty"`
	EtaSquared        *float64 `json:"etaSquared,omitempty"`
	PartialEtaSquared *float64 `json:"partialEtaSquared,omitempty"`
	OmegaSquared      *float64 `json:"omegaSquared,omitempty"`
	// Paired design: generalized eta squared and, on corrected terms, the
	// sphericity epsilon applied to both df.
	GeneralizedEtaSquared *float64 `json:"generalizedEtaSquared,omitempty"`
	Epsilon               *float64 `json:"epsilon,omitempty"`
}

type Group struct {
	FactorA string    `json:"factorA"`
	FactorB string    `json:"factorB,omitempty"`
	N       int       `json:"n"`
	Values  []float64 `json:"values"`
	Mean    float64   `json:"mean"`
	SD      *float64  `json:"sd,omitempty"`
	SEM     *float64  `json:"sem,omitempty"`
	CILower *float64  `json:"ciLower,omitempty"`
	CIUpper *float64  `json:"ciUpper,omitempty"`
	// Paired design: values are one per experimental unit (the mean of its
	// technical readings), with the unit ID and reading count in the same order.
	Units    []string `json:"units,omitempty"`
	Readings []int    `json:"readings,omitempty"`
}

type Comparison struct {
	ID         string   `json:"id"`
	LeftA      string   `json:"leftA"`
	LeftB      string   `json:"leftB,omitempty"`
	RightA     string   `json:"rightA"`
	RightB     string   `json:"rightB,omitempty"`
	Contrast   string   `json:"contrast"`
	Context    string   `json:"context,omitempty"`
	Difference float64  `json:"difference"`
	Lower      *float64 `json:"lower,omitempty"`
	Upper      *float64 `json:"upper,omitempty"`
	AdjustedP  float64  `json:"adjustedP"`
	Correction string   `json:"correction"`
	// Paired simple effects: family, paired t statistic, its df, unadjusted
	// p and number of pairs. Lower/Upper are then individual paired-t limits.
	Family    string   `json:"family,omitempty"`
	Statistic *float64 `json:"statistic,omitempty"`
	DF        *float64 `json:"df,omitempty"`
	P         *float64 `json:"p,omitempty"`
	N         int      `json:"n,omitempty"`
}

// SimpleEffect is the repeated-measures ANOVA of B inside one level of A.
type SimpleEffect struct {
	Context     string             `json:"context"`
	FactorA     string             `json:"factorA"`
	Terms       []Term             `json:"terms"`
	Corrections map[string]float64 `json:"corrections,omitempty"`
}

type RandomComponent struct {
	Level    string  `json:"level"`
	Variance float64 `json:"variance"`
}

type Diagnostic struct {
	Code      string   `json:"code"`
	Statistic *float64 `json:"statistic,omitempty"`
	P         *float64 `json:"p,omitempty"`
	Details   string   `json:"details,omitempty"`
}

type FixedCoefficient struct {
	Name     string   `json:"name"`
	Estimate float64  `json:"estimate"`
	SE       float64  `json:"se"`
	DF       *float64 `json:"df,omitempty"`
	Lower    *float64 `json:"lower,omitempty"`
	Upper    *float64 `json:"upper,omitempty"`
}
type MixedModel struct {
	Family                     string             `json:"family"`
	Fixed                      string             `json:"fixed"`
	Estimation                 string             `json:"estimation"`
	Random                     string             `json:"random"`
	ResidualCovariance         string             `json:"residualCovariance"`
	Test                       string             `json:"test"`
	LevelsA                    []string           `json:"levelsA"`
	LevelsB                    []string           `json:"levelsB"`
	RandomVariance             float64            `json:"randomVariance"`
	ResidualVariance           float64            `json:"residualVariance"`
	LogLikelihood              float64            `json:"logLikelihood"`
	BoundaryTolerance          float64            `json:"boundaryTolerance"`
	CoefficientConfidenceLevel float64            `json:"coefficientConfidenceLevel,omitempty"`
	CoefficientIntervalMethod  string             `json:"coefficientIntervalMethod,omitempty"`
	FixedCoefficients          []FixedCoefficient `json:"fixedCoefficients"`
	RandomComponents           []RandomComponent  `json:"randomComponents,omitempty"`
}

type Results struct {
	Engine          string             `json:"engine"`
	Calculation     string             `json:"calculation"`
	Method          string             `json:"method"`
	SSType          string             `json:"ssType,omitempty"`
	Terms           []Term             `json:"terms"`
	Groups          []Group            `json:"groups"`
	Comparisons     []Comparison       `json:"comparisons"`
	Diagnostics     []Diagnostic       `json:"diagnostics"`
	Residuals       []float64          `json:"residuals"`
	QQTheoretical   []float64          `json:"qqTheoretical"`
	QQObserved      []float64          `json:"qqObserved"`
	Warnings        []string           `json:"warnings"`
	Corrections     map[string]float64 `json:"corrections,omitempty"`
	Model           *MixedModel        `json:"model,omitempty"`
	EffectIntervals []EffectInterval   `json:"effectIntervals,omitempty"`
	Design          string             `json:"design,omitempty"`
	SimpleEffects   []SimpleEffect     `json:"simpleEffects,omitempty"`
}

type EffectInterval struct {
	Source          string   `json:"source"`
	Effect          string   `json:"effect"`
	ConfidenceLevel float64  `json:"confidenceLevel"`
	Method          string   `json:"method"`
	Status          string   `json:"status"`
	Lower           *float64 `json:"lower,omitempty"`
	Upper           *float64 `json:"upper,omitempty"`
	LowerAtBoundary bool     `json:"lowerAtBoundary,omitempty"`
}

// StatisticalAnalysis preserves the definition, source snapshot and results.
// Recalculation creates another revision; saved results are never rewritten
// when a source or organization attribute changes.
type StatisticalAnalysis struct {
	Schema        int       `json:"schema"`
	ID            string    `json:"id"`
	PreviousID    string    `json:"previousId,omitempty"`
	Created       time.Time `json:"created"`
	Snapshot      Snapshot  `json:"snapshot"`
	Results       Results   `json:"results"`
	AppVersion    string    `json:"appVersion"`
	SourceChanged bool      `json:"sourceChanged"`
	ResultOrigin  string    `json:"resultOrigin"` // local bundled browser worker, not a server attestation
}

func (d Definition) Validate() error {
	if d.Schema != Schema || d.Module != "lightscattering" || strings.TrimSpace(d.Title) == "" || !ValidText(d.Title, 240) || !ValidText(d.FactorAName, 120) || !ValidText(d.FactorBName, 120) || len(d.Observations) < 2 || len(d.Observations) > 10000 || !(d.Alpha > 0 && d.Alpha < 1) || math.IsNaN(d.Alpha) || math.IsInf(d.Alpha, 0) {
		return ErrDefinition
	}
	if !d.StructureReviewed || (d.Structure != "independent" && d.Structure != "repeated") {
		return ErrStructure
	}
	if !ValidText(d.DesignID, 96) {
		return ErrDefinition
	}
	if d.Design != "" {
		return d.validateWithinWithin()
	}
	if d.Adjustment != "" {
		return ErrMethod
	}
	switch d.Variable {
	case model.EffectiveDiameter, model.Polydispersity, model.CountRate, model.AverageCountRate, model.BaselineIndex:
	default:
		return ErrDefinition
	}
	switch d.Method {
	case "one_way", "welch", "two_way":
		if d.Structure != "independent" {
			return ErrDesign
		}
	case "repeated", "mixed":
		if d.Structure != "repeated" {
			return ErrDesign
		}
	default:
		return ErrMethod
	}
	if d.EffectCI && d.Method != "one_way" {
		return ErrDesign
	}
	if d.PostHoc != "none" && d.PostHoc != "tukey" && d.PostHoc != "dunnett" {
		return ErrMethod
	}
	if d.PostHoc == "tukey" && (d.Method == "welch" || d.Method == "repeated") {
		return ErrDesign
	}
	if d.Method == "mixed" && (d.PostHoc != "none" || d.SphericityCorrection != "") {
		return ErrDesign
	}
	if d.PostHoc == "dunnett" {
		if d.Alpha >= 0.5 {
			return ErrDefinition
		}
		if d.Method != "one_way" {
			return ErrDesign
		}
		if strings.TrimSpace(d.Control) == "" || !ValidText(d.Control, 120) {
			return ErrControl
		}
	} else if d.Control != "" {
		return ErrMethod
	}
	if d.SphericityCorrection != "" && d.SphericityCorrection != "GG" && d.SphericityCorrection != "HF" {
		return ErrDefinition
	}
	seen := map[string]bool{}
	for _, o := range d.Observations {
		if o.MeasurementID == "" || seen[o.MeasurementID] || !ValidText(o.FactorA, 120) || !ValidText(o.FactorB, 120) || !ValidText(o.ExcludeReason, 1000) {
			return ErrDefinition
		}
		seen[o.MeasurementID] = true
		if o.ExcludeReason == "" && (d.Method == "one_way" || d.Method == "welch") && o.FactorB != "" {
			return ErrDesign
		}
		if o.ExcludeReason == "" && (strings.TrimSpace(o.FactorA) == "" || ((d.Method == "two_way" || d.Method == "repeated" || d.Method == "mixed") && strings.TrimSpace(o.FactorB) == "")) {
			return ErrDesign
		}
	}
	return nil
}

// validateWithinWithin checks the paired Condition x Time definition: both
// factors are within-unit, so only repeated structures and methods apply.
func (d Definition) validateWithinWithin() error {
	if d.Design != DesignWithinWithin {
		return ErrDefinition
	}
	switch d.Variable {
	case model.EffectiveDiameter, model.Polydispersity, model.CountRate, model.AverageCountRate, model.BaselineIndex:
	default:
		return ErrDefinition
	}
	if d.Structure != "repeated" {
		// Both factors vary within each unit: independence is pseudoreplication.
		return ErrPseudoreplication
	}
	switch d.Method {
	case "repeated":
		if d.SphericityCorrection != "GG" && d.SphericityCorrection != "HF" {
			return ErrDefinition
		}
	case "mixed":
		if d.SphericityCorrection != "" {
			return ErrDesign
		}
	default:
		return ErrDesign
	}
	if d.EffectCI || d.Control != "" {
		return ErrDesign
	}
	switch d.PostHoc {
	case "none":
		if d.Adjustment != "" {
			return ErrMethod
		}
	case "simple_effects":
		if d.Adjustment != "holm" && d.Adjustment != "bonferroni" {
			return ErrDefinition
		}
	default:
		return ErrDesign
	}
	seen := map[string]bool{}
	for _, o := range d.Observations {
		if o.MeasurementID == "" || seen[o.MeasurementID] || !ValidText(o.FactorA, 120) || !ValidText(o.FactorB, 120) || !ValidText(o.ExcludeReason, 1000) {
			return ErrDefinition
		}
		seen[o.MeasurementID] = true
		if o.ExcludeReason == "" && (strings.TrimSpace(o.FactorA) == "" || strings.TrimSpace(o.FactorB) == "") {
			return ErrDesign
		}
	}
	return nil
}

// Plain labels remain valid JSON through both the Go and R serializers.
func ValidText(s string, limit int) bool {
	return len(s) <= limit && strings.IndexFunc(s, unicode.IsControl) < 0
}
