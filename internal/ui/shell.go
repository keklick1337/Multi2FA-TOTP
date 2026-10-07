package ui

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/keklick1337/Multi2FA-TOTP/internal/assets"
	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
)

type pageID int

const (
	pageAccounts pageID = iota
	pageVaults
	pageSettings
)

// wideBreakpoint switches between the left rail and the bottom bar.
const wideBreakpoint = 640

// shell is the unlocked chrome: navigation, the current page and the toast layer.
type shell struct {
	a      *App
	root   fyne.CanvasObject
	page   pageID
	holder *fyne.Container
	slide  *slideLayout
	nav    *fyne.Container
	navL   *navLayout
	sep    *canvas.Rectangle
	items  map[pageID]*navButton
	addBtn *navButton
	toast  *toast
	anim   *fyne.Animation
	drag   *fyne.Container
}

func newShell(a *App) *shell {
	T := i18n.T
	s := &shell{a: a, items: map[pageID]*navButton{}, toast: newToast()}

	logo := canvas.NewImageFromResource(assets.Icon)
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(36, 36))

	s.items[pageAccounts] = newNavButton(theme.HomeIcon(), T("nav.accounts"), func() { s.show(pageAccounts, true) })
	s.addBtn = newNavButton(theme.ContentAddIcon(), T("nav.add"), nil)
	s.addBtn.accent = true
	s.addBtn.onTap = func() { a.touch(); a.showAddMenu(s.addBtn) }
	s.items[pageVaults] = newNavButton(theme.StorageIcon(), T("nav.vaults"), func() { s.show(pageVaults, true) })
	s.items[pageSettings] = newNavButton(theme.SettingsIcon(), T("nav.settings"), func() { s.show(pageSettings, true) })
	lock := newNavButton(assets.LockIcon, T("nav.lock"), a.lock)

	s.navL = &navLayout{}
	s.nav = container.New(s.navL, logo, s.items[pageAccounts], s.addBtn, s.items[pageVaults], s.items[pageSettings], lock)
	navBG := canvas.NewRectangle(theme.Color(theme.ColorNameHeaderBackground))
	s.sep = canvas.NewRectangle(theme.Color(theme.ColorNameSeparator))

	s.slide = &slideLayout{}
	s.holder = container.New(s.slide)
	content := container.NewStack(s.holder, s.toast.holder)

	sl := &shellLayout{s: s}
	s.drag = container.NewWithoutLayout()
	s.root = container.NewStack(newActivityArea(a.touch), container.New(sl, navBG, s.nav, s.sep, content), s.drag)
	return s
}

func (s *shell) show(p pageID, animate bool) {
	a := s.a
	a.touch()
	var obj fyne.CanvasObject
	switch p {
	case pageAccounts:
		obj = a.accounts.content
	case pageVaults:
		obj = a.vaultsPage()
	case pageSettings:
		obj = a.settingsPage()
	}
	same := s.page == p && len(s.holder.Objects) > 0 && s.holder.Objects[0] == obj
	s.page = p
	for id, b := range s.items {
		b.setActive(id == p)
	}
	s.holder.Objects = []fyne.CanvasObject{obj}
	s.holder.Refresh()
	if animate && !same {
		s.animate()
	}
	if p == pageAccounts {
		a.cur().view.applyFilter()
	}
}

// refreshPage rebuilds the visible page (vault and settings pages are generated on demand).
func (s *shell) refreshPage() {
	if s.page != pageAccounts {
		s.show(s.page, false)
	}
}

func (s *shell) animate() {
	if s.anim != nil {
		s.anim.Stop()
	}
	s.anim = fyne.NewAnimation(220*time.Millisecond, func(f float32) {
		s.slide.offset = (1 - f) * 28
		s.holder.Refresh()
	})
	s.anim.Curve = fyne.AnimationEaseOut
	s.anim.Start()
}

// shellLayout objects: nav background, nav, separator, content.
type shellLayout struct{ s *shell }

const (
	railWidth = 92
	barHeight = 64
)

func (l *shellLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	bg, nav, sep, content := objs[0], objs[1], objs[2], objs[3]
	if size.Width >= wideBreakpoint {
		l.s.navL.vertical = true
		bg.Move(fyne.NewPos(0, 0))
		bg.Resize(fyne.NewSize(railWidth, size.Height))
		nav.Move(fyne.NewPos(0, 0))
		nav.Resize(fyne.NewSize(railWidth, size.Height))
		sep.Move(fyne.NewPos(railWidth, 0))
		sep.Resize(fyne.NewSize(1, size.Height))
		content.Move(fyne.NewPos(railWidth+1, 0))
		content.Resize(fyne.NewSize(size.Width-railWidth-1, size.Height))
	} else {
		l.s.navL.vertical = false
		y := size.Height - barHeight
		content.Move(fyne.NewPos(0, 0))
		content.Resize(fyne.NewSize(size.Width, y-1))
		sep.Move(fyne.NewPos(0, y-1))
		sep.Resize(fyne.NewSize(size.Width, 1))
		bg.Move(fyne.NewPos(0, y))
		bg.Resize(fyne.NewSize(size.Width, barHeight))
		nav.Move(fyne.NewPos(0, y))
		nav.Resize(fyne.NewSize(size.Width, barHeight))
	}
	nav.Refresh()
}

