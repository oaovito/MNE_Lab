package design

import (
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Candidate is a measurement the person wants placed in a design, with the
// names it can be recognized by.
type Candidate struct {
	MeasurementID string `json:"measurementId"`
	FileName      string `json:"fileName"`
	SampleID      string `json:"sampleId,omitempty"`
	Label         string `json:"label,omitempty"`
}

// Suggestion is a proposal read from a name. It is organizational evidence
// only: nothing is applied until the person confirms it, and they can
// correct every part of it.
type Suggestion struct {
	MeasurementID string   `json:"measurementId"`
	Source        string   `json:"source"`   // label, sample or file
	Evidence      string   `json:"evidence"` // the text the proposal was read from
	Condition     string   `json:"condition,omitempty"`
	UnitID        string   `json:"unitId,omitempty"`
	UnitLabel     string   `json:"unitLabel,omitempty"`
	NewUnit       bool     `json:"newUnit,omitempty"`
	Time          string   `json:"time,omitempty"`
	Notes         []string `json:"notes,omitempty"`
}

var dayToken = regexp.MustCompile(`(?i)^(?:d|day|days|dia|dias)(\d+(?:[.,]\d+)?)$`)
var dayWord = regexp.MustCompile(`(?i)^(?:d|day|days|dia|dias)$`)
var shortUnit = regexp.MustCompile(`^[\pL][\pL\pN]{0,2}$`)

// Suggest proposes a condition, an experimental unit and a time point for
// each candidate from its label, sample identification or file name, in
// that order. It recognizes only the condition and time levels and unit
// labels the design already declares; a single short remaining word may be
// proposed as a new unit label. A name that matches more than one level is
// left without a proposal for that part.
func Suggest(d Design, cands []Candidate) []Suggestion {
	var condLabels []string
	for _, c := range d.Conditions {
		condLabels = append(condLabels, c.Label)
	}
	out := []Suggestion{}
	for _, c := range cands {
		best := Suggestion{MeasurementID: c.MeasurementID}
		for _, src := range []struct{ name, text string }{{"label", c.Label}, {"sample", c.SampleID}, {"file", strings.TrimSuffix(c.FileName, path.Ext(c.FileName))}} {
			if strings.TrimSpace(src.text) == "" {
				continue
			}
			s := suggestOne(d, condLabels, src.text)
			s.MeasurementID, s.Source, s.Evidence = c.MeasurementID, src.name, src.text
			if score(s) > score(best) || (score(best) == 0 && len(best.Notes) == 0 && len(s.Notes) > 0) {
				best = s
			}
		}
		if score(best) > 0 || len(best.Notes) > 0 {
			out = append(out, best)
		}
	}
	return out
}

func score(s Suggestion) int {
	n := 0
	if s.Condition != "" {
		n += 4
	}
	if s.UnitID != "" || s.UnitLabel != "" {
		n += 2
	}
	if s.Time != "" {
		n++
	}
	return n
}

func suggestOne(d Design, condLabels []string, text string) Suggestion {
	var s Suggestion
	words := tokens(text)
	// Time: "day 7", "d7", "D14" or a declared label, matched to the
	// declared levels only.
	if len(d.Times) > 0 {
		var hits []string
		var used [][2]int
		for i, w := range words {
			num := ""
			span := [2]int{i, i + 1}
			if m := dayToken.FindStringSubmatch(w); m != nil {
				num = m[1]
			} else if dayWord.MatchString(w) && i+1 < len(words) {
				if _, err := strconv.ParseFloat(strings.Replace(words[i+1], ",", ".", 1), 64); err == nil {
					num, span = words[i+1], [2]int{i, i + 2}
				}
			}
			if num == "" {
				continue
			}
			v, _ := strconv.ParseFloat(strings.Replace(num, ",", ".", 1), 64)
			for _, tl := range d.Times {
				if tl.Offset != nil && *tl.Offset == v && (tl.Unit == "days" || tl.Unit == "") && !contains(hits, tl.Label) {
					hits = append(hits, tl.Label)
					used = append(used, span)
				}
			}
		}
		for _, tl := range d.Times {
			lw := tokens(tl.Label)
			for i := 0; len(lw) > 0 && i+len(lw) <= len(words); i++ {
				if equalWords(words[i:i+len(lw)], lw) && !contains(hits, tl.Label) {
					hits = append(hits, tl.Label)
					used = append(used, [2]int{i, i + len(lw)})
				}
			}
		}
		switch {
		case len(hits) == 1:
			s.Time = hits[0]
			words = cut(words, used)
		case len(hits) > 1:
			s.Notes = append(s.Notes, "design.suggest_ambiguous_time")
		}
	}
	// Condition: a declared label as a whole word sequence.
	var condHits []string
	var condSpans [][2]int
	for _, l := range condLabels {
		lw := tokens(l)
		for i := 0; len(lw) > 0 && i+len(lw) <= len(words); i++ {
			if equalWords(words[i:i+len(lw)], lw) {
				condHits = append(condHits, l)
				condSpans = append(condSpans, [2]int{i, i + len(lw)})
				break
			}
		}
	}
	switch {
	case len(condHits) == 1:
		s.Condition = condHits[0]
		words = cut(words, condSpans)
	case len(condHits) > 1:
		s.Notes = append(s.Notes, "design.suggest_ambiguous_condition")
	}
	// Unit: a declared unit label as a whole word sequence, or a single
	// short remaining word as a new label.
	var unitHits []Unit
	for _, u := range d.Units {
		lw := tokens(u.Label)
		for i := 0; len(lw) > 0 && i+len(lw) <= len(words); i++ {
			if equalWords(words[i:i+len(lw)], lw) {
				unitHits = append(unitHits, u)
				break
			}
		}
	}
	switch {
	case len(unitHits) == 1:
		s.UnitID, s.UnitLabel = unitHits[0].ID, unitHits[0].Label
	case len(unitHits) > 1:
		s.Notes = append(s.Notes, "design.suggest_ambiguous_unit")
	case len(words) == 1 && shortUnit.MatchString(words[0]):
		s.UnitLabel, s.NewUnit = words[0], true
	}
	return s
}

func cut(words []string, spans [][2]int) []string {
	drop := map[int]bool{}
	for _, sp := range spans {
		for i := sp[0]; i < sp[1]; i++ {
			drop[i] = true
		}
	}
	var out []string
	for i, w := range words {
		if !drop[i] {
			out = append(out, w)
		}
	}
	return out
}
