//go:build windows

package volume

import "testing"

// TestEdgeHitThickness pins what the hot zone thickness setting does: the
// pointer triggers within that many pixels of a selected edge, the same
// count on every edge, and not one pixel further in.
func TestEdgeHitThickness(t *testing.T) {
	screen := rect{Left: -1920, Top: 0, Right: 0, Bottom: 1080}
	for _, c := range []struct {
		name      string
		p         point
		thickness int32
		edges     Edges
		want      bool
	}{
		{"bottom last row", point{-960, 1079}, 1, EdgeBottom, true},
		{"bottom just outside 1", point{-960, 1078}, 1, EdgeBottom, false},
		{"bottom inside 6", point{-960, 1074}, 6, EdgeBottom, true},
		{"bottom outside 6", point{-960, 1073}, 6, EdgeBottom, false},
		{"top first row", point{-960, 0}, 1, EdgeTop, true},
		{"top just outside 1", point{-960, 1}, 1, EdgeTop, false},
		{"top inside 48", point{-960, 47}, 48, EdgeTop, true},
		{"top outside 48", point{-960, 48}, 48, EdgeTop, false},
		{"left first column", point{-1920, 500}, 1, EdgeLeft, true},
		{"left just outside 1", point{-1919, 500}, 1, EdgeLeft, false},
		{"right last column", point{-1, 500}, 1, EdgeRight, true},
		{"right outside 6", point{-7, 500}, 6, EdgeRight, false},
		{"unselected edge", point{-960, 1079}, 6, EdgeTop | EdgeLeft, false},
	} {
		if got := edgeHit(c.p, screen, c.thickness, c.edges); got != c.want {
			t.Errorf("%s: edgeHit(%+v, %d) = %v, want %v", c.name, c.p, c.thickness, got, c.want)
		}
	}
}

func TestEdgeThicknessScalesWithDPI(t *testing.T) {
	for _, c := range []struct {
		dip  int
		dpi  uint32
		want int32
	}{{4, 96, 4}, {4, 144, 6}, {32, 144, 48}, {1, 96, 1}, {0, 96, 1}} {
		if got := edgeThickness(c.dip, c.dpi); got != c.want {
			t.Errorf("edgeThickness(%d, %d) = %d, want %d", c.dip, c.dpi, got, c.want)
		}
	}
}
