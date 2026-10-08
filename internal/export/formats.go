// Package export is the Smart Export engine: it recognizes what is being
// exported, recommends suitable formats without hiding the other valid
// ones, writes real files in each format (never renamed content), names
// them usefully, never overwrites silently, and keeps provenance.
//
// Everything is generated locally and works offline. Nothing is sent
// anywhere: the destination is always a folder the person chose.
package export

// Content kinds the export dialog adapts to.
const (
	KindGraph   = "graph"   // a scientific figure and its underlying data
	KindDataset = "dataset" // normalized measurements as tables
	KindCycle   = "cycle"   // a cycle: results, data or the full package
	KindFile    = "file"    // an imported original file
	KindBatch   = "batch"   // several items at once
)

// Format groups.
const (
	GroupFigure   = "figure"
	GroupData     = "data"
	GroupPackage  = "package"
	GroupOriginal = "original"
)

// Format describes one implemented output format.
type Format struct {
	ID     string `json:"id"`
	Ext    string `json:"ext"`
	MIME   string `json:"mime"`
	Group  string `json:"group"`
	Vector bool   `json:"vector,omitempty"`
	Lossy  bool   `json:"lossy,omitempty"`
	DPI    bool   `json:"dpi,omitempty"`   // resolution is stored in the file
	Alpha  bool   `json:"alpha,omitempty"` // supports a transparent background
	// Hint and Warning are interface keys explaining the recommendation
	// or the caveat (a caveat informs, it never blocks).
	Hint    string `json:"hint,omitempty"`
	Warning string `json:"warning,omitempty"`
}

// Formats lists every implemented format. EPS and Parquet are not offered
// until they can be generated and validated reliably.
var Formats = map[string]Format{
	"png":  {ID: "png", Ext: ".png", MIME: "image/png", Group: GroupFigure, DPI: true, Alpha: true, Hint: "export.hint.png"},
	"svg":  {ID: "svg", Ext: ".svg", MIME: "image/svg+xml", Group: GroupFigure, Vector: true, Alpha: true, Hint: "export.hint.svg"},
	"pdf":  {ID: "pdf", Ext: ".pdf", MIME: "application/pdf", Group: GroupFigure, Vector: true, Alpha: true, Hint: "export.hint.pdf"},
	"tiff": {ID: "tiff", Ext: ".tiff", MIME: "image/tiff", Group: GroupFigure, DPI: true, Alpha: true, Hint: "export.hint.tiff"},
	"jpeg": {ID: "jpeg", Ext: ".jpg", MIME: "image/jpeg", Group: GroupFigure, DPI: true, Lossy: true, Warning: "export.warn.jpeg"},
	"webp": {ID: "webp", Ext: ".webp", MIME: "image/webp", Group: GroupFigure, Alpha: true, Hint: "export.hint.webp", Warning: "export.warn.webp_dpi"},

	"csv":  {ID: "csv", Ext: ".csv", MIME: "text/csv", Group: GroupData, Hint: "export.hint.csv"},
	"xlsx": {ID: "xlsx", Ext: ".xlsx", MIME: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Group: GroupData, Hint: "export.hint.xlsx"},
	"tsv":  {ID: "tsv", Ext: ".tsv", MIME: "text/tab-separated-values", Group: GroupData, Hint: "export.hint.tsv"},
	"txt":  {ID: "txt", Ext: ".txt", MIME: "text/plain", Group: GroupData, Hint: "export.hint.txt"},
	"json": {ID: "json", Ext: ".json", MIME: "application/json", Group: GroupData, Hint: "export.hint.json"},

	"package":  {ID: "package", Ext: ".zip", MIME: "application/zip", Group: GroupPackage, Hint: "export.hint.package"},
	"zip":      {ID: "zip", Ext: ".zip", MIME: "application/zip", Group: GroupPackage, Hint: "export.hint.zip"},
	"original": {ID: "original", Group: GroupOriginal, Hint: "export.hint.original"},
}

