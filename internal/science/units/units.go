// Package units is the measurement-unit registry: the units the parsers
// recognize (system units), the dimensions they belong to, and the
// display-only conversion between units with an exact, known factor.
//
// Units are identified by their exact spelling. Spelling variants ("um",
// "µm", "μm") stay distinct, are never merged and never inherit one
// another's conversion. A conversion is a separate display value next to
// the original value and unit, which are never changed.
//
// "Measurement unit" here is a unit of measure (nm, kcps). It is unrelated
// to the experimental units of the statistics module.
package units

import (
	"errors"
	"math"
	"math/big"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/oaovito/mne_lab/internal/science/model"
)

// Dimension is the physical dimension of a unit.
type Dimension string

// Dimensions. Unclassified is a unit whose dimension is not established;
// it is never offered for a typed field and never converted.
const (
	Length            Dimension = "length"
	CountRate         Dimension = "count_rate"
	Time              Dimension = "time"
	Temperature       Dimension = "temperature"
	ElectricPotential Dimension = "electric_potential"
	Viscosity         Dimension = "viscosity"
	Dimensionless     Dimension = "dimensionless"
	Arbitrary         Dimension = "arbitrary"
	Unclassified      Dimension = "unclassified"
)

// Dimensions lists every dimension in display order.
var Dimensions = []Dimension{Length, CountRate, Time, Temperature, ElectricPotential, Viscosity, Dimensionless, Arbitrary, Unclassified}

// ValidDimension reports whether d is a known dimension.
func ValidDimension(d Dimension) bool {
	for _, x := range Dimensions {
		if x == d {
			return true
		}
	}
	return false
}

// fieldDimensions states the dimension of each LIGHTSCATTERING parameter a
// person can assign a unit to (the column mapping). Polydispersity and
// Baseline Index are dimensionless (SCIENTIFIC_RULES.md).
var fieldDimensions = map[string]Dimension{
	model.EffectiveDiameter: Length,
	model.CountRate:         CountRate,
	model.AverageCountRate:  CountRate,
	model.Polydispersity:    Dimensionless,
	model.BaselineIndex:     Dimensionless,
}

// FieldDimension returns the dimension of a parameter key.
func FieldDimension(key string) (Dimension, bool) {
	d, ok := fieldDimensions[key]
	return d, ok
}

// Unit is one entry of the registry.
type Unit struct {
	ID        string    `json:"id"`
	Symbol    string    `json:"symbol"`
	Name      string    `json:"name,omitempty"`
	Dimension Dimension `json:"dimension"`
	System    bool      `json:"system"`
}

// Errors (stable identifiers).
var (
	ErrSymbol          = errors.New("units.invalid_symbol")
	ErrName            = errors.New("units.invalid_name")
	ErrDimension       = errors.New("units.invalid_dimension")
	ErrNotStated       = errors.New("units.unit_not_stated")
	ErrNoExactFactor   = errors.New("units.no_exact_factor")
	ErrIncompatible    = errors.New("units.incompatible_dimensions")
	ErrNotFiniteNumber = errors.New("units.invalid_value")
)

// ValidSymbol applies the rules of a user-assigned mapping unit: non-empty,
// at most 64 bytes of valid UTF-8, no surrounding space and no control
// characters. The text is kept exactly as written.
func ValidSymbol(s string) bool {
	return s != "" && len(s) <= 64 && utf8.ValidString(s) && strings.TrimSpace(s) == s && strings.IndexFunc(s, unicode.IsControl) < 0
}

// ValidName checks the optional descriptive name of a custom unit.
func ValidName(s string) bool {
	return utf8.ValidString(s) && utf8.RuneCountInString(s) <= 120 && strings.TrimSpace(s) == s && strings.IndexFunc(s, unicode.IsControl) < 0
}

// DimensionOf returns the dimension of a symbol that the application knows
// without a person's declaration: a system unit or a conversion target.
func DimensionOf(symbol string) (Dimension, bool) {
	if u, ok := SystemBySymbol(symbol); ok {
		return u.Dimension, true
	}
	if f, ok := exact[symbol]; ok {
		return f.dim, true
	}
	return "", false
}

