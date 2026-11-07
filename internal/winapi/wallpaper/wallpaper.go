// Package wallpaper reads the desktop wallpaper, whose colors tint the mica
// background: Windows' own mica draws the wallpaper behind a window, which a
// translucent window cannot, so the app takes its tint from the average color
// of the image instead.
package wallpaper

import (
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procSystemParametersInfo = windows.NewLazySystemDLL("user32.dll").NewProc("SystemParametersInfoW")

// SPI_GETDESKWALLPAPER copies the wallpaper's path into a buffer of UTF-16
// units.
const spiGetDesktopWallpaper = 0x0073

// Average returns the average color of the desktop wallpaper, or false when
// Windows reports no image wallpaper (a solid color, a slideshow, or a
// policy) or the file cannot be read.
func Average() (red, green, blue uint8, ok bool) {
	if err := procSystemParametersInfo.Find(); err != nil {
		return 0, 0, 0, false
	}
	var buffer [4096]uint16
	copied, _, _ := procSystemParametersInfo.Call(
		spiGetDesktopWallpaper,
		uintptr(len(buffer)),
		uintptr(unsafe.Pointer(&buffer[0])),
		0,
	)
	if copied == 0 {
		return 0, 0, 0, false
	}
	path := windows.UTF16ToString(buffer[:])
	if path == "" {
		return 0, 0, 0, false
	}
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, 0, false
	}
	defer file.Close()
	src, _, err := image.Decode(file)
	if err != nil {
		return 0, 0, 0, false
	}
	bounds := src.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return 0, 0, 0, false
	}
	// A few thousand samples are enough for an average, and cheap on a 4K
	// image: one pixel every step, at most 96 across.
	step := max(bounds.Dx(), bounds.Dy()) / 96
	if step < 1 {
		step = 1
	}
	var sumR, sumG, sumB, count uint64
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			r, g, b, a := src.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			sumR += uint64(r >> 8)
			sumG += uint64(g >> 8)
			sumB += uint64(b >> 8)
			count++
		}
	}
	if count == 0 {
		return 0, 0, 0, false
	}
	return uint8(sumR / count), uint8(sumG / count), uint8(sumB / count), true
}
