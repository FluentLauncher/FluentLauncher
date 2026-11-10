package app

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestUpdateCardBuildsAcrossStatesAndLocales(t *testing.T) {
	for _, language := range []struct{ tag, check, install string }{
		{"zh-Hans", "检查更新", "更新并重启"},
		{"zh-Hant", "檢查更新", "更新並重新啟動"},
		{"en", "Check for updates", "Update and restart"},
	} {
		t.Run(language.tag, func(t *testing.T) {
			setCatalogLocale(t, language.tag)
			a := newLocalizedApp()
			a.settingsPage = len(settingPages()) - 1
			tt := ui.NewTester(a.settingsView, 1180, 840)
			if !tt.HasText(language.check) || tt.HasText(language.install) {
				t.Fatal("idle update controls are incorrect")
			}
			a.updates.available = &mygo.Update{Version: "1.2.3", Notes: "Release notes"}
			tt.Frame()
			if !tt.HasText(language.install) || !tt.HasText("Release notes") {
				t.Fatal("new version is not offered")
			}
			a.updates.busy = true
			a.updates.message = "Downloading"
			tt.Frame()
			_ = tt.Click(language.check)
			_ = tt.Click(language.install)
			tt.Frame()
			if a.updates.message != "Downloading" {
				t.Fatal("a busy update accepted a second operation")
			}
			a.updates.available = nil
			a.updates.busy = false
			tt.Frame()
			if tt.HasText(language.install) {
				t.Fatal("install button survived removal of the update")
			}
		})
	}
}
