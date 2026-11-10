package app

import (
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/shortcuts"
	"fluentlauncher/internal/plugin"
	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/locales"
	"github.com/egoist/mygo/ui"
)

func TestPluginShortcutCatalogAndDisabledRetention(t *testing.T) {
	a := newLocalizedApp()
	a.plugins = plugin.NewManager()
	t.Cleanup(a.plugins.Dispose)
	a.settings.Plugins.EnabledIDs = []string{"vendor.tool"}
	a.settingsDraft = cloneSettings(a.settings)
	manifest := protocol.Manifest{ID: "vendor.tool", Name: "Example plugin", Commands: []protocol.Command{{ID: "capture", Title: "Capture region", Scope: shortcuts.Global, Activation: "invoke", DefaultShortcuts: []string{"Ctrl+Alt+A"}}}}
	a.plugins.Configure([]protocol.Registration{{Manifest: manifest}}, a.settings.Plugins)
	const id = "plugin:vendor.tool:capture"
	if _, ok := a.desiredGlobalShortcuts(a.settings)["Ctrl+Alt+A"]; ok {
		t.Fatal("plugin recommendation automatically registered")
	}
	if !a.setShortcutBinding(id, shortcuts.Binding{Keys: []string{"Ctrl+Alt+A"}, Enabled: true}, false) {
		t.Fatal(a.shortcutEdit.Error)
	}
	a.settings = cloneSettings(a.settingsDraft)
	if a.desiredGlobalShortcuts(a.settings)["Ctrl+Alt+A"] != id {
		t.Fatal("enabled plugin command not registered")
	}
	a.settings.Plugins.EnabledIDs = nil
	if _, ok := a.desiredGlobalShortcuts(a.settings)["Ctrl+Alt+A"]; ok {
		t.Fatal("disabled plugin retained active hotkey")
	}
	if !a.settings.Shortcuts[id].Enabled {
		t.Fatal("plugin disable erased user binding")
	}
	a.settingsDraft = cloneSettings(a.settings)
	a.settingsPage = pageShortcuts
	a.shortcutFilter = "Example plugin"
	tt := ui.NewTester(a.settingsView, 820, 660)
	if !tt.HasText("Capture region") || !tt.HasText(locales.ShortcutsUnavailable()) {
		t.Fatal("plugin command or unavailable state missing")
	}
}

func TestGlobalFallbackSurvivesLocalEdits(t *testing.T) {
	a := newLocalizedApp()
	calls := 0
	a.globalShortcuts = shortcuts.Registry{Active: map[string]string{"Alt+Shift+Space": shortcuts.Toggle}, Register: func(key, id string) error { calls++; return errors.New("occupied") }, Unregister: func(string) { calls++ }}
	value := cloneSettings(a.settings)
	value.Shortcuts["search.next"] = shortcuts.Binding{Keys: []string{"Ctrl+J"}, Enabled: true}
	if err := a.syncGlobalShortcuts(value, true); err != nil || calls != 0 {
		t.Fatalf("local edit retried emergency opener: %d %v", calls, err)
	}
}

func TestFailedHotkeyEditRestoresDraft(t *testing.T) {
	a := newLocalizedApp()
	a.settings.Normalize()
	a.appliedTheme = a.settings.Theme
	a.settingsDraft = cloneSettings(a.settings)
	a.settingsDraft.Shortcuts[shortcuts.Toggle] = shortcuts.Binding{Keys: []string{"Ctrl+F7"}, Enabled: true}
	a.globalShortcuts = shortcuts.Registry{Active: map[string]string{"Alt+Space": shortcuts.Toggle}, Register: func(key, id string) error {
		if key == "Ctrl+F7" {
			return errors.New("occupied")
		}
		return nil
	}, Unregister: func(string) {}}
	a.flushSettingsDraft()
	if a.draftChanged() || a.settingsError == "" || a.globalShortcuts.Active["Alt+Space"] != shortcuts.Toggle {
		t.Fatalf("failed edit was retried or lost old binding: %s %+v", a.settingsError, a.globalShortcuts.Active)
	}
}

