//go:build windows

package monitor

import (
	"os"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/layout"
)

// Opt-in native geometry check; it opens only a hidden-then-shown test window
// and never moves the pointer or touches the launcher's settings.
// FL_MONITOR_NATIVE_TEST=1 go test ./internal/winapi/monitor -count=1 -v
func TestMain(m *testing.M) {
	if os.Getenv("FL_MONITOR_NATIVE_TEST") != "1" {
		os.Exit(m.Run())
	}
	mygo.App.SetName("FluentLauncher monitor test")
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

// The panel opens on the first try on every monitor, whatever the monitor it
// was on before: across rising and falling DPI and negative origins.
func TestPlaceNativeCrossDPI(t *testing.T) {
	if os.Getenv("FL_MONITOR_NATIVE_TEST") != "1" {
		t.Skip("set FL_MONITOR_NATIVE_TEST=1 to check native window positions")
	}
	mygo.RunOnMain(func() {
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "Monitor placement test", Hidden: true, Frameless: true,
			SkipTaskbar: true, Width: 760, Height: 520,
			Content: ui.View(func(c *ui.Context) { ui.Text(c, "placement") }),
		})
		defer win.Close()
		hwnd := win.NativeHandle()
		displays := mygo.Screen.Displays()
		for i := 0; i < 2*len(displays); i++ {
			display := displays[i%len(displays)]
			area, err := describe(uintptr(display.ID))
			if err != nil {
				t.Error(err)
				return
			}
			want := layout.Calculate(area.Work, area.Scale, 760, 520, &layout.Anchor{X: 0.5, Y: 0.3})
			if err := Place(hwnd, want); err != nil {
				t.Error(err)
				return
			}
			win.Show()
			gotArea, got, err := OfWindow(hwnd)
			if err != nil {
				t.Error(err)
				return
			}
			t.Logf("display=%s scale=%.2f actual=%+v expected=%+v", display.Label, area.Scale, got, want)
			if got != want {
				t.Errorf("display %s: actual pixels %+v, want %+v", display.Label, got, want)
			}
			if gotArea != area {
				t.Errorf("display %s: window reported on %+v, want %+v", display.Label, gotArea, area)
			}
			win.Hide()
		}
	})
}
