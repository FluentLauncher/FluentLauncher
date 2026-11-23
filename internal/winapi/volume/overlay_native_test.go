//go:build windows

package volume

import (
	"os"
	"testing"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Opt-in native geometry check; it opens only a test overlay and never changes
// the volume or the launcher's settings. Only the click test, behind a second
// switch, moves the pointer, and it puts it back.
// FL_VOLUME_NATIVE_TEST=1 go test ./internal/winapi/volume -run TestOverlayNative -count=1 -v
func TestMain(m *testing.M) {
	if os.Getenv("FL_VOLUME_NATIVE_TEST") != "1" {
		os.Exit(m.Run())
	}
	mygo.App.SetName("FluentLauncher volume test")
	mygo.App.OnWindowAllClosed(func() {})
	result := make(chan int, 1)
	mygo.App.WhenReady(func() {
		go func() {
			result <- m.Run()
			mygo.App.Quit()
		}()
	})
	if err := mygo.App.Run(); err != nil {
		panic(err)
	}
	os.Exit(<-result)
}

// A window that turns topmost after the overlay was created sits above it in
// the topmost band; showing the overlay has to raise it again.
func TestOverlayNativeAboveLaterTopmost(t *testing.T) {
	if os.Getenv("FL_VOLUME_NATIVE_TEST") != "1" {
		t.Skip("set FL_VOLUME_NATIVE_TEST=1 to check native window order")
	}
	mygo.RunOnMain(func() {
		options := mygo.WindowOptions{
			Title: "Volume order test", Hidden: true, Frameless: true,
			Transparent: true, AlwaysOnTop: true, SkipTaskbar: true,
			DisableResize: true, Width: 240, Height: 44,
			Content: ui.View(func(c *ui.Context) { ui.Text(c, "Volume 64") }),
		}
		win := mygo.NewWindow(options)
		defer win.Close()
		win.SetIgnoreMouseEvents(true)
		later := mygo.NewWindow(options)
		defer later.Close()
		later.SetIgnoreMouseEvents(true)
		later.ShowInactive()
		later.SetAlwaysOnTop(true)

		if err := PositionOverlay(win.NativeHandle(), 240, 44, 20); err != nil {
			t.Error(err)
			return
		}
		win.ShowInactive()
		getWindow := user32.NewProc("GetWindow")
		const previous = 3 // GW_HWNDPREV: the next window up in Z order
		for above, _, _ := getWindow.Call(win.NativeHandle(), previous); above != 0; above, _, _ = getWindow.Call(above, previous) {
			if above == later.NativeHandle() {
				t.Error("a window that became topmost later covers the overlay")
				return
			}
		}
	})
}

func TestOverlayNativeCrossDPI(t *testing.T) {
	if os.Getenv("FL_VOLUME_NATIVE_TEST") != "1" {
		t.Skip("set FL_VOLUME_NATIVE_TEST=1 to check native window positions")
	}
	mygo.RunOnMain(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "Volume placement test", Hidden: true, Frameless: true,
			Transparent: true, AlwaysOnTop: true, SkipTaskbar: true,
			DisableResize: true, Width: 240, Height: 44,
			Content: ui.View(func(c *ui.Context) { ui.Text(c, "Volume 64") }),
		})
		defer win.Close()
		win.SetIgnoreMouseEvents(true)
		hwnd := win.NativeHandle()
		getRect := user32.NewProc("GetWindowRect")
		foreground := user32.NewProc("GetForegroundWindow")
		focusBefore, _, _ := foreground.Call()
		displays := mygo.Screen.Displays()
		// Reuse the same window across every display and back, exercising both
		// increasing and decreasing DPI, including negative monitor origins.
		for i := 0; i < 2*len(displays); i++ {
			display := displays[i%len(displays)]
			info := monitorInfo{CbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
			if ok, _, err := procGetMonitorInfoW.Call(uintptr(display.ID), uintptr(unsafe.Pointer(&info))); ok == 0 {
				t.Errorf("GetMonitorInfoW: %v", err)
				return
			}
			dpi := uint32(display.ScaleFactor*96 + 0.5)
			want := overlayBounds(info.RcWork, dpi, 240, 44, 20)
			if err := positionOverlay(hwnd, want); err != nil {
				t.Error(err)
				return
			}
			win.ShowInactive()
			var got rect
			if ok, _, err := getRect.Call(hwnd, uintptr(unsafe.Pointer(&got))); ok == 0 {
				t.Errorf("GetWindowRect: %v", err)
				return
			}
			t.Logf("display=%s dpi=%d actual=%+v expected=%+v", display.Label, dpi, got, want)
			if got != want {
				t.Errorf("display %s: actual pixels %+v, want %+v", display.Label, got, want)
			}
		}
		if err := PositionOverlay(hwnd, 240, 44, 20); err != nil {
			t.Error(err)
		}
		if focusAfter, _, _ := foreground.Call(); focusAfter != focusBefore {
			t.Error("the overlay took keyboard focus")
		}
	})
}

