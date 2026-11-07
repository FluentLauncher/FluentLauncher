package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/mistquay/fluentlauncher/internal/core/model"
	"github.com/mistquay/fluentlauncher/internal/core/search"
	"github.com/mistquay/fluentlauncher/internal/core/settings"
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
	_ = ui.NewTester(a.view, 760, 560)
	a.actionsOpen = false
	a.promptOpen = true
	a.promptTitle = "Alias"
	a.promptValue = "x"
	_ = ui.NewTester(a.view, 760, 560)
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

func TestSettingsViewBuilds(t *testing.T) {
	a := &App{settings: settings.Default(), settingsDraft: settings.Default()}
	for page := range settingPages {
		a.settingsPage = page
		_ = ui.NewTester(a.settingsView, 900, 660)
	}
}
