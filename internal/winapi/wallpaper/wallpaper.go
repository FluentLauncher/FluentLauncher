// Package wallpaper samples the desktop wallpaper for an adjustable Mica-like
// background. No live window contents are captured.
package wallpaper

import (
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procSystemParametersInfo = windows.NewLazySystemDLL("user32.dll").NewProc("SystemParametersInfoW")

// Palette returns broad upper/lower wallpaper colors, or false when Windows
// has no readable image wallpaper. Call it from a worker, never a view.
func Palette() ([2]color.NRGBA, bool) {
	if err := procSystemParametersInfo.Find(); err != nil {
		return [2]color.NRGBA{}, false
	}
	var buffer [4096]uint16
	ok, _, _ := procSystemParametersInfo.Call(0x0073, uintptr(len(buffer)), uintptr(unsafe.Pointer(&buffer[0])), 0)
	if ok == 0 {
		return [2]color.NRGBA{}, false
	}
	file, err := os.Open(windows.UTF16ToString(buffer[:]))
	if err != nil {
		return [2]color.NRGBA{}, false
	}
	defer file.Close()
	src, _, err := image.Decode(file)
	if err != nil {
		return [2]color.NRGBA{}, false
	}
	return samplePalette(src)
}

func samplePalette(src image.Image) ([2]color.NRGBA, bool) {
	bounds := src.Bounds()
	step := max(max(bounds.Dx(), bounds.Dy())/96, 1)
	var sums [2][4]uint64
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		band := 0
		if y >= bounds.Min.Y+bounds.Dy()/2 {
			band = 1
		}
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			c := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			if c.A == 0 {
				continue
			}
			sums[band][0] += uint64(c.R)
			sums[band][1] += uint64(c.G)
			sums[band][2] += uint64(c.B)
			sums[band][3]++
		}
	}
	var colors [2]color.NRGBA
	for i, sum := range sums {
		if sum[3] == 0 {
			// Tiny or partially transparent images may fill only one band.
			sum = sums[1-i]
		}
		if sum[3] == 0 {
			return colors, false
		}
		colors[i] = color.NRGBA{R: uint8(sum[0] / sum[3]), G: uint8(sum[1] / sum[3]), B: uint8(sum[2] / sum[3]), A: 255}
	}
	return colors, true
}
