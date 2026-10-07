// Command genicon renders the logo SVG and writes every icon format the builds need.
//
//	go run ./scripts/genicon                        uses assets/icon.svg
//	go run ./scripts/genicon -svg path/to/logo.svg  any other SVG
//
// Rendering uses rsvg-convert (librsvg).
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"image"
	"image/draw"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	xdraw "golang.org/x/image/draw"
)

const master = 2048

// renderSVG rasterizes an SVG at the master size with librsvg's command line tool.
func renderSVG(path string) *image.NRGBA {
	out, err := exec.Command("rsvg-convert", "-w", strconv.Itoa(master), "-h", strconv.Itoa(master), path).Output()
	if err != nil {
		log.Fatalf("rsvg-convert %s: %v (install librsvg)", path, err)
	}
	src, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		log.Fatal(err)
	}
	img := image.NewNRGBA(src.Bounds())
	draw.Draw(img, img.Bounds(), src, image.Point{}, draw.Src)
	return img
}

func resize(src image.Image, size int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

func encode(img image.Image) []byte {
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		log.Fatal(err)
	}
	return b.Bytes()
}

func write(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Println("wrote", path)
}

// ico stores PNG-compressed images, supported since Windows Vista.
func ico(src image.Image, sizes []int) []byte {
	var hdr, body bytes.Buffer
	binary.Write(&hdr, binary.LittleEndian, []uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for _, s := range sizes {
		data := encode(resize(src, s))
		dim := byte(s)
		if s >= 256 {
			dim = 0
		}
		hdr.Write([]byte{dim, dim, 0, 0})
		binary.Write(&hdr, binary.LittleEndian, []uint16{1, 32})
		binary.Write(&hdr, binary.LittleEndian, []uint32{uint32(len(data)), uint32(offset)})
		body.Write(data)
		offset += len(data)
	}
	return append(hdr.Bytes(), body.Bytes()...)
}

func icns(src image.Image) []byte {
	entries := []struct {
		typ  string
		size int
	}{{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024}}
	var body bytes.Buffer
	for _, e := range entries {
		data := encode(resize(src, e.size))
		body.WriteString(e.typ)
		binary.Write(&body, binary.BigEndian, uint32(8+len(data)))
		body.Write(data)
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

func main() {
	svg := flag.String("svg", "assets/icon.svg", "logo to render (needs rsvg-convert)")
	flag.Parse()
	img := renderSVG(*svg)
	big := resize(img, 1024)
	write("internal/assets/icon.png", encode(resize(img, 512)))
	write("assets/icon.png", encode(big))
	for _, s := range []int{16, 24, 32, 48, 64, 128, 256, 512} {
		write(filepath.Join("assets", "hicolor", strconv.Itoa(s)+"x"+strconv.Itoa(s), "apps", "multi2fa.png"), encode(resize(img, s)))
	}
	write("assets/icon.ico", ico(img, []int{16, 24, 32, 48, 64, 128, 256}))
	write("assets/icon.icns", icns(img))
}
