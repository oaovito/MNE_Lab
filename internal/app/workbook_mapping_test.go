package app

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/model"
)

func TestWorkbookMappingReceiptBindsPhysicalSelectionAndPreservesOriginal(t *testing.T) {
	_, _, p := inspectionProfile(t)
	data, e := os.ReadFile("../../testdata/workbook-mapping/invented.xlsx")
	if e != nil {
		t.Fatal(e)
	}
	s := &model.ImportSelection{Mapping: &model.ColumnMapping{Schema: 2, Module: "lightscattering", Sheet: "Raw Data", Decimal: "dot", HeaderRow: 2, FirstRow: 3, LastRow: 28, SampleColumn: 1, Columns: []model.MappedColumn{{Column: 2, Key: model.EffectiveDiameter, Unit: "nm"}}}}
	before := profileFingerprint(t, p)
	review, e := p.InspectImportSelection("invented.xlsx", data, s)
	if e != nil || review.Receipt == "" || review.Measurements != 26 || len(review.Result.Measurements) != 10 || review.Result.Parser != lightscattering.WorkbookMappingVersion || review.Result.SourceInfo.Container != "OOXML workbook" || review.Result.SourceInfo.ScientificValidation != "UNVALIDATED" {
		t.Fatal("review", e, review)
	}
	old, err := p.reviewClaim(review.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	old.Parser = "dls-workbook-column-reader/1.0.0"
	payload, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	oldReceipt := base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(p.importMAC(payload))
	if _, err := p.ConfirmImportSelection("invented.xlsx", data, oldReceipt, false, s); !errors.Is(err, ErrImportChanged) {
		t.Fatal("obsolete reader receipt accepted", err)
	}
	if _, err := p.SaveImportProfile("Old reader", oldReceipt); !errors.Is(err, ErrImportChanged) {
		t.Fatal("obsolete reader profile saved", err)
	}
	for name, edit := range map[string]func(*model.ColumnMapping){"sheet": func(m *model.ColumnMapping) { m.Sheet = "Other" }, "header": func(m *model.ColumnMapping) { m.HeaderRow = 1 }, "first": func(m *model.ColumnMapping) { m.FirstRow = 4 }, "last": func(m *model.ColumnMapping) { m.LastRow = 27 }, "column": func(m *model.ColumnMapping) { m.Columns[0].Column = 3 }, "unit": func(m *model.ColumnMapping) { m.Columns[0].Unit = "um" }, "decimal": func(m *model.ColumnMapping) { m.Decimal = "comma" }, "sample": func(m *model.ColumnMapping) { m.SampleColumn = 0 }} {
		t.Run(name, func(t *testing.T) {
			changed := s.Clone()
			edit(changed.Mapping)
			if _, e := p.ConfirmImportSelection("invented.xlsx", data, review.Receipt, false, changed); !errors.Is(e, ErrImportChanged) {
				t.Fatal("changed recipe confirmed", e)
			}
		})
	}
	if !reflect.DeepEqual(before, profileFingerprint(t, p)) {
		t.Fatal("inspection/rejections wrote profile")
	}
	profile, e := p.SaveImportProfile("Invented workbook recipe", review.Receipt)
	if e != nil || profile.Schema != 3 || profile.Parser != lightscattering.WorkbookMappingVersion || profile.Format != "xlsx" || !sameImportSelection(profile.Selection, s) {
		t.Fatal("recipe", e)
	}
	imported, e := p.ConfirmImportSelection("invented.xlsx", data, review.Receipt, false, profile.Selection)
	if e != nil || imported.Measurements != 26 || imported.Status != "partial" {
		t.Fatal("import", e)
	}
	file, ms, e := p.File(imported.FileID)
	if e != nil || len(ms) != 26 || ms[25].SourceSheet != "Raw Data" || ms[25].SourceRange != "A28:B28" || ms[25].Params[model.EffectiveDiameter].Raw != "23.0000" || ms[25].Params[model.EffectiveDiameter].Line != 28 || file.ImportSelection.Mapping.LastRow != 28 {
		t.Fatal("source truncated", e)
	}
	_, original, e := p.Original(file.ID)
	if e != nil || !bytes.Equal(original, data) {
		t.Fatal("original altered")
	}
	again, e := p.InspectImportSelection("invented.xlsx", data, s)
	if e != nil || again.Existing != file.ID {
		t.Fatal("duplicate identity")
	}
	changed := s.Clone()
	changed.Mapping.FirstRow = 4
	other, e := p.InspectImportSelection("invented.xlsx", data, changed)
	if e != nil || other.Existing != "" || other.Receipt == "" {
		t.Fatal("distinct selection deduplicated")
	}
}
