package app

import (
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/locales"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestBackdropMaterialsStayDistinct(t *testing.T) {
	for _, mode := range []settings.BackgroundMode{settings.BackgroundAcrylic, settings.BackgroundMica, settings.BackgroundSolid, settings.BackgroundGradient, settings.BackgroundImage} {
		value := settings.ThemeSettings{Background: mode}
		want := mygo.VibrancyNone
		if mode == settings.BackgroundMica {
			want = mygo.VibrancyMica
		}
		if backdropMaterial(value) != want || blursBehind(value) != (mode == settings.BackgroundAcrylic) {
			t.Fatalf("unexpected material/blur for %s", mode)
		}
	}
}

func TestNativeMicaHasNoTintOrOpacityOverlay(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, dark := range []bool{false, true} {
			for _, opacity := range []float64{0, 0.55, 1} {
				t.Run(fmt.Sprintf("native=%t/dark=%t/oldOpacity=%g", native, dark, opacity), func(t *testing.T) {
					a := newLocalizedApp()
					a.settings.Theme.Background = settings.BackgroundMica
					a.settings.Theme.BackgroundColor = "#FF0000"
					a.settings.Theme.Opacity = opacity
					var background, surface ui.Color
					tt := ui.NewTester(func(c *ui.Context) {
						theme := *c.Theme()
						a.applyUITheme(&theme)
						c.SetTheme(&theme)
						background, surface = theme.Background, theme.Surface
						ui.Grid(c).Fill().Children(func() { a.drawBackground(c, &theme) })
					}, 120, 80)
					tt.SetDark(dark)
					tt.SetVibrancy(native)
					pixel := tt.Image().RGBAAt(50, 40)
					if native {
						if pixel.A != 0 {
							t.Fatalf("app overlay covered native mica: %+v", pixel)
						}
					} else if pixel.A != 255 || pixel.R != background.R || pixel.G != background.G || pixel.B != background.B {
						t.Fatalf("fallback did not use the opaque theme base: %+v vs %+v", pixel, background)
					}
					if surface.A != ui.RGB(255, 255, 255).Alpha(0.8).A {
						t.Fatal("old opacity changed mica's control surfaces")
					}
					if background == ui.RGB(255, 0, 0) {
						t.Fatal("saved tint changed the native mica theme")
					}
				})
			}
		}
	}
}

func TestMicaSettingsHideAppearanceOverrides(t *testing.T) {
	for _, tag := range []string{"zh-Hans", "zh-Hant", "en"} {
		setCatalogLocale(t, tag)
		for _, width := range []int{820, 1180} {
			a := newLocalizedApp()
			a.settingsPage = pageAppearance
			a.settings.Theme.Background = settings.BackgroundMica
			a.settingsDraft = a.settings
			tt := ui.NewTester(a.settingsView, width, 840)
			if !tt.HasText(locales.SettingsMicaNativeHint()) {
				t.Fatal("native mica explanation missing")
			}
			for _, label := range []string{locales.SettingsOpacity(), locales.SettingsBackgroundColor(), "云母强度", "雲母強度", "Mica strength"} {
				if _, ok := tt.Find(label); ok {
					t.Fatalf("%s: mica still offers %s", tag, label)
				}
			}
			a.settingsDraft.Theme.Background = settings.BackgroundAcrylic
			a.settings.Theme = a.settingsDraft.Theme
			tt.Frame()
			if !tt.HasText(locales.SettingsOpacity()) || !tt.HasText(locales.SettingsBackgroundColor()) {
				t.Fatal("acrylic lost its appearance controls")
			}
		}
	}
}
