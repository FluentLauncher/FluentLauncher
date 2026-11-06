package search

import (
	"path/filepath"
	"strings"

	"github.com/mistquay/fluentlauncher/internal/core/model"
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
		commands = append(commands, CommandItem{Command: CommandRunAsAdministrator, Title: "以管理员身份运行"})
	}
	if hasLocalPath(entry) {
		title := "打开文件位置"
		if entry.Kind == model.Directory {
			title = "在资源管理器中打开"
		}
		commands = append(commands,
			CommandItem{Command: CommandOpenLocation, Title: title},
			CommandItem{Command: CommandCopyPath, Title: "复制路径"})
	} else if entry.Kind == model.URL {
		commands = append(commands, CommandItem{Command: CommandCopyPath, Title: "复制网址"})
	}
	for _, action := range entry.Actions {
		commands = append(commands, CommandItem{Command: CommandPluginAction, Title: action.Title, Value: action.ID})
	}
	if entry.PreviewText != "" {
		commands = append(commands, CommandItem{Command: CommandPluginPreview, Title: "查看文本预览"})
	}
	if learns(entry) {
		pinTitle := "固定到前面"
		if pinned {
			pinTitle = "取消固定"
		}
		commands = append(commands,
			CommandItem{Command: CommandSetAlias, Title: "设置别名"},
			CommandItem{Command: CommandTogglePinned, Title: pinTitle},
			CommandItem{Command: CommandHide, Title: "不再显示此条目"})
	}
	if anyHidden {
		commands = append(commands, CommandItem{Command: CommandRestoreHidden, Title: "恢复隐藏的条目"})
	}
	return commands
}

func openTitle(entry model.Entry) string {
	switch entry.Kind {
	case model.URL:
		return "在浏览器中打开"
	case model.Directory:
		return "打开文件夹"
	case model.Calculation:
		return "复制计算结果"
	case model.Command:
		return "打开"
	case model.Window:
		return "切换到此窗口"
	case model.Workspace:
		return "依次打开工作区"
	}
	if entry.RunElevated {
		return "以管理员身份打开"
	}
	return "打开"
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
