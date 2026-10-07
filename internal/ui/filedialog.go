package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
)

// File dialogs use the platform's own picker (XDG portal / zenity on Linux, the system dialogs on
// Windows and macOS) and fall back to Fyne's built-in dialog when none is available.

var (
	errCanceled    = errors.New("canceled")
	errUnavailable = errors.New("native dialog unavailable")
)

// fileFilter is a named set of extensions such as ".m2fa"; nil means any file.
type fileFilter struct {
	name string
	exts []string
}

func (f *fileFilter) patterns() []string {
	var out []string
	for _, e := range f.exts {
		out = append(out, "*"+e)
	}
	return out
}

// openFile asks for an existing file.
func (a *App) openFile(title string, filter *fileFilter, done func(path string)) {
	a.runFileDialog(func() (string, error) { return nativeOpen(title, filter) }, done, func() {
		d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
			if err != nil {
				a.showError(err)
				return
			}
			if r != nil {
				path := r.URI().Path()
				r.Close()
				done(path)
			}
		}, a.win)
		if filter != nil {
			d.SetFilter(storage.NewExtensionFileFilter(filter.exts))
		}
		d.Show()
		d.Resize(fyne.NewSize(760, 540))
	})
}

// saveFile asks where to write a new file; ext is appended when the user leaves it out.
func (a *App) saveFile(title, name string, filter *fileFilter, done func(path string)) {
	fix := func(path string) string {
		if filter != nil && len(filter.exts) > 0 {
			ext := filter.exts[0]
			if !strings.HasSuffix(strings.ToLower(path), ext) {
				path += ext
			}
		}
		return path
	}
	a.runFileDialog(func() (string, error) { return nativeSave(title, name, filter) }, func(p string) { done(fix(p)) }, func() {
		d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
			if err != nil {
				a.showError(err)
				return
			}
			if w != nil {
				path := w.URI().Path()
				w.Close()
				removeIfEmpty(path)
				done(fix(path))
			}
		}, a.win)
		d.SetFileName(name)
		if filter != nil {
			d.SetFilter(storage.NewExtensionFileFilter(filter.exts))
		}
		d.Show()
		d.Resize(fyne.NewSize(760, 540))
	})
}

func (a *App) runFileDialog(native func() (string, error), done func(string), fallback func()) {
	if isMobile() {
		fallback()
		return
	}
	go func() {
		path, err := native()
		fyne.Do(func() {
			switch {
			case errors.Is(err, errCanceled):
			case err != nil:
				fallback()
			default:
				done(path)
			}
		})
	}()
}

// removeIfEmpty deletes the placeholder file a save dialog creates before we write the real one.
func removeIfEmpty(path string) {
	if st, err := os.Stat(path); err == nil && st.Size() == 0 {
		os.Remove(path)
	}
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return filepath.Dir(os.Args[0])
}
