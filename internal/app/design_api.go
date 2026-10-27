package app

import (
	"net/http"

	"github.com/oaovito/mne_lab/internal/science/design"
)

// designRoutes serves experimental designs, the series catalog and unit
// renaming. Like statistics, every request names the open Account and
// Profile, so a request from another profile's page is refused.
func (s *Server) designRoutes() {
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
	s.handle("GET /api/series", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		q := r.URL.Query()
		return p.Series(SeriesQuery{FileID: q.Get("file"), DesignID: q.Get("design"), Q: q.Get("q")})
	})
	s.handle("GET /api/designs", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		return p.Designs()
	})
	s.handle("GET /api/designs/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		return p.Design(pathID(r))
	})
	s.handle("POST /api/designs", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		var in struct {
			Design     design.Design `json:"design"`
			PreviousID string        `json:"previousId"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		v, err := p.SaveDesign(in.Design, in.PreviousID)
		if err == nil {
			s.app.hub.Publish("library", nil)
		}
		return v, err
	})
	s.handle("POST /api/designs/review", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		var in struct {
			Design design.Design `json:"design"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return p.ReviewDesign(in.Design)
	})
	s.handle("POST /api/designs/suggest", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		var in struct {
			Design       design.Design `json:"design"`
			Measurements []string      `json:"measurements"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return p.SuggestDesign(in.Design, in.Measurements)
	})
	s.handle("PATCH /api/experimental-units/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		var in struct {
			Label string `json:"label"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		u, err := p.RenameExperimentalUnit(pathID(r), in.Label)
		if err == nil {
			s.app.hub.Publish("library", nil)
		}
		return u, err
	})
}
