//go:build linux && !android

package capture

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/godbus/dbus/v5"
)

// On Wayland the X11 grabber only sees XWayland windows, so the desktop portal
// and common CLI tools are tried first.
func platformCapture() ([]image.Image, error) {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		return nil, errors.New("not wayland")
	}
	if img, err := portalScreenshot(); err == nil {
		return []image.Image{img}, nil
	}
	return toolScreenshot()
}

func toolScreenshot() ([]image.Image, error) {
	dir, err := os.MkdirTemp("", "m2fa-shot-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "s.png")
	tools := [][]string{
		{"grim", out},
		{"spectacle", "-b", "-n", "-f", "-o", out},
		{"gnome-screenshot", "-f", out},
	}
	for _, t := range tools {
		if _, err := exec.LookPath(t[0]); err != nil {
			continue
		}
		if err := exec.Command(t[0], t[1:]...).Run(); err != nil {
			continue
		}
		if img, err := loadImage(out); err == nil {
			return []image.Image{img}, nil
		}
	}
	return nil, errors.New("no screenshot tool available")
}

func portalScreenshot() (image.Image, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	tokenBytes := make([]byte, 8)
	_, _ = rand.Read(tokenBytes)
	token := "m2fa" + hex.EncodeToString(tokenBytes)

	sig := make(chan *dbus.Signal, 4)
	conn.Signal(sig)
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	); err != nil {
		return nil, err
	}

	obj := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	opts := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(token),
		"interactive":  dbus.MakeVariant(false),
	}
	var handle dbus.ObjectPath
	if err := obj.Call("org.freedesktop.portal.Screenshot.Screenshot", 0, "", opts).Store(&handle); err != nil {
		return nil, err
	}

	timeout := time.After(60 * time.Second)
	for {
		select {
		case s := <-sig:
			if s.Path != handle || len(s.Body) < 2 {
				continue
			}
			code, _ := s.Body[0].(uint32)
			if code != 0 {
				return nil, fmt.Errorf("screenshot cancelled (code %d)", code)
			}
			results, _ := s.Body[1].(map[string]dbus.Variant)
			uriV, ok := results["uri"]
			if !ok {
				return nil, errors.New("portal returned no uri")
			}
			u, err := url.Parse(uriV.Value().(string))
			if err != nil {
				return nil, err
			}
			img, err := loadImage(u.Path)
			_ = os.Remove(u.Path)
			return img, err
		case <-timeout:
			return nil, errors.New("screenshot portal timed out")
		}
	}
}

func loadImage(path string) (image.Image, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}
