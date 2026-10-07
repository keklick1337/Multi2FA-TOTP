//go:build android || ios

package ui

func nativeOpen(string, *fileFilter) (string, error)         { return "", errUnavailable }
func nativeSave(string, string, *fileFilter) (string, error) { return "", errUnavailable }
