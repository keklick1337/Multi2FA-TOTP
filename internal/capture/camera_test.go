//go:build !android && !ios

package capture

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/keklick1337/Multi2FA-TOTP/internal/qr"
)

func TestSplitJPEG(t *testing.T) {
	var stream bytes.Buffer
	img := image.NewGray(image.Rect(0, 0, 64, 48))
	for i := 0; i < 3; i++ {
		img.SetGray(i, i, color.Gray{Y: 200})
		stream.Write([]byte{0x00, 0x13})
		if err := jpeg.Encode(&stream, img, nil); err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	splitJPEG(&stream, func(b []byte) {
		if _, err := jpeg.Decode(bytes.NewReader(b)); err != nil {
			t.Errorf("frame %d: %v", n, err)
		}
		n++
	})
	if n != 3 {
		t.Fatalf("got %d frames", n)
	}
}

// TestFFmpegPipeline feeds a looping QR image through the same ffmpeg output chain the camera uses.
func TestFFmpegPipeline(t *testing.T) {
	bin, err := FFmpegPath()
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	const uri = "otpauth://totp/Test:me?secret=JBSWY3DPEHPK3PXP&issuer=Test"
	code, _ := qr.Encode(uri, 400)
	f, _ := os.CreateTemp(t.TempDir(), "*.jpg")
	jpeg.Encode(f, code, nil)
	f.Close()

	cmd := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-loop", "1", "-i", f.Name(), "-t", "2",
		"-an", "-vf", "fps=8,scale='min(960,iw)':-2", "-f", "image2pipe", "-c:v", "mjpeg", "-q:v", "4", "-")
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	found := ""
	deadline := time.Now().Add(10 * time.Second)
	splitJPEG(out, func(b []byte) {
		if found != "" || time.Now().After(deadline) {
			return
		}
		img, err := jpeg.Decode(bytes.NewReader(b))
		if err != nil {
			return
		}
		if res := qr.Quick(img); len(res) > 0 {
			found = res[0]
		}
	})
	if found != uri {
		t.Fatalf("QR not decoded from ffmpeg frames, got %q", found)
	}
}
