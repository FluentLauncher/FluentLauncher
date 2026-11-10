package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/settings"
	"fluentlauncher/locales"
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
			a.updates.available = &mygo.Update{Version: "1.2.3", Notes: "A specific release change"}
			tt.Frame()
			if !tt.HasText(language.install) || tt.HasText("A specific release change") {
				t.Fatal("new version is not offered")
			}
			if err := tt.Click(locales.CommonOpen()); err != nil {
				t.Fatal(err)
			}
			if !a.updates.notesOpen || !tt.HasText("A specific release change") {
				t.Fatal("release notes did not open in the dialog")
			}
			tt.Key(0, ui.KeyEscape)
			if a.updates.notesOpen || tt.HasText("A specific release change") {
				t.Fatal("Escape did not dismiss the release notes")
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

func TestUpdateChannelChoiceClearsOfferAndDisablesWhileBusy(t *testing.T) {
	for _, tag := range []string{"zh-Hans", "zh-Hant", "en"} {
		t.Run(tag, func(t *testing.T) {
			setCatalogLocale(t, tag)
			a := newLocalizedApp()
			a.settingsPage = pageOther
			a.updates.available = &mygo.Update{Version: "0.1.1", Notes: "Old channel offer"}
			tt := ui.NewTester(a.settingsView, 1180, 840)
			if err := tt.Click(locales.UpdateChannelNightly()); err != nil {
				t.Fatal(err)
			}
			if a.settingsDraft.UpdateChannel != settings.UpdateChannelNightly || a.updates.available != nil || tt.HasText(locales.UpdateInstall()) {
				t.Fatal("channel choice retained the previous channel's update")
			}
			// Until autosave applies the choice, checking must not query the
			// previous channel. A real check would require a live app window.
			_ = tt.Click(locales.UpdateCheck())
			if a.updates.busy {
				t.Fatal("checked the previous channel before the choice was saved")
			}
			a.settings.UpdateChannel = settings.UpdateChannelNightly
			tt.Frame()
			a.updates.busy = true
			tt.Frame()
			_ = tt.Click(locales.UpdateChannelStable())
			if a.settingsDraft.UpdateChannel != settings.UpdateChannelNightly {
				t.Fatal("channel selector changed during an update")
			}
		})
	}
}

func TestUpdateChannelKeyboardSelection(t *testing.T) {
	setCatalogLocale(t, "en")
	a := newLocalizedApp()
	a.settingsPage = pageOther
	tt := ui.NewTester(a.settingsView, 1180, 840)
	if err := tt.Click(locales.UpdateChannelStable()); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyRight)
	if a.settingsDraft.UpdateChannel != settings.UpdateChannelNightly {
		t.Fatal("keyboard selection was not saved to the draft")
	}
	tt.Frame()
	if a.updates.channelIndex != 1 {
		t.Fatal("channel selection was lost during the next frame")
	}
	tt.Key(0, ui.KeyLeft)
	if a.settingsDraft.UpdateChannel != settings.UpdateChannelStable {
		t.Fatal("keyboard selection could not return to stable")
	}
}

