package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/keklick1337/Multi2FA-TOTP/internal/assets"
	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
)

const prefCloseToTray = "close_to_tray"

// installTray adds the system tray icon (desktop only) and the close-to-tray behavior.
func (a *App) installTray() {
	desk, ok := a.fa.(desktop.App)
	if !ok {
		return
	}
	T := i18n.T
	show := func() {
		a.win.Show()
		a.win.RequestFocus()
	}
	desk.SetSystemTrayIcon(assets.Icon)
	desk.SetSystemTrayMenu(fyne.NewMenu(AppName,
		fyne.NewMenuItem(T("tray.show"), show),
		fyne.NewMenuItem(T("lock.now"), a.lock),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(T("tray.quit"), a.fa.Quit),
	))
	a.win.SetCloseIntercept(func() {
		if a.fa.Preferences().Bool(prefCloseToTray) {
			a.win.Hide()
			return
		}
		a.fa.Quit()
	})
}
