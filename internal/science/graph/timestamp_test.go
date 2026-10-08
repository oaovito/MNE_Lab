package graph

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/science/model"
)

func TestGraphTimestampProvenanceKeepsSourceSemantics(t *testing.T) {
	for _, known := range []bool{false, true} {
		in, ids := input(t, "synthetic-count-rate-types.txt")
		m := in.Measurements[ids[0]]
		at := time.Date(2026, 2, 16, 9, 0, 0, 125000000, time.UTC)
		if known {
			at = at.In(time.FixedZone("synthetic offset", -3*3600))
		}
		m.MeasuredAt = &model.Timestamp{Time: at, Raw: "original synthetic date", TZKnown: known, Ambiguous: true, Confirmed: true, Source: "file"}
		in.Measurements[m.ID] = m
		r, err := Distribution(Definition{Measurements: ids}, in)
		if err != nil {
			t.Fatal(err)
		}
		s := r.Provenance.Sources[0]
		if s.MeasuredAt != m.MeasuredAt.ISOTime() || s.MeasuredTimestamp == nil || *s.MeasuredTimestamp != *m.MeasuredAt || s.MeasuredTimestamp == m.MeasuredAt {
			t.Fatal("time semantics were dropped or source pointer aliased")
		}
		var encoded struct {
			Sources []Source `json:"sources"`
		}
		b, err := json.Marshal(r.Provenance)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &encoded); err != nil {
			t.Fatal(err)
		}
		ts := encoded.Sources[0].MeasuredTimestamp
		if ts == nil || ts.TZKnown != known || !ts.Ambiguous || !ts.Confirmed || ts.Raw != m.MeasuredAt.Raw || !ts.Time.Equal(at) {
			t.Fatal("JSON lost original flags/time")
		}
		if r.Series[0].Meta["measuredAt"] != s.MeasuredAt {
			t.Fatal("metadata and provenance disagree")
		}
		m.MeasuredAt.Raw = "later edit"
		if s.MeasuredTimestamp.Raw != "original synthetic date" {
			t.Fatal("saved result changed after source edit")
		}
	}
	if s := (Input{}).Source(model.Measurement{}); s.MeasuredTimestamp != nil || s.MeasuredAt != "" {
		t.Fatal("missing timestamp invented")
	}
}
