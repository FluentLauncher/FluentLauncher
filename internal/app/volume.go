package app

import (
	"fmt"
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/mistquay/fluentlauncher/internal/winapi/volume"
)

func (a *App) createVolumeOverlay() {
	a.volumeWin = mygo.NewWindow(mygo.WindowOptions{
		Hidden:        true,
		Frameless:     true,
		Transparent:   true,
		AlwaysOnTop:   true,
		SkipTaskbar:   true,
		DisableResize: true,
		DisableMove:   true,
		CornerRadius:  12,
		Width:         220,
		Height:        84,
		Content:       ui.View(a.volumeView),
	})
	a.volumeWin.SetIgnoreMouseEvents(true)
}

func (a *App) volumeView(c *ui.Context) {
	theme := *c.Theme()
	percent := a.volumePct
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	ui.Column(c).Fill().Padding(14).Gap(8).Background(theme.Surface).Radius(12).Children(func() {
		ui.Text(c, fmt.Sprintf("音量 %d%%", percent)).FontSize(16).Bold().TextColor(theme.Text)
		ui.Box(c).Height(6).Radius(3).Background(theme.Border).Children(func() {
			ui.Box(c).WidthPercent(float32(percent)).Height(6).Radius(3).Background(theme.Accent)
		})
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
		a.notice = "无法启用边缘滚轮调音量：" + err.Error()
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
	a.volumeWin.Update(func() { a.volumePct = percent })
	d := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	w, h := a.volumeWin.Size()
	a.volumeWin.SetPosition(
		d.WorkArea.X+(d.WorkArea.Width-w)/2,
		d.WorkArea.Y+d.WorkArea.Height-h-48,
	)
	a.volumeWin.ShowInactive()
	if a.volumeTimer != nil {
		a.volumeTimer.Stop()
	}
	a.volumeTimer = time.AfterFunc(1200*time.Millisecond, func() {
		if a.volumeWin != nil {
			a.volumeWin.Hide()
		}
	})
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
