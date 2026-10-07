//go:build windows

package capture

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

var (
	dshowNew = regexp.MustCompile(`"([^"]+)"\s+\(video\)`)
	dshowOld = regexp.MustCompile(`\]\s+"([^"]+)"\s*$`)
)

func ListCameras() ([]Camera, error) {
	out, err := ffmpegStderr("-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy")
	if err != nil {
		return nil, err
	}
	var cams []Camera
	seen := map[string]bool{}
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			cams = append(cams, Camera{ID: name, Name: name})
		}
	}
	legacyVideo := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if m := dshowNew.FindStringSubmatch(line); m != nil {
			add(m[1])
			continue
		}
		switch {
		case strings.Contains(line, "DirectShow video devices"):
			legacyVideo = true
			continue
		case strings.Contains(line, "DirectShow audio devices"):
			legacyVideo = false
			continue
		case strings.Contains(line, "Alternative name"):
			continue
		}
		if legacyVideo {
			if m := dshowOld.FindStringSubmatch(line); m != nil {
				add(m[1])
			}
		}
	}
	return cams, nil
}

func inputArgs(cam Camera) []string {
	return []string{"-f", "dshow", "-i", "video=" + cam.ID}
}

func extraFFmpegDirs() []string {
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(exe), "ffmpeg", "bin"))
	}
	return dirs
}

func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
