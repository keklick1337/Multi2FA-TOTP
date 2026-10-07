package ui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
	"github.com/keklick1337/Multi2FA-TOTP/internal/otp"
	"github.com/keklick1337/Multi2FA-TOTP/internal/qr"
	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

// requirePassword asks for the credentials of a vault again before sensitive actions.
func (a *App) requirePassword(s *session, reason string, then func()) {
	T := i18n.T
	needKey := s.v.KeyFileRequired()
	form := a.newCredForm(s.path, needKey, needKey)
	msg := widget.NewLabel(reason)
	msg.Wrapping = fyne.TextWrapWord
	errLabel := widget.NewLabel("")
	errLabel.Importance = widget.DangerImportance
	progress := widget.NewProgressBarInfinite()
	progress.Hide()
	progress.Stop()

	var d *dialog.CustomDialog
	var okBtn *button
	submit := func() {
		if !a.isUnlocked() || form.pw.Len() == 0 || okBtn.Disabled() {
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
			ok := s.v.VerifyCredentials(creds)
			cleanup()
			fyne.Do(func() {
				progress.Stop()
				progress.Hide()
				okBtn.Enable()
				if !a.isUnlocked() {
					return
				}
				if !ok {
					errLabel.SetText(T("unlock.wrong"))
					a.win.Canvas().Focus(form.pw)
					return
				}
				d.Hide()
				then()
			})
		}()
	}
	form.pw.OnSubmitted = submit
	okBtn = newIconButton(T("confirm"), theme.ConfirmIcon(), submit)
	okBtn.Importance = widget.HighImportance
	cancel := newButton(T("cancel"), func() { form.pw.Clear(); d.Hide() })

	content := container.NewVBox(append([]fyne.CanvasObject{msg}, append(form.objects(), progress, errLabel)...)...)
	d = dialog.NewCustomWithoutButtons(T("auth.title")+": "+s.name(), content, a.win)
	d.SetButtons([]fyne.CanvasObject{cancel, okBtn})
	d.Resize(fyne.NewSize(420, 0))
	d.Show()
	a.win.Canvas().Focus(form.pw)
}

func qrImage(text string, size int) (*canvas.Image, error) {
	img, err := qr.Encode(text, 512)
	if err != nil {
		return nil, err
	}
	c := canvas.NewImageFromImage(img)
	c.FillMode = canvas.ImageFillContain
	c.ScaleMode = canvas.ImageScalePixels
	c.SetMinSize(fyne.NewSize(float32(size), float32(size)))
	return c, nil
}

func readOnlyText(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Selectable = true
	l.Wrapping = fyne.TextWrapBreak
	l.TextStyle.Monospace = true
	return l
}

// revealEntry shows secret, link and QR of one account so it can be moved to another device.
func (a *App) revealEntry(s *session, e *vault.Entry) {
	T := i18n.T
	a.requirePassword(s, T("auth.reveal"), func() {
		k, err := e.Reveal()
		if err != nil {
			a.showError(err)
			return
		}
		uri := k.URI()
		img, err := qrImage(uri, 260)
		if err != nil {
			a.showError(err)
			return
		}
		copySecret := newIconButton("", theme.ContentCopyIcon(), func() {
			a.copySecret(k.Secret)
			a.notify(T("copied.secret"), a.clipboardNote())
		})
		copyLink := newIconButton("", theme.ContentCopyIcon(), func() {
			a.copySecret(uri)
			a.notify(T("copied.link"), a.clipboardNote())
		})

		alg := string(orDefault(e.Algorithm, otp.SHA1))
		details := fmt.Sprintf("%s · %s · %d %s", strings.ToUpper(string(orDefault(e.Type, otp.TOTP))), alg, orDefault(e.Digits, 6), T("reveal.digits"))
		if e.Type != otp.HOTP {
			details += fmt.Sprintf(" · %d %s", orDefault(e.Period, otp.DefaultPeriod), T("reveal.seconds"))
		}
		info := widget.NewLabel(details)
		info.Alignment = fyne.TextAlignCenter
		warn := widget.NewLabel(T("reveal.warning"))
		warn.Wrapping = fyne.TextWrapWord
		warn.Importance = widget.WarningImportance

		form := widget.NewForm(
			widget.NewFormItem(T("editor.secret"), container.NewBorder(nil, nil, nil, copySecret, readOnlyText(k.Secret))),
			widget.NewFormItem(T("reveal.link"), container.NewBorder(nil, nil, nil, copyLink, readOnlyText(uri))),
		)
		content := container.NewVBox(container.NewCenter(img), info, form, warn)
		d := dialog.NewCustom(e.Label(), T("close"), container.NewVScroll(content), a.win)
		d.Resize(fyne.NewSize(520, 640))
		d.Show()
		a.autoClose(d, 2*time.Minute)
	})
}

// autoClose hides a dialog with secrets after a while even if the user is active.
func (a *App) autoClose(d dialog.Dialog, after time.Duration) {
	t := time.AfterFunc(after, func() { fyne.Do(d.Hide) })
	a.onLock(func() { t.Stop() })
}

