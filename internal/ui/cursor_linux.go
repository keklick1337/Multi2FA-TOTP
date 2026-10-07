//go:build linux && !android

package ui

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ensureCursorTheme makes GLFW use the desktop's cursor theme. GLFW's Wayland backend only reads
// XCURSOR_THEME/XCURSOR_SIZE; without them libwayland-cursor falls back to the old X11 core
// cursors (the crooked "hand2" instead of the usual pointer) at the wrong size.
func ensureCursorTheme() {
	theme, size := os.Getenv("XCURSOR_THEME"), os.Getenv("XCURSOR_SIZE")
	if theme != "" && size != "" {
		return
	}
	for _, detect := range []func() (string, string){kdeCursor, xrdbCursor, gnomeCursor, gtkCursor, defaultIndexCursor} {
		t, s := detect()
		if theme == "" && t != "" && cursorThemeExists(t) {
			theme = t
		}
		if size == "" && s != "" && s != "0" {
			size = s
		}
		if theme != "" && size != "" {
			break
		}
	}
	if theme == "" {
		for _, t := range []string{"breeze_cursors", "Adwaita", "Breeze_Light", "Yaru", "DMZ-White"} {
			if cursorThemeExists(t) {
				theme = t
				break
			}
		}
	}
	if theme != "" {
		os.Setenv("XCURSOR_THEME", theme)
	}
	if size != "" {
		os.Setenv("XCURSOR_SIZE", size)
	}
}

func configDir() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return d
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".config")
}

// iniValue returns key from section ("" matches any section) of an ini-style file.
func iniValue(path, section, key string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	cur := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			cur = line[1 : len(line)-1]
			continue
		}
		if section != "" && cur != section {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == key {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

func kdeCursor() (string, string) {
	p := filepath.Join(configDir(), "kcminputrc")
	return iniValue(p, "Mouse", "cursorTheme"), iniValue(p, "Mouse", "cursorSize")
}

func run(name string, args ...string) string {
	if _, err := exec.LookPath(name); err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func xrdbCursor() (string, string) {
	if os.Getenv("DISPLAY") == "" {
		return "", ""
	}
	var theme, size string
	for _, line := range strings.Split(run("xrdb", "-query"), "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "Xcursor.theme":
			theme = strings.TrimSpace(v)
		case "Xcursor.size":
			size = strings.TrimSpace(v)
		}
	}
	return theme, size
}

func gnomeCursor() (string, string) {
	t := strings.Trim(run("gsettings", "get", "org.gnome.desktop.interface", "cursor-theme"), `'`)
	s := run("gsettings", "get", "org.gnome.desktop.interface", "cursor-size")
	if i := strings.LastIndex(s, " "); i >= 0 {
		s = s[i+1:]
	}
	return t, s
}

func gtkCursor() (string, string) {
	p := filepath.Join(configDir(), "gtk-3.0", "settings.ini")
	return iniValue(p, "Settings", "gtk-cursor-theme-name"), iniValue(p, "Settings", "gtk-cursor-theme-size")
}

func defaultIndexCursor() (string, string) {
	h, _ := os.UserHomeDir()
	for _, p := range []string{filepath.Join(h, ".icons", "default", "index.theme"), filepath.Join(h, ".local", "share", "icons", "default", "index.theme"), "/usr/share/icons/default/index.theme"} {
		if t := iniValue(p, "Icon Theme", "Inherits"); t != "" {
			return strings.Split(t, ",")[0], ""
		}
	}
	return "", ""
}

func cursorThemeExists(name string) bool {
	h, _ := os.UserHomeDir()
	dirs := []string{filepath.Join(h, ".icons"), filepath.Join(h, ".local", "share", "icons")}
	for _, d := range strings.Split(os.Getenv("XDG_DATA_DIRS"), ":") {
		if d != "" {
			dirs = append(dirs, filepath.Join(d, "icons"))
		}
	}
	dirs = append(dirs, "/usr/share/icons", "/usr/local/share/icons")
	for _, d := range dirs {
		if st, err := os.Stat(filepath.Join(d, name, "cursors")); err == nil && st.IsDir() {
			return true
		}
	}
	return false
}
