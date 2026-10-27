package graph

// The data behind a design graph, for exports: every reading with its
// experimental unit, condition, time point and replicate kind; each unit's
// value (the mean of its readings, decision D4); and the summary of each
// condition × time over experimental units. These are the numbers the graph
// draws, computed by the same code, in the stored units.

import (
	"math"

	"github.com/oaovito/mne_lab/internal/science/design"
)

// DesignReading is one measurement placed in the design.
type DesignReading struct {
	MeasurementID string  `json:"measurementId"`
	UnitID        string  `json:"unitId"`
	Unit          string  `json:"unit"` // experimental unit name
	Condition     string  `json:"condition"`
	Time          string  `json:"time"`
	ReplicateKind string  `json:"replicateKind"`
	Value         float64 `json:"value"`
	Raw           string  `json:"raw"`
	StoredUnit    string  `json:"storedUnit"`
}

// DesignUnitValue is one experimental unit's value in one cell.
type DesignUnitValue struct {
	UnitID     string  `json:"unitId"`
	Unit       string  `json:"unit"`
	Condition  string  `json:"condition"`
	Time       string  `json:"time"`
	Readings   int     `json:"readings"`
	Value      float64 `json:"value"`
	StoredUnit string  `json:"storedUnit"`
}

// DesignSummary describes one condition × time over experimental units.
type DesignSummary struct {
	Condition  string   `json:"condition"`
	Time       string   `json:"time"`
	N          int      `json:"n"`
	Mean       float64  `json:"mean"`
	SD         *float64 `json:"sd,omitempty"`
	SEM        *float64 `json:"sem,omitempty"`
	StoredUnit string   `json:"storedUnit"`
}

// DesignTable is the data of a design graph.
type DesignTable struct {
	Param    string            `json:"param"`
	Readings []DesignReading   `json:"readings"`
	Units    []DesignUnitValue `json:"units"`
	Summary  []DesignSummary   `json:"summary"`
}

// DesignTables returns the data a design graph draws.
func DesignTables(def Definition, in Input, din DesignInput) (DesignTable, error) {
	var res Result
	dd, err := collectDesign(def, in, din, &res)
	if err != nil {
		return DesignTable{}, err
	}
	times := dd.times
	if def.Kind == KindPairedCondition {
		at := PairedConditionAt(def, din.Design)
		times = nil
		for _, tl := range dd.times {
			if tl.Label == at {
				times = append(times, tl)
			}
		}
	}
	kinds := map[string]string{}
	for _, a := range din.Design.Assignments {
		kinds[a.MeasurementID] = a.ReplicateKind
	}
	out := DesignTable{Param: def.Param, Readings: []DesignReading{}, Units: []DesignUnitValue{}, Summary: []DesignSummary{}}
	for _, cond := range dd.conds {
		su := dd.unitOf[cond]
		for _, tl := range times {
			var values []float64
			for _, u := range dd.units {
				c := dd.cells[[3]string{u, cond, tl.Label}]
				if c == nil {
					continue
				}
				for i, id := range c.ids {
					q := in.Measurements[id].Params[def.Param]
					kind := kinds[id]
					if kind == "" {
						kind = design.ReplicateExperimental
					}
					out.Readings = append(out.Readings, DesignReading{MeasurementID: id, UnitID: u, Unit: dd.labels[u], Condition: cond, Time: tl.Label,
						ReplicateKind: kind, Value: c.values[i], Raw: q.Raw, StoredUnit: su})
				}
				v := c.unitValue()
				values = append(values, v)
				out.Units = append(out.Units, DesignUnitValue{UnitID: u, Unit: dd.labels[u], Condition: cond, Time: tl.Label, Readings: len(c.values), Value: v, StoredUnit: su})
			}
			if len(values) == 0 {
				continue
			}
			s := DesignSummary{Condition: cond, Time: tl.Label, N: len(values), StoredUnit: su}
			for _, v := range values {
				s.Mean += v
			}
			s.Mean /= float64(len(values))
			if len(values) >= 2 {
				ss := 0.0
				for _, v := range values {
					ss += (v - s.Mean) * (v - s.Mean)
				}
				sd := math.Sqrt(ss / float64(len(values)-1))
				sem := sd / math.Sqrt(float64(len(values)))
				s.SD, s.SEM = &sd, &sem
			}
			out.Summary = append(out.Summary, s)
		}
	}
	return out, nil
}
