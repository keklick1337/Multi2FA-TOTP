package main

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// copyToClipboard hands text to the platform's clipboard tool.
func copyToClipboard(text string) error {
	var tools [][]string
	switch runtime.GOOS {
	case "windows":
		tools = [][]string{{"clip.exe"}}
	case "darwin":
		tools = [][]string{{"pbcopy"}}
	default:
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			tools = append(tools, []string{"wl-copy"})
		}
		tools = append(tools, []string{"xclip", "-selection", "clipboard"}, []string{"xsel", "--clipboard", "--input"})
		if os.Getenv("TERMUX_VERSION") != "" {
			tools = append(tools, []string{"termux-clipboard-set"})
		}
	}
	for _, t := range tools {
		path, err := exec.LookPath(t[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(path, t[1:]...)
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	return errors.New("no clipboard tool found (install wl-clipboard, xclip or xsel)")
}
