package ui

import (
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

func (a *App) vaultsPage() fyne.CanvasObject {
	T := i18n.T
	var open []fyne.CanvasObject
	for i, s := range a.sessions {
		name := widget.NewLabelWithStyle(s.name(), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		name.Truncation = fyne.TextTruncateEllipsis
		info := i18n.N("count.total", len(s.v.Entries()))
		if i == 0 {
			info = T("vault.main") + "  ·  " + info
		}
		if s == a.cur() {
			info += "  ·  " + T("vault.active")
		}
		meta := widget.NewLabel(info)
		meta.SizeName = theme.SizeNameCaptionText
		path := widget.NewLabel(s.path)
		path.SizeName = theme.SizeNameCaptionText
		path.Truncation = fyne.TextTruncateEllipsis

		showBtn := newIconButton(T("vault.show"), theme.NavigateNextIcon(), func() { a.switchVault(s) })
		menuBtn := newIconButton("", theme.MoreVerticalIcon(), nil)
		menuBtn.Importance = widget.LowImportance
		menuBtn.OnTapped = func() {
			a.touch()
			items := []*fyne.MenuItem{
				fyne.NewMenuItemWithIcon(T("vault.showfile"), theme.FolderOpenIcon(), func() { a.reveal(s.path) }),
				fyne.NewMenuItemWithIcon(T("password.change"), theme.AccountIcon(), func() { a.changeMasterKey(s) }),
				fyne.NewMenuItemWithIcon(T("backup.export"), theme.UploadIcon(), func() { a.exportBackup(s) }),
				fyne.NewMenuItemWithIcon(T("transfer.title"), theme.MailForwardIcon(), func() { a.transferAll(s) }),
			}
			if i > 0 {
				items = append(items, fyne.NewMenuItemSeparator(),
					fyne.NewMenuItemWithIcon(T("vault.close"), theme.CancelIcon(), func() { a.closeSession(s) }))
			}
			showMenuBelow(a.win, menuBtn, fyne.NewMenu("", items...))
		}
		row := container.NewBorder(nil, nil, container.NewCenter(widget.NewIcon(theme.StorageIcon())), container.NewCenter(container.NewHBox(showBtn, menuBtn)),
			container.NewVBox(name, meta, path))
		open = append(open, row)
		if i < len(a.sessions)-1 {
			open = append(open, widget.NewSeparator())
		}
	}

	var recent []fyne.CanvasObject
	for _, p := range a.recentVaults() {
		if a.sessionByPath(p) != nil {
			continue
		}
		name := widget.NewLabelWithStyle(vaultName(p), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		path := widget.NewLabel(p)
		path.SizeName = theme.SizeNameCaptionText
		path.Truncation = fyne.TextTruncateEllipsis
		forget := newIconButton("", theme.ContentClearIcon(), func() {
			a.forgetVault(p)
			a.shell.refreshPage()
		})
		forget.Importance = widget.LowImportance
		var actions fyne.CanvasObject
		if vault.Exists(p) {
			show := newIconButton("", theme.FolderOpenIcon(), func() { a.reveal(p) })
			show.Importance = widget.LowImportance
			actions = container.NewHBox(newIconButton(T("unlock.button"), theme.LoginIcon(), func() { a.unlockExtra(p) }), show, forget)
		} else {
			path.SetText(T("vault.missing") + "  ·  " + p)
			path.Importance = widget.DangerImportance
			actions = container.NewHBox(newIconButton(T("vault.locate"), theme.SearchIcon(), func() { a.locateVault(p) }), forget)
		}
		recent = append(recent, container.NewBorder(nil, nil, container.NewCenter(widget.NewIcon(theme.StorageIcon())),
			container.NewCenter(actions), container.NewVBox(name, path)))
	}

	actions := container.NewGridWrap(fyne.NewSize(240, 40),
		newIconButton(T("vault.open"), theme.FolderOpenIcon(), a.openVaultFile),
		newIconButton(T("vault.new"), theme.ContentAddIcon(), a.newVaultFile),
		newIconButton(T("vault.merge"), theme.ContentPasteIcon(), a.mergeVaults),
		newIconButton(T("backup.import"), theme.DownloadIcon(), a.importBackup),
	)
	hint := widget.NewLabel(T("vault.hint"))
	hint.Wrapping = fyne.TextWrapWord

	body := []fyne.CanvasObject{pageHeader(T("nav.vaults")), card(T("vault.opened"), open...)}
	if len(recent) > 0 {
		body = append(body, card(T("vault.recent"), recent...))
	}
	body = append(body, card(T("vault.actions"), actions, hint))
	return container.NewVScroll(container.NewPadded(container.NewVBox(body...)))
}

// unlockExtra asks for the credentials of another vault and opens it next to the current ones.
func (a *App) unlockExtra(path string) {
	T := i18n.T
	if s := a.sessionByPath(path); s != nil {
		a.switchVault(s)
		return
	}
	info, err := vault.Inspect(path)
	if err != nil {
		a.showError(err)
		return
	}
	form := a.newCredForm(path, info.KeyFile, info.KeyFile)
	msg := labelWrap(T("vault.password.for", vaultName(path)))
	errLabel := widget.NewLabel("")
	errLabel.Importance = widget.DangerImportance
	progress := widget.NewProgressBarInfinite()
	progress.Hide()
	progress.Stop()

	var d *dialog.CustomDialog
	var okBtn *button
	submit := func() {
		if form.pw.Len() == 0 || okBtn.Disabled() {
			return
		}
		creds, cleanup, err := form.credentials()
		form.pw.Clear()
		if err != nil {
			errLabel.SetText(err.Error())
			return
		}
		okBtn.Disable()
		progress.Show()
		progress.Start()
		go func() {
			v, err := vault.Open(path, creds)
			cleanup()
			fyne.Do(func() {
				progress.Stop()
				progress.Hide()
				okBtn.Enable()
				if !a.isUnlocked() {
					if v != nil {
						v.Close()
					}
					return
				}
				if err != nil {
					if errors.Is(err, vault.ErrWrongPassword) {
						err = errors.New(T("unlock.wrong"))
					}
					errLabel.SetText(err.Error())
					a.win.Canvas().Focus(form.pw)
					return
				}
				form.remember(path)
				d.Hide()
				a.openSession(path, v)
				a.notify(T("vault.opened.toast", vaultName(path)), securitySummary(v.Security(), v.KeyFileRequired()))
			})
		}()
	}
	form.pw.OnSubmitted = submit
	okBtn = newIconButton(T("unlock.button"), theme.LoginIcon(), submit)
	okBtn.Importance = widget.HighImportance
	cancel := newButton(T("cancel"), func() { form.pw.Clear(); d.Hide() })
	content := container.NewVBox(append([]fyne.CanvasObject{msg}, append(form.objects(), progress, errLabel)...)...)
	d = dialog.NewCustomWithoutButtons(T("vault.unlock"), content, a.win)
	d.SetButtons([]fyne.CanvasObject{cancel, okBtn})
	d.Resize(fyne.NewSize(420, 0))
	d.Show()
	a.win.Canvas().Focus(form.pw)
}

func (a *App) openVaultFile() {
	a.openFile(i18n.T("vault.open"), &fileFilter{name: i18n.T("filter.vaults"), exts: []string{vaultExt, backupExt}}, a.unlockExtra)
}

func (a *App) newVaultFile() {
	a.createVault(nil, nil, func(*session) {})
}

// createVault creates a new vault file, copies entries of src into it (with their folders) and opens it.
func (a *App) createVault(src *session, entries []*vault.Entry, done func(*session)) {
	T := i18n.T
	a.askNewVaultPassword(T("vault.new"), T("vault.new.info"), "vault-2"+vaultExt, vaultExt,
		func(path string, creds vault.Credentials, resolve func() (vault.Security, error), cleanup func()) {
			a.runBusy(func() (func(), error) {
				defer cleanup()
				sec, err := resolve()
				if err != nil {
					return nil, err
				}
				v, err := vault.Create(path, creds, vault.Options{Security: sec, Overwrite: true})
				if err != nil {
					return nil, err
				}
				added := 0
				if src != nil && len(entries) > 0 {
					if added, _, err = v.ImportFrom(src.v, entries, false); err != nil {
						v.Close()
						return nil, err
					}
				}
				return func() {
					s := a.openSession(path, v)
					a.notify(T("vault.created", vaultName(path)), i18n.N("count.total", added))
					done(s)
				}, nil
			})
		})
}

// runBusy runs slow work (key derivation) off the UI thread behind a progress dialog.
func (a *App) runBusy(work func() (func(), error)) {
	prog := dialog.NewCustomWithoutButtons(i18n.T("working"), widget.NewProgressBarInfinite(), a.win)
	prog.Show()
	go func() {
		then, err := work()
		releaseMemory()
		fyne.Do(func() {
			prog.Hide()
			if !a.isUnlocked() {
				return
			}
			if err != nil {
				a.showError(err)
				return
			}
			if then != nil {
				then()
			}
		})
	}()
}

func (a *App) mergeVaults() {
	T := i18n.T
	if len(a.sessions) < 2 {
		dialog.ShowInformation(T("vault.merge"), T("vault.merge.need2"), a.win)
		return
	}
	var names []string
	for _, s := range a.sessions {
		names = append(names, s.name())
	}
	from := newSelect(names, nil)
	into := newSelect(names, nil)
	from.SetSelectedIndex(min(1, len(names)-1))
	into.SetSelectedIndex(0)
	if cur := a.cur(); cur != a.primary() {
		from.SetSelected(cur.name())
	}
	info := widget.NewLabel(T("vault.merge.info"))
	info.Wrapping = fyne.TextWrapWord
	form := widget.NewForm(
		widget.NewFormItem(T("vault.merge.from"), from),
		widget.NewFormItem(T("vault.merge.into"), into),
	)
	d := dialog.NewCustomConfirm(T("vault.merge"), T("vault.merge.button"), T("cancel"), container.NewVBox(info, form), func(ok bool) {
		if !ok {
			return
		}
		fi, ii := from.SelectedIndex(), into.SelectedIndex()
		if fi < 0 || ii < 0 || fi == ii {
			a.showError(errors.New(T("vault.merge.same")))
			return
		}
		src, dst := a.sessions[fi], a.sessions[ii]
		added, skipped, err := dst.v.ImportFrom(src.v, src.v.Entries(), true)
		if err != nil {
			a.showError(err)
			return
		}
		dst.view.reload()
		a.shell.refreshPage()
		a.notify(T("vault.merged", src.name(), dst.name()), T("import.result", added, skipped))
	}, a.win)
	d.Resize(fyne.NewSize(440, 0))
	d.Show()
}

// selectionMenu lists bulk actions for the checked accounts.
func (a *App) selectionMenu() *fyne.Menu {
	T := i18n.T
	src := a.cur()
	targets := func(move bool) *fyne.Menu {
		var items []*fyne.MenuItem
		for _, s := range a.sessions {
			if s == src {
				continue
			}
			items = append(items, fyne.NewMenuItemWithIcon(s.name(), theme.StorageIcon(), func() { a.transferSelected(src, s, move) }))
		}
		if len(items) > 0 {
			items = append(items, fyne.NewMenuItemSeparator())
		}
		items = append(items, fyne.NewMenuItemWithIcon(T("vault.new"), theme.ContentAddIcon(), func() { a.exportSelected(src, move) }))
		return fyne.NewMenu("", items...)
	}
	copyItem := fyne.NewMenuItemWithIcon(T("select.copyto"), theme.ContentCopyIcon(), nil)
	copyItem.ChildMenu = targets(false)
	moveItem := fyne.NewMenuItemWithIcon(T("select.moveto"), theme.ContentCutIcon(), nil)
	moveItem.ChildMenu = targets(true)
	return fyne.NewMenu("",
		fyne.NewMenuItemWithIcon(T("folder.moveto"), theme.FolderIcon(), func() {
			sel := a.selectionOrWarn(src)
			if len(sel) == 0 {
				return
			}
			src.view.pickFolder(T("folder.moveto"), "", func(dst string) {
				var ids []string
				for _, e := range sel {
					ids = append(ids, e.ID)
				}
				src.view.moveEntries(ids, dst)
				a.setSelectMode(false)
			})
		}),
		copyItem,
		moveItem,
		fyne.NewMenuItemWithIcon(T("select.export"), theme.UploadIcon(), func() { a.exportSelected(src, false) }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItemWithIcon(T("select.delete"), theme.DeleteIcon(), func() { a.deleteSelected(src) }),
	)
}

func (a *App) selectionOrWarn(src *session) []*vault.Entry {
	sel := src.view.selectedEntries()
	if len(sel) == 0 {
		a.notify(i18n.T("select.none"), "")
	}
	return sel
}

func (a *App) transferSelected(src, dst *session, move bool) {
	T := i18n.T
	sel := a.selectionOrWarn(src)
	if len(sel) == 0 {
		return
	}
	added, skipped, err := dst.v.ImportFrom(src.v, sel, false)
	if err != nil {
		a.showError(err)
		return
	}
	if move {
		var ids []string
		for _, e := range sel {
			if dst.v.Contains(e) {
				ids = append(ids, e.ID)
			}
		}
		if err := src.v.DeleteIDs(ids); err != nil {
			a.showError(err)
			return
		}
	}
	dst.view.reload()
	a.setSelectMode(false)
	src.view.reload()
	title := T("select.copied", dst.name())
	if move {
		title = T("select.moved", dst.name())
	}
	a.notify(title, T("import.result", added, skipped))
}

func (a *App) exportSelected(src *session, move bool) {
	sel := a.selectionOrWarn(src)
	if len(sel) == 0 {
		return
	}
	a.requirePassword(src, i18n.T("auth.export"), func() {
		a.createVault(src, sel, func(*session) {
			if move {
				var ids []string
				for _, e := range sel {
					ids = append(ids, e.ID)
				}
				if err := src.v.DeleteIDs(ids); err != nil {
					a.showError(err)
				}
				src.view.reload()
			}
			a.setSelectMode(false)
		})
	})
}

func (a *App) deleteSelected(src *session) {
	T := i18n.T
	sel := a.selectionOrWarn(src)
	if len(sel) == 0 {
		return
	}
	d := dialog.NewConfirm(T("select.delete"), i18n.N("select.delete.confirm", len(sel), src.name()), func(ok bool) {
		if !ok {
			return
		}
		var ids []string
		for _, e := range sel {
			ids = append(ids, e.ID)
		}
		if err := src.v.DeleteIDs(ids); err != nil {
			a.showError(err)
			return
		}
		a.setSelectMode(false)
		src.view.reload()
		a.notify(T("select.deleted", len(ids)), "")
	}, a.win)
	d.SetConfirmImportance(widget.DangerImportance)
	d.Show()
}

// reveal opens the system file manager at a vault file.
func (a *App) reveal(path string) {
	if err := showInFileManager(path); err != nil {
		a.showError(err)
	}
}

// locateVault replaces a recent vault that was moved or renamed with its new location.
func (a *App) locateVault(old string) {
	T := i18n.T
	a.openFile(T("vault.locate")+": "+vaultName(old), &fileFilter{name: T("filter.vaults"), exts: []string{vaultExt, backupExt}}, func(path string) {
		if _, err := vault.Inspect(path); err != nil {
			a.showError(err)
			return
		}
		a.forgetVault(old)
		if a.fa.Preferences().StringWithFallback(prefLast, "") == old {
			a.fa.Preferences().SetString(prefLast, path)
		}
		if kf := a.fa.Preferences().String(prefKeyFile + absPath(old)); kf != "" {
			a.fa.Preferences().SetString(prefKeyFile+absPath(path), kf)
		}
		a.rememberVault(path)
		if a.isUnlocked() {
			a.shell.refreshPage()
			a.unlockExtra(path)
		} else {
			a.lockPath = path
			a.showLockScreen()
		}
	})
}
