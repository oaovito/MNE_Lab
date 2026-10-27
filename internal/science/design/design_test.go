package design

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func f(v float64) *float64 { return &v }

// paired builds the synthetic owner design: units A, B, C, each measured
// under F-SR and its centrifuged C-SR aliquot at days 0, 7, 14 and 21.
// Every identifier is invented.
func paired() Design {
	d := Design{Name: "Synthetic paired design", Module: "lightscattering",
		Units:      []Unit{{ID: "u-a", Label: "A"}, {ID: "u-b", Label: "B"}, {ID: "u-c", Label: "C"}},
		Conditions: []Condition{{Label: "F-SR"}, {Label: "C-SR", DerivedFrom: "F-SR", Process: "centrifugation"}},
		Times:      []TimeLevel{{Label: "Day 0", Offset: f(0), Unit: "days"}, {Label: "Day 7", Offset: f(7), Unit: "days"}, {Label: "Day 14", Offset: f(14), Unit: "days"}, {Label: "Day 21", Offset: f(21), Unit: "days"}}}
	for _, u := range d.Units {
		for _, c := range d.Conditions {
			for _, t := range d.Times {
				d.Assignments = append(d.Assignments, Assignment{MeasurementID: fmt.Sprintf("m-%s-%s-%s", u.Label, c.Label, t.Label), UnitID: u.ID, Condition: c.Label, Time: t.Label})
			}
		}
	}
	d.Pairs = DerivedPairs(d)
	return Normalize(d)
}

// B§60: 24 measurements are 3 independent experimental units.
func TestPairedDesignHasThreeUnitsNotTwentyFour(t *testing.T) {
	d := paired()
	if err := Validate(d); err != nil {
		t.Fatal(err)
	}
	s := Detect(d, nil)
	if s.Measurements != 24 || s.ExperimentalUnits != 3 || s.Observations != 24 || s.TechnicalReadings != 0 {
		t.Fatalf("measurements %d, units %d, observations %d, technical %d", s.Measurements, s.ExperimentalUnits, s.Observations, s.TechnicalReadings)
	}
	if s.ExperimentalUnits == s.Measurements {
		t.Fatal("measurements counted as experimental units")
	}
	if !s.ConditionWithin || !s.TimeWithin || !s.Complete || s.Detected != StructurePairedLongitudinal || s.Recommended != RecommendConditionTime {
		t.Fatalf("structure %+v", s)
	}
	if len(s.Pairs) != 3 {
		t.Fatalf("pairs %+v", s.Pairs)
	}
	for i, u := range []string{"u-a", "u-b", "u-c"} {
		p := s.Pairs[i]
		if p.UnitID != u || p.From != "F-SR" || p.To != "C-SR" || !p.Complete || len(p.Times) != 4 {
			t.Fatalf("pair %d: %+v", i, p)
		}
	}
	if len(s.Findings) != 0 {
		t.Fatalf("findings on a clean design: %+v", s.Findings)
	}
}

// B§62: twelve repeated reads of unit A are one experimental unit.
func TestTechnicalReadingsDoNotAddUnits(t *testing.T) {
	d := Design{Name: "Synthetic technical reads", Units: []Unit{{ID: "u-a", Label: "A"}}, Conditions: []Condition{{Label: "F-SR"}}, Times: []TimeLevel{{Label: "Day 7"}}}
	for i := 0; i < 12; i++ {
		kind := ReplicateTechnical
		if i == 0 {
			kind = ReplicateExperimental
		}
		d.Assignments = append(d.Assignments, Assignment{MeasurementID: fmt.Sprintf("read-%d", i), UnitID: "u-a", Condition: "F-SR", Time: "Day 7", ReplicateKind: kind})
	}
	d = Normalize(d)
	if err := Validate(d); err != nil {
		t.Fatal(err)
	}
	s := Detect(d, nil)
	if s.ExperimentalUnits != 1 || s.Observations != 1 || s.TechnicalReadings != 11 || s.Measurements != 12 {
		t.Fatalf("%+v", s)
	}
	// Marking all twelve as experimental does not create units either; it
	// is reported for review.
	for i := range d.Assignments {
		d.Assignments[i].ReplicateKind = ReplicateExperimental
	}
	s = Detect(d, nil)
	if s.ExperimentalUnits != 1 || len(s.Findings) != 1 || s.Findings[0].Code != "design.cell_several_experimental" {
		t.Fatalf("%+v", s)
	}
}

