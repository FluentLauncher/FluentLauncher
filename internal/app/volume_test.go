package app

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestVolumeViewUpdates(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprint("dark=", dark), func(t *testing.T) {
			a := newLocalizedApp()
			tt := ui.NewTester(a.volumeView, volumeOverlayWidth, volumeOverlayHeight)
			tt.SetDark(dark)
			for _, tc := range []struct {
				percent int
				label   string
			}{{64, "64"}, {0, "0"}, {100, "100"}, {-1, "0"}, {101, "100"}} {
				a.volumePct = tc.percent
				tt.Frame()
				r, ok := tt.Find(tc.label)
				if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > volumeOverlayWidth || r.Y+r.H > volumeOverlayHeight {
					t.Fatalf("volume %d is missing or outside the overlay: %+v", tc.percent, r)
				}
			}
		})
	}
}
