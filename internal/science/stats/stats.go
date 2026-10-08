// Package stats computes the descriptive statistics LIGHTSCATTERING
// reports. Definitions are explicit so results can be reproduced
// independently:
//
//	mean = Σx / n
//	SD   = sqrt( Σ(x − mean)² / (n − 1) )   (sample standard deviation)
//
// Both are reported only with n ≥ 2; individual values are always kept.
package stats

import "math"

// MinN is the minimum number of observations for mean and SD.
const MinN = 2

// Summary describes a set of observations.
type Summary struct {
	N      int       `json:"n"`
	Values []float64 `json:"values"`
	Mean   *float64  `json:"mean,omitempty"`
	SD     *float64  `json:"sd,omitempty"`
	Min    *float64  `json:"min,omitempty"`
	Max    *float64  `json:"max,omitempty"`
	// Definition documents the formula applied.
	Definition string `json:"definition"`
}

// Describe summarizes values. It uses Welford's algorithm for numerical
// stability.
func Describe(values []float64) Summary {
	s := Summary{N: len(values), Values: append([]float64(nil), values...), Definition: "mean; sample SD (n−1); n≥2"}
	if len(values) == 0 {
		return s
	}
	mn, mx := values[0], values[0]
	for _, v := range values {
		mn, mx = math.Min(mn, v), math.Max(mx, v)
	}
	s.Min, s.Max = &mn, &mx
	if len(values) < MinN {
		return s
	}
	mean, m2 := 0.0, 0.0
	for i, v := range values {
		d := v - mean
		mean += d / float64(i+1)
		m2 += d * (v - mean)
	}
	sd := math.Sqrt(m2 / float64(len(values)-1))
	s.Mean, s.SD = &mean, &sd
	return s
}
