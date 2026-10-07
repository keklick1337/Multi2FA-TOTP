//go:build darwin && !ios

package capture

import (
	"os/exec"
	"regexp"
	"strings"
)

var avfDevice = regexp.MustCompile(`\]\s*\[(\d+)\]\s*(.+)$`)

func ListCameras() ([]Camera, error) {
	out, err := ffmpegStderr("-hide_banner", "-f", "avfoundation", "-list_devices", "true", "-i", "")
	if err != nil {
		return nil, err
	}
	var cams []Camera
	video := false
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.Contains(line, "AVFoundation video devices"):
			video = true
			continue
		case strings.Contains(line, "AVFoundation audio devices"):
			video = false
			continue
		}
		if !video {
			continue
		}
		m := avfDevice.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil || strings.Contains(strings.ToLower(m[2]), "capture screen") {
			continue
		}
		cams = append(cams, Camera{ID: m[1], Name: strings.TrimSpace(m[2])})
	}
	return cams, nil
}

func inputArgs(cam Camera) []string {
	return []string{"-f", "avfoundation", "-framerate", "30", "-i", cam.ID + ":none"}
}

func extraFFmpegDirs() []string {
	return []string{"/opt/homebrew/bin", "/usr/local/bin"}
}

func hideConsole(*exec.Cmd) {}
