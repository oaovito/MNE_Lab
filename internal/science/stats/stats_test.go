package stats

import (
	"math"
	"testing"
)

func TestDescribe(t *testing.T) {
	// Reference values computed by hand: mean 2, sample variance 1.
	s := Describe([]float64{1, 2, 3})
	if s.N != 3 || *s.Mean != 2 || math.Abs(*s.SD-1) > 1e-12 {
		t.Fatalf("%+v", s)
	}
	one := Describe([]float64{245.3})
	if one.Mean != nil || one.SD != nil || len(one.Values) != 1 {
		t.Fatal("one observation must not yield mean/SD")
	}
	if e := Describe(nil); e.N != 0 || e.Min != nil {
		t.Fatal("empty")
	}
	// Large offsets stay accurate (Welford).
	big := Describe([]float64{1e9 + 4, 1e9 + 7, 1e9 + 13, 1e9 + 16})
	if math.Abs(*big.SD-math.Sqrt(30)) > 1e-6 {
		t.Fatalf("sd %v", *big.SD)
	}
}