// transferAll shows Google Authenticator compatible migration QR codes for every account of a vault.
func (a *App) transferAll(s *session) {
	T := i18n.T
	if len(s.v.Entries()) == 0 {
		dialog.ShowInformation(T("transfer.title"), T("transfer.empty"), a.win)
		return
	}
	a.requirePassword(s, T("auth.transfer"), func() {
		keys, err := vault.RevealKeys(s.v.Entries())
		if err != nil {
			a.showError(err)
			return
		}
		uris := otp.EncodeMigration(keys, 8)
		idx := 0
		holder := container.NewCenter()
		pageLabel := widget.NewLabel("")
		pageLabel.Alignment = fyne.TextAlignCenter
		var prev, next *button
		show := func() {
			img, err := qrImage(uris[idx], 320)
			if err != nil {
				a.showError(err)
				return
			}
			holder.Objects = []fyne.CanvasObject{img}
			holder.Refresh()
			pageLabel.SetText(T("transfer.page", idx+1, len(uris)))
			prev.Disable()
			next.Disable()
			if idx > 0 {
				prev.Enable()
			}
			if idx < len(uris)-1 {
				next.Enable()
			}
		}
		prev = newIconButton("", theme.NavigateBackIcon(), func() { a.touch(); idx--; show() })
		next = newIconButton("", theme.NavigateNextIcon(), func() { a.touch(); idx++; show() })
		show()
		hint := widget.NewLabel(T("transfer.hint"))
		hint.Wrapping = fyne.TextWrapWord
		content := container.NewVBox(hint, holder, container.NewBorder(nil, nil, prev, next, pageLabel))
		d := dialog.NewCustom(T("transfer.title")+": "+s.name(), T("close"), content, a.win)
		d.Resize(fyne.NewSize(480, 0))
		d.Show()
		a.autoClose(d, 5*time.Minute)
	})
}

// changeMasterKey re-keys a vault: new password, optional key file, cipher and key derivation.
func (a *App) changeMasterKey(s *session) {
	T := i18n.T
	needKey := s.v.KeyFileRequired()
	current := a.newCredForm(s.path, needKey, needKey)
	current.pw.placeholder = T("password.current")
	pw, pw2, strength := newPasswordFields()
	keyRow, keyPath := a.newKeyFileField()
	cur := s.v.Security()
	sec := newSecurityForm(&cur)
	info := labelWrap(T("kdf.current", securitySummary(cur, needKey)))

	form := widget.NewForm(
		widget.NewFormItem(T("password.current"), container.NewVBox(current.objects()...)),
		widget.NewFormItem(T("password.new"), pw),
		widget.NewFormItem(T("password.repeat"), pw2),
		widget.NewFormItem("", strength),
		widget.NewFormItem(T("keyfile.title"), keyRow),
	)
	content := container.NewVScroll(container.NewVBox(form, info, widget.NewSeparator(), sec.obj))
	content.SetMinSize(fyne.NewSize(520, 460))
	sec.onLayout = content.Refresh
	var d *dialog.ConfirmDialog
	pw2.OnSubmitted = func() { d.Confirm() }
	d = dialog.NewCustomConfirm(T("password.change")+": "+s.name(), T("save"), T("cancel"), content, func(ok bool) {
		defer func() { current.pw.Clear(); pw.Clear(); pw2.Clear() }()
		if !ok || !a.isUnlocked() {
			return
		}
		if err := validateNewPassword(pw, pw2); err != nil {
			a.showError(err)
			return
		}
		if err := sec.check(); err != nil {
			a.showError(err)
			return
		}
		resolve := sec.resolver()
		cur, curCleanup, err := current.credentials()
		if err != nil {
			a.showError(err)
			return
		}
		next, nextCleanup, err := makeCredentials(pw, keyPath(), false)
		if err != nil {
			curCleanup()
			a.showError(err)
			return
		}
		kf := keyPath()
		a.runBusy(func() (func(), error) {
			defer curCleanup()
			defer nextCleanup()
			if !s.v.VerifyCredentials(cur) {
				return nil, errors.New(T("password.current.wrong"))
			}
			newSec, err := resolve()
			if err != nil {
				return nil, err
			}
			if err := s.v.ChangeCredentials(next, newSec); err != nil {
				return nil, err
			}
			return func() {
				key := prefKeyFile + absPath(s.path)
				if kf == "" {
					a.fa.Preferences().RemoveValue(key)
				} else {
					a.fa.Preferences().SetString(key, kf)
				}
				a.notify(T("password.changed"), securitySummary(s.v.Security(), kf != ""))
				a.shell.refreshPage()
			}, nil
		})
	}, a.win)
	d.Resize(fyne.NewSize(600, 640))
	d.Show()
	a.win.Canvas().Focus(current.pw)
}

