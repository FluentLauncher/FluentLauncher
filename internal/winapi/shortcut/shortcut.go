//go:build windows

// Package shortcut reads Windows .lnk targets through WScript.Shell.
package shortcut

import (
	"errors"
	"os"
	"runtime"
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
	// COM state belongs to the OS thread: keep the goroutine on the thread it
	// initialized until every object is released.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// S_FALSE means the thread already was in this apartment; it still takes
	// a matching CoUninitialize. Another apartment (RPC_E_CHANGED_MODE) works
	// as is and must not be uninitialized.
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err == nil || isSFalse(err) {
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
	// The VARIANT owns the shortcut object: ToIDispatch does not AddRef, and
	// Clear releases it. Releasing it as well freed it twice and crashed.
	defer result.Clear()
	shortcut := result.ToIDispatch()
	if shortcut == nil {
		return Target{}, false
	}
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

func isSFalse(err error) bool {
	var oleErr *ole.OleError
	return errors.As(err, &oleErr) && oleErr.Code() == 1 // S_FALSE
}

func property(dispatch *ole.IDispatch, name string) string {
	value, err := oleutil.GetProperty(dispatch, name)
	if err != nil || value == nil {
		return ""
	}
	defer value.Clear()
	return value.ToString()
}
