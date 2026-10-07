package ui

import (
	"hash/fnv"
	"image/color"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// activityArea is an invisible background that reports mouse activity for auto-lock.
type activityArea struct {
	widget.BaseWidget
	on func()
}

func newActivityArea(on func()) *activityArea {
	a := &activityArea{on: on}
	a.ExtendBaseWidget(a)
	return a
}

func (a *activityArea) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

func (a *activityArea) MouseIn(*desktop.MouseEvent)    { a.on() }
func (a *activityArea) MouseMoved(*desktop.MouseEvent) { a.on() }
func (a *activityArea) MouseOut()                      {}
func (a *activityArea) Tapped(*fyne.PointEvent)        { a.on() }

var _ desktop.Hoverable = (*activityArea)(nil)

// searchEntry intercepts paste (so images can be pasted while it has focus) and Enter/Escape.
type searchEntry struct {
	widget.Entry
	onPaste          func() bool
	onSubmit         func()
	onEscape         func()
	onKey            func()
	onShortcut       func(fyne.Shortcut) bool
	onBackspaceEmpty func()
}

func newSearchEntry() *searchEntry {
	e := &searchEntry{}
	e.ExtendBaseWidget(e)
	return e
}

func (e *searchEntry) TypedShortcut(s fyne.Shortcut) {
	if e.onKey != nil {
		e.onKey()
	}
	if _, ok := s.(*fyne.ShortcutPaste); ok && e.onPaste != nil && e.onPaste() {
		return
	}
	if _, ok := s.(*desktop.CustomShortcut); ok && e.onShortcut != nil && e.onShortcut(s) {
		return
	}
	e.Entry.TypedShortcut(s)
}

func (e *searchEntry) TypedKey(k *fyne.KeyEvent) {
	if e.onKey != nil {
		e.onKey()
	}
	switch k.Name {
	case fyne.KeyReturn, fyne.KeyEnter:
		if e.onSubmit != nil {
			e.onSubmit()
		}
	case fyne.KeyEscape:
		if e.onEscape != nil {
			e.onEscape()
		}
	case fyne.KeyBackspace:
		if e.Text == "" && e.onBackspaceEmpty != nil {
			e.onBackspaceEmpty()
			return
		}
		e.Entry.TypedKey(k)
	default:
		e.Entry.TypedKey(k)
	}
}

// timeBar is a thin countdown bar.
type timeBar struct {
	widget.BaseWidget
	frac float32
	col  color.Color
}

func newTimeBar() *timeBar {
	b := &timeBar{frac: 1}
	b.ExtendBaseWidget(b)
	return b
}

func (b *timeBar) Set(frac float32, col color.Color) {
	b.frac, b.col = frac, col
	b.Refresh()
}

func (b *timeBar) CreateRenderer() fyne.WidgetRenderer {
	r := &timeBarRenderer{b: b, bg: canvas.NewRectangle(theme.Color(theme.ColorNameInputBorder)), fg: canvas.NewRectangle(theme.Color(theme.ColorNamePrimary))}
	r.bg.CornerRadius, r.fg.CornerRadius = 2, 2
	return r
}

type timeBarRenderer struct {
	b      *timeBar
	bg, fg *canvas.Rectangle
}

func (r *timeBarRenderer) Layout(s fyne.Size) {
	r.bg.Resize(s)
	r.fg.Resize(fyne.NewSize(s.Width*r.b.frac, s.Height))
}
func (r *timeBarRenderer) MinSize() fyne.Size { return fyne.NewSize(96, 4) }
func (r *timeBarRenderer) Refresh() {
	r.bg.FillColor = theme.Color(theme.ColorNameInputBorder)
	if r.b.col != nil {
		r.fg.FillColor = r.b.col
	}
	r.Layout(r.b.Size())
	r.bg.Refresh()
	r.fg.Refresh()
}
func (r *timeBarRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.bg, r.fg} }
func (r *timeBarRenderer) Destroy()                     {}

// avatar is a colored circle with the issuer's initial.
type avatar struct {
	circle *canvas.Circle
	text   *canvas.Text
	obj    fyne.CanvasObject
}

var avatarPalette = []color.NRGBA{
	{0x3b, 0x82, 0xf6, 0xff}, {0x8b, 0x5c, 0xf6, 0xff}, {0xec, 0x48, 0x99, 0xff},
	{0xef, 0x44, 0x44, 0xff}, {0xf5, 0x9e, 0x0b, 0xff}, {0x10, 0xb9, 0x81, 0xff},
	{0x06, 0xb6, 0xd4, 0xff}, {0x64, 0x74, 0x8b, 0xff}, {0x84, 0xcc, 0x16, 0xff},
}

func newAvatar() *avatar {
	a := &avatar{circle: canvas.NewCircle(avatarPalette[0]), text: canvas.NewText("", color.White)}
	a.text.TextStyle.Bold = true
	a.text.TextSize = 17
	a.text.Alignment = fyne.TextAlignCenter
	a.obj = container.NewGridWrap(fyne.NewSize(40, 40), container.NewStack(a.circle, container.NewCenter(a.text)))
	return a
}

func (a *avatar) Set(name string) {
	initial := "?"
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			initial = string(unicode.ToUpper(r))
			break
		}
	}
	h := fnv.New32a()
	h.Write([]byte(name))
	a.circle.FillColor = avatarPalette[h.Sum32()%uint32(len(avatarPalette))]
	a.text.Text = initial
	a.circle.Refresh()
	a.text.Refresh()
}

// collapsible is a titled section that can be expanded; onToggle lets the owner refresh the
// surrounding scroll container, which Fyne does not resize by itself.
func collapsible(title string, content fyne.CanvasObject, onToggle func()) fyne.CanvasObject {
	content.Hide()
	var btn *button
	btn = newIconButton(title, theme.MenuExpandIcon(), func() {
		if content.Visible() {
			content.Hide()
			btn.SetIcon(theme.MenuExpandIcon())
		} else {
			content.Show()
			btn.SetIcon(theme.MenuDropDownIcon())
		}
		if onToggle != nil {
			onToggle()
		}
	})
	btn.Alignment = widget.ButtonAlignLeading
	btn.Importance = widget.LowImportance
	return container.NewVBox(btn, content)
}
