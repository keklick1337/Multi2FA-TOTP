package qr

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	const uri = "otpauth://totp/GitHub:alice?secret=JBSWY3DPEHPK3PXP&issuer=GitHub"
	code, err := Encode(uri, 300)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(code)
	if err != nil || len(got) != 1 || got[0] != uri {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestFindsTwoCodesInLargeScreenshot(t *testing.T) {
	a, _ := Encode("otpauth://totp/A?secret=JBSWY3DPEHPK3PXP", 260)
	b, _ := Encode("otpauth://totp/B?secret=KRSXG5CTMVRXEZLU", 260)
	screen := image.NewRGBA(image.Rect(0, 0, 2560, 1440))
	draw.Draw(screen, screen.Bounds(), &image.Uniform{color.RGBA{40, 44, 52, 255}}, image.Point{}, draw.Src)
	draw.Draw(screen, image.Rect(300, 400, 560, 660), a, image.Point{}, draw.Src)
	draw.Draw(screen, image.Rect(1800, 700, 2060, 960), b, image.Point{}, draw.Src)
	got, err := Decode(screen)
	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
}
