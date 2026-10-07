//go:build !linux || android

package ui

// Windows and macOS give native cursors to GLFW directly.
func ensureCursorTheme() {}
