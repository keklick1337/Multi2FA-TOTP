//go:build linux && !android

package capture

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func ListCameras() ([]Camera, error) {
	if _, err := FFmpegPath(); err != nil {
		return nil, err
	}
	nodes, _ := filepath.Glob("/sys/class/video4linux/video*")
	sort.Strings(nodes)
	var cams []Camera
	for _, n := range nodes {
		// Each physical camera exposes extra metadata nodes; only index 0 captures video.
		if idx, err := os.ReadFile(filepath.Join(n, "index")); err == nil && strings.TrimSpace(string(idx)) != "0" {
			continue
		}
		dev := "/dev/" + filepath.Base(n)
		name := dev
		if b, err := os.ReadFile(filepath.Join(n, "name")); err == nil {
			name = strings.TrimSpace(string(b)) + " (" + dev + ")"
		}
		cams = append(cams, Camera{ID: dev, Name: name})
	}
	return cams, nil
}

func inputArgs(cam Camera) []string {
	return []string{"-f", "v4l2", "-i", cam.ID}
}

func extraFFmpegDirs() []string { return nil }

func hideConsole(*exec.Cmd) {}
