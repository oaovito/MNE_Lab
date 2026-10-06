// Package cycle implements temporal studies (Cycles): a set of planned time
// points (start + k·interval up to the duration) and the measurements
// associated with each point.
//
// Hours, Days and Weeks are exact durations. Months and Years use calendar
// arithmetic: adding one month to 31 January gives the last day of
// February, never "30 days later".
package cycle

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/stats"
)

// Unit is a cycle time unit.
type Unit string

const (
	Hours  Unit = "hours"
	Days   Unit = "days"
	Weeks  Unit = "weeks"
	Months Unit = "months"
	Years  Unit = "years"
)

// Units lists valid units in display order.
var Units = []Unit{Hours, Days, Weeks, Months, Years}

// Limits from the specification.
const (
	MinCount = 1
	MaxCount = 99
)

// Errors (stable identifiers).
var (
	ErrUnit     = errors.New("cycle.invalid_unit")
	ErrInterval = errors.New("cycle.interval_out_of_range")
	ErrDuration = errors.New("cycle.duration_out_of_range")
	ErrOrder    = errors.New("cycle.interval_exceeds_duration")
	ErrName     = errors.New("cycle.name_required")
)

// Config defines a cycle.
type Config struct {
	Name       string    `json:"name"`
	Experiment string    `json:"experiment,omitempty"`
	SampleID   string    `json:"sampleId,omitempty"`
	Start      time.Time `json:"start"`
	StartTZ    bool      `json:"startTzKnown"`
	Interval   int       `json:"interval"`
	Unit       Unit      `json:"unit"`
	Duration   int       `json:"duration"`
	Replicates int       `json:"replicates"`
	Params     []string  `json:"params"`
}

// Validate checks the configuration against the specification limits.
func (c Config) Validate() error {
	if c.Name == "" {
		return ErrName
	}
	ok := false
	for _, u := range Units {
		if u == c.Unit {
			ok = true
		}
	}
	if !ok {
		return ErrUnit
	}
	if c.Interval < MinCount || c.Interval > MaxCount {
		return ErrInterval
	}
	if c.Duration < MinCount || c.Duration > MaxCount {
		return ErrDuration
	}
	if c.Interval > c.Duration {
		return ErrOrder
	}
	return nil
}

// Add moves t forward by n units with the semantics described above.
func Add(t time.Time, u Unit, n int) time.Time {
	switch u {
	case Hours:
		return t.Add(time.Duration(n) * time.Hour)
	case Days:
		return t.AddDate(0, 0, n)
	case Weeks:
		return t.AddDate(0, 0, 7*n)
	case Months:
		return addMonthsClamped(t, n)
	case Years:
		return addMonthsClamped(t, 12*n)
	}
	return t
}

