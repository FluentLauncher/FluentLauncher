package app

import (
	"fmt"
	"strconv"
	"strings"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/core/shortcuts"
	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/locales"
	"github.com/egoist/mygo/ui"
)

type shortcutCommand struct {
	shortcuts.Command
	Title, Group, Description string
	PluginCommand             protocol.Command
}

type shortcutEditor struct {
	ID              string
	Keys            []string
	Recorded, Error string
	Focus           ui.Handle
}

func shortcutTitle(id string) string {
	switch id {
	case shortcuts.Toggle:
		return locales.ShortcutsToggle()
	case "search.previous", "actions.previous":
		return locales.ShortcutsPrevious()
	case "search.next", "actions.next":
		return locales.ShortcutsNext()
	case "search.execute", "actions.execute":
		return locales.ShortcutsExecute()
	case "search.dismiss":
		return locales.ShortcutsDismissSearch()
	case "search.actions":
		return locales.ShortcutsOpenActions()
	case "actions.dismiss":
		return locales.ShortcutsBack()
	case "prompt.confirm":
		return locales.CommonConfirm()
	case "prompt.cancel":
		return locales.CommonCancel()
	case "preview.left":
		return locales.ShortcutsLeft()
	case "preview.right":
		return locales.ShortcutsRight()
	case "preview.up":
		return locales.ShortcutsUp()
	case "preview.down":
		return locales.ShortcutsDown()
	}
	if strings.HasPrefix(id, "actions.slot.") {
		n, _ := strconv.Atoi(strings.TrimPrefix(id, "actions.slot."))
		return fmt.Sprintf(locales.ShortcutsSlot(), n)
	}
	return id
}

func shortcutScope(scope shortcuts.Scope) string {
	switch scope {
	case shortcuts.Global:
		return locales.ShortcutsGlobal()
	case shortcuts.Search:
		return locales.ShortcutsSearch()
	case shortcuts.Actions:
		return locales.ShortcutsActions()
	case shortcuts.Prompt:
		return locales.ShortcutsPrompt()
	default:
		return locales.ShortcutsPreview()
	}
}

func (a *App) shortcutCommands(value settings.LauncherSettings) []shortcutCommand {
	var commands []shortcutCommand
	for _, command := range shortcuts.Builtins() {
		commands = append(commands, shortcutCommand{Command: command, Title: shortcutTitle(command.ID), Group: shortcutScope(command.Scope)})
	}
	if a.plugins != nil {
		for _, status := range a.plugins.Statuses() {
			manifest := status.Registration.Manifest
			for _, command := range manifest.Commands {
				commands = append(commands, shortcutCommand{
					Command: shortcuts.Command{ID: "plugin:" + manifest.ID + ":" + command.ID, Scope: command.Scope, Defaults: command.DefaultShortcuts, PluginID: manifest.ID,
						Available: containsFold(value.Plugins.EnabledIDs, manifest.ID) && !status.Paused && !status.Replacing},
					Title: command.Title, Group: manifest.Name, Description: command.Description, PluginCommand: command,
				})
			}
		}
	}
	return commands
}

func commandDefinitions(commands []shortcutCommand) []shortcuts.Command {
	definitions := make([]shortcuts.Command, 0, len(commands))
	for _, command := range commands {
		definitions = append(definitions, command.Command)
	}
	return definitions
}

func (a *App) commandBinding(id string) shortcuts.Binding {
	// Built-ins do not need a plugin status snapshot on the input path.
	for _, command := range shortcuts.Builtins() {
		if command.ID == id {
			return shortcuts.Effective(command, a.settings.Shortcuts)
		}
	}
	commands := a.shortcutCommands(a.settings)
	for _, command := range commands {
		if command.ID == id {
			binding := shortcuts.Effective(command.Command, a.settings.Shortcuts)
			if !command.Available {
				binding.Enabled = false
			}
			for _, conflict := range shortcuts.Conflicts(commandDefinitions(commands), a.settings.Shortcuts) {
				if conflict.Second == id {
					binding.Enabled = false
				}
			}
			return binding
		}
	}
	return shortcuts.Binding{}
}

func (a *App) primaryShortcutHint(id string) string {
	binding := a.commandBinding(id)
	if !binding.Enabled || len(binding.Keys) == 0 {
		return ""
	}
	return binding.Keys[0]
}

func (a *App) shortcutHint(id string) string {
	binding := a.commandBinding(id)
	if !binding.Enabled || len(binding.Keys) == 0 {
		return ""
	}
	return strings.Join(binding.Keys, " / ")
}

