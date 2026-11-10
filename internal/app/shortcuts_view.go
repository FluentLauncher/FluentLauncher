package app

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"fluentlauncher/internal/core/shortcuts"
	"fluentlauncher/locales"
	"github.com/egoist/mygo/ui"
)

var shortcutMoreIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/></svg>`))
var shortcutEditIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"><path d="m15 4 5 5M4 20l5-1L20 8a2 2 0 0 0-5-5L4 14z"/></svg>`))

func (a *App) shortcutsPage(c *ui.Context) {
	theme := c.Theme()
	// The shared settings column spaces sections, not individual command rows.
	// Keep this page in one child and own its compact rhythm here.
	ui.Column(c).Gap(theme.Space(2)).Children(func() {
		if a.shortcutEdit.ID != "" {
			a.shortcutRecorder(c)
			return
		}
		ui.TextInput(c, &a.shortcutFilter).Height(theme.Space(8)).Placeholder(locales.ShortcutsFilter()).Label(locales.ShortcutsFilter())
		ui.Text(c, locales.ShortcutsCompactHint()).FontSize(theme.FontSize * 0.9).TextColor(theme.TextMuted).Wrap().Tooltip(locales.ShortcutsHint() + "\n" + locales.ShortcutsLocalHint())
		commands := a.shortcutCommands(a.settingsDraft)
		scopeOrder := map[shortcuts.Scope]int{shortcuts.Global: 0, shortcuts.Search: 1, shortcuts.Actions: 2, shortcuts.Prompt: 3, shortcuts.Preview: 4}
		sort.SliceStable(commands, func(i, j int) bool {
			left, right := commands[i], commands[j]
			if left.PluginID != "" || right.PluginID != "" {
				return left.PluginID == "" && right.PluginID != ""
			}
			return scopeOrder[left.Scope] < scopeOrder[right.Scope]
		})
		conflicts := shortcuts.Conflicts(commandDefinitions(commands), a.settingsDraft.Shortcuts)
		query := strings.ToLower(strings.TrimSpace(a.shortcutFilter))
		ui.Column(c).Gap(0).Children(func() {
			group := ""
			count := 0
			for _, command := range commands {
				binding := shortcuts.Effective(command.Command, a.settingsDraft.Shortcuts)
				if query != "" && !strings.Contains(strings.ToLower(command.Title+" "+command.Group+" "+command.PluginID+" "+strings.Join(binding.Keys, " ")), query) {
					continue
				}
				groupID := "builtin:" + string(command.Scope)
				if command.PluginID != "" {
					groupID = "plugin:" + command.PluginID
				}
				if groupID != group {
					group = groupID
					ui.Row(c).Padding(theme.Space(2), theme.Space(2)).Gap(theme.Space(3)).AlignItems(ui.Center).Children(func() {
						ui.Text(c, command.Group).Bold().TextColor(theme.TextMuted).Tooltip(command.PluginID)
						ui.Divider(c).Grow(1).Height(1)
					})
				}
				a.shortcutRow(c, command, binding, conflicts)
				count++
			}
			if count == 0 {
				ui.Text(c, locales.ShortcutsEmpty()).Padding(theme.Space(3), theme.Space(2)).TextColor(theme.TextMuted)
			}
		})
		if a.hotkeyNotice != "" {
			ui.Text(c, a.hotkeyNotice).TextColor(theme.Warning).Wrap()
		}
	})
}