func addMonthsClamped(t time.Time, n int) time.Time {
	y, m, d := t.Date()
	total := int(m) - 1 + n
	ny, nm := y+total/12, time.Month(total%12+1)
	if total < 0 && total%12 != 0 {
		ny--
	}
	last := time.Date(ny, nm+1, 0, 0, 0, 0, 0, t.Location()).Day()
	if d > last {
		d = last
	}
	return time.Date(ny, nm, d, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}

// Point is a planned time point.
type Point struct {
	Index  int       `json:"index"`
	Offset int       `json:"offset"` // in units from the start
	Unit   Unit      `json:"unit"`
	Target time.Time `json:"target"`
}

// Points lists the planned points: offsets 0, interval, 2·interval … ≤ duration.
func (c Config) Points() ([]Point, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	var out []Point
	for k := 0; k*c.Interval <= c.Duration; k++ {
		off := k * c.Interval
		out = append(out, Point{Index: k, Offset: off, Unit: c.Unit, Target: Add(c.Start, c.Unit, off)})
	}
	return out, nil
}

// Assignment links a measurement to a point.
type Assignment struct {
	MeasurementID string `json:"measurementId"`
	Point         int    `json:"point"` // -1 when unassigned
	// Status: "confirmed" (user), "auto" (unambiguous rule match),
	// "needs_confirmation" or "unassigned".
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	// Replicate number within the point (1-based), when known.
	Replicate int `json:"replicate,omitempty"`
}

// Candidate is a measurement offered for association.
type Candidate struct {
	ID         string
	SampleID   string
	Experiment string
	MeasuredAt *model.Timestamp
}

// window returns the half-distance to the neighbor points around index i.
func window(points []Point, i int) (before, after time.Duration) {
	if i > 0 {
		before = points[i].Target.Sub(points[i-1].Target) / 2
	} else if len(points) > 1 {
		before = points[1].Target.Sub(points[0].Target) / 2
	}
	if i < len(points)-1 {
		after = points[i+1].Target.Sub(points[i].Target) / 2
	} else if i > 0 {
		after = points[i].Target.Sub(points[i-1].Target) / 2
	}
	return
}

// Associate proposes point assignments. The explicit rule: a measurement
// belongs to the point whose window (half the distance to each neighbor)
// contains its real timestamp. Anything doubtful — missing or ambiguous
// date, different sample or experiment, outside every window, exactly on a
// boundary — is returned as needing confirmation, never silently placed.
// Real timestamps are never rewritten to fit the plan.
func Associate(c Config, points []Point, cands []Candidate) []Assignment {
	var out []Assignment
	for _, m := range cands {
		a := Assignment{MeasurementID: m.ID, Point: -1}
		switch {
		case m.MeasuredAt == nil:
			a.Status, a.Reason = "unassigned", "cycle.missing_measurement_date"
		case m.MeasuredAt.Ambiguous && !m.MeasuredAt.Confirmed:
			a.Status, a.Reason = "needs_confirmation", "cycle.ambiguous_date"
		default:
			t := m.MeasuredAt.Time
			best, tie := -1, false
			for i, p := range points {
				b, af := window(points, i)
				d := t.Sub(p.Target)
				if (d < 0 && -d < b) || (d >= 0 && d < af) {
					if best >= 0 {
						tie = true
					}
					best = i
				} else if (d < 0 && -d == b) || (d > 0 && d == af) {
					tie = true
				}
			}
			switch {
			case best < 0:
				a.Status, a.Reason = "needs_confirmation", "cycle.outside_all_points"
			case tie:
				a.Point, a.Status, a.Reason = best, "needs_confirmation", "cycle.between_points"
			case c.SampleID != "" && m.SampleID != "" && m.SampleID != c.SampleID:
				a.Point, a.Status, a.Reason = best, "needs_confirmation", "cycle.sample_mismatch"
			case c.Experiment != "" && m.Experiment != "" && m.Experiment != c.Experiment:
				a.Point, a.Status, a.Reason = best, "needs_confirmation", "cycle.experiment_mismatch"
			default:
				a.Point, a.Status = best, "auto"
			}
		}
		out = append(out, a)
	}
	return out
}

// PointResult holds the statistics of one parameter at one point.
type PointResult struct {
	Point   int           `json:"point"`
	Offset  int           `json:"offset"`
	Target  time.Time     `json:"target"`
	Missing bool          `json:"missing"`
	Summary stats.Summary `json:"summary"`
	Sources []string      `json:"sources"`
}

// Series computes one parameter over the cycle from the accepted
// assignments ("auto" and "confirmed"). Points without measurements are
// reported as missing, never interpolated.
func Series(points []Point, as []Assignment, values map[string]float64) []PointResult {
	byPoint := map[int][]string{}
	for _, a := range as {
		if a.Point >= 0 && (a.Status == "auto" || a.Status == "confirmed") {
			byPoint[a.Point] = append(byPoint[a.Point], a.MeasurementID)
		}
	}
	var out []PointResult
	for _, p := range points {
		ids := byPoint[p.Index]
		sort.Strings(ids)
		var vs []float64
		var used []string
		for _, id := range ids {
			if v, ok := values[id]; ok && !math.IsNaN(v) {
				vs = append(vs, v)
				used = append(used, id)
			}
		}
		out = append(out, PointResult{Point: p.Index, Offset: p.Offset, Target: p.Target, Missing: len(vs) == 0, Summary: stats.Describe(vs), Sources: used})
	}
	return out
}

// Label returns a neutral label key for a point, e.g. ("weeks", 2) — the
// interface renders it as "Week 2" / "Semana 2".
func Label(p Point) string { return fmt.Sprintf("%s:%d", p.Unit, p.Offset) }
