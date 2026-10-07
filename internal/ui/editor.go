package ui

import (
	"errors"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
	"github.com/keklick1337/Multi2FA-TOTP/internal/otp"
	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

// showEditor edits an existing entry, or creates a new one (optionally prefilled from a scanned key).
func (m *vaultView) showEditor(existing *vault.Entry, prefill *otp.Key) {
	T := i18n.T
	a := m.a
	a.touch()
	isNew := existing == nil

	issuer := widget.NewEntry()
	account := widget.NewEntry()
	secret := widget.NewPasswordEntry()
	typ := newSelect([]string{"TOTP", "HOTP"}, nil)
	alg := newSelect([]string{"SHA1", "SHA256", "SHA512"}, nil)
	digits := newSelect([]string{"6", "7", "8"}, nil)
	period := widget.NewEntry()
	counter := widget.NewEntry()
	tags := widget.NewEntry()
	tags.SetPlaceHolder(T("editor.tags.hint"))
	notes := widget.NewMultiLineEntry()
	notes.Wrapping = fyne.TextWrapWord
	notes.SetMinRowsVisible(3)
	favorite := newCheck(T("editor.favorite"), nil)

	fill := func(k otp.Key) {
		issuer.SetText(k.Issuer)
		account.SetText(k.Account)
		typ.SetSelected(strings.ToUpper(string(orDefault(k.Type, otp.TOTP))))
		alg.SetSelected(string(orDefault(k.Algorithm, otp.SHA1)))
		digits.SetSelected(strconv.Itoa(orDefault(k.Digits, otp.DefaultDigits)))
		period.SetText(strconv.Itoa(orDefault(k.Period, otp.DefaultPeriod)))
		counter.SetText(strconv.FormatUint(k.Counter, 10))
	}

	switch {
	case existing != nil:
		fill(existing.Key)
		secret.SetPlaceHolder(T("editor.secret.keep"))
		tags.SetText(strings.Join(existing.Tags, ", "))
		notes.SetText(existing.Notes)
		favorite.SetChecked(existing.Favorite)
	case prefill != nil:
		fill(*prefill)
		secret.SetText(prefill.Secret)
	default:
		fill(otp.Key{})
		secret.SetPlaceHolder(T("editor.secret.hint"))
	}

	for _, e := range []*widget.Entry{issuer, account, tags, notes, period, counter} {
		e.OnChanged = func(string) { a.touch() }
	}

	// Pasting a whole otpauth:// link into the secret field fills every field.
	secret.OnChanged = func(s string) {
		a.touch()
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), "otpauth://") {
			return
		}
		if k, err := otp.ParseURI(s); err == nil {
			fill(*k)
			secret.SetText(k.Secret)
		}
	}
	syncType := func(s string) {
		if s == "HOTP" {
			period.Disable()
			counter.Enable()
		} else {
			period.Enable()
			counter.Disable()
		}
	}
	typ.OnChanged = syncType
	syncType(typ.Selected)

	advanced := widget.NewForm(
		widget.NewFormItem(T("editor.type"), typ),
		widget.NewFormItem(T("editor.algorithm"), alg),
		widget.NewFormItem(T("editor.digits"), digits),
		widget.NewFormItem(T("editor.period"), period),
		widget.NewFormItem(T("editor.counter"), counter),
	)
	acc := widget.NewAccordion(widget.NewAccordionItem(T("editor.advanced"), advanced))
	if prefill != nil && (prefill.Type == otp.HOTP || prefill.Digits != 6 || prefill.Period != 30 || prefill.Algorithm != otp.SHA1) {
		acc.Open(0)
	}

	form := widget.NewForm(
		widget.NewFormItem(T("editor.issuer"), issuer),
		widget.NewFormItem(T("editor.account"), account),
		widget.NewFormItem(T("editor.secret"), secret),
		widget.NewFormItem(T("editor.tags"), tags),
		widget.NewFormItem(T("editor.notes"), notes),
		widget.NewFormItem("", favorite),
	)
	content := container.NewVScroll(container.NewVBox(form, acc))
	content.SetMinSize(fyne.NewSize(460, 420))

	title := T("editor.edit")
	if isNew {
		title = T("editor.new")
	}
	title += ": " + m.s.name()

	var d *dialog.CustomDialog
	save := func() {
		e := &vault.Entry{Folder: m.folder}
		if existing != nil {
			*e = *existing
		}
		e.Issuer = issuer.Text
		e.Account = account.Text
		if s := strings.TrimSpace(secret.Text); s != "" || isNew {
			e.Secret = s
		}
		e.Type = otp.Type(strings.ToLower(typ.Selected))
		e.Algorithm = otp.Algorithm(alg.Selected)
		e.Digits, _ = strconv.Atoi(digits.Selected)
		p, err := strconv.Atoi(strings.TrimSpace(period.Text))
		if err != nil && e.Type == otp.TOTP {
			a.showError(otp.ErrBadPeriod)
			return
		}
		e.Period = p
		c, err := strconv.ParseUint(strings.TrimSpace(counter.Text), 10, 64)
		if err != nil && e.Type == otp.HOTP {
			a.showError(errors.New(T("editor.counter.invalid")))
			return
		}
		e.Counter = c
		e.Tags = strings.Split(tags.Text, ",")
		e.Notes = strings.TrimSpace(notes.Text)
		e.Favorite = favorite.Checked
		if strings.TrimSpace(e.Issuer) == "" && strings.TrimSpace(e.Account) == "" {
			a.showError(errors.New(T("editor.name.required")))
			return
		}

		if isNew {
			err = m.s.v.Add(e)
		} else {
			err = m.s.v.Update(e)
		}
		if err != nil {
			if errors.Is(err, vault.ErrDuplicate) {
				err = errors.New(T("editor.duplicate"))
			}
			a.showError(err)
			return
		}
		secret.SetText("")
		d.Hide()
		m.reload()
		if isNew {
			a.notify(T("added", e.Label()), m.s.name())
		} else {
			a.notify(T("saved", e.Label()), m.s.name())
		}
	}

	saveBtn := newButton(T("save"), save)
	saveBtn.Importance = widget.HighImportance
	cancelBtn := newButton(T("cancel"), func() { secret.SetText(""); d.Hide() })
	d = dialog.NewCustomWithoutButtons(title, content, a.win)
	d.SetButtons([]fyne.CanvasObject{cancelBtn, saveBtn})
	d.Show()
	if isNew && prefill == nil {
		a.win.Canvas().Focus(issuer)
	}
}

func orDefault[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}
	return v
}
