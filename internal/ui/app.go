package ui

import (
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"

	"github.com/keklick1337/Multi2FA-TOTP/internal/assets"
	"github.com/keklick1337/Multi2FA-TOTP/internal/i18n"
	"github.com/keklick1337/Multi2FA-TOTP/internal/vault"
)

const (
	AppName    = "Multi2FA TOTP"
	AppID      = "io.github.keklick1337.multi2fa"
	Author     = "keklick1337"
	RepoURL    = "https://github.com/keklick1337/Multi2FA-TOTP"
	prefLang   = "language"
	prefTheme  = "theme"
	prefRecent = "recent_vaults"
	prefLast   = "last_vault"
	backupExt  = ".m2fab"
	vaultExt   = ".m2fa"
	maxRecent  = 12
)

var Version = "dev"

// session is one unlocked vault.
type session struct {
	path string
	v    *vault.Vault
	view *vaultView
}

func (s *session) name() string { return vaultName(s.path) }

type App struct {
	fa          fyne.App
	win         fyne.Window
	defaultPath string
	lockPath    string

	sessions []*session
	current  *session
	shell    *shell
	accounts *accountsPage
	search   *searchEntry

	selectMode bool

	lastActive atomic.Int64
	clipSeq    int
	clipText   string
	failCount  int
	retryAfter time.Time
	lockHooks  []func()
	capturing  bool
}

// Run starts the UI. openPath (from the command line or a file association) is unlocked first.
func Run(defaultPath, openPath string) {
	ensureCursorTheme()
	fa := app.NewWithID(AppID)
	fa.SetIcon(assets.Icon)
	i18n.Set(fa.Preferences().StringWithFallback(prefLang, i18n.Detect()))
	applyTheme(fa, fa.Preferences().StringWithFallback(prefTheme, "system"))

	if defaultPath == "" {
		// Mobile platforms: keep the vault in the app's private storage.
		defaultPath = filepath.Join(fa.Storage().RootURI().Path(), "vault.m2fa")
	}
	a := &App{fa: fa, defaultPath: defaultPath}
	a.lockPath = fa.Preferences().StringWithFallback(prefLast, defaultPath)
	if !vault.Exists(a.lockPath) {
		a.lockPath = defaultPath
	}
	if openPath != "" {
		a.lockPath = absPath(openPath)
		if vault.Exists(a.lockPath) {
			a.rememberVault(a.lockPath)
		}
	}
	a.win = fa.NewWindow(AppName)
	a.win.SetIcon(assets.Icon)
	a.win.Resize(fyne.NewSize(760, 720))
	a.win.CenterOnScreen()
	a.win.SetMaster()
	a.win.SetOnDropped(a.onDropped)

	fa.Lifecycle().SetOnExitedForeground(func() {
		if a.isUnlocked() && !a.capturing && a.settings().LockOnFocusLoss {
			a.lock()
		}
	})

	c := a.win.Canvas()
	c.SetOnTypedKey(func(*fyne.KeyEvent) { a.touch() })
	c.SetOnTypedRune(func(r rune) {
		a.touch()
		if a.isUnlocked() && c.Overlays().Top() == nil && a.shell.page == pageAccounts {
			c.Focus(a.search)
			a.search.TypedRune(r)
		}
	})
	a.installShortcuts()

	a.installTray()
	a.showLockScreen()
	go a.loop()
	a.win.ShowAndRun()
	a.shutdown()
}

func (a *App) isUnlocked() bool { return len(a.sessions) > 0 }

func (a *App) primary() *session { return a.sessions[0] }

// cur is the vault whose accounts are shown.
func (a *App) cur() *session {
	if a.current != nil {
		return a.current
	}
	return a.primary()
}

// settings are global and live in the first (primary) vault.
func (a *App) settings() vault.Settings {
	if !a.isUnlocked() {
		return vault.DefaultSettings
	}
	return a.primary().v.Settings()
}

func (a *App) loop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for range t.C {
		fyne.Do(a.tick)
	}
}

func (a *App) tick() {
	if !a.isUnlocked() {
		return
	}
	if mins := a.settings().AutoLockMinutes; mins > 0 {
		idle := time.Since(time.Unix(0, a.lastActive.Load()))
		if idle >= time.Duration(mins)*time.Minute {
			a.lock()
			return
		}
	}
	if a.shell.page == pageAccounts {
		a.cur().view.tick()
	}
}

func (a *App) touch() {
	a.lastActive.Store(time.Now().UnixNano())
}

// onLock registers cleanup to run the next time the app is locked (camera streams, timers).
func (a *App) onLock(fn func()) {
	a.lockHooks = append(a.lockHooks, fn)
}