// Small enough for a narrow side panel, tall enough to show one account under the header.
const (
	minWindowWidth  = 340
	minWindowHeight = 300
)

func (l *shellLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	c := objs[3].MinSize()
	return fyne.NewSize(max(c.Width, minWindowWidth), max(c.Height+barHeight+1, minWindowHeight))
}

const navItemWidth = 64

// navLayout objects: logo, then buttons; the last button (lock) is pinned to the end.
type navLayout struct{ vertical bool }

func (l *navLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	logo, buttons := objs[0], objs[1:]
	pad := theme.Padding()
	if l.vertical {
		logo.Show()
		ls := logo.MinSize()
		logo.Resize(ls)
		logo.Move(fyne.NewPos((size.Width-ls.Width)/2, pad*3))
		y := ls.Height + pad*6
		h := float32(60)
		for i, b := range buttons {
			b.Resize(fyne.NewSize(size.Width-pad*2, h))
			if i == len(buttons)-1 {
				b.Move(fyne.NewPos(pad, size.Height-h-pad*2))
				continue
			}
			b.Move(fyne.NewPos(pad, y))
			y += h + pad
		}
		return
	}
	logo.Hide()
	w := size.Width / float32(len(buttons))
	for i, b := range buttons {
		b.Resize(fyne.NewSize(w, size.Height-pad))
		b.Move(fyne.NewPos(float32(i)*w, pad/2))
	}
}

func (l *navLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(5*navItemWidth, barHeight)
}

// slideLayout fills its child and shifts it horizontally for page transitions.
type slideLayout struct{ offset float32 }

func (l *slideLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Resize(size)
		o.Move(fyne.NewPos(l.offset, 0))
	}
}

// MinSize ignores the pages on purpose: a page never resizes the window. Pages adapt to the
// space they get (wrapping grids, scrolling), so switching pages keeps the window size stable.
func (l *slideLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, 0)
}

// navButton is an icon with a caption, used in the rail and the bottom bar.
type navButton struct {
	widget.BaseWidget
	icon    fyne.Resource
	label   string
	onTap   func()
	active  bool
	accent  bool
	hovered bool
	bg      *canvas.Rectangle
	img     *canvas.Image
	text    *canvas.Text
	anim    *fyne.Animation
}

func newNavButton(icon fyne.Resource, label string, onTap func()) *navButton {
	b := &navButton{icon: icon, label: label, onTap: onTap}
	b.ExtendBaseWidget(b)
	return b
}

func (b *navButton) setActive(on bool) {
	if b.anim != nil {
		b.anim.Stop()
		b.anim = nil
	}
	b.active = on
	b.Refresh()
}

func (b *navButton) CreateRenderer() fyne.WidgetRenderer {
	b.bg = canvas.NewRectangle(color.Transparent)
	b.bg.CornerRadius = 12
	b.img = canvas.NewImageFromResource(b.icon)
	b.img.FillMode = canvas.ImageFillContain
	b.text = canvas.NewText(b.label, theme.Color(theme.ColorNameForeground))
	b.text.TextSize = 11
	b.text.Alignment = fyne.TextAlignCenter
	r := &navButtonRenderer{b: b}
	r.Refresh()
	return r
}

func (b *navButton) Tapped(*fyne.PointEvent) {
	if b.onTap != nil {
		b.onTap()
	}
}

func (b *navButton) MouseIn(*desktop.MouseEvent) {
	b.hovered = true
	b.fade(true)
}
func (b *navButton) MouseMoved(*desktop.MouseEvent) {}
func (b *navButton) MouseOut() {
	b.hovered = false
	b.fade(false)
}
func (b *navButton) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (b *navButton) restColor() color.Color {
	if b.active {
		return withAlpha(theme.Color(theme.ColorNamePrimary), 0x30)
	}
	return color.Transparent
}

func (b *navButton) fade(in bool) {
	if b.bg == nil || b.accent {
		return
	}
	if b.anim != nil {
		b.anim.Stop()
	}
	to := b.restColor()
	if in && !b.active {
		to = theme.Color(theme.ColorNameHover)
	}
	b.anim = canvas.NewColorRGBAAnimation(b.bg.FillColor, to, 150*time.Millisecond, func(c color.Color) {
		b.bg.FillColor = c
		b.bg.Refresh()
	})
	b.anim.Start()
}

type navButtonRenderer struct{ b *navButton }

