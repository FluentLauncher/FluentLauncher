// Command icons prepares the runtime icons from the original brand artwork.
package main

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
)

func main() {
	source := flag.String("source", "assets/brand/source.png", "original brand PNG")
	out := flag.String("out", "internal/bundled", "generated runtime asset directory")
	flag.Parse()
	data, err := os.ReadFile(*source)
	if err != nil {
		log.Fatal(err)
	}
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		log.Fatal(err)
	}
	icon := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	draw.CatmullRom.Scale(icon, icon.Bounds(), src, src.Bounds(), draw.Src, nil)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, icon); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*out, 0755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(*out, "icon.png"), encoded.Bytes(), 0644); err != nil {
		log.Fatal(err)
	}
	tray := prepareTrayIcon(data, 48)
	if len(tray) == 0 {
		log.Fatal("failed to prepare tray icon")
	}
	if err := os.WriteFile(filepath.Join(*out, "tray.png"), tray, 0644); err != nil {
		log.Fatal(err)
	}
}
func prepareTrayIcon(data []byte, size int) []byte {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		log.Printf("brand icon: %v", err)
		return nil
	}
	bounds := src.Bounds()
	scaled := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), src, bounds, draw.Over, nil)
	clearBorderBackground(scaled)
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, scaled); err != nil {
		log.Printf("brand icon: %v", err)
		return nil
	}
	return buffer.Bytes()
}

// clearBorderBackground floods the near-white pixels connected to the
// image border and makes them transparent; the white glyph inside the
// blue square is not connected to the border and stays.
func clearBorderBackground(img *image.NRGBA) {
	b := img.Bounds()
	nearlyWhite := func(x, y int) bool {
		r, g, bl, a := img.At(x, y).RGBA()
		if a < 0xffff/2 {
			return false
		}
		return r > 0xe800 && g > 0xe800 && bl > 0xe800
	}
	visited := make([]bool, b.Max.X*b.Max.Y)
	queue := make([]image.Point, 0, 64)
	push := func(x, y int) {
		if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
			return
		}
		i := y*b.Max.X + x
		if visited[i] || !nearlyWhite(x, y) {
			return
		}
		visited[i] = true
		queue = append(queue, image.Point{x, y})
	}
	for x := b.Min.X; x < b.Max.X; x++ {
		push(x, b.Min.Y)
		push(x, b.Max.Y-1)
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		push(b.Min.X, y)
		push(b.Max.X-1, y)
	}
	for len(queue) > 0 {
		p := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		img.SetNRGBA(p.X, p.Y, color.NRGBA{})
		push(p.X+1, p.Y)
		push(p.X-1, p.Y)
		push(p.X, p.Y+1)
		push(p.X, p.Y-1)
	}
	// Pixels blended between the background and the mark keep a pale rim;
	// fade them by how white they are so the edge stays clean on a dark
	// taskbar.
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			px := img.NRGBAAt(x, y)
			if px.A == 0 || visited[y*b.Max.X+x] {
				continue
			}
			edge := false
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if nx >= b.Min.X && ny >= b.Min.Y && nx < b.Max.X && ny < b.Max.Y && visited[ny*b.Max.X+nx] {
					edge = true
					break
				}
			}
			if !edge {
				continue
			}
			white := min(min(px.R, px.G), px.B)
			px.A = uint8(float64(px.A) * (1 - float64(white)/255))
			img.SetNRGBA(x, y, px)
		}
	}
}
