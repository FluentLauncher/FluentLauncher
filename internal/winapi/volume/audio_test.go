//go:build windows

package volume

import "testing"

func TestNextLevel(t *testing.T) {
	for _, tc := range []struct {
		current, steps float32
		step           int
		want           int
	}{
		{0.5, 1, 2, 52},
		{0.5, -1, 2, 48},
		{0.5, 2, 5, 60},
		{0.99, 1, 2, 100},
		{0.01, -1, 2, 0},
		{0.5, 1, 0, 52}, // falls back to StepPercent
	} {
		if got := toPercent(nextLevel(tc.current, tc.steps, tc.step)); got != tc.want {
			t.Errorf("nextLevel(%v, %v, %d) = %d%%, want %d%%", tc.current, tc.steps, tc.step, got, tc.want)
		}
	}
}