func (r *navButtonRenderer) Layout(size fyne.Size) {
	b := r.b
	icon := float32(24)
	if b.accent {
		d := float32(46)
		b.bg.Resize(fyne.NewSize(d, d))
		b.bg.Move(fyne.NewPos((size.Width-d)/2, (size.Height-d)/2))
		b.img.Resize(fyne.NewSize(icon+4, icon+4))
		b.img.Move(fyne.NewPos((size.Width-icon-4)/2, (size.Height-icon-4)/2))
		b.text.Hide()
		return
	}
	b.text.Show()
	th := b.text.MinSize().Height
	top := (size.Height - icon - th - 2) / 2
	b.bg.Resize(fyne.NewSize(min(size.Width-4, 64), size.Height))
	b.bg.Move(fyne.NewPos((size.Width-b.bg.Size().Width)/2, 0))
	b.img.Resize(fyne.NewSize(icon, icon))
	b.img.Move(fyne.NewPos((size.Width-icon)/2, top))
	b.text.Resize(fyne.NewSize(size.Width, th))
	b.text.Move(fyne.NewPos(0, top+icon+2))
}

func (r *navButtonRenderer) MinSize() fyne.Size { return fyne.NewSize(56, 52) }

func (r *navButtonRenderer) Refresh() {
	b := r.b
	switch {
	case b.accent:
		b.bg.CornerRadius = 23
		b.bg.FillColor = theme.Color(theme.ColorNamePrimary)
		b.img.Resource = theme.NewColoredResource(b.icon, theme.ColorNameForegroundOnPrimary)
	case b.active:
		b.bg.FillColor = b.restColor()
		b.img.Resource = theme.NewPrimaryThemedResource(b.icon)
		b.text.Color = theme.Color(theme.ColorNamePrimary)
		b.text.TextStyle.Bold = true
	default:
		if !b.hovered {
			b.bg.FillColor = color.Transparent
		}
		b.img.Resource = theme.NewThemedResource(b.icon)
		b.text.Color = theme.Color(theme.ColorNamePlaceHolder)
		b.text.TextStyle.Bold = false
	}
	b.bg.Refresh()
	b.img.Refresh()
	b.text.Refresh()
	r.Layout(b.Size())
}

func (r *navButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.b.bg, r.b.img, r.b.text}
}
func (r *navButtonRenderer) Destroy() {}

// toast is a two-line notification sliding in from the bottom.
type toast struct {
	holder *fyne.Container
	lay    *toastLayout
	card   fyne.CanvasObject
	title  *widget.Label
	detail *widget.Label
	seq    int
	anim   *fyne.Animation
}

func newToast() *toast {
	t := &toast{}
	t.title = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	t.title.Truncation = fyne.TextTruncateEllipsis
	t.detail = widget.NewLabel("")
	t.detail.Truncation = fyne.TextTruncateEllipsis
	t.detail.SizeName = theme.SizeNameCaptionText
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameOverlayBackground))
	bg.CornerRadius = 12
	bg.StrokeColor = theme.Color(theme.ColorNameSeparator)
	bg.StrokeWidth = 1
	icon := widget.NewIcon(theme.NewSuccessThemedResource(theme.ConfirmIcon()))
	body := container.NewBorder(nil, nil, container.NewCenter(icon), nil, container.New(layout.NewCustomPaddedVBoxLayout(-10), t.title, t.detail))
	t.card = container.NewStack(bg, container.NewPadded(body))
	t.card.Hide()
	t.lay = &toastLayout{}
	t.holder = container.New(t.lay, t.card)
	return t
}

func (t *toast) show(title, detail string) {
	t.seq++
	seq := t.seq
	t.title.SetText(title)
	t.detail.SetText(detail)
	if detail == "" {
		t.detail.Hide()
	} else {
		t.detail.Show()
	}
	t.card.Show()
	t.slide(1, 0)
	time.AfterFunc(3500*time.Millisecond, func() {
		fyne.Do(func() {
			if seq == t.seq {
				t.slide(0, 1)
			}
		})
	})
}

func (t *toast) slide(from, to float32) {
	if t.anim != nil {
		t.anim.Stop()
	}
	hide := to == 1
	t.anim = fyne.NewAnimation(220*time.Millisecond, func(f float32) {
		t.lay.drop = from + (to-from)*f
		t.holder.Refresh()
		if hide && f == 1 {
			t.card.Hide()
		}
	})
	t.anim.Curve = fyne.AnimationEaseOut
	t.anim.Start()
}

// toastLayout places the card at the bottom center; drop 0..1 slides it out of view.
type toastLayout struct{ drop float32 }

func (l *toastLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	c := objs[0]
	w := min(size.Width-24, 460)
	h := c.MinSize().Height
	c.Resize(fyne.NewSize(w, h))
	c.Move(fyne.NewPos((size.Width-w)/2, size.Height-h-14+(h+20)*l.drop))
}

func (l *toastLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
