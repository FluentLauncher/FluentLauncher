//go:build windows

// Package monitor places windows in physical pixels.
//
// MyGo's DIPs are not one coordinate space across monitors: a display's work
// area is converted with that display's DPI, while Window.SetBounds converts
// with the DPI of the monitor the window is on now. Moving a window to a
// monitor of another scale through DIPs therefore lands it wrong until it is
// already there. Pixels mean the same on every monitor.
package monitor

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	"fluentlauncher/internal/core/layout"
)

const (
	monitorDefaultToPrimary = 1
	monitorDefaultToNearest = 2
	mdtEffectiveDPI         = 0
	swpNoZOrder             = 0x0004
	swpNoActivate           = 0x0010
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	shcore = windows.NewLazySystemDLL("shcore.dll")

	procGetCursorPos      = user32.NewProc("GetCursorPos")
	procMonitorFromPoint  = user32.NewProc("MonitorFromPoint")
	procMonitorFromWindow = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW   = user32.NewProc("GetMonitorInfoW")
	procGetWindowRect     = user32.NewProc("GetWindowRect")
	procSetWindowPos      = user32.NewProc("SetWindowPos")
	procGetDpiForWindow   = user32.NewProc("GetDpiForWindow")
	procGetDpiForMonitor  = shcore.NewProc("GetDpiForMonitor")
)

type point struct{ X, Y int32 }

type rect struct{ Left, Top, Right, Bottom int32 }

type monitorInfo struct {
	CbSize    uint32
	RcMonitor rect
	RcWork    rect
	DwFlags   uint32
}

// Area is a monitor's work area in physical pixels and its scale (DPI / 96),
// the inputs layout.Calculate takes.
type Area struct {
	Work  layout.WorkArea
	Scale float64
}

// AtCursor returns the monitor under the pointer, or the nearest one.
func AtCursor() (Area, error) {
	var cursor point
	if ok, _, err := procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor))); ok == 0 {
		return Area{}, fmt.Errorf("GetCursorPos: %w", err)
	}
	return fromPoint(cursor, monitorDefaultToNearest)
}

// Primary returns the primary monitor, which always holds the point 0, 0.
func Primary() (Area, error) {
	return fromPoint(point{}, monitorDefaultToPrimary)
}

// OfWindow returns the window's bounds and the monitor that holds most of it,
// both in physical pixels.
func OfWindow(hwnd uintptr) (Area, layout.Rectangle, error) {
	var r rect
	if ok, _, err := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return Area{}, layout.Rectangle{}, fmt.Errorf("GetWindowRect: %w", err)
	}
	handle, _, err := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	if handle == 0 {
		return Area{}, layout.Rectangle{}, fmt.Errorf("MonitorFromWindow: %w", err)
	}
	area, err := describe(handle)
	return area, layout.Rectangle{X: int(r.Left), Y: int(r.Top), Width: int(r.Right - r.Left), Height: int(r.Bottom - r.Top)}, err
}

// Place moves and resizes the window to bounds in physical pixels, keeping
// its Z order and focus. Call it on the window's UI thread.
func Place(hwnd uintptr, bounds layout.Rectangle) error {
	place := func() error {
		if ok, _, err := procSetWindowPos.Call(hwnd, 0,
			uintptr(int32(bounds.X)), uintptr(int32(bounds.Y)),
			uintptr(int32(bounds.Width)), uintptr(int32(bounds.Height)),
			swpNoZOrder|swpNoActivate); ok == 0 {
			return fmt.Errorf("SetWindowPos: %w", err)
		}
		return nil
	}
	dpiBefore, _, _ := procGetDpiForWindow.Call(hwnd)
	if err := place(); err != nil {
		return err
	}
	if dpiAfter, _, _ := procGetDpiForWindow.Call(hwnd); dpiAfter != dpiBefore {
		// Crossing to a monitor of another DPI sends WM_DPICHANGED, which MyGo
		// answers with Windows' suggested rectangle: the old size rescaled,
		// not the one asked for. Place it again now that the DPI is settled.
		return place()
	}
	return nil
}

func fromPoint(p point, flags uintptr) (Area, error) {
	handle, _, err := procMonitorFromPoint.Call(uintptr(uint32(p.X))|uintptr(uint32(p.Y))<<32, flags)
	if handle == 0 {
		return Area{}, fmt.Errorf("MonitorFromPoint: %w", err)
	}
	return describe(handle)
}

func describe(handle uintptr) (Area, error) {
	info := monitorInfo{CbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, err := procGetMonitorInfoW.Call(handle, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return Area{}, fmt.Errorf("GetMonitorInfoW: %w", err)
	}
	var dpiX, dpiY uint32
	if hr, _, _ := procGetDpiForMonitor.Call(handle, mdtEffectiveDPI,
		uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY))); hr != 0 || dpiX == 0 {
		return Area{}, fmt.Errorf("GetDpiForMonitor: HRESULT %#x", hr)
	}
	work := info.RcWork
	return Area{
		Work:  layout.WorkArea{X: int(work.Left), Y: int(work.Top), Width: int(work.Right - work.Left), Height: int(work.Bottom - work.Top)},
		Scale: float64(dpiX) / 96,
	}, nil
}
