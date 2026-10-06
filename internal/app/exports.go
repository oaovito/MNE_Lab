package app

import (
	"errors"
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/oaovito/mne_lab/internal/export"
	"github.com/oaovito/mne_lab/internal/paths"
	"github.com/oaovito/mne_lab/internal/plot"
	"github.com/oaovito/mne_lab/internal/science/graph"
	"github.com/oaovito/mne_lab/internal/science/model"
	"github.com/oaovito/mne_lab/internal/science/reference"
	"github.com/oaovito/mne_lab/internal/secure"
	"github.com/oaovito/mne_lab/internal/store"
	"github.com/oaovito/mne_lab/internal/version"
)

// Errors (stable identifiers).
var (
	ErrTempDestination = errors.New("export.destination_is_temporary")
	ErrNothing         = errors.New("export.nothing_selected")
)

// ExportItem is one thing to export.
type ExportItem struct {
	Kind string   `json:"kind"` // graph, dataset, cycle, file
	ID   string   `json:"id,omitempty"`
	IDs  []string `json:"ids,omitempty"` // dataset: measurement ids
}

// ExportRequest comes from the export dialog.
type ExportRequest struct {
	Items       []ExportItem       `json:"items"`
	Formats     []string           `json:"formats"`
	Preset      string             `json:"preset"`
	Figure      plot.Spec          `json:"figure"`
	Data        export.DataOptions `json:"data"`
	Destination string             `json:"destination"`
	Name        string             `json:"name"`
	Collision   string             `json:"collision"`
}

// ExportResult is what was written, or the name that already exists.
type ExportResult struct {
	Saved    []export.Saved `json:"saved,omitempty"`
	Conflict string         `json:"conflict,omitempty"`
	Suggest  string         `json:"suggest,omitempty"` // a free alternative name
}

