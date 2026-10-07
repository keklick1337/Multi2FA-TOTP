//go:build !linux && !ios

package capture

import (
	"errors"
	"image"
)

func platformCapture() ([]image.Image, error) {
	return nil, errors.New("not used")
}
