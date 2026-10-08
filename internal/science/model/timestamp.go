package model

import "time"

// ISOTime formats the reported time without inventing a zone. A timestamp
// with TZKnown=false carries a wall clock, even when Time uses UTC as storage.
// Fractional seconds are retained when present in the parsed value.
func (ts Timestamp) ISOTime() string {
	if ts.TZKnown {
		return ts.Time.Format(time.RFC3339Nano)
	}
	return ts.Time.Format("2006-01-02T15:04:05.999999999")
}
