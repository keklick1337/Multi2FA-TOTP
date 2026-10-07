package ui

import (
	"errors"
	"image/color"
	"math"
	"slices"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/keklick1337/Multi2FA-TOTP/internal/assets"
	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

const minPasswordLen = 8

const resetConfirmWord = "DELETE"

func (a *App) showLockScreen() {
	var content fyne.CanvasObject
	var focus fyne.Focusable
	if vault.Exists(a.lockPath) {
		content, focus = a.unlockScreen()
	} else {
		content, focus = a.createScreen()
	}
	wipeSecureEntries()
	a.win.SetContent(content)
	a.win.Canvas().Focus(focus)
}

// lockCard is the centered column of the lock screens; it can shake on errors.
type lockCard struct {
	box    *fyne.Container
	lay    *cardLayout
	scroll *container.Scroll
	outer  fyne.CanvasObject
}

// refresh recomputes the scroll area after the card changed size.
func (c *lockCard) refresh() {
	c.box.Refresh()
	c.scroll.Refresh()
}

func (a *App) lockFrame(subtitle string, body fyne.CanvasObject) *lockCard {
	T := i18n.T
	logo := canvas.NewImageFromResource(assets.Icon)
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(104, 104))

	title := widget.NewLabelWithStyle(AppName, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	title.SizeName = theme.SizeNameHeadingText
	tagline := widget.NewLabel(T("app.tagline"))
	tagline.Alignment = fyne.TextAlignCenter
	tagline.SizeName = theme.SizeNameCaptionText
	tagline.Wrapping = fyne.TextWrapWord
	sub := widget.NewLabel(subtitle)
	sub.Alignment = fyne.TextAlignCenter
	sub.Wrapping = fyne.TextWrapWord

	c := &lockCard{lay: &cardLayout{width: 380}}
	c.box = container.New(c.lay, container.NewVBox(logo, title, tagline, sub, a.vaultPicker(), body))
	footer := container.NewPadded(a.languagePicker())
	minSize := canvas.NewRectangle(color.Transparent)
	minSize.SetMinSize(fyne.NewSize(minWindowWidth, minWindowHeight))
	c.scroll = container.NewVScroll(container.NewPadded(c.box))
	c.outer = container.NewStack(minSize, container.NewBorder(nil, footer, nil, nil, c.scroll))
	return c
}

func (c *lockCard) shake() {
	anim := fyne.NewAnimation(420*time.Millisecond, func(f float32) {
		c.lay.dx = float32(math.Sin(float64(f)*math.Pi*6)) * 10 * (1 - f)
		c.box.Refresh()
	})
	anim.Start()
}

// vaultPicker selects which vault file the lock screen unlocks or creates.
func (a *App) vaultPicker() fyne.CanvasObject {
	paths := a.recentVaults()
	names := make([]string, 0, len(paths))
	for _, p := range paths {
		n := vaultName(p)
		for i := 2; slices.Contains(names, n); i++ {
			n = vaultName(p) + " (" + strconv.Itoa(i) + ")"
		}
		names = append(names, n)
	}
	sel := newSelect(names, nil)
	for i, p := range paths {
		if samePath(p, a.lockPath) {
			sel.SetSelectedIndex(i)
		}
	}
	if sel.SelectedIndex() < 0 {
		sel.Options = append(sel.Options, vaultName(a.lockPath))
		paths = append(paths, a.lockPath)
		sel.SetSelectedIndex(len(paths) - 1)
	}
	sel.OnChanged = func(string) {
		if i := sel.SelectedIndex(); i >= 0 && i < len(paths) && !samePath(paths[i], a.lockPath) {
			a.lockPath = paths[i]
			a.showLockScreen()
		}
	}
	T := i18n.T
	filter := &fileFilter{name: T("filter.vaults"), exts: []string{vaultExt, backupExt}}
	openBtn := newIconButton("", theme.FolderOpenIcon(), func() {
		a.openFile(T("vault.open"), filter, func(path string) {
			a.lockPath = path
			a.rememberVault(path)
			a.showLockScreen()
		})
	})
	newBtn := newIconButton("", theme.ContentAddIcon(), func() {
		a.saveFile(T("vault.new"), "vault"+vaultExt, &fileFilter{name: T("filter.vaults"), exts: []string{vaultExt}}, func(path string) {
			a.lockPath = path
			a.showLockScreen()
		})
	})
	return container.NewBorder(nil, nil, container.NewHBox(widget.NewIcon(theme.StorageIcon())), container.NewHBox(openBtn, newBtn), sel)
}

func (a *App) languagePicker() fyne.CanvasObject {
	sel := newSelect(i18n.Names(), nil)
	sel.SetSelected(i18n.NameOf(i18n.Current()))
	sel.OnChanged = func(name string) {
		code := i18n.CodeOf(name)
		if code == i18n.Current() {
			return
		}
		a.setLanguage(code)
		a.rebuild()
	}
	return container.NewHBox(layout.NewSpacer(), widget.NewIcon(theme.SettingsIcon()), sel)
}

func (a *App) unlockScreen() (fyne.CanvasObject, fyne.Focusable) {
	T := i18n.T
	path := a.lockPath
	info, infoErr := vault.Inspect(path)
	form := a.newCredForm(path, info.KeyFile, info.KeyFile)
	pw := form.pw
	errLabel := widget.NewLabel("")
	errLabel.Importance = widget.DangerImportance
	errLabel.Alignment = fyne.TextAlignCenter
	errLabel.Wrapping = fyne.TextWrapWord
	errLabel.Hide()
	progress := widget.NewProgressBarInfinite()
	progress.Hide()
	progress.Stop()
	meta := widget.NewLabel("")
	meta.Alignment = fyne.TextAlignCenter
	meta.SizeName = theme.SizeNameCaptionText
	if infoErr == nil {
		meta.SetText(securitySummary(vault.Security{KDF: info.KDF}, info.KeyFile))
	}

	var frame *lockCard
	var btn *button
	fail := func(msg string) {
		errLabel.SetText(msg)
		errLabel.Show()
		frame.shake()
		a.win.Canvas().Focus(pw)
	}
	submit := func() {
		if btn.Disabled() || pw.Len() == 0 {
			return
		}
		if wait := time.Until(a.retryAfter); wait > 0 {
			fail(T("unlock.wait", int(wait.Seconds())+1))
			return
		}
		creds, cleanup, err := form.credentials()
		pw.Clear()
		if err != nil {
			fail(err.Error())
			return
		}
		btn.Disable()
		pw.Disable()
		progress.Show()
		progress.Start()
		errLabel.Hide()
		go func() {
			v, err := vault.Open(path, creds)
			cleanup()
			fyne.Do(func() {
				progress.Stop()
				progress.Hide()
				btn.Enable()
				pw.Enable()
				if err != nil {
					a.failCount++
					if a.failCount >= 3 {
						a.retryAfter = time.Now().Add(time.Duration(min(1<<(a.failCount-3), 60)) * time.Second)
					}
					if errors.Is(err, vault.ErrWrongPassword) {
						fail(T("unlock.wrong"))
					} else {
						fail(err.Error())
					}
					return
				}
				form.remember(path)
				a.unlocked(path, v)
			})
		}()
	}
	pw.OnSubmitted = submit
	btn = newIconButton(T("unlock.button"), theme.LoginIcon(), submit)
	btn.Importance = widget.HighImportance

	forgot := newButton(T("reset.link"), a.confirmReset)
	forgot.Importance = widget.LowImportance

	body := container.NewVBox(append(form.objects(), btn, progress, errLabel, meta, forgot)...)
	frame = a.lockFrame(T("unlock.subtitle"), body)
	return frame.outer, pw
}

func (a *App) confirmReset() {
	T := i18n.T
	confirm := widget.NewEntry()
	confirm.SetPlaceHolder(resetConfirmWord)
	msg := widget.NewLabel(T("reset.warning", vaultName(a.lockPath), resetConfirmWord))
	msg.Wrapping = fyne.TextWrapWord
	content := container.NewVBox(msg, confirm)
	d := dialog.NewCustomConfirm(T("reset.title"), T("reset.button"), T("cancel"), content, func(ok bool) {
		if !ok {
			return
		}
		if confirm.Text != resetConfirmWord {
			dialog.ShowInformation(T("reset.title"), T("reset.mismatch"), a.win)
			return
		}
		if err := vault.Destroy(a.lockPath); err != nil {
			a.showError(err)
			return
		}
		a.failCount = 0
		a.retryAfter = time.Time{}
		a.showLockScreen()
	}, a.win)
	d.SetConfirmImportance(widget.DangerImportance)
	d.Resize(fyne.NewSize(440, 0))
	d.Show()
	a.win.Canvas().Focus(confirm)
}

func (a *App) createScreen() (fyne.CanvasObject, fyne.Focusable) {
	T := i18n.T
	path := a.lockPath
	pw, pw2, strength := newPasswordFields()
	pw.placeholder = T("create.password")
	pw2.placeholder = T("create.confirm")
	sec := newSecurityForm(nil)
	keyRow, keyPath := a.newKeyFileField()

	errLabel := widget.NewLabel("")
	errLabel.Importance = widget.DangerImportance
	errLabel.Wrapping = fyne.TextWrapWord
	errLabel.Hide()
	progress := widget.NewProgressBarInfinite()
	progress.Hide()
	progress.Stop()
	hint := widget.NewLabel(T("create.hint", minPasswordLen))
	hint.Wrapping = fyne.TextWrapWord
	hint.SizeName = theme.SizeNameCaptionText

	var frame *lockCard
	var btn *button
	fail := func(msg string) {
		errLabel.SetText(msg)
		errLabel.Show()
		frame.shake()
	}
	submit := func() {
		if btn.Disabled() {
			return
		}
		if err := validateNewPassword(pw, pw2); err != nil {
			fail(err.Error())
			return
		}
		if err := sec.check(); err != nil {
			fail(err.Error())
			return
		}
		resolve := sec.resolver()
		creds, cleanup, err := makeCredentials(pw, keyPath(), false)
		pw.Clear()
		pw2.Clear()
		if err != nil {
			fail(err.Error())
			return
		}
		kf := keyPath()
		btn.Disable()
		progress.Show()
		progress.Start()
		errLabel.Hide()
		go func() {
			security, err := resolve()
			var v *vault.Vault
			if err == nil {
				v, err = vault.Create(path, creds, vault.Options{Security: security})
			}
			cleanup()
			fyne.Do(func() {
				progress.Stop()
				progress.Hide()
				btn.Enable()
				if err != nil {
					fail(err.Error())
					return
				}
				if kf != "" {
					a.fa.Preferences().SetString(prefKeyFile+absPath(path), kf)
				}
				a.unlocked(path, v)
			})
		}()
	}
	pw2.OnSubmitted = submit
	btn = newIconButton(T("create.button"), theme.ConfirmIcon(), submit)
	btn.Importance = widget.HighImportance

	advanced := collapsible(T("kdf.protection"), container.NewVBox(
		sec.obj,
		widget.NewSeparator(),
		widget.NewForm(widget.NewFormItem(T("keyfile.title"), keyRow)),
		labelWrap(T("keyfile.hint")),
	), func() { frame.refresh() })
	sec.onLayout = func() {
		if frame != nil {
			frame.refresh()
		}
	}

	frame = a.lockFrame(T("create.subtitle", vaultName(path)), container.NewVBox(pw, pw2, strength, hint, advanced, btn, progress, errLabel))
	return frame.outer, pw
}

// wrapLabel is a regular-size label that wraps instead of widening its container.
func wrapLabel(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	return l
}

func labelWrap(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	l.SizeName = theme.SizeNameCaptionText
	return l
}

// cardLayout centers a single child, caps its width and applies a horizontal shake offset.
type cardLayout struct {
	width float32
	dx    float32
}

func (l *cardLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	s := objs[0].MinSize()
	return fyne.NewSize(min(s.Width, 260), s.Height)
}

func (l *cardLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	w := min(l.width, size.Width)
	h := objs[0].MinSize().Height
	objs[0].Resize(fyne.NewSize(w, h))
	objs[0].Move(fyne.NewPos((size.Width-w)/2+l.dx, max(0, (size.Height-h)/2)))
}
