package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var verbs = regexp.MustCompile(`%[a-z]`)

func TestCatalogsComplete(t *testing.T) {
	for code, cat := range catalogs {
		for key, src := range en {
			if strings.Contains(key, "|") {
				continue
			}
			tr, ok := cat[key]
			if !ok {
				t.Errorf("%s: missing %q", code, key)
				continue
			}
			if !slices.Equal(verbs.FindAllString(src, -1), verbs.FindAllString(tr, -1)) {
				t.Errorf("%s: %q format verbs differ", code, key)
			}
		}
		for key, tr := range cat {
			base, _, plural := strings.Cut(key, "|")
			src, ok := en[base]
			if !ok {
				t.Errorf("%s: unknown key %q", code, key)
				continue
			}
			if plural && !slices.Equal(verbs.FindAllString(src, -1), verbs.FindAllString(tr, -1)) {
				t.Errorf("%s: %q format verbs differ", code, key)
			}
		}
	}
}

func TestUIKeysExist(t *testing.T) {
	re := regexp.MustCompile(`\b[TN]\("([a-z0-9_.]*[a-z0-9_])"`)
	files, _ := filepath.Glob("../ui/*.go")
	for _, f := range files {
		b, _ := os.ReadFile(f)
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			if _, ok := en[m[1]]; !ok {
				t.Errorf("%s uses unknown key %q", filepath.Base(f), m[1])
			}
		}
	}
}

func TestPlural(t *testing.T) {
	defer Set("en")
	cases := []struct {
		lang string
		n    int
		want string
	}{
		{"en", 1, "1 account"}, {"en", 0, "0 accounts"}, {"en", 2, "2 accounts"},
		{"fr", 0, "0 compte"}, {"fr", 2, "2 comptes"},
		{"ru", 1, "1 аккаунт"}, {"ru", 3, "3 аккаунта"}, {"ru", 5, "5 аккаунтов"},
		{"ru", 11, "11 аккаунтов"}, {"ru", 21, "21 аккаунт"}, {"ru", 112, "112 аккаунтов"}, {"ru", 24, "24 аккаунта"},
		{"uk", 22, "22 акаунти"}, {"uk", 15, "15 акаунтів"},
	}
	for _, c := range cases {
		Set(c.lang)
		if got := N("count.total", c.n); got != c.want {
			t.Errorf("%s %d: got %q, want %q", c.lang, c.n, got, c.want)
		}
	}
}
