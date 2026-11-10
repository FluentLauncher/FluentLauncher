package app

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/plugin"
	"fluentlauncher/internal/testutil"
	"fluentlauncher/internal/winapi/everything"
	"fluentlauncher/locales"
)

func TestToolPluginConflictsOnlyBlockRelatedChanges(t *testing.T) {
	a := newLocalizedApp()
	a.pluginRepo = plugin.NewRepository(filepath.Join(t.TempDir(), "plugins"))
	source := testutil.PluginFixture(t, "vendor.example")
	manifest := filepath.Join(source, "plugin.json")
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	// Fixture uses echo; change it to a custom trigger shared with UUID.
	var registration map[string]any
	if err := json.Unmarshal(data, &registration); err != nil {
		t.Fatal(err)
	}
	registration["keywords"] = []string{"uid"}
	data, err = json.Marshal(registration)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.pluginRepo.Install(source, false); err != nil {
		t.Fatal(err)
	}
	value := cloneSettings(a.settings)
	value.Plugins.EnabledIDs = []string{"vendor.example"}
	for i := range value.Tools {
		if value.Tools[i].ID == "uuid" {
			value.Tools[i].Keywords = []string{"uid"}
		}
	}
	if a.validateToolPluginConflicts(value) == nil {
		t.Fatal("new tool/plugin overlap was accepted")
	}
	// A historical overlap must not prevent changing an unrelated setting.
	a.settings = cloneSettings(value)
	value.Width = 800
	if err := a.validateToolPluginConflicts(value); err != nil {
		t.Fatal(err)
	}
	a.settings.Plugins.EnabledIDs = nil
	if a.validateToolPluginConflicts(value) == nil {
		t.Fatal("enabling a conflicting plugin was accepted")
	}
}

