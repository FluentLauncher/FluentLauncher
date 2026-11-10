package app

import (
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/settings"
	"fluentlauncher/locales"
)

// The size of the preview canvas, in DIPs, and the parts of the miniature
// panel it draws. They fit the settings column at the window's minimum width.
const (
	previewWidth  float32 = 420
	previewHeight float32 = 210
	// previewGrip is how close to an edge the pointer resizes the panel.
	previewGrip float32 = 12
	// previewSnap is how close to an edge of the work area the panel snaps
	// flush with it, as windows do.
	previewSnap float32 = 8
)

// panelDrag is what a press on the preview keeps while it drags: whether it
// moves the panel or resizes it from its right or bottom edge, and the size
// and place the panel had when the press went down.
type panelDrag struct {
	active         bool
	move           bool
	right, bottom  bool
	startX, startY float32
	width, height  float64
	panelX, panelY float32
}

// panelSizePreview draws the launcher panel on a miniature of the display's
// work area: the pointer resizes it from the right edge, the bottom edge or
// the corner, and moves it by dragging its body, which is where the panel
// opens. The size and place apply to the real panel and are saved with the
// settings.
func (a *App) panelSizePreview(c *ui.Context) {
	theme := c.Theme()
	workWidth, workHeight := float32(a.workArea.Width), float32(a.workArea.Height)
	if workWidth <= 0 || workHeight <= 0 {
		workWidth, workHeight = 1920, 1080
	}
	scale := min(previewWidth/workWidth, previewHeight/workHeight)
	deskWidth, deskHeight := workWidth*scale, workHeight*scale
	deskX, deskY := (previewWidth-deskWidth)/2, (previewHeight-deskHeight)/2
	panelWidth := min(float32(a.settingsDraft.Width)*scale, deskWidth-8)
	panelHeight := min(float32(a.settingsDraft.Height)*scale, deskHeight-8)
	slackX, slackY := max(deskWidth-panelWidth, 0), max(deskHeight-panelHeight, 0)
	panelX := deskX + slackX*float32(a.settingsDraft.AnchorX)
	panelY := deskY + slackY*float32(a.settingsDraft.AnchorY)

	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SizePreviewHint()).FontSize(11).TextColor(theme.TextMuted).Grow(1)
			ui.Text(c, locales.SizePreviewDimensions(strconv.Itoa(a.settingsDraft.Width), strconv.Itoa(a.settingsDraft.Height))).
				FontSize(12).TextColor(theme.Text)
			if ui.Button(c, locales.SizePreviewReset()).Clicked() {
				defaults := settings.Default()
				a.settingsDraft.Width = defaults.Width
				a.settingsDraft.Height = defaults.Height
				a.settingsDraft.AnchorX = defaults.AnchorX
				a.settingsDraft.AnchorY = defaults.AnchorY
				a.previewPanelSize()
			}
		})

		preview := ui.Box(c).Width(previewWidth).Height(previewHeight).Radius(10).
			Border(1, theme.Border).Background(theme.Surface.Alpha(0.35)).
			Focusable().Label(locales.SizePreviewLabel())
		preview.Draw(func(p *ui.Painter, r ui.Rect) {
			// Painter coordinates are the window's: r is the canvas box.
			desk := ui.Rect{X: r.X + deskX, Y: r.Y + deskY, W: deskWidth, H: deskHeight}
			p.Fill(desk, theme.Background.Alpha(0.35), 6)
			p.Stroke(desk, theme.Border, 6, 1)
			panel := ui.Rect{X: r.X + panelX, Y: r.Y + panelY, W: panelWidth, H: panelHeight}
			p.Clip(panel, 4, func() {
				// The miniature cannot ask DWM for a second material inside
				// this window. Mica uses the same opaque theme fallback as
				// the real view when native materials are unavailable.
				if a.settingsDraft.Theme.Background == settings.BackgroundMica || a.settingsDraft.Theme.Background == settings.BackgroundAcrylic {
					drawBackgroundPlate(p, panel, theme, a.settingsDraft.Theme, false)
				} else {
					p.Fill(panel, theme.Accent.Alpha(0.14), 4)
				}
				// The miniature keeps the panel's real proportions: the
				// search field's height, the result rows' height, and as
				// many rows as the window height leaves room for, so that
				// dragging shows more of them instead of squeezing all.
				themeValue := a.settingsDraft.Theme
				searchH := max(float32(clampFloat(themeValue.SearchHeight, 36, 120))*scale, 6)
				rowH := max((float32(a.iconSize)+float32(themeValue.RowPadding))*scale, 5)
				footerH := max(24*scale, 5)
				field := ui.Rect{X: panel.X + 4, Y: panel.Y + 3, W: max(panel.W-8, 0), H: max(searchH-6, 3)}
				p.Fill(field, theme.Surface.Alpha(0.9), 3)
				p.Fill(ui.Rect{X: field.X + 4, Y: field.Y + field.H/2 - 1, W: max(field.W*0.4, 0), H: 2},
					theme.TextMuted.Alpha(0.55), 1)
				y := panel.Y + searchH
				for i := 0; i < 40; i++ {
					if y+rowH > panel.Y+panel.H-footerH {
						break
					}
					if i == 0 {
						p.Fill(ui.Rect{X: panel.X + 2, Y: y, W: max(panel.W-4, 0), H: rowH}, theme.Accent.Alpha(0.75), 2)
					}
					inset := max(rowH*0.18, 1)
					p.Fill(ui.Rect{X: panel.X + 5, Y: y + inset, W: max(rowH-2*inset, 1), H: max(rowH-2*inset, 1)},
						theme.TextMuted.Alpha(0.55), 1)
					p.Fill(ui.Rect{X: panel.X + 5 + rowH, Y: y + inset + 1, W: max(panel.W*0.4, 0), H: 2},
						theme.TextMuted.Alpha(0.5), 1)
					if rowH > 8 {
						p.Fill(ui.Rect{X: panel.X + 5 + rowH, Y: y + inset + 4, W: max(panel.W*0.28, 0), H: 1.5},
							theme.TextMuted.Alpha(0.28), 1)
					}
					y += rowH
				}
				// The footer along the bottom edge.
				p.Fill(ui.Rect{X: panel.X, Y: panel.Y + panel.H - footerH, W: panel.W, H: footerH},
					theme.Surface.Alpha(0.45), 0)
				p.Fill(ui.Rect{X: panel.X + 4, Y: panel.Y + panel.H - footerH/2, W: panel.W * 0.4, H: 2},
					theme.TextMuted.Alpha(0.4), 1)
			})
			p.Stroke(panel, theme.Accent, 4, 1.5)
			// The grips, on the edges the pointer resizes by.
			grip := theme.Accent.Alpha(0.6)
			p.Fill(ui.Rect{X: panel.X + panel.W - 1.5, Y: panel.Y + 10, W: 3, H: max(panel.H-20, 0)}, grip, 1.5)
			p.Fill(ui.Rect{X: panel.X + 10, Y: panel.Y + panel.H - 1.5, W: max(panel.W-20, 0), H: 3}, grip, 1.5)
			p.Fill(ui.Rect{X: panel.X + panel.W - 8, Y: panel.Y + panel.H - 8, W: 10, H: 10}, theme.Accent, 2)
			if preview.FocusVisible() {
				p.FocusRing(r, [4]float32{10})
			}
		})

		x, y, over := preview.PointerPosition()
		inPanel := over && x >= panelX && x <= panelX+panelWidth && y >= panelY && y <= panelY+panelHeight
		right := over && abs32(x-(panelX+panelWidth)) <= previewGrip &&
			y >= panelY-previewGrip && y <= panelY+panelHeight+previewGrip
		bottom := over && abs32(y-(panelY+panelHeight)) <= previewGrip &&
			x >= panelX-previewGrip && x <= panelX+panelWidth+previewGrip
		switch {
		case right && bottom:
			preview.Cursor(ui.CursorResizeNWSE)
		case right:
			preview.Cursor(ui.CursorResizeE)
		case bottom:
			preview.Cursor(ui.CursorResizeS)
		case inPanel:
			preview.Cursor(ui.CursorPointer)
		}
		drag := ui.Local[panelDrag](preview, "panel-drag", func() panelDrag { return panelDrag{} })
		switch {
		case preview.Pressed() && !drag.active && (right || bottom || inPanel):
			drag.active, drag.right, drag.bottom, drag.move = true, right, bottom, !right && !bottom
			drag.startX, drag.startY = x, y
			drag.width, drag.height = float64(a.settingsDraft.Width), float64(a.settingsDraft.Height)
			drag.panelX, drag.panelY = panelX, panelY
		case preview.Pressed() && drag.active:
			if drag.move {
				// The panel follows the pointer from where the press took
				// it; its place is kept as the share of the room around it,
				// so the next window opens there.
				movedX := drag.panelX + (x - drag.startX)
				movedY := drag.panelY + (y - drag.startY)
				movedX = max(deskX, min(movedX, deskX+slackX))
				movedY = max(deskY, min(movedY, deskY+slackY))
				movedX = snapEdge(movedX, deskX, deskX+slackX)
				movedY = snapEdge(movedY, deskY, deskY+slackY)
				if slackX > 0 {
					a.settingsDraft.AnchorX = clampFloat01(float64((movedX - deskX) / slackX))
				}
				if slackY > 0 {
					a.settingsDraft.AnchorY = clampFloat01(float64((movedY - deskY) / slackY))
				}
			} else {
				width, height := drag.width, drag.height
				if drag.right {
					width += float64(x-drag.startX) / float64(scale)
				}
				if drag.bottom {
					height += float64(y-drag.startY) / float64(scale)
				}
				a.settingsDraft.Width = clampInt(int(math.Round(width)), 480, 1600)
				a.settingsDraft.Height = clampInt(int(math.Round(height)), 320, 1200)
			}
			a.previewPanelSize()
		case !preview.Pressed() && drag.active:
			drag.active = false
			a.previewPanelSize()
		}
		// The keyboard moves the panel too, once the preview has the focus.
		step := float32(0.05)
		switch {
		case a.elementShortcut(c, preview, "preview.left"):
			a.settingsDraft.AnchorX = clampFloat01(a.settingsDraft.AnchorX - float64(step))
			a.previewPanelSize()
		case a.elementShortcut(c, preview, "preview.right"):
			a.settingsDraft.AnchorX = clampFloat01(a.settingsDraft.AnchorX + float64(step))
			a.previewPanelSize()
		case a.elementShortcut(c, preview, "preview.up"):
			a.settingsDraft.AnchorY = clampFloat01(a.settingsDraft.AnchorY - float64(step))
			a.previewPanelSize()
		case a.elementShortcut(c, preview, "preview.down"):
			a.settingsDraft.AnchorY = clampFloat01(a.settingsDraft.AnchorY + float64(step))
			a.previewPanelSize()
		}
	})
}

// resizeDraft moves the draft size by the given pixels, which the tests use
// to check the bounds and the keyboard could use for fine steps.
func (a *App) resizeDraft(dx, dy int) {
	a.settingsDraft.Width = clampInt(a.settingsDraft.Width+dx, 480, 1600)
	a.settingsDraft.Height = clampInt(a.settingsDraft.Height+dy, 320, 1200)
	a.previewPanelSize()
}

// snapEdge pulls v flush with the start or the end of the room when it is
// close enough, as windows snap to the edges of a screen.
func snapEdge(v, from, to float32) float32 {
	if v-from <= previewSnap {
		return from
	}
	if to-v <= previewSnap {
		return to
	}
	return v
}

func clampFloat01(v float64) float64 {
	return math.Min(math.Max(v, 0), 1)
}

func clampInt(value, low, high int) int {
	return min(max(value, low), high)
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
