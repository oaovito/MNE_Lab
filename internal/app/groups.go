package app

// Groups and saved selections (addendum B §23-24, A §28-29). A group names
// a set of measurements ("F-SR" holds F-SR A, B and C); a saved selection
// keeps a set of series chosen in the Series Explorer. Both store
// references only: identifiers of measurements or series, never values, so
// a group never duplicates raw data and always reads the current records.
// A graph pins the member list of its groups when it is saved (graph
// GroupSnapshots), so editing a group later never changes a saved graph;
// the change is reported until the person chooses the new members.
// Groups sync with the profile like every other library object.

import (
	"errors"
	"sort"
	"time"

	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/series"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
)

// Group kinds.
const (
	GroupKindGroup     = "group"
	GroupKindSelection = "selection"
)

// MaxGroupMembers bounds a group so it stays a reference list.
const MaxGroupMembers = 2000

// Errors (stable identifiers).
var (
	ErrGroupName    = errors.New("groups.name_required")
	ErrGroupEmpty   = errors.New("groups.empty")
	ErrGroupMember  = errors.New("groups.unknown_measurement")
	ErrGroupTooBig  = errors.New("groups.limit_reached")
	ErrGroupInUse   = errors.New("groups.in_use")
	ErrGroupMissing = errors.New("groups.not_found")
)

// Group is a saved group or selection.
type Group struct {
	Schema int    `json:"schema"`
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	// Measurements are the members (references). For a selection they are
	// derived from SeriesIDs on save.
	Measurements []string `json:"measurements"`
	// SeriesIDs is the saved selection of series (stable catalog IDs).
	SeriesIDs []string  `json:"seriesIds,omitempty"`
	Notes     string    `json:"notes,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
}

// SaveGroup creates or updates a group. Every member must be a
// measurement of this profile; no value is copied.
func (p *Profile) SaveGroup(g Group) (Group, error) {
	g.Name = clean(g.Name, 120)
	if g.Name == "" {
		return g, ErrGroupName
	}
	g.Notes = clean(g.Notes, 4000)
	g.Tags = cleanTags(g.Tags)
	g.Schema = 1
	if len(g.SeriesIDs) > 0 {
		g.Kind = GroupKindSelection
		if len(g.SeriesIDs) > graph.MaxSeries {
			return g, ErrGroupTooBig
		}
		seen := map[string]bool{}
		var ids []string
		for _, id := range g.SeriesIDs {
			if _, err := series.Parse(id); err != nil || len(id) > 400 {
				return g, graph.ErrInvalidSeries
			}
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		g.SeriesIDs = ids
		g.Measurements = graph.SelectionMeasurements(ids)
	} else {
		g.Kind = GroupKindGroup
		g.SeriesIDs = nil
		g.Measurements = uniqueMeasurementIDs(g.Measurements)
	}
	if len(g.Measurements) == 0 {
		return g, ErrGroupEmpty
	}
	if len(g.Measurements) > MaxGroupMembers {
		return g, ErrGroupTooBig
	}
	now := time.Now().UTC()
	err := p.St.Update(func(t *store.Tx) error {
		for _, id := range g.Measurements {
			var m model.Measurement
			if _, err := t.Get(CollMeasurements, id, &m); err != nil {
				return ErrGroupMember
			}
		}
		if g.ID == "" {
			g.ID, g.Created = secure.NewID(), now
		} else {
			var old Group
			if _, err := t.Get(CollGroups, g.ID, &old); err == nil {
				g.Created = old.Created
			} else {
				g.Created = now
			}
		}
		g.Updated = now
		_, err := t.Put(CollGroups, g.ID, g)
		return err
	})
	return g, err
}

// Groups lists the saved groups and selections, newest first.
func (p *Profile) Groups() ([]Group, error) {
	out := []Group{}
	err := p.St.View(func(t *store.Tx) error {
		recs, err := t.List(CollGroups)
		for _, r := range recs {
			var g Group
			if decodeRecord(r, &g) == nil && g.ID != "" {
				out = append(out, g)
			}
		}
		return err
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	return out, err
}

// Group loads one group.
func (p *Profile) Group(id string) (Group, error) {
	var g Group
	err := p.St.View(func(t *store.Tx) error {
		_, err := t.Get(CollGroups, id, &g)
		if errors.Is(err, store.ErrNotFound) {
			return ErrGroupMissing
		}
		return err
	})
	return g, err
}

// DeleteGroup removes a group no graph reads. Cycles and analyses built
// from a group keep their own references, so they do not block it.
func (p *Profile) DeleteGroup(id string) error {
	return p.St.Update(func(t *store.Tx) error {
		var g Group
		if _, err := t.Get(CollGroups, id, &g); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return ErrGroupMissing
			}
			return err
		}
		recs, err := t.List(CollGraphs)
		if err != nil {
			return err
		}
		for _, r := range recs {
			var d graph.Definition
			if decodeRecord(r, &d) == nil && contains(d.Groups, id) {
				return ErrGroupInUse
			}
		}
		return t.Delete(CollGroups, id)
	})
}
