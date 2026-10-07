//go:build !android && !ios

package capture

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"golang.design/x/clipboard"
)

var (
	clipOnce sync.Once
	clipErr  error
)

func clipInit() error {
	clipOnce.Do(func() { clipErr = clipboard.Init() })
	return clipErr
}

var ErrClipboardEmpty = errors.New("clipboard has no image or otpauth link")

// ClipboardContent is what a paste produced: raw image bytes and/or text.
type ClipboardContent struct {
	Image []byte
	Text  string
}

// ReadClipboard returns an image (pasted screenshot or copied image file) or text from the clipboard.
func ReadClipboard() (*ClipboardContent, error) {
	if err := clipInit(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if img, err := clipboard.Read(ctx, clipboard.FmtImage); err == nil && len(img) > 0 {
		return &ClipboardContent{Image: img}, nil
	}
	if files, err := clipboard.ReadFiles(ctx); err == nil {
		for _, f := range files {
			if isImageFile(f) {
				if b, err := os.ReadFile(f); err == nil {
					return &ClipboardContent{Image: b}, nil
				}
			}
		}
	}
	if txt, err := clipboard.Read(ctx, clipboard.FmtText); err == nil && len(txt) > 0 {
		return &ClipboardContent{Text: string(txt)}, nil
	}
	return nil, ErrClipboardEmpty
}

func isImageFile(name string) bool {
	n := strings.ToLower(name)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".bmp", ".webp"} {
		if strings.HasSuffix(n, ext) {
			return true
		}
	}
	return false
}
