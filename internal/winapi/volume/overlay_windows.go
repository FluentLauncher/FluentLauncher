//go:build windows

package volume

import (
	"fmt"
	"math"
	"unsafe"
)

var (
	procGetCursorPos    = user32.NewProc("GetCursorPos")
	procSetWindowPos    = user32.NewProc("SetWindowPos")
	procGetDpiForWindow = user32.NewProc("GetDpiForWindow")
)

// PositionOverlay centers a non-activating overlay above the work area's bottom
// on the monitor under the pointer. Call it on the window's UI thread. Sizes
// are DIPs; screen coordinates stay in physical pixels throughout, so moving
// from a monitor with a different DPI cannot put the overlay off screen.
func PositionOverlay(hwnd uintptr, width, height, gap int) error {
	var cursor point
	if ok, _, err := procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor))); ok == 0 {
		return fmt.Errorf("GetCursorPos: %w", err)
	}
	monitor, _, err := procMonitorFromPoint.Call(
		uintptr(uint32(cursor.X))|uintptr(uint32(cursor.Y))<<32, monitorNearest)
	if monitor == 0 {
		return fmt.Errorf("MonitorFromPoint: %w", err)
	}
	info := monitorInfo{CbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, err := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return fmt.Errorf("GetMonitorInfoW: %w", err)
	}
	var dpiX, dpiY uint32
	if hr, _, _ := procGetDpiForMonitor.Call(monitor, dpiEffective,
		uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY))); hr != 0 {
		return fmt.Errorf("GetDpiForMonitor: HRESULT %#x", hr)
	}
	return positionOverlay(hwnd, overlayBounds(info.RcWork, dpiX, width, height, gap))
}

func overlayBounds(work rect, dpi uint32, width, height, gap int) rect {
	px := func(dip int) int32 { return int32(math.Round(float64(dip) * float64(dpi) / 96)) }
	w, h := px(width), px(height)
	x := work.Left + (work.Right-work.Left-w)/2
	y := work.Bottom - h - px(gap)
	return rect{Left: x, Top: y, Right: x + w, Bottom: y + h}
}

func positionOverlay(hwnd uintptr, bounds rect) error {
	const noZOrderNoActivate = 0x0004 | 0x0010
	place := func() error {
		if ok, _, err := procSetWindowPos.Call(hwnd, 0,
			uintptr(bounds.Left), uintptr(bounds.Top),
			uintptr(bounds.Right-bounds.Left), uintptr(bounds.Bottom-bounds.Top),
			noZOrderNoActivate); ok == 0 {
			return fmt.Errorf("SetWindowPos: %w", err)
		}
		return nil
	}
	dpiBefore, _, _ := procGetDpiForWindow.Call(hwnd)
	if err := place(); err != nil {
		return err
	}
	if dpiAfter, _, _ := procGetDpiForWindow.Call(hwnd); dpiAfter != dpiBefore {
		// MyGo handles WM_DPICHANGED by applying Windows' suggested size. That
		// scales our already scaled pixel size again; restore it at the new DPI.
		return place()
	}
	return nil
}