// Opt-in artifacts for visual review; normal test runs do not write images.
func TestShortcutsRender(t *testing.T) {
	dir := os.Getenv("FL_TEST_RENDER_DIR")
	if dir == "" {
		t.Skip("set FL_TEST_RENDER_DIR to export UI previews")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		for _, recording := range []bool{false, true} {
			a := newLocalizedApp()
			a.settingsPage = pageShortcuts
			a.globalShortcuts.Active = map[string]string{"Alt+Space": shortcuts.Toggle}
			if recording {
				a.beginShortcutEdit("search.actions", []string{"Ctrl+K", "Shift+F10", "Menu"})
				a.shortcutEdit.Recorded = "Ctrl+J"
			}
			name := "light"
			if dark {
				name = "dark"
			}
			if recording {
				name += "-recording"
			}
			for _, viewport := range []struct {
				width, height int
				scale         float32
				suffix        string
			}{
				{1180, 840, 1, ""}, {820, 660, 1, "-narrow"}, {1180, 840, 1.5, "-150"},
			} {
				image := ui.Render(func(c *ui.Context) {
					theme := ui.LightTheme()
					if dark {
						theme = ui.DarkTheme()
					}
					c.SetTheme(theme)
					a.settingsView(c)
				}, viewport.width, viewport.height, viewport.scale)
				file, err := os.Create(filepath.Join(dir, "shortcuts-"+name+viewport.suffix+".png"))
				if err != nil {
					t.Fatal(err)
				}
				err = png.Encode(file, image)
				closeErr := file.Close()
				if err != nil {
					t.Fatal(err)
				}
				if closeErr != nil {
					t.Fatal(closeErr)
				}
			}
		}
	}
}

func TestRemappedSearchKeysWithInputAndListFocus(t *testing.T) {
	for _, listFocus := range []bool{false, true} {
		a := newLocalizedApp()
		a.resultsReady = true
		a.settings.Shortcuts["search.next"] = shortcuts.Binding{Keys: []string{"Ctrl+J"}, Enabled: true}
		a.settings.Shortcuts["search.execute"] = shortcuts.Binding{Keys: []string{"Ctrl+Enter"}, Enabled: true}
		a.settings.Shortcuts["search.actions"] = shortcuts.Binding{Keys: []string{"Ctrl+L"}, Enabled: true}
		a.results = []model.Result{{Entry: model.Entry{ID: "one", Title: "One", Kind: model.Plugin}}, {Entry: model.Entry{ID: "two", Title: "Two", Kind: model.Plugin}}}
		focused := false
		tt := ui.NewTester(func(c *ui.Context) {
			a.view(c)
			if listFocus && !focused {
				a.list.Focus(c)
				focused = true
			}
		}, 760, 560)
		if !listFocus {
			a.input.Focus()
		}
		tt.Frame()
		tt.Key(0, ui.KeyDown)
		tt.Frame()
		if a.chosen != 0 {
			t.Fatalf("old Down still selected a row, listFocus=%v", listFocus)
		}
		tt.Key(ui.Ctrl, ui.KeyJ)
		tt.Frame()
		if a.chosen != 1 {
			t.Fatalf("new next key not applied, listFocus=%v", listFocus)
		}
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		if a.notice != "" {
			t.Fatalf("old Enter executed: %s", a.notice)
		}
		tt.Key(ui.Ctrl, ui.KeyEnter)
		tt.Frame()
		if a.notice == "" {
			t.Fatalf("new execute key did not execute: listFocus=%v chosen=%d query=%q", listFocus, a.chosen, a.query)
		}
		tt.Key(ui.Ctrl, ui.KeyK)
		tt.Frame()
		if a.actionsOpen {
			t.Fatal("old action key still opens panel")
		}
		tt.Key(ui.Ctrl, ui.KeyL)
		tt.Frame()
		if !a.actionsOpen {
			t.Fatal("new action key did not open panel")
		}
	}
}