func (a *App) unlocked(path string, v *vault.Vault) {
	a.failCount = 0
	a.touch()
	a.rememberVault(path)
	a.fa.Preferences().SetString(prefLast, path)
	s := &session{path: path, v: v}
	a.sessions = []*session{s}
	a.current = s
	a.buildShell()
	a.win.Canvas().Focus(a.search)
	go releaseMemory()
}

func (a *App) buildShell() {
	for _, s := range a.sessions {
		s.view = newVaultView(a, s)
	}
	a.accounts = newAccountsPage(a)
	a.shell = newShell(a)
	a.win.SetContent(a.shell.root)
	a.shell.show(pageAccounts, false)
	a.switchVault(a.cur())
}

// openSession adds an unlocked vault and shows it.
func (a *App) openSession(path string, v *vault.Vault) *session {
	s := &session{path: path, v: v}
	s.view = newVaultView(a, s)
	a.sessions = append(a.sessions, s)
	a.rememberVault(path)
	a.switchVault(s)
	go releaseMemory()
	return s
}

func (a *App) switchVault(s *session) {
	if a.current != s && a.selectMode {
		a.setSelectMode(false)
	}
	a.current = s
	a.accounts.showVault(s)
	if a.shell.page != pageAccounts {
		a.shell.show(pageAccounts, true)
	}
}

func (a *App) sessionByPath(path string) *session {
	for _, s := range a.sessions {
		if samePath(s.path, path) {
			return s
		}
	}
	return nil
}

func (a *App) closeSession(s *session) {
	i := slices.Index(a.sessions, s)
	if i < 0 {
		return
	}
	if i == 0 {
		a.notify(i18n.T("vault.primary.close"), "")
		return
	}
	s.v.Close()
	a.sessions = slices.Delete(a.sessions, i, i+1)
	defer releaseMemory()
	if a.current == s {
		a.current = nil
		a.accounts.showVault(a.cur())
	}
	a.shell.refreshPage()
}

func (a *App) lock() {
	if !a.isUnlocked() {
		return
	}
	for _, fn := range a.lockHooks {
		fn()
	}
	a.lockHooks = nil
	a.closeOverlays()
	a.clearClipboardIfOurs()
	a.lockPath = a.primary().path
	for _, s := range a.sessions {
		s.v.Close()
	}
	a.sessions = nil
	a.current = nil
	a.shell = nil
	a.accounts = nil
	a.search = nil
	a.selectMode = false
	wipeSecureEntries()
	a.showLockScreen()
	releaseMemory()
}

// releaseMemory returns freed vault structures to the OS right away instead of keeping them in the Go heap.
func releaseMemory() {
	runtime.GC()
	debug.FreeOSMemory()
}

func (a *App) closeOverlays() {
	ov := a.win.Canvas().Overlays()
	for ov.Top() != nil {
		ov.Remove(ov.Top())
	}
}

func (a *App) shutdown() {
	a.clearClipboardIfOurs()
	for _, s := range a.sessions {
		s.v.Close()
	}
	a.sessions = nil
}

// copySecret puts text on the clipboard and schedules clearing it.
func (a *App) copySecret(text string) {
	a.fa.Clipboard().SetContent(text)
	a.clipSeq++
	a.clipText = text
	seq := a.clipSeq
	secs := a.settings().ClipboardClearSecs
	if secs <= 0 {
		return
	}
	time.AfterFunc(time.Duration(secs)*time.Second, func() {
		fyne.Do(func() {
			if seq == a.clipSeq {
				a.clearClipboardIfOurs()
			}
		})
	})
}

func (a *App) clearClipboardIfOurs() {
	if a.clipText == "" {
		return
	}
	if a.fa.Clipboard().Content() == a.clipText {
		a.fa.Clipboard().SetContent("")
	}
	a.clipText = ""
}

// clipboardNote is the second toast line after copying something sensitive.
func (a *App) clipboardNote() string {
	if secs := a.settings().ClipboardClearSecs; secs > 0 {
		return i18n.T("clipboard.clears", secs)
	}
	return ""
}

// notify shows a short two-line toast at the bottom of the window.
func (a *App) notify(title, detail string) {
	if a.shell != nil {
		a.shell.toast.show(title, detail)
	}
}

func (a *App) showError(err error) {
	dialog.ShowError(err, a.win)
}

func (a *App) setSelectMode(on bool) {
	if !a.isUnlocked() {
		return
	}
	a.selectMode = on
	if !on {
		for _, s := range a.sessions {
			clear(s.view.selected)
		}
	}
	a.accounts.syncSelectMode()
	for _, s := range a.sessions {
		s.view.list.Refresh()
	}
}

func (a *App) updateSelection() {
	if a.accounts != nil {
		a.accounts.updateSelection()
	}
}

