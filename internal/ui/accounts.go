package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
)

// crumb is one breadcrumb button; it is also a drop target for drag and drop.
type crumb struct {
	btn    *button
	folder string
}

type accountsPage struct {
	a         *App
	crumbBox  *fyne.Container
	crumbs    []crumb
	content   fyne.CanvasObject
	vaultBtn  *button
	count     *widget.Label
	selectBtn *button
	listHold  *fyne.Container
	actionBar fyne.CanvasObject
	selLabel  *widget.Label
	selAll    *checkBox
}

func newAccountsPage(a *App) *accountsPage {
	T := i18n.T
	p := &accountsPage{a: a}

	a.search = newSearchEntry()
	a.search.SetPlaceHolder(T("search.placeholder"))
	a.search.ActionItem = widget.NewIcon(theme.SearchIcon())
	a.search.OnChanged = func(string) {
		a.touch()
		for _, s := range a.sessions {
			s.view.applyFilter()
		}
	}
	a.search.onKey = a.touch
	a.search.onPaste = a.pasteFromClipboard
	a.search.onShortcut = a.handleShortcut
	a.search.onSubmit = func() {
		if v := a.cur().view; len(v.shown) > 0 && !a.selectMode {
			v.copyCode(v.shown[0])
		}
	}
	a.search.onBackspaceEmpty = func() { a.cur().view.goUp() }
	a.search.onEscape = func() {
		if a.selectMode {
			a.setSelectMode(false)
		} else if a.search.Text != "" {
			a.search.SetText("")
		}
	}

	p.vaultBtn = newIconButton("", theme.StorageIcon(), nil)
	p.vaultBtn.Alignment = widget.ButtonAlignLeading
	p.vaultBtn.Importance = widget.LowImportance
	p.vaultBtn.OnTapped = func() {
		a.touch()
		showMenuBelow(a.win, p.vaultBtn, a.vaultSwitchMenu())
	}
	p.count = widget.NewLabel("")
	p.count.SizeName = theme.SizeNameCaptionText
	p.selectBtn = newIconButton("", theme.CheckButtonCheckedIcon(), func() {
		a.touch()
		a.setSelectMode(!a.selectMode)
	})
	p.selectBtn.Importance = widget.LowImportance

	newFolderBtn := newIconButton(T("folder.new"), theme.FolderNewIcon(), func() {
		a.touch()
		v := a.cur().view
		v.newFolder(v.folder)
	})
	newFolderBtn.Importance = widget.LowImportance
	header := container.NewBorder(nil, nil, nil, container.NewHBox(p.count, p.selectBtn), p.vaultBtn)
	p.crumbBox = container.NewHBox()

	p.selLabel = widget.NewLabel("")
	p.selAll = newCheck(T("select.all"), func(b bool) { a.cur().view.selectAll(b) })
	actionsBtn := newIconButton(T("select.actions"), theme.MenuDropDownIcon(), nil)
	actionsBtn.Importance = widget.HighImportance
	actionsBtn.OnTapped = func() {
		a.touch()
		showMenuAbove(a.win, actionsBtn, a.selectionMenu())
	}
	doneBtn := newIconButton("", theme.CancelIcon(), func() { a.setSelectMode(false) })
	doneBtn.Importance = widget.LowImportance
	p.actionBar = container.NewPadded(container.NewBorder(nil, nil,
		container.NewHBox(p.selAll, p.selLabel), container.NewHBox(actionsBtn, doneBtn)))
	p.actionBar.Hide()

	p.listHold = container.NewStack()
	crumbScroll := container.NewBorder(nil, nil, nil, newFolderBtn, container.NewHScroll(p.crumbBox))
	top := container.NewPadded(container.NewVBox(header, a.search, crumbScroll))
	p.content = container.NewBorder(top, p.actionBar, nil, nil, p.listHold)
	return p
}

func (p *accountsPage) showVault(s *session) {
	p.vaultBtn.SetText(s.name())
	p.listHold.Objects = []fyne.CanvasObject{s.view.content}
	p.listHold.Refresh()
	s.view.applyFilter()
}

// updateCrumbs shows the path of the current folder: vault > folder > subfolder.
func (p *accountsPage) updateCrumbs() {
	v := p.a.cur().view
	p.crumbs = p.crumbs[:0]
	var objs []fyne.CanvasObject
	add := func(label, folder string, icon fyne.Resource) {
		btn := newIconButton(label, icon, func() { v.openFolder(folder) })
		btn.Importance = widget.LowImportance
		p.crumbs = append(p.crumbs, crumb{btn: btn, folder: folder})
		objs = append(objs, btn)
	}
	add(i18n.T("folder.root"), "", theme.HomeIcon())
	if q := strings.TrimSpace(p.a.search.Text); q != "" {
		objs = append(objs, widget.NewLabel("›  "+i18n.T("folder.search")))
	} else {
		id := v.folder
		var chain []string
		for id != "" {
			chain = append([]string{id}, chain...)
			f := v.s.v.Folder(id)
			if f == nil {
				break
			}
			id = f.Parent
		}
		for _, fid := range chain {
			objs = append(objs, widget.NewLabel("›"))
			add(v.folderName(fid), fid, theme.FolderIcon())
		}
	}
	p.crumbBox.Objects = objs
	p.crumbBox.Refresh()
}