// Conversion is a value prepared for display in another unit. The
// original value, its source text and its unit are carried unchanged next
// to the display value; nothing here is ever written back to data.
type Conversion struct {
	OriginalValue float64 `json:"originalValue"`
	OriginalRaw   string  `json:"originalRaw,omitempty"`
	OriginalUnit  string  `json:"originalUnit"`
	DisplayValue  float64 `json:"displayValue"`
	DisplayUnit   string  `json:"displayUnit"`
	// Factor is the exact ratio applied (display = original × Factor),
	// written as an integer or a fraction such as "1/1000".
	Factor string `json:"factor"`
	// DisplayDecimals carries the decimals of the original text through a
	// power-of-ten conversion; -1 when they cannot be derived.
	DisplayDecimals int  `json:"displayDecimals"`
	Converted       bool `json:"converted"`
}

// Ratio returns the exact factor from one unit to another: a value in
// from multiplied by the ratio is the same quantity in to. Only exact
// spellings of the same dimension in the D6 table have one.
func Ratio(from, to string) (*big.Rat, error) {
	if from == "" || to == "" {
		return nil, ErrNotStated
	}
	if from == to {
		return big.NewRat(1, 1), nil
	}
	df, okf := DimensionOf(from)
	dt, okt := DimensionOf(to)
	if okf && okt && df != dt {
		return nil, ErrIncompatible
	}
	ff, okf := exact[from]
	ft, okt := exact[to]
	if !okf || !okt {
		return nil, ErrNoExactFactor
	}
	if ff.dim != ft.dim {
		return nil, ErrIncompatible
	}
	// from = ff.num/ff.den base; to = ft.num/ft.den base.
	r := new(big.Rat).SetFrac64(ff.num*ft.den, ff.den*ft.num)
	return r, nil
}

// Display converts a value for display only. It refuses a missing unit,
// incompatible dimensions and any pair without an exact factor.
func Display(value float64, raw, from, to string) (Conversion, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return Conversion{}, ErrNotFiniteNumber
	}
	r, err := Ratio(from, to)
	if err != nil {
		return Conversion{}, err
	}
	c := Conversion{OriginalValue: value, OriginalRaw: raw, OriginalUnit: from, DisplayUnit: to, Factor: r.RatString(), DisplayDecimals: -1, Converted: from != to}
	num, den := r.Num(), r.Denom()
	// The factors of one dimension divide one another, so the ratio is an
	// integer or the reciprocal of one: a single correctly rounded
	// operation, never a product of two rounded factors.
	switch {
	case den.IsInt64() && den.Int64() == 1:
		n, _ := new(big.Float).SetInt(num).Float64()
		c.DisplayValue = value * n
	case num.IsInt64() && num.Int64() == 1:
		d, _ := new(big.Float).SetInt(den).Float64()
		c.DisplayValue = value / d
	default:
		f, _ := r.Float64()
		c.DisplayValue = value * f
	}
	if raw != "" {
		if shift, ok := powerOfTen(r); ok {
			if d := (model.Quantity{Raw: raw}).Decimals() - shift; d >= 0 {
				c.DisplayDecimals = d
			} else {
				c.DisplayDecimals = 0
			}
		}
	}
	return c, nil
}

// DisplayQuantity converts a quantity for display. q is a copy: the
// stored quantity, its raw text and its unit are not touched.
func DisplayQuantity(q model.Quantity, to string) (Conversion, error) {
	return Display(q.Value, q.Raw, q.Unit, to)
}

// powerOfTen returns k when r = 10^k (k may be negative).
func powerOfTen(r *big.Rat) (int, bool) {
	ten := big.NewInt(10)
	pow := func(x *big.Int) (int, bool) {
		k := 0
		x = new(big.Int).Set(x)
		for x.Cmp(big.NewInt(1)) > 0 {
			q, m := new(big.Int).QuoRem(x, ten, new(big.Int))
			if m.Sign() != 0 {
				return 0, false
			}
			x, k = q, k+1
		}
		return k, true
	}
	n, ok1 := pow(r.Num())
	d, ok2 := pow(r.Denom())
	if !ok1 || !ok2 {
		return 0, false
	}
	return n - d, true
}
