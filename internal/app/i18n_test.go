package app

import (
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
)

// setCatalogLocale switches the language the catalog messages speak. The
// generated message functions read i18n.Locale(), which in the running app
// is what mygo.App.SetLocale sets; a test has no main loop for SetLocale to
// apply on, so it pins the hook directly. Clearing it returns "" — the
// catalog's source language, zh-Hans.
func setCatalogLocale(t *testing.T, tag string) {
	t.Helper()
	i18n.SetLocaleFunc(func() string { return tag })
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
}

func newLocalizedApp() *App {
	return &App{
		settings:      settings.Default(),
		settingsDraft: settings.Default(),
		profiles:      map[string]model.Profile{},
	}
}

// TestSettingsViewFollowsLocale proves the settings window speaks the app's
// language: the catalog messages are fetched at build time, so a locale the
// tester sets shows up in the very next frame, without a restart.
func TestSettingsViewFollowsLocale(t *testing.T) {
	setCatalogLocale(t, "zh-Hans")
	tt := ui.NewTester(newLocalizedApp().settingsView, 1180, 900)
	if !tt.HasText("界面语言") {
		t.Fatalf("source language does not show the language row: %q", tt.Texts())
	}

	setCatalogLocale(t, "en")
	tt = ui.NewTester(newLocalizedApp().settingsView, 1180, 900)
	if !tt.HasText("Language") {
		t.Fatal("the English locale does not show the language row")
	}
	if tt.HasText("界面语言") {
		t.Fatal("the English locale still shows Chinese")
	}
	for _, want := range []string{"General", "Appearance"} {
		if !tt.HasText(want) {
			t.Fatalf("English sidebar lacks %q: %q", want, tt.Texts())
		}
	}

	setCatalogLocale(t, "zh-Hant")
	tt = ui.NewTester(newLocalizedApp().settingsView, 1180, 900)
	if !tt.HasText("介面語言") {
		t.Fatal("Traditional Chinese does not show the language row")
	}
	// Each language names itself, whatever the interface language is; the
	// options live in the dropdown, so open it first.
	if err := tt.Click("跟隨系統"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("简体中文") || !tt.HasText("繁體中文") || !tt.HasText("English") {
		t.Fatalf("the language dropdown lost an autonym: %q", tt.Texts())
	}
}

// TestLanguageSettingValidates covers the persisted tag: "" follows the
// system, the three catalog languages are accepted, anything else is not.
func TestLanguageSettingValidates(t *testing.T) {
	s := settings.Default()
	for _, tag := range []string{"", "zh-Hans", "zh-Hant", "en"} {
		s.Language = tag
		if err := s.Validate(); err != nil {
			t.Fatalf("Language %q: %v", tag, err)
		}
	}
	s.Language = "fr"
	if err := s.Validate(); err == nil {
		t.Fatal("an unsupported language was accepted")
	}
}
