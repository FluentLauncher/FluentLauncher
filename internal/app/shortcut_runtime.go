package app

import (
	"context"
	"log"
	"reflect"
	"strings"
	"time"

	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/core/shortcuts"
	"fluentlauncher/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func globalAccelerator(key string) string {
	replacer := strings.NewReplacer("BracketLeft", "[", "BracketRight", "]", "Backquote", "`", "Backslash", "\\", "Minus", "-", "Equal", "=", "Comma", ",", "Period", ".", "Slash", "/", "Semicolon", ";", "Quote", "'")
	return replacer.Replace(key)
}

func (a *App) initShortcutRegistry() {
	if a.globalShortcuts.Register != nil {
		return
	}
	a.globalShortcuts.Register = func(key, id string) error {
		return mygo.GlobalShortcut.Register(globalAccelerator(key), func() {
			if a.shortcutEdit.ID != "" {
				a.shortcutEdit.Recorded = key
				if a.settingsWin != nil {
					a.settingsWin.Invalidate()
				}
				return
			}
			if id == shortcuts.Toggle {
				a.toggle()
				return
			}
			for _, command := range a.shortcutCommands(a.settings) {
				if command.ID == id && command.Available {
					a.runPluginCommand(command)
					return
				}
			}
		})
	}
	a.globalShortcuts.Unregister = func(key string) { mygo.GlobalShortcut.Unregister(globalAccelerator(key)) }
}

func (a *App) desiredGlobalShortcuts(value settings.LauncherSettings) map[string]string {
	commands := a.shortcutCommands(value)
	blocked := map[string]bool{}
	for _, conflict := range shortcuts.Conflicts(commandDefinitions(commands), value.Shortcuts) {
		blocked[conflict.Second] = true
	}
	wanted := map[string]string{}
	for _, command := range commands {
		binding := shortcuts.Effective(command.Command, value.Shortcuts)
		if command.Scope != shortcuts.Global || !command.Available || !binding.Enabled || blocked[command.ID] {
			continue
		}
		for _, key := range binding.Keys {
			normal, err := shortcuts.ValidateKey(key, command.Scope)
			if err == nil {
				wanted[normal] = command.ID
			}
		}
	}
	return wanted
}

func (a *App) syncGlobalShortcuts(value settings.LauncherSettings, strict bool) error {
	a.initShortcutRegistry()
	wanted := a.desiredGlobalShortcuts(value)
	oldToggle := a.commandBinding(shortcuts.Toggle)
	newToggle := shortcuts.Effective(shortcuts.Builtins()[0], value.Shortcuts)
	// Keep an emergency opener across unrelated edits. Retrying the occupied
	// preferred key would otherwise make every plugin/local edit fail.
	if reflect.DeepEqual(oldToggle, newToggle) {
		for active, id := range a.globalShortcuts.Active {
			if id != shortcuts.Toggle || wanted[active] != "" {
				continue
			}
			occupied := false
			for _, command := range a.shortcutCommands(value) {
				binding := shortcuts.Effective(command.Command, value.Shortcuts)
				if command.ID == shortcuts.Toggle || !command.Available || !binding.Enabled {
					continue
				}
				for _, key := range binding.Keys {
					chord, err := shortcuts.Parse(key)
					if err == nil && chord.String() == active {
						occupied = true
					}
				}
			}
			if occupied {
				continue
			}
			for key, owner := range wanted {
				if owner == shortcuts.Toggle {
					delete(wanted, key)
				}
			}
			wanted[active] = shortcuts.Toggle
		}
	}
	err := a.globalShortcuts.Apply(wanted, strict)
	a.hotkeyActive = ""
	for key, owner := range a.globalShortcuts.Active {
		if owner == shortcuts.Toggle {
			a.hotkeyActive = key
			break
		}
	}
	if err != nil {
		a.hotkeyNotice = locales.ShortcutsRegisterFailed() + ": " + err.Error()
	} else {
		a.hotkeyNotice = ""
	}
	return err
}

func (a *App) registerHotkey() {
	if err := a.syncGlobalShortcuts(a.settings, false); err != nil {
		log.Printf("shortcuts: %v", err)
	}
	binding := a.commandBinding(shortcuts.Toggle)
	if !binding.Enabled || len(binding.Keys) == 0 {
		return
	}
	for key, id := range a.globalShortcuts.Active {
		if id == shortcuts.Toggle {
			a.hotkeyActive = key
			if !containsFold(binding.Keys, key) {
				a.hotkeyNotice = locales.HotkeyOccupiedFallback(strings.Join(binding.Keys, " / "), key)
			}
			return
		}
	}
	// Only the launcher opener gets an emergency fallback at startup/reload.
	for _, key := range []string{"Alt+Shift+Space", "Ctrl+Alt+Space"} {
		conflicts := false
		for _, command := range a.shortcutCommands(a.settings) {
			b := shortcuts.Effective(command.Command, a.settings.Shortcuts)
			if command.ID == shortcuts.Toggle || !command.Available || !b.Enabled {
				continue
			}
			for _, other := range b.Keys {
				chord, e := shortcuts.Parse(other)
				if e == nil && chord.String() == key {
					conflicts = true
				}
			}
		}
		if conflicts {
			continue
		}
		if err := a.globalShortcuts.Register(key, shortcuts.Toggle); err == nil {
			a.globalShortcuts.Active[key] = shortcuts.Toggle
			a.hotkeyActive = key
			a.hotkeyNotice = locales.HotkeyOccupiedFallback(strings.Join(binding.Keys, " / "), key)
			return
		}
	}
	a.hotkeyNotice = locales.HotkeyAllOccupied()
}

func (a *App) runPluginCommand(command shortcutCommand) {
	if a.shortcutEdit.ID != "" || !command.Available {
		return
	}
	if command.PluginCommand.Activation == "showQuery" {
		a.show()
		a.query = command.PluginCommand.Query
		a.updateResults()
		a.input.Focus()
		return
	}
	if a.pluginCommandBusy == nil {
		a.pluginCommandBusy = map[string]bool{}
	}
	if a.pluginCommandBusy[command.ID] {
		return
	}
	a.pluginCommandBusy[command.ID] = true
	a.hidePanel()
	manager := a.plugins
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		err := manager.ExecuteCommand(ctx, command.PluginID, command.PluginCommand.ID)
		a.publishUpdate(func() {
			delete(a.pluginCommandBusy, command.ID)
			if err != nil {
				a.notice = locales.ShortcutsPluginFailed() + ": " + err.Error()
				_ = mygo.NewNotification(mygo.NotificationOptions{Title: command.Title, Body: a.notice}).Show()
			}
		})
	}()
}

func (a *App) pluginShortcuts(c *ui.Context) {
	for _, command := range a.shortcutCommands(a.settings) {
		if command.PluginID == "" || command.Scope != shortcuts.Search || !command.Available {
			continue
		}
		if a.shortcut(c, command.ID) {
			a.runPluginCommand(command)
			return
		}
	}
}