func TestShortcutConflictReplacementKeepsOtherBindings(t *testing.T) {
	a := newLocalizedApp()
	a.settingsDraft.Shortcuts["search.next"] = shortcuts.Binding{Keys: []string{"Ctrl+J", "Down"}, Enabled: true}
	if a.setShortcutBinding("search.previous", shortcuts.Binding{Keys: []string{"Ctrl+J"}, Enabled: true}, false) {
		t.Fatal("conflict silently applied")
	}
	if !strings.HasPrefix(a.shortcutEdit.Error, locales.ShortcutsConflict()) {
		t.Fatal("missing conflict explanation")
	}
	if !a.setShortcutBinding("search.previous", shortcuts.Binding{Keys: []string{"Ctrl+J"}, Enabled: true}, true) {
		t.Fatal(a.shortcutEdit.Error)
	}
	if keys := a.settingsDraft.Shortcuts["search.next"].Keys; len(keys) != 1 || keys[0] != "Down" {
		t.Fatalf("replacement removed unrelated key: %+v", keys)
	}
	if _, ok := a.settings.Shortcuts["search.previous"]; ok {
		t.Fatal("draft changed saved settings map")
	}
}

func TestShortcutRecorderAndPageSizes(t *testing.T) {
	for _, tag := range []string{"zh-Hans", "zh-Hant", "en"} {
		setCatalogLocale(t, tag)
		for _, width := range []int{820, 1180} {
			a := newLocalizedApp()
			a.settingsPage = pageShortcuts
			a.shortcutFilter = shortcutTitle(shortcuts.Toggle)
			tt := ui.NewTester(a.settingsView, width, 660)
			editLabel := locales.CommonEdit() + ": " + shortcutTitle(shortcuts.Toggle)
			if err := tt.Click(editLabel); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			tt.Key(ui.Ctrl|ui.Shift, ui.KeyF7)
			tt.Frame()
			if a.shortcutEdit.Recorded != "Ctrl+Shift+F7" {
				t.Fatalf("recorder lost input: %+v", a.shortcutEdit)
			}
			if err := tt.Click(locales.CommonConfirm()); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if a.shortcutEdit.ID != "" || !containsFold(a.settingsDraft.Shortcuts[shortcuts.Toggle].Keys, "Ctrl+Shift+F7") {
				t.Fatalf("recording not saved: %+v", a.shortcutEdit)
			}
			for _, label := range []string{editLabel, locales.CommonEnable() + ": " + shortcutTitle(shortcuts.Toggle), locales.ShortcutsMore() + ": " + shortcutTitle(shortcuts.Toggle)} {
				bounds, ok := tt.Find(label)
				if !ok || bounds.W <= 0 || bounds.X < 0 || bounds.X+bounds.W > float32(width) {
					t.Fatalf("%s at %d: clipped %s %+v", tag, width, label, bounds)
				}
			}
		}
	}
}

