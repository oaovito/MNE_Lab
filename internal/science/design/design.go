// Package design records how measurements are organized into an
// experiment: which independent experimental unit (preparation) each
// measurement belongs to, under which condition and at which time point,
// and whether it is the unit's observation or a technical repeat.
//
// A design is organization, never data: it references measurements by
// identifier and never changes them. Every save is a new immutable
// revision linked to the previous one, with the field-level changes it
// made, so the history of the organization is its provenance.
package design

import (
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Version identifies the design contract in provenance records.
const Version = "experimental-design/1"

// Unit kinds and replicate kinds.
const (
	// KindIndependentPreparation is a separately prepared formulation: the
	// experimental unit the statistics count.
	KindIndependentPreparation = "independent_preparation"
	// ReplicateExperimental marks the measurement that is the unit's
	// observation for its condition and time.
	ReplicateExperimental = "experimental"
	// ReplicateTechnical marks a repeated read of the same unit, condition
	// and time. It never adds to the number of experimental units.
	ReplicateTechnical = "technical"
	// ReplicateInstrumentReading is a run row inside one instrument report.
	ReplicateInstrumentReading = "instrument_reading"
)

// TimeUnits are the time units a level may state (the cycle units).
var TimeUnits = []string{"hours", "days", "weeks", "months", "years"}

// Limits keep a design reviewable and its records small.
const (
	MaxUnits       = 200
	MaxConditions  = 20
	MaxTimes       = 60
	MaxAssignments = 5000
	maxLabel       = 120
	maxNote        = 2000
)

// Stable error identifiers.
var (
	ErrInvalid     = errors.New("design.invalid")
	ErrName        = errors.New("design.name_required")
	ErrUnit        = errors.New("design.unknown_unit")
	ErrLevel       = errors.New("design.unknown_level")
	ErrDuplicate   = errors.New("design.duplicate_level")
	ErrMeasurement = errors.New("design.duplicate_measurement")
	ErrDerivation  = errors.New("design.invalid_derivation")
	ErrPair        = errors.New("design.invalid_pair")
	ErrLimit       = errors.New("design.limit_reached")
	ErrConflict    = errors.New("design.revision_conflict")
	ErrNotFound    = errors.New("design.not_found")
)

// Unit is an experimental unit of the design. ID is the profile's
// experimental unit record; Label is its name when the revision was saved.
type Unit struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

// Condition is a level of the condition factor. DerivedFrom names the level
// this one is obtained from within the same unit (a C-SR aliquot is
// centrifuged from the same unit's F-SR preparation), so both belong to the
// same experimental unit.
type Condition struct {
	Label       string `json:"label"`
	DerivedFrom string `json:"derivedFrom,omitempty"`
	Process     string `json:"process,omitempty"`
}

// TimeLevel is a time point. Offset and Unit are optional; they are only
// what the person states, never taken from import or file dates.
type TimeLevel struct {
	Label  string   `json:"label"`
	Offset *float64 `json:"offset,omitempty"`
	Unit   string   `json:"unit,omitempty"`
}

// Assignment places one measurement in the design.
type Assignment struct {
	MeasurementID string `json:"measurementId"`
	UnitID        string `json:"unitId,omitempty"`
	Condition     string `json:"condition,omitempty"`
	Time          string `json:"time,omitempty"`
	ReplicateKind string `json:"replicateKind"`
	Note          string `json:"note,omitempty"`
}

// Pair records that two conditions of one unit are paired observations of
// that same unit (F-SR A ↔ C-SR A). A pair never joins two units.
type Pair struct {
	UnitID string `json:"unitId"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// Change is one field-level difference between a revision and the one it
// replaces.
type Change struct {
	Field         string `json:"field"`
	MeasurementID string `json:"measurementId,omitempty"`
	Subject       string `json:"subject,omitempty"`
	From          string `json:"from,omitempty"`
	To            string `json:"to,omitempty"`
}

// Sources of a revision.
const (
	SourceUser       = "user"
	SourceSuggestion = "suggestion" // proposals the person reviewed and confirmed
)

// Design is one immutable revision.
type Design struct {
	ID          string       `json:"id"`
	Root        string       `json:"root"`
	Revision    int          `json:"revision"`
	PreviousID  string       `json:"previousId,omitempty"`
	Created     time.Time    `json:"created"`
	Version     string       `json:"version"`
	Module      string       `json:"module"`
	Name        string       `json:"name"`
	Notes       string       `json:"notes,omitempty"`
	CycleID     string       `json:"cycleId,omitempty"`
	Units       []Unit       `json:"units"`
	Conditions  []Condition  `json:"conditions"`
	Times       []TimeLevel  `json:"times"`
	Assignments []Assignment `json:"assignments"`
	Pairs       []Pair       `json:"pairs"`
	Source      string       `json:"source"`
	Reason      string       `json:"reason,omitempty"`
	Changes     []Change     `json:"changes"`
}

func validText(s string, limit int) bool {
	return len([]rune(s)) <= limit && strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) < 0
}

func validLabel(s string) bool {
	return s != "" && validText(s, maxLabel) && !strings.ContainsAny(s, "\n\t")
}

// Normalize trims text and fills defaults; it never invents a level, a
// unit or an assignment.
func Normalize(d Design) Design {
	d.Name = strings.TrimSpace(d.Name)
	d.Notes = strings.TrimSpace(d.Notes)
	d.Reason = strings.TrimSpace(d.Reason)
	if d.Source == "" {
		d.Source = SourceUser
	}
	if d.Module == "" {
		d.Module = "lightscattering"
	}
	d.Units = append([]Unit{}, d.Units...)
	for i := range d.Units {
		d.Units[i].ID = strings.TrimSpace(d.Units[i].ID)
		d.Units[i].Label = strings.TrimSpace(d.Units[i].Label)
		if d.Units[i].Kind == "" {
			d.Units[i].Kind = KindIndependentPreparation
		}
	}
	d.Conditions = append([]Condition{}, d.Conditions...)
	for i := range d.Conditions {
		c := &d.Conditions[i]
		c.Label, c.DerivedFrom, c.Process = strings.TrimSpace(c.Label), strings.TrimSpace(c.DerivedFrom), strings.TrimSpace(c.Process)
	}
	d.Times = append([]TimeLevel{}, d.Times...)
	for i := range d.Times {
		d.Times[i].Label, d.Times[i].Unit = strings.TrimSpace(d.Times[i].Label), strings.TrimSpace(d.Times[i].Unit)
	}
	d.Assignments = append([]Assignment{}, d.Assignments...)
	for i := range d.Assignments {
		a := &d.Assignments[i]
		a.MeasurementID, a.UnitID = strings.TrimSpace(a.MeasurementID), strings.TrimSpace(a.UnitID)
		a.Condition, a.Time, a.Note = strings.TrimSpace(a.Condition), strings.TrimSpace(a.Time), strings.TrimSpace(a.Note)
		if a.ReplicateKind == "" {
			a.ReplicateKind = ReplicateExperimental
		}
	}
	d.Pairs = append([]Pair{}, d.Pairs...)
	for i := range d.Pairs {
		p := &d.Pairs[i]
		p.UnitID, p.From, p.To = strings.TrimSpace(p.UnitID), strings.TrimSpace(p.From), strings.TrimSpace(p.To)
	}
	return d
}

// Validate checks the design's own consistency. Whether its units and
// measurements exist in the profile is checked by the caller.
func Validate(d Design) error {
	if d.Name == "" {
		return ErrName
	}
	if !validText(d.Name, maxLabel) || strings.ContainsAny(d.Name, "\n\t") || !validText(d.Notes, maxNote) || !validText(d.Reason, maxNote) ||
		d.Module != "lightscattering" || (d.Source != SourceUser && d.Source != SourceSuggestion) {
		return ErrInvalid
	}
	if len(d.Units) > MaxUnits || len(d.Conditions) > MaxConditions || len(d.Times) > MaxTimes || len(d.Assignments) > MaxAssignments || len(d.Pairs) > MaxUnits*MaxConditions {
		return ErrLimit
	}
	units := map[string]bool{}
	for _, u := range d.Units {
		if u.ID == "" || !validText(u.Label, maxLabel) || u.Kind != KindIndependentPreparation {
			return ErrInvalid
		}
		if units[u.ID] {
			return ErrDuplicate
		}
		units[u.ID] = true
	}
	conditions := map[string]Condition{}
	for _, c := range d.Conditions {
		if !validLabel(c.Label) || !validText(c.Process, maxLabel) {
			return ErrInvalid
		}
		if _, dup := conditions[strings.ToLower(c.Label)]; dup {
			return ErrDuplicate
		}
		conditions[strings.ToLower(c.Label)] = c
	}
	for _, c := range d.Conditions {
		if c.DerivedFrom == "" {
			if c.Process != "" {
				return ErrDerivation
			}
			continue
		}
		parent, ok := conditions[strings.ToLower(c.DerivedFrom)]
		if !ok || strings.EqualFold(parent.Label, c.Label) || parent.DerivedFrom != "" {
			// One derivation step from a base level; no chains or loops.
			return ErrDerivation
		}
	}
	times := map[string]bool{}
	for _, tl := range d.Times {
		if !validLabel(tl.Label) {
			return ErrInvalid
		}
		if tl.Offset != nil && (math.IsNaN(*tl.Offset) || math.IsInf(*tl.Offset, 0) || *tl.Offset < 0) {
			return ErrInvalid
		}
		if tl.Unit != "" && !contains(TimeUnits, tl.Unit) {
			return ErrInvalid
		}
		if times[strings.ToLower(tl.Label)] {
			return ErrDuplicate
		}
		times[strings.ToLower(tl.Label)] = true
	}
	seen := map[string]bool{}
	for _, a := range d.Assignments {
		if a.MeasurementID == "" || !validText(a.Note, maxNote) {
			return ErrInvalid
		}
		if seen[a.MeasurementID] {
			return ErrMeasurement
		}
		seen[a.MeasurementID] = true
		if a.UnitID != "" && !units[a.UnitID] {
			return ErrUnit
		}
		if a.Condition != "" && conditionLabel(d, a.Condition) != a.Condition {
			return ErrLevel
		}
		if a.Time != "" && timeLabel(d, a.Time) != a.Time {
			return ErrLevel
		}
		if a.ReplicateKind != ReplicateExperimental && a.ReplicateKind != ReplicateTechnical {
			return ErrInvalid
		}
	}
	pairs := map[[3]string]bool{}
	for _, p := range d.Pairs {
		if !units[p.UnitID] || conditionLabel(d, p.From) != p.From || conditionLabel(d, p.To) != p.To || strings.EqualFold(p.From, p.To) {
			return ErrPair
		}
		k := [3]string{p.UnitID, strings.ToLower(p.From), strings.ToLower(p.To)}
		r := [3]string{p.UnitID, k[2], k[1]}
		if pairs[k] || pairs[r] {
			return ErrPair
		}
		pairs[k] = true
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// conditionLabel returns the declared spelling of a condition, or "".
func conditionLabel(d Design, s string) string {
	for _, c := range d.Conditions {
		if c.Label == s {
			return c.Label
		}
	}
	return ""
}

func timeLabel(d Design, s string) string {
	for _, tl := range d.Times {
		if tl.Label == s {
			return tl.Label
		}
	}
	return ""
}

// Time returns the declared time level with that label.
func (d Design) Time(label string) (TimeLevel, bool) {
	for _, tl := range d.Times {
		if tl.Label == label {
			return tl, true
		}
	}
	return TimeLevel{}, false
}

// Assignment returns the assignment of a measurement.
func (d Design) Assignment(measurementID string) (Assignment, bool) {
	for _, a := range d.Assignments {
		if a.MeasurementID == measurementID {
			return a, true
		}
	}
	return Assignment{}, false
}

// UnitLabel returns the label a unit had when this revision was saved.
func (d Design) UnitLabel(id string) string {
	for _, u := range d.Units {
		if u.ID == id {
			return u.Label
		}
	}
	return ""
}

// DerivedPairs proposes the pairs implied by derived conditions: every unit
// that has measurements under both a base condition and a level derived
// from it. This is a proposal for the person to keep or change.
func DerivedPairs(d Design) []Pair {
	has := map[[2]string]bool{}
	for _, a := range d.Assignments {
		if a.UnitID != "" && a.Condition != "" {
			has[[2]string{a.UnitID, a.Condition}] = true
		}
	}
	var out []Pair
	for _, u := range d.Units {
		for _, c := range d.Conditions {
			if c.DerivedFrom != "" && has[[2]string{u.ID, c.DerivedFrom}] && has[[2]string{u.ID, c.Label}] {
				out = append(out, Pair{UnitID: u.ID, From: c.DerivedFrom, To: c.Label})
			}
		}
	}
	return out
}

func fmtOffset(tl TimeLevel) string {
	s := tl.Label
	if tl.Offset != nil {
		s += " (" + trimFloat(*tl.Offset) + " " + tl.Unit + ")"
	}
	return strings.TrimSpace(s)
}

func trimFloat(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// Diff lists the field-level changes from prev to next, in a stable order.
func Diff(prev, next Design) []Change {
	var out []Change
	add := func(c Change) {
		if c.From != c.To {
			out = append(out, c)
		}
	}
	add(Change{Field: "name", From: prev.Name, To: next.Name})
	add(Change{Field: "notes", From: prev.Notes, To: next.Notes})
	add(Change{Field: "cycle", From: prev.CycleID, To: next.CycleID})

	pu, nu := map[string]Unit{}, map[string]Unit{}
	for _, u := range prev.Units {
		pu[u.ID] = u
	}
	for _, u := range next.Units {
		nu[u.ID] = u
		if p, ok := pu[u.ID]; !ok {
			add(Change{Field: "unit_added", Subject: u.ID, To: u.Label})
		} else if p.Label != u.Label {
			add(Change{Field: "unit_label", Subject: u.ID, From: p.Label, To: u.Label})
		}
	}
	for _, u := range prev.Units {
		if _, ok := nu[u.ID]; !ok {
			add(Change{Field: "unit_removed", Subject: u.ID, From: u.Label})
		}
	}

	pc := map[string]Condition{}
	for _, c := range prev.Conditions {
		pc[c.Label] = c
	}
	nc := map[string]bool{}
	for _, c := range next.Conditions {
		nc[c.Label] = true
		p, ok := pc[c.Label]
		if !ok {
			add(Change{Field: "condition_added", Subject: c.Label, To: c.Label})
			p = Condition{Label: c.Label}
		}
		add(Change{Field: "condition_derived_from", Subject: c.Label, From: p.DerivedFrom, To: c.DerivedFrom})
		add(Change{Field: "condition_process", Subject: c.Label, From: p.Process, To: c.Process})
	}
	for _, c := range prev.Conditions {
		if !nc[c.Label] {
			add(Change{Field: "condition_removed", Subject: c.Label, From: c.Label})
		}
	}

	pt := map[string]TimeLevel{}
	for _, tl := range prev.Times {
		pt[tl.Label] = tl
	}
	nt := map[string]bool{}
	for _, tl := range next.Times {
		nt[tl.Label] = true
		if p, ok := pt[tl.Label]; !ok {
			add(Change{Field: "time_added", Subject: tl.Label, To: fmtOffset(tl)})
		} else {
			add(Change{Field: "time_offset", Subject: tl.Label, From: fmtOffset(p), To: fmtOffset(tl)})
		}
	}
	for _, tl := range prev.Times {
		if !nt[tl.Label] {
			add(Change{Field: "time_removed", Subject: tl.Label, From: fmtOffset(tl)})
		}
	}

	pa := map[string]Assignment{}
	for _, a := range prev.Assignments {
		pa[a.MeasurementID] = a
	}
	na := map[string]bool{}
	for _, a := range next.Assignments {
		na[a.MeasurementID] = true
		p, ok := pa[a.MeasurementID]
		if !ok {
			add(Change{Field: "measurement_added", MeasurementID: a.MeasurementID, To: a.MeasurementID})
			p = Assignment{MeasurementID: a.MeasurementID}
		}
		add(Change{Field: "unit", MeasurementID: a.MeasurementID, From: p.UnitID, To: a.UnitID})
		add(Change{Field: "condition", MeasurementID: a.MeasurementID, From: p.Condition, To: a.Condition})
		add(Change{Field: "time", MeasurementID: a.MeasurementID, From: p.Time, To: a.Time})
		add(Change{Field: "replicate_kind", MeasurementID: a.MeasurementID, From: p.ReplicateKind, To: a.ReplicateKind})
		add(Change{Field: "note", MeasurementID: a.MeasurementID, From: p.Note, To: a.Note})
	}
	for _, a := range prev.Assignments {
		if !na[a.MeasurementID] {
			add(Change{Field: "measurement_removed", MeasurementID: a.MeasurementID, From: a.MeasurementID})
		}
	}

	key := func(p Pair) string { return p.UnitID + "\x00" + p.From + "\x00" + p.To }
	pp, np := map[string]bool{}, map[string]bool{}
	for _, p := range prev.Pairs {
		pp[key(p)] = true
	}
	for _, p := range next.Pairs {
		np[key(p)] = true
		if !pp[key(p)] {
			add(Change{Field: "pair_added", Subject: p.UnitID, To: p.From + " ↔ " + p.To})
		}
	}
	for _, p := range prev.Pairs {
		if !np[key(p)] {
			add(Change{Field: "pair_removed", Subject: p.UnitID, From: p.From + " ↔ " + p.To})
		}
	}
	if out == nil {
		out = []Change{}
	}
	return out
}

// Revise turns an edited draft into the next immutable revision of prev
// (or the first revision when prev is nil). The caller supplies the new
// revision's identifier and time.
func Revise(prev *Design, draft Design, id string, now time.Time) (Design, error) {
	d := Normalize(draft)
	if err := Validate(d); err != nil {
		return Design{}, err
	}
	d.ID, d.Created, d.Version = id, now.UTC(), Version
	if prev == nil {
		d.Root, d.Revision, d.PreviousID = id, 1, ""
		d.Changes = Diff(Design{}, d)
	} else {
		d.Root, d.Revision, d.PreviousID = prev.Root, prev.Revision+1, prev.ID
		d.Changes = Diff(*prev, d)
	}
	return d, nil
}

// Heads returns the revisions no other revision replaces, newest first.
// Two heads with the same root mean two edits of the same revision (for
// example on two machines); both are kept and shown.
func Heads(all []Design) []Design {
	replaced := map[string]bool{}
	for _, d := range all {
		if d.PreviousID != "" {
			replaced[d.PreviousID] = true
		}
	}
	var out []Design
	for _, d := range all {
		if !replaced[d.ID] {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Created.Equal(out[j].Created) {
			return out[i].Created.After(out[j].Created)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// History returns the chain of revisions ending at id, newest first.
func History(all []Design, id string) []Design {
	byID := map[string]Design{}
	for _, d := range all {
		byID[d.ID] = d
	}
	var out []Design
	seen := map[string]bool{}
	for cur, ok := byID[id]; ok && !seen[cur.ID]; cur, ok = byID[cur.PreviousID] {
		seen[cur.ID] = true
		out = append(out, cur)
	}
	return out
}
