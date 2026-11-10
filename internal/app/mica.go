package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/winapi/wallpaper"
)

// Load and sample wallpaper off the UI thread. Reopening the panel refreshes
// it at most once a minute; slider drags only reuse the cached two colors.
func (a *App) refreshMicaPalette() {
	if a.panel == nil || a.micaPaletteLoading || time.Since(a.micaPaletteAt) < time.Minute {
		return
	}
	a.micaPaletteLoading = true
	go func() {
		colors, ok := wallpaper.Palette()
		a.panel.Update(func() {
			a.micaPaletteLoading = false
			a.micaPaletteAt = time.Now()
			a.micaPalette = [2]ui.Color{}
			if ok {
				for i, color := range colors {
					a.micaPalette[i] = ui.RGB(color.R, color.G, color.B)
				}
			}
			if a.settingsWin != nil {
				a.settingsWin.Invalidate()
			}
			if a.workspaceWin != nil {
				a.workspaceWin.Invalidate()
			}
		})
	}()
}
