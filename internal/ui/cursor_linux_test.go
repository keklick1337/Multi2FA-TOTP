//go:build linux && !android

package ui

import (
	"os"
	"testing"
)

func TestEnsureCursorThemeKeepsExplicit(t *testing.T) {
	t.Setenv("XCURSOR_THEME", "MyTheme")
	t.Setenv("XCURSOR_SIZE", "32")
	ensureCursorTheme()
	if os.Getenv("XCURSOR_THEME") != "MyTheme" || os.Getenv("XCURSOR_SIZE") != "32" {
		t.Fatal("explicit cursor settings were overridden")
	}
}

func TestEnsureCursorThemeDetects(t *testing.T) {
	t.Setenv("XCURSOR_THEME", "")
	t.Setenv("XCURSOR_SIZE", "")
	ensureCursorTheme()
	t.Logf("detected XCURSOR_THEME=%q XCURSOR_SIZE=%q", os.Getenv("XCURSOR_THEME"), os.Getenv("XCURSOR_SIZE"))
}
