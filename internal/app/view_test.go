package app

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/store"
	"fluentlauncher/locales"
)

func TestLauncherViewBuilds(t *testing.T) {
	a := &App{
		settings:      settings.Default(),
		settingsDraft: settings.Default(),
		profiles:      map[string]model.Profile{},
	}
	a.results = []model.Result{{Entry: model.Entry{ID: "x", Title: "Example", Subtitle: "subtitle", Kind: model.Application}}}
	a.chosen = 0
	tt := ui.NewTester(a.view, 760, 560)
	if !tt.HasText("Example") {
		t.Fatal("result row not built")
	}
	a.actions = search.CommandsFor(a.results[0].Entry, false, false)
	a.actionsOpen = true
	tt = ui.NewTester(a.view, 760, 560)
	if !tt.HasText("打开") {
		t.Fatal("action panel not built")
	}
	a.actionsOpen = false
	a.promptOpen = true
	a.promptTitle = "Alias"
	a.promptValue = "x"
	tt = ui.NewTester(a.view, 760, 560)
	if !tt.HasText("Alias") {
		t.Fatal("prompt not built")
	}
}

func TestRowClickAndEnterExecute(t *testing.T) {
	newApp := func() *App {
		a := &App{
			settings:      settings.Default(),
			settingsDraft: settings.Default(),
			profiles:      map[string]model.Profile{},
			resultsReady:  true,
		}
		// A plugin entry without actions fails execution with a notice,
		// which proves the launch path ran without touching windows.
		a.results = []model.Result{{Entry: model.Entry{ID: "p", Title: "PluginRow", Kind: model.Plugin}}}
		return a
	}
	const notice = "该插件条目没有可执行动作"

	a := newApp()
	tt := ui.NewTester(a.view, 760, 560)
	if err := tt.Click("PluginRow"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.notice != notice {
		t.Fatalf("row click did not execute: notice=%q", a.notice)
	}

	a = newApp()
	tt = ui.NewTester(a.view, 760, 560)
	a.input.Focus()
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.notice != notice {
		t.Fatalf("enter in search box did not execute: notice=%q", a.notice)
	}
}

func TestActionPanelKeyboard(t *testing.T) {
	for _, shortcut := range []struct {
		name string
		mods ui.Modifiers
		key  ui.Key
	}{
		{"Ctrl+K", ui.Ctrl, ui.KeyK},
		{"Shift+F10", ui.Shift, ui.KeyF10},
		{"Menu", 0, ui.KeyContextMenu},
	} {
		t.Run(shortcut.name, func(t *testing.T) {
			a := newLocalizedApp()
			a.query = "Example"
			a.results = []model.Result{
				{Entry: model.Entry{ID: "other", Title: "Another Example", Kind: model.Application}},
				{Entry: model.Entry{ID: "x", Title: "Example", Kind: model.Application}},
			}
			tt := ui.NewTester(a.view, 760, 560)
			a.input.Focus()
			tt.Frame()
			tt.Key(0, ui.KeyDown)
			tt.Frame()
			tt.Key(shortcut.mods, shortcut.key)
			tt.Frame()
			if !a.actionsOpen || !tt.HasText("设置别名") {
				t.Fatal("keyboard shortcut did not open the selected result's actions")
			}
			tt.Key(0, ui.KeyDown)
			tt.Frame()
			if a.actionIndex != 1 {
				t.Fatalf("Down selected action %d, want 1", a.actionIndex)
			}
			tt.Key(0, ui.KeyUp)
			tt.Frame()
			if a.actionIndex != 0 {
				t.Fatalf("Up selected action %d, want 0", a.actionIndex)
			}
			tt.Key(0, ui.KeyEscape)
			tt.Frame()
			if a.actionsOpen || a.query != "Example" || !tt.Focused(locales.PanelSearchLabel()) {
				t.Fatal("Escape did not return to the search field with the query preserved")
			}
			// Reopening must retain working shortcuts after the focus target
			// disappeared and returned in a later build.
			tt.Key(shortcut.mods, shortcut.key)
			tt.Frame()
			tt.Key(0, ui.KeyDown)
			tt.Key(0, ui.KeyDown)
			tt.Key(0, ui.KeyEnter)
			tt.Frame()
			if !a.promptOpen || a.promptEntry != "x" {
				t.Fatal("Enter did not execute the selected alias action")
			}
		})
	}
}

func TestResultActionsShortcutHint(t *testing.T) {
	for _, tag := range []string{"zh-Hans", "zh-Hant", "en"} {
		t.Run(tag, func(t *testing.T) {
			setCatalogLocale(t, tag)
			for _, width := range []int{480, 760} {
				a := newLocalizedApp()
				a.results = []model.Result{{Entry: model.Entry{
					ID: "x", Title: "An application with a very long name that fills the result row",
					Subtitle: "A long subtitle that must leave room for the action shortcut hint", Kind: model.Application,
				}}}
				tt := ui.NewTester(a.view, width, 560)
				bounds, ok := tt.Find(a.primaryShortcutHint("search.actions"))
				if !ok || bounds.W <= 0 || bounds.X < 0 || bounds.X+bounds.W > float32(width) {
					t.Fatalf("%d: action entry shortcut missing or clipped: %+v", width, bounds)
				}
			}
		})
	}
}

func TestActionPanelNumberedShortcuts(t *testing.T) {
	for _, target := range []string{"", `C:\Apps\Example.exe`} {
		for _, width := range []int{480, 760} {
			t.Run(strconv.Itoa(width)+"/"+target, func(t *testing.T) {
				a := newLocalizedApp()
				a.results = []model.Result{{Entry: model.Entry{
					ID: "x", Title: "Example", Kind: model.Application, LaunchTarget: target,
				}}}
				tt := ui.NewTester(a.view, width, 560)
				a.input.Focus()
				tt.Frame()
				tt.Key(ui.Ctrl, ui.KeyK)
				tt.Frame()
				aliasIndex := -1
				for i, action := range a.actions {
					if action.Command == search.CommandSetAlias {
						aliasIndex = i
					}
					label := "Alt+" + strconv.Itoa(i+1)
					bounds, ok := tt.Find(label)
					if !ok || bounds.X < 0 || bounds.X+bounds.W > float32(width) {
						t.Fatalf("shortcut %s missing or outside the panel: %+v", label, bounds)
					}
				}
				if aliasIndex < 0 {
					t.Fatal("alias action missing")
				}
				tt.Key(ui.Alt, ui.Key1+ui.Key(aliasIndex))
				tt.Frame()
				if !a.promptOpen || a.promptEntry != "x" || a.actionIndex != aliasIndex {
					t.Fatal("Alt+number did not execute its displayed action")
				}
			})
		}
	}
}

func TestActionPanelToggleKeepsQuery(t *testing.T) {
	a := newLocalizedApp()
	a.query = "Example"
	a.results = []model.Result{{Entry: model.Entry{ID: "x", Title: "Example", Kind: model.Application}}}
	tt := ui.NewTester(a.view, 760, 560)
	a.input.Focus()
	tt.Frame()
	for range 2 {
		tt.Key(ui.Ctrl, ui.KeyK)
		tt.Frame()
		if !a.actionsOpen {
			t.Fatal("Ctrl+K did not open the actions")
		}
		tt.Key(ui.Ctrl, ui.KeyK)
		tt.Frame()
		if a.actionsOpen || a.query != "Example" || !tt.Focused(locales.PanelSearchLabel()) {
			t.Fatal("Ctrl+K did not return to the search field with the query preserved")
		}
	}
}

func TestSettingsAppearanceInteractions(t *testing.T) {
	a := &App{
		settings:      settings.Default(),
		settingsDraft: settings.Default(),
		profiles:      map[string]model.Profile{},
	}
	a.settingsDraft.Theme.Background = settings.BackgroundAcrylic
	tt := ui.NewTester(a.settingsView, 900, 660)
	// Pointer clicks, not label targeting: a full-window background layer
	// that forgets IgnoreInput passes label clicks but swallows the pointer.
	clickText := func(s string) {
		t.Helper()
		rect, ok := tt.Find(s)
		if !ok {
			t.Fatalf("text %q not found", s)
		}
		tt.ClickAt(rect.X+rect.W/2, rect.Y+rect.H/2)
		tt.Frame()
	}
	clickText("外观")
	if a.settingsPage != pageAppearance {
		t.Fatalf("settings page = %d, want %d", a.settingsPage, pageAppearance)
	}
	if !tt.HasText("不透明度") {
		t.Fatal("opacity card missing for acrylic")
	}
	if tt.HasText("高斯模糊") {
		t.Fatal("image card shown while acrylic selected")
	}
	clickText("亚克力") // the dropdown anchor
	clickText("图片")
	if a.settingsDraft.Theme.Background != settings.BackgroundImage {
		t.Fatalf("background = %q, want image", a.settingsDraft.Theme.Background)
	}
	if !tt.HasText("高斯模糊") {
		t.Fatal("image card missing after switching to image")
	}
}

func TestSettingsSaveAutomatically(t *testing.T) {
	a := &App{settings: settings.Default(), settingsDraft: settings.Default()}
	a.settingsPage = pageAppearance
	tt := ui.NewTester(a.settingsView, 900, 660)
	if tt.HasText("取消") {
		t.Fatal("settings still offer a cancel button")
	}
	if !tt.HasText("更改会自动保存") {
		t.Fatal("settings do not say that changes save themselves")
	}
	a.settingsDraft.Theme.Opacity = 0.4
	if !a.draftChanged() {
		t.Fatal("a changed draft is not reported")
	}
}
func TestSettingsViewBuilds(t *testing.T) {
	a := &App{settings: settings.Default(), settingsDraft: settings.Default()}
	for page := range settingPages() {
		a.settingsPage = page
		_ = ui.NewTester(a.settingsView, 900, 660)
	}
}

func TestSettingsSizePreview(t *testing.T) {
	a := &App{settings: settings.Default(), settingsDraft: settings.Default()}
	tt := ui.NewTester(a.settingsView, 900, 660)
	if !tt.HasText("拖动面板移动位置，拖边缘调整大小") {
		t.Fatal("size preview hint missing")
	}
	if !tt.HasText("760 × 560") {
		t.Fatal("size preview does not show the draft size")
	}
	a.resizeDraft(-10000, -10000)
	if a.settingsDraft.Width != 480 || a.settingsDraft.Height != 320 {
		t.Fatalf("size below the minimum is %d×%d", a.settingsDraft.Width, a.settingsDraft.Height)
	}
	a.resizeDraft(10000, 10000)
	if a.settingsDraft.Width != 1600 || a.settingsDraft.Height != 1200 {
		t.Fatalf("size above the maximum is %d×%d", a.settingsDraft.Width, a.settingsDraft.Height)
	}
}

func TestBlursBehind(t *testing.T) {
	for _, name := range []settings.BackgroundMode{settings.BackgroundAcrylic} {
		if !blursBehind(settings.ThemeSettings{Background: name}) {
			t.Fatalf("%s does not blur what is behind the window", name)
		}
	}
	for _, name := range []settings.BackgroundMode{
		settings.BackgroundMica, settings.BackgroundSolid, settings.BackgroundGradient, settings.BackgroundImage,
	} {
		if blursBehind(settings.ThemeSettings{Background: name}) {
			t.Fatalf("%s blurs what is behind the window", name)
		}
	}
}

func TestWorkspaceViewBuilds(t *testing.T) {
	a := &App{settings: settings.Default(), settingsDraft: settings.Default()}
	a.workspaceName = "工作区"
	a.workspaceSteps = []settings.WorkspaceStep{
		{Name: "记事本", Succeeded: true},
		{Name: "便签", Error: "找不到路径"},
	}
	tt := ui.NewTester(a.workspaceView, 520, 420)
	if !tt.HasText("工作区") {
		t.Fatal("workspace result view not built")
	}
}

func TestTrayDoubleClick(t *testing.T) {
	var last time.Time
	start := time.Now()
	if isDoubleClick(&last, start) {
		t.Fatal("the first click is not a double click")
	}
	if !isDoubleClick(&last, start.Add(trayDoubleClick/2)) {
		t.Fatal("a second click within the interval is a double click")
	}
	if !last.IsZero() {
		t.Fatal("a double click does not start another pair")
	}
	if isDoubleClick(&last, start.Add(2*time.Second)) {
		t.Fatal("a late click is the first of a new pair")
	}
}

// TestOtherPageActionsReportDone covers the Other page's data buttons: they
// confirm what they did in the settings footer instead of staying silent,
// which made a reset look like a click that did nothing.
func TestOtherPageActionsReportDone(t *testing.T) {
	db, err := store.OpenDB(context.Background(), filepath.Join(t.TempDir(), "launcher.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	a := &App{
		settings:        settings.Default(),
		settingsDraft:   settings.Default(),
		profiles:        map[string]model.Profile{},
		db:              db,
		appEngine:       search.NewEngine(),
		fileEngine:      search.NewEngine(),
		customEngine:    search.NewEngine(),
		workspaceEngine: search.NewEngine(),
		settingsPage:    pageOther,
	}
	tt := ui.NewTester(a.settingsView, 900, 660)
	// resetLearning does the database wipe off the UI thread and marshals
	// this finish back through the settings window, which a tester has none
	// of; drive the UI half directly.
	a.finishResetLearning()
	tt.Frame()
	if !tt.HasText("学习数据已重置。") {
		t.Fatalf("no confirmation after the reset: %q", tt.Texts())
	}

	a.profiles["gone"] = model.Profile{EntryID: "gone", Hidden: true}
	if err := tt.Click("恢复隐藏条目"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("已恢复 1 个隐藏条目。") {
		t.Fatalf("no confirmation after restoring: %q", tt.Texts())
	}
	if a.profiles["gone"].Hidden {
		t.Fatal("the entry is still hidden")
	}
}

// TestRegenerateToolValue rerolls the selected random tool in place, keeping
// the bounds or length the user asked for (they ride in the entry ID).
func TestRegenerateToolValue(t *testing.T) {
	a := &App{
		settings:      settings.Default(),
		settingsDraft: settings.Default(),
		profiles:      map[string]model.Profile{},
	}
	a.results = search.UUIDResults("62152892-92c1-425f-b32e-075f31040bde")
	a.chosen = 1
	a.resultsReady = true
	a.regenerateToolValue()
	got := a.results[0].Entry
	if got.Title == "62152892-92c1-425f-b32e-075f31040bde" ||
		!regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(got.Title) {
		t.Fatalf("uuid reroll = %+v", got)
	}
	// Rerolling from any format row rerolls all of them to the same UUID.
	if plain := a.results[1].Entry; plain.Title != strings.ReplaceAll(got.Title, "-", "") || plain.CopyText != plain.Title {
		t.Fatalf("reroll did not stay consistent: %+v / %+v", got, plain)
	}

	a.results = []model.Result{{Entry: model.Entry{
		ID: "rand:7-9:8", Title: "8", CopyText: "8", Kind: model.Calculation, SourceID: "utilities"}}}
	for i := 0; i < 20; i++ {
		a.regenerateToolValue()
		n, err := strconv.Atoi(a.results[0].Entry.Title)
		if err != nil || n < 7 || n > 9 {
			t.Fatalf("rand reroll left the 7-9 range: %q", a.results[0].Entry.Title)
		}
	}
}
