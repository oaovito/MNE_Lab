package design

import (
	"sort"
	"strings"
)

// Cell is one unit × condition × time combination of a design.
type Cell struct {
	UnitID       string   `json:"unitId"`
	Condition    string   `json:"condition,omitempty"`
	Time         string   `json:"time,omitempty"`
	Experimental int      `json:"experimental"`
	Technical    int      `json:"technical"`
	Measurements []string `json:"measurements,omitempty"`
}

// PairStatus says at which time points both members of a pair were measured.
type PairStatus struct {
	Pair
	Times    []string `json:"times"`
	Complete bool     `json:"complete"`
}

// Finding is a warning about how the design may be read.
type Finding struct {
	Code         string   `json:"code"`
	Units        []string `json:"units,omitempty"`
	Measurements []string `json:"measurements,omitempty"`
	Subject      string   `json:"subject,omitempty"`
}

// Detected structures.
const (
	StructurePairedLongitudinal = "paired_longitudinal" // units × within condition × within time
	StructurePaired             = "paired"              // units × within condition
	StructureLongitudinal       = "longitudinal"        // units × within time
	StructureIndependent        = "independent_groups"  // each unit under one condition, once
	StructureEmpty              = "empty"
	StructureOther              = "other"
)

// Structure summarizes a design for review before any analysis. Its counts
// answer one question first: how many independent experimental units are
// there? Measurements, technical repeats and instrument readings never add
// to that number.
type Structure struct {
	Measurements      int          `json:"measurements"`
	Assigned          int          `json:"assigned"`
	ExperimentalUnits int          `json:"experimentalUnits"`
	Observations      int          `json:"observations"`
	TechnicalReadings int          `json:"technicalReadings"`
	Units             []string     `json:"units"`
	Conditions        []string     `json:"conditions"`
	Times             []string     `json:"times"`
	ConditionWithin   bool         `json:"conditionWithin"`
	TimeWithin        bool         `json:"timeWithin"`
	Complete          bool         `json:"complete"`
	Cells             []Cell       `json:"cells"`
	Missing           []Cell       `json:"missing"`
	Pairs             []PairStatus `json:"pairs"`
	Detected          string       `json:"detected"`
	Recommended       string       `json:"recommended,omitempty"`
	Findings          []Finding    `json:"findings"`
}

// Recommendation for a paired longitudinal design (addendum: Condition ×
// Time with the experimental unit as subject). It is a suggestion to
// review; it never runs or configures an analysis by itself.
const RecommendConditionTime = "condition_time_repeated"

// MeasurementInfo is what the assessment may read about a measurement.
type MeasurementInfo struct {
	SampleID string
}

