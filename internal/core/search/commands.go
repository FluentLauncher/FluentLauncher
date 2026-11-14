package search

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/locales"
)

type Command uint8

const (
	CommandOpen Command = iota
	CommandRunAsAdministrator
	CommandOpenLocation
	CommandCopyPath
	CommandCopyValue
	CommandRegenerate
	CommandPluginAction
	CommandPluginPreview
	CommandSetAlias
	CommandTogglePinned
	CommandHide
	CommandRestoreHidden
)

type CommandItem struct {
	Command Command
	Title   string
	Value   string
}

// CommandsFor lists only the actions that apply to the entry. The context
// menu and the keyboard action panel share this list.
func CommandsFor(entry model.Entry, pinned, anyHidden bool) []CommandItem {
	if commands := toolCommands(entry); commands != nil {
		return commands
	}
	commands := []CommandItem{{Command: CommandOpen, Title: openTitle(entry)}}
	if entry.Kind == model.Application && !entry.RunElevated {
		commands = append(commands, CommandItem{Command: CommandRunAsAdministrator, Title: locales.CmdRunAsAdministrator()})
	}
	if hasLocalPath(entry) {
		title := locales.CmdOpenFileLocation()
		if entry.Kind == model.Directory {
			title = locales.CmdOpenInExplorer()
		}
		commands = append(commands,
			CommandItem{Command: CommandOpenLocation, Title: title},
			CommandItem{Command: CommandCopyPath, Title: locales.CmdCopyPath()})
	} else if entry.Kind == model.URL {
		commands = append(commands, CommandItem{Command: CommandCopyPath, Title: locales.CmdCopyUrl()})
	}
	for _, action := range entry.Actions {
		commands = append(commands, CommandItem{Command: CommandPluginAction, Title: action.Title, Value: action.ID})
	}
	if entry.PreviewText != "" {
		commands = append(commands, CommandItem{Command: CommandPluginPreview, Title: locales.CmdViewTextPreview()})
	}
	if learns(entry) {
		pinTitle := locales.CmdPin()
		if pinned {
			pinTitle = locales.CmdUnpin()
		}
		commands = append(commands,
			CommandItem{Command: CommandSetAlias, Title: locales.CmdSetAlias()},
			CommandItem{Command: CommandTogglePinned, Title: pinTitle},
			CommandItem{Command: CommandHide, Title: locales.CmdHide()})
	}
	if anyHidden {
		commands = append(commands, CommandItem{Command: CommandRestoreHidden, Title: locales.CmdRestoreHidden()})
	}
	return commands
}

// toolCommands lists the variants of an instant-tool result: alternative
// copies of the value, and a reroll for the random ones. Nil means "not a
// tool", and CommandsFor falls back to the generic list.
func toolCommands(entry model.Entry) []CommandItem {
	if entry.SourceID != "utilities" || entry.Kind != model.Calculation {
		return nil
	}
	value := entry.CopyText
	commands := []CommandItem{{Command: CommandOpen, Title: openTitle(entry)}}
	switch {
	case strings.HasPrefix(entry.ID, "uuid:"):
		// The formats are rows of their own; the panel only rerolls.
		commands = append(commands, CommandItem{Command: CommandRegenerate, Title: locales.CmdRegenerate()})
	case strings.HasPrefix(entry.ID, "date:"):
		// Every date row carries the ISO date as its key.
		if _, _, key, ok := splitFormatID(entry.ID); ok {
			if t, err := time.ParseInLocation("2006-01-02", key, time.Local); err == nil {
				commands = append(commands, CommandItem{Command: CommandCopyValue, Title: locales.CmdCopyTimestamp(),
					Value: strconv.FormatInt(t.Unix(), 10)})
			}
		}
	case strings.HasPrefix(entry.ID, "time:"):
		// The formats, with the date among them, are rows of their own.
	case strings.HasPrefix(entry.ID, "color:"):
		commands = append(commands,
			CommandItem{Command: CommandCopyValue, Title: locales.CmdCopyRGB(), Value: colorRGB(value)},
			CommandItem{Command: CommandCopyValue, Title: locales.CmdCopyHSL(), Value: colorHSL(value)})
	case strings.HasPrefix(entry.ID, "convert:"):
		commands = append(commands,
			CommandItem{Command: CommandCopyValue, Title: locales.CmdCopyWithUnit(), Value: entry.Title})
	case strings.HasPrefix(entry.ID, "rand:"), strings.HasPrefix(entry.ID, "pass:"):
		commands = append(commands, CommandItem{Command: CommandRegenerate, Title: locales.CmdRegenerate()})
	default:
		return nil
	}
	return commands
}

func colorRGB(hex string) string {
	r, g, b, ok := parseHexColor(hex)
	if !ok {
		return hex
	}
	return fmt.Sprintf("rgb(%d, %d, %d)", r, g, b)
}

func colorHSL(hex string) string {
	r, g, b, ok := parseHexColor(hex)
	if !ok {
		return hex
	}
	h, s, l := rgbToHSL(r, g, b)
	return fmt.Sprintf("hsl(%d, %d%%, %d%%)", h, s, l)
}

func parseHexColor(hex string) (int, int, int, bool) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(v >> 16 & 0xff), int(v >> 8 & 0xff), int(v & 0xff), true
}

func openTitle(entry model.Entry) string {
	switch entry.Kind {
	case model.URL:
		return locales.CmdOpenInBrowser()
	case model.Directory:
		return locales.CmdOpenFolder()
	case model.Calculation:
		if strings.HasPrefix(entry.ID, "calc:") {
			return locales.CmdCopyCalcResult()
		}
		return locales.CmdCopyValue()
	case model.Command:
		return locales.CmdOpen()
	case model.Window:
		return locales.CmdSwitchToWindow()
	case model.Workspace:
		return locales.CmdOpenWorkspace()
	}
	if entry.RunElevated {
		return locales.CmdOpenAsAdministrator()
	}
	return locales.CmdOpen()
}

func hasLocalPath(entry model.Entry) bool {
	switch entry.Kind {
	case model.Application, model.File, model.Directory:
	default:
		return false
	}
	return entry.LaunchTarget != "" && filepath.IsAbs(entry.LaunchTarget) &&
		!strings.HasPrefix(entry.LaunchTarget, `\\`)
}

func learns(entry model.Entry) bool {
	switch entry.Kind {
	case model.Application, model.File, model.Directory:
		return true
	}
	return false
}
