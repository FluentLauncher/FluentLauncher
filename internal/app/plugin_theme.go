package app

import (
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/plugin/protocol"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func pluginTheme(value settings.ThemeSettings, dark bool, prefs mygo.ThemePreferences) protocol.Theme {
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}
	// Match the pinned MyGo Theme.follow policy before app overrides. Plugins
	// may start while every launcher window is hidden, without a UI Context.
	if a := prefs.Accent; a.A > 0 {
		t.Accent = ui.RGB(a.R, a.G, a.B)
		t.AccentText = ui.RGB(255, 255, 255)
		if uint8(0.2126*float32(a.R)+0.7152*float32(a.G)+0.0722*float32(a.B)+0.5) > 165 {
			t.AccentText = ui.RGB(24, 24, 27)
		}
	}
	if prefs.HighContrast {
		t.Border = t.Border.Mix(t.Text, 0.45)
		t.TextMuted = t.TextMuted.Mix(t.Text, 0.4)
	}
	applyThemeSettings(t, value)
	if t.Inverse.A == 0 {
		t.Inverse = t.Text
	}
	if t.InverseText.A == 0 {
		t.InverseText = t.Background
	}
	flatten := func(c ui.Color) uint32 {
		a := uint32(c.A)
		blend := func(v, b uint8) uint32 { return (uint32(v)*a + uint32(b)*(255-a) + 127) / 255 }
		return blend(c.R, t.Background.R)<<16 | blend(c.G, t.Background.G)<<8 | blend(c.B, t.Background.B)
	}
	return protocol.Theme{Dark: dark, Background: flatten(t.Background), Surface: flatten(t.Surface), SurfaceHover: flatten(t.SurfaceHover), Border: flatten(t.Border), Text: flatten(t.Text), TextMuted: flatten(t.TextMuted), Accent: flatten(t.Accent), AccentText: flatten(t.AccentText), Danger: flatten(t.Danger), Inverse: flatten(t.Inverse), InverseText: flatten(t.InverseText), Radius: t.Radius, Spacing: t.Spacing}
}

func (a *App) syncPluginTheme(value settings.ThemeSettings) {
	if a.plugins == nil {
		return
	}
	dark := mygo.Theme.IsDark()
	if value.Mode == settings.ThemeDark {
		dark = true
	}
	if value.Mode == settings.ThemeLight {
		dark = false
	}
	a.plugins.SetTheme(pluginTheme(value, dark, mygo.Theme.Preferences()))
}
