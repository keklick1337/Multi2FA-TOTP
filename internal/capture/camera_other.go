//go:build (!linux && !darwin && !windows) || android || ios

package capture

import (
	"errors"
	"os/exec"
)

func ListCameras() ([]Camera, error) {
	return nil, errors.New("camera capture is not supported on this platform")
}

func inputArgs(Camera) []string { return nil }

func extraFFmpegDirs() []string { return nil }

func hideConsole(*exec.Cmd) {}
