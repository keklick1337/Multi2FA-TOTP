package qr

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"slices"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/multi/qrcode"
	single "github.com/makiuchi-d/gozxing/qrcode"
	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

var ErrNotFound = errors.New("no QR code found")

// DecodeBytes decodes an encoded image (PNG, JPEG, GIF, BMP, WebP) and returns every QR payload in it.
func DecodeBytes(data []byte) ([]string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return Decode(img)
}

// Decode tries hard to find all QR codes: several scales, both binarizers and inverted colors,
// because screenshots are often huge and dark-themed.
func Decode(img image.Image) ([]string, error) {
	var found []string
	add := func(s string) {
		if s != "" && !slices.Contains(found, s) {
			found = append(found, s)
		}
	}
	for _, candidate := range variants(img) {
		for _, s := range scan(candidate) {
			add(s)
		}
		if len(found) > 0 {
			return found, nil
		}
	}
	return nil, ErrNotFound
}

// Quick is a cheaper single pass, used for camera frames.
func Quick(img image.Image) []string {
	return scan(img)
}

func variants(img image.Image) []image.Image {
	out := []image.Image{img}
	b := img.Bounds()
	longest := max(b.Dx(), b.Dy())
	for _, target := range []int{1600, 1000, 600} {
		if longest > target {
			out = append(out, scale(img, target))
		}
	}
	if longest < 300 && longest > 0 {
		out = append(out, scale(img, 600))
	}
	out = append(out, invert(img))
	return out
}

func scan(img image.Image) []string {
	src := gozxing.NewLuminanceSourceFromImage(img)
	hints := map[gozxing.DecodeHintType]interface{}{gozxing.DecodeHintType_TRY_HARDER: true}
	var out []string
	for _, bin := range []gozxing.Binarizer{gozxing.NewHybridBinarizer(src), gozxing.NewGlobalHistgramBinarizer(src)} {
		bmp, err := gozxing.NewBinaryBitmap(bin)
		if err != nil {
			continue
		}
		if res, err := qrcode.NewQRCodeMultiReader().DecodeMultiple(bmp, hints); err == nil {
			for _, r := range res {
				out = append(out, r.GetText())
			}
		}
		if len(out) == 0 {
			if r, err := single.NewQRCodeReader().Decode(bmp, hints); err == nil {
				out = append(out, r.GetText())
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return out
}

func scale(img image.Image, longest int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w >= h {
		h = h * longest / w
		w = longest
	} else {
		w = w * longest / h
		h = longest
	}
	dst := image.NewGray(image.Rect(0, 0, max(w, 1), max(h, 1)))
	xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

func invert(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewGray(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			g := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			dst.SetGray(x, y, color.Gray{Y: 255 - g.Y})
		}
	}
	return dst
}

// Encode renders text as a QR code image with a quiet zone.
func Encode(text string, size int) (image.Image, error) {
	hints := map[gozxing.EncodeHintType]interface{}{gozxing.EncodeHintType_MARGIN: 2}
	m, err := single.NewQRCodeWriter().Encode(text, gozxing.BarcodeFormat_QR_CODE, size, size, hints)
	if err != nil {
		return nil, err
	}
	return m, nil
}
