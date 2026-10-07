package ui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

const (
	prefColorTheme   = "color_theme"
	prefCustomThemes = "custom_themes"
)

// palette is one variant (dark or light) of a color theme.
type palette struct {
	Background string `json:"background"`
	Surface    string `json:"surface"`
	Text       string `json:"text"`
}

// colorTheme is a named accent with dark and light palettes; everything else is derived.
type colorTheme struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Accent  string  `json:"accent"`
	Dark    palette `json:"dark"`
	Light   palette `json:"light"`
	builtin bool
}

var builtinThemes = []colorTheme{
	{ID: "indigo", Name: "Indigo", Accent: "#6366F1",
		Dark: palette{"#16161D", "#20202B", "#E8E8F0"}, Light: palette{"#F6F6FA", "#FFFFFF", "#1B1B26"}},
	{ID: "ocean", Name: "Ocean", Accent: "#0EA5E9",
		Dark: palette{"#0B1620", "#132332", "#E2EEF7"}, Light: palette{"#F2F8FC", "#FFFFFF", "#0F2233"}},
	{ID: "emerald", Name: "Emerald", Accent: "#10B981",
		Dark: palette{"#0E1613", "#16221D", "#E3F2EA"}, Light: palette{"#F3FAF6", "#FFFFFF", "#11251C"}},
	{ID: "sunset", Name: "Sunset", Accent: "#F97316",
		Dark: palette{"#19120F", "#261B16", "#F6E9E1"}, Light: palette{"#FFF8F2", "#FFFFFF", "#2A1A12"}},
	{ID: "rose", Name: "Rose", Accent: "#F43F5E",
		Dark: palette{"#191014", "#26181E", "#F6E4EA"}, Light: palette{"#FFF5F7", "#FFFFFF", "#2A1219"}},
	{ID: "graphite", Name: "Graphite", Accent: "#94A3B8",
		Dark: palette{"#111214", "#1B1D20", "#E6E8EB"}, Light: palette{"#F4F5F6", "#FFFFFF", "#1A1C1E"}},
	{ID: "nord", Name: "Nord", Accent: "#88C0D0",
		Dark: palette{"#2E3440", "#3B4252", "#ECEFF4"}, Light: palette{"#ECEFF4", "#FFFFFF", "#2E3440"}},
	{ID: "dracula", Name: "Dracula", Accent: "#BD93F9",
		Dark: palette{"#282A36", "#343746", "#F8F8F2"}, Light: palette{"#F8F8F2", "#FFFFFF", "#282A36"}},
	{ID: "solarized", Name: "Solarized", Accent: "#268BD2",
		Dark: palette{"#002B36", "#073642", "#EEE8D5"}, Light: palette{"#FDF6E3", "#FFFFFF", "#073642"}},
}

func init() {
	for i := range builtinThemes {
		builtinThemes[i].builtin = true
	}
}

func parseHex(s string) (color.NRGBA, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return color.NRGBA{}, fmt.Errorf("invalid color %q", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("invalid color %q", s)
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 0xff}, nil
}

func mustHex(s string) color.NRGBA {
	c, err := parseHex(s)
	if err != nil {
		return color.NRGBA{0x80, 0x80, 0x80, 0xff}
	}
	return c
}

func toHex(c color.Color) string {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return fmt.Sprintf("#%02X%02X%02X", n.R, n.G, n.B)
}

func (t colorTheme) validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return errors.New("theme has no name")
	}
	for _, c := range []string{t.Accent, t.Dark.Background, t.Dark.Surface, t.Dark.Text, t.Light.Background, t.Light.Surface, t.Light.Text} {
		if _, err := parseHex(c); err != nil {
			return err
		}
	}
	return nil
}

func newThemeID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "custom-" + hex.EncodeToString(b)
}

// themeStore keeps the custom themes in the application preferences.
type themeStore struct{ prefs fyne.Preferences }

func (s themeStore) custom() []colorTheme {
	var list []colorTheme
	if raw := s.prefs.String(prefCustomThemes); raw != "" {
		_ = json.Unmarshal([]byte(raw), &list)
	}
	return slices.DeleteFunc(list, func(t colorTheme) bool { return t.validate() != nil })
}

func (s themeStore) saveCustom(list []colorTheme) {
	b, _ := json.Marshal(list)
	s.prefs.SetString(prefCustomThemes, string(b))
}

func (s themeStore) all() []colorTheme {
	return append(slices.Clone(builtinThemes), s.custom()...)
}

func (s themeStore) find(id string) (colorTheme, bool) {
	for _, t := range s.all() {
		if t.ID == id {
			return t, true
		}
	}
	return builtinThemes[0], false
}

func (s themeStore) current() colorTheme {
	t, _ := s.find(s.prefs.StringWithFallback(prefColorTheme, builtinThemes[0].ID))
	return t
}

