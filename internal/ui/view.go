package ui

import (
	"image/color"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
	"github.com/keklick1337/Multi2FA-TOTP/internal/otp"
	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

const revealDuration = 15 * time.Second

// listItem is either a folder or an account row.
type listItem struct {
	f    *vault.Folder
	e    *vault.Entry
	path string
}

// vaultView is the account list of one open vault, positioned in one of its folders.
type vaultView struct {
	a        *App
	s        *session
	content  fyne.CanvasObject
	list     *widget.List
	empty    fyne.CanvasObject
	emptyMsg *widget.Label
	noMatch  fyne.CanvasObject
	folder   string
	all      []*vault.Entry
	folders  []*vault.Folder
	counts   map[string]int
	items    []listItem
	shown    []*vault.Entry
	revealed map[string]time.Time
	selected map[string]bool
	flash    string
	rows     map[*entryRow]bool
	dropRow  *entryRow
}

func newVaultView(a *App, s *session) *vaultView {
	T := i18n.T
	m := &vaultView{a: a, s: s, revealed: map[string]time.Time{}, selected: map[string]bool{}, rows: map[*entryRow]bool{}}

	m.list = widget.NewList(
		func() int { return len(m.items) },
		func() fyne.CanvasObject {
			obj, r := newRow(m)
			m.rows[r] = true
			return obj
		},
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id < len(m.items) {
				rowOf(o).set(m.items[id])
			}
		},
	)

	m.emptyMsg = widget.NewLabel(T("empty.text"))
	m.emptyMsg.Alignment = fyne.TextAlignCenter
	m.emptyMsg.Wrapping = fyne.TextWrapWord
	icon := canvas.NewImageFromResource(theme.AccountIcon())
	icon.SetMinSize(fyne.NewSize(56, 56))
	// GridWrap reflows the buttons onto more rows when the window is narrow.
	actions := container.NewGridWrap(fyne.NewSize(150, 40))
	if !isMobile() {
		actions.Add(newIconButton(T("add.camera.short"), theme.MediaVideoIcon(), a.scanCamera))
		actions.Add(newIconButton(T("add.screen.short"), theme.ComputerIcon(), a.scanScreen))
	}
	actions.Add(newIconButton(T("add.manual.short"), theme.DocumentCreateIcon(), func() { m.showEditor(nil, nil) }))
	actions.Add(newIconButton(T("folder.new"), theme.FolderNewIcon(), func() { m.newFolder(m.folder) }))
	m.empty = container.NewVBox(layout.NewSpacer(), container.NewCenter(icon), m.emptyMsg, container.NewCenter(actions), layout.NewSpacer())
	noMatch := widget.NewLabel(T("search.nomatch"))
	noMatch.Alignment = fyne.TextAlignCenter
	m.noMatch = container.NewCenter(noMatch)

	m.content = container.NewStack(m.list, m.empty, m.noMatch)
	m.load()
	return m
}

// load reads entries and folders from the vault and precomputes recursive counts.
func (m *vaultView) load() {
	m.all = m.s.v.Entries()
	m.folders = m.s.v.Folders()
	if m.folder != "" && m.s.v.Folder(m.folder) == nil {
		m.folder = ""
	}
	parent := map[string]string{}
	for _, f := range m.folders {
		parent[f.ID] = f.Parent
	}
	m.counts = map[string]int{}
	for _, e := range m.all {
		for id, n := e.Folder, 0; id != "" && n <= len(m.folders); id, n = parent[id], n+1 {
			m.counts[id]++
		}
	}
	for id := range m.selected {
		if m.s.v.Get(id) == nil {
			delete(m.selected, id)
		}
	}
}

// reload re-reads the vault after a change.
func (m *vaultView) reload() {
	m.load()
	m.applyFilter()
}

func (m *vaultView) query() string {
	if m.a.search == nil {
		return ""
	}
	return strings.TrimSpace(m.a.search.Text)
}

