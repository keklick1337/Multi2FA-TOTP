package ui

import (
	"errors"
	"image"
	"io"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/keklick1337/Multi2FA-TOTP/internal/capture"
	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
	"github.com/keklick1337/Multi2FA-TOTP/internal/otp"
	"github.com/keklick1337/Multi2FA-TOTP/internal/qr"
)

var imageExts = []string{".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp"}

// importPayloads turns decoded QR texts / links into accounts.
// A single account opens the editor so the user can name it; several are imported in bulk.
func (a *App) importPayloads(payloads []string) {
	T := i18n.T
	if !a.isUnlocked() {
		return
	}
	target := a.cur()
	var keys []*otp.Key
	var lastErr error
	for _, p := range payloads {
		ks, err := otp.ParseAny(p)
		if err != nil {
			lastErr = err
			continue
		}
		keys = append(keys, ks...)
	}
	if len(keys) == 0 {
		if lastErr != nil && !errors.Is(lastErr, otp.ErrNotOTPAuth) {
			a.showError(errors.New(T("import.invalid", lastErr.Error())))
		} else if len(payloads) > 0 {
			a.showError(errors.New(T("import.nototp")))
		} else {
			a.showError(errors.New(T("import.noqr")))
		}
		return
	}
	if len(keys) == 1 {
		if target.v.HasSecret(keys[0].Secret) {
			dialog.ShowInformation(T("import.title"), T("editor.duplicate"), a.win)
			return
		}
		target.view.showEditor(nil, keys[0])
		return
	}

	var names []string
	for _, k := range keys {
		names = append(names, "• "+k.Label())
	}
	list := widget.NewLabel(strings.Join(names, "\n"))
	scroll := container.NewVScroll(list)
	scroll.SetMinSize(fyne.NewSize(380, 220))
	content := container.NewBorder(wrapLabel(i18n.N("import.found", len(keys), target.name())), nil, nil, nil, scroll)
	dialog.ShowCustomConfirm(T("import.title"), T("import.button"), T("cancel"), content, func(ok bool) {
		if !ok || !a.isUnlocked() {
			return
		}
		added, skipped, err := target.v.AddKeys(keys, target.view.folder)
		if err != nil {
			a.showError(err)
			return
		}
		target.view.reload()
		a.notify(T("import.done", target.name()), T("import.result", added, skipped))
	}, a.win)
}

// decodeAsync runs QR detection off the UI thread with a progress dialog.
func (a *App) decodeAsync(decode func() ([]string, error)) {
	prog := dialog.NewCustomWithoutButtons(i18n.T("scan.searching"), widget.NewProgressBarInfinite(), a.win)
	prog.Show()
	go func() {
		payloads, err := decode()
		fyne.Do(func() {
			prog.Hide()
			if !a.isUnlocked() {
				return
			}
			if err != nil && !errors.Is(err, qr.ErrNotFound) {
				a.showError(err)
				return
			}
			a.importPayloads(payloads)
		})
	}()
}

func (a *App) importImageBytes(data []byte) {
	a.decodeAsync(func() ([]string, error) { return qr.DecodeBytes(data) })
}

// pasteFromClipboard handles Ctrl+V: a screenshot/image, a copied image file or an otpauth link.
// It returns false when the clipboard holds nothing importable so normal text paste can proceed.
func (a *App) pasteFromClipboard() bool {
	if !a.isUnlocked() {
		return false
	}
	c, err := capture.ReadClipboard()
	if err != nil {
		if txt := a.fa.Clipboard().Content(); hasOTPLink(txt) {
			a.importPayloads(extractLinks(txt))
			return true
		}
		return false
	}
	if len(c.Image) > 0 {
		a.importImageBytes(c.Image)
		return true
	}
	if hasOTPLink(c.Text) {
		a.importPayloads(extractLinks(c.Text))
		return true
	}
	return false
}

func hasOTPLink(s string) bool {
	return strings.Contains(strings.ToLower(s), "otpauth")
}

func extractLinks(s string) []string {
	var out []string
	for _, f := range strings.Fields(s) {
		if strings.HasPrefix(strings.ToLower(f), "otpauth") {
			out = append(out, f)
		}
	}
	return out
}