func (p *accountsPage) updateCount(shown, total int, query string) {
	if strings.TrimSpace(query) != "" {
		p.count.SetText(i18n.T("count.filtered", shown, total))
	} else {
		p.count.SetText(i18n.N("count.total", total))
	}
}

func (p *accountsPage) syncSelectMode() {
	if p.a.selectMode {
		p.selectBtn.Importance = widget.HighImportance
		p.actionBar.Show()
	} else {
		p.selectBtn.Importance = widget.LowImportance
		p.actionBar.Hide()
	}
	p.selectBtn.Refresh()
	p.updateSelection()
}

func (p *accountsPage) updateSelection() {
	v := p.a.cur().view
	n := len(v.selectedEntries())
	p.selLabel.SetText(i18n.T("select.count", n))
	all := n > 0 && n == len(v.shown)
	if p.selAll.Checked != all {
		cb := p.selAll.OnChanged
		p.selAll.OnChanged = nil
		p.selAll.SetChecked(all)
		p.selAll.OnChanged = cb
	}
}

func (a *App) showAddMenu(anchor fyne.CanvasObject) {
	T := i18n.T
	var items []*fyne.MenuItem
	if !isMobile() {
		items = append(items,
			fyne.NewMenuItemWithIcon(T("add.camera"), theme.MediaVideoIcon(), a.scanCamera),
			fyne.NewMenuItemWithIcon(T("add.screen"), theme.ComputerIcon(), a.scanScreen))
	}
	m := fyne.NewMenu("", append(items,
		fyne.NewMenuItemWithIcon(T("add.image"), theme.FileImageIcon(), a.openImage),
		fyne.NewMenuItemWithIcon(T("add.paste"), theme.ContentPasteIcon(), a.pasteMenuAction),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItemWithIcon(T("add.manual"), theme.DocumentCreateIcon(), func() { a.cur().view.showEditor(nil, nil) }),
		fyne.NewMenuItemWithIcon(T("folder.new"), theme.FolderNewIcon(), func() { v := a.cur().view; v.newFolder(v.folder) }),
		fyne.NewMenuItemWithIcon(T("backup.import"), theme.DownloadIcon(), a.importBackup),
	)...)
	pop := widget.NewPopUpMenu(m, a.win.Canvas())
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	size := pop.MinSize()
	cs := a.win.Canvas().Size()
	var at fyne.Position
	if a.shell.navL.vertical {
		at = fyne.NewPos(pos.X+anchor.Size().Width, pos.Y)
	} else {
		at = fyne.NewPos(pos.X+anchor.Size().Width/2-size.Width/2, pos.Y-size.Height)
	}
	at.X = max(0, min(at.X, cs.Width-size.Width))
	at.Y = max(0, min(at.Y, cs.Height-size.Height))
	pop.ShowAtPosition(at)
}

func (a *App) vaultSwitchMenu() *fyne.Menu {
	T := i18n.T
	var items []*fyne.MenuItem
	for _, s := range a.sessions {
		it := fyne.NewMenuItemWithIcon(s.name(), theme.StorageIcon(), func() { a.switchVault(s) })
		it.Checked = s == a.cur()
		items = append(items, it)
	}
	items = append(items,
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItemWithIcon(T("vault.open"), theme.FolderOpenIcon(), a.openVaultFile),
		fyne.NewMenuItemWithIcon(T("vault.new"), theme.ContentAddIcon(), a.newVaultFile),
		fyne.NewMenuItemWithIcon(T("vault.manage"), theme.SettingsIcon(), func() { a.shell.show(pageVaults, true) }),
	)
	return fyne.NewMenu("", items...)
}

func showMenuBelow(w fyne.Window, anchor fyne.CanvasObject, m *fyne.Menu) {
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	widget.ShowPopUpMenuAtPosition(m, w.Canvas(), pos.Add(fyne.NewPos(0, anchor.Size().Height)))
}

func showMenuAbove(w fyne.Window, anchor fyne.CanvasObject, m *fyne.Menu) {
	pop := widget.NewPopUpMenu(m, w.Canvas())
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	size := pop.MinSize()
	x := min(pos.X, w.Canvas().Size().Width-size.Width)
	pop.ShowAtPosition(fyne.NewPos(max(x, 0), max(pos.Y-size.Height, 0)))
}

// pageHeader is the large title shown on the vaults and settings pages.
func pageHeader(title string) fyne.CanvasObject {
	l := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.SizeName = theme.SizeNameHeadingText
	return l
}

// card groups related controls on a subtle rounded background.
func card(title string, content ...fyne.CanvasObject) fyne.CanvasObject {
	return widget.NewCard("", "", container.NewVBox(append([]fyne.CanvasObject{sectionTitle(title)}, content...)...))
}

func sectionTitle(s string) fyne.CanvasObject {
	l := widget.NewLabelWithStyle(strings.ToUpper(s), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.SizeName = theme.SizeNameCaptionText
	return l
}