// askNewVaultPassword asks for the password and protection of a new vault or backup file,
// then for its location. done runs with credentials and a resolver for the security setup.
func (a *App) askNewVaultPassword(title, info, fileName, ext string, done func(path string, creds vault.Credentials, resolve func() (vault.Security, error), cleanup func())) {
	T := i18n.T
	pw, pw2, strength := newPasswordFields()
	sec := newSecurityForm(nil)
	form := widget.NewForm(
		widget.NewFormItem(T("password.new"), pw),
		widget.NewFormItem(T("password.repeat"), pw2),
		widget.NewFormItem("", strength),
	)
	var content *container.Scroll
	refresh := func() {
		if content != nil {
			content.Refresh()
		}
	}
	advanced := collapsible(T("kdf.protection"), sec.obj, refresh)
	sec.onLayout = refresh
	content = container.NewVScroll(container.NewVBox(labelWrap(info), form, advanced))
	content.SetMinSize(fyne.NewSize(500, 320))
	var d *dialog.ConfirmDialog
	pw2.OnSubmitted = func() { d.Confirm() }
	d = dialog.NewCustomConfirm(title, T("backup.choose"), T("cancel"), content, func(ok bool) {
		defer func() { pw.Clear(); pw2.Clear() }()
		if !ok || !a.isUnlocked() {
			return
		}
		if err := validateNewPassword(pw, pw2); err != nil {
			a.showError(err)
			return
		}
		if err := sec.check(); err != nil {
			a.showError(err)
			return
		}
		resolve := sec.resolver()
		creds, cleanup, err := makeCredentials(pw, "", false)
		if err != nil {
			a.showError(err)
			return
		}
		a.saveFile(title, fileName, &fileFilter{name: T("filter.vaults"), exts: []string{ext}}, func(path string) {
			if !a.isUnlocked() {
				cleanup()
				return
			}
			if a.sessionByPath(path) != nil {
				cleanup()
				a.showError(errors.New(T("vault.inuse")))
				return
			}
			done(path, creds, resolve, cleanup)
		})
	}, a.win)
	d.Resize(fyne.NewSize(560, 0))
	d.Show()
	a.win.Canvas().Focus(pw)
}

// exportBackup writes an encrypted copy of a vault (with folders) protected by its own password.
func (a *App) exportBackup(s *session) {
	T := i18n.T
	a.requirePassword(s, T("auth.backup"), func() {
		name := "multi2fa-" + s.name() + "-" + time.Now().Format("2006-01-02") + backupExt
		a.askNewVaultPassword(T("backup.export"), T("backup.password.info"), name, backupExt,
			func(path string, creds vault.Credentials, resolve func() (vault.Security, error), cleanup func()) {
				a.runBusy(func() (func(), error) {
					defer cleanup()
					sec, err := resolve()
					if err != nil {
						return nil, err
					}
					b, err := vault.Create(path, creds, vault.Options{Security: sec, Overwrite: true})
					if err != nil {
						return nil, err
					}
					defer b.Close()
					added, _, err := b.ImportFrom(s.v, s.v.Entries(), true)
					if err != nil {
						return nil, err
					}
					return func() {
						dialog.ShowInformation(T("backup.export"), i18n.N("backup.done", added, filepath.Base(path)), a.win)
					}, nil
				})
			})
	})
}

func (a *App) importBackup() {
	a.openFile(i18n.T("backup.import"), &fileFilter{name: i18n.T("filter.vaults"), exts: []string{backupExt, vaultExt}}, a.importBackupFile)
}

// importBackupFile merges a backup or another vault file (with its folders) into the current vault.
func (a *App) importBackupFile(path string) {
	T := i18n.T
	target := a.cur()
	info, _ := vault.Inspect(path)
	form := a.newCredForm(path, info.KeyFile, info.KeyFile)
	msg := labelWrap(T("backup.import.password", filepath.Base(path), target.name()))
	content := container.NewVBox(append([]fyne.CanvasObject{msg}, form.objects()...)...)
	var d *dialog.ConfirmDialog
	form.pw.OnSubmitted = func() { d.Confirm() }
	d = dialog.NewCustomConfirm(T("backup.import"), T("import.button"), T("cancel"), content, func(ok bool) {
		defer form.pw.Clear()
		if !ok || !a.isUnlocked() {
			return
		}
		creds, cleanup, err := form.credentials()
		if err != nil {
			a.showError(err)
			return
		}
		a.runBusy(func() (func(), error) {
			src, err := vault.Open(path, creds)
			cleanup()
			if err != nil {
				if errors.Is(err, vault.ErrWrongPassword) {
					err = errors.New(T("unlock.wrong"))
				}
				return nil, err
			}
			defer src.Close()
			added, skipped, err := target.v.ImportFrom(src, src.Entries(), true)
			if err != nil {
				return nil, err
			}
			return func() {
				target.view.reload()
				a.notify(T("backup.imported", target.name()), T("import.result", added, skipped))
			}, nil
		})
	}, a.win)
	d.Resize(fyne.NewSize(440, 0))
	d.Show()
	a.win.Canvas().Focus(form.pw)
}
