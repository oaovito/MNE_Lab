package cycle

import (
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/science/model"
)

func ts(s string) *model.Timestamp {
	t, _ := time.Parse("2006-01-02 15:04", s)
	return &model.Timestamp{Time: t, Source: "file"}
}

func TestLimitsAllUnits(t *testing.T) {
	start := time.Date(2026, 1, 31, 9, 0, 0, 0, time.UTC)
	for _, u := range Units {
		for _, n := range []int{1, 99} {
			c := Config{Name: "c", Start: start, Interval: 1, Unit: u, Duration: n}
			ps, err := c.Points()
			if err != nil || len(ps) != n+1 {
				t.Fatalf("%s %d: %v %d", u, n, err, len(ps))
			}
		}
		for _, bad := range []Config{
			{Name: "c", Unit: u, Interval: 0, Duration: 5},
			{Name: "c", Unit: u, Interval: 1, Duration: 100},
			{Name: "c", Unit: u, Interval: 6, Duration: 5},
		} {
			if bad.Validate() == nil {
				t.Fatalf("invalid config accepted: %+v", bad)
			}
		}
	}
	if (Config{Name: "c", Unit: "minutes", Interval: 1, Duration: 1}).Validate() != ErrUnit {
		t.Fatal("unknown unit accepted")
	}
}

func TestCalendarSemantics(t *testing.T) {
	jan31 := time.Date(2026, 1, 31, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		u    Unit
		n    int
		want string
	}{
		{Months, 1, "2026-02-28 10:00"},
		{Months, 2, "2026-03-31 10:00"},
		{Months, 13, "2027-02-28 10:00"},
		{Years, 2, "2028-01-31 10:00"},
		{Hours, 99, "2026-02-04 13:00"},
		{Weeks, 4, "2026-02-28 10:00"},
		{Days, 1, "2026-02-01 10:00"},
	}
	for _, c := range cases {
		if got := Add(jan31, c.u, c.n).Format("2006-01-02 15:04"); got != c.want {
			t.Fatalf("%d %s: %s want %s", c.n, c.u, got, c.want)
		}
	}
	feb29 := time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)
	if got := Add(feb29, Years, 1).Format("2006-01-02"); got != "2029-02-28" {
		t.Fatalf("leap day + 1 year: %s", got)
	}
}

func TestStabilityStudyAssociation(t *testing.T) {
	start, _ := time.Parse("2006-01-02 15:04", "2026-10-05 09:00")
	c := Config{Name: "A1 stability", SampleID: "A1", Start: start, Interval: 1, Unit: Weeks, Duration: 4, Replicates: 3}
	ps, _ := c.Points()
	if len(ps) != 5 {
		t.Fatalf("Week 0..4 expected, got %d", len(ps))
	}
	cands := []Candidate{
		{ID: "w0r1", SampleID: "A1", MeasuredAt: ts("2026-10-05 09:10")},
		{ID: "w0r2", SampleID: "A1", MeasuredAt: ts("2026-10-05 09:14")},
		{ID: "w0r3", SampleID: "A1", MeasuredAt: ts("2026-10-05 09:18")},
		{ID: "w1", SampleID: "A1", MeasuredAt: ts("2026-10-13 15:00")}, // 1 day late: still week 1
		{ID: "other", SampleID: "B2", MeasuredAt: ts("2026-10-19 09:00")},
		{ID: "nodate", SampleID: "A1"},
		{ID: "amb", SampleID: "A1", MeasuredAt: &model.Timestamp{Time: start, Ambiguous: true}},
	}
	as := Associate(c, ps, cands)
	want := map[string]struct {
		point  int
		status string
	}{
		"w0r1": {0, "auto"}, "w0r2": {0, "auto"}, "w0r3": {0, "auto"}, "w1": {1, "auto"},
		"other": {2, "needs_confirmation"}, "nodate": {-1, "unassigned"}, "amb": {-1, "needs_confirmation"},
	}
	for _, a := range as {
		w := want[a.MeasurementID]
		if a.Point != w.point || a.Status != w.status {
			t.Fatalf("%s: got %d/%s want %d/%s (%s)", a.MeasurementID, a.Point, a.Status, w.point, w.status, a.Reason)
		}
	}
	vals := map[string]float64{"w0r1": 240, "w0r2": 245, "w0r3": 250, "w1": 251, "other": 999}
	series := Series(ps, as, vals)
	if series[0].Summary.N != 3 || *series[0].Summary.Mean != 245 || *series[0].Summary.SD != 5 {
		t.Fatalf("week 0: %+v", series[0].Summary)
	}
	if series[1].Summary.N != 1 || series[1].Summary.Mean != nil {
		t.Fatal("single replicate must not produce a mean")
	}
	if !series[2].Missing || !series[3].Missing || !series[4].Missing {
		t.Fatal("points without accepted measurements must be gaps")
	}
}