// Clicking the overlay must leave the foreground window where it was. This
// one moves the pointer and clicks the test overlay, then puts the pointer
// back; run it only with the desktop free:
// FL_VOLUME_NATIVE_TEST=1 FL_VOLUME_NATIVE_CLICK=1 go test ./internal/winapi/volume -run TestOverlayNativeClickKeepsFocus -count=1 -v
func TestOverlayNativeClickKeepsFocus(t *testing.T) {
	if os.Getenv("FL_VOLUME_NATIVE_TEST") != "1" || os.Getenv("FL_VOLUME_NATIVE_CLICK") != "1" {
		t.Skip("set FL_VOLUME_NATIVE_TEST=1 and FL_VOLUME_NATIVE_CLICK=1 to click a test overlay")
	}
	clicked := make(chan struct{}, 1)
	var win *mygo.Window
	mygo.RunOnMain(func() {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title: "Volume click test", Hidden: true, Frameless: true,
			Transparent: true, AlwaysOnTop: true, SkipTaskbar: true,
			DisableResize: true, Width: 240, Height: 44,
			Content: ui.View(func(c *ui.Context) {
				if ui.ButtonBase(c).Fill().Label("Speaker").Clicked() {
					clicked <- struct{}{}
				}
			}),
		})
		if err := SetNoActivate(win.NativeHandle()); err != nil {
			t.Error(err)
		}
		if err := PositionOverlay(win.NativeHandle(), 240, 44, 20); err != nil {
			t.Error(err)
		}
		win.ShowInactive()
	})
	defer mygo.RunOnMain(func() { win.Close() })
	if t.Failed() {
		return
	}

	var bounds rect
	user32.NewProc("GetWindowRect").Call(win.NativeHandle(), uintptr(unsafe.Pointer(&bounds)))
	foreground := user32.NewProc("GetForegroundWindow")
	before, _, _ := foreground.Call()
	var saved point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&saved)))
	setCursorPos := user32.NewProc("SetCursorPos")
	defer setCursorPos.Call(uintptr(saved.X), uintptr(saved.Y))
	setCursorPos.Call(uintptr((bounds.Left+bounds.Right)/2), uintptr((bounds.Top+bounds.Bottom)/2))
	mouseEvent := user32.NewProc("mouse_event")
	const leftDown, leftUp = 0x0002, 0x0004
	mouseEvent.Call(leftDown, 0, 0, 0, 0)
	mouseEvent.Call(leftUp, 0, 0, 0, 0)

	select {
	case <-clicked:
	case <-time.After(2 * time.Second):
		t.Fatal("the overlay did not receive the click")
	}
	if after, _, _ := foreground.Call(); after != before {
		t.Errorf("clicking the overlay moved the foreground window from %#x to %#x", before, after)
	}
}
