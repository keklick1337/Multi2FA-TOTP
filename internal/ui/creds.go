package ui

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/awnumar/memguard"

	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

const prefKeyFile = "keyfile:"

// credForm collects a password and, when needed, a key file.
type credForm struct {
	a        *App
	pw       *secureEntry
	keyPath  string
	keyLabel *widget.Label
	keyRow   fyne.CanvasObject
	required bool
}

// newCredForm builds the inputs for unlocking vaultPath. With showKeyFile the key file row is
// visible; the last key file used for this vault is preselected.
func (a *App) newCredForm(vaultPath string, showKeyFile, required bool) *credForm {
	T := i18n.T
	f := &credForm{a: a, pw: newSecureEntry(T("unlock.password")), required: required}
	if vaultPath != "" {
		f.keyPath = a.fa.Preferences().String(prefKeyFile + absPath(vaultPath))
	}
	f.keyLabel = widget.NewLabel("")
	f.keyLabel.Truncation = fyne.TextTruncateEllipsis
	choose := newIconButton(T("keyfile.choose"), theme.FolderOpenIcon(), func() {
		a.openFile(T("keyfile.title"), nil, func(path string) {
			f.keyPath = path
			f.updateLabel()
		})
	})
	clearBtn := newIconButton("", theme.ContentClearIcon(), func() { f.keyPath = ""; f.updateLabel() })
	clearBtn.Importance = widget.LowImportance
	f.keyRow = container.NewBorder(nil, nil, widget.NewIcon(theme.FileIcon()), container.NewHBox(choose, clearBtn), f.keyLabel)
	if !showKeyFile {
		f.keyRow.Hide()
	}
	f.updateLabel()
	return f
}

func (f *credForm) updateLabel() {
	if f.keyPath == "" {
		if f.required {
			f.keyLabel.SetText(i18n.T("keyfile.required"))
		} else {
			f.keyLabel.SetText(i18n.T("keyfile.none"))
		}
		return
	}
	f.keyLabel.SetText(filepath.Base(f.keyPath))
}

func (f *credForm) objects() []fyne.CanvasObject { return []fyne.CanvasObject{f.pw, f.keyRow} }

// credentials returns the inputs as vault credentials in locked memory and a cleanup func.
func (f *credForm) credentials() (vault.Credentials, func(), error) {
	return makeCredentials(f.pw, f.keyPath, f.required)
}

func makeCredentials(pw *secureEntry, keyPath string, required bool) (vault.Credentials, func(), error) {
	var bufs []*memguard.LockedBuffer
	cleanup := func() {
		for _, b := range bufs {
			b.Destroy()
		}
	}
	var c vault.Credentials
	if p := pw.Copy(); p != nil {
		bufs = append(bufs, p)
		c.Password = p.Bytes()
	}
	if keyPath == "" && required {
		return c, cleanup, errors.New(i18n.T("keyfile.required"))
	}
	if keyPath != "" {
		data, err := os.ReadFile(keyPath)
		if err != nil {
			cleanup()
			return c, func() {}, fmt.Errorf("%s: %w", i18n.T("keyfile.title"), err)
		}
		if len(data) == 0 {
			cleanup()
			return c, func() {}, errors.New(i18n.T("keyfile.empty"))
		}
		kb := memguard.NewBufferFromBytes(data)
		bufs = append(bufs, kb)
		c.KeyFile = kb.Bytes()
	}
	return c, cleanup, nil
}

func (f *credForm) remember(vaultPath string) {
	key := prefKeyFile + absPath(vaultPath)
	if f.keyPath == "" {
		f.a.fa.Preferences().RemoveValue(key)
	} else {
		f.a.fa.Preferences().SetString(key, f.keyPath)
	}
}

func absPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// newKeyFileField is the optional key file chooser for new or re-keyed vaults.
func (a *App) newKeyFileField() (fyne.CanvasObject, func() string) {
	T := i18n.T
	path := ""
	label := widget.NewLabel(T("keyfile.none"))
	label.Truncation = fyne.TextTruncateEllipsis
	set := func(p string) {
		path = p
		if p == "" {
			label.SetText(T("keyfile.none"))
		} else {
			label.SetText(filepath.Base(p))
		}
	}
	choose := newIconButton("", theme.FolderOpenIcon(), func() {
		a.openFile(T("keyfile.title"), nil, set)
	})
	gen := newIconButton(T("keyfile.generate"), theme.ContentAddIcon(), func() {
		a.saveFile(T("keyfile.generate"), "multi2fa.keyx", nil, func(p string) {
			if err := writeKeyFile(p); err != nil {
				a.showError(err)
				return
			}
			set(p)
			dialog.ShowInformation(T("keyfile.title"), T("keyfile.generated", filepath.Base(p)), a.win)
		})
	})
	clearBtn := newIconButton("", theme.ContentClearIcon(), func() { set("") })
	clearBtn.Importance = widget.LowImportance
	row := container.NewBorder(nil, nil, widget.NewIcon(theme.FileIcon()), container.NewHBox(choose, gen, clearBtn), label)
	return row, func() string { return path }
}

// writeKeyFile creates a key file with 128 random bytes.
func writeKeyFile(path string) error {
	b := make([]byte, 128)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	defer memguard.WipeBytes(b)
	return os.WriteFile(path, b, 0o600)
}

func levelLabel(l vault.Level) string {
	return i18n.T("kdf.level."+l.Name, l.Target.Seconds(), l.MemoryKiB/1024)
}

// securitySummary describes how a vault is protected; a zero cipher (not known before unlock) is omitted.
func securitySummary(sec vault.Security, keyFile bool) string {
	T := i18n.T
	var parts []string
	if sec.Cipher != 0 {
		parts = append(parts, sec.Cipher.String())
	}
	p := sec.KDF
	parts = append(parts, p.Kind.String())
	switch p.Kind {
	case vault.KDFArgon2id:
		parts = append(parts, T("sec.sum.argon", p.MemoryMiB(), p.Iterations, p.Threads))
	case vault.KDFScrypt:
		parts = append(parts, T("sec.sum.scrypt", p.LogN, p.BlockSize, p.Threads, p.MemoryMiB()))
	case vault.KDFPBKDF2:
		parts = append(parts, T("sec.sum.pbkdf2", p.Iterations))
	}
	if keyFile {
		parts = append(parts, T("keyfile.title"))
	}
	return strings.Join(parts, " · ")
}

func validateNewPassword(pw, pw2 *secureEntry) error {
	switch {
	case pw.Len() < minPasswordLen:
		return errors.New(i18n.T("create.short", minPasswordLen))
	case !pw.Equal(pw2):
		return errors.New(i18n.T("create.mismatch"))
	}
	return nil
}

// newPasswordFields builds password + confirmation inputs with a strength meter.
func newPasswordFields() (pw, pw2 *secureEntry, strength *widget.ProgressBar) {
	T := i18n.T
	pw = newSecureEntry(T("password.new"))
	pw2 = newSecureEntry(T("password.repeat"))
	strength = widget.NewProgressBar()
	strength.Max = 4
	strength.TextFormatter = func() string { return T(fmt.Sprintf("strength.%d", int(strength.Value))) }
	pw.OnChanged = func() { strength.SetValue(float64(strengthOf(pw.Bytes()))) }
	pw.OnSubmitted = func() {
		if c := fyne.CurrentApp().Driver().CanvasForObject(pw2); c != nil {
			c.Focus(pw2)
		}
	}
	return
}
