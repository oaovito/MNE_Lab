package app

import (
	"net/http"

	"github.com/oaovito/mne_lab/internal/science/analysis"
)

func (s *Server) statisticsRoutes() {
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
	s.handle("GET /api/experimental-units", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		return p.ExperimentalUnits()
	})
	s.handle("POST /api/experimental-units", func(w http.ResponseWriter, r *http.Request) (any, error) {
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
		unit, err := p.CreateExperimentalUnit(in.Label)
		if err == nil {
			s.app.hub.Publish("library", nil)
		}
		return unit, err
	})
	s.handle("POST /api/statistics/prepare", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		var in analysis.Definition
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return p.PrepareAnalysis(in)
	})
	s.handle("POST /api/statistics", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		var in struct {
			Definition analysis.Definition `json:"definition"`
			Receipt    string              `json:"receipt"`
			Results    analysis.Results    `json:"results"`
			PreviousID string              `json:"previousId"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		value, err := p.SaveAnalysis(in.Definition, in.Receipt, in.Results, in.PreviousID)
		if err == nil {
			s.app.hub.Publish("library", nil)
		}
		return value, err
	})
	s.handle("GET /api/statistics", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		return p.Analyses()
	})
	s.handle("GET /api/statistics/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		return p.Analysis(pathID(r))
	})
}
