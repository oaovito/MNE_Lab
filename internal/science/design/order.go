package design

// Order of time levels. Time points are put in the order of their numeric
// offset in the design (day 0, 7, 14, 21), never in the byte order of their
// labels ("0", "14", "21", "7"). A label with no offset in the design is
// placed by the number it starts with, when it starts with one; labels
// without any number come after the numeric ones, in byte order, and byte
// order also breaks ties. The order only arranges levels for display,
// exports and the statistics engine; no value is derived from it.

import (
	"regexp"
	"sort"
	"strconv"
)

var leadingNumber = regexp.MustCompile(`^\s*([-+]?(?:\d+(?:\.\d*)?|\.\d+))`)

// timeKeys returns the numeric position of each label: the design offset
// when every offset of the design is stated in one unit, otherwise the
// number the label starts with.
func timeKeys(labels []string, times []TimeLevel) map[string]float64 {
	offsets := map[string]float64{}
	unit, same := "", true
	first := true
	for _, tl := range times {
		if tl.Offset == nil {
			continue
		}
		if first {
			unit, first = tl.Unit, false
		} else if tl.Unit != unit {
			same = false
		}
		offsets[tl.Label] = *tl.Offset
	}
	out := map[string]float64{}
	for _, l := range labels {
		if v, ok := offsets[l]; ok && same {
			out[l] = v
			continue
		}
		if m := leadingNumber.FindStringSubmatch(l); m != nil {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				out[l] = v
			}
		}
	}
	return out
}

// OrderTimeLabels returns the labels in time order (see the package
// comment of this file); times are the design's declared levels, if any.
func OrderTimeLabels(labels []string, times []TimeLevel) []string {
	out := append([]string(nil), labels...)
	keys := timeKeys(out, times)
	sort.SliceStable(out, func(i, j int) bool {
		a, aok := keys[out[i]]
		b, bok := keys[out[j]]
		switch {
		case aok && bok && a != b:
			return a < b
		case aok != bok:
			return aok
		}
		return out[i] < out[j]
	})
	return out
}

// OrderedTimes returns the design's time levels in time order.
func (d Design) OrderedTimes() []TimeLevel {
	var labels []string
	byLabel := map[string]TimeLevel{}
	for _, tl := range d.Times {
		labels = append(labels, tl.Label)
		byLabel[tl.Label] = tl
	}
	var out []TimeLevel
	for _, l := range OrderTimeLabels(labels, d.Times) {
		out = append(out, byLabel[l])
	}
	return out
}

// OrderConditionLabels returns the labels in the design's declared order of
// conditions, the others after them in byte order.
func OrderConditionLabels(labels []string, conds []Condition) []string {
	rank := map[string]int{}
	for i, c := range conds {
		rank[c.Label] = i + 1
	}
	out := append([]string(nil), labels...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := rank[out[i]], rank[out[j]]
		switch {
		case a != 0 && b != 0:
			return a < b
		case (a != 0) != (b != 0):
			return a != 0
		}
		return out[i] < out[j]
	})
	return out
}
