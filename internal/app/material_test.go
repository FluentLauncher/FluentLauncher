package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/locales"
)

func TestBackdropMaterialsStayDistinct(t *testing.T) {
	for _, mode := range []settings.BackgroundMode{
		settings.BackgroundAcrylic, settings.BackgroundMica, settings.BackgroundSolid,
		settings.BackgroundGradient, settings.BackgroundImage,
	} {
		value := settings.ThemeSettings{Background: mode}
		if blursBehind(value) != (mode == settings.BackgroundAcrylic) {
			t.Fatalf("unexpected live desktop blur for %s", mode)
		}
	}
}

func TestMicaBackgroundOpacity(t *testing.T) {
	for _, wallpaper := range []bool{false, true} {
		for _, opacity := range []float64{0, 0.55, 1} {
			t.Run(fmt.Sprintf("wallpaper=%t/opacity=%g", wallpaper, opacity), func(t *testing.T) {
				a := newLocalizedApp()
				if wallpaper {
					a.micaPalette = [2]ui.Color{ui.RGB(80, 120, 200), ui.RGB(180, 100, 60)}
				}
				a.settings.Theme.Background = settings.BackgroundMica
				a.settings.Theme.BackgroundColor = "#80336699"
				a.settings.Theme.Opacity = opacity
				tt := ui.NewTester(func(c *ui.Context) {
					theme := *c.Theme()
					a.applyUITheme(&theme)
					c.SetTheme(&theme)
					ui.Grid(c).Fill().Children(func() { a.drawBackground(c, &theme) })
				}, 120, 80)
				pixel := tt.Image().RGBAAt(50, 40)
				wantAlpha := uint8(opacity*255 + 0.5)
				if pixel.A != wantAlpha {
					t.Fatalf("background alpha = %d, want %d", pixel.A, wantAlpha)
				}
				if !wallpaper && opacity == 1 && (pixel.R != 0x33 || pixel.G != 0x66 || pixel.B != 0x99) {
					t.Fatalf("custom background alpha bypassed the opacity slider: %+v", pixel)
				}
			})
		}
	}
}

func TestMicaOpacityKeepsTextOpaque(t *testing.T) {
	a := newLocalizedApp()
	a.settings.Theme.Background = settings.BackgroundMica
	a.settings.Theme.Opacity = 0
	a.results = []model.Result{{Entry: model.Entry{ID: "opacity", Title: "Opaque text", Kind: model.Application}}}
	tt := ui.NewTester(a.view, 760, 560)
	if alpha := tt.Image().RGBAAt(400, 400).A; alpha != 0 {
		t.Fatalf("zero opacity left an opaque material behind the panel: %d", alpha)
	}
	r, ok := tt.Find("Opaque text")
	if !ok {
		t.Fatal("result title missing")
	}
	for y := int(r.Y); y < int(r.Y+r.H); y++ {
		for x := int(r.X); x < int(r.X+r.W); x++ {
			if tt.Image().RGBAAt(x, y).A == 255 {
				return
			}
		}
	}
	t.Fatal("background opacity faded the result text")
}

func TestMicaStrengthDoesNotChangeOpacity(t *testing.T) {
	for _, opacity := range []float64{0.55, 1} {
		a := newLocalizedApp()
		a.settings.Theme.Background = settings.BackgroundMica
		a.settings.Theme.Opacity = opacity
		a.settings.Theme.MicaStrength = 0
		a.micaPalette = [2]ui.Color{ui.RGB(40, 150, 70), ui.RGB(30, 60, 140)}
		tt := ui.NewTester(func(c *ui.Context) {
			theme := *c.Theme()
			a.applyUITheme(&theme)
			c.SetTheme(&theme)
			ui.Grid(c).Fill().Children(func() { a.drawBackground(c, &theme) })
		}, 120, 80)
		plain := tt.Image().RGBAAt(50, 40)
		a.settings.Theme.MicaStrength = 1
		tt.Frame()
		mica := tt.Image().RGBAAt(50, 40)
		if plain.A != mica.A || plain == mica {
			t.Fatalf("strength must change color while preserving alpha: plain=%+v mica=%+v", plain, mica)
		}
		if tt.Image().RGBAAt(50, 10) == tt.Image().RGBAAt(50, 70) {
			t.Fatal("full opacity erased the wallpaper-derived mica variation")
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
		if !tt.HasText(locales.SettingsBackgroundColorMicaHint()) || !tt.HasText(locales.SettingsOpacityMicaHint()) ||
			!tt.HasText(locales.SettingsMicaStrength()) || !tt.HasText(locales.SettingsBackgroundOpacity()) {
			t.Fatalf("%s: mica still has acrylic's descriptions", tag)
		}
	}
}

// Synthetic wallpaper colors keep previews reproducible without user files.
func TestMicaPreview(t *testing.T) {
	dir := os.Getenv("FL_MATERIAL_ARTIFACT_DIR")
	if dir == "" {
		t.Skip("set FL_MATERIAL_ARTIFACT_DIR to render mica previews")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		for _, opacity := range []float64{0.25, 0.55, 1} {
			a := newLocalizedApp()
			a.settingsHover = -1
			a.settingsPage = pageAppearance
			a.settings.Theme.Background = settings.BackgroundMica
			a.settings.Theme.Opacity = opacity
			a.micaPalette = [2]ui.Color{ui.RGB(80, 120, 200), ui.RGB(180, 100, 60)}
			a.settingsDraft = a.settings
			tt := ui.NewTester(a.settingsView, 1180, 840)
			tt.SetDark(dark)
			tt.Advance(time.Second)
			path := filepath.Join(dir, fmt.Sprintf("mica-dark-%t-opacity-%.2f.png", dark, opacity))
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
}
