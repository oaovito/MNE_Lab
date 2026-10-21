package plot

import (
	"testing"

	"github.com/oaovito/mne_lab/internal/science/graph"
)

func TestAxisLabelSaysWhenTheFileStatesNoUnit(t *testing.T) {
	T := func(key string, kv ...string) string {
		return map[string]string{"param.effective_diameter": "Effective Diameter", "param.polydispersity": "Polydispersity",
			"plot.unit_not_stated": "unit not stated in the file"}[key]
	}
	for _, c := range []struct {
		axis graph.Axis
		want string
	}{
		{graph.Axis{Label: "param.effective_diameter", Unit: "nm"}, "Effective Diameter (nm)"},
		{graph.Axis{Label: "param.effective_diameter"}, "Effective Diameter (unit not stated in the file)"},
		{graph.Axis{Label: "param.polydispersity"}, "Polydispersity"},
	} {
		if got := AxisLabel(c.axis, T); got != c.want {
			t.Fatalf("%+v: %q, want %q", c.axis, got, c.want)
		}
	}
}
