//go:build windows

// Package teardown removes what must not outlive the Go runtime's last moments.
package teardown

import (
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                = windows.NewLazySystemDLL("user32.dll")
	procEnumThreadWindows = user32.NewProc("EnumThreadWindows")
	procGetClassNameW     = user32.NewProc("GetClassNameW")
	procDestroyWindow     = user32.NewProc("DestroyWindow")
)

// DestroyThreadWindows destroys the calling thread's top-level windows whose
// class name starts with classPrefix, with everything they own or parent, and
// returns how many it destroyed. Call it on the thread that owns the windows,
// after its message loop has stopped.
//
// A window with a Go window procedure must not survive into ExitProcess. The
// runtime holds a lock of its own across that call, and DLLs still tear down
// inside it: an input method that parented a helper window to ours destroys it
// there, Windows notifies the parent, and the callback into Go throws and then
// waits for that lock forever. The process stays behind with one thread.
func DestroyThreadWindows(classPrefix string) int {
	var found []uintptr
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var name [256]uint16
		n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)))
		if strings.HasPrefix(windows.UTF16ToString(name[:n]), classPrefix) {
			found = append(found, hwnd)
		}
		return 1
	})
	procEnumThreadWindows.Call(uintptr(windows.GetCurrentThreadId()), callback, 0)
	destroyed := 0
	for _, hwnd := range found {
		// One of them may own another, which is then gone already.
		if ok, _, _ := procDestroyWindow.Call(hwnd); ok != 0 {
			destroyed++
		}
	}
	return destroyed
}
