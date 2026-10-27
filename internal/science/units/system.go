package units

// System units are exactly the unit spellings the LIGHTSCATTERING parsers
// recognize (lightscattering/parser.go: unitPattern and the value-suffix
// list of splitValueUnit, and the d(nm) column of nanobrook.go), with the
// spellings the parser documentation names as distinct ("C" and "°C",
// "mcps" and "Mcps", "um" and "µm"). Spelling variants are separate entries:
// none is an alias of another and none inherits another's conversion factor.
//
// The parser matches its pattern without regard to case and keeps the text
// as the file writes it, so a file may still state a spelling that is not
// listed here (for example "NM"); such a unit is shown as written and is
// not a registered unit.
//
// A dimension is stated only where the symbol names one unambiguously.
// "deg" and "°" can be an angle or a temperature in instrument reports, so
// they stay unclassified and are never offered for a typed field.
var system = []Unit{
	{ID: "sys.nm", Symbol: "nm", Dimension: Length},
	{ID: "sys.micro-sign-m", Symbol: "µm", Dimension: Length}, // U+00B5 MICRO SIGN
	{ID: "sys.greek-mu-m", Symbol: "μm", Dimension: Length},   // U+03BC GREEK SMALL LETTER MU
	{ID: "sys.um", Symbol: "um", Dimension: Length},
	{ID: "sys.mm", Symbol: "mm", Dimension: Length},
	{ID: "sys.cps", Symbol: "cps", Dimension: CountRate},
	{ID: "sys.kcps", Symbol: "kcps", Dimension: CountRate},
	{ID: "sys.kcps-upper", Symbol: "Kcps", Dimension: CountRate},
	{ID: "sys.mcps-upper", Symbol: "Mcps", Dimension: CountRate},
	{ID: "sys.mcps-lower", Symbol: "mcps", Dimension: CountRate},
	{ID: "sys.s", Symbol: "s", Dimension: Time},
	{ID: "sys.ms", Symbol: "ms", Dimension: Time},
	{ID: "sys.degree-c", Symbol: "°C", Dimension: Temperature},
	{ID: "sys.c", Symbol: "C", Dimension: Temperature},
	{ID: "sys.k", Symbol: "K", Dimension: Temperature},
	{ID: "sys.mv", Symbol: "mV", Dimension: ElectricPotential},
	{ID: "sys.cp", Symbol: "cP", Dimension: Viscosity},
	{ID: "sys.percent", Symbol: "%", Dimension: Dimensionless},
	{ID: "sys.a-u", Symbol: "a.u.", Dimension: Arbitrary},
	{ID: "sys.au", Symbol: "au", Dimension: Arbitrary},
	{ID: "sys.deg", Symbol: "deg", Dimension: Unclassified},
	{ID: "sys.degree", Symbol: "°", Dimension: Unclassified},
}

// System returns the system units in display order (a copy).
func System() []Unit {
	out := make([]Unit, len(system))
	copy(out, system)
	for i := range out {
		out[i].System = true
	}
	return out
}

// SystemByID finds a system unit by its stable identifier.
func SystemByID(id string) (Unit, bool) {
	for _, u := range system {
		if u.ID == id {
			u.System = true
			return u, true
		}
	}
	return Unit{}, false
}

// SystemBySymbol finds a system unit by its exact spelling.
func SystemBySymbol(symbol string) (Unit, bool) {
	for _, u := range system {
		if u.Symbol == symbol {
			u.System = true
			return u, true
		}
	}
	return Unit{}, false
}

// exact holds the only conversion factors the application knows (decision
// D6): each symbol is an exact integer multiple or fraction of its
// dimension's base unit. Only these exact spellings convert; "μm" (Greek
// mu), "um", "Kcps" and "mcps" do not, because a spelling variant is never
// assumed to be the same unit. "m", "min", "h" and "d" are not parser
// spellings, so they are conversion targets but not system units.
var exact = map[string]factor{
	"nm":   {Length, 1, 1_000_000_000},
	"µm":   {Length, 1, 1_000_000},
	"m":    {Length, 1, 1},
	"cps":  {CountRate, 1, 1},
	"kcps": {CountRate, 1_000, 1},
	"Mcps": {CountRate, 1_000_000, 1},
	"s":    {Time, 1, 1},
	"min":  {Time, 60, 1},
	"h":    {Time, 3_600, 1},
	"d":    {Time, 86_400, 1},
}

// factor states one unit as num/den of its dimension's base unit.
type factor struct {
	dim      Dimension
	num, den int64
}
