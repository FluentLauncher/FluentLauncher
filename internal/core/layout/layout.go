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

type Anchor struct {
	X, Y float64
}

func Calculate(work WorkArea, scale, logicalWidth, logicalHeight float64, anchor *Anchor) Rectangle {
	margin := int(math.Round(16 * scale))
	width := clamp(int(math.Ceil(logicalWidth*scale)), 1, max(1, work.Width-2*margin))
	height := clamp(int(math.Ceil(logicalHeight*scale)), 1, max(1, work.Height-2*margin))
	left := work.X + (work.Width-width)/2
	top := work.Y + int(math.Round(float64(work.Height)*0.28))
	if anchor != nil {
		left = work.X + int(math.Round(anchor.X*float64(work.Width)))
		top = work.Y + int(math.Round(anchor.Y*float64(work.Height)))
	}
	return Rectangle{
		X:      clamp(left, work.X+margin, work.X+work.Width-margin-width),
		Y:      clamp(top, work.Y+margin, work.Y+work.Height-margin-height),
		Width:  width,
		Height: height,
	}
}

func Capture(work WorkArea, x, y int) Anchor {
	return Anchor{
		X: clampFloat((float64(x-work.X))/float64(work.Width), 0, 1),
		Y: clampFloat((float64(y-work.Y))/float64(work.Height), 0, 1),
	}
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