// Detect computes the structure of a design. info is optional.
func Detect(d Design, info map[string]MeasurementInfo) Structure {
	s := Structure{Measurements: len(d.Assignments), Cells: []Cell{}, Missing: []Cell{}, Pairs: []PairStatus{}, Findings: []Finding{}, Units: []string{}, Conditions: []string{}, Times: []string{}}
	for _, c := range d.Conditions {
		s.Conditions = append(s.Conditions, c.Label)
	}
	for _, tl := range d.Times {
		s.Times = append(s.Times, tl.Label)
	}
	type key struct{ u, c, t string }
	cells := map[key]*Cell{}
	var order []key
	unitConds := map[string]map[string]bool{}
	unitTimes := map[string]map[string]bool{}
	units := map[string]bool{}
	var unassigned []string
	for _, a := range d.Assignments {
		if a.UnitID == "" {
			unassigned = append(unassigned, a.MeasurementID)
			continue
		}
		s.Assigned++
		units[a.UnitID] = true
		k := key{a.UnitID, a.Condition, a.Time}
		c := cells[k]
		if c == nil {
			c = &Cell{UnitID: a.UnitID, Condition: a.Condition, Time: a.Time}
			cells[k] = c
			order = append(order, k)
		}
		c.Measurements = append(c.Measurements, a.MeasurementID)
		if a.ReplicateKind == ReplicateTechnical {
			c.Technical++
		} else {
			c.Experimental++
		}
		if unitConds[a.UnitID] == nil {
			unitConds[a.UnitID], unitTimes[a.UnitID] = map[string]bool{}, map[string]bool{}
		}
		if a.Condition != "" {
			unitConds[a.UnitID][a.Condition] = true
		}
		if a.Time != "" {
			unitTimes[a.UnitID][a.Time] = true
		}
	}
	// Units in declaration order; the count is distinct unit identities.
	for _, u := range d.Units {
		if units[u.ID] {
			s.Units = append(s.Units, u.ID)
		}
	}
	s.ExperimentalUnits = len(s.Units)
	for _, k := range order {
		c := *cells[k]
		s.Cells = append(s.Cells, c)
		s.TechnicalReadings += c.Technical
		if c.Experimental > 0 || c.Technical > 0 {
			s.Observations++
		}
		if c.Experimental > 1 {
			s.Findings = append(s.Findings, Finding{Code: "design.cell_several_experimental", Units: []string{c.UnitID}, Measurements: c.Measurements, Subject: cellName(c)})
		}
	}
	for _, u := range s.Units {
		if len(unitConds[u]) > 1 {
			s.ConditionWithin = true
		}
		if len(unitTimes[u]) > 1 {
			s.TimeWithin = true
		}
	}
	// Completeness over the declared levels actually used.
	condLevels, timeLevels := usedLevels(d.Conditions, d.Assignments), usedTimes(d.Times, d.Assignments)
	if len(condLevels) == 0 {
		condLevels = []string{""}
	}
	if len(timeLevels) == 0 {
		timeLevels = []string{""}
	}
	// A factor that varies within units needs every level in every unit; a
	// factor that does not (each unit under one level) needs only its own.
	own := func(levels []string, has map[string]bool, within bool) []string {
		if within || len(has) == 0 {
			return levels
		}
		var out []string
		for _, l := range levels {
			if has[l] {
				out = append(out, l)
			}
		}
		return out
	}
	for _, u := range s.Units {
		for _, c := range own(condLevels, unitConds[u], s.ConditionWithin) {
			for _, t := range own(timeLevels, unitTimes[u], s.TimeWithin) {
				if cells[key{u, c, t}] == nil {
					s.Missing = append(s.Missing, Cell{UnitID: u, Condition: c, Time: t})
				}
			}
		}
	}
	s.Complete = s.ExperimentalUnits > 0 && len(s.Missing) == 0 && len(unassigned) == 0
	for _, p := range d.Pairs {
		ps := PairStatus{Pair: p, Times: []string{}}
		for _, t := range timeLevels {
			if cells[key{p.UnitID, p.From, t}] != nil && cells[key{p.UnitID, p.To, t}] != nil {
				ps.Times = append(ps.Times, t)
			}
		}
		ps.Complete = len(ps.Times) == len(timeLevels)
		if len(ps.Times) == 0 {
			s.Findings = append(s.Findings, Finding{Code: "design.pair_without_observations", Units: []string{p.UnitID}, Subject: p.From + " ↔ " + p.To})
		}
		s.Pairs = append(s.Pairs, ps)
	}
	switch {
	case s.ExperimentalUnits == 0:
		s.Detected = StructureEmpty
	case s.ConditionWithin && s.TimeWithin:
		s.Detected = StructurePairedLongitudinal
	case s.ConditionWithin:
		s.Detected = StructurePaired
	case s.TimeWithin:
		s.Detected = StructureLongitudinal
	case len(s.Cells) == s.ExperimentalUnits:
		s.Detected = StructureIndependent
	default:
		s.Detected = StructureOther
	}
	if s.Detected == StructurePairedLongitudinal && s.ExperimentalUnits >= 2 {
		s.Recommended = RecommendConditionTime
	}
	if len(unassigned) > 0 {
		s.Findings = append(s.Findings, Finding{Code: "design.measurement_without_unit", Measurements: unassigned})
	}
	if len(s.Missing) > 0 && s.ExperimentalUnits > 0 {
		s.Findings = append(s.Findings, Finding{Code: "design.incomplete"})
	}
	s.Findings = append(s.Findings, Assess(d, info)...)
	return s
}

func usedLevels(levels []Condition, as []Assignment) []string {
	used := map[string]bool{}
	for _, a := range as {
		if a.UnitID != "" {
			used[a.Condition] = true
		}
	}
	var out []string
	for _, c := range levels {
		if used[c.Label] {
			out = append(out, c.Label)
		}
	}
	if used[""] && len(out) > 0 {
		out = append(out, "")
	}
	return out
}

