package ui

import (
	"fmt"
	"net/url"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/keklick1337/Multi2FA-TOTP/internal/assets"
	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
)

var (
	autoLockChoices  = []int{0, 1, 5, 10, 15, 30, 60}
	clipClearChoices = []int{0, 10, 20, 30, 60, 120}
	themeChoices     = []string{"system", "light", "dark"}
)

func autoLockLabel(m int) string {
	if m == 0 {
		return i18n.T("never")
	}
	return i18n.T("minutes", m)
}

func clipLabel(s int) string {
	if s == 0 {
		return i18n.T("never")
	}
	return i18n.T("seconds", s)
}

func labels(vals []int, f func(int) string) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = f(v)
	}
	return out
}

// intSelect is a dropdown over fixed numeric choices.
func intSelect(choices []int, current int, label func(int) string, set func(int)) *selectBox {
	sel := newSelect(labels(choices, label), nil)
	if !slices.Contains(choices, current) {
		sel.Options = append(sel.Options, label(current))
	}
	sel.SetSelected(label(current))
	sel.OnChanged = func(string) {
		if i := sel.SelectedIndex(); i >= 0 && i < len(choices) {
			set(choices[i])
		}
	}
	return sel
}

// settingRow puts a title and an optional description on the left and a control on the right.
func settingRow(title, desc string, control fyne.CanvasObject) fyne.CanvasObject {
	t := widget.NewLabel(title)
	t.Wrapping = fyne.TextWrapWord
	left := container.NewVBox(t)
	if desc != "" {
		d := widget.NewLabel(desc)
		d.Wrapping = fyne.TextWrapWord
		d.SizeName = theme.SizeNameCaptionText
		left.Add(d)
	}
	return container.NewBorder(nil, nil, nil, container.NewCenter(container.NewGridWrap(fyne.NewSize(170, control.MinSize().Height), control)), left)
}

func (a *App) settingsPage() fyne.CanvasObject {
	T := i18n.T
	s := a.settings()
	main := a.primary()
	persist := func() {
		if err := main.v.SetSettings(s); err != nil {
			a.showError(err)
		}
		a.touch()
		a.cur().view.list.Refresh()
	}

	autoLock := intSelect(autoLockChoices, s.AutoLockMinutes, autoLockLabel, func(v int) { s.AutoLockMinutes = v; persist() })
	clip := intSelect(clipClearChoices, s.ClipboardClearSecs, clipLabel, func(v int) { s.ClipboardClearSecs = v; persist() })
	focusLock := newCheck("", func(b bool) { s.LockOnFocusLoss = b; persist() })
	focusLock.SetChecked(s.LockOnFocusLoss)
	hide := newCheck("", func(b bool) { s.HideCodes = b; persist() })
	hide.SetChecked(s.HideCodes)

	security := card(T("settings.security"),
		settingRow(T("settings.autolock"), T("settings.autolock.desc"), autoLock),
		settingRow(T("settings.clipboard"), T("settings.clipboard.desc"), clip),
		settingRow(T("settings.focuslock"), "", focusLock),
		settingRow(T("settings.hidecodes"), T("settings.hidecodes.desc"), hide),
	)

	lang := newSelect(i18n.Names(), nil)
	lang.SetSelected(i18n.NameOf(i18n.Current()))
	lang.OnChanged = func(name string) {
		if code := i18n.CodeOf(name); code != i18n.Current() {
			a.setLanguage(code)
			a.rebuild()
		}
	}
	themeNames := []string{T("theme.system"), T("theme.light"), T("theme.dark")}
	themeSel := newSelect(themeNames, nil)
	current := a.fa.Preferences().StringWithFallback(prefTheme, "system")
	themeSel.SetSelectedIndex(max(0, slices.Index(themeChoices, current)))
	themeSel.OnChanged = func(string) {
		mode := themeChoices[max(0, themeSel.SelectedIndex())]
		if mode == a.fa.Preferences().StringWithFallback(prefTheme, "system") {
			return
		}
		a.fa.Preferences().SetString(prefTheme, mode)
		applyTheme(a.fa, mode)
		a.rebuild()
	}
	tray := newCheck("", func(b bool) { a.fa.Preferences().SetBool(prefCloseToTray, b) })
	tray.SetChecked(a.fa.Preferences().Bool(prefCloseToTray))
	appearance := card(T("settings.appearance"),
		settingRow(T("settings.language"), "", lang),
		settingRow(T("settings.mode"), "", themeSel),
		settingRow(T("settings.tray"), T("settings.tray.desc"), tray),
	)
	colors := card(T("theme.title"), a.themeGallery())

	cur := a.cur()
	protection := card(T("kdf.protection"),
		labelWrap(T("kdf.current.vault", cur.name(), securitySummary(cur.v.Security(), cur.v.KeyFileRequired()))),
		labelWrap(T("kdf.memory.note")),
		container.NewHBox(newIconButton(T("password.change"), theme.AccountIcon(), func() { a.changeMasterKey(cur) })),
	)

	path := widget.NewLabel(main.path)
	path.Wrapping = fyne.TextWrapBreak
	path.Selectable = true
	path.SizeName = theme.SizeNameCaptionText
	storageItems := []fyne.CanvasObject{
		wrapLabel(T("settings.storage.desc", main.name())),
		path,
		container.NewHBox(newIconButton(T("vault.showfile"), theme.FolderOpenIcon(), func() { a.reveal(main.path) })),
	}
	if fileAssocSupported {
		storageItems = append(storageItems, settingRow(T("assoc.title"), T("assoc.desc"),
			newIconButton(T("assoc.button"), theme.FileIcon(), func() {
				if err := associateFiles(); err != nil {
					a.showError(err)
					return
				}
				a.notify(T("assoc.done"), "")
			})))
	}
	storage := card(T("settings.storage"), storageItems...)

	shortcuts := card(T("help.shortcuts"), a.shortcutsGrid())
	about := a.aboutCard()
	return container.NewVScroll(container.NewPadded(container.NewVBox(
		pageHeader(T("nav.settings")),
		security,
		protection,
		appearance,
		colors,
		storage,
		shortcuts,
		about,
	)))
}

