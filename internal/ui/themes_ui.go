package ui

import (
	"encoding/json"
	"errors"
	"image/color"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
)

func (a *App) themes() themeStore { return themeStore{a.fa.Preferences()} }

// themeGallery shows every color theme as a clickable preview card.
func (a *App) themeGallery() fyne.CanvasObject {
	T := i18n.T
	store := a.themes()
	current := store.current().ID
	dark := isDarkNow(a.fa)
	grid := container.NewGridWrap(fyne.NewSize(132, 104))
	for _, t := range store.all() {
		grid.Add(newThemeCard(t, t.ID == current, dark, func() { a.selectTheme(t.ID) }, func(pos fyne.Position) {
			widget.ShowPopUpMenuAtPosition(a.themeMenu(t), a.win.Canvas(), pos)
		}))
	}
	newBtn := newIconButton(T("theme.new"), theme.ContentAddIcon(), func() {
		c := store.current()
		c.ID, c.Name, c.builtin = newThemeID(), c.Name+" "+T("theme.copy"), false
		a.editTheme(c, true)
	})
	importBtn := newIconButton(T("theme.import"), theme.DownloadIcon(), a.importTheme)
	return container.NewVBox(grid, container.NewHBox(newBtn, importBtn), labelWrap(T("theme.hint")))
}

func (a *App) selectTheme(id string) {
	a.fa.Preferences().SetString(prefColorTheme, id)
	applyTheme(a.fa, a.fa.Preferences().StringWithFallback(prefTheme, "system"))
	a.rebuild()
}

func (a *App) themeMenu(t colorTheme) *fyne.Menu {
	T := i18n.T
	items := []*fyne.MenuItem{
		fyne.NewMenuItemWithIcon(T("theme.apply"), theme.ConfirmIcon(), func() { a.selectTheme(t.ID) }),
	}
	if !t.builtin {
		items = append(items, fyne.NewMenuItemWithIcon(T("menu.edit"), theme.DocumentCreateIcon(), func() { a.editTheme(t, false) }))
	}
	items = append(items,
		fyne.NewMenuItemWithIcon(T("theme.duplicate"), theme.ContentCopyIcon(), func() {
			c := t
			c.ID, c.Name, c.builtin = newThemeID(), t.Name+" "+T("theme.copy"), false
			a.editTheme(c, true)
		}),
		fyne.NewMenuItemWithIcon(T("theme.export"), theme.UploadIcon(), func() { a.exportTheme(t) }),
	)
	if !t.builtin {
		items = append(items, fyne.NewMenuItemSeparator(), fyne.NewMenuItemWithIcon(T("menu.delete"), theme.DeleteIcon(), func() {
			dialog.ShowConfirm(T("theme.delete"), T("theme.delete.confirm", t.Name), func(ok bool) {
				if !ok {
					return
				}
				store := a.themes()
				store.remove(t.ID)
				if store.prefs.String(prefColorTheme) == t.ID {
					a.selectTheme(builtinThemes[0].ID)
					return
				}
				a.shell.refreshPage()
			}, a.win)
		}))
	}
	return fyne.NewMenu("", items...)
}

