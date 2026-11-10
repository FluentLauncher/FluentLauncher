package settings

import (
	"encoding/json"
	"math"
	"testing"
)

func TestDefaultValidates(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestMicaStrengthDefaultsAndValidation(t *testing.T) {
	for _, data := range []string{`{}`, `{"Background":"Mica","Opacity":0.7}`} {
		var theme ThemeSettings
		if err := json.Unmarshal([]byte(data), &theme); err != nil {
			t.Fatal(err)
		}
		if theme.MicaStrength != DefaultTheme().MicaStrength {
			t.Fatalf("legacy theme lost mica default: %+v", theme)
		}
	}
	var theme ThemeSettings
	if err := json.Unmarshal([]byte(`{"MicaStrength":0}`), &theme); err != nil || theme.MicaStrength != 0 {
		t.Fatal("explicit zero mica strength was replaced by its default")
	}
	for _, value := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1)} {
		theme = DefaultTheme()
		theme.MicaStrength = value
		if ValidateTheme(theme) == nil {
			t.Fatalf("invalid mica strength %v accepted", value)
		}
	}
	theme = DefaultTheme()
	theme.MicaStrength, theme.Opacity = 0.8, 0.4
	data, err := json.Marshal(theme)
	if err != nil {
		t.Fatal(err)
	}
	var restored ThemeSettings
	if err := json.Unmarshal(data, &restored); err != nil || restored.MicaStrength != 0.8 || restored.Opacity != 0.4 {
		t.Fatal("mica strength and opacity did not persist independently")
	}
}

func TestMissingGroupsKeepDefaults(t *testing.T) {
	var s LauncherSettings
	if err := json.Unmarshal([]byte(`{"Hotkey":"Ctrl+Space","Width":800}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.Width != 800 || s.Height != 560 || s.MaxResults != 50 {
		t.Fatalf("unexpected sizes: %+v", s)
	}
	if s.Theme.FontSize != 17 || s.Theme.FontFamily != "Segoe UI Variable" {
		t.Fatalf("theme defaults lost: %+v", s.Theme)
	}
	if !s.Applications.IncludeStoreApps || !s.Files.IncludeDirectories {
		t.Fatalf("group defaults lost: %+v %+v", s.Applications, s.Files)
	}
	if s.VolumeWheel.Edges != EdgeBottom || s.VolumeWheel.ThicknessDip != 4 {
		t.Fatalf("volume defaults lost: %+v", s.VolumeWheel)
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyRecentField(t *testing.T) {
	var s LauncherSettings
	if err := json.Unmarshal([]byte(`{"ShowRecentSearches":true}`), &s); err != nil {
		t.Fatal(err)
	}
	if !s.ShowRecentEntries || s.ShowRecentSearches {
		t.Fatalf("legacy field not migrated: %+v", s)
	}
}

func TestInvalidSettings(t *testing.T) {
	s := Default()
	s.Width = 100
	if err := s.Validate(); err == nil {
		t.Fatal("expected width validation error")
	}
	s = Default()
	s.Theme.FontSize = 50
	if err := s.Validate(); err == nil {
		t.Fatal("expected theme validation error")
	}
	s = Default()
	s.Theme.SearchHeight = 200
	if err := s.Validate(); err == nil {
		t.Fatal("expected search height validation error")
	}
	s = Default()
	s.CustomItems = []CustomLaunchItem{{ID: "x", Name: "x", Target: `\\server\share`}}
	if err := s.Validate(); err == nil {
		t.Fatal("expected UNC target validation error")
	}
}

func TestThemePresets(t *testing.T) {
	if Preset("Compact").FontSize != 15 || Preset("Dark").Mode != ThemeDark {
		t.Fatal("presets changed")
	}
	if !ValidURLTemplate("https://example.com/?q={query}") {
		t.Fatal("valid template rejected")
	}
	if ValidURLTemplate("file:///c:/{query}") {
		t.Fatal("non-http template accepted")
	}
}

func TestAnchorDefaultsAndBounds(t *testing.T) {
	var s LauncherSettings
	if err := json.Unmarshal([]byte(`{"Width":800}`), &s); err != nil {
		t.Fatal(err)
	}
	if s.AnchorX != 0.5 || s.AnchorY != 0.45 {
		t.Fatalf("anchor defaults = %v,%v", s.AnchorX, s.AnchorY)
	}
	var off LauncherSettings
	if err := json.Unmarshal([]byte(`{"AnchorX":2,"AnchorY":-1}`), &off); err != nil {
		t.Fatal(err)
	}
	if off.AnchorX != 1 || off.AnchorY != 0 {
		t.Fatalf("anchor not clamped: %v,%v", off.AnchorX, off.AnchorY)
	}
	s = Default()
	s.AnchorX = 1.5
	if err := s.Validate(); err == nil {
		t.Fatal("expected anchor validation error")
	}
}
