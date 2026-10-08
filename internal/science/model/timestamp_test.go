package model

import (
	"testing"
	"time"
)

func TestTimestampISOTimePreservesClockAndPrecision(t *testing.T) {
	for _, tc := range []struct {
		name, value, want string
		known             bool
	}{
		{"wall", "2026-02-16T09:00:00Z", "2026-02-16T09:00:00", false},
		{"wall-fraction", "2026-02-16T09:00:00.125Z", "2026-02-16T09:00:00.125", false},
		{"offset", "2026-02-16T09:00:00.125-03:00", "2026-02-16T09:00:00.125-03:00", true},
		{"utc", "2026-02-16T12:00:00Z", "2026-02-16T12:00:00Z", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339Nano, tc.value)
			if err != nil {
				t.Fatal(err)
			}
			ts := Timestamp{Time: at, Raw: tc.value, TZKnown: tc.known}
			if got := ts.ISOTime(); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if ts.Raw != tc.value || ts.TZKnown != tc.known || ts.Time != at {
				t.Fatal("source changed")
			}
		})
	}
}
