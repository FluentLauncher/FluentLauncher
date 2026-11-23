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

// The speaker follows the system flyout: a cross when muted, no waves at
// zero, then one or two waves as the level rises.
var (
	volumeMutedIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"><path d="M11 5 6 9H3v6h3l5 4z"/><path d="m16 9 6 6"/><path d="m22 9-6 6"/></svg>`))
	volumeZeroIcon  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"><path d="M11 5 6 9H3v6h3l5 4z"/></svg>`))
	volumeLowIcon   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"><path d="M11 5 6 9H3v6h3l5 4z"/><path d="M15.5 8.5a5 5 0 0 1 0 7"/></svg>`))
	volumeHighIcon  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"><path d="M11 5 6 9H3v6h3l5 4z"/><path d="M15.5 8.5a5 5 0 0 1 0 7"/><path d="M18.5 5.5a9 9 0 0 1 0 13"/></svg>`))
)

func volumeIconFor(percent int, muted bool) *ui.SVG {
	switch {
	case muted:
		return volumeMutedIcon
	case percent == 0:
		return volumeZeroIcon
	case percent < 50:
		return volumeLowIcon
	default:
		return volumeHighIcon
	}
}

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
	// Windows rounds the window and clips the blur to its corners; its own
	// 1px border would sit on the translucent plate as a light rim.
	if err := glass.HideBorder(a.volumeWin.NativeHandle()); err != nil {
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
	// A muted device keeps its level; the bar and number show it greyed
	// out, as the system flyout does, so a moving bar never hides the mute.
	fill, label := theme.Accent, theme.Text
	if a.volumeMuted {
		fill, label = theme.TextMuted, theme.TextMuted
	}
	// The plate fills the window without corners of its own: Windows rounds
	// the window, and a second radius would leave a ring of the blur between
	// the two outlines.
	ui.Row(c).Fill().Padding(8, 14).Gap(12).AlignItems(ui.Center).
		Background(theme.Surface).Children(func() {
		ui.Icon(c, volumeIconFor(percent, a.volumeMuted)).FontSize(18).TextColor(label)
		ui.Box(c).Grow(1).Height(5).Radius(2.5).Background(theme.Border.Alpha(0.6)).Children(func() {
			ui.Box(c).WidthPercent(float32(percent)).Height(5).Radius(2.5).Background(fill)
		})
		ui.Text(c, strconv.Itoa(percent)).FontSize(13).TextColor(label).Width(26)
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
	if state, err := volume.Get(); err == nil {
		a.volumePct, a.volumeMuted = state.Percent, state.Muted
	}
}

func (a *App) onVolumeChange(state volume.State) {
	if a.volumeWin == nil {
		return
	}
	// Runs on the hook's worker goroutine; the timer and the pin flag are
	// UI-thread state, so they move inside the window's update.
	a.volumeWin.Update(func() {
		a.volumePct, a.volumeMuted = state.Percent, state.Muted
		a.showVolumeOverlay()
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
}

// showVolumeOverlay puts the overlay near the bottom of the display the
// pointer is on, centered, and shows it without taking the focus.
func (a *App) showVolumeOverlay() {
	if a.volumeWin == nil {
		return
	}
	if err := volume.PositionOverlay(a.volumeWin.NativeHandle(), volumeOverlayWidth, volumeOverlayHeight, volumeOverlayGap); err != nil {
		log.Printf("volume overlay: %v", err)
	}
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
