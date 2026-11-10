// Package shortcuts defines keyboard bindings without depending on a UI or OS.
package shortcuts

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Scope string

const (
	Global  Scope = "global"
	Search  Scope = "search"
	Actions Scope = "actions"
	Prompt  Scope = "prompt"
	Preview Scope = "preview"
	Toggle        = "launcher.toggle"
)

// Binding distinguishes an absent override from an explicitly cleared binding.
type Binding struct {
	Keys    []string `json:"Keys"`
	Enabled bool     `json:"Enabled"`
}

type Command struct {
	ID        string
	Scope     Scope
	Defaults  []string
	PluginID  string
	Available bool
}

func Builtins() []Command {
	commands := []Command{
		{ID: Toggle, Scope: Global, Defaults: []string{"Alt+Space"}},
		{ID: "search.previous", Scope: Search, Defaults: []string{"Up"}},
		{ID: "search.next", Scope: Search, Defaults: []string{"Down"}},
		{ID: "search.execute", Scope: Search, Defaults: []string{"Enter"}},
		{ID: "search.dismiss", Scope: Search, Defaults: []string{"Escape"}},
		{ID: "search.actions", Scope: Search, Defaults: []string{"Ctrl+K", "Shift+F10", "Menu"}},
		{ID: "actions.previous", Scope: Actions, Defaults: []string{"Up"}},
		{ID: "actions.next", Scope: Actions, Defaults: []string{"Down"}},
		{ID: "actions.execute", Scope: Actions, Defaults: []string{"Enter"}},
		{ID: "actions.dismiss", Scope: Actions, Defaults: []string{"Escape", "Ctrl+K"}},
		{ID: "prompt.confirm", Scope: Prompt, Defaults: []string{"Enter"}},
		{ID: "prompt.cancel", Scope: Prompt, Defaults: []string{"Escape"}},
		{ID: "preview.left", Scope: Preview, Defaults: []string{"Left"}},
		{ID: "preview.right", Scope: Preview, Defaults: []string{"Right"}},
		{ID: "preview.up", Scope: Preview, Defaults: []string{"Up"}},
		{ID: "preview.down", Scope: Preview, Defaults: []string{"Down"}},
	}
	for i := 1; i <= 9; i++ {
		commands = append(commands, Command{ID: "actions.slot." + strconv.Itoa(i), Scope: Actions, Defaults: []string{"Alt+" + strconv.Itoa(i)}})
	}
	for i := range commands {
		commands[i].Available = true
	}
	return commands
}

func Effective(command Command, overrides map[string]Binding) Binding {
	if binding, ok := overrides[command.ID]; ok {
		return binding
	}
	return Binding{Keys: append([]string{}, command.Defaults...), Enabled: command.PluginID == ""}
}

type Chord struct {
	Ctrl, Alt, Shift, Win bool
	Key                   string
}

// KeyNames is the shared vocabulary of the recorder, parser and UI adapter.
func KeyNames() []string {
	names := []string{"Enter", "Escape", "Space", "Backspace", "Delete", "Insert", "Home", "End", "PageUp", "PageDown", "Left", "Right", "Up", "Down", "Menu", "Minus", "Equal", "Comma", "Period", "Slash", "Semicolon", "Quote", "BracketLeft", "BracketRight", "Backslash", "Backquote"}
	for i := 1; i <= 12; i++ {
		names = append(names, "F"+strconv.Itoa(i))
	}
	for ch := '0'; ch <= '9'; ch++ {
		names = append(names, string(ch))
	}
	for ch := 'A'; ch <= 'Z'; ch++ {
		names = append(names, string(ch))
	}
	return names
}

func Parse(value string) (Chord, error) {
	var chord Chord
	parts := strings.Split(value, "+")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch strings.ToLower(part) {
		case "ctrl", "control":
			if chord.Ctrl {
				return chord, fmt.Errorf("duplicate modifier: %s", part)
			}
			chord.Ctrl = true
		case "alt":
			if chord.Alt {
				return chord, fmt.Errorf("duplicate modifier: %s", part)
			}
			chord.Alt = true
		case "shift":
			if chord.Shift {
				return chord, fmt.Errorf("duplicate modifier: %s", part)
			}
			chord.Shift = true
		case "win", "super":
			if chord.Win {
				return chord, fmt.Errorf("duplicate modifier: %s", part)
			}
			chord.Win = true
		default:
			if chord.Key != "" {
				return chord, fmt.Errorf("one key required: %s", value)
			}
			if strings.EqualFold(part, "esc") {
				part = "Escape"
			}
			if strings.EqualFold(part, "contextmenu") {
				part = "Menu"
			}
			for _, name := range KeyNames() {
				if strings.EqualFold(part, name) {
					chord.Key = name
					break
				}
			}
			if chord.Key == "" {
				return chord, fmt.Errorf("unsupported key: %s", part)
			}
		}
	}
	if chord.Key == "" {
		return chord, fmt.Errorf("key required: %s", value)
	}
	return chord, nil
}

