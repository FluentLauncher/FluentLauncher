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

func TestSettingsViewBuilds(t *testing.T) {
	a := &App{settings: settings.Default(), settingsDraft: settings.Default()}
	for page := range settingPages {
		a.settingsPage = page
		_ = ui.NewTester(a.settingsView, 900, 660)
	}
}
