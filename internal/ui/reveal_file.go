package ui

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// showInFileManager opens the system file manager at a file, selecting it where supported.
func showInFileManager(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", "/select,", filepath.Clean(path))
	case "darwin":
		cmd = exec.Command("open", "-R", path)
	default:
		// FileManager1 selects the file in Dolphin, Nautilus, Nemo and others; xdg-open opens the folder.
		cmd = exec.Command("dbus-send", "--session", "--print-reply", "--dest=org.freedesktop.FileManager1",
			"/org/freedesktop/FileManager1", "org.freedesktop.FileManager1.ShowItems",
			"array:string:file://"+path, "string:")
		if err := cmd.Run(); err == nil {
			return nil
		}
		cmd = exec.Command("xdg-open", filepath.Dir(path))
	}
	return cmd.Start()
}
