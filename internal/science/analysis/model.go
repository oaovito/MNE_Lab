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

	"github.com/oaovito/mne_lab/internal/science/model"
)

const Schema = 1
const CalculationVersion = "mnelab-statistics/1"
const EngineVersion = "webR/0.6.0; R/4.6.0"

func ExpectedEngine(d Definition) string {
	if d.Method == "mixed" {
		return EngineVersion + "; nlme/3.1-169; mixed-random-intercept/1"
	}
	if d.PostHoc == "dunnett" {
		return EngineVersion + "; multcomp/1.4-30; mvtnorm/1.2-4; Dunnett/1"
	}
	return EngineVersion
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
)

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
	Schema               int                     `json:"schema"`
	Module               string                  `json:"module"`
	Title                string                  `json:"title"`
	CycleID              string                  `json:"cycleId,omitempty"`
	Variable             string                  `json:"variable"`
	FactorAName          string                  `json:"factorAName"`
	FactorBName          string                  `json:"factorBName,omitempty"`
	Structure            string                  `json:"structure"` // independent or repeated; explicit user decision
	StructureReviewed    bool                    `json:"structureReviewed"`
	Method               string                  `json:"method"`
	Alpha                float64                 `json:"alpha"`
	PostHoc              string                  `json:"postHoc"`
	Control              string                  `json:"control,omitempty"`
	SphericityCorrection string                  `json:"sphericityCorrection,omitempty"`
	Observations         []ObservationDefinition `json:"observations"`
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
}

type Diagnostic struct {
	Code      string   `json:"code"`
	Statistic *float64 `json:"statistic,omitempty"`
	P         *float64 `json:"p,omitempty"`
	Details   string   `json:"details,omitempty"`
}

type FixedCoefficient struct {
	Name     string  `json:"name"`
	Estimate float64 `json:"estimate"`
	SE       float64 `json:"se"`
}
type MixedModel struct {
	Family             string             `json:"family"`
	Fixed              string             `json:"fixed"`
	Estimation         string             `json:"estimation"`
	Random             string             `json:"random"`
	ResidualCovariance string             `json:"residualCovariance"`
	Test               string             `json:"test"`
	LevelsA            []string           `json:"levelsA"`
	LevelsB            []string           `json:"levelsB"`
	RandomVariance     float64            `json:"randomVariance"`
	ResidualVariance   float64            `json:"residualVariance"`
	LogLikelihood      float64            `json:"logLikelihood"`
	BoundaryTolerance  float64            `json:"boundaryTolerance"`
	FixedCoefficients  []FixedCoefficient `json:"fixedCoefficients"`
}

type Results struct {
	Engine        string             `json:"engine"`
	Calculation   string             `json:"calculation"`
	Method        string             `json:"method"`
	SSType        string             `json:"ssType,omitempty"`
	Terms         []Term             `json:"terms"`
	Groups        []Group            `json:"groups"`
	Comparisons   []Comparison       `json:"comparisons"`
	Diagnostics   []Diagnostic       `json:"diagnostics"`
	Residuals     []float64          `json:"residuals"`
	QQTheoretical []float64          `json:"qqTheoretical"`
	QQObserved    []float64          `json:"qqObserved"`
	Warnings      []string           `json:"warnings"`
	Corrections   map[string]float64 `json:"corrections,omitempty"`
	Model         *MixedModel        `json:"model,omitempty"`
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

// Plain labels remain valid JSON through both the Go and R serializers.
func ValidText(s string, limit int) bool {
	return len(s) <= limit && strings.IndexFunc(s, unicode.IsControl) < 0
}