func TestUpdateNotesLongContentAndMissingNotes(t *testing.T) {
	setCatalogLocale(t, "en")
	a := newLocalizedApp()
	a.settingsPage = pageOther
	a.updates.available = &mygo.Update{Version: "0.1.1", Notes: "## Improvements\n\n" + strings.Repeat("- A long update entry with details about this release.\n", 70) + "\nEnd of release notes."}
	tt := ui.NewTester(a.settingsView, 820, 660)
	if err := tt.Click(locales.CommonOpen()); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{locales.UpdateToVersion("0.1.1"), locales.UpdateLater(), locales.UpdateInstall()} {
		bounds, ok := tt.Find(text)
		if !ok || bounds.Y < 0 || bounds.Y+bounds.H > 660 {
			t.Fatalf("long notes pushed %q out of the dialog", text)
		}
	}
	tt.Scroll(410, 300, 0, 20000)
	last, ok := tt.Find("End of release notes.")
	if !ok || last.Y < 0 || last.Y+last.H > 600 {
		t.Fatal("long release notes could not scroll to the end")
	}
	a.updates.busy = true
	a.updates.installing = true
	a.updates.progress = 42
	a.updates.message = locales.UpdateDownloading(42)
	tt.Frame()
	later, ok := tt.Find(locales.UpdateLater())
	if !ok || later.Y+later.H > 660 || !tt.HasText(locales.UpdateDownloading(42)) {
		t.Fatal("download progress pushed the dialog's actions out of view")
	}
	if err := tt.Click(locales.UpdateLater()); err != nil {
		t.Fatal(err)
	}
	if a.updates.notesOpen {
		t.Fatal("Later did not close the dialog")
	}
	a.updates.available.Notes = ""
	tt.Frame()
	_ = tt.Click(locales.CommonOpen())
	if a.updates.notesOpen || !tt.HasText(locales.UpdateNotesEmpty()) {
		t.Fatal("an empty notes dialog was offered")
	}
}

// Export the real update card in both themes and all supported languages,
// without opening a native window or accessing personal settings.
func TestUpdateCardRender(t *testing.T) {
	dir := os.Getenv("FL_TEST_RENDER_DIR")
	if dir == "" {
		t.Skip("set FL_TEST_RENDER_DIR to export UI previews")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	previousVersion := mygo.App.Version()
	mygo.App.SetVersion("0.1.0")
	t.Cleanup(func() { mygo.App.SetVersion(previousVersion) })
	for _, tag := range []string{"zh-Hans", "zh-Hant", "en"} {
		setCatalogLocale(t, tag)
		for _, dark := range []bool{false, true} {
			for _, width := range []int{820, 1180} {
				mygo.App.SetVersion("0.1.0")
				if tag == "en" && width == 820 {
					mygo.App.SetVersion("0.1.1-nightly.20261110.123.1")
				}
				a := newLocalizedApp()
				a.settingsPage = pageOther
				a.settings.Theme.Background = settings.BackgroundSolid
				a.settings.Theme.Opacity = 1
				a.settings.UpdateChannel = settings.UpdateChannelNightly
				a.settingsDraft = a.settings
				notes := "## New features\n\n- Choose stable or preview updates.\n- Publish nightly builds automatically.\n- Publish stable releases with one click.\n\n## Improvements\n\n- Read release notes in a separate dialog.\n- See download progress while updating.\n- Use the update controls with the keyboard.\n\n## Fixes\n\n- Clear the previous offer when switching channels.\n- Keep personal data when updating the app."
				if tag != "en" {
					notes = "## 新功能\n\n- 增加稳定版与预览版更新通道。\n- 推送代码后自动生成独立 nightly 构建。\n- 支持一键递增版本并发布正式版。\n\n## 改进\n\n- 更新内容在独立窗口内排版，长说明可以滚动阅读。\n- 下载时显示进度，并禁止重复安装。\n- 更新通道支持键盘选择，跟随应用主题。\n\n## 修复\n\n- 切换通道后清除旧版本提示。\n- 更新程序时保留个人配置、数据库与已安装插件。"
				}
				a.updates.available = &mygo.Update{Version: "0.1.1-nightly.20261110.124.1", Notes: notes}
				for _, dialog := range []bool{false, true} {
					a.updates.notesOpen = dialog
					image := ui.Render(func(c *ui.Context) {
						theme := ui.LightTheme()
						if dark {
							theme = ui.DarkTheme()
						}
						c.SetTheme(theme)
						a.settingsView(c)
					}, width, 840, 1)
					path := filepath.Join(dir, fmt.Sprintf("updates-%s-dark-%t-%d-dialog-%t.png", tag, dark, width, dialog))
					file, err := os.Create(path)
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(file, image)
					closeErr := file.Close()
					if err != nil || closeErr != nil {
						t.Fatalf("render: %v, close: %v", err, closeErr)
					}
				}
			}
		}
	}
}
