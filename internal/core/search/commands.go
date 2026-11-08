package search

import (
	"path/filepath"
	"strings"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/locales"
)

type Command uint8

const (
	CommandOpen Command = iota
	CommandRunAsAdministrator
	CommandOpenLocation
	CommandCopyPath
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
