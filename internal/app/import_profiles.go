package app

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oaovito/mne_lab/internal/science/lightscattering"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
)

var (
	ErrImportProfileName   = errors.New("ls.import_profile_name")
	ErrImportProfileExists = errors.New("ls.import_profile_exists")
	ErrImportProfileLimit  = errors.New("ls.import_profile_limit")
	ErrImportProfileFormat = errors.New("ls.import_profile_format")
)

// ImportProfile is a user-saved worksheet selection, never source data or a
// cached review receipt. Later mappings can version this recipe explicitly.
type ImportProfile struct {
	ID        string                 `json:"id"`
	Schema    int                    `json:"schema"`
	Name      string                 `json:"name"`
	Format    string                 `json:"format"`
	Selection *model.ImportSelection `json:"selection,omitempty"`
	Parser    string                 `json:"parser"`
	Spec      string                 `json:"spec"`
	CreatedAt time.Time              `json:"createdAt"`
}

func (p *Profile) ImportProfiles() ([]ImportProfile, error) {
	if p.closed.Load() {
		return nil, ErrImportScope
	}
	profiles := []ImportProfile{}
	err := p.St.View(func(tx *store.Tx) error {
		records, err := tx.List(CollImportProfiles)
		if err != nil {
			return err
		}
		for _, record := range records {
			var profile ImportProfile
			if err := decodeRecord(record, &profile); err != nil {
				return err
			}
			profiles = append(profiles, profile)
		}
		return nil
	})
	sort.Slice(profiles, func(i, j int) bool {
		a, b := strings.ToLower(profiles[i].Name), strings.ToLower(profiles[j].Name)
		if a == b {
			return profiles[i].ID < profiles[j].ID
		}
		return a < b
	})
	return profiles, err
}

// SaveImportProfile requires the authenticated receipt for the reviewed
// configuration. The user explicitly saves it; inspection itself stays read-only.
func (p *Profile) SaveImportProfile(name, receipt string) (ImportProfile, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 96 || strings.ContainsAny(name, "\r\n\x00") {
		return ImportProfile{}, ErrImportProfileName
	}
	claim, err := p.reviewClaim(receipt)
	if err != nil {
		return ImportProfile{}, err
	}
	manual := claim.Selection != nil && claim.Selection.Mapping != nil
	parser, spec := lightscattering.MappingIdentity(nil)
	if manual {
		parser, spec = lightscattering.MappingIdentity(claim.Selection.Mapping)
	}
	if (!manual && (claim.Parser != lightscattering.Version || claim.Spec != lightscattering.Parse(nil).Spec)) || (manual && (claim.Parser != parser || claim.Spec != spec)) {
		return ImportProfile{}, ErrImportChanged
	}
	if (!manual && claim.Format != "xlsx") || (manual && ((claim.Selection.Mapping.Schema == 1 && claim.Format != "csv" && claim.Format != "tsv") || (claim.Selection.Mapping.Schema == 2 && claim.Format != "xlsx"))) {
		return ImportProfile{}, ErrImportProfileFormat
	}
	selection, err := lightscattering.NormalizeSelection(claim.Selection)
	if err != nil {
		return ImportProfile{}, err
	}
	schema := 1
	if manual {
		schema = 2
		if claim.Selection.Mapping.Schema == 2 {
			schema = 3
		}
	}
	profile := ImportProfile{ID: secure.NewID(), Schema: schema, Name: name, Format: claim.Format, Selection: selection, Parser: claim.Parser, Spec: claim.Spec, CreatedAt: time.Now().UTC()}
	done := p.app.begin("import-profile")
	defer done()
	err = p.St.Update(func(tx *store.Tx) error {
		records, err := tx.List(CollImportProfiles)
		if err != nil {
			return err
		}
		if len(records) >= 100 {
			return ErrImportProfileLimit
		}
		for _, record := range records {
			var old ImportProfile
			if err := decodeRecord(record, &old); err != nil {
				return err
			}
			if strings.EqualFold(old.Name, name) {
				return ErrImportProfileExists
			}
		}
		_, err = tx.Put(CollImportProfiles, profile.ID, profile)
		return err
	})
	if err != nil {
		return ImportProfile{}, err
	}
	return profile, nil
}

func (p *Profile) DeleteImportProfile(id string) error {
	if p.closed.Load() || p.app.exiting.Load() {
		return ErrImportScope
	}
	done := p.app.begin("import-profile")
	defer done()
	return p.St.Update(func(tx *store.Tx) error {
		if _, err := tx.GetRecord(CollImportProfiles, id); err != nil {
			return err
		}
		return tx.Delete(CollImportProfiles, id)
	})
}
