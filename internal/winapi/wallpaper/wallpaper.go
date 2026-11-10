// Package wallpaper diffuses the desktop wallpaper for an adjustable Mica-like
// background. No live window contents are captured.
package wallpaper

import (
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"unsafe"

	_ "golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"golang.org/x/sys/windows"
)

var procSystemParametersInfo = windows.NewLazySystemDLL("user32.dll").NewProc("SystemParametersInfoW")

// Diffuse returns a small, opaque spatial texture with image details removed.
// Call it from a worker, never a view.
func Diffuse() (*image.NRGBA, bool) {
	if err := procSystemParametersInfo.Find(); err != nil {
		return nil, false
	}
	var buffer [4096]uint16
	ok, _, _ := procSystemParametersInfo.Call(0x0073, uintptr(len(buffer)), uintptr(unsafe.Pointer(&buffer[0])), 0)
	if ok == 0 {
		return nil, false
	}
	file, err := os.Open(windows.UTF16ToString(buffer[:]))
	if err != nil {
		return nil, false
	}
	defer file.Close()
	src, _, err := image.Decode(file)
	if err != nil {
		return nil, false
	}
	return diffuse(src)
}

func diffuse(src image.Image) (*image.NRGBA, bool) {
	if src.Bounds().Empty() {
		return nil, false
	}
	small := image.NewNRGBA(image.Rect(0, 0, 16, 10))
	draw.BiLinear.Scale(small, small.Bounds(), src, src.Bounds(), draw.Src, nil)
	// Repeated small box filters discard fine edges while retaining broad
	// two-dimensional color/light distribution. Work is bounded by 16x10.
	for pass := 0; pass < 3; pass++ {
		blurred := image.NewNRGBA(small.Bounds())
		for y := 0; y < 10; y++ {
			for x := 0; x < 16; x++ {
				var r, g, b, weight uint32
				for dy := -2; dy <= 2; dy++ {
					for dx := -2; dx <= 2; dx++ {
						c := small.NRGBAAt(min(max(x+dx, 0), 15), min(max(y+dy, 0), 9))
						r += uint32(c.R) * uint32(c.A)
						g += uint32(c.G) * uint32(c.A)
						b += uint32(c.B) * uint32(c.A)
						weight += uint32(c.A)
					}
				}
				if weight != 0 {
					blurred.SetNRGBA(x, y, color.NRGBA{R: uint8(r / weight), G: uint8(g / weight), B: uint8(b / weight), A: 255})
				}
			}
		}
		small = blurred
	}
	texture := image.NewNRGBA(image.Rect(0, 0, 64, 40))
	draw.BiLinear.Scale(texture, texture.Bounds(), small, small.Bounds(), draw.Src, nil)
	if texture.NRGBAAt(32, 20).A == 0 {
		return nil, false
	}
	return texture, true
}
