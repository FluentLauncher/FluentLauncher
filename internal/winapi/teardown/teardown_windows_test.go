//go:build windows

package teardown

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestDestroyThreadWindows(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	createWindow := user32.NewProc("CreateWindowExW")
	isWindow := user32.NewProc("IsWindow")
	create := func(class string, style, parent uintptr) uintptr {
		name, _ := windows.UTF16PtrFromString(class)
		hwnd, _, err := createWindow.Call(0, uintptr(unsafe.Pointer(name)), 0, style, 0, 0, 10, 10, parent, 0, 0, 0)
		if hwnd == 0 {
			t.Fatalf("CreateWindowExW %s: %v", class, err)
		}
		t.Cleanup(func() { procDestroyWindow.Call(hwnd) })
		return hwnd
	}
	alive := func(hwnd uintptr) bool {
		ok, _, _ := isWindow.Call(hwnd)
		return ok != 0
	}
	const popup, child = 0x80000000, 0x40000000
	// Hidden system-class windows stand in for the framework's: two matching
	// top-level windows, a foreign helper parented to one of them as an input
	// method does, and a window of another class that has to stay.
	first := create("Static", popup, 0)
	second := create("Static", popup, 0)
	helper := create("Button", child, first)
	other := create("Edit", popup, 0)

	if got := DestroyThreadWindows("Stat"); got != 2 {
		t.Errorf("destroyed %d windows, want 2", got)
	}
	for name, hwnd := range map[string]uintptr{"first": first, "second": second, "helper": helper} {
		if alive(hwnd) {
			t.Errorf("the %s window survived", name)
		}
	}
	if !alive(other) {
		t.Error("a window of another class was destroyed")
	}
	if got := DestroyThreadWindows("Stat"); got != 0 {
		t.Errorf("a second pass destroyed %d windows, want 0", got)
	}
}
