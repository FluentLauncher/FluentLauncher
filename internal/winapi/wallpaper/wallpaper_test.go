package wallpaper

import (
	"image"
	"image/color"
	"testing"
)

func TestDiffuseKeepsSpatialLightWithoutImageDetails(t *testing.T) {
	src := image.NewNRGBA(image.Rect(10, 20, 210, 120))
	for y := 20; y < 120; y++ {
		for x := 10; x < 210; x++ {
			c := color.NRGBA{R: 40, G: 80, B: 50, A: 255}
			if x >= 110 {
				c = color.NRGBA{R: 180, G: 210, B: 220, A: 255}
			}
			src.SetNRGBA(x, y, c)
		}
	}
	texture, ok := diffuse(src)
	if !ok || texture.Bounds() != image.Rect(0, 0, 64, 40) {
		t.Fatal("no bounded diffuse texture")
	}
	left, middle, right := texture.NRGBAAt(5, 20), texture.NRGBAAt(32, 20), texture.NRGBAAt(58, 20)
	if !(left.R < middle.R && middle.R < right.R) {
		t.Fatalf("spatial light was flattened or the sharp edge retained: %+v %+v %+v", left, middle, right)
	}
	if _, ok := diffuse(image.NewNRGBA(image.Rect(0, 0, 2, 2))); ok {
		t.Fatal("transparent wallpaper should use the theme fallback")
	}
	tiny := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	tiny.SetNRGBA(0, 0, color.NRGBA{R: 100, G: 120, B: 140, A: 255})
	if _, ok := diffuse(tiny); !ok {
		t.Fatal("single-color wallpaper should remain usable")
	}
}
