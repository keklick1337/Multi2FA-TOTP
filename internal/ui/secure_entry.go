package ui

import (
	"image/color"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/awnumar/memguard"

	"github.com/keklick1337/Multi2FA-TOTP/internal/assets"
)

const secureEntryCap = 1024

var (
	liveSecureMu sync.Mutex
	liveSecure   = map[*secureEntry]struct{}{}
)

// wipeSecureEntries destroys every password buffer; used on lock and when screens change.
func wipeSecureEntries() {
	liveSecureMu.Lock()
	list := make([]*secureEntry, 0, len(liveSecure))
	for e := range liveSecure {
		list = append(list, e)
	}
	liveSecureMu.Unlock()
	for _, e := range list {
		e.Clear()
	}
}

// secureEntry is a password field that keeps the typed bytes in mlocked, guarded memory
// (memguard) instead of a Go string, and never renders the text.
type secureEntry struct {
	widget.BaseWidget
	buf         *memguard.LockedBuffer
	n           int
	runes       []uint8
	placeholder string
	focused     bool
	disabled    bool
	hovered     bool

	OnSubmitted func()
	OnChanged   func()
}

func newSecureEntry(placeholder string) *secureEntry {
	e := &secureEntry{placeholder: placeholder}
	e.ExtendBaseWidget(e)
	return e
}

// Bytes is a view into locked memory. Do not keep it after Clear.
func (e *secureEntry) Bytes() []byte {
	if e.buf == nil {
		return nil
	}
	return e.buf.Bytes()[:e.n]
}

func (e *secureEntry) Len() int { return len(e.runes) }

// Copy returns the password in a fresh locked buffer owned by the caller (nil if empty).
func (e *secureEntry) Copy() *memguard.LockedBuffer {
	if e.n == 0 {
		return nil
	}
	return memguard.NewBufferFromBytes(append([]byte(nil), e.Bytes()...))
}

// Equal compares two fields in constant time for equal lengths.
func (e *secureEntry) Equal(o *secureEntry) bool {
	a, b := e.Bytes(), o.Bytes()
	if len(a) != len(b) {
		return false
	}
	var d byte
	for i := range a {
		d |= a[i] ^ b[i]
	}
	return d == 0
}

func (e *secureEntry) Clear() {
	if e.buf != nil {
		e.buf.Destroy()
		e.buf = nil
	}
	e.n = 0
	e.runes = e.runes[:0]
	liveSecureMu.Lock()
	delete(liveSecure, e)
	liveSecureMu.Unlock()
	e.Refresh()
}

func (e *secureEntry) Disable() { e.disabled = true; e.Refresh() }
func (e *secureEntry) Enable()  { e.disabled = false; e.Refresh() }
func (e *secureEntry) Disabled() bool {
	return e.disabled
}

func (e *secureEntry) appendRune(r rune) {
	if e.disabled || r < 0x20 {
		return
	}
	var tmp [utf8.UTFMax]byte
	l := utf8.EncodeRune(tmp[:], r)
	if e.n+l > secureEntryCap {
		return
	}
	if e.buf == nil {
		e.buf = memguard.NewBuffer(secureEntryCap)
		liveSecureMu.Lock()
		liveSecure[e] = struct{}{}
		liveSecureMu.Unlock()
	}
	copy(e.buf.Bytes()[e.n:], tmp[:l])
	memguard.WipeBytes(tmp[:])
	e.n += l
	e.runes = append(e.runes, uint8(l))
	e.changed()
}

func (e *secureEntry) backspace() {
	if e.disabled || len(e.runes) == 0 {
		return
	}
	l := int(e.runes[len(e.runes)-1])
	e.runes = e.runes[:len(e.runes)-1]
	memguard.WipeBytes(e.buf.Bytes()[e.n-l : e.n])
	e.n -= l
	e.changed()
}

func (e *secureEntry) changed() {
	e.Refresh()
	if e.OnChanged != nil {
		e.OnChanged()
	}
}

func (e *secureEntry) TypedRune(r rune) { e.appendRune(r) }

func (e *secureEntry) TypedKey(k *fyne.KeyEvent) {
	switch k.Name {
	case fyne.KeyBackspace:
		e.backspace()
	case fyne.KeyReturn, fyne.KeyEnter:
		if e.OnSubmitted != nil && !e.disabled {
			e.OnSubmitted()
		}
	}
}

func (e *secureEntry) TypedShortcut(s fyne.Shortcut) {
	if _, ok := s.(*fyne.ShortcutPaste); ok {
		text := fyne.CurrentApp().Clipboard().Content()
		for _, r := range strings.TrimRight(text, "\r\n") {
			e.appendRune(r)
		}
	}
}

