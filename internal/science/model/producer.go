package model

import "time"

// ProducerOrigin records the person's declaration about who created a file.
// It is independent of import/sync/package transport and parser recognition.
// No declaration establishes authenticity or scientific validity.
type ProducerOrigin struct {
	Kind            string     `json:"kind"`        // unknown, instrument_export, user_created_table
	Declaration     string     `json:"declaration"` // unknown or user_declared; never verified
	Vendor          string     `json:"vendor,omitempty"`
	Instrument      string     `json:"instrument,omitempty"`
	Software        string     `json:"software,omitempty"`
	SoftwareVersion string     `json:"softwareVersion,omitempty"`
	Evidence        string     `json:"evidence,omitempty"` // person's source/reference note, not fetched
	DeclaredAt      *time.Time `json:"declaredAt,omitempty"`
}

// CloneProducerOrigin makes an independent snapshot. Legacy or absent metadata
// remains explicitly unknown; neither extension nor parsed fields fill it in.
func CloneProducerOrigin(origin *ProducerOrigin) *ProducerOrigin {
	if origin == nil {
		return &ProducerOrigin{Kind: "unknown", Declaration: "unknown"}
	}
	copy := *origin
	if origin.DeclaredAt != nil {
		at := *origin.DeclaredAt
		copy.DeclaredAt = &at
	}
	return &copy
}
