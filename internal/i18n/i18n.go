package i18n

import (
	"fmt"
	"strings"
	"sync/atomic"

	"fyne.io/fyne/v2/lang"
)

type Language struct {
	Code string
	Name string
}

var Languages = []Language{
	{"en", "English"},
	{"ru", "Русский"},
	{"uk", "Українська"},
	{"de", "Deutsch"},
	{"es", "Español"},
	{"fr", "Français"},
}

var catalogs = map[string]map[string]string{
	"en": en,
	"ru": ru,
	"uk": uk,
	"de": de,
	"es": es,
	"fr": fr,
}

var current atomic.Value

func init() { current.Store("en") }

func Set(code string) {
	if _, ok := catalogs[code]; !ok {
		code = "en"
	}
	current.Store(code)
}

func Current() string { return current.Load().(string) }

// Detect picks the system language if it is supported.
func Detect() string {
	loc := strings.ToLower(lang.SystemLocale().LanguageString())
	if i := strings.IndexAny(loc, "-_"); i > 0 {
		loc = loc[:i]
	}
	if _, ok := catalogs[loc]; ok {
		return loc
	}
	return "en"
}

func NameOf(code string) string {
	for _, l := range Languages {
		if l.Code == code {
			return l.Name
		}
	}
	return code
}

func CodeOf(name string) string {
	for _, l := range Languages {
		if l.Name == name {
			return l.Code
		}
	}
	return "en"
}

func Names() []string {
	out := make([]string, len(Languages))
	for i, l := range Languages {
		out[i] = l.Name
	}
	return out
}

// pluralForm returns the CLDR plural category of n for lang: "one", "few" or "" (other/many).
func pluralForm(lang string, n int) string {
	if n < 0 {
		n = -n
	}
	switch lang {
	case "ru", "uk":
		switch {
		case n%10 == 1 && n%100 != 11:
			return "one"
		case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
			return "few"
		}
		return ""
	case "fr":
		if n <= 1 {
			return "one"
		}
		return ""
	}
	if n == 1 {
		return "one"
	}
	return ""
}

// N is T for a count: it picks the key's plural variant ("key|one", "key|few") for n and formats
// the string with n followed by args. The plain key holds the remaining form.
func N(key string, n int, args ...any) string {
	args = append([]any{n}, args...)
	if form := pluralForm(Current(), n); form != "" {
		if _, ok := catalogs[Current()][key+"|"+form]; ok {
			return T(key+"|"+form, args...)
		}
	}
	return T(key, args...)
}

// T returns the translated string for key, formatted with args when given.
func T(key string, args ...any) string {
	s, ok := catalogs[Current()][key]
	if !ok {
		if s, ok = en[key]; !ok {
			s = key
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}