func (e *secureEntry) FocusGained() { e.focused = true; e.Refresh() }
func (e *secureEntry) FocusLost()   { e.focused = false; e.Refresh() }

func (e *secureEntry) Tapped(*fyne.PointEvent) {
	if e.disabled {
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(e); c != nil {
		c.Focus(e)
	}
}

func (e *secureEntry) MouseIn(*desktop.MouseEvent)    { e.hovered = true; e.Refresh() }
func (e *secureEntry) MouseMoved(*desktop.MouseEvent) {}
func (e *secureEntry) MouseOut()                      { e.hovered = false; e.Refresh() }
func (e *secureEntry) Cursor() desktop.Cursor         { return desktop.TextCursor }

func (e *secureEntry) CreateRenderer() fyne.WidgetRenderer {
	r := &secureEntryRenderer{e: e,
		bg:     canvas.NewRectangle(color.Transparent),
		text:   canvas.NewText("", color.Black),
		cursor: canvas.NewRectangle(color.Black),
		icon:   canvas.NewImageFromResource(assets.LockIcon),
	}
	r.bg.CornerRadius = theme.InputRadiusSize()
	r.bg.StrokeWidth = theme.InputBorderSize() * 2
	r.Refresh()
	return r
}

type secureEntryRenderer struct {
	e      *secureEntry
	bg     *canvas.Rectangle
	text   *canvas.Text
	cursor *canvas.Rectangle
	icon   *canvas.Image
}

func (r *secureEntryRenderer) MinSize() fyne.Size {
	h := theme.TextSize() + theme.InnerPadding()*2 + theme.InputBorderSize()*2
	return fyne.NewSize(120, h)
}

func (r *secureEntryRenderer) Layout(s fyne.Size) {
	pad := theme.InnerPadding()
	r.bg.Resize(s)
	ts := r.text.MinSize()
	r.text.Move(fyne.NewPos(pad, (s.Height-ts.Height)/2))
	r.text.Resize(fyne.NewSize(s.Width-pad*3-theme.IconInlineSize(), ts.Height))
	is := theme.IconInlineSize()
	r.icon.Resize(fyne.NewSize(is, is))
	r.icon.Move(fyne.NewPos(s.Width-pad-is, (s.Height-is)/2))
	x := pad
	if r.e.n > 0 {
		x += ts.Width
	}
	r.cursor.Resize(fyne.NewSize(2, theme.TextSize()+2))
	r.cursor.Move(fyne.NewPos(min(x, s.Width-pad*2-is), (s.Height-theme.TextSize()-2)/2))
}

func (r *secureEntryRenderer) Refresh() {
	e := r.e
	r.bg.FillColor = theme.Color(theme.ColorNameInputBackground)
	switch {
	case e.focused:
		r.bg.StrokeColor = theme.Color(theme.ColorNamePrimary)
	case e.hovered && !e.disabled:
		r.bg.StrokeColor = theme.Color(theme.ColorNameHover)
	default:
		r.bg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	}
	if len(e.runes) == 0 {
		r.text.Text = e.placeholder
		r.text.Color = theme.Color(theme.ColorNamePlaceHolder)
	} else {
		r.text.Text = strings.Repeat("•", min(len(e.runes), 64))
		r.text.Color = theme.Color(theme.ColorNameForeground)
		if e.disabled {
			r.text.Color = theme.Color(theme.ColorNameDisabled)
		}
	}
	r.text.TextSize = theme.TextSize()
	r.cursor.FillColor = theme.Color(theme.ColorNamePrimary)
	r.cursor.Hidden = !e.focused
	r.bg.Refresh()
	r.text.Refresh()
	r.cursor.Refresh()
	r.icon.Refresh()
	r.Layout(e.Size())
}

func (r *secureEntryRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.text, r.cursor, r.icon}
}
func (r *secureEntryRenderer) Destroy() {}

// strengthOf scores a password from its bytes without making a string copy.
func strengthOf(b []byte) int {
	var lower, upper, digit, other bool
	n := 0
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		b = b[size:]
		n++
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		default:
			other = true
		}
	}
	classes := 0
	for _, f := range []bool{lower, upper, digit, other} {
		if f {
			classes++
		}
	}
	score := 0
	switch {
	case n >= 16:
		score = 3
	case n >= 12:
		score = 2
	case n >= 8:
		score = 1
	}
	if classes >= 3 {
		score++
	}
	if n < 8 {
		score = 0
	}
	return min(score, 4)
}

var (
	_ fyne.Focusable     = (*secureEntry)(nil)
	_ fyne.Shortcutable  = (*secureEntry)(nil)
	_ fyne.Disableable   = (*secureEntry)(nil)
	_ desktop.Hoverable  = (*secureEntry)(nil)
	_ desktop.Cursorable = (*secureEntry)(nil)
)
