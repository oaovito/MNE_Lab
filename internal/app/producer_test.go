package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/export"
	"github.com/oaovito/mne_lab/internal/plot"
	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
)

func TestProducerOriginIsIndependentScopedAndPreservesAnalysis(t *testing.T) {
	_, c, p := inspectionProfile(t)
	def := statisticalFixture(t, p)
	prepared, err := p.PrepareAnalysis(def)
	if err != nil {
		t.Fatal(err)
	}
	results := statisticalContract(prepared)
	saved, err := p.SaveAnalysis(def, prepared.Receipt, results, "")
	if err != nil {
		t.Fatal(err)
	}
	id := prepared.Sources[0].File.ID
	before, original, err := p.Original(id)
	if err != nil || before.ProducerOrigin != nil {
		t.Fatal("parser inferred producer", err)
	}
	headers := map[string]string{"X-Account-ID": p.acct.ID(), "X-Profile-ID": p.Entry.ID}
	declaration := ProducerDeclaration{Kind: "user_created_table", Software: "Invented table editor", SoftwareVersion: "synthetic-v1", Evidence: "Invented test reference"}
	path := "/api/files/" + id + "/producer-origin"
	if code, _ := c.do("PUT", path, declaration, nil); code < 400 {
		t.Fatal("unscoped declaration accepted")
	}
	var after model.SourceFile
	code, body := c.do("PUT", path, declaration, headers)
	if code != 200 || json.Unmarshal(body, &after) != nil || after.ProducerOrigin == nil {
		t.Fatalf("declaration failed: %d", code)
	}
	o := after.ProducerOrigin
	if o.Declaration != "user_declared" || o.Kind != declaration.Kind || o.DeclaredAt == nil || o.DeclaredAt.IsZero() {
		t.Fatal("declaration gained verification or lost audit time")
	}
	_, unchanged, err := p.Original(id)
	if err != nil || !bytes.Equal(original, unchanged) || after.SHA256 != before.SHA256 || after.Origin != before.Origin || after.Parser != before.Parser || after.Spec != before.Spec || after.Status != before.Status || !reflect.DeepEqual(after.SourceInfo, before.SourceInfo) {
		t.Fatal("producer edited source or parser/scientific status", err)
	}
	if _, err := p.SaveAnalysis(def, prepared.Receipt, results, ""); !errors.Is(err, analysis.ErrSource) {
		t.Fatal("changed producer accepted stale statistics receipt", err)
	}
	got, err := p.Analysis(saved.ID)
	if err != nil || !got.SourceChanged || got.Snapshot.Sources[0].File.ProducerOrigin != nil || !reflect.DeepEqual(got.Results, saved.Results) {
		t.Fatal("declaration rewrote saved scientific snapshot", err)
	}
	fresh, err := p.PrepareAnalysis(def)
	if err != nil || fresh.Sources[0].File.ProducerOrigin.Software != declaration.Software || fresh.SourceHash == prepared.SourceHash {
		t.Fatal("fresh snapshot lacks declaration", err)
	}
	var other ProfileView
	c.json("POST", "/api/profiles", map[string]any{"username": "Separate producer test", "storageMode": "usb_only"}, &other)
	c.json("POST", "/api/profiles/"+other.ID+"/open", map[string]any{}, nil)
	if code, _ := c.do("PUT", path, declaration, headers); code < 400 {
		t.Fatal("stale profile declaration accepted")
	}
	headers["X-Profile-ID"] = other.ID
	if code, _ := c.do("PUT", path, declaration, headers); code < 400 {
		t.Fatal("foreign source edited")
	}
	c.json("POST", "/api/profiles/"+p.Entry.ID+"/open", map[string]any{}, nil)
	var reopened struct {
		File model.SourceFile `json:"file"`
	}
	c.json("GET", "/api/files/"+id, nil, &reopened)
	if !reflect.DeepEqual(reopened.File.ProducerOrigin, o) {
		t.Fatal("encrypted close/reopen lost declaration")
	}
}