func TestCompactShortcutRowsAndMenu(t *testing.T) {
	a := newLocalizedApp()
	a.settingsPage = pageShortcuts
	a.globalShortcuts.Active = map[string]string{"Alt+Space": shortcuts.Toggle}
	tt := ui.NewTester(a.settingsView, 1180, 840)
	previous, ok := tt.Find(locales.CommonEdit() + ": " + shortcutTitle("search.previous"))
	if !ok {
		t.Fatal("previous row missing")
	}
	next, ok := tt.Find(locales.CommonEdit() + ": " + shortcutTitle("search.next"))
	if !ok {
		t.Fatal("next row missing")
	}
	if delta := next.Y - previous.Y; delta > 48 || delta < 32 {
		t.Fatalf("rows are not compact: %.1f DIP", delta)
	}
	if tt.HasText(locales.ShortcutsActive()) || tt.HasText(locales.ShortcutsClear()) {
		t.Fatal("repeated statuses or secondary buttons remain in the list")
	}
	if _, ok := tt.Find(locales.CommonEdit() + ": " + shortcutTitle("actions.dismiss")); !ok {
		t.Fatal("first viewport does not reach the action panel commands")
	}

	a.shortcutFilter = shortcutTitle(shortcuts.Toggle)
	tt.Frame()
	toggleLabel := locales.CommonEnable() + ": " + shortcutTitle(shortcuts.Toggle)
	if err := tt.Click(toggleLabel); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.settingsDraft.Shortcuts[shortcuts.Toggle].Enabled {
		t.Fatal("switch did not disable shortcut")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if !a.settingsDraft.Shortcuts[shortcuts.Toggle].Enabled {
		t.Fatal("keyboard did not enable shortcut")
	}
	moreLabel := locales.ShortcutsMore() + ": " + shortcutTitle(shortcuts.Toggle)
	if err := tt.Click(moreLabel); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText(locales.ShortcutsDefaults()) {
		t.Fatal("more menu did not open")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if tt.HasText(locales.ShortcutsDefaults()) {
		t.Fatal("Escape did not dismiss more menu")
	}
	if !tt.Focused(moreLabel) {
		t.Fatal("menu did not return focus to its button")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if err := tt.Click(locales.ShortcutsClear()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(a.settingsDraft.Shortcuts[shortcuts.Toggle].Keys) != 0 {
		t.Fatal("menu did not clear binding")
	}
	if err := tt.Click(moreLabel); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click(locales.ShortcutsDefaults()); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, overridden := a.settingsDraft.Shortcuts[shortcuts.Toggle]; overridden {
		t.Fatal("menu did not restore defaults")
	}
}

func TestRecorderCapturesEscapeAndEnterWithoutExecuting(t *testing.T) {
	a := newLocalizedApp()
	a.settingsPage = pageShortcuts
	a.beginShortcutEdit("search.actions", nil)
	tt := ui.NewTester(a.settingsView, 1180, 840)
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.shortcutEdit.Recorded != "Escape" || a.shortcutEdit.ID == "" {
		t.Fatal("Escape canceled recording")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.shortcutEdit.Recorded != "Enter" || a.shortcutEdit.ID == "" {
		t.Fatal("Enter submitted recording")
	}
}

func TestActionAndPromptRemapping(t *testing.T) {
	a := newLocalizedApp()
	a.settings.Shortcuts["actions.slot.1"] = shortcuts.Binding{Keys: []string{"Ctrl+1"}, Enabled: true}
	a.settings.Shortcuts["actions.dismiss"] = shortcuts.Binding{Keys: []string{"Ctrl+Backspace"}, Enabled: true}
	a.settings.Shortcuts["prompt.confirm"] = shortcuts.Binding{Keys: []string{"Ctrl+Enter"}, Enabled: true}
	a.settings.Shortcuts["prompt.cancel"] = shortcuts.Binding{Keys: []string{"Ctrl+Escape"}, Enabled: true}
	a.results = []model.Result{{Entry: model.Entry{ID: "one", Title: "One", Kind: model.Application}}}
	a.openActions()
	tt := ui.NewTester(a.view, 760, 560)
	if !tt.HasText("Ctrl+1") {
		t.Fatal("action hint did not follow binding")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if !a.actionsOpen {
		t.Fatal("old action dismiss remained active")
	}
	tt.Key(ui.Ctrl, ui.KeyBackspace)
	tt.Frame()
	if a.actionsOpen {
		t.Fatal("new action dismiss failed")
	}
	a.promptOpen = true
	a.promptTitle = "Alias"
	a.promptValue = "value"
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !a.promptOpen {
		t.Fatal("old prompt confirm remained active")
	}
	tt.Key(ui.Ctrl, ui.KeyEnter)
	tt.Frame()
	if a.promptOpen {
		t.Fatal("new prompt confirm failed")
	}
}
