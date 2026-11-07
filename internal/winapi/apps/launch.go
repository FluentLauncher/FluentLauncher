//go:build windows

package apps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
	"golang.org/x/sys/windows"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/winapi/windowlist"
)

var (
	shell32            = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteEx = shell32.NewProc("ShellExecuteExW")
)

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	swShowNormal          = 1
)

type shellExecuteInfo struct {
	CbSize       uint32
	FMask        uint32
	Hwnd         uintptr
	LpVerb       *uint16
	LpFile       *uint16
	LpParameters *uint16
	LpDirectory  *uint16
	NShow        int32
	HInstApp     uintptr
	LpIDList     uintptr
	LpClass      *uint16
	HkeyClass    uintptr
	DwHotKey     uint32
	HIcon        uintptr
	HProcess     uintptr
}

// Launch opens an entry with the requested action.
func Launch(ctx context.Context, entry model.Entry, action model.EntryAction) model.Outcome {
	if err := ctx.Err(); err != nil {
		return model.Outcome{Error: err.Error()}
	}
	if entry.Kind == model.Window {
		return windowlist.Activate(entry.LaunchTarget)
	}
	switch entry.Kind {
	case model.Plugin, model.Calculation, model.Command, model.Workspace:
		return model.Outcome{Error: "此动作由对应来源处理。"}
	}
	if action == model.OpenLocation {
		if entry.LaunchTarget == "" || !filepath.IsAbs(entry.LaunchTarget) {
			return model.Outcome{Error: "此条目没有本地位置。"}
		}
		if info, err := os.Stat(entry.LaunchTarget); err == nil && info.IsDir() {
			if err := mygo.Shell.OpenPath(entry.LaunchTarget); err != nil {
				return model.Outcome{Error: err.Error()}
			}
			return model.Outcome{Succeeded: true}
		}
		mygo.Shell.ShowItemInFolder(entry.LaunchTarget)
		return model.Outcome{Succeeded: true}
	}
	if action == model.CopyPath || action == model.CopyText || entry.Kind == model.Calculation {
		return model.Outcome{Error: "此动作由对应来源处理。"}
	}
	runAs := action == model.RunAsAdministrator || (action == model.Open && entry.RunElevated)
	if runAs && entry.Kind != model.Application {
		return model.Outcome{Error: "仅应用支持以管理员身份运行。"}
	}
	if entry.Kind == model.URL {
		if err := mygo.Shell.OpenExternal(entry.LaunchTarget); err != nil {
			return model.Outcome{Error: err.Error()}
		}
		return model.Outcome{Succeeded: true}
	}
	if entry.Kind == model.Setting {
		// A settings page is a protocol URI that the shell's "open" verb hands
		// to its handler; a control panel applet or tool is a program that
		// needs ShellExecuteEx so that it receives its arguments.
		if entry.Arguments == "" {
			if err := mygo.Shell.OpenExternal(entry.LaunchTarget); err != nil {
				return model.Outcome{Error: err.Error()}
			}
			return model.Outcome{Succeeded: true}
		}
		return shellExecute(entry.LaunchTarget, entry.Arguments, entry.WorkingDir, false)
	}
	if entry.Arguments == "" && entry.WorkingDir == "" && !runAs {
		if entry.AppID != "" {
			if err := mygo.Shell.LaunchApp(mygo.InstalledApp{Name: entry.Title, ID: entry.AppID, Path: entry.IconPath}); err != nil {
				return model.Outcome{Error: err.Error()}
			}
			return model.Outcome{Succeeded: true}
		}
		if err := mygo.Shell.OpenPath(entry.LaunchTarget); err != nil {
			return model.Outcome{Error: err.Error()}
		}
		return model.Outcome{Succeeded: true}
	}
	return shellExecute(entry.LaunchTarget, entry.Arguments, entry.WorkingDir, runAs)
}

func shellExecute(file, arguments, directory string, runAs bool) model.Outcome {
	verb := "open"
	if runAs {
		verb = "runas"
	}
	verbPtr, err := windows.UTF16PtrFromString(verb)
	if err != nil {
		return model.Outcome{Error: err.Error()}
	}
	filePtr, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return model.Outcome{Error: err.Error()}
	}
	var argsPtr, dirPtr *uint16
	if arguments != "" {
		argsPtr, _ = windows.UTF16PtrFromString(arguments)
	}
	if directory != "" {
		dirPtr, _ = windows.UTF16PtrFromString(directory)
	}
	info := shellExecuteInfo{
		CbSize:       uint32(unsafe.Sizeof(shellExecuteInfo{})),
		FMask:        seeMaskNoCloseProcess | seeMaskNoAsync,
		LpVerb:       verbPtr,
		LpFile:       filePtr,
		LpParameters: argsPtr,
		LpDirectory:  dirPtr,
		NShow:        swShowNormal,
	}
	r, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		if errno, ok := callErr.(syscall.Errno); ok && errno == 1223 {
			return model.Outcome{Canceled: true, Error: "用户取消了系统权限确认。"}
		}
		if callErr != nil && callErr != syscall.Errno(0) {
			return model.Outcome{Error: callErr.Error()}
		}
		return model.Outcome{Error: fmt.Sprintf("ShellExecuteEx 失败：%v", windows.GetLastError())}
	}
	if info.HProcess != 0 {
		windows.CloseHandle(windows.Handle(info.HProcess))
	}
	return model.Outcome{Succeeded: true}
}
