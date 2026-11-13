//go:build windows

package volume

import (
	"os"
	"testing"
	"unsafe"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Opt-in native geometry check; it opens only a test overlay and never changes
// the volume, pointer position or the launcher's settings.
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
