package graph

import (
	"encoding/json"
	"strings"
	"testing"
)

// Marker, line style and legend choices are optional visual fields: a saved
// choice reaches the series, and a definition without them stays as it was.
func TestOptionalSeriesStyles(t *testing.T) {
	in, ids := input(t, "synthetic-tab.txt", "synthetic-two-runs.txt")
	def := Definition{Measurements: ids, Series: map[string]SeriesStyle{ids[1]: {Marker: "diamond", LineStyle: "dotted"}}}
	r, err := Distribution(def, in)
	if err != nil {
		t.Fatal(err)
	}
	if r.Series[1].Marker != "diamond" || r.Series[1].LineStyle != "dotted" || r.Series[0].Marker != "" || r.Series[0].LineStyle != "" {
		t.Fatalf("styles: %+v %+v", r.Series[0], r.Series[1])
	}

	old := `{"schema":1,"id":"g","title":"t","kind":"dls_distribution","measurements":["a"],"series":{"a":{"color":"#2C7FB8"}},"visual":{"legend":true,"metadata":true,"grid":true,"points":false,"lineWidth":1.75,"fontScale":1},"created":"0001-01-01T00:00:00Z","updated":"0001-01-01T00:00:00Z"}`
	var d Definition
	if err := json.Unmarshal([]byte(old), &d); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(d)
	for _, k := range []string{"marker", "lineStyle", "legendMode", "legendSeries"} {
		if strings.Contains(string(b), k) {
			t.Fatalf("an older definition gained %q: %s", k, b)
		}
	}
	d.Visual.LegendMode, d.Visual.LegendSeries = "selected", []string{"a"}
	b, _ = json.Marshal(d)
	var back Definition
	if err := json.Unmarshal(b, &back); err != nil || back.Visual.LegendMode != "selected" || len(back.Visual.LegendSeries) != 1 {
		t.Fatalf("legend choice lost: %v %+v", err, back.Visual)
	}
}