// Section is one block of the export dialog.
type Section struct {
	ID      string   `json:"id"` // recommended, figure, data, results, full, original, all
	Formats []string `json:"formats"`
}

// Options is what the dialog offers for a kind of content: recommended
// formats first, then every compatible format (nothing valid is hidden).
type Options struct {
	Kind     string    `json:"kind"`
	Sections []Section `json:"sections"`
	Presets  []string  `json:"presets"`
	Default  Choice    `json:"default"`
}

// Choice is a preselected configuration.
type Choice struct {
	Preset  string   `json:"preset"`
	Formats []string `json:"formats"`
}

// Export presets.
const (
	PresetQuick        = "quick"
	PresetPresentation = "presentation"
	PresetPublication  = "publication"
	PresetRawData      = "raw"
	PresetPackage      = "package"
	PresetCustom       = "custom"
)

var (
	figureAll = []string{"png", "svg", "pdf", "tiff", "jpeg", "webp"}
	dataAll   = []string{"xlsx", "csv", "tsv", "txt", "json"}
)

// For returns the dialog structure for a kind of content.
func For(kind string) Options {
	o := Options{Kind: kind}
	switch kind {
	case KindGraph:
		o.Sections = []Section{
			{"recommended", []string{"png", "svg", "pdf", "tiff"}},
			{"data", []string{"csv", "xlsx", "tsv", "txt"}},
			{"all", append(append([]string{}, figureAll...), dataAll...)},
		}
		o.Presets = []string{PresetQuick, PresetPresentation, PresetPublication, PresetRawData, PresetCustom}
		o.Default = Choice{PresetQuick, []string{"png"}}
	case KindDataset:
		o.Sections = []Section{
			{"recommended", []string{"xlsx", "csv", "tsv"}},
			{"all", append([]string{}, dataAll...)},
			{"original", []string{"original"}},
		}
		o.Presets = []string{PresetQuick, PresetRawData, PresetCustom}
		o.Default = Choice{PresetRawData, []string{"xlsx", "csv"}}
	case KindCycle:
		o.Sections = []Section{
			{"full", []string{"package"}},
			{"results", []string{"pdf", "png", "svg", "tiff"}},
			{"data", []string{"xlsx", "csv", "json"}},
			{"all", append(append([]string{"package"}, figureAll...), dataAll...)},
		}
		o.Presets = []string{PresetPackage, PresetPublication, PresetPresentation, PresetRawData, PresetCustom}
		o.Default = Choice{PresetPackage, []string{"package"}}
	case KindFile:
		o.Sections = []Section{{"original", []string{"original"}}, {"all", append([]string{"original"}, dataAll...)}}
		o.Presets = []string{PresetQuick, PresetCustom}
		o.Default = Choice{PresetQuick, []string{"original"}}
	default:
		o.Kind = KindBatch
		o.Sections = []Section{
			{"recommended", []string{"png", "svg", "csv"}},
			{"all", append(append(append([]string{}, figureAll...), dataAll...), "original")},
		}
		o.Presets = []string{PresetQuick, PresetPresentation, PresetPublication, PresetRawData, PresetPackage, PresetCustom}
		o.Default = Choice{PresetQuick, []string{"png"}}
	}
	return o
}

// SmartDefault preselects formats for a kind and preset.
func SmartDefault(kind, preset string) Choice {
	c := Choice{Preset: preset}
	switch preset {
	case PresetPublication:
		c.Formats = []string{"svg", "tiff"}
	case PresetPresentation:
		c.Formats = []string{"png"}
	case PresetRawData:
		c.Formats = []string{"xlsx", "csv"}
	case PresetPackage:
		c.Formats = []string{"package"}
	default:
		return For(kind).Default
	}
	if kind == KindDataset && (preset == PresetPublication || preset == PresetPresentation) {
		return For(kind).Default
	}
	return c
}

// Compatible reports whether a format may be used for a kind.
func Compatible(kind, format string) bool {
	for _, s := range For(kind).Sections {
		for _, f := range s.Formats {
			if f == format {
				return true
			}
		}
	}
	return false
}
