// Package volume holds the screen-edge hit rules and wheel accumulation.
// The Windows hook and the overlay both use these pure functions.
package volume

import (
	"math"

	"fluentlauncher/internal/core/settings"
)

type Rect struct {
	Left, Top, Right, Bottom int
}

// Find returns the selected edges whose trigger band contains the point.
// Corners can hit two edges; the caller handles one wheel event once.
func Find(monitor Rect, x, y int, selected settings.ScreenEdges, thickness int) settings.ScreenEdges {
	if x < monitor.Left || x >= monitor.Right || y < monitor.Top || y >= monitor.Bottom || thickness <= 0 {
		return settings.EdgeNone
	}
	var hit settings.ScreenEdges
	if selected&settings.EdgeTop != 0 && y < monitor.Top+thickness {
		hit |= settings.EdgeTop
	}
	if selected&settings.EdgeBottom != 0 && y >= monitor.Bottom-thickness {
		hit |= settings.EdgeBottom
	}
	if selected&settings.EdgeLeft != 0 && x < monitor.Left+thickness {
		hit |= settings.EdgeLeft
	}
	if selected&settings.EdgeRight != 0 && x >= monitor.Right-thickness {
		hit |= settings.EdgeRight
	}
	return hit
}

// RemoveShared drops edges that touch another monitor, so scrolling in the
// middle of a multi-monitor desktop does not change volume.
func RemoveShared(hit settings.ScreenEdges, monitor Rect, x, y int, hasMonitor func(int, int) bool) settings.ScreenEdges {
	if hit&settings.EdgeTop != 0 && hasMonitor(x, monitor.Top-1) {
		hit &^= settings.EdgeTop
	}
	if hit&settings.EdgeBottom != 0 && hasMonitor(x, monitor.Bottom) {
		hit &^= settings.EdgeBottom
	}
	if hit&settings.EdgeLeft != 0 && hasMonitor(monitor.Left-1, y) {
		hit &^= settings.EdgeLeft
	}
	if hit&settings.EdgeRight != 0 && hasMonitor(monitor.Right, y) {
		hit &^= settings.EdgeRight
	}
	return hit
}

func Thickness(dip int, dpi uint32) int {
	return max(1, int(math.Ceil(float64(dip)*float64(max(dpi, 96))/96)))
}

// WheelAccumulator turns high-resolution wheel deltas into whole notches.
type WheelAccumulator struct {
	remainder int
}

func (w *WheelAccumulator) Add(delta int) int {
	w.remainder += delta
	notches := w.remainder / 120
	w.remainder -= notches * 120
	return notches
}

func (w *WheelAccumulator) Reset() {
	w.remainder = 0
}

// NextVolume returns the new integer percentage, clamped to 0–100.
func NextVolume(currentScalar float64, notches, stepPercent int) int {
	value := int(math.Round(math.Min(math.Max(currentScalar, 0), 1)*100)) + notches*stepPercent
	return min(max(value, 0), 100)
}
