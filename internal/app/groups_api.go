package app

import (
	"net/http"
)

// groupRoutes serves groups and saved selections, and the combination of
// saved graphs. Every request names the open Account and Profile, so a
// request from another profile's page is refused.
func (s *Server) groupRoutes() {
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
	s.handle("GET /api/groups", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		return p.Groups()
	})
	s.handle("GET /api/groups/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		return p.Group(pathID(r))
	})
	s.handle("POST /api/groups", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		var g Group
		if err := decode(r, &g); err != nil {
			return nil, err
		}
		out, err := p.SaveGroup(g)
		if err == nil {
			s.app.hub.Publish("library", nil)
		}
		return out, err
	})
	s.handle("DELETE /api/groups/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		err = p.DeleteGroup(pathID(r))
		if err == nil {
			s.app.hub.Publish("library", nil)
		}
		return nil, err
	})

	type combineIn struct {
		IDs         []string `json:"ids"`
		Title       string   `json:"title"`
		DisplayUnit string   `json:"displayUnit"`
	}
	// Check reports whether graphs can be combined, why not, and the exact
	// display-unit conversions offered when their units differ.
	s.handle("POST /api/graphs/combine/check", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		var in combineIn
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		return p.CheckCombine(in.IDs, in.DisplayUnit)
	})
	// Combine creates a new graph from the parents' definitions and data;
	// the parents are only read. An incompatible combination is refused
	// (the interface shows the check's reasons before offering it).
	s.handle("POST /api/graphs/combine", func(w http.ResponseWriter, r *http.Request) (any, error) {
		p, err := scope(r)
		if err != nil {
			return nil, err
		}
		var in combineIn
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		def, ck, err := p.CombineGraphs(in.IDs, in.Title, in.DisplayUnit)
		if err != nil {
			return nil, err
		}
		s.app.hub.Publish("library", nil)
		return map[string]any{"graph": def, "check": ck}, nil
	})
}