// upsert stores a custom theme, replacing one with the same ID.
func (s themeStore) upsert(t colorTheme) {
	list := s.custom()
	if i := slices.IndexFunc(list, func(x colorTheme) bool { return x.ID == t.ID }); i >= 0 {
		list[i] = t
	} else {
		list = append(list, t)
	}
	s.saveCustom(list)
}

func (s themeStore) remove(id string) {
	s.saveCustom(slices.DeleteFunc(s.custom(), func(t colorTheme) bool { return t.ID == id }))
}

// appTheme renders a colorTheme for Fyne. mode forces light or dark; empty follows the system.
type appTheme struct {
	fyne.Theme
	ct     colorTheme
	forced bool
	mode   fyne.ThemeVariant
	dark   paletteColors
	light  paletteColors
	accent color.NRGBA
}

type paletteColors struct{ bg, surface, text color.NRGBA }

func newAppTheme(ct colorTheme, mode string) *appTheme {
	t := &appTheme{Theme: theme.DefaultTheme(), ct: ct, accent: mustHex(ct.Accent)}
	t.dark = paletteColors{mustHex(ct.Dark.Background), mustHex(ct.Dark.Surface), mustHex(ct.Dark.Text)}
	t.light = paletteColors{mustHex(ct.Light.Background), mustHex(ct.Light.Surface), mustHex(ct.Light.Text)}
	switch mode {
	case "light":
		t.forced, t.mode = true, theme.VariantLight
	case "dark":
		t.forced, t.mode = true, theme.VariantDark
	}
	return t
}

func alpha(c color.NRGBA, a uint8) color.NRGBA { c.A = a; return c }

// mix blends a towards b by f (0..1).
func mix(a, b color.NRGBA, f float64) color.NRGBA {
	l := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f) }
	return color.NRGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 0xff}
}

func luminance(c color.NRGBA) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

func (t *appTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if t.forced {
		v = t.mode
	}
	p := t.light
	if v == theme.VariantDark {
		p = t.dark
	}
	switch n {
	case theme.ColorNameBackground:
		return p.bg
	case theme.ColorNameForeground:
		return p.text
	case theme.ColorNamePrimary, theme.ColorNameHyperlink:
		return t.accent
	case theme.ColorNameForegroundOnPrimary:
		if luminance(t.accent) > 0.6 {
			return color.NRGBA{0x10, 0x10, 0x14, 0xff}
		}
		return color.White
	case theme.ColorNameFocus:
		return alpha(t.accent, 0x99)
	case theme.ColorNameSelection:
		return alpha(t.accent, 0x44)
	case theme.ColorNameButton, theme.ColorNameInputBackground:
		return mix(p.surface, p.text, 0.04)
	case theme.ColorNameDisabledButton:
		return mix(p.surface, p.bg, 0.5)
	case theme.ColorNameInputBorder:
		return alpha(p.text, 0x30)
	case theme.ColorNameHeaderBackground:
		return mix(p.bg, p.text, 0.035)
	case theme.ColorNameOverlayBackground, theme.ColorNameMenuBackground:
		return p.surface
	case theme.ColorNameHover:
		return alpha(p.text, 0x16)
	case theme.ColorNamePressed:
		return alpha(p.text, 0x28)
	case theme.ColorNameSeparator:
		return alpha(p.text, 0x1e)
	case theme.ColorNamePlaceHolder:
		return mix(p.text, p.bg, 0.45)
	case theme.ColorNameDisabled:
		return mix(p.text, p.bg, 0.6)
	case theme.ColorNameScrollBar:
		return alpha(p.text, 0x55)
	case theme.ColorNameShadow:
		if v == theme.VariantDark {
			return color.NRGBA{0, 0, 0, 0x88}
		}
		return color.NRGBA{0, 0, 0, 0x30}
	}
	return t.Theme.Color(n, v)
}

// preview returns the palette shown for the current mode, for theme cards.
func (t colorTheme) preview(dark bool) paletteColors {
	if dark {
		return paletteColors{mustHex(t.Dark.Background), mustHex(t.Dark.Surface), mustHex(t.Dark.Text)}
	}
	return paletteColors{mustHex(t.Light.Background), mustHex(t.Light.Surface), mustHex(t.Light.Text)}
}

func applyTheme(fa fyne.App, mode string) {
	ct := themeStore{fa.Preferences()}.current()
	fa.Settings().SetTheme(newAppTheme(ct, mode))
}

func isDarkNow(fa fyne.App) bool {
	switch fa.Preferences().StringWithFallback(prefTheme, "system") {
	case "dark":
		return true
	case "light":
		return false
	}
	return fa.Settings().ThemeVariant() == theme.VariantDark
}
