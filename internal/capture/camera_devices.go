//go:build windows || darwin

package capture

import (
	"bytes"
	"context"
)

// ffmpegStderr runs ffmpeg and returns its stderr; device listings are printed there.
func ffmpegStderr(args ...string) (string, error) {
	cmd, err := ffmpegCommand(context.Background(), args...)
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run()
	return stderr.String(), nil
}
