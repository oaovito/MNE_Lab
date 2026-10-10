package app

import (
	"bytes"
	"errors"
	"github.com/oaovito/mne_lab/internal/export"
	"github.com/oaovito/mne_lab/internal/science/analysis"
	"github.com/oaovito/mne_lab/internal/science/cycle"
	"github.com/oaovito/mne_lab/internal/store"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/xuri/excelize/v2"
	"strings"
)

func appMapping() *model.ImportSelection {
	return &model.ImportSelection{Mapping: &model.ColumnMapping{Schema: 1, Module: "lightscattering", Delimiter: "comma", Decimal: "dot", HeaderRecord: 1, FirstRecord: 2, LastRecord: 26, Columns: []model.MappedColumn{{Column: 2, Key: model.EffectiveDiameter, Unit: "nm"}}}}
}
func TestMappedImportReviewBindsAllChoicesAndPreservesFullSource(t *testing.T) {
	_, _, p := inspectionProfile(t)
	data := []byte("sample,unknown size (um)\n")
	for i := 0; i < 25; i++ {
		data = append(data, []byte("invented,1.2300\n")...)
	}
	s := appMapping()
	before := profileFingerprint(t, p)
	review, e := p.InspectImportSelection("invented.csv", data, s)
	if e != nil || review.Receipt == "" || review.Result.Parser != lightscattering.MappingVersion || review.Result.SourceInfo.ScientificValidation != "UNVALIDATED" || review.Measurements != 25 || len(review.Result.Measurements) != 10 || !review.PreviewTruncated {
		t.Fatal("bad reviewed mapping", e)
	}
	edits := map[string]func(*model.ImportSelection){"unit": func(s *model.ImportSelection) { s.Mapping.Columns[0].Unit = "um" }, "key": func(s *model.ImportSelection) { s.Mapping.Columns[0].Key = model.Polydispersity }, "column": func(s *model.ImportSelection) { s.Mapping.Columns[0].Column = 1 }, "decimal": func(s *model.ImportSelection) { s.Mapping.Decimal = "comma" }, "delimiter": func(s *model.ImportSelection) { s.Mapping.Delimiter = "semicolon" }, "records": func(s *model.ImportSelection) { s.Mapping.LastRecord = 25 }, "sample": func(s *model.ImportSelection) { s.Mapping.SampleColumn = 1 }}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			changed := s.Clone()
			edit(changed)
			if _, err := p.ConfirmImportSelection("invented.csv", data, review.Receipt, false, changed); !errors.Is(err, ErrImportChanged) {
				t.Fatal("changed recipe accepted", err)
			}
		})
	}
	if _, err := p.ConfirmImportSelection("other.csv", data, review.Receipt, false, s); !errors.Is(err, ErrImportChanged) {
		t.Fatal("changed name accepted")
	}
	if _, err := p.ConfirmImportSelection("invented.csv", append(append([]byte{}, data...), 10), review.Receipt, false, s); !errors.Is(err, ErrImportChanged) {
		t.Fatal("changed bytes accepted")
	}
	if _, err := p.ConfirmImport("invented.csv", data, review.Receipt, false); !errors.Is(err, ErrImportChanged) {
		t.Fatal("removed recipe accepted")
	}
	if !reflect.DeepEqual(before, profileFingerprint(t, p)) {
		t.Fatal("preview/rejected confirmations wrote profile")
	}
	profile, e := p.SaveImportProfile("Invented column recipe", review.Receipt)
	if e != nil || profile.Schema != 2 || profile.Format != "csv" || profile.Parser != lightscattering.MappingVersion || !sameImportSelection(profile.Selection, review.Selection) {
		t.Fatal("saved recipe lost choices", e)
	}
	imported, e := p.ConfirmImportSelection("invented.csv", data, review.Receipt, false, s)
	if e != nil || imported.Status != "partial" || imported.Measurements != 25 {
		t.Fatal("full mapped import failed", e)
	}
	file, ms, e := p.File(imported.FileID)
	if e != nil || len(ms) != 25 || file.Status != "partial" || file.SourceInfo.ScientificValidation != "UNVALIDATED" {
		t.Fatal("stored mapping", e)
	}
	q := ms[24].Params[model.EffectiveDiameter]
	if q.Raw != "1.2300" || q.Line != 26 || q.UnitOrigin != "user_mapping" || q.SourceColumn != 2 {
		t.Fatal("full source/provenance truncated", q)
	}
	_, original, e := p.Original(file.ID)
	if e != nil || !bytes.Equal(original, data) {
		t.Fatal("original changed")
	}
	snapshot := (graph.Input{Files: map[string]model.SourceFile{file.ID: file}}).Source(ms[0])
	snapshot.ImportSelection.Mapping.Columns[0].Unit = "changed"
	snapshot.Parameters[model.EffectiveDiameter] = model.Quantity{}
	if file.ImportSelection.Mapping.Columns[0].Unit != "nm" || ms[0].Params[model.EffectiveDiameter].UnitOrigin != "user_mapping" {
		t.Fatal("snapshot aliases scientific source")
	}
	again, e := p.InspectImportSelection("invented.csv", data, s)
	if e != nil || again.Existing != file.ID {
		t.Fatal("recipe duplicate lost")
	}
	changed := s.Clone()
	changed.Mapping.Columns[0].Unit = "um"
	other, e := p.InspectImportSelection("invented.csv", data, changed)
	if e != nil || other.Existing != "" || other.Receipt == "" {
		t.Fatal("distinct recipe incorrectly deduplicated")
	}
	invalid := []byte("a,b\ninvented,bad\n")
	bad := s.Clone()
	bad.Mapping.LastRecord = 2
	start := profileFingerprint(t, p)
	r, e := p.InspectImportSelection("bad.csv", invalid, bad)
	if e != nil || r.Receipt != "" || r.Result.Status != "failed" {
		t.Fatal("bad recipe signed")
	}
	if !reflect.DeepEqual(start, profileFingerprint(t, p)) {
		t.Fatal("failed mapping wrote source")
	}
}