func (m *vaultView) applyFilter() {
	q := m.query()
	m.items = m.items[:0]
	m.shown = m.shown[:0]
	if q != "" {
		lq := strings.ToLower(q)
		for _, f := range m.folders {
			if strings.Contains(strings.ToLower(f.Name), lq) {
				m.items = append(m.items, listItem{f: f, path: m.pathText(f.Parent)})
			}
		}
		for _, e := range m.all {
			if e.Matches(q) {
				m.items = append(m.items, listItem{e: e, path: m.pathText(e.Folder)})
				m.shown = append(m.shown, e)
			}
		}
	} else {
		for _, f := range m.folders {
			if f.Parent == m.folder {
				m.items = append(m.items, listItem{f: f})
			}
		}
		for _, e := range m.all {
			if e.Folder == m.folder {
				m.items = append(m.items, listItem{e: e})
				m.shown = append(m.shown, e)
			}
		}
	}
	m.empty.Hide()
	m.noMatch.Hide()
	switch {
	case len(m.items) == 0 && q != "":
		m.noMatch.Show()
	case len(m.items) == 0:
		if m.folder == "" {
			m.emptyMsg.SetText(i18n.T("empty.text"))
		} else {
			m.emptyMsg.SetText(i18n.T("folder.empty"))
		}
		m.empty.Show()
	}
	m.list.UnselectAll()
	m.list.Refresh()
	if m.a.accounts != nil && m.a.cur() == m.s {
		m.a.accounts.updateCount(len(m.shown), len(m.all), q)
		m.a.accounts.updateCrumbs()
		m.a.updateSelection()
	}
}

func (m *vaultView) pathText(folder string) string {
	p := m.s.v.FolderPath(folder)
	if len(p) == 0 {
		return ""
	}
	return strings.Join(p, " / ")
}

// openFolder navigates into a folder ("" is the top level).
func (m *vaultView) openFolder(id string) {
	m.a.touch()
	m.folder = id
	if m.a.search != nil && m.a.search.Text != "" {
		m.a.search.SetText("")
	} else {
		m.applyFilter()
	}
	m.list.ScrollToTop()
}

func (m *vaultView) goUp() {
	if m.folder == "" {
		return
	}
	if f := m.s.v.Folder(m.folder); f != nil {
		m.openFolder(f.Parent)
	} else {
		m.openFolder("")
	}
}

func (m *vaultView) tick() {
	now := time.Now()
	for id, until := range m.revealed {
		if now.After(until) {
			delete(m.revealed, id)
		}
	}
	m.list.Refresh()
}

func (m *vaultView) hidden(e *vault.Entry) bool {
	if !m.a.settings().HideCodes {
		return false
	}
	_, ok := m.revealed[e.ID]
	return !ok
}

func (m *vaultView) selectedEntries() []*vault.Entry {
	var out []*vault.Entry
	for _, e := range m.all {
		if m.selected[e.ID] {
			out = append(out, e)
		}
	}
	return out
}

func (m *vaultView) toggleSelected(e *vault.Entry) {
	m.a.touch()
	if m.selected[e.ID] {
		delete(m.selected, e.ID)
	} else {
		m.selected[e.ID] = true
	}
	m.list.Refresh()
	m.a.updateSelection()
}

func (m *vaultView) selectAll(on bool) {
	m.a.touch()
	clear(m.selected)
	if on {
		for _, e := range m.shown {
			m.selected[e.ID] = true
		}
	}
	m.list.Refresh()
	m.a.updateSelection()
}

func (m *vaultView) copyCode(e *vault.Entry) {
	m.a.touch()
	code, _, err := e.Code(time.Now())
	if err != nil {
		m.a.showError(err)
		return
	}
	m.a.copySecret(code)
	m.revealed[e.ID] = time.Now().Add(revealDuration)
	m.flash = e.ID
	m.list.Refresh()
	m.flash = ""
	m.a.notify(i18n.T("copied", e.Label()), m.a.clipboardNote())
}

func (m *vaultView) nextHOTP(e *vault.Entry) {
	m.a.touch()
	if err := m.s.v.IncrementCounter(e.ID); err != nil {
		m.a.showError(err)
		return
	}
	m.reload()
	if fresh := m.s.v.Get(e.ID); fresh != nil {
		m.copyCode(fresh)
	}
}

func (m *vaultView) toggleFavorite(e *vault.Entry) {
	c := *e
	c.Favorite = !c.Favorite
	if err := m.s.v.Update(&c); err != nil {
		m.a.showError(err)
		return
	}
	m.reload()
}

