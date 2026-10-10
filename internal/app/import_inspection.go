package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oaovito/mne_lab/internal/science/ingest"
	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
)

var (
	ErrImportReview  = errors.New("ls.import_review_required")
	ErrImportChanged = errors.New("ls.import_review_changed")
	ErrImportScope   = errors.New("ls.import_profile_changed")
)

// ImportInspection is an ephemeral, bounded preview. It has no record/blob ID.
type ImportInspection struct {
	Tabular          *lightscattering.TabularPreview `json:"tabular,omitempty"`
	Selection        *model.ImportSelection          `json:"selection,omitempty"`
	Name             string                          `json:"name"`
	SHA256           string                          `json:"sha256"`
	Format           string                          `json:"format"`
	Module           string                          `json:"module,omitempty"`
	Result           ingest.Result                   `json:"result"`
	Measurements     int                             `json:"measurements"`
	PreviewTruncated bool                            `json:"previewTruncated"`
	NeedsDate        bool                            `json:"needsDate"`
	Existing         string                          `json:"existing,omitempty"`
	AccountID        string                          `json:"accountId"`
	ProfileID        string                          `json:"profileId"`
	Receipt          string                          `json:"receipt,omitempty"`
}

type importClaim struct {
	Format    string                 `json:"format"`
	Selection *model.ImportSelection `json:"selection,omitempty"`
	Version   int                    `json:"v"`
	Account   string                 `json:"account"`
	Profile   string                 `json:"profile"`
	Name      string                 `json:"name"`
	SHA256    string                 `json:"sha256"`
	Parser    string                 `json:"parser"`
	Spec      string                 `json:"spec"`
	Expires   int64                  `json:"expires"`
}

func (p *Profile) importMAC(payload []byte) []byte {
	mac := hmac.New(sha256.New, []byte(p.app.importSecret))
	mac.Write([]byte("mnelab.import-review/1\x00"))
	mac.Write(payload)
	return mac.Sum(nil)
}

// InspectImport reads the original without changing the profile or its queue.
func (p *Profile) InspectImport(name string, data []byte) (ImportInspection, error) {
	return p.InspectImportSelection(name, data, nil)
}

func sameImportSelection(a, b *model.ImportSelection) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