func (c Chord) String() string {
	parts := []string{}
	if c.Ctrl {
		parts = append(parts, "Ctrl")
	}
	if c.Alt {
		parts = append(parts, "Alt")
	}
	if c.Shift {
		parts = append(parts, "Shift")
	}
	if c.Win {
		parts = append(parts, "Win")
	}
	return strings.Join(append(parts, c.Key), "+")
}

func ValidateKey(value string, scope Scope) (string, error) {
	c, err := Parse(value)
	if err != nil {
		return "", err
	}
	if c.Key == "Delete" && c.Ctrl && c.Alt || c.Key == "L" && c.Win || c.Key == "F4" && c.Alt || c.Key == "Escape" && c.Ctrl && c.Shift {
		return "", fmt.Errorf("system reserved shortcut: %s", c.String())
	}
	// Preserve the editor's standard editing commands in local text fields.
	if scope != Global && c.Ctrl && !c.Alt && !c.Shift && !c.Win && len(c.Key) == 1 && strings.Contains("ACVXZY", c.Key) {
		return "", fmt.Errorf("text editing shortcut: %s", c.String())
	}
	if scope == Global && (!c.Ctrl && !c.Alt && !c.Win || c.Key == "Menu") {
		return "", fmt.Errorf("global shortcut requires Ctrl, Alt or Win: %s", c.String())
	}
	return c.String(), nil
}

func Overlap(a, b Scope) bool { return a == Global || b == Global || a == b }

type Conflict struct{ Key, First, Second string }

// Conflicts includes only available, enabled commands. Ordering is stable so
// startup diagnostics and UI replacement always choose the same owner.
func Conflicts(commands []Command, overrides map[string]Binding) []Conflict {
	type owner struct {
		id    string
		scope Scope
	}
	used := map[string][]owner{}
	var conflicts []Conflict
	for _, command := range commands {
		binding := Effective(command, overrides)
		if !command.Available || !binding.Enabled {
			continue
		}
		for _, key := range binding.Keys {
			chord, err := Parse(key)
			if err != nil {
				continue
			}
			key = chord.String()
			for _, previous := range used[key] {
				if previous.id != command.ID && Overlap(previous.scope, command.Scope) {
					conflicts = append(conflicts, Conflict{key, previous.id, command.ID})
				}
			}
			used[key] = append(used[key], owner{command.ID, command.Scope})
		}
	}
	return conflicts
}

func Validate(overrides map[string]Binding) error {
	scopes := map[string]Scope{}
	for _, command := range Builtins() {
		scopes[command.ID] = command.Scope
	}
	ids := make([]string, 0, len(overrides))
	for id := range overrides {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		binding := overrides[id]
		if id == "" || len(binding.Keys) > 12 {
			return fmt.Errorf("invalid shortcut binding: %s", id)
		}
		seen := map[string]bool{}
		for _, key := range binding.Keys {
			// Uninstalled plugin overrides have no known scope. Keep their
			// syntax valid; the host validates scope when the plugin is present.
			chord, err := Parse(key)
			normal := chord.String()
			if scope, known := scopes[id]; known {
				normal, err = ValidateKey(key, scope)
			}
			if err != nil {
				return fmt.Errorf("%s: %w", id, err)
			}
			if seen[normal] {
				return fmt.Errorf("duplicate shortcut: %s", normal)
			}
			seen[normal] = true
		}
	}
	if conflicts := Conflicts(Builtins(), overrides); len(conflicts) > 0 {
		return fmt.Errorf("shortcut conflict: %s (%s / %s)", conflicts[0].Key, conflicts[0].First, conflicts[0].Second)
	}
	return nil
}