func TestConditionEncodedIntoUnitsIsReported(t *testing.T) {
	d := Design{Name: "Synthetic split", Conditions: []Condition{{Label: "F-SR"}, {Label: "C-SR", DerivedFrom: "F-SR"}},
		Units: []Unit{{ID: "u1", Label: "F-SR A"}, {ID: "u2", Label: "C-SR A"}, {ID: "u3", Label: "F-SR B"}, {ID: "u4", Label: "C-SR B"}}}
	for i, u := range d.Units {
		cond := "F-SR"
		if i%2 == 1 {
			cond = "C-SR"
		}
		d.Assignments = append(d.Assignments, Assignment{MeasurementID: "m" + u.ID, UnitID: u.ID, Condition: cond})
	}
	d = Normalize(d)
	s := Detect(d, map[string]MeasurementInfo{"mu1": {SampleID: "S-1"}, "mu3": {SampleID: "S-1"}})
	var codes []string
	for _, f := range s.Findings {
		codes = append(codes, f.Code)
	}
	if !reflect.DeepEqual(codes, []string{"design.units_split_by_condition", "design.units_split_by_condition", "design.sample_in_several_units"}) {
		t.Fatalf("%v %+v", codes, s.Findings)
	}
	if !reflect.DeepEqual(s.Findings[0].Units, []string{"u1", "u2"}) || s.ExperimentalUnits != 4 || s.Detected != StructureIndependent {
		t.Fatalf("%+v", s)
	}
}