func (m *vaultView) confirmDelete(e *vault.Entry) {
	T := i18n.T
	d := dialog.NewConfirm(T("delete.title"), T("delete.message", e.Label()), func(ok bool) {
		if !ok {
			return
		}
		if err := m.s.v.Delete(e.ID); err != nil {
			m.a.showError(err)
			return
		}
		m.reload()
		m.a.notify(T("deleted", e.Label()), "")
	}, m.a.win)
	d.SetConfirmImportance(widget.DangerImportance)
	d.Show()
}

func (m *vaultView) entryMenu(e *vault.Entry) *fyne.Menu {
	T := i18n.T
	fav := T("menu.favorite")
	if e.Favorite {
		fav = T("menu.unfavorite")
	}
	items := []*fyne.MenuItem{
		fyne.NewMenuItemWithIcon(T("menu.copy"), theme.ContentCopyIcon(), func() { m.copyCode(e) }),
	}
	if e.Type == otp.HOTP {
		items = append(items, fyne.NewMenuItemWithIcon(T("menu.next"), theme.ViewRefreshIcon(), func() { m.nextHOTP(e) }))
	}
	items = append(items,
		fyne.NewMenuItemWithIcon(fav, theme.ConfirmIcon(), func() { m.toggleFavorite(e) }),
		fyne.NewMenuItemWithIcon(T("menu.edit"), theme.DocumentCreateIcon(), func() { m.showEditor(e, nil) }),
		fyne.NewMenuItemWithIcon(T("folder.moveto"), theme.FolderIcon(), func() {
			m.pickFolder(T("folder.moveto"), "", func(dst string) { m.moveEntries([]string{e.ID}, dst) })
		}),
		fyne.NewMenuItemWithIcon(T("menu.reveal"), theme.VisibilityIcon(), func() { m.a.revealEntry(m.s, e) }),
		fyne.NewMenuItemWithIcon(T("menu.select"), theme.CheckButtonCheckedIcon(), func() {
			if !m.a.selectMode {
				m.a.setSelectMode(true)
			}
			m.toggleSelected(e)
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItemWithIcon(T("menu.delete"), theme.DeleteIcon(), func() { m.confirmDelete(e) }),
	)
	return fyne.NewMenu("", items...)
}

// entryRow renders an account (avatar, names, live code, countdown, actions) or a folder.
// On desktop rows are wrapped in dragRow so they can be dropped on folders; on touch screens
// dragging would steal the list scrolling, so they stay plain.
type entryRow struct {
	widget.BaseWidget
	self     fyne.CanvasObject
	m        *vaultView
	item     listItem
	e        *vault.Entry
	bg       *canvas.Rectangle
	check    *checkBox
	checkPad *canvas.Rectangle
	av       *avatar
	folderIc *widget.Icon
	issuer   *widget.Label
	account  *widget.Label
	codeBox  fyne.CanvasObject
	code     *canvas.Text
	secs     *canvas.Text
	bar      *timeBar
	next     *button
	copyBtn  *button
	menuBtn  *button
	chevron  *widget.Icon
	updating bool
	anim     *fyne.Animation
}

// newRow returns the list object and the row behind it.
func newRow(m *vaultView) (fyne.CanvasObject, *entryRow) {
	r := newEntryRow(m)
	if isMobile() {
		r.self = r
		r.ExtendBaseWidget(r)
		return r, r
	}
	d := &dragRow{entryRow: r}
	r.self = d
	d.ExtendBaseWidget(d)
	return d, r
}

func rowOf(o fyne.CanvasObject) *entryRow {
	if d, ok := o.(*dragRow); ok {
		return d.entryRow
	}
	return o.(*entryRow)
}

func newEntryRow(m *vaultView) *entryRow {
	r := &entryRow{m: m}
	r.bg = canvas.NewRectangle(color.Transparent)
	r.bg.CornerRadius = theme.InputRadiusSize()
	r.check = newCheck("", func(bool) {
		if !r.updating && r.e != nil {
			m.toggleSelected(r.e)
		}
	})
	// Folders cannot be ticked; in selection mode this keeps them aligned with the accounts.
	r.checkPad = canvas.NewRectangle(color.Transparent)
	r.checkPad.SetMinSize(r.check.MinSize())
	r.checkPad.Hide()
	r.av = newAvatar()
	r.folderIc = widget.NewIcon(theme.NewPrimaryThemedResource(theme.FolderIcon()))
	r.issuer = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	r.issuer.Truncation = fyne.TextTruncateEllipsis
	r.account = widget.NewLabel("")
	r.account.Truncation = fyne.TextTruncateEllipsis
	r.account.SizeName = theme.SizeNameCaptionText

	r.code = canvas.NewText("000 000", theme.Color(theme.ColorNamePrimary))
	r.code.TextSize = 22
	r.code.TextStyle = fyne.TextStyle{Monospace: true, Bold: true}
	r.code.Alignment = fyne.TextAlignTrailing
	r.secs = canvas.NewText("", theme.Color(theme.ColorNamePlaceHolder))
	r.secs.TextSize = 11
	r.secs.TextStyle.Monospace = true
	r.bar = newTimeBar()

	r.next = newIconButton("", theme.ViewRefreshIcon(), func() {
		if r.e != nil {
			m.nextHOTP(r.e)
		}
	})
	r.copyBtn = newIconButton("", theme.ContentCopyIcon(), func() {
		if r.e != nil {
			m.copyCode(r.e)
		}
	})
	r.copyBtn.Importance = widget.LowImportance
	r.menuBtn = newIconButton("", theme.MoreVerticalIcon(), nil)
	r.menuBtn.Importance = widget.LowImportance
	r.menuBtn.OnTapped = func() {
		m.a.touch()
		showMenuBelow(m.a.win, r.menuBtn, r.menu())
	}
	r.chevron = widget.NewIcon(theme.NavigateNextIcon())
	return r
}

func (r *entryRow) menu() *fyne.Menu {
	if r.item.f != nil {
		return r.m.folderMenu(r.item.f)
	}
	return r.m.entryMenu(r.e)
}

func (r *entryRow) CreateRenderer() fyne.WidgetRenderer {
	names := container.New(layout.NewCustomPaddedVBoxLayout(-10), r.issuer, r.account)
	r.codeBox = container.NewVBox(
		container.NewHBox(r.code),
		container.NewBorder(nil, nil, nil, r.secs, container.NewCenter(container.NewGridWrap(fyne.NewSize(80, 4), r.bar))),
	)
	right := container.NewHBox(container.NewCenter(r.codeBox), container.NewCenter(r.next), container.NewCenter(r.copyBtn),
		container.NewCenter(r.chevron), container.NewCenter(r.menuBtn))
	icon := container.NewStack(r.av.obj, container.NewCenter(container.NewGridWrap(fyne.NewSize(32, 32), r.folderIc)))
	left := container.NewHBox(container.NewCenter(container.NewStack(r.checkPad, r.check)), container.NewCenter(icon))
	row := container.NewBorder(nil, nil, left, right, container.New(vCenterFillLayout{}, names))
	return widget.NewSimpleRenderer(container.NewStack(r.bg, container.NewPadded(row)))
}

func (r *entryRow) set(it listItem) {
	changed := r.item.e != it.e || r.item.f != it.f
	r.item = it
	r.e = it.e
	if changed {
		r.setBG(color.Transparent)
	}
	if it.f != nil {
		r.setFolder(it)
		return
	}
	e := it.e
	r.av.obj.Show()
	r.folderIc.Hide()
	r.chevron.Hide()
	r.codeBox.Show()
	name := e.Issuer
	if name == "" {
		name = e.Account
	}
	r.av.Set(name)
	title := name
	if title == "" {
		title = i18n.T("unnamed")
	}
	if e.Favorite {
		title = "★ " + title
	}
	r.issuer.SetText(title)
	var parts []string
	if it.path != "" {
		parts = append(parts, it.path)
	}
	if e.Issuer != "" && e.Account != "" {
		parts = append(parts, e.Account)
	}
	if len(e.Tags) > 0 {
		parts = append(parts, "#"+strings.Join(e.Tags, " #"))
	}
	r.account.SetText(strings.Join(parts, "  ·  "))

	r.updating = true
	r.check.SetChecked(r.m.selected[e.ID])
	r.updating = false
	// Rows are recycled: a row that showed a folder before must become clickable again.
	r.checkPad.Hide()
	r.check.Enable()
	if r.m.a.selectMode {
		r.check.Show()
		r.copyBtn.Hide()
	} else {
		r.check.Hide()
		r.copyBtn.Show()
	}
	if r.m.flash == e.ID {
		r.animateBG(withAlpha(theme.Color(theme.ColorNameSuccess), 0x66), color.Transparent, 600*time.Millisecond)
	}
	r.refreshCode()
}

func (r *entryRow) setFolder(it listItem) {
	f := it.f
	r.av.obj.Hide()
	r.folderIc.Show()
	r.codeBox.Hide()
	r.next.Hide()
	r.copyBtn.Hide()
	r.chevron.Show()
	r.check.Hide()
	r.checkPad.Hidden = !r.m.a.selectMode
	r.checkPad.Refresh()
	r.issuer.SetText(f.Name)
	sub := i18n.N("folder.count", r.m.counts[f.ID])
	if it.path != "" {
		sub = it.path + "  ·  " + sub
	}
	r.account.SetText(sub)
	if r == r.m.dropRow {
		r.bg.FillColor = withAlpha(theme.Color(theme.ColorNamePrimary), 0x55)
		r.bg.Refresh()
	}
}

func (r *entryRow) refreshCode() {
	e := r.e
	code, remaining, err := e.Code(time.Now())
	if err != nil {
		code = "------"
	}
	if r.m.hidden(e) {
		code = strings.Repeat("•", len(code))
	}
	r.code.Text = otp.FormatCode(code)
	primary := theme.Color(theme.ColorNamePrimary)
	if e.Type == otp.HOTP {
		r.next.Show()
		r.secs.Text = "#" + strconv.FormatUint(e.Counter, 10)
		r.bar.Hide()
		r.code.Color = primary
	} else {
		r.next.Hide()
		r.bar.Show()
		period := e.Period
		if period <= 0 {
			period = otp.DefaultPeriod
		}
		col := primary
		if remaining <= 5 {
			col = theme.Color(theme.ColorNameError)
		} else if remaining <= 10 {
			col = theme.Color(theme.ColorNameWarning)
		}
		r.code.Color = col
		r.secs.Text = strconv.Itoa(remaining) + "s"
		r.bar.Set(float32(remaining)/float32(period), col)
	}
	r.code.Refresh()
	r.secs.Refresh()
}

func (r *entryRow) setBG(c color.Color) {
	if r.anim != nil {
		r.anim.Stop()
		r.anim = nil
	}
	r.bg.FillColor = c
	r.bg.Refresh()
}

func (r *entryRow) animateBG(from, to color.Color, d time.Duration) {
	if r.anim != nil {
		r.anim.Stop()
	}
	r.anim = canvas.NewColorRGBAAnimation(from, to, d, func(c color.Color) {
		r.bg.FillColor = c
		r.bg.Refresh()
	})
	r.anim.Curve = fyne.AnimationEaseOut
	r.anim.Start()
}

func (r *entryRow) Tapped(*fyne.PointEvent) {
	switch {
	case r.item.f != nil:
		r.m.openFolder(r.item.f.ID)
	case r.e == nil:
	case r.m.a.selectMode:
		r.m.toggleSelected(r.e)
	default:
		r.m.copyCode(r.e)
	}
}

func (r *entryRow) TappedSecondary(ev *fyne.PointEvent) {
	if r.item.f != nil || r.e != nil {
		r.m.a.touch()
		widget.ShowPopUpMenuAtPosition(r.menu(), r.m.a.win.Canvas(), ev.AbsolutePosition)
	}
}

func (r *entryRow) MouseIn(*desktop.MouseEvent) {
	r.m.a.touch()
	if r != r.m.dropRow {
		r.animateBG(color.Transparent, theme.Color(theme.ColorNameHover), 150*time.Millisecond)
	}
}

func (r *entryRow) MouseMoved(*desktop.MouseEvent) { r.m.a.touch() }

func (r *entryRow) MouseOut() {
	if r != r.m.dropRow {
		r.animateBG(r.bg.FillColor, color.Transparent, 200*time.Millisecond)
	}
}

func (r *entryRow) Cursor() desktop.Cursor { return desktop.PointerCursor }

func withAlpha(c color.Color, a uint8) color.Color {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	n.A = a
	return n
}

// vCenterFillLayout stretches its child horizontally and centers it vertically.
type vCenterFillLayout struct{}

func (vCenterFillLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, objs[0].MinSize().Height)
}

func (vCenterFillLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	h := objs[0].MinSize().Height
	objs[0].Resize(fyne.NewSize(size.Width, h))
	objs[0].Move(fyne.NewPos(0, (size.Height-h)/2))
}
