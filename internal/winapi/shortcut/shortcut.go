//go:build windows

// Package shortcut reads Windows .lnk targets through WScript.Shell.
package shortcut

import (
	"os"
	"strings"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

type Target struct {
	Path             string
	Arguments        string
	WorkingDirectory string
	IconPath         string
}

// Resolve reads a shortcut's target, arguments and working directory. It
// returns false when the shortcut cannot be read.
func Resolve(path string) (Target, bool) {
	initialized := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED) == nil
	if initialized {
		defer ole.CoUninitialize()
	}
	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		return Target{}, false
	}
	defer unknown.Release()
	dispatch, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return Target{}, false
	}
	defer dispatch.Release()
	result, err := oleutil.CallMethod(dispatch, "CreateShortcut", path)
	if err != nil {
		return Target{}, false
	}
	shortcut := result.ToIDispatch()
	defer result.Clear()
	if shortcut == nil {
		return Target{}, false
	}
	defer shortcut.Release()
	target := Target{
		Path:             property(shortcut, "TargetPath"),
		Arguments:        property(shortcut, "Arguments"),
		WorkingDirectory: property(shortcut, "WorkingDirectory"),
		IconPath:         property(shortcut, "IconLocation"),
	}
	if strings.TrimSpace(target.Path) == "" {
		return Target{}, false
	}
	target.Path = os.ExpandEnv(target.Path)
	target.Arguments = os.ExpandEnv(target.Arguments)
	target.WorkingDirectory = os.ExpandEnv(target.WorkingDirectory)
	if index := strings.Index(target.IconPath, ","); index >= 0 {
		target.IconPath = target.IconPath[:index]
	}
	target.IconPath = os.ExpandEnv(target.IconPath)
	return target, true
}

func property(dispatch *ole.IDispatch, name string) string {
	value, err := oleutil.GetProperty(dispatch, name)
	if err != nil || value == nil {
		return ""
	}
	defer value.Clear()
	return value.ToString()
}