// editTheme edits a custom theme with live preview; cancel restores the active theme.
func (a *App) editTheme(t colorTheme, isNew bool) {
	T := i18n.T
	mode := a.fa.Preferences().StringWithFallback(prefTheme, "system")
	draft := t
	preview := func() { a.fa.Settings().SetTheme(newAppTheme(draft, mode)) }
	name := widget.NewEntry()
	name.SetText(draft.Name)
	name.OnChanged = func(s string) { draft.Name = s }

	field := func(label string, get func() string, set func(string)) *widget.FormItem {
		swatch := canvas.NewRectangle(mustHex(get()))
		swatch.CornerRadius = 6
		swatch.StrokeWidth = 1
		swatch.StrokeColor = theme.Color(theme.ColorNameInputBorder)
		swatch.SetMinSize(fyne.NewSize(36, 28))
		hexEntry := widget.NewEntry()
		hexEntry.SetText(get())
		apply := func(v string) {
			c, err := parseHex(v)
			if err != nil {
				return
			}
			set(toHex(c))
			swatch.FillColor = c
			swatch.Refresh()
			preview()
		}
		hexEntry.OnChanged = apply
		pick := newIconButton("", theme.ColorPaletteIcon(), func() {
			p := dialog.NewColorPicker(label, "", func(c color.Color) {
				hexEntry.SetText(toHex(c))
			}, a.win)
			p.Advanced = true
			p.SetColor(mustHex(get()))
			p.Show()
		})
		return widget.NewFormItem(label, container.NewBorder(nil, nil, container.NewCenter(swatch), pick, hexEntry))
	}
	form := widget.NewForm(
		widget.NewFormItem(T("theme.name"), name),
		field(T("theme.accent"), func() string { return draft.Accent }, func(v string) { draft.Accent = v }),
	)
	darkForm := widget.NewForm(
		field(T("theme.background"), func() string { return draft.Dark.Background }, func(v string) { draft.Dark.Background = v }),
		field(T("theme.surface"), func() string { return draft.Dark.Surface }, func(v string) { draft.Dark.Surface = v }),
		field(T("theme.text"), func() string { return draft.Dark.Text }, func(v string) { draft.Dark.Text = v }),
	)
	lightForm := widget.NewForm(
		field(T("theme.background"), func() string { return draft.Light.Background }, func(v string) { draft.Light.Background = v }),
		field(T("theme.surface"), func() string { return draft.Light.Surface }, func(v string) { draft.Light.Surface = v }),
		field(T("theme.text"), func() string { return draft.Light.Text }, func(v string) { draft.Light.Text = v }),
	)
	content := container.NewVScroll(container.NewVBox(form,
		sectionTitle(T("theme.mode.dark")), darkForm,
		sectionTitle(T("theme.mode.light")), lightForm,
		labelWrap(T("theme.preview.hint"))))
	content.SetMinSize(fyne.NewSize(460, 420))
	title := T("theme.edit")
	if isNew {
		title = T("theme.new")
	}
	d := dialog.NewCustomConfirm(title, T("save"), T("cancel"), content, func(ok bool) {
		if !ok {
			applyTheme(a.fa, mode)
			return
		}
		draft.Name = strings.TrimSpace(draft.Name)
		if err := draft.validate(); err != nil {
			applyTheme(a.fa, mode)
			a.showError(err)
			return
		}
		a.themes().upsert(draft)
		a.selectTheme(draft.ID)
	}, a.win)
	d.Resize(fyne.NewSize(520, 600))
	d.Show()
	preview()
}

func (a *App) exportTheme(t colorTheme) {
	T := i18n.T
	b, _ := json.MarshalIndent(t, "", "  ")
	name := strings.ToLower(strings.ReplaceAll(t.Name, " ", "-")) + ".json"
	a.saveFile(T("theme.export"), name, &fileFilter{name: T("filter.themes"), exts: []string{".json"}}, func(path string) {
		if err := os.WriteFile(path, b, 0o644); err != nil {
			a.showError(err)
			return
		}
		a.notify(T("theme.exported", t.Name), path)
	})
}

func (a *App) importTheme() {
	T := i18n.T
	a.openFile(T("theme.import"), &fileFilter{name: T("filter.themes"), exts: []string{".json"}}, func(path string) {
		b, err := readLimited(path, 64<<10)
		if err != nil {
			a.showError(err)
			return
		}
		var t colorTheme
		if err := json.Unmarshal(b, &t); err != nil {
			a.showError(errors.New(T("theme.invalid")))
			return
		}
		if err := t.validate(); err != nil {
			a.showError(err)
			return
		}
		t.ID = newThemeID()
		a.themes().upsert(t)
		a.selectTheme(t.ID)
		a.notify(T("theme.imported", t.Name), "")
	})
}

// themeCard is a clickable preview of a color theme.
type themeCard struct {
	widget.BaseWidget
	t        colorTheme
	selected bool
	dark     bool
	onTap    func()
	onMenu   func(fyne.Position)
	hovered  bool
	border   *canvas.Rectangle
}

func newThemeCard(t colorTheme, selected, dark bool, onTap func(), onMenu func(fyne.Position)) *themeCard {
	c := &themeCard{t: t, selected: selected, dark: dark, onTap: onTap, onMenu: onMenu}
	c.ExtendBaseWidget(c)
	return c
}

