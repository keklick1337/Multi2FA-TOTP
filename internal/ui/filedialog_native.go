//go:build !android && !ios

package ui

import (
	"errors"
	"net/url"
	"runtime"

	"github.com/ncruces/zenity"
	"github.com/rymdport/portal/filechooser"
)

func nativeOpen(title string, f *fileFilter) (string, error) {
	if runtime.GOOS == "linux" {
		opts := &filechooser.OpenFileOptions{CurrentFolder: homeDir(), Filters: portalFilters(f)}
		if uris, err := filechooser.OpenFile("", title, opts); err == nil {
			return firstPath(uris)
		}
	}
	return zenityResult(zenity.SelectFile(zenity.Title(title), zenityFilters(f)))
}

func nativeSave(title, name string, f *fileFilter) (string, error) {
	if runtime.GOOS == "linux" {
		opts := &filechooser.SaveFileOptions{CurrentFolder: homeDir(), CurrentName: name, Filters: portalFilters(f)}
		if uris, err := filechooser.SaveFile("", title, opts); err == nil {
			return firstPath(uris)
		}
	}
	return zenityResult(zenity.SelectFileSave(zenity.Title(title), zenity.Filename(name), zenity.ConfirmOverwrite(), zenityFilters(f)))
}

func portalFilters(f *fileFilter) []*filechooser.Filter {
	if f == nil {
		return nil
	}
	pf := &filechooser.Filter{Name: f.name}
	for _, p := range f.patterns() {
		pf.Rules = append(pf.Rules, filechooser.Rule{Type: filechooser.GlobPattern, Pattern: p})
	}
	return []*filechooser.Filter{pf}
}

func zenityFilters(f *fileFilter) zenity.FileFilters {
	if f == nil {
		return nil
	}
	return zenity.FileFilters{{Name: f.name, Patterns: f.patterns(), CaseFold: true}}
}

func firstPath(uris []string) (string, error) {
	if len(uris) == 0 {
		return "", errCanceled
	}
	u, err := url.Parse(uris[0])
	if err != nil || u.Scheme != "file" {
		return "", errUnavailable
	}
	return u.Path, nil
}

func zenityResult(path string, err error) (string, error) {
	switch {
	case errors.Is(err, zenity.ErrCanceled):
		return "", errCanceled
	case err != nil:
		return "", errUnavailable
	case path == "":
		return "", errCanceled
	}
	return path, nil
}
