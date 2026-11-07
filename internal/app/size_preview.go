package app

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/settings"
)

// panelSize is the size of the preview canvas, in DIPs. It fits the settings
// column at the window's minimum width.
const (
	previewWidth  float32 = 420
	previewHeight float32 = 210
	// previewGrip is how close to an edge the pointer resizes the panel.
	previewGrip float32 = 12
)

// panelSizeDrag is what a press on the preview keeps while it drags: the
// edge it grabbed and the size the panel had when it went down.
type panelSizeDrag struct {
	active         bool
	right, bottom  bool
	startX, startY float32
	width, height  float64
}

// panelSizePreview draws the launcher panel on a miniature of the display's
// work area, and sets its size when the pointer drags the panel's right
// edge, bottom edge or bottom-right corner. It replaces the number fields
// that asked for pixels: the proportions change while dragging.
func (a *App) panelSizePreview(c *ui.Context) {
	theme := c.Theme()
	workWidth, workHeight := float32(a.workArea.Width), float32(a.workArea.Height)
	if workWidth <= 0 || workHeight <= 0 {
		workWidth, workHeight = 1920, 1080
	}
	scale := min(previewWidth/workWidth, previewHeight/workHeight)
	deskWidth, deskHeight := workWidth*scale, workHeight*scale
	deskX, deskY := (previewWidth-deskWidth)/2, (previewHeight-deskHeight)/2
	panelWidth := float32(a.settingsDraft.Width) * scale
	panelHeight := float32(a.settingsDraft.Height) * scale
	panelWidth = min(panelWidth, deskWidth-8)
	panelHeight = min(panelHeight, deskHeight-8)
	panelX := deskX + (deskWidth-panelWidth)/2
	panelY := deskY + deskHeight*0.28
	panelY = min(panelY, deskY+deskHeight-panelHeight-4)
	panelY = max(panelY, deskY+4)

	ui.Column(c).Gap(8).Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "拖动面板的右边、下边或右下角调整大小").FontSize(11).TextColor(theme.TextMuted).Grow(1)
			ui.Text(c, fmt.Sprintf("%d × %d", a.settingsDraft.Width, a.settingsDraft.Height)).
				FontSize(12).TextColor(theme.Text)
			if ui.Button(c, "恢复默认").Clicked() {
				defaults := settings.Default()
				a.settingsDraft.Width = defaults.Width
				a.settingsDraft.Height = defaults.Height
				a.previewPanelSize()
			}
		})

		preview := ui.Box(c).Width(previewWidth).Height(previewHeight).Radius(10).
			Border(1, theme.Border).Background(theme.Surface.Alpha(0.3)).
			Focusable().Label("窗口尺寸预览")
		preview.Draw(func(p *ui.Painter, r ui.Rect) {
			// Painter coordinates are the window's: r is the canvas box.
			desk := ui.Rect{X: r.X + deskX, Y: r.Y + deskY, W: deskWidth, H: deskHeight}
			p.Fill(desk, theme.Background.Alpha(0.4), 6)
			p.Stroke(desk, theme.Border, 6, 1)
			panel := ui.Rect{X: r.X + panelX, Y: r.Y + panelY, W: panelWidth, H: panelHeight}
			p.Fill(panel, theme.Accent.Alpha(0.16), 4)
			p.Stroke(panel, theme.Accent, 4, 1.5)
			// A hint of the search field and the result rows, so that the
			// miniature reads as the panel it sizes.
			if panelWidth > 60 && panelHeight > 40 {
				p.Fill(ui.Rect{X: panel.X + 5, Y: panel.Y + 5, W: panel.W - 10, H: max(panel.H*0.16, 6)},
					theme.Surface.Alpha(0.85), 3)
				rowHeight := float32(5)
				for i := 0; i < 3; i++ {
					y := panel.Y + panel.H*0.32 + float32(i)*(rowHeight+4)
					if y+rowHeight > panel.Y+panel.H-5 {
						break
					}
					p.Fill(ui.Rect{X: panel.X + 7, Y: y, W: rowHeight, H: rowHeight},
						theme.TextMuted.Alpha(0.7), 1)
					p.Fill(ui.Rect{X: panel.X + 15, Y: y + 1, W: panel.W * 0.45, H: rowHeight - 2},
						theme.TextMuted.Alpha(0.45), 1)
				}
			}
			// The three grips, on the edges the pointer resizes by.
			grip := theme.Accent.Alpha(0.6)
			p.Fill(ui.Rect{X: panel.X + panel.W - 1.5, Y: panel.Y + 10, W: 3, H: max(panel.H-20, 0)}, grip, 1.5)
			p.Fill(ui.Rect{X: panel.X + 10, Y: panel.Y + panel.H - 1.5, W: max(panel.W-20, 0), H: 3}, grip, 1.5)
			p.Fill(ui.Rect{X: panel.X + panel.W - 8, Y: panel.Y + panel.H - 8, W: 10, H: 10}, theme.Accent, 2)
			if preview.FocusVisible() {
				p.FocusRing(r, [4]float32{10})
			}
		})

		x, y, over := preview.PointerPosition()
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
		}
		drag := ui.Local[panelSizeDrag](preview, "size-drag", func() panelSizeDrag { return panelSizeDrag{} })
		switch {
		case preview.Pressed() && !drag.active && (right || bottom):
			drag.active, drag.right, drag.bottom = true, right, bottom
			drag.startX, drag.startY = x, y
			drag.width, drag.height = float64(a.settingsDraft.Width), float64(a.settingsDraft.Height)
		case preview.Pressed() && drag.active:
			width, height := drag.width, drag.height
			if drag.right {
				width += float64(x-drag.startX) / float64(scale)
			}
			if drag.bottom {
				height += float64(y-drag.startY) / float64(scale)
			}
			a.settingsDraft.Width = clampInt(int(math.Round(width)), 480, 1600)
			a.settingsDraft.Height = clampInt(int(math.Round(height)), 320, 1200)
			a.previewPanelSize()
		case !preview.Pressed() && drag.active:
			drag.active = false
			a.previewPanelSize()
		}
		// The keyboard resizes the panel too, once the preview has the focus.
		switch {
		case preview.Shortcut(0, ui.KeyLeft):
			a.resizeDraft(-10, 0)
		case preview.Shortcut(0, ui.KeyRight):
			a.resizeDraft(10, 0)
		case preview.Shortcut(0, ui.KeyUp):
			a.resizeDraft(0, -10)
		case preview.Shortcut(0, ui.KeyDown):
			a.resizeDraft(0, 10)
		}
	})
}

// applyDraftSize is called by the preview's drag through previewPanelSize,
// which applies the size to the real panel without persisting it.
func (a *App) resizeDraft(dx, dy int) {
	a.settingsDraft.Width = clampInt(a.settingsDraft.Width+dx, 480, 1600)
	a.settingsDraft.Height = clampInt(a.settingsDraft.Height+dy, 320, 1200)
	a.previewPanelSize()
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
