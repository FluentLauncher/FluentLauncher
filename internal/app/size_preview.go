package app

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/settings"
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
	previewSnap float32 = 10
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
	anchorX        float64
	anchorY        float64
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
			ui.Text(c, "拖动面板移动位置，拖边缘调整大小").FontSize(11).TextColor(theme.TextMuted).Grow(1)
			ui.Text(c, fmt.Sprintf("%d × %d", a.settingsDraft.Width, a.settingsDraft.Height)).
				FontSize(12).TextColor(theme.Text)
			if ui.Button(c, "恢复默认").Clicked() {
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
			Focusable().Label("窗口尺寸与位置预览")
		preview.Draw(func(p *ui.Painter, r ui.Rect) {
			// Painter coordinates are the window's: r is the canvas box.
			desk := ui.Rect{X: r.X + deskX, Y: r.Y + deskY, W: deskWidth, H: deskHeight}
			p.Fill(desk, theme.Background.Alpha(0.35), 6)
			p.Stroke(desk, theme.Border, 6, 1)
			panel := ui.Rect{X: r.X + panelX, Y: r.Y + panelY, W: panelWidth, H: panelHeight}
			p.Clip(panel, 4, func() {
				p.Fill(panel, theme.Accent.Alpha(0.14), 4)
				// The miniature is drawn at a fixed size, as the real panel
				// lays out its rows: growing the panel shows more of them
				// instead of squeezing them all.
				field := ui.Rect{X: panel.X + 4, Y: panel.Y + 4, W: max(panel.W-8, 0), H: 9}
				p.Fill(field, theme.Surface.Alpha(0.9), 3)
				p.Line(panel.X, panel.Y+16, panel.X+panel.W, panel.Y+16, 1, theme.Border.Alpha(0.6))
				for i := 0; i < 12; i++ {
					y := panel.Y + 21 + float32(i)*14
					if y+11 > panel.Y+panel.H-12 {
						break
					}
					if i == 0 {
						p.Fill(ui.Rect{X: panel.X + 4, Y: y, W: panel.W - 8, H: 11}, theme.Accent.Alpha(0.75), 2)
					}
					p.Fill(ui.Rect{X: panel.X + 8, Y: y + 2, W: 7, H: 7}, theme.TextMuted.Alpha(0.6), 1)
					p.Fill(ui.Rect{X: panel.X + 19, Y: y + 3, W: panel.W * 0.45, H: 2}, theme.TextMuted.Alpha(0.55), 1)
					p.Fill(ui.Rect{X: panel.X + 19, Y: y + 7, W: panel.W * 0.3, H: 1.5}, theme.TextMuted.Alpha(0.3), 1)
				}
				// The footer along the bottom edge.
				p.Fill(ui.Rect{X: panel.X, Y: panel.Y + panel.H - 9, W: panel.W, H: 9}, theme.Surface.Alpha(0.45), 0)
				p.Fill(ui.Rect{X: panel.X + 5, Y: panel.Y + panel.H - 6, W: panel.W * 0.4, H: 2}, theme.TextMuted.Alpha(0.4), 1)
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
			drag.anchorX, drag.anchorY = a.settingsDraft.AnchorX, a.settingsDraft.AnchorY
		case preview.Pressed() && drag.active:
			if drag.move {
				// The pointer drags the panel inside the work area; its place
				// is kept as the share of the room around it, so the next
				// window opens there.
				movedX := panelX + (x - drag.startX)
				movedY := panelY + (y - drag.startY)
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
		case preview.Shortcut(0, ui.KeyLeft):
			a.settingsDraft.AnchorX = clampFloat01(a.settingsDraft.AnchorX - float64(step))
			a.previewPanelSize()
		case preview.Shortcut(0, ui.KeyRight):
			a.settingsDraft.AnchorX = clampFloat01(a.settingsDraft.AnchorX + float64(step))
			a.previewPanelSize()
		case preview.Shortcut(0, ui.KeyUp):
			a.settingsDraft.AnchorY = clampFloat01(a.settingsDraft.AnchorY - float64(step))
			a.previewPanelSize()
		case preview.Shortcut(0, ui.KeyDown):
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
