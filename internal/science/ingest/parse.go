// Package ingest dispatches content to independent scientific adapters. Binary
// container decoding, instrument field semantics and normalization stay apart.
package ingest

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/malvern/dts"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/module"
)

var ErrSelection = errors.New("dts.selection_not_supported")

const DTSSpec = "dts-observed-envelope/0.1-unvalidated"

// Identity supports version-bound review receipts without applying a different
// adapter's parser/spec identity to this original. Dispatch is content-first.
func Identity(name string, data []byte) (string, string) {
	if dts.IsCompound(data) || strings.EqualFold(filepath.Ext(name), ".dts") {
		return dts.Version, DTSSpec
	}
	if strings.EqualFold(filepath.Ext(name), ".ods") || lightscattering.IsODSPackage(data) {
		return lightscattering.ODSReaderVersion, ""
	}
	return lightscattering.Version, lightscattering.Parse(nil).Spec
}

// IdentitySelection binds manual recipes to their own reader and provisional contract.
func IdentitySelection(name string, data []byte, selection *model.ImportSelection) (string, string) {
	if selection != nil && selection.Mapping != nil && !dts.IsCompound(data) && !strings.EqualFold(filepath.Ext(name), ".dts") {
		return lightscattering.MappingVersion, lightscattering.MappingSpec
	}
	return Identity(name, data)
}

type Result struct {
	lightscattering.Result
	Module     string            `json:"module"`
	SourceInfo *model.SourceInfo `json:"sourceInfo,omitempty"`
}

// Inspect retains literal table cells for review without changing the parser,
// normalized values, receipt semantics or scientific classification.
func Inspect(name string, data []byte, selection *model.ImportSelection) (Result, string, *lightscattering.TabularPreview) {
	if dts.IsCompound(data) || strings.EqualFold(filepath.Ext(name), ".dts") {
		r, format := Parse(name, data, selection)
		return r, format, nil
	}
	r, format, table := lightscattering.InspectFileSelection(name, data, selection)
	if format == "ods" {
		return Result{Result: r}, format, table // literal container, no scientific adapter assigned
	}
	return mappedResult(r, selection), format, table
}

func Parse(name string, data []byte, selection *model.ImportSelection) (Result, string) {
	if !dts.IsCompound(data) && !strings.EqualFold(filepath.Ext(name), ".dts") {
		r, format := lightscattering.ParseFileSelection(name, data, selection)
		if format == "ods" {
			return Result{Result: r}, format
		}
		return mappedResult(r, selection), format
	}
	out := Result{Result: lightscattering.Result{Status: "failed", Parser: dts.Version, Spec: DTSSpec, Measurements: []model.Measurement{}, Recognized: []string{}}}
	if selection != nil {
		out.Error = ErrSelection.Error()
		return out, "dts"
	}
	report, err := dts.Inspect(data)
	if err != nil && !errors.Is(err, dts.ErrUnsupported) {
		out.Error = err.Error()
		return out, "dts"
	}
	out.Status, out.Encoding = "partial", "binary CFB / UTF-16LE tagged strings"
	out.Module = module.UnknownMalvern
	if errors.Is(err, dts.ErrUnsupported) {
		out.Module = module.UnknownCompound
		report.Warnings = append(report.Warnings, "dts.structure_unsupported")
	}
	// Unlabelled strings may contain operator/sample names and historical paths.
	// Keep them only in explicit private dev inspection; the original retains
	// all bytes. Normal library metadata reports coverage without guessing roles.
	for i := range report.Records {
		report.Records[i].Strings = nil
	}
	details, _ := json.Marshal(report)
	out.SourceInfo = &model.SourceInfo{Vendor: "Malvern / Zetasizer (record-envelope evidence)", Container: "Microsoft Compound File", Support: "PARTIAL", ScientificValidation: "UNVALIDATED", Details: details}
	if out.Module == module.UnknownCompound {
		out.SourceInfo.Vendor = "UNKNOWN"
	}
	out.Warnings = report.Warnings
	format := "dts"
	if out.Module == module.UnknownCompound {
		format = "compound"
	}
	return out, format
}

func mappedResult(r lightscattering.Result, selection *model.ImportSelection) Result {
	out := Result{Result: r, Module: module.LightScattering}
	if selection != nil && selection.Mapping != nil && r.Status != "failed" {
		out.SourceInfo = &model.SourceInfo{Vendor: "USER_DECLARED", Container: "Delimited text", Support: "PARTIAL", ScientificValidation: "UNVALIDATED"}
	}
	return out
}
