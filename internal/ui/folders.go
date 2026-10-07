package ui

import (
	"errors"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

const treeRoot = "/"

func (m *vaultView) folderName(id string) string {
	if id == "" {
		return m.s.name()
	}
	if f := m.s.v.Folder(id); f != nil {
		return f.Name
	}
	return m.s.name()
}

func (m *vaultView) folderMenu(f *vault.Folder) *fyne.Menu {
	T := i18n.T
	return fyne.NewMenu("",
		fyne.NewMenuItemWithIcon(T("folder.open"), theme.FolderOpenIcon(), func() { m.openFolder(f.ID) }),
		fyne.NewMenuItemWithIcon(T("folder.newsub"), theme.FolderNewIcon(), func() { m.newFolder(f.ID) }),
		fyne.NewMenuItemWithIcon(T("folder.rename"), theme.DocumentCreateIcon(), func() { m.renameFolder(f) }),
		fyne.NewMenuItemWithIcon(T("folder.moveto"), theme.FolderIcon(), func() {
			m.pickFolder(T("folder.moveto"), f.ID, func(dst string) { m.moveFolder(f.ID, dst) })
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItemWithIcon(T("folder.delete"), theme.DeleteIcon(), func() { m.deleteFolder(f) }),
	)
}

func (m *vaultView) askName(title, initial string, done func(string)) {
	T := i18n.T
	name := widget.NewEntry()
	name.SetText(initial)
	name.SetPlaceHolder(T("folder.name"))
	var d *dialog.ConfirmDialog
	name.OnSubmitted = func(string) { d.Confirm() }
	d = dialog.NewCustomConfirm(title, T("save"), T("cancel"), name, func(ok bool) {
		if ok {
			done(name.Text)
		}
	}, m.a.win)
	d.Resize(fyne.NewSize(380, 0))
	d.Show()
	m.a.win.Canvas().Focus(name)
}

func (m *vaultView) newFolder(parent string) {
	T := i18n.T
	m.askName(T("folder.new"), "", func(name string) {
		f, err := m.s.v.AddFolder(name, parent)
		if err != nil {
			m.a.showError(err)
			return
		}
		m.reload()
		m.a.notify(T("folder.created", f.Name), m.folderName(parent))
	})
}

func (m *vaultView) renameFolder(f *vault.Folder) {
	m.askName(i18n.T("folder.rename"), f.Name, func(name string) {
		if err := m.s.v.RenameFolder(f.ID, name); err != nil {
			m.a.showError(err)
			return
		}
		m.reload()
	})
}

func (m *vaultView) deleteFolder(f *vault.Folder) {
	T := i18n.T
	d := dialog.NewConfirm(T("folder.delete"), T("folder.delete.confirm", f.Name, m.folderName(f.Parent)), func(ok bool) {
		if !ok {
			return
		}
		if err := m.s.v.DeleteFolder(f.ID); err != nil {
			m.a.showError(err)
			return
		}
		m.reload()
		m.a.notify(T("folder.deleted", f.Name), "")
	}, m.a.win)
	d.SetConfirmImportance(widget.DangerImportance)
	d.Show()
}

func (m *vaultView) moveEntries(ids []string, folder string) {
	if len(ids) == 0 {
		return
	}
	if err := m.s.v.MoveEntries(ids, folder); err != nil {
		m.a.showError(err)
		return
	}
	m.reload()
	m.a.notify(i18n.T("folder.moved", m.folderName(folder)), i18n.N("count.total", len(ids)))
}

func (m *vaultView) moveFolder(id, parent string) {
	if id == parent {
		return
	}
	if err := m.s.v.MoveFolder(id, parent); err != nil {
		if errors.Is(err, vault.ErrFolderCycle) {
			m.a.notify(i18n.T("folder.cycle"), "")
			return
		}
		m.a.showError(err)
		return
	}
	m.reload()
	m.a.notify(i18n.T("folder.moved", m.folderName(parent)), m.folderName(id))
}

// pickFolder shows the folder tree of the vault; exclude hides a folder and its subtree.
func (m *vaultView) pickFolder(title, exclude string, done func(string)) {
	T := i18n.T
	var hidden map[string]bool
	if exclude != "" {
		hidden = m.s.v.Subtree(exclude)
	}
	children := map[string][]string{}
	for _, f := range m.s.v.Folders() {
		if hidden[f.ID] {
			continue
		}
		p := f.Parent
		if p == "" {
			p = treeRoot
		}
		children[p] = append(children[p], f.ID)
	}
	selected := treeRoot
	tree := widget.NewTree(
		func(uid string) []string {
			if uid == "" {
				return []string{treeRoot}
			}
			return children[uid]
		},
		func(uid string) bool { return uid == "" || uid == treeRoot || len(children[uid]) > 0 },
		func(bool) fyne.CanvasObject {
			return container.NewHBox(widget.NewIcon(theme.FolderIcon()), widget.NewLabel(""))
		},
		func(uid string, _ bool, o fyne.CanvasObject) {
			box := o.(*fyne.Container)
			icon, label := box.Objects[0].(*widget.Icon), box.Objects[1].(*widget.Label)
			if uid == treeRoot {
				icon.SetResource(theme.StorageIcon())
				label.SetText(m.s.name())
				return
			}
			icon.SetResource(theme.FolderIcon())
			label.SetText(m.folderName(uid))
		},
	)
	tree.OnSelected = func(uid string) { selected = uid }
	tree.OpenAllBranches()
	tree.Select(treeRoot)
	scroll := container.NewVScroll(tree)
	scroll.SetMinSize(fyne.NewSize(360, 300))
	newBtn := newIconButton(T("folder.new"), theme.FolderNewIcon(), nil)
	var d *dialog.ConfirmDialog
	newBtn.OnTapped = func() {
		parent := selected
		if parent == treeRoot {
			parent = ""
		}
		m.askName(T("folder.new"), "", func(name string) {
			f, err := m.s.v.AddFolder(name, parent)
			if err != nil {
				m.a.showError(err)
				return
			}
			p := f.Parent
			if p == "" {
				p = treeRoot
			}
			children[p] = append(children[p], f.ID)
			slices.Sort(children[p])
			tree.Refresh()
			tree.OpenAllBranches()
			tree.Select(f.ID)
			m.reload()
		})
	}
	d = dialog.NewCustomConfirm(title, T("folder.moveto.button"), T("cancel"), container.NewBorder(nil, newBtn, nil, nil, scroll), func(ok bool) {
		if !ok {
			return
		}
		if selected == treeRoot {
			selected = ""
		}
		done(selected)
	}, m.a.win)
	d.Show()
}
