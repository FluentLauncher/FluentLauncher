package wallpaper

import (
	"image"
	"image/color"
	"testing"
)

func TestPaletteKeepsBroadWallpaperVariation(t *testing.T) {
	src := image.NewNRGBA(image.Rect(10, 20, 14, 24))
	upper, lower := color.NRGBA{R: 200, G: 40, B: 20, A: 255}, color.NRGBA{R: 20, G: 60, B: 180, A: 255}
	for y := 20; y < 24; y++ {
		for x := 10; x < 14; x++ {
			value := upper
			if y >= 22 {
				value = lower
			}
			src.SetNRGBA(x, y, value)
		}
	}
	colors, ok := samplePalette(src)
	if !ok || colors != [2]color.NRGBA{upper, lower} {
		t.Fatalf("palette lost upper/lower wallpaper colors: %+v, ok=%t", colors, ok)
	}
	tiny := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	tiny.SetNRGBA(0, 0, upper)
	colors, ok = samplePalette(tiny)
	if !ok || colors != [2]color.NRGBA{upper, upper} {
		t.Fatalf("single-pixel wallpaper has no usable palette: %+v", colors)
	}
	if _, ok := samplePalette(image.NewNRGBA(image.Rect(0, 0, 2, 2))); ok {
		t.Fatal("transparent wallpaper should use the theme fallback")
	}
}