func (a *App) pasteMenuAction() {
	if !a.pasteFromClipboard() {
		a.notify(i18n.T("paste.nothing"), "")
	}
}

var (
	scNew      = &desktop.CustomShortcut{KeyName: fyne.KeyN, Modifier: fyne.KeyModifierShortcutDefault}
	scOpen     = &desktop.CustomShortcut{KeyName: fyne.KeyO, Modifier: fyne.KeyModifierShortcutDefault}
	scScreen   = &desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift}
	scCamera   = &desktop.CustomShortcut{KeyName: fyne.KeyK, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift}
	scLock     = &desktop.CustomShortcut{KeyName: fyne.KeyL, Modifier: fyne.KeyModifierShortcutDefault}
	scFind     = &desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault}
	scSettings = &desktop.CustomShortcut{KeyName: fyne.KeyComma, Modifier: fyne.KeyModifierShortcutDefault}
	scSelect   = &desktop.CustomShortcut{KeyName: fyne.KeyE, Modifier: fyne.KeyModifierShortcutDefault}
	scVaults   = &desktop.CustomShortcut{KeyName: fyne.KeyO, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift}
	scHome     = &desktop.CustomShortcut{KeyName: fyne.Key1, Modifier: fyne.KeyModifierShortcutDefault}
)

func (a *App) shortcutActions() map[fyne.Shortcut]func() {
	return map[fyne.Shortcut]func(){
		scNew:      func() { a.cur().view.showEditor(nil, nil) },
		scOpen:     a.openImage,
		scScreen:   a.scanScreen,
		scCamera:   a.scanCamera,
		scLock:     a.lock,
		scFind:     func() { a.shell.show(pageAccounts, true); a.win.Canvas().Focus(a.search) },
		scSettings: func() { a.shell.show(pageSettings, true) },
		scSelect:   func() { a.shell.show(pageAccounts, true); a.setSelectMode(!a.selectMode) },
		scVaults:   func() { a.shell.show(pageVaults, true) },
		scHome:     func() { a.shell.show(pageAccounts, true) },
	}
}

// handleShortcut runs app-wide shortcuts; the search entry forwards its unhandled ones here.
func (a *App) handleShortcut(s fyne.Shortcut) bool {
	if !a.isUnlocked() || a.win.Canvas().Overlays().Top() != nil {
		return false
	}
	for sc, fn := range a.shortcutActions() {
		if sc.ShortcutName() == s.ShortcutName() {
			a.touch()
			fn()
			return true
		}
	}
	return false
}

func (a *App) installShortcuts() {
	c := a.win.Canvas()
	for sc := range a.shortcutActions() {
		c.AddShortcut(sc, func(s fyne.Shortcut) { a.handleShortcut(s) })
	}
	c.AddShortcut(&fyne.ShortcutPaste{}, func(fyne.Shortcut) {
		if a.isUnlocked() && c.Overlays().Top() == nil {
			a.touch()
			a.pasteMenuAction()
		}
	})
}

// rebuild re-creates the visible UI, used after a language or theme change.
func (a *App) rebuild() {
	if !a.isUnlocked() {
		a.showLockScreen()
		return
	}
	query := a.search.Text
	page := a.shell.page
	a.closeOverlays()
	a.selectMode = false
	a.buildShell()
	a.search.SetText(query)
	a.shell.show(page, false)
}

func (a *App) setLanguage(code string) {
	i18n.Set(code)
	a.fa.Preferences().SetString(prefLang, code)
	a.installTray()
}

func (a *App) recentVaults() []string {
	list := a.fa.Preferences().StringList(prefRecent)
	if !slices.ContainsFunc(list, func(p string) bool { return samePath(p, a.defaultPath) }) {
		list = append(list, a.defaultPath)
	}
	return list
}

func (a *App) rememberVault(path string) {
	list := slices.DeleteFunc(a.fa.Preferences().StringList(prefRecent), func(p string) bool { return samePath(p, path) })
	list = append([]string{path}, list...)
	if len(list) > maxRecent {
		list = list[:maxRecent]
	}
	a.fa.Preferences().SetStringList(prefRecent, list)
}

func (a *App) forgetVault(path string) {
	list := slices.DeleteFunc(a.fa.Preferences().StringList(prefRecent), func(p string) bool { return samePath(p, path) })
	a.fa.Preferences().SetStringList(prefRecent, list)
}

func vaultName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

func samePath(a, b string) bool {
	pa, err1 := filepath.Abs(a)
	pb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(pa, pb)
	}
	return pa == pb
}

func isMac() bool { return runtime.GOOS == "darwin" }

func isMobile() bool { return fyne.CurrentDevice().IsMobile() }