func (c *themeCard) CreateRenderer() fyne.WidgetRenderer {
	p := c.t.preview(c.dark)
	accent := mustHex(c.t.Accent)
	bg := canvas.NewRectangle(p.bg)
	bg.CornerRadius = 10
	surface := canvas.NewRectangle(p.surface)
	surface.CornerRadius = 6
	bar := canvas.NewRectangle(accent)
	bar.CornerRadius = 3
	line1 := canvas.NewRectangle(alpha(p.text, 0xcc))
	line1.CornerRadius = 2
	line2 := canvas.NewRectangle(alpha(p.text, 0x55))
	line2.CornerRadius = 2
	dot := canvas.NewCircle(accent)
	name := canvas.NewText(c.t.Name, theme.Color(theme.ColorNameForeground))
	name.TextSize = 12
	name.Alignment = fyne.TextAlignCenter
	if c.selected {
		name.TextStyle.Bold = true
	}
	c.border = canvas.NewRectangle(color.Transparent)
	c.border.CornerRadius = 12
	c.border.StrokeWidth = 2
	objs := []fyne.CanvasObject{c.border, bg, surface, bar, line1, line2, dot, name}
	r := &themeCardRenderer{c: c, objs: objs, bg: bg, surface: surface, bar: bar, l1: line1, l2: line2, dot: dot, name: name}
	r.Refresh()
	return r
}

func (c *themeCard) Tapped(*fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap()
	}
}

func (c *themeCard) TappedSecondary(ev *fyne.PointEvent) {
	if c.onMenu != nil {
		c.onMenu(ev.AbsolutePosition)
	}
}

func (c *themeCard) MouseIn(*desktop.MouseEvent)    { c.hovered = true; c.Refresh() }
func (c *themeCard) MouseMoved(*desktop.MouseEvent) {}
func (c *themeCard) MouseOut()                      { c.hovered = false; c.Refresh() }
func (c *themeCard) Cursor() desktop.Cursor         { return desktop.PointerCursor }

type themeCardRenderer struct {
	c                        *themeCard
	objs                     []fyne.CanvasObject
	bg, surface, bar, l1, l2 *canvas.Rectangle
	dot                      *canvas.Circle
	name                     *canvas.Text
}

func (r *themeCardRenderer) Layout(s fyne.Size) {
	pad := float32(4)
	nameH := r.name.MinSize().Height
	ph := s.Height - nameH - pad*2
	r.c.border.Move(fyne.NewPos(0, 0))
	r.c.border.Resize(fyne.NewSize(s.Width, ph+pad*2))
	r.bg.Move(fyne.NewPos(pad, pad))
	r.bg.Resize(fyne.NewSize(s.Width-pad*2, ph))
	w := s.Width - pad*2
	r.surface.Move(fyne.NewPos(pad+10, pad+12))
	r.surface.Resize(fyne.NewSize(w-20, ph-24))
	r.bar.Move(fyne.NewPos(pad+18, pad+20))
	r.bar.Resize(fyne.NewSize(w*0.35, 8))
	r.l1.Move(fyne.NewPos(pad+18, pad+36))
	r.l1.Resize(fyne.NewSize(w*0.55, 5))
	r.l2.Move(fyne.NewPos(pad+18, pad+46))
	r.l2.Resize(fyne.NewSize(w*0.4, 5))
	r.dot.Move(fyne.NewPos(pad+w-34, pad+ph-34))
	r.dot.Resize(fyne.NewSize(16, 16))
	r.name.Move(fyne.NewPos(0, ph+pad*2))
	r.name.Resize(fyne.NewSize(s.Width, nameH))
}

func (r *themeCardRenderer) MinSize() fyne.Size { return fyne.NewSize(120, 96) }

func (r *themeCardRenderer) Refresh() {
	c := r.c
	switch {
	case c.selected:
		c.border.StrokeColor = theme.Color(theme.ColorNamePrimary)
	case c.hovered:
		c.border.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	default:
		c.border.StrokeColor = color.Transparent
	}
	r.name.Color = theme.Color(theme.ColorNameForeground)
	c.border.Refresh()
	r.name.Refresh()
	r.Layout(c.Size())
}

func (r *themeCardRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *themeCardRenderer) Destroy()                     {}
