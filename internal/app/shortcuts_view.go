package app

import (
	"fmt"
	"strings"

	"fluentlauncher/internal/core/shortcuts"
	"fluentlauncher/locales"
	"github.com/egoist/mygo/ui"
)

func (a *App) shortcutsPage(c *ui.Context) {
	theme := c.Theme()
	ui.Text(c, locales.ShortcutsHint()).TextColor(theme.TextMuted).Wrap()
	if a.shortcutEdit.ID != "" {
		a.shortcutRecorder(c)
		return
	}
	ui.TextInput(c, &a.shortcutFilter).Placeholder(locales.ShortcutsFilter()).Label(locales.ShortcutsFilter())
	commands := a.shortcutCommands(a.settingsDraft)
	conflicts := shortcuts.Conflicts(commandDefinitions(commands), a.settingsDraft.Shortcuts)
	query := strings.ToLower(strings.TrimSpace(a.shortcutFilter))
	group := ""
	count := 0
	for _, command := range commands {
		binding := shortcuts.Effective(command.Command, a.settingsDraft.Shortcuts)
		if query != "" && !strings.Contains(strings.ToLower(command.Title+" "+command.Group+" "+command.PluginID+" "+strings.Join(binding.Keys, " ")), query) {
			continue
		}
		if command.Group != group {
			group = command.Group
			ui.Text(c, group).Bold().TextColor(theme.Text).Padding(8, 0)
		}
		c.Key(command.ID)
		ui.Column(c).Gap(6).Padding(10, 0).Children(func() {
			status := locales.ShortcutsDisabled()
			color := theme.TextMuted
			if binding.Enabled && len(binding.Keys) > 0 {
				if !command.Available {
					status = locales.ShortcutsUnavailable()
				} else {
					status = locales.ShortcutsActive()
					color = theme.Success
					if command.Scope == shortcuts.Global {
						active := []string{}
						for key, id := range a.globalShortcuts.Active {
							if id == command.ID {
								active = append(active, key)
							}
						}
						if len(active) == 0 {
							status = locales.ShortcutsDisabled()
							color = theme.Warning
						} else {
							status = locales.ShortcutsActual() + ": " + strings.Join(active, " / ")
						}
					}
				}
			}
			for _, conflict := range conflicts {
				if conflict.First == command.ID || conflict.Second == command.ID {
					status = locales.ShortcutsConflict() + ": " + conflict.Key
					color = theme.Warning
					break
				}
			}
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, command.Title).Grow(1).TextColor(theme.Text).Wrap()
				if command.PluginID != "" {
					ui.Text(c, shortcutScope(command.Scope)).TextColor(theme.TextMuted).SingleLine()
				}
				ui.Text(c, status).TextColor(color).Wrap()
			})
			if command.Description != "" {
				ui.Text(c, command.Description).TextColor(theme.TextMuted).Wrap()
			}
			keys := strings.Join(binding.Keys, " / ")
			if keys == "" {
				keys = locales.ShortcutsUnbound()
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, keys).Grow(1).TextColor(theme.Text).Wrap()
				if compactButton(c, locales.CommonEdit()).Clicked() {
					a.beginShortcutEdit(command.ID, binding.Keys)
				}
				label := locales.CommonEnable()
				if binding.Enabled {
					label = locales.CommonDisable()
				}
				if compactButton(c, label).Clicked() {
					if !binding.Enabled && len(binding.Keys) == 0 {
						a.beginShortcutEdit(command.ID, nil)
					} else {
						binding.Enabled = !binding.Enabled
						a.setShortcutBinding(command.ID, binding, false)
					}
				}
				if compactButton(c, locales.ShortcutsClear()).Clicked() {
					a.setShortcutBinding(command.ID, shortcuts.Binding{Keys: []string{}}, false)
				}
				if compactButton(c, locales.ShortcutsDefaults()).Clicked() {
					// Restoring a conflicting default opens an explicit replacement.
					candidate := cloneSettings(a.settingsDraft)
					delete(candidate.Shortcuts, command.ID)
					conflicted := false
					for _, conflict := range shortcuts.Conflicts(commandDefinitions(commands), candidate.Shortcuts) {
						if conflict.First == command.ID || conflict.Second == command.ID {
							conflicted = true
						}
					}
					if conflicted {
						a.beginShortcutEdit(command.ID, command.Defaults)
					} else {
						a.settingsDraft = candidate
					}
				}
			})
		})
		ui.Divider(c)
		count++
	}
	if count == 0 {
		ui.Text(c, locales.ShortcutsEmpty()).TextColor(theme.TextMuted)
	}
	ui.Text(c, locales.ShortcutsLocalHint()).TextColor(theme.TextMuted).Wrap()
	if a.hotkeyNotice != "" {
		ui.Text(c, a.hotkeyNotice).TextColor(theme.Warning).Wrap()
	}
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