func TestProducerOriginResearchPackageUsesSharedSourceSnapshot(t *testing.T) {
	_, _, p := inspectionProfile(t)
	imp := p.Import("invented-package-source.txt", []byte("Sample ID: Invented\nEffective Diameter (nm): 12.00\n"), false)
	if imp.Error != "" {
		t.Fatal(imp)
	}
	declared, err := p.DeclareProducerOrigin(imp.FileID, ProducerDeclaration{Kind: "instrument_export", Software: "Invented exporter", SoftwareVersion: "synthetic-v1"})
	if err != nil {
		t.Fatal(err)
	}
	in, err := p.Input(declared.Measurements)
	if err != nil {
		t.Fatal(err)
	}
	cy := CycleDoc{ID: "invented-cycle", Measurements: declared.Measurements}
	// Figures have separate renderer tests; inspect the actual assembled metadata
	// and normalized dataset without introducing an unrelated plotted reference.
	outputs, _, err := p.researchPackage(cy, in, graph.CycleInput{}, "invented-research", func([]string, plot.Spec, string) ([]output, error) { return nil, nil }, export.DataOptions{})
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, out := range outputs {
		if out.name == "provenance/provenance.json" || out.name == "normalized-data/measurements.json" {
			var decoded struct {
				Sources    []graph.Source `json:"sources"`
				Provenance []graph.Source `json:"provenance"`
			}
			if json.Unmarshal(out.data, &decoded) != nil {
				t.Fatal("invalid package provenance JSON")
			}
			sources := append(decoded.Sources, decoded.Provenance...)
			if len(sources) != 1 || !reflect.DeepEqual(sources[0].ProducerOrigin, declared.ProducerOrigin) || sources[0].SHA256 != declared.SHA256 {
				t.Fatal("research/Cycle export lost producer source snapshot")
			}
			found++
		}
	}
	if found != 2 {
		t.Fatal("missing independent package and Cycle dataset provenance")
	}
}

func TestProducerOriginRejectsClaimsAndInvalidPayloadWithoutPartialWrites(t *testing.T) {
	_, c, p := inspectionProfile(t)
	imp := p.Import("invented-producer.txt", []byte("Sample ID: Invented\nEffective Diameter (nm): 12.00\n"), false)
	if imp.Error != "" {
		t.Fatal(imp)
	}
	initial, _, _ := p.File(imp.FileID)
	for _, in := range []ProducerDeclaration{
		{Kind: "verified"}, {Kind: "instrument_export", Vendor: strings.Repeat("a", 161)},
		{Kind: "instrument_export", Evidence: strings.Repeat("a", 2001)},
		{Kind: "instrument_export", Software: "editor\x00hidden"},
		{Kind: "instrument_export", SoftwareVersion: "line\nbreak"},
		{Kind: "unknown", Vendor: "guessed"}, {Kind: "user_created_table", Vendor: string([]byte{0xff})},
	} {
		if _, err := p.DeclareProducerOrigin(imp.FileID, in); !errors.Is(err, ErrProducerOrigin) {
			t.Fatal("invalid declaration accepted", err)
		}
	}
	headers := map[string]string{"X-Account-ID": p.acct.ID(), "X-Profile-ID": p.Entry.ID}
	for _, payload := range []string{
		`{"kind":"instrument_export","declaration":"verified"}`,
		`{"kind":"instrument_export","declaredAt":"2000-01-01T00:00:00Z"}`,
		`{"kind":"instrument_export"} {"kind":"unknown"}`,
		`{"kind":"instrument_export","scientificValidation":"VALIDATED"}`,
		`{"kind":"instrument_export"}` + strings.Repeat(" ", 16<<10) + `{"kind":"unknown"}`,
		"{\"kind\":\"instrument_export\",\"vendor\":\"\xff\"}",
	} {
		if code, _ := c.do("PUT", "/api/files/"+imp.FileID+"/producer-origin", []byte(payload), headers); code < 400 {
			t.Fatal("forged status/time/trailing JSON accepted")
		}
	}
	got, _, err := p.File(imp.FileID)
	if err != nil || !reflect.DeepEqual(initial, got) {
		t.Fatal("rejected declaration changed file", err)
	}
	if _, err := p.DeclareProducerOrigin(imp.FileID, ProducerDeclaration{Kind: "instrument_export", Vendor: " Invented maker "}); err != nil {
		t.Fatal(err)
	}
	cleared, err := p.DeclareProducerOrigin(imp.FileID, ProducerDeclaration{Kind: "unknown"})
	if err != nil || cleared.ProducerOrigin.Vendor != "" || cleared.ProducerOrigin.Kind != "unknown" || cleared.ProducerOrigin.Declaration != "user_declared" {
		t.Fatal("explicit unknown did not clear prior declaration", err)
	}
	if err := p.TrashFile(imp.FileID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := p.DeclareProducerOrigin(imp.FileID, ProducerDeclaration{Kind: "user_created_table"}); !errors.Is(err, ErrProducerOrigin) {
		t.Fatal("trashed source edited", err)
	}
}