func (p *Profile) InspectImportSelection(name string, data []byte, selection *model.ImportSelection) (ImportInspection, error) {
	selection, err := lightscattering.NormalizeSelection(selection)
	if err != nil {
		return ImportInspection{}, err
	}
	if p.closed.Load() || p.app.exiting.Load() {
		return ImportInspection{}, ErrImportScope
	}
	if name == "" || len(name) > 512 {
		return ImportInspection{}, errors.New("ls.name_required")
	}
	if len(data) == 0 {
		return ImportInspection{}, ErrEmptyFile
	}
	if len(data) > lightscattering.MaxFileSize {
		return ImportInspection{}, ErrFileTooLarge
	}
	r, format, tabular := ingest.Inspect(name, data, selection)
	out := ImportInspection{Tabular: tabular, Selection: selection, Name: name, SHA256: secure.HashHex(data), Format: format, Result: r, Measurements: len(r.Measurements), AccountID: p.acct.ID(), ProfileID: p.Entry.ID}
	if format == "nanobrook-text" {
		out.Format = "txt"
		switch r.Delimiter {
		case "tab":
			out.Format = "tsv"
		case "comma", "semicolon":
			out.Format = "csv"
		}
	}
	if tabular != nil {
		out.Format = tabular.Format
		out.PreviewTruncated = tabular.Truncated
	}
	for _, m := range r.Measurements {
		if m.MeasuredAt == nil || m.MeasuredAt.Ambiguous {
			out.NeedsDate = true
		}
		if len(m.Params) > 0 || m.Dist != nil || len(m.Distributions) > 0 {
			out.Module = "lightscattering"
		}
	}
	if r.SourceInfo != nil && r.Status == "partial" {
		out.Module = r.Module
	}
	if out.Module == "" && r.Status != "failed" {
		out.Result.Status = "failed"
		out.Result.Error = lightscattering.ErrUnrecognized.Error()
	}
	// Inspection may report an existing original, but never updates it.
	err = p.St.View(func(tx *store.Tx) error {
		records, err := tx.List(CollFiles)
		if err != nil {
			return err
		}
		for _, record := range records {
			var f model.SourceFile
			if decodeRecord(record, &f) == nil && !f.Trashed && f.SHA256 == out.SHA256 && sameImportSelection(f.ImportSelection, selection) {
				out.Existing = f.ID
				break
			}
		}
		return nil
	})
	if err != nil {
		return ImportInspection{}, err
	}
	if p.closed.Load() {
		return ImportInspection{}, ErrImportScope
	}
	if r.Status != "failed" && out.Module != "" {
		claim := importClaim{Version: 1, Account: out.AccountID, Profile: out.ProfileID, Name: name, SHA256: out.SHA256, Parser: r.Parser, Spec: r.Spec, Expires: time.Now().Add(30 * time.Minute).Unix(), Selection: selection, Format: format}
		payload, _ := json.Marshal(claim)
		out.Receipt = base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(p.importMAC(payload))
	}
	// Bound only the preview; confirmation reparses the complete original.
	if len(out.Result.Measurements) > 10 {
		out.Result.Measurements = out.Result.Measurements[:10]
		out.PreviewTruncated = true
	}
	// Bound individual source strings too: a single metadata cell can occupy
	// most of an otherwise small file. The untouched original is used to save.
	bound := func(value *string, limit int) {
		if len(*value) <= limit {
			return
		}
		end := limit
		for end > 0 && !utf8.ValidString((*value)[:end]) {
			end--
		}
		*value = (*value)[:end]
		out.PreviewTruncated = true
	}
	for i := range out.Result.Sheets {
		for j := range out.Result.Sheets[i].Headers {
			bound(&out.Result.Sheets[i].Headers[j], 256)
		}
	}
	for i := range out.Result.Measurements {
		m := &out.Result.Measurements[i]
		bound(&m.SampleID, 2048)
		if m.MeasuredAt != nil {
			bound(&m.MeasuredAt.Raw, 2048)
		}
		for key, q := range m.Params {
			bound(&q.Raw, 2048)
			bound(&q.Label, 256)
			bound(&q.Unit, 128)
			m.Params[key] = q
		}
		if len(m.Fields) > 50 {
			m.Fields = m.Fields[:50]
			out.PreviewTruncated = true
		}
		for j := range m.Fields {
			f := &m.Fields[j]
			bound(&f.Label, 256)
			bound(&f.Text, 2048)
			bound(&f.Unit, 128)
		}
		trim := func(d *model.Distribution) {
			for i := range d.Columns {
				c := &d.Columns[i]
				bound(&c.Label, 256)
				bound(&c.Unit, 128)
				if len(c.Values) > 25 {
					c.Values = c.Values[:25]
					out.PreviewTruncated = true
				}
				if len(c.Raw) > 25 {
					c.Raw = c.Raw[:25]
					out.PreviewTruncated = true
				}
				for j := range c.Raw {
					bound(&c.Raw[j], 2048)
				}
			}
			if len(d.Auxiliary) > 50 {
				d.Auxiliary = d.Auxiliary[:50]
				out.PreviewTruncated = true
			}
			for j := range d.Auxiliary {
				f := &d.Auxiliary[j]
				bound(&f.Label, 256)
				bound(&f.Text, 2048)
				bound(&f.Unit, 128)
			}
			if len(d.SourceLines) > 25 {
				d.SourceLines = d.SourceLines[:25]
				out.PreviewTruncated = true
			}
		}
		if m.Dist != nil {
			trim(m.Dist)
		}
		for j := range m.Distributions {
			trim(&m.Distributions[j])
		}
	}
	return out, nil
}

// ConfirmImport rejects changed originals, names, parser versions or profiles
// before Import can write anything. There is no server-side preview cache.
func (p *Profile) ConfirmImport(name string, data []byte, receipt string, force bool) (ImportResult, error) {
	return p.ConfirmImportSelection(name, data, receipt, force, nil)
}
func (p *Profile) ConfirmImportSelection(name string, data []byte, receipt string, force bool, selection *model.ImportSelection) (ImportResult, error) {
	selection, err := lightscattering.NormalizeSelection(selection)
	if err != nil {
		return ImportResult{}, err
	}
	claim, err := p.reviewClaim(receipt)
	if err != nil {
		return ImportResult{}, err
	}
	parser, spec := ingest.IdentitySelection(name, data, selection)
	if !sameImportSelection(claim.Selection, selection) || len(data) == 0 || len(data) > lightscattering.MaxFileSize || name != claim.Name || secure.HashHex(data) != claim.SHA256 || claim.Parser != parser || claim.Spec != spec {
		return ImportResult{}, ErrImportChanged
	}
	return p.importSelected(name, data, force, selection), nil
}

// reviewClaim also authorizes saving a reviewed selection; no file is stored.
func (p *Profile) reviewClaim(receipt string) (importClaim, error) {
	if len(receipt) > 65536 || receipt == "" {
		return importClaim{}, ErrImportReview
	}
	parts := strings.Split(receipt, ".")
	if len(parts) != 2 {
		return importClaim{}, ErrImportReview
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return importClaim{}, ErrImportReview
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, p.importMAC(payload)) {
		return importClaim{}, ErrImportReview
	}
	var claim importClaim
	if json.Unmarshal(payload, &claim) != nil || claim.Version != 1 || time.Now().Unix() >= claim.Expires {
		return importClaim{}, ErrImportReview
	}
	if p.closed.Load() || p.app.exiting.Load() || claim.Account != p.acct.ID() || claim.Profile != p.Entry.ID {
		return importClaim{}, ErrImportScope
	}
	return claim, nil
}
