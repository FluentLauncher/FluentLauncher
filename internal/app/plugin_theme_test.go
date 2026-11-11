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