func TestToolsSettingsEditAndSwitchIdentity(t *testing.T) {
	a := newLocalizedApp()
	a.settingsPage = pageTools
	tt := ui.NewTester(a.settingsView, 1180, 840)
	if !tt.HasText(locales.ToolsBuiltInHint()) || !tt.HasText(locales.ToolsPrefix()) {
		t.Fatal("tool configuration is not discoverable")
	}
	if err := tt.Click(locales.ToolsPrefix() + ": " + search.ToolName("calc")); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("@@")
	tt.Frame()
	if a.settingsDraft.Tool("calc").Prefix != "@@" {
		t.Fatal("prefix edit was lost on rebuild")
	}
	// Filter the list while editors disappear and return with stable identities.
	a.toolFilter = "uuid"
	tt.Frame()
	if err := tt.Click(locales.ToolsKeywords() + ": " + search.ToolName("uuid")); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("uid")
	tt.Frame()
	if a.settingsDraft.Tool("uuid").Keywords[0] != "uid" {
		t.Fatal("keyword edit was lost")
	}
	// Commas typed one event at a time must survive normalization/rebuilds.
	tt.Type(",")
	tt.Frame()
	tt.Type(" unique")
	tt.Frame()
	if words := a.settingsDraft.Tool("uuid").Keywords; len(words) != 2 || words[1] != "unique" {
		t.Fatalf("typing an alias separator lost input: %v", words)
	}
	if err := tt.Click(locales.ToolsGlobal() + ": " + search.ToolName("uuid")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.settingsDraft.Tool("uuid").GlobalQuery {
		t.Fatal("normal-search switch did not disable tool routing")
	}
	a.toolFilter = "calc"
	tt.Frame()
	if a.settingsDraft.Tool("calc").Prefix != "@@" {
		t.Fatal("changing tools lost the calculator prefix")
	}
	if err := tt.Click(locales.CommonEnable() + ": " + search.ToolName("calc")); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.settingsDraft.Tool("calc").Enabled || a.settingsDraft.EnableCalculator {
		t.Fatal("inline switch did not disable the calculator")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if !a.settingsDraft.Tool("calc").Enabled || !a.settingsDraft.EnableCalculator {
		t.Fatal("keyboard did not restore the calculator toggle")
	}
}

func TestToolsListShowsEveryToolAndFilters(t *testing.T) {
	a := newLocalizedApp()
	a.settingsPage = pageTools
	tt := ui.NewTester(a.settingsView, 1180, 840)
	for _, tool := range a.settingsDraft.Tools {
		if !tt.HasText(search.ToolName(tool.ID)) {
			t.Fatalf("tool hidden behind a selector: %s", tool.ID)
		}
		if _, ok := tt.Find(locales.CommonEnable() + ": " + search.ToolName(tool.ID)); !ok {
			t.Fatalf("tool lacks inline toggle: %s", tool.ID)
		}
	}
	if err := tt.Click(locales.ToolsFilter()); err != nil {
		t.Fatal(err)
	}
	tt.Type("=")
	tt.Frame()
	if !tt.HasText(search.ToolName("calc")) || tt.HasText(search.ToolName("uuid")) {
		t.Fatal("prefix filter did not narrow the list")
	}
	if err := tt.Click(locales.ToolsPrefix() + ": " + search.ToolName("calc")); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("@@")
	tt.Frame()
	if !tt.Focused(locales.ToolsPrefix()+": "+search.ToolName("calc")) || a.settingsDraft.Tool("calc").Prefix != "@@" {
		t.Fatal("editing the filtered prefix removed its row or lost focus")
	}
	if err := tt.Click(locales.ToolsFilter()); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("no-such-tool")
	tt.Frame()
	if !tt.HasText(locales.ToolsFilterEmpty()) {
		t.Fatal("empty filter result does not explain how to recover")
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	tt.Frame()
	if !tt.HasText(search.ToolName("uuid")) {
		t.Fatal("clearing filter did not restore tools")
	}
	tt.Scroll(700, 500, 0, 10000)
	tt.Frame()
	lastToggle := locales.CommonEnable() + ": " + search.ToolName("urldecode")
	if bounds, ok := tt.Find(lastToggle); !ok || bounds.W <= 0 || bounds.H <= 0 {
		t.Fatal("the last tool cannot be reached by scrolling")
	}
	if err := tt.Click(lastToggle); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.settingsDraft.Tool("urldecode").Enabled {
		t.Fatal("scrolled tool toggle did not update its own setting")
	}
}

func TestSearchSettingsLayouts(t *testing.T) {
	for _, lang := range []string{"zh-Hans", "zh-Hant", "en"} {
		setCatalogLocale(t, lang)
		for _, dark := range []bool{false, true} {
			for _, size := range [][2]int{{900, 660}, {1180, 840}} {
				for _, page := range []int{pageTools, pageFiles} {
					a := newLocalizedApp()
					a.settingsPage = page
					a.settings.Theme.Background = settings.BackgroundSolid
					a.settings.Theme.Opacity = 1
					a.settingsDraft = cloneSettings(a.settings)
					a.everything = everything.NewProvider(func() settings.FileIndexOptions { return a.settings.Files }, nil)
					tt := ui.NewTester(a.settingsView, size[0], size[1])
					tt.SetDark(dark)
					tt.Advance(300 * time.Millisecond)
					if page == pageTools {
						for _, label := range []string{locales.ToolsPrefix(), locales.ToolsKeywords(), locales.ToolsNormalSearch()} {
							bounds, ok := tt.Find(label)
							if !ok || bounds.W <= 0 || bounds.X < 0 || bounds.X+bounds.W > float32(size[0]) {
								t.Fatalf("%s %v: clipped %s %+v", lang, size, label, bounds)
							}
						}
						for _, tool := range a.settingsDraft.Tools {
							// Offscreen controls are clipped by Tester; bring each
							// tool into view to verify its actual editable bounds.
							a.toolFilter = tool.ID
							tt.Frame()
							previousRight := float32(0)
							for _, label := range []string{locales.ToolsPrefix(), locales.ToolsKeywords(), locales.ToolsGlobal(), locales.CommonEnable()} {
								bounds, ok := tt.Find(label + ": " + search.ToolName(tool.ID))
								if !ok || bounds.W <= 0 || bounds.X < previousRight || bounds.X+bounds.W > float32(size[0]) {
									t.Fatalf("%s %v: overlapping/clipped control %s %s %+v", lang, size, tool.ID, label, bounds)
								}
								previousRight = bounds.X + bounds.W
							}
						}
						a.toolFilter = ""
						tt.Frame()
					} else if !tt.HasText(locales.EverythingCard()) || !tt.HasText(locales.SettingsFilesLocalRebuild()) {
						t.Fatal("Everything and local indexing were not separated")
					}
					if dir := os.Getenv("FL_SEARCH_ARTIFACT_DIR"); dir != "" && size[0] == 1180 {
						if err := os.MkdirAll(dir, 0755); err != nil {
							t.Fatal(err)
						}
						path := filepath.Join(dir, fmt.Sprintf("search-settings-%s-%d-dark-%t.png", lang, page, dark))
						file, err := os.Create(path)
						if err != nil {
							t.Fatal(err)
						}
						err = png.Encode(file, tt.Image())
						file.Close()
						if err != nil {
							t.Fatal(err)
						}
					}
				}
			}
		}
	}
}
