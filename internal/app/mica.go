package app

import (
	"image"
	"image/color"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/winapi/wallpaper"
)

type micaState struct {
	wallpaper *image.NRGBA
	loadedAt  time.Time
	loading   bool
	surface   *ui.Bitmap
	key       micaSurfaceKey
}

type micaSurfaceKey struct {
	wallpaper *image.NRGBA
	tint      ui.Color
	text      ui.Color
	strength  float64
}

// Decode, downsample and diffuse wallpaper off the UI thread. Slider drags
// reuse its small spatial texture; opacity never rebuilds the bitmap.
func (a *App) refreshMicaWallpaper() {
	if a.panel == nil || a.mica.loading || time.Since(a.mica.loadedAt) < time.Minute {
		return
	}
	a.mica.loading = true
	go func() {
		texture, _ := wallpaper.Diffuse()
		a.panel.Update(func() {
			a.mica.loading = false
			a.mica.loadedAt = time.Now()
			a.mica.wallpaper = texture
			if a.settingsWin != nil {
				a.settingsWin.Invalidate()
			}
			if a.workspaceWin != nil {
				a.workspaceWin.Invalidate()
			}
		})
	}()
}

// At most 64x40 pixels are toned when a material parameter changes. Keep
// brightness tied to the theme and retain spatial light/chroma variation,
// rather than mixing raw dark wallpaper colors directly into a light UI.
func (a *App) micaSurface(theme *ui.Theme, value settings.ThemeSettings) *ui.Bitmap {
	if value.Background != settings.BackgroundMica || a.mica.wallpaper == nil {
		return nil
	}
	tint := theme.Background
	if parsed, ok := parseOpaqueColor(value.BackgroundColor); ok {
		tint = parsed
	}
	key := micaSurfaceKey{a.mica.wallpaper, tint, theme.Text, value.MicaStrength}
	if a.mica.surface != nil && a.mica.key == key {
		return a.mica.surface
	}
	img := image.NewNRGBA(a.mica.wallpaper.Bounds())
	baseLuma := 0.2126*float64(tint.R) + 0.7152*float64(tint.G) + 0.0722*float64(tint.B)
	targetLuma := baseLuma
	if value.BackgroundColor == "" {
		// Derive the material's neutral lift/shade from the active theme,
		// including custom foregrounds, rather than fixing light/dark colors.
		textLuma := 0.2126*float64(theme.Text.R) + 0.7152*float64(theme.Text.G) + 0.0722*float64(theme.Text.B)
		targetLuma += (textLuma - baseLuma) * 0.05
	}
	strength := clampFloat(value.MicaStrength, 0, 1)
	channel := func(base uint8, shift float64) uint8 {
		return uint8(math.Round(clampFloat(float64(base)+strength*shift, 0, 255)))
	}
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			src := a.mica.wallpaper.NRGBAAt(x, y)
			luma := 0.2126*float64(src.R) + 0.7152*float64(src.G) + 0.0722*float64(src.B)
			shade := targetLuma - baseLuma + (luma-128)*0.12
			img.SetNRGBA(x, y, color.NRGBA{
				R: channel(tint.R, shade+(float64(src.R)-luma)*0.18),
				G: channel(tint.G, shade+(float64(src.G)-luma)*0.18),
				B: channel(tint.B, shade+(float64(src.B)-luma)*0.18), A: 255,
			})
		}
	}
	a.mica.key, a.mica.surface = key, ui.NewBitmap(img)
	return a.mica.surface
}
