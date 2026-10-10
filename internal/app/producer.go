package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/store"
)

var ErrProducerOrigin = errors.New("ls.invalid_producer_origin")

func decodeProducerDeclaration(body io.Reader) (ProducerDeclaration, error) {
	var in ProducerDeclaration
	data, err := io.ReadAll(io.LimitReader(body, (16<<10)+1))
	if err != nil || len(data) > 16<<10 || !utf8.Valid(data) {
		return in, ErrProducerOrigin
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF {
		return in, ErrProducerOrigin
	}
	return in, nil
}

// ProducerDeclaration deliberately excludes verification status and timestamps.
// Neither can be supplied by the caller as an authentication/science claim.
type ProducerDeclaration struct {
	Kind            string `json:"kind"`
	Vendor          string `json:"vendor"`
	Instrument      string `json:"instrument"`
	Software        string `json:"software"`
	SoftwareVersion string `json:"softwareVersion"`
	Evidence        string `json:"evidence"`
}

func (p *Profile) DeclareProducerOrigin(id string, in ProducerDeclaration) (model.SourceFile, error) {
	if p.closed.Load() {
		return model.SourceFile{}, ErrImportScope
	}
	if in.Kind != "unknown" && in.Kind != "instrument_export" && in.Kind != "user_created_table" {
		return model.SourceFile{}, ErrProducerOrigin
	}
	fields := []*string{&in.Vendor, &in.Instrument, &in.Software, &in.SoftwareVersion, &in.Evidence}
	for i, field := range fields {
		limit := 160
		if i == 4 {
			limit = 2000
		}
		if !utf8.ValidString(*field) || utf8.RuneCountInString(*field) > limit || strings.ContainsFunc(*field, unicode.IsControl) {
			return model.SourceFile{}, ErrProducerOrigin
		}
		*field = strings.TrimSpace(*field)
		if in.Kind == "unknown" && *field != "" {
			return model.SourceFile{}, ErrProducerOrigin
		}
	}
	now := time.Now().UTC()
	declared := &model.ProducerOrigin{Kind: in.Kind, Declaration: "user_declared", Vendor: in.Vendor,
		Instrument: in.Instrument, Software: in.Software, SoftwareVersion: in.SoftwareVersion, Evidence: in.Evidence, DeclaredAt: &now}
	var file model.SourceFile
	err := p.St.Update(func(tx *store.Tx) error {
		if _, err := tx.Get(CollFiles, id, &file); err != nil {
			return err
		}
		if file.Trashed {
			return ErrProducerOrigin
		}
		file.ProducerOrigin = declared
		_, err := tx.Put(CollFiles, id, file)
		return err
	})
	return file, err
}