var shortcutSpecialKeys = map[string]ui.Key{
	"Enter": ui.KeyEnter, "Escape": ui.KeyEscape, "Space": ui.KeySpace, "Backspace": ui.KeyBackspace, "Delete": ui.KeyDelete, "Insert": ui.KeyInsert,
	"Home": ui.KeyHome, "End": ui.KeyEnd, "PageUp": ui.KeyPageUp, "PageDown": ui.KeyPageDown, "Left": ui.KeyLeft, "Right": ui.KeyRight, "Up": ui.KeyUp, "Down": ui.KeyDown, "Menu": ui.KeyContextMenu,
	"Minus": ui.KeyMinus, "Equal": ui.KeyEqual, "Comma": ui.KeyComma, "Period": ui.KeyPeriod, "Slash": ui.KeySlash, "Semicolon": ui.KeySemicolon, "Quote": ui.KeyQuote, "BracketLeft": ui.KeyBracketLeft, "BracketRight": ui.KeyBracketRight, "Backslash": ui.KeyBackslash, "Backquote": ui.KeyBackquote,
}

func shortcutUI(chord shortcuts.Chord) (ui.Modifiers, ui.Key) {
	var mods ui.Modifiers
	if chord.Ctrl {
		mods |= ui.Ctrl
	}
	if chord.Alt {
		mods |= ui.Alt
	}
	if chord.Shift {
		mods |= ui.Shift
	}
	if chord.Win {
		mods |= ui.Super
	}
	key := shortcutSpecialKeys[chord.Key]
	if len(chord.Key) == 1 {
		ch := chord.Key[0]
		if ch >= 'A' && ch <= 'Z' {
			key = ui.KeyA + ui.Key(ch-'A')
		}
		if ch >= '0' && ch <= '9' {
			key = ui.Key0 + ui.Key(ch-'0')
		}
	} else if strings.HasPrefix(chord.Key, "F") {
		n, _ := strconv.Atoi(chord.Key[1:])
		if n >= 1 && n <= 12 {
			key = ui.KeyF1 + ui.Key(n-1)
		}
	}
	return mods, key
}

func chordFromUI(mods ui.Modifiers, key ui.Key) (string, bool) {
	for _, name := range shortcuts.KeyNames() {
		_, candidate := shortcutUI(shortcuts.Chord{Key: name})
		if candidate == key {
			return (shortcuts.Chord{Ctrl: mods&ui.Ctrl != 0, Alt: mods&ui.Alt != 0, Shift: mods&ui.Shift != 0, Win: mods&ui.Super != 0, Key: name}).String(), true
		}
	}
	return "", false
}

func (a *App) matchesShortcut(id string, mods ui.Modifiers, key ui.Key) bool {
	if a.shortcutEdit.ID != "" {
		return false
	}
	binding := a.commandBinding(id)
	if !binding.Enabled {
		return false
	}
	for _, value := range binding.Keys {
		chord, err := shortcuts.Parse(value)
		if err != nil {
			continue
		}
		m, k := shortcutUI(chord)
		if m == mods && k == key {
			return true
		}
	}
	return false
}

func (a *App) shortcut(c *ui.Context, id string) bool {
	if a.shortcutEdit.ID != "" {
		return false
	}
	binding := a.commandBinding(id)
	if !binding.Enabled {
		return false
	}
	for _, value := range binding.Keys {
		chord, err := shortcuts.Parse(value)
		if err != nil {
			continue
		}
		mods, key := shortcutUI(chord)
		if c.Shortcut(mods, key) {
			return true
		}
	}
	return false
}

func (a *App) elementShortcut(c *ui.Context, element ui.Element, id string) bool {
	binding := a.commandBinding(id)
	if !binding.Enabled || a.shortcutEdit.ID != "" {
		return false
	}
	for _, value := range binding.Keys {
		chord, err := shortcuts.Parse(value)
		if err != nil {
			continue
		}
		mods, key := shortcutUI(chord)
		if element.Shortcut(mods, key) {
			return true
		}
	}
	return false
}

// Search input uses this before its editor consumes Enter, arrows or Menu.
func (a *App) searchTextKey(k ui.TextKey) bool {
	for _, id := range []string{"search.dismiss", "search.execute", "search.next", "search.previous", "search.actions"} {
		if a.matchesShortcut(id, k.Mods, k.Key) {
			a.runSearchCommand(id)
			return true
		}
	}
	for _, command := range a.shortcutCommands(a.settings) {
		if command.PluginID != "" && command.Scope == shortcuts.Search && command.Available && a.matchesShortcut(command.ID, k.Mods, k.Key) {
			a.runPluginCommand(command)
			return true
		}
	}
	return false
}

func (a *App) runSearchCommand(id string) {
	switch id {
	case "search.dismiss":
		if a.query != "" {
			a.query = ""
			a.notice = ""
			a.updateResults()
		} else {
			a.hidePanel()
		}
	case "search.execute":
		a.executeSelected(model.Open)
	case "search.next":
		a.choose(1)
	case "search.previous":
		a.choose(-1)
	case "search.actions":
		a.openActions()
	}
}
