package app

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/winapi/everything"
	"fluentlauncher/internal/winapi/shortcut"
	"fluentlauncher/locales"
)

// Run only on the app discovery worker; .lnk resolution never runs per keystroke.
func (a *App) rememberEverything(entries []model.Entry) {
	if a.everything == nil {
		return
	}
	for _, entry := range entries {
		path := entry.LaunchTarget
		base := filepath.Base(path)
		if strings.EqualFold(filepath.Ext(path), ".lnk") && strings.HasPrefix(strings.ToLower(base), "everything") {
			if target, ok := shortcut.Resolve(path); ok {
				path = target.Path
			}
		}
		if strings.EqualFold(filepath.Base(path), "Everything.exe") || strings.EqualFold(filepath.Base(path), "Everything64.exe") {
			a.everything.RememberExecutable(path)
			return
		}
	}
}

func (a *App) toolsPage(c *ui.Context) {
	if a.toolSelection == "" {
		a.toolSelection = "calc"
	}
	ui.Text(c, locales.ToolsIntro()).TextColor(c.Theme().TextMuted).Wrap()
	names := make([]string, 0, len(a.settingsDraft.Tools))
	for _, tool := range a.settingsDraft.Tools {
		names = append(names, search.ToolName(tool.ID))
	}
	a.themedSelect(c, "tool-choice", locales.SettingsPageTools(), "", names, search.ToolName(a.toolSelection), func(name string) {
		for _, tool := range a.settingsDraft.Tools {
			if search.ToolName(tool.ID) == name {
				a.toolSelection = tool.ID
			}
		}
	})
	for i := range a.settingsDraft.Tools {
		tool := &a.settingsDraft.Tools[i]
		if tool.ID != a.toolSelection {
			continue
		}
		c.Key(tool.ID)
		settingCard(c, search.ToolName(tool.ID), func(c *ui.Context) {
			searchSettingRow(c, locales.CommonEnable(), "", func(c *ui.Context) {
				if ui.Switch(c, &tool.Enabled).Label(locales.CommonEnable()).Changed() && tool.ID == "calc" {
					a.settingsDraft.EnableCalculator = tool.Enabled
				}
			})
			searchSettingRow(c, locales.ToolsPrefix(), locales.ToolsPrefixHint(), func(c *ui.Context) {
				ui.TextInput(c, &tool.Prefix).Width(180).Label(locales.ToolsPrefix() + ": " + search.ToolName(tool.ID))
			})
			searchSettingRow(c, locales.ToolsKeywords(), locales.ToolsKeywordsHint(), func(c *ui.Context) {
				row := ui.Row(c)
				words := ui.Local[string](row, "keywords", func() string { return strings.Join(tool.Keywords, ", ") })
				row.Children(func() {
					field := ui.TextInput(c, words).Width(240).Label(locales.ToolsKeywords() + ": " + search.ToolName(tool.ID))
					if field.Changed() {
						tool.Keywords = splitComma(*words)
					}
					if !field.Focused() {
						*words = strings.Join(tool.Keywords, ", ")
					}
				})
			})
			searchSettingRow(c, locales.ToolsGlobal(), locales.ToolsGlobalHint(), func(c *ui.Context) {
				ui.Switch(c, &tool.GlobalQuery).Label(locales.ToolsGlobal() + ": " + search.ToolName(tool.ID)).Disabled(tool.Prefix != "")
			})
			ui.Text(c, "tool "+strings.Join(tool.Keywords, " / tool ")).TextColor(c.Theme().TextMuted).Wrap()
		})
	}
}

func searchSettingRow(c *ui.Context, label, hint string, control func(*ui.Context)) {
	theme := c.Theme()
	ui.Row(c).Gap(theme.Space(4)).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(theme.Space(1)).Children(func() {
			ui.Text(c, label).Wrap()
			if hint != "" {
				ui.Text(c, hint).FontSize(theme.FontSize * 0.9).TextColor(theme.TextMuted).Wrap()
			}
		})
		control(c)
	})
}

