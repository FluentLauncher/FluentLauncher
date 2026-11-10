package settings

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestDefaultValidates(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateChannelDefaultsAndPersistence(t *testing.T) {
	for _, input := range []string{`{}`, `{"UpdateChannel":"stable"}`, `{"UpdateChannel":"nightly"}`} {
		var s LauncherSettings
		if err := json.Unmarshal([]byte(input), &s); err != nil {
			t.Fatal(err)
		}
		if err := s.Validate(); err != nil {
			t.Fatal(err)
		}
		for _, version := range []string{"0.1.0", "0.1.1-nightly.20261110.123.1"} {
			want := s.UpdateChannel
			if want == UpdateChannelDefault {
				want = UpdateChannelStable
				if version != "0.1.0" {
					want = UpdateChannelNightly
				}
			}
			if got := s.EffectiveUpdateChannel(version); got != want {
				t.Fatalf("%s on %s: got %s, want %s", input, version, got, want)
			}
		}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var restored LauncherSettings
		if err := json.Unmarshal(data, &restored); err != nil || restored.UpdateChannel != s.UpdateChannel {
			t.Fatalf("channel choice did not persist: %s, %v", data, err)
		}
	}
	s := Default()
	s.UpdateChannel = "unknown"
	if err := s.Validate(); err == nil {
		t.Fatal("unsupported update channel was accepted")
	}
}

func TestLegacyMicaControlsAreIgnored(t *testing.T) {
	var theme ThemeSettings
	if err := json.Unmarshal([]byte(`{"Background":"Mica","Opacity":0.4,"MicaStrength":0.8}`), &theme); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTheme(theme); err != nil {
		t.Fatal(err)
	}
	if theme.Opacity != 0.4 {
		t.Fatal("opacity for other materials was overwritten")
	}
	data, err := json.Marshal(theme)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("MicaStrength")) {
		t.Fatal("obsolete mica strength was persisted")
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
