package capture

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Camera capture is delegated to ffmpeg: it is the only realistic way to read
// webcams on Linux, Windows and macOS from Go without per-platform C bindings.

type Camera struct {
	ID   string
	Name string
}

var ErrNoFFmpeg = errors.New("ffmpeg not found: install it or place it next to the Multi2FA executable to use the camera")

func FFmpegPath() (string, error) {
	name := "ffmpeg"
	if runtime.GOOS == "windows" {
		name = "ffmpeg.exe"
	}
	if exe, err := os.Executable(); err == nil {
		local := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(local); err == nil {
			return local, nil
		}
	}
	for _, dir := range extraFFmpegDirs() {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	return "", ErrNoFFmpeg
}

func ffmpegCommand(ctx context.Context, args ...string) (*exec.Cmd, error) {
	bin, err := FFmpegPath()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	hideConsole(cmd)
	return cmd, nil
}

type Stream struct {
	Frames <-chan image.Image
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	err    error
	stderr bytes.Buffer
}

// OpenCamera starts streaming low-rate JPEG frames from the camera.
func OpenCamera(cam Camera) (*Stream, error) {
	ctx, cancel := context.WithCancel(context.Background())
	args := []string{"-hide_banner", "-loglevel", "error"}
	args = append(args, inputArgs(cam)...)
	args = append(args,
		"-an",
		"-vf", "fps=8,scale='min(960,iw)':-2",
		"-f", "image2pipe", "-c:v", "mjpeg", "-q:v", "4", "-",
	)
	cmd, err := ffmpegCommand(ctx, args...)
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	frames := make(chan image.Image, 1)
	s := &Stream{Frames: frames, cancel: cancel, done: make(chan struct{})}
	cmd.Stderr = &limitedWriter{buf: &s.stderr, max: 4096}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	go func() {
		defer close(s.done)
		defer close(frames)
		splitJPEG(stdout, func(b []byte) {
			img, err := jpeg.Decode(bytes.NewReader(b))
			if err != nil {
				return
			}
			select {
			case frames <- img:
			default:
				select {
				case <-frames:
				default:
				}
				frames <- img
			}
		})
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			s.mu.Lock()
			msg := strings.TrimSpace(s.stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			s.err = errors.New(msg)
			s.mu.Unlock()
		}
	}()
	return s, nil
}

func (s *Stream) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *Stream) Close() {
	s.cancel()
	<-s.done
}

func splitJPEG(r io.Reader, emit func([]byte)) {
	br := bufio.NewReaderSize(r, 1<<20)
	var buf []byte
	var prev byte
	inFrame := false
	for {
		b, err := br.ReadByte()
		if err != nil {
			return
		}
		if !inFrame {
			if prev == 0xFF && b == 0xD8 {
				inFrame = true
				buf = append(make([]byte, 0, 128*1024), 0xFF, 0xD8)
			}
		} else {
			buf = append(buf, b)
			if prev == 0xFF && b == 0xD9 {
				emit(buf)
				inFrame = false
				b = 0
			}
			if len(buf) > 32<<20 {
				inFrame = false
			}
		}
		prev = b
	}
}

type limitedWriter struct {
	buf *bytes.Buffer
	max int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if room := w.max - w.buf.Len(); room > 0 {
		w.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}
