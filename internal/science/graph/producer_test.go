package graph

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/science/model"
)

func TestProducerOriginSnapshotsNeverInferOrAlias(t *testing.T) {
	in, ids := input(t, "synthetic-count-rate-types.txt")
	m := in.Measurements[ids[0]]
	unknown := in.Source(m).ProducerOrigin
	if unknown == nil || unknown.Kind != "unknown" || unknown.Declaration != "unknown" || unknown.Vendor != "" {
		t.Fatal("parser recognition inferred producer")
	}
	at := time.Now().UTC()
	expectedAt := at
	f := in.Files[m.FileID]
	f.ProducerOrigin = &model.ProducerOrigin{Kind: "instrument_export", Declaration: "user_declared", Vendor: "Invented manufacturer", SoftwareVersion: "test-v1", DeclaredAt: &at}
	in.Files[m.FileID] = f
	r, err := Distribution(Definition{Measurements: ids}, in)
	if err != nil {
		t.Fatal(err)
	}
	copy := r.Provenance.Sources[0].ProducerOrigin
	if !reflect.DeepEqual(copy, f.ProducerOrigin) || copy == f.ProducerOrigin || copy.DeclaredAt == f.ProducerOrigin.DeclaredAt {
		t.Fatal("provenance missing or aliases declaration")
	}
	data, err := json.Marshal(copy)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip model.ProducerOrigin
	if json.Unmarshal(data, &roundtrip) != nil || !reflect.DeepEqual(&roundtrip, copy) {
		t.Fatal("declaration JSON changed")
	}
	f.ProducerOrigin.Vendor = "later edit"
	*f.ProducerOrigin.DeclaredAt = at.Add(time.Hour)
	if copy.Vendor != "Invented manufacturer" || !copy.DeclaredAt.Equal(expectedAt) {
		t.Fatal("old graph changed after source edit")
	}
}
