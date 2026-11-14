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
	theme := c.Theme()
	ui.Column(c).Gap(theme.Space(3)).Children(func() {
		ui.Text(c, locales.ToolsBuiltInHint()).TextColor(theme.TextMuted).Wrap()
		ui.Row(c).Gap(theme.Space(3)).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.toolFilter).Grow(1).Height(theme.Space(8)).
				Placeholder(locales.ToolsFilter()).Label(locales.ToolsFilter())
			ui.Text(c, fmt.Sprintf("%s · %d", locales.ToolsBuiltIn(), len(a.settingsDraft.Tools))).TextColor(theme.TextMuted)
		})
		ui.Text(c, locales.ToolsListHint()).FontSize(theme.FontSize * 0.9).TextColor(theme.TextMuted).Wrap().Tooltip(locales.ToolsIntro())
		query := strings.ToLower(strings.TrimSpace(a.toolFilter))
		c.Key("tool-table")
		ui.Column(c).Gap(0).Border(1, theme.Border).Radius(theme.Radius).Children(func() {
			toolsGrid(c).Padding(theme.Space(2), theme.Space(3)).Background(theme.Surface).Children(func() {
				for i, label := range []string{locales.ToolsBuiltIn(), locales.ToolsPrefix(), locales.ToolsKeywords(), locales.ToolsNormalSearch(), locales.CommonEnable()} {
					text := ui.Text(c, label).ColumnStart(i + 1).Bold().Wrap().TextColor(theme.TextMuted)
					switch i {
					case 1:
						text.Tooltip(locales.ToolsPrefixHint())
					case 2:
						text.Tooltip(locales.ToolsKeywordsHint())
					case 3:
						text.Tooltip(locales.ToolsGlobalHint())
					}
				}
			})
			count := 0
			for _, group := range []struct {
				title string
				ids   []string
			}{
				{locales.ToolsGroupValues(), []string{"calc", "uuid", "rand", "pass"}},
				{locales.ToolsGroupDates(), []string{"date", "time", "timestamp", "week", "days"}},
			} {
				heading := false
				for _, id := range group.ids {
					for i := range a.settingsDraft.Tools {
						tool := &a.settingsDraft.Tools[i]
						if tool.ID != id {
							continue
						}
						matches := query == "" || strings.Contains(strings.ToLower(search.ToolName(id)+" "+id+" "+tool.Prefix+" "+strings.Join(tool.Keywords, " ")), query)
						if !matches && !a.toolRowHandle(id).FocusWithin(c) {
							continue
						}
						if !heading {
							c.Key("group:" + group.ids[0])
							ui.Text(c, group.title).Padding(theme.Space(2), theme.Space(3)).FontSize(theme.FontSize * 0.9).Bold().TextColor(theme.TextMuted)
							heading = true
						}
						c.Key(tool.ID)
						a.toolSettingsRow(c, tool)
						count++
					}
				}
			}
			if count == 0 {
				ui.Text(c, locales.ToolsFilterEmpty()).Padding(theme.Space(4), theme.Space(3)).TextColor(theme.TextMuted).Wrap()
			}
		})
	})
}

// Every row and its header share tracks, so editing stays aligned as the
// settings window changes size. Flexible name/keyword tracks absorb the space.
func toolsGrid(c *ui.Context) ui.Element {
	theme := c.Theme()
	return ui.Grid(c).ColumnTracks(ui.Fr(1.2), ui.Fixed(theme.Space(24)), ui.Fr(1.8), ui.Fixed(theme.Space(20)), ui.Fixed(theme.Space(14))).
		Gap(theme.Space(3)).FillWidth()
}

func (a *App) toolSettingsRow(c *ui.Context, tool *settings.ToolOptions) {
	theme := c.Theme()
	name := search.ToolName(tool.ID)
	row := toolsGrid(c).Bind(a.toolRowHandle(tool.ID)).MinHeight(theme.Space(11)).Padding(theme.Space(1.5), theme.Space(3)).Transition(feedbackTransition)
	if row.Hovered() {
		row.Background(theme.SurfaceHover)
	}
	words := ui.Local[string](row, "keywords", func() string { return strings.Join(tool.Keywords, ", ") })
	row.Children(func() {
		color := theme.Text
		if !tool.Enabled {
			color = theme.TextMuted
		}
		ui.Row(c).ColumnStart(1).AlignItems(ui.Center).Children(func() {
			ui.Text(c, name).TextColor(color).Wrap().Tooltip("tool " + strings.Join(tool.Keywords, " / tool "))
		})
		ui.TextInput(c, &tool.Prefix).ColumnStart(2).FillWidth().Label(locales.ToolsPrefix() + ": " + name).Tooltip(locales.ToolsPrefixHint())
		field := ui.TextInput(c, words).ColumnStart(3).FillWidth().Label(locales.ToolsKeywords() + ": " + name).Tooltip(locales.ToolsKeywordsHint())
		if field.Changed() {
			tool.Keywords = splitComma(*words)
		}
		if !field.Focused() {
			*words = strings.Join(tool.Keywords, ", ")
		}
		ui.Row(c).ColumnStart(4).Center().Children(func() {
			ui.Switch(c, &tool.GlobalQuery).Label(locales.ToolsGlobal() + ": " + name).Tooltip(locales.ToolsGlobalHint()).Disabled(tool.Prefix != "")
		})
		ui.Row(c).ColumnStart(5).Center().Children(func() {
			if ui.Switch(c, &tool.Enabled).Label(locales.CommonEnable()+": "+name).Changed() && tool.ID == "calc" {
				a.settingsDraft.EnableCalculator = tool.Enabled
			}
		})
	})
}

func (a *App) toolRowHandle(id string) *ui.Handle {
	if a.toolRowHandles == nil {
		a.toolRowHandles = map[string]*ui.Handle{}
	}
	if a.toolRowHandles[id] == nil {
		a.toolRowHandles[id] = &ui.Handle{}
	}
	return a.toolRowHandles[id]
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
