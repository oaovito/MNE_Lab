package export

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// NameInfo describes exported content for file name suggestions.
type NameInfo struct {
	Title    string    // user title, used when it is meaningful
	Samples  []string  // sample identifiers involved
	Module   string    // "DLS"
	Subject  string    // "Stability", a parameter ("PDI") or empty
	Date     time.Time // measurement date when there is a single one
	Point    string    // "Week4" for one cycle point
	Duration string    // "4Weeks" for a whole cycle
}

var paramTokens = map[string]string{
	"effective_diameter": "EffDiameter",
	"polydispersity":     "PDI",
	"count_rate":         "CountRate",
	"average_count_rate": "AverageCountRate",
	"baseline_index":     "BaselineIndex",
}

// ParamToken is the short name of a parameter in file names.
func ParamToken(key string) string { return paramTokens[key] }

// CycleTokens returns "Week4" and "4Weeks" style tokens.
func CycleTokens(unit string, offset, duration int) (point, total string) {
	singular := map[string]string{"hours": "Hour", "days": "Day", "weeks": "Week", "months": "Month", "years": "Year"}[unit]
	plural := map[string]string{"hours": "Hours", "days": "Days", "weeks": "Weeks", "months": "Months", "years": "Years"}[unit]
	if duration == 1 {
		plural = singular
	}
	return fmt.Sprintf("%s%d", singular, offset), fmt.Sprintf("%d%s", duration, plural)
}

// defaultTitles are generic titles that make poor file names.
var defaultTitles = map[string]bool{"graph": true, "untitled": true, "new graph": true, "gráfico": true, "sem título": true, "sin título": true, "export": true}

// Suggest builds a useful file name (without extension), such as
// A1_DLS_2026-10-05, A1_DLS_Cycle_Week4 or A1_Stability_4Weeks.
func Suggest(n NameInfo) string {
	var parts []string
	samples := uniqueSorted(n.Samples)
	switch {
	case len(samples) == 1:
		parts = append(parts, samples[0])
	case len(samples) == 2:
		parts = append(parts, samples[0]+"-"+samples[1])
	case len(samples) > 2:
		parts = append(parts, fmt.Sprintf("%s+%d", samples[0], len(samples)-1))
	}
	title := strings.TrimSpace(n.Title)
	if title != "" && !defaultTitles[strings.ToLower(title)] && !strings.EqualFold(title, strings.Join(parts, "")) {
		// A title that already names the sample ("F-A1 stability") stands alone.
		if len(parts) == 1 && len(title) > len(parts[0]) && strings.EqualFold(title[:len(parts[0])], parts[0]) {
			parts = nil
		}
		parts = append(parts, title)
		if n.Point != "" {
			parts = append(parts, n.Point)
		}
	} else {
		if n.Module != "" && n.Subject == "" {
			parts = append(parts, n.Module)
		}
		if n.Subject != "" {
			parts = append(parts, n.Subject)
		}
		switch {
		case n.Point != "":
			parts = append(parts, "Cycle", n.Point)
		case n.Duration != "":
			parts = append(parts, n.Duration)
		case !n.Date.IsZero():
			parts = append(parts, n.Date.Format("2006-01-02"))
		}
	}
	if len(parts) == 0 {
		parts = []string{"MNE-Lab", time.Now().Format("2006-01-02")}
	}
	return compact(strings.Join(parts, "_"))
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

var reserved = map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true}

// Sanitize makes a name valid on Windows, macOS and Linux file systems
// (including FAT32/exFAT USB drives). It changes only what those systems
// refuse, so a name the person typed keeps its spaces and accents.
func Sanitize(name string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"/\|?*`, r) {
			r = '-'
		}
		b.WriteRune(r)
	}
	// Windows refuses names that end with a dot or a space.
	s := strings.TrimRight(b.String(), " .")
	if r := []rune(s); len(r) > 120 {
		s = strings.TrimRight(string(r[:120]), " .")
	}
	if strings.Trim(s, " ._-") == "" {
		s = "MNE-Lab_export"
	}
	base := s
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	if reserved[strings.ToUpper(strings.TrimSpace(base))] {
		s = "_" + s
	}
	return s
}

// Adjusted reports whether Sanitize had to change a typed name.
func Adjusted(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && Sanitize(name) != name
}

// compact turns the parts of a suggested name into one word-like name
// (A1_DLS_2026-10-05).
func compact(name string) string {
	var b strings.Builder
	lastSep := false
	for _, r := range Sanitize(name) {
		if unicode.IsSpace(r) {
			r = '_'
		}
		sep := r == '_' || r == '-'
		if sep && lastSep {
			continue
		}
		lastSep = sep
		b.WriteRune(r)
	}
	if s := strings.Trim(b.String(), "._-"); s != "" {
		return s
	}
	return "MNE-Lab_export"
}

// WithExt applies the format's extension. An extension the person typed
// for another export format is replaced: content is never given a false
// extension.
func WithExt(name, ext string) string {
	name = Sanitize(name)
	cur := strings.ToLower(filepath.Ext(name))
	if cur == strings.ToLower(ext) || (ext == ".jpg" && cur == ".jpeg") || (ext == ".tiff" && cur == ".tif") {
		return name
	}
	for _, f := range Formats {
		if f.Ext != "" && cur == f.Ext {
			name = strings.TrimSuffix(name, filepath.Ext(name))
			break
		}
	}
	if cur == ".jpeg" || cur == ".tif" {
		name = strings.TrimSuffix(name, filepath.Ext(name))
	}
	return name + ext
}

// KeepBothName returns "name (2).ext", "name (3).ext"… for the first n
// where exists reports false.
func KeepBothName(name string, exists func(string) bool) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; i < 10000; i++ {
		c := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if !exists(c) {
			return c
		}
	}
	return fmt.Sprintf("%s (%d)%s", base, time.Now().UnixNano(), ext)
}
