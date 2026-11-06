//go:build windows

// Package windowlist enumerates switchable top-level windows.
package windowlist

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/mistquay/fluentlauncher/internal/core/model"
)

var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	dwmapi                       = windows.NewLazySystemDLL("dwmapi.dll")
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procIsWindow                 = user32.NewProc("IsWindow")
	procIsIconic                 = user32.NewProc("IsIconic")
	procShowWindow               = user32.NewProc("ShowWindow")
	procBringWindowToTop         = user32.NewProc("BringWindowToTop")
	procGetWindow                = user32.NewProc("GetWindow")
	procGetWindowLongPtrW        = user32.NewProc("GetWindowLongPtrW")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowTextLengthW     = user32.NewProc("GetWindowTextLengthW")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procGetCurrentThreadId       = kernel32.NewProc("GetCurrentThreadId")
	procDwmGetWindowAttribute    = dwmapi.NewProc("DwmGetWindowAttribute")
)

const (
	gwOwner      = 4
	gwlExStyle   = ^uintptr(19) // -20
	wsExToolWin  = 0x00000080
	dwmwaCloaked = 14
	swRestore    = 9
	swShow       = 5
)

// List returns visible, non-tool, non-cloaked windows owned by other
// processes. It enumerates once per call; callers do not poll.
func List(ctx context.Context) ([]model.Entry, error) {
	self := windows.GetCurrentProcessId()
	entries := make([]model.Entry, 0, 32)
	callback := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		if ctx.Err() != nil {
			return 0
		}
		title, pid, ok := switchable(hwnd, self)
		if !ok {
			return 1
		}
		exe := processPath(pid)
		subtitle := "运行中的窗口 · 切换到此窗口"
		var aliases []string
		if exe != "" {
			name := filepath.Base(exe)
			name = name[:len(name)-len(filepath.Ext(name))]
			subtitle = name + " · 切换到此窗口"
			if name != "" {
				aliases = []string{name}
			}
		}
		entries = append(entries, model.Entry{
			ID:           fmt.Sprintf("window:%d", hwnd),
			Title:        title,
			Subtitle:     subtitle,
			LaunchTarget: strconv.FormatUint(uint64(hwnd), 10),
			Kind:         model.Window,
			SourceID:     "windows",
			IconPath:     exe,
			Aliases:      aliases,
		})
		return 1
	})
	r, _, err := procEnumWindows.Call(callback, 0)
	if r == 0 && err != nil && err != syscall.Errno(0) {
		return entries, err
	}
	return entries, nil
}

func switchable(hwnd uintptr, self uint32) (string, uint32, bool) {
	if !boolCall(procIsWindowVisible, hwnd) {
		return "", 0, false
	}
	if owner, _, _ := procGetWindow.Call(hwnd, gwOwner); owner != 0 {
		return "", 0, false
	}
	style, _, _ := procGetWindowLongPtrW.Call(hwnd, gwlExStyle)
	if style&wsExToolWin != 0 {
		return "", 0, false
	}
	var cloaked int32
	if r, _, _ := procDwmGetWindowAttribute.Call(hwnd, dwmwaCloaked,
		uintptr(unsafe.Pointer(&cloaked)), unsafe.Sizeof(cloaked)); r == 0 && cloaked != 0 {
		return "", 0, false
	}
	length, _, _ := procGetWindowTextLengthW.Call(hwnd)
	if length == 0 {
		return "", 0, false
	}
	n := int(length)
	buffer := make([]uint16, n+1)
	if r, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(n+1)); r == 0 {
		return "", 0, false
	}
	title := windows.UTF16ToString(buffer)
	if title == "" {
		return "", 0, false
	}
	var pid uint32
	procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 || pid == self {
		return "", 0, false
	}
	return title, pid, true
}

func processPath(pid uint32) string {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 1024)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buffer[:size])
}

// Activate restores and foregrounds a window, or explains why Windows
// refused the foreground switch.
func Activate(handleText string) model.Outcome {
	value, err := strconv.ParseUint(handleText, 10, 64)
	if err != nil || value == 0 || !boolCall(procIsWindow, uintptr(value)) {
		return model.Outcome{Error: "该窗口已经关闭，请重新搜索。"}
	}
	hwnd := uintptr(value)
	if boolCall(procIsIconic, hwnd) {
		procShowWindow.Call(hwnd, swRestore)
	}
	if boolCall(procSetForegroundWindow, hwnd) {
		return model.Outcome{Succeeded: true}
	}
	foreground, _, _ := procGetForegroundWindow.Call()
	var ignored uint32
	foregroundThread, _, _ := procGetWindowThreadProcessID.Call(foreground, uintptr(unsafe.Pointer(&ignored)))
	current, _, _ := procGetCurrentThreadId.Call()
	if foregroundThread != 0 && foregroundThread != current &&
		boolCall(procAttachThreadInput, current, foregroundThread, 1) {
		ok := boolCall(procSetForegroundWindow, hwnd)
		procAttachThreadInput.Call(current, foregroundThread, 0)
		if ok {
			return model.Outcome{Succeeded: true}
		}
	}
	procShowWindow.Call(hwnd, swShow)
	procBringWindowToTop.Call(hwnd)
	now, _, _ := procGetForegroundWindow.Call()
	if now == hwnd {
		return model.Outcome{Succeeded: true}
	}
	return model.Outcome{Error: "Windows 拒绝了前台切换。"}
}

func boolCall(proc *windows.LazyProc, args ...uintptr) bool {
	r, _, _ := proc.Call(args...)
	return r != 0
}
