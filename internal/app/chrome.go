package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// captionBar draws the title bar of a window created without the system's
// own (mygo.WindowOptions.TitleBarStyle with HideWindowButtons): the mark,
// the title, the room a drag moves the window by, and the buttons that
// minimize and close it. The window keeps the shadow and the resizing edges
// of a titled one, so only the frame is gone.
func (a *App) captionBar(c *ui.Context, theme *ui.Theme, title string, win *mygo.Window) {
	row := ui.Row(c).Height(40).Padding(8, 12).Gap(10).AlignItems(ui.Center).DragWindow()
	row.Children(func() {
		if logo := a.brandBitmap(); logo != nil {
			ui.Image(c, logo).Size(18, 18).Radius(4)
		}
		ui.Text(c, title).FontSize(12).TextColor(theme.Text).Grow(1).SingleLine()
		a.windowButton(c, theme, "—", false, func() {
			if win != nil {
				win.Minimize()
			}
		})
		a.windowButton(c, theme, "✕", true, func() {
			if win != nil {
				win.Close()
			}
		})
	})
}

// windowButton is one of the window's own controls: a glyph on a plate that
// shows the theme's hover color, and the danger color for close.
func (a *App) windowButton(c *ui.Context, theme *ui.Theme, glyph string, close bool, action func()) {
	button := ui.ButtonBase(c).Size(34, 26).Radius(6).Label(glyph)
	var label ui.Element
	button.Children(func() {
		label = ui.Text(c, glyph).FontSize(11).TextColor(theme.Text)
	})
	if button.Hovered() {
		if close {
			button.Background(theme.Danger.Alpha(0.9))
			label.TextColor(theme.AccentText)
		} else {
			button.Background(theme.Surface.Alpha(0.6))
			label.TextColor(theme.Text)
		}
	}
	if button.Clicked() {
		action()
	}
}