// Estimate summarizes an export before it runs.
type Estimate struct {
	Files       int        `json:"files"`
	Bytes       int64      `json:"bytes"`
	Pixels      [2]int     `json:"pixels,omitempty"`
	DPI         float64    `json:"dpi,omitempty"`
	SizeMM      [2]float64 `json:"sizeMm,omitempty"`
	Name        string     `json:"name"`
	Zip         bool       `json:"zip"`
	Destination string     `json:"destination"`
	Exists      bool       `json:"exists,omitempty"` // a file with this name is already there
	// NameAdjusted tells that characters the file system refuses were
	// replaced in the typed name.
	NameAdjusted bool     `json:"nameAdjusted,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

type output struct {
	name string // with extension
	role string
	data []byte
}

func (a *App) decimal() string {
	if p, err := a.Profile(); err == nil && p.Settings().Decimal != "" {
		return p.Settings().Decimal
	}
	switch a.Lang() {
	case "pt-BR", "es":
		return ","
	}
	return "."
}

// ExportDestinations lists suggested folders: the default export folder
// and the person's usual folders; cloud desktop folders when present.
func (a *App) ExportDestinations() []Destination {
	var out []Destination
	add := func(id, p string) {
		if p == "" {
			return
		}
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, Destination{ID: id, Path: p})
		} else if id == "default" {
			out = append(out, Destination{ID: id, Path: p, Create: true})
		}
	}
	add("default", a.L.Exports)
	if home, err := os.UserHomeDir(); err == nil {
		add("desktop", filepath.Join(home, "Desktop"))
		add("documents", filepath.Join(home, "Documents"))
		add("downloads", filepath.Join(home, "Downloads"))
	}
	for id, p := range providerFolders() {
		add("cloud."+id, p)
	}
	for _, d := range removableDrives() {
		add("drive", d)
	}
	return out
}

// Destination is a suggested export folder.
type Destination struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Create bool   `json:"create,omitempty"`
}

func (a *App) checkDestination(dir string) (string, error) {
	if dir == "" {
		dir = a.L.Exports
	}
	dir = filepath.Clean(dir)
	if !filepath.IsAbs(dir) {
		return "", export.ErrDestination
	}
	// Exports never go where Temporary Mode cleanup deletes.
	if a.L.Mode == paths.Temporary && paths.Within(a.L.Root, dir) {
		return "", ErrTempDestination
	}
	if dir == a.L.Exports {
		os.MkdirAll(dir, 0o755)
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", export.ErrDestination
	}
	return dir, nil
}

// WithVisual applies a graph's own display choices (legend, grid,
// metadata, line width, text size) to an output preset. An explicit figure
// configuration from the export dialog is used as given instead.
func WithVisual(s plot.Spec, v graph.Visual) plot.Spec {
	s.Legend, s.Grid, s.Metadata = v.Legend, v.Grid, v.Metadata
	s.Points = s.Points || v.Points
	if v.LineWidth > 0 {
		s.LineWidth *= v.LineWidth / graph.DefaultVisual().LineWidth
	}
	if v.FontScale > 0 {
		s.FontSize *= v.FontScale
	}
	return s
}

// figureSpec returns the requested figure configuration.
func (a *App) figureSpec(req ExportRequest) plot.Spec {
	s := req.Figure
	if s.Width == 0 || s.Height == 0 {
		preset := plot.PresetScreen
		switch req.Preset {
		case export.PresetPublication:
			preset = plot.PresetPublication
		case export.PresetPresentation:
			preset = plot.PresetPresentation
		}
		s = plot.Preset(preset)
	}
	if s.Decimal == "" {
		s.Decimal = a.decimal()
	}
	return s
}

// Export runs an export request.
func (a *App) Export(req ExportRequest) (ExportResult, error) {
	p, err := a.Profile()
	if err != nil {
		return ExportResult{}, err
	}
	done := a.begin("export")
	defer done()
	if len(req.Items) == 0 || len(req.Formats) == 0 {
		return ExportResult{}, ErrNothing
	}
	dir, err := a.checkDestination(req.Destination)
	if err != nil {
		return ExportResult{}, err
	}
	spec := a.figureSpec(req)
	outs, base, err := p.produce(req, spec)
	if err != nil {
		return ExportResult{}, err
	}
	if req.Name != "" {
		base = export.Sanitize(req.Name)
	}
	var name string
	var write func(io.Writer) error
	var size int64
	if len(outs) == 1 && len(req.Items) == 1 {
		o := outs[0]
		name = export.WithExt(base, filepath.Ext(o.name))
		write, size = export.Bytes(o.data), int64(len(o.data))
	} else {
		ar := export.NewArchive(base, export.KindBatch, base, version.String(), version.Schema, time.Now())
		if req.Items[0].Kind == export.KindCycle && contains(req.Formats, "package") {
			ar.Kind = export.KindCycle
		}
		for _, o := range outs {
			dir, file := filepath.Split(o.name)
			ar.Add(strings.TrimSuffix(filepath.ToSlash(dir), "/"), file, o.role, o.data)
		}
		ar.Readme = p.readme(base)
		name = export.WithExt(base, ".zip")
		size = ar.Size()
		write = func(w io.Writer) error {
			_, err := ar.WriteZip(w)
			return err
		}
	}
	saved, err := export.Save(dir, name, req.Collision, size, write)
	if errors.Is(err, export.ErrExists) {
		alt := export.KeepBothName(name, func(n string) bool { return export.Exists(dir, n) })
		return ExportResult{Conflict: name, Suggest: alt}, nil
	}
	if err != nil {
		return ExportResult{}, err
	}
	p.recordExport(req, []export.Saved{saved}, dir)
	return ExportResult{Saved: []export.Saved{saved}}, nil
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func (p *Profile) recordExport(req ExportRequest, saved []export.Saved, dir string) {
	var names []string
	for _, s := range saved {
		names = append(names, s.Name)
	}
	p.St.Update(func(t *store.Tx) error {
		_, err := t.Put(CollExports, secure.NewID(), map[string]any{"at": time.Now().UTC(), "items": req.Items, "formats": req.Formats,
			"preset": req.Preset, "names": names, "folder": dir})
		return err
	})
	p.UpdateSettings(func(s *ProfileSettings) {
		if s.Export.Formats == nil {
			s.Export.Formats = map[string]string{}
		}
		s.Export.Preset = req.Preset
		s.Export.Formats[req.Items[0].Kind] = strings.Join(req.Formats, ",")
		if req.Figure.Width > 0 {
			f := req.Figure
			s.Export.Figure = &f
		}
		s.Export.CSVDecimal = req.Data.Decimal
		s.Export.Destination = dir
	})
}

// ExportHistory lists recent exports, newest first.
func (p *Profile) ExportHistory() ([]map[string]any, error) {
	var out []map[string]any
	err := p.St.View(func(t *store.Tx) error {
		recs, err := t.List(CollExports)
		store.SortByUpdated(recs)
		for i, r := range recs {
			if i >= 100 {
				break
			}
			var m map[string]any
			if decodeRecord(r, &m) == nil {
				m["id"] = r.ID
				out = append(out, m)
			}
		}
		return err
	})
	return out, err
}

// produce generates every output of a request (in memory) and a base name.
func (p *Profile) produce(req ExportRequest, spec plot.Spec) ([]output, string, error) {
	var outs []output
	base := ""
	multi := len(req.Items) > 1
	for _, it := range req.Items {
		o, b, err := p.produceItem(it, req, spec, multi)
		if err != nil {
			return nil, "", err
		}
		if base == "" {
			base = b
		}
		outs = append(outs, o...)
	}
	if multi {
		base = export.Sanitize(base + "_" + p.app.T("export.batch_suffix", "n", fmt.Sprint(len(req.Items))))
	}
	if len(outs) == 0 {
		return nil, "", ErrNothing
	}
	return outs, base, nil
}

func (p *Profile) produceItem(it ExportItem, req ExportRequest, spec plot.Spec, multi bool) ([]output, string, error) {
	T := p.app.T
	switch it.Kind {
	case export.KindGraph:
		def, err := p.Graph(it.ID)
		if err != nil {
			return nil, "", err
		}
		if req.Figure.Width == 0 {
			spec = WithVisual(spec, def.Visual)
		}
		return p.graphOutputs(def, req.Formats, spec, req.Data, multi)
	case export.KindDataset:
		in, err := p.Input(it.IDs)
		if err != nil {
			return nil, "", err
		}
		ds := export.MeasurementData(T("export.dataset_title"), it.IDs, in, nil, T)
		ds.AddCommonMeta(T, version.String(), version.Schema)
		base := export.Suggest(export.NameInfo{Samples: samplesOf(in), Module: "DLS", Subject: "Data", Date: singleDate(in)})
		var outs []output
		for _, f := range req.Formats {
			if f == "original" {
				for fid := range in.Files {
					o, err := p.originalOutput(fid, multi || len(in.Files) > 1)
					if err != nil {
						return nil, "", err
					}
					outs = append(outs, o)
				}
				continue
			}
			if !export.Compatible(export.KindDataset, f) {
				return nil, "", export.ErrFormat
			}
			b, err := export.WriteData(ds, f, req.Data)
			if err != nil {
				return nil, "", err
			}
			outs = append(outs, output{name: base + export.Formats[f].Ext, role: "data", data: b})
		}
		return outs, base, nil
	case export.KindFile:
		var outs []output
		f, _, err := p.File(it.ID)
		if err != nil {
			return nil, "", err
		}
		base := strings.TrimSuffix(f.Name, filepath.Ext(f.Name))
		for _, format := range req.Formats {
			if format == "original" {
				o, err := p.originalOutput(it.ID, multi)
				if err != nil {
					return nil, "", err
				}
				outs = append(outs, o)
				continue
			}
			sub, _, err := p.produceItem(ExportItem{Kind: export.KindDataset, IDs: f.Measurements}, ExportRequest{Formats: []string{format}, Data: req.Data}, spec, multi)
			if err != nil {
				return nil, "", err
			}
			for i := range sub {
				sub[i].name = base + filepath.Ext(sub[i].name)
			}
			outs = append(outs, sub...)
		}
		return outs, base, nil
	case export.KindCycle:
		return p.cycleOutputs(it.ID, req, spec, multi)
	}
	return nil, "", export.ErrFormat
}

func (p *Profile) originalOutput(fileID string, inFolder bool) (output, error) {
	f, b, err := p.Original(fileID)
	if err != nil {
		return output{}, err
	}
	name := f.Name
	if inFolder {
		name = export.DirOriginals + "/" + f.Name
	}
	return output{name: name, role: "original", data: b}, nil
}

func samplesOf(in graph.Input) []string {
	var s []string
	for _, m := range in.Measurements {
		s = append(s, m.SampleID)
	}
	return s
}

func singleDate(in graph.Input) time.Time {
	var d time.Time
	for _, m := range in.Measurements {
		if m.MeasuredAt == nil {
			continue
		}
		day := m.MeasuredAt.Time.Truncate(24 * time.Hour)
		if d.IsZero() {
			d = day
		} else if !d.Equal(day) {
			return time.Time{}
		}
	}
	return d
}

func (p *Profile) graphName(def graph.Definition, res graph.Result, in graph.Input) string {
	n := export.NameInfo{Title: def.Title, Samples: samplesOf(in), Module: "DLS"}
	switch def.Kind {
	case graph.KindDistribution:
		n.Date = singleDate(in)
	case graph.KindParameterTime:
		n.Subject = export.ParamToken(def.Param)
		if cy, err := p.Cycle(def.CycleID); err == nil {
			_, n.Duration = export.CycleTokens(string(cy.Config.Unit), 0, cy.Config.Duration)
		}
	case graph.KindDistributionAt:
		if cy, err := p.Cycle(def.CycleID); err == nil {
			_, n.Duration = export.CycleTokens(string(cy.Config.Unit), 0, cy.Config.Duration)
			n.Subject = "DLS_Cycle"
		}
	}
	return export.Suggest(n)
}

// graphOutputs renders a graph and/or its data in the requested formats.
func (p *Profile) graphOutputs(def graph.Definition, formats []string, spec plot.Spec, dopt export.DataOptions, multi bool) ([]output, string, error) {
	T := p.app.T
	res, err := p.Compute(def)
	if err != nil {
		return nil, "", err
	}
	ids := def.Measurements
	var ci *graph.CycleInput
	var in graph.Input
	if def.Kind != graph.KindDistribution {
		cy, err := p.Cycle(def.CycleID)
		if err != nil {
			return nil, "", err
		}
		var c graph.CycleInput
		in, c, err = p.cycleInput(cy)
		if err != nil {
			return nil, "", err
		}
		ci = &c
		ids = cy.Measurements
	} else if in, err = p.Input(ids); err != nil {
		return nil, "", err
	}
	base := p.graphName(def, res, in)
	fig := plot.Layout(res, spec, T)
	var outs []output
	sub := ""
	if multi {
		sub = base + "/"
	}
	for _, f := range formats {
		info, ok := export.Formats[f]
		if !ok || !export.Compatible(export.KindGraph, f) {
			return nil, "", export.ErrFormat
		}
		var b []byte
		switch info.Group {
		case export.GroupFigure:
			b, err = renderFigure(fig, spec, f, fig.Title)
		case export.GroupData:
			var ds export.DataSet
			switch {
			case def.Kind == graph.KindParameterTime:
				ds = export.ParameterData(fig.Title, def.Param, in, *ci, T)
			case ci != nil:
				points := export.PointLabels{}
				for _, a := range ci.Assignments {
					if a.Point >= 0 && (a.Status == "auto" || a.Status == "confirmed") {
						points[a.MeasurementID] = ci.Points[a.Point]
					}
				}
				var assigned []string
				for id := range points {
					assigned = append(assigned, id)
				}
				sort.Strings(assigned)
				ds = export.MeasurementData(fig.Title, assigned, in, points, T)
			default:
				ds = export.MeasurementData(fig.Title, ids, in, nil, T)
			}
			ds.AddCommonMeta(T, version.String(), version.Schema, export.KV{Key: "graph", Label: T("meta.graph"), Value: def.ID})
			b, err = export.WriteData(ds, f, dopt)
		}
		if err != nil {
			return nil, "", err
		}
		outs = append(outs, output{name: sub + base + info.Ext, role: info.Group, data: b})
	}
	return outs, base, nil
}

func renderFigure(fig plot.Figure, spec plot.Spec, format, title string) ([]byte, error) {
	meta := export.ImageMeta{Title: title, Software: version.String(), Created: time.Now().UTC()}
	switch format {
	case "svg":
		return plot.SVG(fig, spec), nil
	case "pdf":
		return plot.PDF(fig, spec, meta.Created)
	}
	if spec.Background == "transparent" && !export.Formats[format].Alpha {
		spec.Background = "solid" // JPEG has no transparency: the chosen background is used
	}
	im, err := plot.Raster(fig, spec)
	if err != nil {
		return nil, err
	}
	var bg color.Color = color.White
	if spec.BackColor != "" {
		bg = plot.ParseColor(spec.BackColor)
	}
	return export.EncodeImage(im, format, spec.DPI, bg, meta)
}

// cycleOutputs builds cycle results, data, or the Complete Research Package.
func (p *Profile) cycleOutputs(id string, req ExportRequest, spec plot.Spec, multi bool) ([]output, string, error) {
	T := p.app.T
	cy, err := p.Cycle(id)
	if err != nil {
		return nil, "", err
	}
	in, ci, err := p.cycleInput(cy)
	if err != nil {
		return nil, "", err
	}
	samples := []string{cy.Config.SampleID}
	if cy.Config.SampleID == "" {
		samples = samplesOf(in)
	}
	_, total := export.CycleTokens(string(cy.Config.Unit), 0, cy.Config.Duration)
	base := export.Suggest(export.NameInfo{Samples: samples, Subject: "Stability", Duration: total, Title: cy.Config.Name})
	var outs []output
	pkg := contains(req.Formats, "package")
	figs := func(formats []string, s plot.Spec, dir string) ([]output, error) {
		var outs []output
		for _, param := range cy.Config.Params {
			def := graph.Definition{Kind: graph.KindParameterTime, CycleID: cy.ID, Param: param, Visual: graph.DefaultVisual(),
				Title: T("param." + param)}
			res, err := graph.ParameterTime(def, in, ci)
			if err != nil {
				continue // parameter absent from these files
			}
			fig := plot.Layout(res, s, T)
			for _, f := range formats {
				b, err := renderFigure(fig, s, f, fig.Title)
				if err != nil {
					return nil, err
				}
				outs = append(outs, output{name: dir + export.ParamToken(param) + "_" + total + export.Formats[f].Ext, role: "graph", data: b})
			}
		}
		def := graph.Definition{Kind: graph.KindDistributionAt, CycleID: cy.ID, Visual: graph.DefaultVisual(), Title: T("graph.kind.dls_by_time")}
		if res, err := graph.DistributionAtPoints(def, in, ci); err == nil {
			fig := plot.Layout(res, s, T)
			for _, f := range formats {
				b, err := renderFigure(fig, s, f, fig.Title)
				if err != nil {
					return nil, err
				}
				outs = append(outs, output{name: dir + "DLS_Cycle_" + total + export.Formats[f].Ext, role: "graph", data: b})
			}
		}
		return outs, nil
	}
	sub := ""
	if multi {
		sub = base + "/"
	}
	if pkg {
		return p.researchPackage(cy, in, ci, base, figs, req.Data)
	}
	var figureFormats, dataFormats []string
	for _, f := range req.Formats {
		switch export.Formats[f].Group {
		case export.GroupFigure:
			figureFormats = append(figureFormats, f)
		case export.GroupData:
			dataFormats = append(dataFormats, f)
		default:
			return nil, "", export.ErrFormat
		}
	}
	if len(figureFormats) > 0 {
		g, err := figs(figureFormats, spec, sub+"graphs/")
		if err != nil {
			return nil, "", err
		}
		outs = append(outs, g...)
	}
	for _, f := range dataFormats {
		ds := cycleDataSet(cy, in, ci, T)
		b, err := export.WriteData(ds, f, req.Data)
		if err != nil {
			return nil, "", err
		}
		outs = append(outs, output{name: sub + base + export.Formats[f].Ext, role: "data", data: b})
		for _, param := range cy.Config.Params {
			pd := export.ParameterData(T("param."+param), param, in, ci, T)
			pd.AddCommonMeta(T, version.String(), version.Schema)
			pb, err := export.WriteData(pd, f, req.Data)
			if err != nil {
				return nil, "", err
			}
			outs = append(outs, output{name: sub + "statistics/" + export.ParamToken(param) + export.Formats[f].Ext, role: "statistics", data: pb})
		}
	}
	return outs, base, nil
}

func cycleDataSet(cy CycleDoc, in graph.Input, ci graph.CycleInput, T Translator) export.DataSet {
	points := export.PointLabels{}
	for _, a := range ci.Assignments {
		if a.Point >= 0 && (a.Status == "auto" || a.Status == "confirmed") {
			points[a.MeasurementID] = ci.Points[a.Point]
		}
	}
	ds := export.MeasurementData(cy.Config.Name, cy.Measurements, in, points, export.Translator(T))
	ds.AddCommonMeta(export.Translator(T), version.String(), version.Schema,
		export.KV{Key: "cycle", Label: T("meta.cycle"), Value: cy.Config.Name},
		export.KV{Key: "cycle_interval", Label: T("meta.cycle_interval"), Value: fmt.Sprintf("%d %s", cy.Config.Interval, T("unit."+string(cy.Config.Unit)))},
		export.KV{Key: "cycle_duration", Label: T("meta.cycle_duration"), Value: fmt.Sprintf("%d %s", cy.Config.Duration, T("unit."+string(cy.Config.Unit)))})
	return ds
}

// Translator is the shared translation function type.
type Translator = func(key string, kv ...string) string

// researchPackage assembles the Complete Research Package: everything
// needed to audit and reproduce the cycle later.
func (p *Profile) researchPackage(cy CycleDoc, in graph.Input, ci graph.CycleInput, base string, figs func([]string, plot.Spec, string) ([]output, error), dopt export.DataOptions) ([]output, string, error) {
	T := p.app.T
	var outs []output
	add := func(name, role string, b []byte) { outs = append(outs, output{name: name, role: role, data: b}) }
	addJSON := func(name, role string, v any) error {
		b, err := jsonIndent(v)
		if err != nil {
			return err
		}
		add(name, role, b)
		return nil
	}
	// Originals, byte for byte.
	var fileIDs []string
	for fid := range in.Files {
		fileIDs = append(fileIDs, fid)
	}
	sort.Strings(fileIDs)
	var files []model.SourceFile
	for _, fid := range fileIDs {
		f, b, err := p.Original(fid)
		if err != nil {
			return nil, "", err
		}
		files = append(files, f)
		add(export.DirOriginals+"/"+f.Name, "original", b)
	}
	// Metadata.
	var ms []model.Measurement
	for _, id := range cy.Measurements {
		if m, ok := in.Measurements[id]; ok {
			ms = append(ms, m)
		}
	}
	if err := addJSON(export.DirMetadata+"/cycle.json", "metadata", map[string]any{"cycle": cy, "points": ci.Points, "assignments": ci.Assignments}); err != nil {
		return nil, "", err
	}
	if err := addJSON(export.DirMetadata+"/files.json", "metadata", files); err != nil {
		return nil, "", err
	}
	// Normalized data.
	ds := cycleDataSet(cy, in, ci, T)
	for _, f := range []string{"csv", "xlsx", "json"} {
		b, err := export.WriteData(ds, f, dopt)
		if err != nil {
			return nil, "", err
		}
		add(export.DirNormalized+"/measurements"+export.Formats[f].Ext, "normalized-data", b)
	}
	if err := addJSON(export.DirNormalized+"/measurements.normalized.json", "normalized-data", ms); err != nil {
		return nil, "", err
	}
	// Statistics per tracked parameter.
	var defs []graph.Definition
	for _, param := range cy.Config.Params {
		pd := export.ParameterData(T("param."+param), param, in, ci, T)
		pd.AddCommonMeta(T, version.String(), version.Schema)
		for _, f := range []string{"csv", "json"} {
			b, err := export.WriteData(pd, f, dopt)
			if err != nil {
				return nil, "", err
			}
			add(export.DirStatistics+"/"+export.ParamToken(param)+export.Formats[f].Ext, "statistics", b)
		}
		defs = append(defs, graph.Definition{Schema: 1, Kind: graph.KindParameterTime, CycleID: cy.ID, Param: param, Visual: graph.DefaultVisual()})
	}
	defs = append(defs, graph.Definition{Schema: 1, Kind: graph.KindDistributionAt, CycleID: cy.ID, Visual: graph.DefaultVisual()})
	if saved, err := p.Graphs(false); err == nil {
		for _, d := range saved {
			if d.CycleID == cy.ID {
				defs = append(defs, d)
			}
		}
	}
	// Graphs: vector and lossless raster at publication quality.
	pub := plot.Preset(plot.PresetPublication)
	pub.Decimal = p.app.decimal()
	g, err := figs([]string{"svg", "pdf", "png"}, pub, export.DirGraphs+"/")
	if err != nil {
		return nil, "", err
	}
	outs = append(outs, g...)
	if err := addJSON(export.DirGraphs+"/definitions.json", "graph-definition", defs); err != nil {
		return nil, "", err
	}
	// Provenance.
	var sources []graph.Source
	for _, m := range ms {
		f := in.Files[m.FileID]
		s := graph.Source{MeasurementID: m.ID, FileID: m.FileID, FileName: f.Name, SHA256: f.SHA256, Parser: m.Parser, Spec: m.Spec, SampleID: m.SampleID, MeasuredAt: export.FormatTime(m.MeasuredAt)}
		if m.Dist != nil {
			s.Lines = fmt.Sprintf("%d-%d", m.Dist.FirstLine, m.Dist.LastLine)
		}
		sources = append(sources, s)
	}
	if err := addJSON(export.DirProvenance+"/provenance.json", "provenance", map[string]any{
		"software": version.String(), "schema": version.Schema, "graphEngine": graph.EngineVersion, "scientificSpec": reference.LightScatteringSpecID,
		"statistics": T("plot.meta.stats"), "rules": []string{"ls.fields", "ls.units", "ls.weighting", "ls.stats", "ls.cycles", "ls.dates"},
		"transformations": []string{"none: values exported exactly as read from the original files", "replicates grouped by cycle point; individual values kept"},
		"sources":         sources, "references": reference.Sources,
	}); err != nil {
		return nil, "", err
	}
	return outs, base, nil
}

func (p *Profile) readme(base string) string {
	T := p.app.T
	var b strings.Builder
	b.WriteString(base + "\r\n\r\n")
	b.WriteString(T("export.readme.intro", "software", version.String()) + "\r\n\r\n")
	for _, k := range []string{"manifest", "metadata", "original-files", "normalized-data", "graphs", "statistics", "provenance"} {
		b.WriteString(k + "/  " + T("export.readme."+k) + "\r\n")
	}
	b.WriteString("\r\n" + T("export.readme.verify") + "\r\n")
	return b.String()
}

// Preview lays out the figure of an export (identical to the final file)
// and estimates the result.
func (a *App) Preview(req ExportRequest) (plot.Figure, Estimate, error) {
	p, err := a.Profile()
	if err != nil {
		return plot.Figure{}, Estimate{}, err
	}
	spec := a.figureSpec(req)
	est := Estimate{DPI: spec.DPI}
	pw, ph := spec.Pixels()
	est.Pixels = [2]int{pw, ph}
	wpt, hpt := spec.SizePt()
	est.SizeMM = [2]float64{wpt / 72 * 25.4, hpt / 72 * 25.4}
	dir, derr := a.checkDestination(req.Destination)
	if derr == nil {
		est.Destination = dir
	} else {
		est.Warnings = append(est.Warnings, derr.Error())
	}
	var fig plot.Figure
	for i, it := range req.Items {
		if it.Kind == export.KindGraph && i == 0 {
			def, err := p.Graph(it.ID)
			if err != nil {
				return fig, est, err
			}
			if req.Figure.Width == 0 {
				spec = WithVisual(spec, def.Visual)
			}
			fig, _, err = p.Render(def, spec)
			if err != nil {
				return fig, est, err
			}
		}
	}
	for _, f := range req.Formats {
		info := export.Formats[f]
		if info.Warning != "" {
			est.Warnings = append(est.Warnings, info.Warning)
		}
		est.Files += len(req.Items)
		px := int64(pw) * int64(ph)
		switch f {
		case "png":
			est.Bytes += px / 9
		case "tiff":
			est.Bytes += px / 7
		case "jpeg":
			est.Bytes += px / 12
		case "webp":
			est.Bytes += px / 14
		case "svg":
			est.Bytes += int64(len(fig.Ops))*90 + 2048
		case "pdf":
			est.Bytes += int64(len(fig.Ops))*45 + 120<<10
		case "package":
			est.Bytes += 2 << 20
		default:
			est.Bytes += 64 << 10
		}
	}
	if err := plot.Spec.Validate(spec, true); err != nil {
		est.Warnings = append(est.Warnings, err.Error())
	}
	// Cycle exports always hold several files (one figure per parameter,
	// statistics tables), so they are delivered as one .zip.
	est.Zip = est.Files > 1 || contains(req.Formats, "package") || (len(req.Items) > 0 && req.Items[0].Kind == export.KindCycle)
	est.Name = p.suggestName(req, est.Zip)
	if req.Name != "" {
		ext := filepath.Ext(est.Name)
		est.Name = export.WithExt(export.Sanitize(req.Name), ext)
		est.NameAdjusted = export.Adjusted(strings.TrimSuffix(req.Name, filepath.Ext(req.Name)))
	}
	if est.Destination != "" && est.Name != "" {
		est.Exists = export.Exists(est.Destination, est.Name)
	}
	return fig, est, nil
}

// suggestName returns the file name an export would receive, computed
// without generating the content (the preview must stay light).
func (p *Profile) suggestName(req ExportRequest, zip bool) string {
	if len(req.Items) == 0 || len(req.Formats) == 0 {
		return ""
	}
	it := req.Items[0]
	base := ""
	switch it.Kind {
	case export.KindGraph:
		if def, err := p.Graph(it.ID); err == nil {
			in, _ := p.Input(def.Measurements)
			if def.CycleID != "" && len(def.Measurements) == 0 {
				if cy, err := p.Cycle(def.CycleID); err == nil {
					in, _ = p.Input(cy.Measurements)
				}
			}
			base = p.graphName(def, graph.Result{}, in)
		}
	case export.KindDataset:
		if in, err := p.Input(it.IDs); err == nil {
			base = export.Suggest(export.NameInfo{Samples: samplesOf(in), Module: "DLS", Subject: "Data", Date: singleDate(in)})
		}
	case export.KindFile:
		if f, _, err := p.File(it.ID); err == nil {
			if !zip && len(req.Formats) == 1 && req.Formats[0] == "original" {
				return f.Name
			}
			base = strings.TrimSuffix(f.Name, filepath.Ext(f.Name))
		}
	case export.KindCycle:
		if cy, err := p.Cycle(it.ID); err == nil {
			samples := []string{cy.Config.SampleID}
			if cy.Config.SampleID == "" {
				if in, err := p.Input(cy.Measurements); err == nil {
					samples = samplesOf(in)
				}
			}
			_, total := export.CycleTokens(string(cy.Config.Unit), 0, cy.Config.Duration)
			base = export.Suggest(export.NameInfo{Samples: samples, Subject: "Stability", Duration: total, Title: cy.Config.Name})
		}
	}
	if base == "" {
		return ""
	}
	if len(req.Items) > 1 {
		base = export.Sanitize(base + "_" + p.app.T("export.batch_suffix", "n", fmt.Sprint(len(req.Items))))
	}
	if zip || len(req.Items) > 1 {
		return export.WithExt(base, ".zip")
	}
	return export.WithExt(base, export.Formats[req.Formats[0]].Ext)
}
