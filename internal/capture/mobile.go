//go:build android || ios

package capture

import (
	"errors"
	"image"
)

var ErrClipboardEmpty = errors.New("clipboard has no image or otpauth link")

type ClipboardContent struct {
	Image []byte
	Text  string
}

// ReadClipboard has no image support on mobile; the UI falls back to the text clipboard.
func ReadClipboard() (*ClipboardContent, error) { return nil, ErrClipboardEmpty }

func CaptureScreens() ([]image.Image, error) {
	return nil, errors.New("screen capture is not available on mobile")
}