func (a *App) shortcutRow(c *ui.Context, command shortcutCommand, binding shortcuts.Binding, conflicts []shortcuts.Conflict) {
	theme := c.Theme()
	issue := ""
	if binding.Enabled && len(binding.Keys) > 0 {
		if !command.Available {
			issue = locales.ShortcutsUnavailable()
		} else if command.Scope == shortcuts.Global {
			active := []string{}
			for key, id := range a.globalShortcuts.Active {
				if id == command.ID {
					active = append(active, key)
				}
			}
			sort.Strings(active)
			configured := append([]string{}, binding.Keys...)
			sort.Strings(configured)
			if len(active) == 0 {
				issue = locales.ShortcutsNotRegistered()
			} else if !slices.Equal(active, configured) {
				issue = locales.ShortcutsActual() + ": " + strings.Join(active, " / ")
			}
		}
	}
	for _, conflict := range conflicts {
		if conflict.First == command.ID || conflict.Second == command.ID {
			issue = locales.ShortcutsConflict() + ": " + conflict.Key
			break
		}
	}
	keys := strings.Join(binding.Keys, " / ")
	if keys == "" {
		keys = locales.ShortcutsUnbound()
	}
	summary := keys
	if len(binding.Keys) > 3 {
		summary = strings.Join(binding.Keys[:2], " / ") + fmt.Sprintf("  +%d", len(binding.Keys)-2)
	}
	c.Key(command.ID)
	row := ui.Row(c).Gap(theme.Space(2)).Padding(theme.Space(1), theme.Space(2)).AlignItems(ui.Center).Radius(theme.Radius)
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(theme.Space(0.5)).Children(func() {
			ui.Text(c, command.Title).TextColor(theme.Text).SingleLine().Tooltip(strings.TrimSpace(command.Title + "\n" + command.Description))
			if issue != "" {
				ui.Text(c, issue).FontSize(theme.FontSize * 0.85).TextColor(theme.Warning).Wrap()
			}
		})
		if command.PluginID != "" {
			ui.Text(c, shortcutScope(command.Scope)).FontSize(theme.FontSize * 0.85).TextColor(theme.TextMuted).SingleLine()
		}
		edit := ui.ButtonBase(c).Width(theme.Space(66)).Height(theme.Space(8)).Radius(theme.Radius).
			Border(1, theme.Border).Background(theme.Surface).Label(locales.CommonEdit() + ": " + command.Title).Tooltip(locales.CommonEdit() + " · " + keys)
		if edit.Hovered() {
			edit.Background(theme.SurfaceHover)
		}
		edit.Children(func() {
			ui.Row(c).Fill().Padding(0, theme.Space(2)).Gap(theme.Space(2)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, summary).Grow(1).SingleLine().TextColor(theme.Text)
				ui.Icon(c, shortcutEditIcon).FontSize(theme.FontSize).TextColor(theme.TextMuted)
			})
		})
		if edit.Clicked() {
			a.beginShortcutEdit(command.ID, binding.Keys)
		}
		enabled := binding.Enabled
		ui.Switch(c, &enabled).Label(locales.CommonEnable() + ": " + command.Title).Tooltip(locales.CommonEnable() + " / " + locales.CommonDisable()).OnClick(func() {
			if enabled && len(binding.Keys) == 0 {
				a.beginShortcutEdit(command.ID, nil)
				return
			}
			binding.Enabled = enabled
			a.setShortcutBinding(command.ID, binding, false)
		})
		moreLabel := locales.ShortcutsMore() + ": " + command.Title
		more := ui.ButtonBase(c).Size(theme.Space(7), theme.Space(7)).Radius(theme.Radius).Label(moreLabel).Tooltip(moreLabel)
		if more.Hovered() {
			more.Background(theme.SurfaceHover)
		}
		more.Children(func() { ui.Icon(c, shortcutMoreIcon).FontSize(theme.FontSize * 1.2).TextColor(theme.TextMuted) })
		more.PopoverMenuButton(func(m *ui.Menu) {
			if m.Item(locales.CommonEdit()).Chosen() {
				a.beginShortcutEdit(command.ID, binding.Keys)
			}
			m.Separator()
			if m.Item(locales.ShortcutsClear()).Chosen() {
				a.setShortcutBinding(command.ID, shortcuts.Binding{Keys: []string{}}, false)
			}
			if m.Item(locales.ShortcutsDefaults()).Chosen() {
				a.restoreShortcutDefaults(command)
			}
		})
	})
	if row.Hovered() {
		row.Background(theme.Surface.Alpha(0.35))
	}
}

func (a *App) restoreShortcutDefaults(command shortcutCommand) {
	candidate := cloneSettings(a.settingsDraft)
	delete(candidate.Shortcuts, command.ID)
	commands := a.shortcutCommands(candidate)
	for _, conflict := range shortcuts.Conflicts(commandDefinitions(commands), candidate.Shortcuts) {
		if conflict.First == command.ID || conflict.Second == command.ID {
			a.beginShortcutEdit(command.ID, command.Defaults)
			return
		}
	}
	a.settingsDraft = candidate
}

func (a *App) beginShortcutEdit(id string, keys []string) {
	a.shortcutEdit = shortcutEditor{ID: id, Keys: append([]string{}, keys...)}
	a.shortcutEdit.Focus.Focus()
}

