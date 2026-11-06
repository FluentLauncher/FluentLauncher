package settings

import (
	"encoding/json"
	"testing"
)

func TestDefaultValidates(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatal(err)
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