func (a *App) shortcutsGrid() fyne.CanvasObject {
	T := i18n.T
	mod := "Ctrl"
	if isMac() {
		mod = "⌘"
	}
	rows := [][2]string{
		{mod + "+F", T("help.sc.search")},
		{"Enter", T("help.sc.enter")},
		{"Esc", T("help.sc.esc")},
		{mod + "+V", T("help.sc.paste")},
		{mod + "+N", T("add.manual")},
		{mod + "+O", T("add.image")},
		{mod + "+Shift+S", T("add.screen")},
		{mod + "+Shift+K", T("add.camera")},
		{mod + "+E", T("select.mode")},
		{mod + "+1", T("nav.accounts")},
		{mod + "+Shift+O", T("nav.vaults")},
		{mod + "+,", T("nav.settings")},
		{mod + "+L", T("lock.now")},
	}
	grid := container.NewGridWithColumns(2)
	for _, r := range rows {
		grid.Add(widget.NewLabelWithStyle(r[0], fyne.TextAlignLeading, fyne.TextStyle{Monospace: true, Bold: true}))
		l := widget.NewLabel(r[1])
		l.Truncation = fyne.TextTruncateEllipsis
		grid.Add(l)
	}
	tip := widget.NewLabel(T("help.tips"))
	tip.Wrapping = fyne.TextWrapWord
	return container.NewVBox(grid, tip)
}

func (a *App) aboutCard() fyne.CanvasObject {
	T := i18n.T
	logo := canvas.NewImageFromResource(assets.Icon)
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(72, 72))
	title := widget.NewLabelWithStyle(fmt.Sprintf("%s %s", AppName, Version), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	desc := widget.NewLabel(T("about.text"))
	desc.Wrapping = fyne.TextWrapWord
	author := widget.NewLabel(T("about.author", Author))
	repo, _ := url.Parse(RepoURL)
	link := widget.NewHyperlink(RepoURL, repo)
	link.Truncation = fyne.TextTruncateEllipsis
	return card(T("help.about"), container.NewBorder(nil, nil, container.NewCenter(logo), nil,
		container.NewVBox(title, author, link)), desc)
}
