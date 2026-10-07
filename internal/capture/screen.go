//go:build !android && !ios

package capture

import (
	"errors"
	"image"

	"github.com/kbinani/screenshot"
)

// CaptureScreens grabs every connected display.
func CaptureScreens() ([]image.Image, error) {
	if imgs, err := platformCapture(); err == nil && len(imgs) > 0 {
		return imgs, nil
	}
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return nil, errors.New("no displays found")
	}
	var out []image.Image
	var lastErr error
	for i := 0; i < n; i++ {
		img, err := screenshot.CaptureDisplay(i)
		if err != nil {
			lastErr = err
			continue
		}
		out = append(out, img)
	}
	if len(out) == 0 {
		return nil, lastErr
	}
	return out, nil
}