func TestManualMappingIntegratesGraphCycleExportsAndAnalysisSnapshots(t *testing.T) {
	for _, workbook := range []bool{false, true} {
		name := "csv"
		if workbook {
			name = "xlsx"
		}
		t.Run(name, func(t *testing.T) {
			_, _, p := inspectionProfile(t)
			data := []byte("sample,value\nA,1.000\nA,2.000\nA,3.000\nB,4.000\nB,5.000\nB,6.000\n")
			s := appMapping()
			s.Mapping.LastRecord = 7
			s.Mapping.SampleColumn = 1
			filename := "invented.csv"
			if workbook {
				f := excelize.NewFile()
				defer f.Close()
				f.SetSheetName("Sheet1", "Mapped")
				for i, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
					for col, raw := range strings.Split(line, ",") {
						address, _ := excelize.CoordinatesToCellName(col+1, i+2)
						if err := f.SetCellStr("Mapped", address, raw); err != nil {
							t.Fatal(err)
						}
					}
				}
				buf, err := f.WriteToBuffer()
				if err != nil {
					t.Fatal(err)
				}
				data = buf.Bytes()
				filename = "invented.xlsx"
				s.Mapping.Schema = 2
				s.Mapping.Sheet = "Mapped"
				s.Mapping.Delimiter = ""
				s.Mapping.HeaderRecord = 0
				s.Mapping.FirstRecord = 0
				s.Mapping.LastRecord = 0
				s.Mapping.HeaderRow = 2
				s.Mapping.FirstRow = 3
				s.Mapping.LastRow = 8
			}
			review, e := p.InspectImportSelection(filename, data, s)
			if e != nil {
				t.Fatal(e)
			}
			imported, e := p.ConfirmImportSelection(filename, data, review.Receipt, false, s)
			if e != nil {
				t.Fatal(e)
			}
			file, ms, e := p.File(imported.FileID)
			if e != nil {
				t.Fatal(e)
			}
			in := graph.Input{Files: map[string]model.SourceFile{file.ID: file}, Measurements: map[string]model.Measurement{}}
			ids := []string{}
			assignments := []cycle.Assignment{}
			d := analysis.Definition{Schema: 1, Module: "lightscattering", Title: "Invented mapped analysis", Variable: model.EffectiveDiameter, Structure: "independent", StructureReviewed: true, Method: "one_way", Alpha: .05, PostHoc: "none"}
			for i, m := range ms {
				in.Measurements[m.ID] = m
				ids = append(ids, m.ID)
				assignments = append(assignments, cycle.Assignment{MeasurementID: m.ID, Point: 0, Status: "confirmed"})
				group := "A"
				if i >= 3 {
					group = "B"
				}
				d.Observations = append(d.Observations, analysis.ObservationDefinition{MeasurementID: m.ID, FactorA: group})
			}
			cfg := cycle.Config{Name: "Explicit invented cycle", Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), StartTZ: true, Interval: 1, Unit: cycle.Days, Duration: 1}
			points, e := cfg.Points()
			if e != nil {
				t.Fatal(e)
			}
			cy := graph.CycleInput{Config: cfg, Points: points, Assignments: assignments}
			g, e := graph.ParameterTime(graph.Definition{Param: model.EffectiveDiameter}, in, cy)
			if e != nil || g.Y.Unit != "nm" || len(g.Series[0].Y) != 6 || !slices.Contains(g.Warnings, "ls.user_mapping_unvalidated") || g.Provenance.Sources[0].Parameters[model.EffectiveDiameter].UnitOrigin != "user_mapping" {
				t.Fatal("graph lost assignment provenance", e)
			}
			translate := func(key string, kv ...string) string { return key }
			measurement := export.MeasurementData("Mapped", ids, in, nil, translate)
			parameter, e := export.ParameterData("Mapped", model.EffectiveDiameter, in, cy, translate)
			if e != nil {
				t.Fatal(e)
			}
			for _, ds := range []export.DataSet{measurement, parameter} {
				table := ds.Tables[0]
				rawValues := map[string]bool{}
				for _, row := range table.Rows {
					fields := map[string]string{}
					for i, c := range table.Columns {
						fields[c.Key] = row[i].String(".")
					}
					rawValues[fields[model.EffectiveDiameter+"_raw"]] = true
					if fields[model.EffectiveDiameter+"_unit_origin"] != "user_mapping" || fields[model.EffectiveDiameter+"_source_column"] != "2" || fields[model.EffectiveDiameter+"_source_label"] != "value" {
						t.Fatal("flat export lost declared origin")
					}
				}
				for _, raw := range []string{"1.000", "2.000", "3.000", "4.000", "5.000", "6.000"} {
					if !rawValues[raw] {
						t.Fatal("export lost original value", raw)
					}
				}
				if len(rawValues) != 6 || ds.Provenance[0].ImportSelection.Mapping == nil {
					t.Fatal("structured export lost recipe")
				}

				for _, format := range []string{"csv", "tsv", "json", "xlsx"} {
					if _, err := export.WriteData(ds, format, export.DataOptions{}); err != nil {
						t.Fatal(format, err)
					}
				}
			}
			prepared, e := p.PrepareAnalysis(d)
			if e != nil || !slices.Contains(prepared.Design.Warnings, "ls.user_mapping_unvalidated") || prepared.Observations[0].Quantity.UnitOrigin != "user_mapping" || prepared.Sources[0].File.ImportSelection.Mapping == nil {
				t.Fatal("analysis discarded mapping", e)
			}
			saved, e := p.SaveAnalysis(d, prepared.Receipt, statisticalContract(prepared), "")
			if e != nil {
				t.Fatal(e)
			}
			ds := export.StatisticalData(saved)
			found := false
			for _, table := range ds.Tables {
				if table.ID == "observations" {
					for i, c := range table.Columns {
						if c.Key == "quantity_unit_origin" && table.Rows[0][i].String(".") == "user_mapping" {
							found = true
						}
					}
				}
			}
			if !found {
				t.Fatal("statistical export lost assignment origin")
			}
			// Alteration of source provenance invalidates fresh-save receipt, while the
			// saved analysis and its export keep the originally reviewed declaration.
			e = p.St.Update(func(tx *store.Tx) error {
				m := ms[0]
				q := m.Params[model.EffectiveDiameter]
				q.UnitOrigin = "changed"
				m.Params[model.EffectiveDiameter] = q
				_, err := tx.Put(CollMeasurements, m.ID, m)
				return err
			})
			if e != nil {
				t.Fatal(e)
			}
			if _, err := p.SaveAnalysis(d, prepared.Receipt, statisticalContract(prepared), ""); !errors.Is(err, analysis.ErrSource) {
				t.Fatal("changed provenance accepted", err)
			}
			old, e := p.Analysis(saved.ID)
			if e != nil || old.Snapshot.Observations[0].Quantity.UnitOrigin != "user_mapping" {
				t.Fatal("saved snapshot rewritten", e)
			}

		})
	}
}