func (a *App) everythingCard(c *ui.Context) {
	options := &a.settingsDraft.Files
	settingCard(c, locales.EverythingCard(), func(c *ui.Context) {
		a.switchRow(c, locales.SettingsFilesEverything(), locales.SettingsFilesEverythingHint(), &options.EnableEverything)
		searchSettingRow(c, locales.EverythingAutoStart(), locales.EverythingAutoStartHint(), func(c *ui.Context) {
			ui.Switch(c, &options.EverythingAutoStart).Label(locales.EverythingAutoStart())
		})
		searchSettingRow(c, locales.EverythingPath(), locales.EverythingPathHint(), func(c *ui.Context) {
			ui.TextInput(c, &options.EverythingPath).Placeholder("Everything.exe").Width(280).Label(locales.EverythingPath())
		})
		ui.Row(c).Gap(c.Theme().Spacing).Children(func() {
			if compactButton(c, locales.EverythingChoose()).Disabled(a.everythingBusy).Clicked() {
				go a.chooseEverything()
			}
			if a.everything != nil {
				status := a.everything.Status()
				if compactButton(c, locales.EverythingConnect()).Disabled(a.everythingBusy || status.State == everything.Starting).Clicked() {
					a.runEverythingAction(false)
				}
				if compactButton(c, locales.EverythingOpen()).Disabled(a.everythingBusy).Clicked() {
					a.runEverythingAction(true)
				}
			}
		})
		if a.everything != nil {
			status := a.everything.Status()
			text := locales.EverythingDisconnected()
			color := c.Theme().TextMuted
			switch status.State {
			case everything.Starting:
				text = locales.EverythingStarting()
			case everything.Connected:
				text, color = locales.EverythingConnected(), c.Theme().Success
			case everything.ConnectionFailed:
				text, color = locales.EverythingFailed()+": "+status.Error, c.Theme().Danger
			}
			ui.Text(c, text).TextColor(color).Wrap()
			if status.Path != "" {
				ui.Text(c, status.Path).TextColor(c.Theme().TextMuted).Wrap()
			}
		}
	})
}

func (a *App) chooseEverything() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: locales.EverythingChoose(),
		Filters: []mygo.FileFilter{{Name: "Everything.exe", Extensions: []string{"exe"}}}})
	a.publishUpdate(func() {
		if err != nil {
			a.settingsError = err.Error()
		} else if len(paths) > 0 {
			a.settingsDraft.Files.EverythingPath = paths[0]
			a.scheduleSettingsSave()
		}
	})
}

func (a *App) runEverythingAction(show bool) {
	a.everythingBusy = true
	options := a.settingsDraft.Files
	go func() {
		var err error
		if show {
			err = a.everything.OpenWithOptions(options)
		} else {
			err = a.everything.ConnectWithOptions(context.Background(), options)
		}
		a.publishUpdate(func() {
			a.everythingBusy = false
			if err != nil {
				a.settingsError = err.Error()
			} else {
				a.settingsError = ""
				a.updateResults()
			}
		})
	}()
}

func (a *App) validateToolPluginConflicts(value settings.LauncherSettings) error {
	if a.pluginRepo == nil {
		return nil
	}
	enabledChanged := !reflect.DeepEqual(value.Plugins.EnabledIDs, a.settings.Plugins.EnabledIDs)
	for _, tool := range value.Tools {
		// Existing plugin/utility overlaps remain usable until their trigger
		// is edited; unrelated settings changes must not become impossible.
		unchangedTool := reflect.DeepEqual(tool, a.settings.Tool(tool.ID))
		if !tool.Enabled || unchangedTool && !enabledChanged {
			continue
		}
		for _, registration := range a.pluginRepo.Installed() {
			if !containsFold(value.Plugins.EnabledIDs, registration.Manifest.ID) {
				continue
			}
			if unchangedTool && containsFold(a.settings.Plugins.EnabledIDs, registration.Manifest.ID) {
				continue
			}
			for _, word := range registration.Manifest.Keywords {
				if tool.Prefix != "" && strings.HasPrefix(strings.ToLower(word), strings.ToLower(tool.Prefix)) ||
					tool.Prefix == "" && tool.GlobalQuery && containsFold(tool.Keywords, word) {
					return fmt.Errorf("%s: %s / %s", locales.ToolsConflict(), search.ToolName(tool.ID), registration.Manifest.Name)
				}
			}
		}
	}
	return nil
}
