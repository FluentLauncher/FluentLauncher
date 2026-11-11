package app

import (
	"fluentlauncher/internal/core/settings"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"image/color"
	"testing"
)

func TestPluginThemeUsesLauncherColorsAndOpaqueSurfaces(t *testing.T) {
	value := settings.DefaultTheme()
	light, dark := pluginTheme(value, false, mygo.ThemePreferences{}), pluginTheme(value, true, mygo.ThemePreferences{})
	if light.Dark || !dark.Dark || light.Background == dark.Background || light.Text == dark.Text {
		t.Fatal("appearance not reflected in plugin palette")
	}
	value.BackgroundColor = "#123456"
	value.ForegroundColor = "#FEDCBA"
	value.AccentColor = "#007766"
	value.CornerRadius = 12
	custom := pluginTheme(value, true, mygo.ThemePreferences{})
	if custom.Background != 0x123456 || custom.Text != 0xFEDCBA || custom.Accent != 0x007766 || custom.Radius != 12 {
		t.Fatalf("custom theme lost: %+v", custom)
	}
	if custom.Surface == dark.Surface || custom.Surface == custom.Background {
		t.Fatal("surface not composited onto launcher background")
	}
	value.Background = settings.BackgroundMica
	if mica := pluginTheme(value, true, mygo.ThemePreferences{}); mica.Background == 0x123456 {
		t.Fatal("native mica incorrectly tinted by custom background")
	}
}

func TestPluginPaletteMatchesMyGoDesktopPreferences(t *testing.T) {
	value := settings.DefaultTheme()
	var actual ui.Theme
	tt := ui.NewTester(func(c *ui.Context) { actual = *c.Theme(); applyThemeSettings(&actual, value) }, 200, 100)
	for _, dark := range []bool{false, true} {
		for _, accent := range []color.NRGBA{{R: 250, G: 204, B: 21, A: 255}, {R: 219, G: 39, B: 119, A: 255}} {
			tt.SetDark(dark)
			tt.SetPreferences(ui.Preferences{Accent: ui.RGB(accent.R, accent.G, accent.B), HighContrast: true})
			got := pluginTheme(value, dark, mygo.ThemePreferences{Accent: accent, HighContrast: true})
			rgb := func(c ui.Color) uint32 { return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B) }
			if got.Accent != rgb(actual.Accent) || got.AccentText != rgb(actual.AccentText) || got.TextMuted != rgb(actual.TextMuted) {
				t.Fatalf("plugin differs from MyGo desktop palette: %+v / %+v", got, actual)
			}
		}
	}
}

func TestPluginThemeCarriesSurfaceAppearance(t *testing.T) {
	v := settings.DefaultTheme()
	v.Opacity = 0.37
	theme := pluginTheme(v, false, mygo.ThemePreferences{})
	if theme.Backdrop != "acrylic" || theme.Opacity == nil || *theme.Opacity != float32(0.37) {
		t.Fatal("acrylic appearance lost", theme)
	}
	v.Background = settings.BackgroundMica
	theme = pluginTheme(v, true, mygo.ThemePreferences{})
	if theme.Backdrop != "mica" || *theme.Opacity != 1 {
		t.Fatal("mica appearance lost", theme)
	}
	theme = pluginTheme(v, true, mygo.ThemePreferences{HighContrast: true})
	if theme.Backdrop != "none" || *theme.Opacity != 1 {
		t.Fatal("high contrast should be opaque", theme)
	}
}

func TestPluginThemeCarriesGradientEndAndClearsIt(t *testing.T) {
	v := settings.DefaultTheme()
	v.Background = settings.BackgroundGradient
	v.BackgroundColor = "#112233"
	v.GradientColor = "#000000"
	v.Opacity = 0.28
	theme := pluginTheme(v, false, mygo.ThemePreferences{})
	if theme.Background != 0x112233 || theme.GradientEnd == nil || *theme.GradientEnd != 0 || *theme.Opacity != float32(0.28) {
		t.Fatal("gradient or opacity lost", theme)
	}
	v.GradientColor = "#AABBCC"
	theme = pluginTheme(v, true, mygo.ThemePreferences{})
	if theme.GradientEnd == nil || *theme.GradientEnd != 0xaabbcc {
		t.Fatal("gradient end update lost", theme)
	}
	if got := pluginTheme(v, true, mygo.ThemePreferences{HighContrast: true}); got.GradientEnd != nil {
		t.Fatal("high contrast retained gradient")
	}
	v.Background = settings.BackgroundSolid
	if got := pluginTheme(v, true, mygo.ThemePreferences{}); got.GradientEnd != nil {
		t.Fatal("switching to solid retained gradient")
	}
}