func (a *App) openImage() {
	a.openFile(i18n.T("add.image"), &fileFilter{name: i18n.T("filter.images"), exts: imageExts}, func(path string) {
		data, err := readLimited(path, 64<<20)
		if err != nil {
			a.showError(err)
			return
		}
		a.importImageBytes(data)
	})
}

func readLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

// scanScreen hides the window, grabs all displays and looks for QR codes.
func (a *App) scanScreen() {
	a.capturing = true
	a.win.Hide()
	go func() {
		time.Sleep(450 * time.Millisecond)
		imgs, err := capture.CaptureScreens()
		fyne.Do(func() {
			a.win.Show()
			a.win.RequestFocus()
			time.AfterFunc(time.Second, func() { fyne.Do(func() { a.capturing = false }) })
			if err != nil {
				a.showError(errors.New(i18n.T("scan.screen.failed", err.Error())))
				return
			}
			a.decodeAsync(func() ([]string, error) {
				var all []string
				for _, img := range imgs {
					if found, err := qr.Decode(img); err == nil {
						all = append(all, found...)
					}
				}
				return all, nil
			})
		})
	}()
}

func (a *App) onDropped(_ fyne.Position, uris []fyne.URI) {
	if !a.isUnlocked() || len(uris) == 0 {
		return
	}
	a.touch()
	u := uris[0]
	ext := strings.ToLower(u.Extension())
	if ext == backupExt || ext == ".m2fa" {
		a.importBackupFile(u.Path())
		return
	}
	data, err := os.ReadFile(u.Path())
	if err != nil {
		a.showError(err)
		return
	}
	a.importImageBytes(data)
}

// scanCamera shows a live preview and stops as soon as an otpauth QR is seen.
func (a *App) scanCamera() {
	T := i18n.T
	preview := canvas.NewImageFromImage(image.NewGray(image.Rect(0, 0, 4, 3)))
	preview.FillMode = canvas.ImageFillContain
	preview.SetMinSize(fyne.NewSize(480, 360))
	statusLabel := widget.NewLabel(T("camera.starting"))
	statusLabel.Wrapping = fyne.TextWrapWord
	camSelect := newSelect(nil, nil)

	var stream *capture.Stream
	var cams []capture.Camera
	closed := false
	stop := func() {
		if stream != nil {
			s := stream
			stream = nil
			go s.Close()
		}
	}

	content := container.NewBorder(camSelect, statusLabel, nil, nil, preview)
	d := dialog.NewCustom(T("camera.title"), T("cancel"), content, a.win)
	d.SetOnClosed(func() { closed = true; stop() })
	a.onLock(func() { closed = true; stop() })

	start := func(cam capture.Camera) {
		stop()
		statusLabel.SetText(T("camera.hint"))
		s, err := capture.OpenCamera(cam)
		if err != nil {
			statusLabel.SetText(err.Error())
			return
		}
		stream = s
		go func() {
			for img := range s.Frames {
				fyne.Do(func() {
					if stream == s {
						preview.Image = img
						preview.Refresh()
					}
				})
				var otpFound []string
				nonOTP := false
				for _, p := range qr.Quick(img) {
					if hasOTPLink(p) {
						otpFound = append(otpFound, p)
					} else {
						nonOTP = true
					}
				}
				if len(otpFound) > 0 {
					fyne.Do(func() {
						if stream != s || closed {
							return
						}
						stop()
						d.Hide()
						a.importPayloads(otpFound)
					})
					return
				}
				if nonOTP {
					fyne.Do(func() { statusLabel.SetText(T("camera.nototp")) })
				}
			}
			if err := s.Err(); err != nil {
				fyne.Do(func() {
					if stream == s {
						statusLabel.SetText(T("camera.error", err.Error()))
					}
				})
			}
		}()
	}

	camSelect.OnChanged = func(name string) {
		for _, c := range cams {
			if c.Name == name {
				start(c)
			}
		}
	}

	d.Show()
	go func() {
		list, err := capture.ListCameras()
		fyne.Do(func() {
			if closed {
				return
			}
			if err != nil {
				statusLabel.SetText(err.Error())
				return
			}
			if len(list) == 0 {
				statusLabel.SetText(T("camera.none"))
				return
			}
			cams = list
			var names []string
			for _, c := range cams {
				names = append(names, c.Name)
			}
			camSelect.Options = names
			camSelect.SetSelected(names[0])
		})
	}()
}
