package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/settings"
	"fluentlauncher/locales"
)

func TestBackdropMaterialsStayDistinct(t *testing.T) {
	for _, mode := range []settings.BackgroundMode{
		settings.BackgroundAcrylic, settings.BackgroundMica, settings.BackgroundSolid,
		settings.BackgroundGradient, settings.BackgroundImage,
	} {
		value := settings.ThemeSettings{Background: mode}
		material := backdropMaterial(value)
		if mode == settings.BackgroundMica {
			if material != mygo.VibrancyMica || blursBehind(value) {
				t.Fatal("mica must use the native material without live desktop blur")
			}
		} else if material != mygo.VibrancyNone || blursBehind(value) != (mode == settings.BackgroundAcrylic) {
			t.Fatalf("unexpected backdrop for %s: %s, blur=%t", mode, material, blursBehind(value))
		}
	}
}

func TestMicaTintAndFallback(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, opacity := range []float64{0, 0.55, 1} {
			t.Run(fmt.Sprintf("native=%t/opacity=%g", native, opacity), func(t *testing.T) {
				a := newLocalizedApp()
				a.settings.Theme.Background = settings.BackgroundMica
				a.settings.Theme.BackgroundColor = "#80336699"
				a.settings.Theme.Opacity = opacity
				tt := ui.NewTester(func(c *ui.Context) {
					theme := *c.Theme()
					a.applyUITheme(&theme)
					c.SetTheme(&theme)
					ui.Grid(c).Fill().Children(func() { a.drawBackground(c, &theme) })
				}, 120, 80)
				tt.SetVibrancy(native)
				pixel := tt.Image().RGBAAt(50, 40)
				wantAlpha := uint8(255)
				if native {
					wantAlpha = uint8(opacity*255 + 0.5)
				}
				if pixel.A != wantAlpha {
					t.Fatalf("tint/fallback alpha = %d, want %d", pixel.A, wantAlpha)
				}
				if opacity == 1 && (pixel.R != 0x33 || pixel.G != 0x66 || pixel.B != 0x99) {
					t.Fatalf("custom background alpha bypassed the opacity slider: %+v", pixel)
				}
			})
		}
	}
}

func TestMicaSettingsExplainTint(t *testing.T) {
	for _, tag := range []string{"zh-Hans", "zh-Hant", "en"} {
		setCatalogLocale(t, tag)
		a := newLocalizedApp()
		a.settingsPage = pageAppearance
		a.settings.Theme.Background = settings.BackgroundMica
		a.settingsDraft = a.settings
		tt := ui.NewTester(a.settingsView, 1180, 840)
		if !tt.HasText(locales.SettingsBackgroundColorMicaHint()) || !tt.HasText(locales.SettingsOpacityMicaHint()) {
			t.Fatalf("%s: mica still has acrylic's descriptions", tag)
		}
	}
}

// These captures deliberately show the opaque fallback. A headless renderer
// cannot reproduce DWM's wallpaper-based Mica material.
func TestMicaFallbackPreview(t *testing.T) {
	dir := os.Getenv("FL_MATERIAL_ARTIFACT_DIR")
	if dir == "" {
		t.Skip("set FL_MATERIAL_ARTIFACT_DIR to render fallback previews")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		a := newLocalizedApp()
		a.settingsHover = -1
		a.settingsPage = pageAppearance
		a.settings.Theme.Background = settings.BackgroundMica
		a.settingsDraft = a.settings
		tt := ui.NewTester(a.settingsView, 1180, 840)
		tt.SetDark(dark)
		tt.Advance(time.Second)
		path := filepath.Join(dir, fmt.Sprintf("mica-fallback-dark-%t.png", dark))
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("write preview: encode=%v, close=%v", err, closeErr)
		}
	}
}
