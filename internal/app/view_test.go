package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/core/settings"
)

func TestLauncherViewBuilds(t *testing.T) {
	a := &App{
		settings:      settings.Default(),
		settingsDraft: settings.Default(),
		icons:         map[string]*ui.Bitmap{},
		iconLoading:   map[string]bool{},
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
			icons:         map[string]*ui.Bitmap{},
			iconLoading:   map[string]bool{},
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

func TestSettingsAppearanceInteractions(t *testing.T) {
	a := &App{
		settings:      settings.Default(),
		settingsDraft: settings.Default(),
		icons:         map[string]*ui.Bitmap{},
		iconLoading:   map[string]bool{},
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
	if a.settingsPage != 1 {
		t.Fatalf("settings page = %d, want 1", a.settingsPage)
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

func TestSettingsViewBuilds(t *testing.T) {
	a := &App{settings: settings.Default(), settingsDraft: settings.Default()}
	for page := range settingPages {
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
	for _, name := range []settings.BackgroundMode{settings.BackgroundAcrylic, settings.BackgroundMica} {
		if !blursBehind(settings.ThemeSettings{Background: name}) {
			t.Fatalf("%s does not blur what is behind the window", name)
		}
	}
	for _, name := range []settings.BackgroundMode{
		settings.BackgroundSolid, settings.BackgroundGradient, settings.BackgroundImage,
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
