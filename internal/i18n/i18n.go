// Package i18n holds MNE Lab's translations. The same catalogs are used by
// the interface and by everything the core renders itself (figures,
// exported tables, tray menu, notifications), so wording never diverges.
//
// Keys are neutral identifiers; translated text is never stored or used as
// an identifier.
package i18n

import (
	"embed"
	"encoding/json"
	"os"
	"strings"
	"sync"
)

//go:embed locales/*.json
var files embed.FS

// Supported languages, in the order offered in Settings.
var Supported = []string{"en", "pt-BR", "es"}

// Fallback is used for languages without a translation.
const Fallback = "en"

var (
	once     sync.Once
	catalogs map[string]map[string]string
)

func load() {
	catalogs = map[string]map[string]string{}
	for _, l := range Supported {
		b, err := files.ReadFile("locales/" + l + ".json")
		if err != nil {
			continue
		}
		m := map[string]string{}
		if json.Unmarshal(b, &m) == nil {
			catalogs[l] = m
		}
	}
}

// Match maps a system or browser language tag to a supported language
// ("pt-PT" and "pt" → pt-BR, "es-MX" → es, unknown → en).
func Match(tag string) string {
	tag = strings.ReplaceAll(strings.TrimSpace(tag), "_", "-")
	if i := strings.IndexAny(tag, ".@"); i >= 0 {
		tag = tag[:i]
	}
	low := strings.ToLower(tag)
	for _, l := range Supported {
		if strings.ToLower(l) == low {
			return l
		}
	}
	switch {
	case strings.HasPrefix(low, "pt"):
		return "pt-BR"
	case strings.HasPrefix(low, "es"):
		return "es"
	case strings.HasPrefix(low, "en"):
		return "en"
	}
	return Fallback
}

// Detect returns the operating system language.
func Detect() string {
	for _, v := range []string{"LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
		if s := os.Getenv(v); s != "" && s != "C" && s != "POSIX" {
			return Match(strings.Split(s, ":")[0])
		}
	}
	if s := systemLocale(); s != "" {
		return Match(s)
	}
	return Fallback
}

// T returns the text of key in lang ({name} placeholders replaced by kv
// pairs), falling back to English and then to the key itself.
func T(lang, key string, kv ...string) string {
	once.Do(load)
	s, ok := catalogs[lang][key]
	if !ok {
		s, ok = catalogs[Fallback][key]
	}
	if !ok {
		s = key
	}
	for i := 0; i+1 < len(kv); i += 2 {
		s = strings.ReplaceAll(s, "{"+kv[i]+"}", kv[i+1])
	}
	return s
}

// Translator binds a language.
func Translator(lang string) func(key string, kv ...string) string {
	return func(key string, kv ...string) string { return T(lang, key, kv...) }
}

// Keys lists every key of a language (tests check completeness).
func Keys(lang string) []string {
	once.Do(load)
	var out []string
	for k := range catalogs[lang] {
		out = append(out, k)
	}
	return out
}
