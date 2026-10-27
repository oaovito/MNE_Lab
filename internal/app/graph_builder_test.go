package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/oaovito/mne_lab/internal/plot"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/series"
	"github.com/oaovito/mne_lab/internal/store"
)

// importTwenty imports the synthetic twenty-series file and returns its
// intensity curves (one per measurement) from the series catalog.
func importTwenty(t *testing.T, p *Profile) []series.Series {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-twenty-series.txt"))
	if err != nil {
		t.Fatal(err)
	}
	r := p.Import("synthetic twenty series.txt", data, false)
	if r.Error != "" || r.Measurements != 20 {
		t.Fatalf("%+v", r)
	}
	cat, err := p.Series(SeriesQuery{FileID: r.FileID})
	if err != nil {
		t.Fatal(err)
	}
	var curves []series.Series
	for _, s := range cat.Series {
		if s.Kind == series.KindCurve && s.Y.Key == "intensity" {
			curves = append(curves, s)
		}
	}
	if len(curves) != 20 {
		t.Fatalf("%d intensity curves", len(curves))
	}
	return curves
}

func graphRecord(t *testing.T, p *Profile, id string) []byte {
	t.Helper()
	var b []byte
	if err := p.St.View(func(tx *store.Tx) error {
		r, err := tx.GetRecord(CollGraphs, id)
		b = append([]byte(nil), r.Data...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return b
}

// B §58: of twenty series, three are chosen, drawn, edited, saved and
// reopened; the other seventeen stay in the catalog and every record is
// byte for byte what it was.
func TestTwentySeriesSelectThreeKeepsTheRest(t *testing.T) {
	_, _, p := inspectionProfile(t)
	curves := importTwenty(t, p)
	before := measurementBytes(t, p)

	pick := []string{curves[2].ID, curves[7].ID, curves[11].ID}
	def, err := p.SaveGraph(graph.Definition{Kind: graph.KindDistribution, Title: "Three of twenty", SeriesIDs: pick, Visual: graph.DefaultVisual()})
	if err != nil {
		t.Fatal(err)
	}
	if def.Schema != graph.DefinitionSchema || len(def.Measurements) != 3 || def.Weighting != "intensity" || def.CalculationVersion != graph.EngineVersion || len(def.SourceFiles) != 1 || len(def.ParserVersions) != 1 {
		t.Fatalf("derived fields: %+v", def)
	}
	res, err := p.Compute(def)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 3 {
		t.Fatalf("%d series drawn", len(res.Series))
	}
	for i, s := range res.Series {
		if s.ID != pick[i] || s.SeriesID != pick[i] {
			t.Fatalf("series %d is %s", i, s.ID)
		}
	}

	// Edit the selection: drop one, add another, then save and reopen.
	def.SeriesIDs = []string{pick[0], pick[2], curves[19].ID}
	def.Series = map[string]graph.SeriesStyle{curves[19].ID: {Label: "Last one", Marker: "diamond"}}
	if def, err = p.SaveGraph(def); err != nil {
		t.Fatal(err)
	}
	reopened, err := p.Graph(def.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reopened.SeriesIDs, []string{pick[0], pick[2], curves[19].ID}) {
		t.Fatalf("reopened selection %v", reopened.SeriesIDs)
	}
	res, err = p.Compute(reopened)
	if err != nil || len(res.Series) != 3 || res.Series[2].Label != "Last one" {
		t.Fatalf("%v %+v", err, res.Series)
	}

	cat, err := p.Series(SeriesQuery{})
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, s := range cat.Series {
		listed[s.ID] = true
	}
	chosen := map[string]bool{}
	for _, id := range reopened.SeriesIDs {
		chosen[id] = true
	}
	rest := 0
	for _, c := range curves {
		if !chosen[c.ID] && listed[c.ID] {
			rest++
		}
	}
	if rest != 17 {
		t.Fatalf("%d unselected series still listed, want 17", rest)
	}
	after := measurementBytes(t, p)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("selecting series for a graph changed measurement records")
	}
}

func TestGraphDefinitionV2RoundTrip(t *testing.T) {
	_, _, p := inspectionProfile(t)
	curves := importTwenty(t, p)
	no, yes := false, true
	in := graph.Definition{Kind: graph.KindDistribution, Title: "Every field", Subtitle: "Synthetic", XLabel: "Diameter", YLabel: "Share",
		SeriesIDs: []string{curves[0].ID, curves[1].ID, curves[0].ID}, FocusSeries: []string{curves[1].ID, "m:gone/p:x"},
		YScale: "log", DisplayUnit: "µm", ShowIndividual: &no, ConnectMeans: &yes, ErrorBars: "sem",
		TextNotes: []graph.TextAnnotation{{Text: "  peak  ", X: 150, Y: 4}, {Text: " "}},
		Series:    map[string]graph.SeriesStyle{curves[0].ID: {Label: "First", Color: "#143A52", Marker: "square", LineStyle: "dashed"}},
		Visual:    graph.Visual{Legend: true, Grid: true, LineWidth: 2, FontScale: 1.1, LegendMode: "selected", LegendSeries: []string{curves[0].ID}, Margin: 6, Preset: "publication"},
		Tags:      []string{"syn"}, Notes: "n",
		// A client cannot write lineage.
		DerivedFrom: []graph.Lineage{{GraphID: "forged", Relation: "duplicate"}}}
	saved, err := p.SaveGraph(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.DerivedFrom) != 0 {
		t.Fatal("a client-supplied lineage was stored")
	}
	if len(saved.SeriesIDs) != 2 || len(saved.FocusSeries) != 1 || len(saved.TextNotes) != 1 || saved.TextNotes[0].Text != "peak" {
		t.Fatalf("normalized: %+v", saved)
	}
	got, err := p.Graph(saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(saved)
	b, _ := json.Marshal(got)
	if !bytes.Equal(a, b) {
		t.Fatalf("round trip differs\n%s\n%s", a, b)
	}
	for _, field := range []string{"seriesIds", "focusSeries", "subtitle", "xLabel", "yLabel", "yScale", "displayUnit", "showIndividual", "connectMeans", "textNotes", "sourceFiles", "parserVersions", "calculationVersion", "margin", "preset", "legendMode", "legendSeries"} {
		if !strings.Contains(string(b), `"`+field+`"`) {
			t.Errorf("field %s not stored", field)
		}
	}
	// Display options reach the computed graph without touching the data.
	res, err := p.Compute(got)
	if err != nil {
		t.Fatal(err)
	}
	if res.Subtitle != "Synthetic" || res.X.Title != "Diameter" || !res.Series[0].Muted || res.Series[1].Muted || len(res.TextNotes) != 1 {
		t.Fatalf("%+v", res)
	}
	// Percent cannot be shown in µm, and a distribution has zeros: both are
	// said, never forced.
	warn := strings.Join(res.Warnings, " ")
	if !strings.Contains(warn, "graph.display_unit_not_converted") || !strings.Contains(warn, "graph.log_needs_positive_values") {
		t.Fatalf("warnings %v", res.Warnings)
	}
}

func TestMissingSeriesWarnsInsteadOfFailing(t *testing.T) {
	_, _, p := inspectionProfile(t)
	curves := importTwenty(t, p)
	def := graph.Definition{Kind: graph.KindDistribution, SeriesIDs: []string{curves[0].ID, "m:deleted/d:table/w:intensity", series.CurveID(curves[1].MeasurementID, "no-such-candidate", "intensity")}, Visual: graph.DefaultVisual()}
	res, err := p.Compute(def)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 1 || len(res.Warnings) < 2 || !strings.HasPrefix(res.Warnings[0], "graph.missing_series:") {
		t.Fatalf("%d series, warnings %v", len(res.Series), res.Warnings)
	}
	def.SeriesIDs = []string{"m:deleted/d:table/w:intensity"}
	if _, err := p.Compute(def); !errors.Is(err, graph.ErrNoData) {
		t.Fatalf("nothing left to draw: %v", err)
	}
	// Mixed variables and the wrong kind of series are refused on save.
	if _, err := p.SaveGraph(graph.Definition{Kind: graph.KindDistribution, SeriesIDs: []string{curves[0].ID, series.CurveID(curves[1].MeasurementID, "table", "volume")}}); !errors.Is(err, graph.ErrMixedVariables) {
		t.Fatalf("mixed weightings: %v", err)
	}
	if _, err := p.SaveGraph(graph.Definition{Kind: graph.KindDistribution, SeriesIDs: []string{series.ScalarID(curves[0].MeasurementID, model.EffectiveDiameter)}}); !errors.Is(err, graph.ErrSeriesKind) {
		t.Fatalf("scalar in a distribution: %v", err)
	}
	if _, err := p.SaveGraph(graph.Definition{Kind: graph.KindDistribution, SeriesIDs: []string{"Synthetic S01"}}); !errors.Is(err, graph.ErrInvalidSeries) {
		t.Fatalf("a label is not a SeriesID: %v", err)
	}
}

// Instrument readings and summary values are drawn exactly as read, and a
// display unit keeps the stored values beside the converted ones.
func TestSeriesValuesReadingsAndDisplayUnit(t *testing.T) {
	_, _, p := inspectionProfile(t)
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "lightscattering", "synthetic-nanobrook-twelve-runs.txt"))
	if err != nil {
		t.Fatal(err)
	}
	r := p.Import("synthetic twelve runs.txt", data, false)
	cat, err := p.Series(SeriesQuery{FileID: r.FileID})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	want := map[string]float64{}
	for _, s := range cat.Series {
		if s.Kind == series.KindReading && s.Y.Key == model.EffectiveDiameter && len(ids) < 4 {
			ids = append(ids, s.ID)
			want[s.ID] = *s.Reading.Value.Value
		}
	}
	if len(ids) != 4 {
		t.Fatalf("%d readings", len(ids))
	}
	before := measurementBytes(t, p)
	def, err := p.SaveGraph(graph.Definition{Kind: graph.KindSeriesValues, SeriesIDs: ids, Visual: graph.DefaultVisual()})
	if err != nil {
		t.Fatal(err)
	}
	if def.Param != model.EffectiveDiameter {
		t.Fatalf("param %q", def.Param)
	}
	res, err := p.Compute(def)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 4 || len(res.X.Categories) != 4 || res.Y.Unit == "" {
		t.Fatalf("%+v", res)
	}
	for _, s := range res.Series {
		if s.Y[0] != want[s.ID] || !strings.HasPrefix(s.Label, "reading:") {
			t.Fatalf("%s drawn as %v, %q", s.ID, s.Y, s.Label)
		}
	}
	if res.Y.Unit != "nm" {
		t.Fatalf("the header states nm, the axis says %q", res.Y.Unit)
	}
	def.DisplayUnit = "µm"
	res, err = p.Compute(def)
	if err != nil {
		t.Fatal(err)
	}
	if res.Y.Unit != "µm" || res.Y.Conversion == nil || res.Y.Conversion.Factor != "1/1000" || res.Y.Conversion.From != "nm" {
		t.Fatalf("axis %+v", res.Y)
	}
	for _, s := range res.Series {
		if s.YOriginal[0] != want[s.ID] || s.Y[0] != want[s.ID]/1000 {
			t.Fatalf("%v %v", s.YOriginal, s.Y)
		}
	}
	// No exact factor, no conversion: the stored unit stays.
	def.DisplayUnit = "kcps"
	if res, err = p.Compute(def); err != nil || res.Y.Unit != "nm" || !strings.Contains(strings.Join(res.Warnings, " "), "graph.display_unit_not_converted") {
		t.Fatalf("%v %+v %v", err, res.Y, res.Warnings)
	}
	if !reflect.DeepEqual(before, measurementBytes(t, p)) {
		t.Fatal("drawing readings changed measurement records")
	}
	// A curve has no place in a values graph.
	if _, err := p.SaveGraph(graph.Definition{Kind: graph.KindSeriesValues, SeriesIDs: append(ids, series.CurveID(cat.Series[0].MeasurementID, "table", "intensity"))}); !errors.Is(err, graph.ErrSeriesKind) {
		t.Fatalf("curve in values graph: %v", err)
	}
}

func TestRenderStopsWhenCancelled(t *testing.T) {
	a, _, p := inspectionProfile(t)
	curves := importTwenty(t, p)
	var ids []string
	for _, c := range curves {
		ids = append(ids, c.ID)
	}
	def := graph.Definition{Kind: graph.KindDistribution, SeriesIDs: ids, Visual: graph.DefaultVisual()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := p.RenderContext(ctx, def, plot.Preset(plot.PresetScreen)); !errors.Is(err, ErrRenderCancelled) {
		t.Fatalf("cancelled render: %v", err)
	}
	// The render route passes the request's context through.
	body, _ := json.Marshal(map[string]any{"definition": def, "width": 800, "height": 500})
	req := httptest.NewRequest(http.MethodPost, "/api/graphs/render", bytes.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	start := time.Now()
	a.srv.mux.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "graph.render_cancelled") || time.Since(start) > 5*time.Second {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	// Not cancelled, the same request renders.
	rec = httptest.NewRecorder()
	a.srv.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/graphs/render", bytes.NewReader(body)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"figure"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String()[:200])
	}
}

// B §14: a duplicate is a new definition with its own ID and a Derived
// From record; editing it leaves the original byte for byte as it was.
func TestDuplicateRecordsLineageAndLeavesOriginal(t *testing.T) {
	_, c, p := inspectionProfile(t)
	curves := importTwenty(t, p)
	orig, err := p.SaveGraph(graph.Definition{Kind: graph.KindDistribution, Title: "Graph A", SeriesIDs: []string{curves[0].ID, curves[1].ID}, Tags: []string{"a"}, Visual: graph.DefaultVisual()})
	if err != nil {
		t.Fatal(err)
	}
	stored := graphRecord(t, p, orig.ID)

	var dup graph.Definition
	c.json("POST", "/api/graphs/"+orig.ID+"/duplicate", map[string]any{"title": "Graph A copy"}, &dup)
	if dup.ID == "" || dup.ID == orig.ID || dup.Title != "Graph A copy" {
		t.Fatalf("%+v", dup)
	}
	if len(dup.DerivedFrom) != 1 || dup.DerivedFrom[0].GraphID != orig.ID || dup.DerivedFrom[0].Relation != graph.RelationDuplicate || dup.DerivedFrom[0].DefinitionHash != graph.Hash(stored) {
		t.Fatalf("lineage %+v", dup.DerivedFrom)
	}
	if !reflect.DeepEqual(dup.SeriesIDs, orig.SeriesIDs) || !reflect.DeepEqual(dup.Tags, orig.Tags) {
		t.Fatal("the copy does not hold the same definition")
	}

	// Edit the copy: other series, style and error bars.
	dup.SeriesIDs = []string{curves[5].ID}
	dup.Series = map[string]graph.SeriesStyle{curves[5].ID: {Color: "#883717"}}
	dup.Visual.Grid = false
	dup.DerivedFrom = nil // a client cannot drop the lineage either
	edited, err := p.SaveGraph(dup)
	if err != nil {
		t.Fatal(err)
	}
	if len(edited.DerivedFrom) != 1 || edited.DerivedFrom[0].GraphID != orig.ID {
		t.Fatalf("lineage lost on save: %+v", edited.DerivedFrom)
	}
	if !bytes.Equal(stored, graphRecord(t, p, orig.ID)) {
		t.Fatal("editing the copy changed the original")
	}
	rel, err := p.Relations()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rel.Graphs[edited.ID].DerivedFrom, []string{orig.ID}) || !reflect.DeepEqual(rel.Graphs[orig.ID].Derived, []string{edited.ID}) {
		t.Fatalf("relations %+v %+v", rel.Graphs[edited.ID], rel.Graphs[orig.ID])
	}
	// A trashed graph is not duplicated.
	if err := p.TrashGraph(orig.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := p.DuplicateGraph(orig.ID, ""); !errors.Is(err, ErrGraphTrashed) {
		t.Fatalf("trashed: %v", err)
	}
}
