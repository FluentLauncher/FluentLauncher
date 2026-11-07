// Package layout places the launcher popup inside a monitor's work area.
// Sizes stay in logical DIPs; only the final rectangle is physical.
package layout

import "math"

type WorkArea struct {
	X, Y, Width, Height int
}

type Rectangle struct {
	X, Y, Width, Height int
}

// Anchor places the panel in the work area, as the share of the room left
// around it: X 0 is flush with the left edge, 1 with the right, 0.5
// centered; Y likewise from the top to the bottom.
type Anchor struct {
	X, Y float64
}

func Calculate(work WorkArea, scale, logicalWidth, logicalHeight float64, anchor *Anchor) Rectangle {
	margin := int(math.Round(16 * scale))
	width := clamp(int(math.Ceil(logicalWidth*scale)), 1, max(1, work.Width-2*margin))
	height := clamp(int(math.Ceil(logicalHeight*scale)), 1, max(1, work.Height-2*margin))
	left := work.X + (work.Width-width)/2
	top := work.Y + int(math.Round(float64(work.Height-height)*0.45))
	if anchor != nil {
		left = work.X + int(math.Round(anchor.X*float64(work.Width-width)))
		top = work.Y + int(math.Round(anchor.Y*float64(work.Height-height)))
	}
	return Rectangle{
		X:      clamp(left, work.X+margin, work.X+work.Width-margin-width),
		Y:      clamp(top, work.Y+margin, work.Y+work.Height-margin-height),
		Width:  width,
		Height: height,
	}
}

// Capture returns the anchor of a panel whose top-left corner is at x, y in
// the work area, as Calculate reads it.
func Capture(work WorkArea, x, y, width, height int) Anchor {
	anchor := Anchor{X: 0.5, Y: 0.45}
	if slack := work.Width - width; slack > 0 {
		anchor.X = clampFloat(float64(x-work.X)/float64(slack), 0, 1)
	}
	if slack := work.Height - height; slack > 0 {
		anchor.Y = clampFloat(float64(y-work.Y)/float64(slack), 0, 1)
	}
	return anchor
}

func clamp(v, low, high int) int {
	if high < low {
		return low
	}
	return min(max(v, low), high)
}

func clampFloat(v, low, high float64) float64 {
	return math.Min(math.Max(v, low), high)
}
