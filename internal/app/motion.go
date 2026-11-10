package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Keep feedback shorter than navigation. MyGo retargets these transitions
// during rapid input and follows the desktop's ReduceMotion preference.
var feedbackMotion = ui.Animation{Duration: 120 * time.Millisecond, Ease: ui.EaseOut}

var navigationMotion = ui.Animation{Duration: 180 * time.Millisecond, Ease: ui.EaseOut}

var feedbackTransition = ui.ElementTransition{Colors: true, Animation: &feedbackMotion}

// A shared identity connects the settings selection across sibling rows.
type settingsSelectionMarker struct{}

func searchFocus(row ui.Element, focused bool, theme *ui.Theme) {
	target := float32(0)
	if focused {
		target = 1
	}
	strength := row.AnimateTo("search-focus", target, feedbackMotion)
	row.Draw(func(p *ui.Painter, r ui.Rect) {
		// Draw inside the search row, without moving its input or divider.
		width := max(r.W-36, 0) * strength
		p.Fill(ui.Rect{X: r.X + (r.W-width)/2, Y: r.Y + r.H - 2, W: width, H: 2},
			withAlpha(theme.Accent, strength), 1)
	})
}