func (a *App) shortcutRecorder(c *ui.Context) {
	theme := c.Theme()
	command := shortcutCommand{}
	for _, item := range a.shortcutCommands(a.settingsDraft) {
		if item.ID == a.shortcutEdit.ID {
			command = item
			break
		}
	}
	ui.Text(c, command.Title).Bold().TextColor(theme.Text)
	ui.Text(c, locales.ShortcutsRecordHint()).TextColor(theme.TextMuted).Wrap()
	ui.Box(c).Bind(&a.shortcutEdit.Focus).Focusable().AutoFocus().FillWidth().Padding(16, 12).
		Background(theme.Surface).Border(1, theme.Border).Radius(theme.Radius).
		Label(locales.ShortcutsRecordArea()).HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputKeyDown || ev.Key == ui.KeyTab {
			return false
		}
		if ev.Repeat {
			return true
		}
		key := ev.Key
		if key == ui.KeyUnknown {
			key = ev.PhysicalKey
		}
		if value, ok := chordFromUI(ev.Mods, key); ok {
			a.shortcutEdit.Recorded = value
			a.shortcutEdit.Error = ""
		}
		return true
	}).Children(func() {
		label := a.shortcutEdit.Recorded
		if label == "" {
			label = locales.ShortcutsRecordArea()
		}
		ui.Text(c, label).TextColor(theme.Text).Wrap()
	})
	ui.Row(c).Children(func() {
		if compactButton(c, locales.ShortcutsAddKey()).Disabled(a.shortcutEdit.Recorded == "").Clicked() {
			a.addRecordedShortcut(command.Scope)
		}
	})
	for index, key := range a.shortcutEdit.Keys {
		c.Key(key)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, key).Grow(1).TextColor(theme.Text)
			if compactButton(c, locales.CommonDelete()).Clicked() {
				a.shortcutEdit.Keys = append(a.shortcutEdit.Keys[:index:index], a.shortcutEdit.Keys[index+1:]...)
			}
		})
	}
	if a.shortcutEdit.Error != "" {
		ui.Text(c, a.shortcutEdit.Error).TextColor(theme.Danger).Wrap()
	}
	ui.Row(c).Gap(8).Children(func() {
		if ui.PrimaryButton(c, locales.CommonConfirm()).Clicked() {
			if a.shortcutEdit.Recorded != "" && !a.addRecordedShortcut(command.Scope) {
				return
			}
			if a.setShortcutBinding(command.ID, shortcuts.Binding{Keys: append([]string{}, a.shortcutEdit.Keys...), Enabled: len(a.shortcutEdit.Keys) > 0}, false) {
				a.shortcutEdit = shortcutEditor{}
			}
		}
		if compactButton(c, locales.CommonCancel()).Clicked() {
			a.shortcutEdit = shortcutEditor{}
		}
	})
	if strings.HasPrefix(a.shortcutEdit.Error, locales.ShortcutsConflict()) {
		if compactButton(c, locales.ShortcutsReplace()).Clicked() {
			if a.setShortcutBinding(command.ID, shortcuts.Binding{Keys: append([]string{}, a.shortcutEdit.Keys...), Enabled: len(a.shortcutEdit.Keys) > 0}, true) {
				a.shortcutEdit = shortcutEditor{}
			}
		}
	}
}

func (a *App) addRecordedShortcut(scope shortcuts.Scope) bool {
	key, err := shortcuts.ValidateKey(a.shortcutEdit.Recorded, scope)
	if err != nil {
		a.shortcutEdit.Error = err.Error()
		return false
	}
	if len(a.shortcutEdit.Keys) >= 12 {
		a.shortcutEdit.Error = locales.ShortcutsTooMany()
		return false
	}
	if !containsFold(a.shortcutEdit.Keys, key) {
		a.shortcutEdit.Keys = append(a.shortcutEdit.Keys, key)
	}
	a.shortcutEdit.Recorded = ""
	return true
}

func (a *App) setShortcutBinding(id string, binding shortcuts.Binding, replace bool) bool {
	value := cloneSettings(a.settingsDraft)
	value.Shortcuts[id] = binding
	commands := a.shortcutCommands(value)
	for _, command := range commands {
		if command.ID == id {
			for _, key := range binding.Keys {
				if _, err := shortcuts.ValidateKey(key, command.Scope); err != nil {
					a.shortcutEdit.Error = err.Error()
					return false
				}
			}
		}
	}
	for _, conflict := range shortcuts.Conflicts(commandDefinitions(commands), value.Shortcuts) {
		if conflict.First != id && conflict.Second != id {
			continue
		}
		other := conflict.First
		if other == id {
			other = conflict.Second
		}
		if !replace {
			if a.shortcutEdit.ID == "" {
				a.beginShortcutEdit(id, binding.Keys)
			}
			title := other
			for _, command := range commands {
				if command.ID == other {
					title = command.Title
					break
				}
			}
			a.shortcutEdit.Error = fmt.Sprintf("%s: %s · %s", locales.ShortcutsConflict(), conflict.Key, title)
			return false
		}
		for _, command := range commands {
			if command.ID != other {
				continue
			}
			previous := shortcuts.Effective(command.Command, value.Shortcuts)
			kept := []string{}
			for _, key := range previous.Keys {
				chord, err := shortcuts.Parse(key)
				if err != nil || chord.String() != conflict.Key {
					kept = append(kept, key)
				}
			}
			previous.Keys = kept
			previous.Enabled = previous.Enabled && len(kept) > 0
			value.Shortcuts[other] = previous
		}
	}
	if err := shortcuts.Validate(value.Shortcuts); err != nil {
		a.shortcutEdit.Error = err.Error()
		return false
	}
	a.settingsDraft = value
	a.settingsError = ""
	return true
}
