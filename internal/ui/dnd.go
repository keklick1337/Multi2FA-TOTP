package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

// dragState describes what is being dragged: one folder or a set of entries.
type dragState struct {
	ids    []string
	folder *vault.Folder
	target *dropTarget
}

// dropTarget is a folder under the pointer: a folder row or a breadcrumb.
type dropTarget struct {
	folder string
	row    *entryRow
	crumb  *button
}

// dragRow is an entryRow that can be dragged with the mouse.
type dragRow struct {
	*entryRow
	drag *dragState
}

func (r *dragRow) Dragged(ev *fyne.DragEvent) {
	m := r.m
	if r.drag == nil {
		d := &dragState{}
		var label string
		switch {
		case r.item.f != nil:
			d.folder = r.item.f
			label = r.item.f.Name
		case r.e != nil:
			if m.a.selectMode && m.selected[r.e.ID] {
				for _, e := range m.selectedEntries() {
					d.ids = append(d.ids, e.ID)
				}
			} else {
				d.ids = []string{r.e.ID}
			}
			label = r.issuer.Text
			if len(d.ids) > 1 {
				label = i18n.N("count.total", len(d.ids))
			}
		default:
			return
		}
		r.drag = d
		m.a.touch()
		m.a.shell.showGhost(label, d.folder != nil)
	}
	m.a.shell.moveGhost(ev.AbsolutePosition)
	t := m.dropTargetAt(ev.AbsolutePosition, r.drag)
	m.highlightDrop(t)
	r.drag.target = t
}

func (r *dragRow) DragEnd() {
	m := r.m
	d := r.drag
	r.drag = nil
	m.a.shell.hideGhost()
	m.highlightDrop(nil)
	if d == nil || d.target == nil {
		return
	}
	if d.folder != nil {
		m.moveFolder(d.folder.ID, d.target.folder)
	} else {
		m.moveEntries(d.ids, d.target.folder)
	}
}

func contains(obj fyne.CanvasObject, p fyne.Position) bool {
	if obj == nil || !obj.Visible() {
		return false
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(obj)
	s := obj.Size()
	return p.X >= pos.X && p.Y >= pos.Y && p.X < pos.X+s.Width && p.Y < pos.Y+s.Height
}

func (m *vaultView) dropTargetAt(p fyne.Position, d *dragState) *dropTarget {
	if contains(m.list, p) {
		for row := range m.rows {
			f := row.item.f
			if f == nil || !contains(row.self, p) {
				continue
			}
			if d.folder != nil && m.s.v.Subtree(d.folder.ID)[f.ID] {
				return nil
			}
			return &dropTarget{folder: f.ID, row: row}
		}
	}
	for _, c := range m.a.accounts.crumbs {
		if contains(c.btn, p) {
			if d.folder != nil && (c.folder == d.folder.ID || m.s.v.Subtree(d.folder.ID)[c.folder]) {
				return nil
			}
			return &dropTarget{folder: c.folder, crumb: c.btn}
		}
	}
	return nil
}

func (m *vaultView) highlightDrop(t *dropTarget) {
	var row *entryRow
	if t != nil {
		row = t.row
	}
	if m.dropRow != row {
		if old := m.dropRow; old != nil {
			m.dropRow = nil
			old.setBG(withAlpha(theme.Color(theme.ColorNamePrimary), 0))
		}
		m.dropRow = row
		if row != nil {
			row.setBG(withAlpha(theme.Color(theme.ColorNamePrimary), 0x55))
		}
	}
	for _, c := range m.a.accounts.crumbs {
		imp := widget.LowImportance
		if t != nil && t.crumb == c.btn {
			imp = widget.HighImportance
		}
		if c.btn.Importance != imp {
			c.btn.Importance = imp
			c.btn.Refresh()
		}
	}
}

// Ghost shown under the pointer while dragging.

func (s *shell) showGhost(label string, folder bool) {
	icon := theme.NewColoredResource(theme.AccountIcon(), theme.ColorNameForegroundOnPrimary)
	if folder {
		icon = theme.NewColoredResource(theme.FolderIcon(), theme.ColorNameForegroundOnPrimary)
	}
	bg := canvas.NewRectangle(withAlpha(theme.Color(theme.ColorNamePrimary), 0xe6))
	bg.CornerRadius = 10
	text := canvas.NewText(label, theme.Color(theme.ColorNameForegroundOnPrimary))
	text.TextStyle.Bold = true
	card := container.NewStack(bg, container.NewPadded(container.NewHBox(widget.NewIcon(icon), container.NewCenter(text))))
	card.Resize(card.MinSize())
	s.drag.Objects = []fyne.CanvasObject{card}
	s.drag.Refresh()
}

func (s *shell) moveGhost(p fyne.Position) {
	if len(s.drag.Objects) == 0 {
		return
	}
	s.drag.Objects[0].Move(p.Add(fyne.NewPos(14, 10)))
}

func (s *shell) hideGhost() {
	s.drag.Objects = nil
	s.drag.Refresh()
}
