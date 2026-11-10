// Brand icons are prepared by scripts/icons; startup only selects their bytes.
package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"

	"github.com/egoist/mygo/ui"
)

var brand struct {
	original []byte
	tray     []byte
}

// SetBrandIcon stores the application logo PNG before Run.
func SetBrandIcon(pngData, trayData []byte) {
	brand.original = pngData
	brand.tray = trayData
}

// trayIconPNG is the logo scaled for the notification area, or nil when
// the logo is missing or undecodable.
func trayIconPNG() []byte {
	if brand.tray != nil {
		return brand.tray
	}
	return fallbackTrayIcon()
}

// windowIconPNG is the logo for title bars and the taskbar.
func windowIconPNG() []byte {
	if brand.original != nil {
		return brand.original
	}
	return trayIconPNG()
}

// brandBitmap decodes the logo once for display inside views.
func (a *App) brandBitmap() *ui.Bitmap {
	if a.brandIcon == nil && len(brand.original) > 0 {
		a.brandIcon, _ = ui.DecodeBitmap(brand.original)
	}
	return a.brandIcon
}

// fallbackTrayIcon draws a simple mark when the logo is unavailable.
func fallbackTrayIcon() []byte {
	const size = 32
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	ink := color.NRGBA{R: 79, G: 124, B: 255, A: 255}
	for y := range size {
		for x := range size {
			dx, dy := float64(x)-16, float64(y)-16
			if dx*dx+dy*dy <= 14*14 {
				img.Set(x, y, ink)
			}
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		log.Printf("tray icon: %v", err)
	}
	return buffer.Bytes()
}
