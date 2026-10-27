package app

import (
	"net/http"

	"github.com/oaovito/mne_lab/internal/science/units"
)

// measurementUnitRoutes serves the measurement-unit registry (Settings →
// Manage Units and the unit selector). Every request is bound to the open
// Account and Profile, like the other profile-scoped scientific records.
func (s *Server) measurementUnitRoutes() {
	scope := func(r *http.Request) (*Profile, error) {
		p, err := s.app.Profile()
		if err != nil {
			return nil, err
		}
		if p.closed.Load() || r.Header.Get("X-Account-ID") != p.acct.ID() || r.Header.Get("X-Profile-ID") != p.Entry.ID {
			return nil, ErrImportScope
		}
		return p, nil
	}
	changed := func(v any, err error) (any, error) {
		if err == nil {
			s.app.hub.Publish("library", nil)
		}
		return v, err
	}
	// ?q= searches symbol and name; ?dimension= or ?field= (a parameter key)
	// keeps compatible units; ?archived=1 includes hidden units; ?usage=1
	// adds the number of objects using each unit.
	s.handle("GET /api/measurement-units", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		v := r.URL.Query()
		q := UnitQuery{Search: v.Get("q"), Dimension: units.Dimension(v.Get("dimension")), IncludeArchived: v.Get("archived") == "1", Usage: v.Get("usage") == "1"}
		if f := v.Get("field"); f != "" {
			d, ok := units.FieldDimension(f)
			if !ok {
				return nil, units.ErrDimension
			}
			q.Dimension = d
		}
		if q.Dimension != "" && !units.ValidDimension(q.Dimension) {
			return nil, units.ErrDimension
		}
		return p.MeasurementUnits(q)
	})
	s.handle("GET /api/measurement-units/dimensions", func(w http.ResponseWriter, r *http.Request) (any, error) {
		if _, err := scope(r); err != nil {
			return nil, err
		}
		return units.Dimensions, nil
	})
	s.handle("GET /api/measurement-units/replacements", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		return p.MeasurementUnitReplacements()
	})
	s.handle("POST /api/measurement-units", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		var in struct {
			Symbol    string          `json:"symbol"`
			Name      string          `json:"name"`
			Dimension units.Dimension `json:"dimension"`
			// Field (a parameter key) gives the dimension when created from
			// a unit selector.
			Field string `json:"field"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		if in.Field != "" {
			d, ok := units.FieldDimension(in.Field)
			if !ok || in.Dimension != "" && in.Dimension != d {
				return nil, units.ErrDimension
			}
			in.Dimension = d
		}
		return changed(p.CreateMeasurementUnit(in.Symbol, in.Name, in.Dimension))
	})
	s.handle("GET /api/measurement-units/{id}/usage", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		return p.MeasurementUnitUsage(pathID(r))
	})
	for _, action := range []string{"archive", "restore"} {
		archived := action == "archive"
		s.handle("POST /api/measurement-units/{id}/"+action, func(w http.ResponseWriter, r *http.Request) (any, error) {
			p, err := scope(r)
			if err != nil {
				return nil, err
			}
			return changed(p.ArchiveMeasurementUnit(pathID(r), archived))
		})
	}
	s.handle("POST /api/measurement-units/{id}/favorite", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		var in struct {
			Favorite bool `json:"favorite"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return changed(p.FavoriteMeasurementUnit(pathID(r), in.Favorite))
	})
	s.handle("POST /api/measurement-units/{id}/used", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		return nil, p.MarkMeasurementUnitUsed(pathID(r))
	})
	// DELETE removes an unused custom unit. For a used one nothing is
	// deleted; the answer carries the usage and the three options.
	s.handle("DELETE /api/measurement-units/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		res, err := p.DeleteMeasurementUnit(pathID(r))
		if err == nil && res.Deleted {
			s.app.hub.Publish("library", nil)
		}
		return res, err
	})
	s.handle("POST /api/measurement-units/{id}/replace", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		var in struct {
			Replacement string `json:"replacement"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return changed(p.ReplaceMeasurementUnit(pathID(r), in.Replacement))
	})
}