func TestValidateRefusesInconsistentDesigns(t *testing.T) {
	cases := map[string]func(*Design){
		"no name":            func(d *Design) { d.Name = "" },
		"unknown unit":       func(d *Design) { d.Assignments[0].UnitID = "u-x" },
		"unknown condition":  func(d *Design) { d.Assignments[0].Condition = "f-sr" },
		"unknown time":       func(d *Design) { d.Assignments[0].Time = "Day 99" },
		"duplicate":          func(d *Design) { d.Assignments[1].MeasurementID = d.Assignments[0].MeasurementID },
		"replicate kind":     func(d *Design) { d.Assignments[0].ReplicateKind = "reading" },
		"derivation chain":   func(d *Design) { d.Conditions = append(d.Conditions, Condition{Label: "X", DerivedFrom: "C-SR"}) },
		"derivation itself":  func(d *Design) { d.Conditions[0].DerivedFrom = "F-SR" },
		"pair across units":  func(d *Design) { d.Pairs = append(d.Pairs, Pair{UnitID: "u-x", From: "F-SR", To: "C-SR"}) },
		"pair one condition": func(d *Design) { d.Pairs[0].To = "F-SR" },
		"pair repeated":      func(d *Design) { d.Pairs = append(d.Pairs, Pair{UnitID: "u-a", From: "C-SR", To: "F-SR"}) },
		"duplicate level":    func(d *Design) { d.Times = append(d.Times, TimeLevel{Label: "day 0"}) },
		"negative time":      func(d *Design) { d.Times[0].Offset = f(-1) },
		"time unit":          func(d *Design) { d.Times[0].Unit = "fortnights" },
		"source":             func(d *Design) { d.Source = "import" },
	}
	for name, edit := range cases {
		d := paired()
		edit(&d)
		if err := Validate(d); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestRevisionsKeepFieldLevelHistory(t *testing.T) {
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	first, err := Revise(nil, paired(), "rev-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Root != "rev-1" || first.Revision != 1 || first.Version != Version || len(first.Changes) == 0 {
		t.Fatalf("%+v", first)
	}
	draft := first
	draft.Assignments = append([]Assignment{}, first.Assignments...)
	draft.Assignments[0].Time, draft.Assignments[0].ReplicateKind = "Day 7", ReplicateTechnical
	draft.Reason = "Corrected a misread label"
	second, err := Revise(&first, draft, "rev-2", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{{Field: "time", MeasurementID: first.Assignments[0].MeasurementID, From: "Day 0", To: "Day 7"}, {Field: "replicate_kind", MeasurementID: first.Assignments[0].MeasurementID, From: ReplicateExperimental, To: ReplicateTechnical}}
	if second.Root != "rev-1" || second.PreviousID != "rev-1" || second.Revision != 2 || !reflect.DeepEqual(second.Changes, want) || second.Reason != "Corrected a misread label" {
		t.Fatalf("%+v", second)
	}
	if first.Assignments[0].Time != "Day 0" {
		t.Fatal("the earlier revision changed")
	}
	all := []Design{first, second}
	if h := Heads(all); len(h) != 1 || h[0].ID != "rev-2" {
		t.Fatalf("heads %+v", h)
	}
	if h := History(all, "rev-2"); len(h) != 2 || h[0].ID != "rev-2" || h[1].ID != "rev-1" {
		t.Fatalf("history %+v", h)
	}
	// Two edits of the same revision stay two visible heads.
	other, _ := Revise(&first, first, "rev-3", now.Add(2*time.Minute))
	if h := Heads(append(all, other)); len(h) != 2 {
		t.Fatalf("diverged heads %+v", h)
	}
	if len(other.Changes) != 0 {
		t.Fatalf("unchanged revision lists changes: %+v", other.Changes)
	}
}

func TestSuggestionIsAProposalFromNames(t *testing.T) {
	d := paired()
	cands := []Candidate{
		{MeasurementID: "1", FileName: "F-SR A day 0.txt"},
		{MeasurementID: "2", FileName: "C-SR_B_d14.txt"},
		{MeasurementID: "3", FileName: "export.txt", SampleID: "c-sr C D21"},
		{MeasurementID: "4", FileName: "F-SR vs C-SR.txt"},
		{MeasurementID: "5", FileName: "notes.txt"},
		{MeasurementID: "6", FileName: "F-SR D day 7.txt"},
	}
	before := fmt.Sprintf("%+v", d)
	got := Suggest(d, cands)
	if fmt.Sprintf("%+v", d) != before {
		t.Fatal("suggesting changed the design")
	}
	byID := map[string]Suggestion{}
	for _, s := range got {
		byID[s.MeasurementID] = s
	}
	if s := byID["1"]; s.Condition != "F-SR" || s.UnitID != "u-a" || s.Time != "Day 0" || s.Source != "file" || s.Evidence != "F-SR A day 0" {
		t.Fatalf("1: %+v", s)
	}
	if s := byID["2"]; s.Condition != "C-SR" || s.UnitID != "u-b" || s.Time != "Day 14" {
		t.Fatalf("2: %+v", s)
	}
	if s := byID["3"]; s.Condition != "C-SR" || s.UnitID != "u-c" || s.Time != "Day 21" || s.Source != "sample" {
		t.Fatalf("3: %+v", s)
	}
	if s := byID["4"]; s.Condition != "" || len(s.Notes) != 1 || s.Notes[0] != "design.suggest_ambiguous_condition" {
		t.Fatalf("4: %+v", s)
	}
	if _, ok := byID["5"]; ok {
		t.Fatal("a proposal without evidence")
	}
	if s := byID["6"]; !s.NewUnit || s.UnitLabel != "D" || s.UnitID != "" || s.Time != "Day 7" {
		t.Fatalf("6: %+v", s)
	}
}