func usedTimes(levels []TimeLevel, as []Assignment) []string {
	used := map[string]bool{}
	for _, a := range as {
		if a.UnitID != "" {
			used[a.Time] = true
		}
	}
	var out []string
	for _, tl := range levels {
		if used[tl.Label] {
			out = append(out, tl.Label)
		}
	}
	if used[""] && len(out) > 0 {
		out = append(out, "")
	}
	return out
}

func cellName(c Cell) string {
	return strings.Trim(strings.Join([]string{c.Condition, c.Time}, " · "), " ·")
}

// Finding codes of Assess.
const (
	FindingUnitsSplitByCondition = "design.units_split_by_condition"
	FindingSampleInSeveralUnits  = "design.sample_in_several_units"
)

// Assess looks for organizations that would make one preparation count as
// several independent units (pseudoreplication): units whose names differ
// only by a condition label, each observed under one condition, and one
// sample identification placed under several units. info is optional. The
// findings are evidence for review; they never change the design.
func Assess(d Design, info map[string]MeasurementInfo) []Finding {
	var out []Finding
	// Units whose names differ only by a condition label ("F-SR A" and
	// "C-SR A"), each observed under a single condition: the condition was
	// probably encoded into the unit, so one preparation became two units.
	conds := map[string][]string{}
	for _, a := range d.Assignments {
		if a.UnitID != "" && a.Condition != "" && !containsFold(conds[a.UnitID], a.Condition) {
			conds[a.UnitID] = append(conds[a.UnitID], a.Condition)
		}
	}
	var condLabels []string
	for _, c := range d.Conditions {
		condLabels = append(condLabels, c.Label)
	}
	stems := map[string][]string{}
	var stemOrder []string
	for _, u := range d.Units {
		if len(conds[u.ID]) != 1 {
			continue
		}
		stem, removed := withoutTokens(u.Label, condLabels)
		if !removed || stem == "" {
			continue
		}
		if stems[stem] == nil {
			stemOrder = append(stemOrder, stem)
		}
		stems[stem] = append(stems[stem], u.ID)
	}
	for _, stem := range stemOrder {
		if ids := stems[stem]; len(ids) > 1 {
			out = append(out, Finding{Code: FindingUnitsSplitByCondition, Units: ids, Subject: stem})
		}
	}
	// The same sample identification under different units.
	if info != nil {
		bySample := map[string][]string{}
		var sampleOrder []string
		for _, a := range d.Assignments {
			sid := strings.TrimSpace(info[a.MeasurementID].SampleID)
			if sid == "" || a.UnitID == "" || containsFold(bySample[sid], a.UnitID) {
				continue
			}
			if bySample[sid] == nil {
				sampleOrder = append(sampleOrder, sid)
			}
			bySample[sid] = append(bySample[sid], a.UnitID)
		}
		for _, sid := range sampleOrder {
			if ids := bySample[sid]; len(ids) > 1 {
				sort.Strings(ids)
				out = append(out, Finding{Code: FindingSampleInSeveralUnits, Units: ids, Subject: sid})
			}
		}
	}
	return out
}

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

// tokens splits a name into words. Hyphens stay inside a word ("F-SR").
func tokens(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !(r == '-' || r == '+' || isWordRune(r))
	})
}

func isWordRune(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 127 && !isSpaceLike(r)
}

func isSpaceLike(r rune) bool {
	switch r {
	case ' ', '–', '—', '·', '↔':
		return true
	}
	return false
}

// withoutTokens removes any of the labels (as whole word sequences, case
// insensitive) from s and reports whether one was found.
func withoutTokens(s string, labels []string) (string, bool) {
	words := tokens(s)
	removed := false
	for _, l := range labels {
		lw := tokens(l)
		if len(lw) == 0 {
			continue
		}
		for i := 0; i+len(lw) <= len(words); i++ {
			if equalWords(words[i:i+len(lw)], lw) {
				words = append(append([]string{}, words[:i]...), words[i+len(lw):]...)
				removed = true
				break
			}
		}
	}
	return strings.ToLower(strings.Join(words, " ")), removed
}

func equalWords(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}
