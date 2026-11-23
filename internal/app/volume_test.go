package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/winapi/volume"
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
				a.volumePct, a.volumeMuted = tc.percent, tc.percent == 0
				tt.Frame()
				r, ok := tt.Find(tc.label)
				if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > volumeOverlayWidth || r.Y+r.H > volumeOverlayHeight {
					t.Fatalf("volume %d is missing or outside the overlay: %+v", tc.percent, r)
				}
			}
		})
	}
}

func TestVolumeIconFor(t *testing.T) {
	for _, tc := range []struct {
		percent int
		muted   bool
		want    *ui.SVG
	}{
		{64, true, volumeMutedIcon},
		{0, true, volumeMutedIcon},
		{0, false, volumeZeroIcon},
		{1, false, volumeLowIcon},
		{49, false, volumeLowIcon},
		{50, false, volumeHighIcon},
		{100, false, volumeHighIcon},
	} {
		if got := volumeIconFor(tc.percent, tc.muted); got != tc.want {
			t.Errorf("volumeIconFor(%d, %v) picked the wrong icon", tc.percent, tc.muted)
		}
	}
}

// Hovering the overlay holds it on screen, and clicking the speaker toggles
// the mute, as on the system flyout.
func TestVolumeSpeakerTogglesMute(t *testing.T) {
	a := newLocalizedApp()
	toggled := make(chan struct{}, 1)
	defer func(old func() (volume.State, error)) { toggleMute = old }(toggleMute)
	toggleMute = func() (volume.State, error) {
		toggled <- struct{}{}
		return volume.State{Percent: 64, Muted: true}, nil
	}
	tt := ui.NewTester(a.volumeView, volumeOverlayWidth, volumeOverlayHeight)
	a.volumePct = 64
	tt.Frame()
	tt.Move(20, volumeOverlayHeight/2)
	tt.Frame()
	if !a.volumeHovered {
		t.Fatal("hovering the overlay does not hold it")
	}
	tt.ClickAt(20, volumeOverlayHeight/2)
	tt.Frame()
	select {
	case <-toggled:
	case <-time.After(2 * time.Second):
		t.Fatal("clicking the speaker does not toggle the mute")
	}
	tt.Move(-10, -10)
	tt.Frame()
	if a.volumeHovered {
		t.Fatal("the overlay stays held after the pointer left")
	}
}
