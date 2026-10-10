package lightscattering

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/oaovito/mne_lab/internal/science/model"
)

func inventedMapping() *model.ImportSelection {
	return &model.ImportSelection{Mapping: &model.ColumnMapping{Schema: 1, Module: "lightscattering", Delimiter: "semicolon", Decimal: "comma", HeaderRecord: 1, FirstRecord: 2, LastRecord: 4, SampleColumn: 1, Columns: []model.MappedColumn{{Column: 2, Key: model.EffectiveDiameter, Unit: "nm"}, {Column: 3, Key: model.AverageCountRate, Unit: ""}}}}
}
func TestExplicitColumnMappingPreservesDeclaredChoicesAndSource(t *testing.T) {
	data := []byte("sample;unknown size label (um);counts\n\"synthetic\nsample\";1,2300;0,0000\nmissing;;\nshort;2,5000\n")
	s := inventedMapping()
	result, format, preview := InspectFileSelection("invented.csv", data, s)
	if result.Status != "partial" || result.Parser != MappingVersion || result.Spec != MappingSpec || format != "csv" || len(result.Measurements) != 3 || preview == nil || preview.Tables[0].RowCount != 4 {
		t.Fatalf("bad mapping %#v", result)
	}
	m := result.Measurements[0]
	q := m.Params[model.EffectiveDiameter]
	if q.Value != 1.23 || q.Raw != "1,2300" || q.Unit != "nm" || q.UnitOrigin != "user_mapping" || q.Label != "unknown size label (um)" || q.Line != 3 || q.SourceColumn != 2 || m.SampleID != "synthetic\nsample" || m.SourceRange != "record:2" {
		t.Fatal("source/assignment conflated", m)
	}
	count := m.Params[model.AverageCountRate]
	if count.Value != 0 || count.Raw != "0,0000" || count.Unit != "" || count.UnitOrigin != "user_mapping" || m.Fields[1].Unit != "" {
		t.Fatal("missing unit or zero changed")
	}
	if len(result.Measurements[1].Params) != 0 || len(result.Measurements[2].Params) != 1 || result.Measurements[0].MeasuredAt != nil || m.ExperimentalUnitID != "" || m.Dist != nil {
		t.Fatal("invented absent quantities, dates or independence")
	}
	full, other := ParseFileSelection("invented.csv", data, s)
	a, _ := json.Marshal(result)
	b, _ := json.Marshal(full)
	if string(a) != string(b) || other != format {
		t.Fatal("inspection/confirmation divergence")
	}
	// Full parser must not consume bounded preview cells or records.
	var large strings.Builder
	large.WriteString("unknown\n")
	for i := 0; i < 35; i++ {
		large.WriteString("1.2300\n")
	}
	only := &model.ImportSelection{Mapping: &model.ColumnMapping{Schema: 1, Module: "lightscattering", Delimiter: "comma", Decimal: "dot", HeaderRecord: 1, FirstRecord: 2, LastRecord: 36, Columns: []model.MappedColumn{{Column: 1, Key: model.EffectiveDiameter}}}}
	r, _, p := InspectFileSelection("large.csv", []byte(large.String()), only)
	if len(r.Measurements) != 35 || len(p.Tables[0].Rows) != 20 || !p.Truncated || r.Measurements[34].Params[model.EffectiveDiameter].Raw != "1.2300" {
		t.Fatal("truncated source imported")
	}
}
func TestColumnMappingRejectsInvalidRecipesAndNumbersAtomically(t *testing.T) {
	recipeCases := map[string]func(*model.ImportSelection){
		"schema": func(s *model.ImportSelection) { s.Mapping.Schema = 2 }, "module": func(s *model.ImportSelection) { s.Mapping.Module = "zeta" },
		"sheet": func(s *model.ImportSelection) { s.Sheets = []model.SheetSelection{{Name: "Sheet1"}} }, "empty": func(s *model.ImportSelection) { s.Mapping.Columns = nil },
		"duplicate field": func(s *model.ImportSelection) { s.Mapping.Columns[1].Key = model.EffectiveDiameter }, "duplicate column": func(s *model.ImportSelection) { s.Mapping.Columns[1].Column = 2 },
		"sample overlap": func(s *model.ImportSelection) { s.Mapping.SampleColumn = 2 }, "unknown field": func(s *model.ImportSelection) { s.Mapping.Columns[0].Key = "zeta_potential" },
		"zero column": func(s *model.ImportSelection) { s.Mapping.Columns[0].Column = 0 }, "unit control": func(s *model.ImportSelection) { s.Mapping.Columns[0].Unit = "nm\n" },
		"huge column": func(s *model.ImportSelection) { s.Mapping.Columns[0].Column = 1025 }, "header overlap": func(s *model.ImportSelection) { s.Mapping.HeaderRecord = 2 },
		"negative rows": func(s *model.ImportSelection) { s.Mapping.FirstRecord = -1 }, "reversed rows": func(s *model.ImportSelection) { s.Mapping.LastRecord = 1 },
		"decimal guess": func(s *model.ImportSelection) { s.Mapping.Decimal = "auto" }, "delimiter guess": func(s *model.ImportSelection) { s.Mapping.Delimiter = "auto" },
	}
	for name, change := range recipeCases {
		t.Run(name, func(t *testing.T) {
			s := inventedMapping()
			change(s)
			if _, err := NormalizeSelection(s); err == nil {
				t.Fatal("unsafe recipe accepted")
			}
		})
	}
	for _, token := range []string{"1.2300", "1,2,3", "NaN", "Inf", "=SUM(1,2)", "1e309", "1e-400", "1 200", "0x12", "1%"} {
		t.Run("numeric "+token, func(t *testing.T) {
			s := inventedMapping()
			s.Mapping.LastRecord = 2
			data := []byte("a;b;c\nx;" + token + ";0\n")
			r, _, _ := InspectFileSelection("invented.csv", data, s)
			if r.Error != ErrMappedNumber.Error() || len(r.Measurements) != 0 {
				t.Fatal("invalid number accepted", r)
			}
		})
	}
	for _, data := range []string{"a;b;c\nx;1,2;0\n", "a;b;c\nx;1,2;0\ny;2,3;0\nz;3,4;0\n\"malformed"} {
		r, _, _ := InspectFileSelection("invented.csv", []byte(data), inventedMapping())
		if r.Status != "failed" || len(r.Measurements) != 0 {
			t.Fatal("missing records or malformed trailing content accepted")
		}
	}
	for _, name := range []string{"invented.xlsx", "invented.ods", "invented.txt"} {
		r, _ := ParseFileSelection(name, []byte("a;b;c\nx;1,2;0\ny;;\nz;;\n"), inventedMapping())
		if r.Status != "failed" {
			t.Fatal("unsupported container accepted")
		}
	}
}
func TestMappingCanonicalCopyAndDistinctCountFields(t *testing.T) {
	s := inventedMapping()
	s.Mapping.Columns[0], s.Mapping.Columns[1] = s.Mapping.Columns[1], s.Mapping.Columns[0]
	n, e := NormalizeSelection(s)
	if e != nil || n.Mapping.Columns[0].Column != 2 {
		t.Fatal("canonical order")
	}
	n.Mapping.Columns[0].Unit = "user"
	if s.Mapping.Columns[1].Unit != "nm" {
		t.Fatal("aliased recipe")
	}
	s.Mapping.Columns = []model.MappedColumn{{Column: 2, Key: model.CountRate, Unit: "kcps"}, {Column: 3, Key: model.AverageCountRate, Unit: "cps"}}
	s.Mapping.LastRecord = 2
	r, _ := ParseFileSelection("invented.tsv", []byte("sample;current;average\nx;2,500;1200\n"), s)
	if r.Measurements[0].Params[model.CountRate].Value != 2.5 || r.Measurements[0].Params[model.AverageCountRate].Value != 1200 {
		t.Fatal("count-rate fields or scales conflated")
	}
}
func FuzzManualDelimitedMapping(f *testing.F) {
	f.Add([]byte("a;b;c\nx;1,2300;0\ny;;\nz;2,5000\n"))
	f.Add([]byte("malformed"))
	f.Fuzz(func(t *testing.T, data []byte) {
		r, _, _ := InspectFileSelection("invented.csv", data, inventedMapping())
		if r.Status == "failed" && len(r.Measurements) > 0 {
			t.Fatal("failed parse retained quantities")
		}
		if _, e := json.Marshal(r); e != nil {
			t.Fatal(e)
		}
	})
}
