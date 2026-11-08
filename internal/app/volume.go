package app

import (
	"log"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/winapi/glass"
	"fluentlauncher/internal/winapi/volume"
	"fluentlauncher/locales"
)

// The volume overlay is a small pill near the bottom of the screen: a
// speaker, the bar and the percentage, over the blurred desktop.
const (
	volumeOverlayWidth  = 240
	volumeOverlayHeight = 44
	volumeOverlayGap    = 20
)

var volumeIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"><path d="M11 5 6 9H3v6h3l5 4z"/><path d="M15.5 8.5a5 5 0 0 1 0 7"/><path d="M18.5 5.5a9 9 0 0 1 0 13"/></svg>`))

func (a *App) createVolumeOverlay() {
	a.volumeWin = mygo.NewWindow(mygo.WindowOptions{
		Hidden:        true,
		Frameless:     true,
		Transparent:   true,
		AlwaysOnTop:   true,
		SkipTaskbar:   true,
		DisableResize: true,
		CornerRadius:  16,
		Width:         volumeOverlayWidth,
		Height:        volumeOverlayHeight,
		Content:       ui.View(a.volumeView),
	})
	a.volumeWin.SetIgnoreMouseEvents(true)
	// The overlay blurs what is behind it, as the system's own volume flyout
	// does, so that it reads on any wallpaper.
	if err := glass.SetBlur(a.volumeWin.NativeHandle(), true); err != nil {
		log.Printf("volume overlay: %v", err)
	}
}

func (a *App) volumeView(c *ui.Context) {
	theme := *c.Theme()
	a.applyUITheme(&theme)
	c.SetTheme(&theme)
	percent := a.volumePct
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	ui.Row(c).Fill().Padding(8, 14).Gap(12).AlignItems(ui.Center).Radius(theme.Radius + 6).
		Background(theme.Surface).Children(func() {
		ui.Icon(c, volumeIcon).FontSize(18).TextColor(theme.Text)
		ui.Box(c).Grow(1).Height(5).Radius(2.5).Background(theme.Border.Alpha(0.6)).Children(func() {
			ui.Box(c).WidthPercent(float32(percent)).Height(5).Radius(2.5).Background(theme.Accent)
		})
		ui.Text(c, strconv.Itoa(percent)).FontSize(13).TextColor(theme.Text).Width(26)
	})
}

func (a *App) applyVolumeSettings() {
	a.stopVolume()
	if !a.settings.VolumeWheel.Enabled {
		return
	}
	controller, err := volume.Start(volume.Options{
		Edges:        volume.Edges(a.settings.VolumeWheel.Edges),
		ThicknessDIP: a.settings.VolumeWheel.ThicknessDip,
		StepPercent:  a.settings.VolumeWheel.StepPercent,
		OnChange:     a.onVolumeChange,
	})
	if err != nil {
		log.Printf("volume hook: %v", err)
		a.notice = locales.VolumeWheelError(err.Error())
		return
	}
	a.volume = controller
	if percent, err := volume.Get(); err == nil {
		a.volumePct = percent
	}
}

func (a *App) onVolumeChange(percent int) {
	if a.volumeWin == nil {
		return
	}
	// Runs on the hook's worker goroutine; the timer and the pin flag are
	// UI-thread state, so they move inside the window's update.
	a.volumeWin.Update(func() {
		a.volumePct = percent
		if a.volumeTimer != nil {
			a.volumeTimer.Stop()
		}
		if a.volumePinned {
			return
		}
		a.volumeTimer = time.AfterFunc(1200*time.Millisecond, func() {
			if a.volumeWin != nil {
				a.volumeWin.Hide()
			}
		})
	})
	a.showVolumeOverlay()
}

// showVolumeOverlay puts the overlay near the bottom of the display the
// pointer is on, centered, and shows it without taking the focus.
func (a *App) showVolumeOverlay() {
	if a.volumeWin == nil {
		return
	}
	display := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	if display.WorkArea.Width <= 0 || display.WorkArea.Height <= 0 {
		display = mygo.Screen.PrimaryDisplay()
	}
	work := mygo.Rectangle{X: display.WorkArea.X, Y: display.WorkArea.Y, Width: display.WorkArea.Width, Height: display.WorkArea.Height}
	if work.Width <= 0 || work.Height <= 0 {
		work = mygo.Rectangle{Width: 1920, Height: 1080}
	}
	x := work.X + (work.Width-volumeOverlayWidth)/2
	y := work.Y + work.Height - volumeOverlayHeight - volumeOverlayGap
	a.volumeWin.SetBounds(mygo.Rectangle{X: x, Y: y, Width: volumeOverlayWidth, Height: volumeOverlayHeight})
	a.volumeWin.ShowInactive()
}

func (a *App) stopVolume() {
	if a.volume != nil {
		a.volume.Stop()
		a.volume = nil
	}
	if a.volumeTimer != nil {
		a.volumeTimer.Stop()
		a.volumeTimer = nil
	}
	if a.volumeWin != nil {
		a.volumeWin.Hide()
	}
}
