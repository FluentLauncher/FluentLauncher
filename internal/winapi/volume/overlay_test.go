//go:build windows

package volume

import "testing"

func TestOverlayBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		work rect
		dpi  uint32
		want rect
	}{
		{"primary150", rect{0, 0, 2560, 1528}, 144, rect{1100, 1432, 1460, 1498}},
		{"left100", rect{-1919, -491, 1, 541}, 96, rect{-1079, 477, -839, 521}},
		{"above100", rect{0, -1079, 1920, -47}, 96, rect{840, -111, 1080, -67}},
		{"right200", rect{2560, 0, 6400, 2080}, 192, rect{4240, 1952, 4720, 2040}},
		{"primary125", rect{0, 0, 1920, 1031}, 120, rect{810, 951, 1110, 1006}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := overlayBounds(tc.work, tc.dpi, 240, 44, 20); got != tc.want {
				t.Fatalf("overlay bounds = %+v, want %+v", got, tc.want)
			}
		})
	}
}
