// Package model is MNE Lab's normalized scientific data model.
//
// Raw data (the original file bytes) and derived data are kept apart: a
// Measurement references its SourceFile by identifier and checksum and is
// never used to rewrite it. Every numeric value keeps its unit and the
// exact text it was read from, so no precision is lost in parsing.
package model

import "time"

// Quantity is a numeric value with its unit and original text.
type Quantity struct {
	Value float64 `json:"value"`
	Raw   string  `json:"raw"`
	Unit  string  `json:"unit,omitempty"`
	// Label is the exact label used in the source file.
	Label string `json:"label,omitempty"`
	// Line is the 1-based line in the source file.
	Line int `json:"line,omitempty"`
}

// Decimals returns the number of decimals in the original text.
func (q Quantity) Decimals() int {
	for i := len(q.Raw) - 1; i >= 0; i-- {
		if q.Raw[i] == '.' || q.Raw[i] == ',' {
			n := 0
			for _, c := range q.Raw[i+1:] {
				if c >= '0' && c <= '9' {
					n++
				} else {
					break
				}
			}
			return n
		}
	}
	return 0
}

// Field is any labeled value found in a file, recognized or not.
type Field struct {
	Key   string   `json:"key,omitempty"` // neutral identifier when recognized
	Label string   `json:"label"`
	Text  string   `json:"text"`
	Unit  string   `json:"unit,omitempty"`
	Num   *float64 `json:"num,omitempty"`
	Line  int      `json:"line"`
}

// Column is one column of a distribution table.
type Column struct {
	Key    string    `json:"key,omitempty"` // diameter, intensity, volume, number
	Label  string    `json:"label"`
	Unit   string    `json:"unit,omitempty"`
	Values []float64 `json:"values"`
	Raw    []string  `json:"raw"`
}

// Distribution is a particle size distribution table.
type Distribution struct {
	ID     string `json:"id,omitempty"`
	Method string `json:"method,omitempty"`
	Format string `json:"format,omitempty"`
	// Auxiliary preserves columns whose scientific meaning is not mapped.
	Auxiliary []Field `json:"auxiliary,omitempty"`
	// SourceLines maps flattened points back to the original report rows.
	SourceLines []int    `json:"sourceLines,omitempty"`
	Columns     []Column `json:"columns"`
	FirstLine   int      `json:"firstLine"`
	LastLine    int      `json:"lastLine"`
}

// Column finds a column by key.
func (d *Distribution) Column(key string) *Column {
	if d == nil {
		return nil
	}
	for i := range d.Columns {
		if d.Columns[i].Key == key {
			return &d.Columns[i]
		}
	}
	return nil
}

// Weightings returns the weighting columns present (intensity, volume, number).
func (d *Distribution) Weightings() []string {
	var out []string
	for _, k := range []string{"intensity", "volume", "number"} {
		if d.Column(k) != nil {
			out = append(out, k)
		}
	}
	return out
}

// Timestamp is a measurement time as stated in the file.
type Timestamp struct {
	// Time is the parsed instant. When TZKnown is false it represents the
	// wall-clock reading interpreted as UTC and must be displayed as-is.
	Time      time.Time `json:"time"`
	Raw       string    `json:"raw"`
	TZKnown   bool      `json:"tzKnown"`
	Ambiguous bool      `json:"ambiguous,omitempty"` // e.g. 03/04 day/month order
	// Confirmed is true when the user confirmed or corrected it.
	Confirmed bool `json:"confirmed,omitempty"`
	// Source: "file" or "user".
	Source string `json:"source"`
}

// Parameter keys recognized by LIGHTSCATTERING.
const (
	EffectiveDiameter = "effective_diameter"
	Polydispersity    = "polydispersity"
	CountRate         = "count_rate"
	AverageCountRate  = "average_count_rate"
	BaselineIndex     = "baseline_index"
	SampleID          = "sample_id"
	MeasuredAt        = "measured_at"
)

// Measurement is one DLS measurement read from a file.
type Measurement struct {
	ID                     string              `json:"id"`
	FileID                 string              `json:"fileId"`
	SourceRange            string              `json:"sourceRange,omitempty"`
	SourceSheet            string              `json:"sourceSheet,omitempty"` // original workbook sheet; Line fields refer to its rows
	Index                  int                 `json:"index"`                 // position inside the file
	SampleID               string              `json:"sampleId,omitempty"`
	MeasuredAt             *Timestamp          `json:"measuredAt,omitempty"`
	Params                 map[string]Quantity `json:"params"`
	Fields                 []Field             `json:"fields"`
	Dist                   *Distribution       `json:"distribution,omitempty"`
	Distributions          []Distribution      `json:"distributions,omitempty"`
	DistributionID         string              `json:"distributionId,omitempty"`
	DistributionSelectedAt *time.Time          `json:"distributionSelectedAt,omitempty"`
	// User-provided organization (never overwrites file data).
	Label      string   `json:"label,omitempty"`
	Replicate  int      `json:"replicate,omitempty"`
	Condition  string   `json:"condition,omitempty"`
	Experiment string   `json:"experiment,omitempty"`
	Group      string   `json:"group,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Notes      string   `json:"notes,omitempty"`
	Parser     string   `json:"parser"`
	Spec       string   `json:"spec"`
}

// Param returns a parameter if present. Absent never means zero.
func (m *Measurement) Param(key string) (Quantity, bool) {
	q, ok := m.Params[key]
	return q, ok
}

// ImportSelection records an explicit, reproducible subset of a workbook.
type ImportSelection struct {
	Sheets []SheetSelection `json:"sheets"`
}
type SheetSelection struct {
	Name  string `json:"name"`
	Range string `json:"range,omitempty"`
}

// SourceFile describes an imported original file. The bytes live in the
// profile's blob store under BlobID and are never modified.
type SourceFile struct {
	ImportSelection *ImportSelection `json:"importSelection,omitempty"`
	ID              string           `json:"id"`
	BlobID          string           `json:"blobId"`
	Name            string           `json:"name"`
	Format          string           `json:"format"`
	Size            int64            `json:"size"`
	SHA256          string           `json:"sha256"`
	ImportedAt      time.Time        `json:"importedAt"`
	Module          string           `json:"module"`
	Parser          string           `json:"parser"`
	Spec            string           `json:"spec"`
	Status          string           `json:"status"` // parsed, partial, failed
	Measurements    []string         `json:"measurements"`
	Encoding        string           `json:"encoding,omitempty"`
	Delimiter       string           `json:"delimiter,omitempty"`
	Decimal         string           `json:"decimal,omitempty"`
	Warnings        []string         `json:"warnings,omitempty"`
	Error           string           `json:"error,omitempty"`
	Experiment      string           `json:"experiment,omitempty"`
	Group           string           `json:"group,omitempty"`
	Tags            []string         `json:"tags,omitempty"`
	Notes           string           `json:"notes,omitempty"`
	Origin          string           `json:"origin"` // import, sync, package
	Trashed         bool             `json:"trashed,omitempty"`
}
