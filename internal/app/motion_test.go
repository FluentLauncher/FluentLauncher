package app

import (
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/locales"
)

func TestSettingsMotionRapidNavigation(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		t.Run(fmt.Sprint("reduced=", reduced), func(t *testing.T) {
			a := newLocalizedApp()
			tt := ui.NewTester(a.settingsView, 900, 660)
			tt.SetPreferences(ui.Preferences{ReduceMotion: reduced, TextScale: 1})
			for _, page := range []int{pageAppearance, pageVolume, pageGeneral, pageAppearance} {
				r, ok := tt.Find(settingPages()[page])
				if !ok {
					t.Fatal("sidebar item missing")
				}
				tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
				tt.Advance(40 * time.Millisecond)
				if a.settingsPage != page {
					t.Fatalf("navigation during animation selected %d, want %d", a.settingsPage, page)
				}
			}
			// Use a field before its page entrance finishes. Animation must
			// neither defer input nor leave a previous page over the controls.
			if err := tt.Click(locales.SettingsBackgroundAcrylic()); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click(locales.SettingsBackgroundSolid()); err != nil {
				t.Fatal(err)
			}
			if a.settingsDraft.Theme.Background != settings.BackgroundSolid {
				t.Fatal("the dropdown ignored input during the page transition")
			}
			tt.Move(850, 640)
			tt.Advance(time.Second)
			if tt.FramePending() {
				t.Fatal("settings animations continued requesting frames at rest")
			}
		})
	}
}

func TestSearchMotionKeepsKeyboardFocus(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		a := newLocalizedApp()
		a.results = []model.Result{
			{Entry: model.Entry{ID: "one", Title: "First", Kind: model.Application}},
			{Entry: model.Entry{ID: "two", Title: "Second", Kind: model.Application}},
		}
		a.query = "retained query"
		tt := ui.NewTester(a.view, 480, 320)
		tt.SetPreferences(ui.Preferences{ReduceMotion: reduced, TextScale: 1})
		a.input.Focus()
		tt.Frame()
		tt.Advance(40 * time.Millisecond)
		tt.Key(0, ui.KeyDown)
		if a.chosen != 1 || !tt.Focused(locales.PanelSearchLabel()) {
			t.Fatal("selection feedback interfered with search focus or navigation")
		}
		tt.Key(ui.Ctrl, ui.KeyK)
		tt.Advance(40 * time.Millisecond)
		tt.Key(0, ui.KeyEscape)
		tt.Advance(200 * time.Millisecond)
		if a.actionsOpen || a.query != "retained query" || !tt.Focused(locales.PanelSearchLabel()) {
			t.Fatal("returning from actions lost the query or search focus")
		}
	}
}

// Opt-in visual harness: renders the real views with a deterministic clock,
// without opening windows, injecting desktop input or accessing user data.
// FL_MOTION_ARTIFACT_DIR=... go test ./internal/app -run TestMotionPreview -count=1
func TestMotionPreview(t *testing.T) {
	dir := os.Getenv("FL_MOTION_ARTIFACT_DIR")
	if dir == "" {
		t.Skip("set FL_MOTION_ARTIFACT_DIR to export motion previews")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		for _, reduced := range []bool{false, true} {
			a := newLocalizedApp()
			a.settings.Theme.Background = settings.BackgroundSolid
			a.settings.Theme.Opacity = 1
			a.settingsDraft = a.settings
			tt := ui.NewTester(a.settingsView, 1180, 840)
			tt.SetDark(dark)
			tt.SetPreferences(ui.Preferences{ReduceMotion: reduced, TextScale: 1})
			name := fmt.Sprintf("settings-dark-%t-reduced-%t.gif", dark, reduced)
			exportMotion(t, tt, filepath.Join(dir, name), func(frame int) {
				if frame == 4 {
					a.settingsPage = pageAppearance
					tt.Frame()
				}
			})
		}
		a := newLocalizedApp()
		a.settings.Theme.Background = settings.BackgroundSolid
		a.settings.Theme.Opacity = 1
		a.settingsDraft = a.settings
		a.results = []model.Result{
			{Entry: model.Entry{ID: "one", Title: "Visual Studio Code", Subtitle: "Application", Kind: model.Application}},
			{Entry: model.Entry{ID: "two", Title: "Windows Terminal", Subtitle: "Application", Kind: model.Application}},
		}
		tt := ui.NewTester(a.view, 760, 560)
		tt.SetDark(dark)
		exportMotion(t, tt, filepath.Join(dir, fmt.Sprintf("search-dark-%t.gif", dark)), func(frame int) {
			if frame == 4 {
				a.input.Focus()
				tt.Frame()
			}
			if frame == 12 {
				tt.Key(0, ui.KeyDown)
			}
		})
	}
}

func exportMotion(t *testing.T, tt *ui.Tester, path string, step func(int)) {
	t.Helper()
	animation := &gif.GIF{LoopCount: 0}
	for frame := 0; frame < 24; frame++ {
		step(frame)
		tt.Advance(40 * time.Millisecond)
		src := tt.Image()
		img := image.NewPaletted(src.Bounds(), palette.Plan9)
		draw.Draw(img, img.Bounds(), src, image.Point{}, draw.Src)
		animation.Image = append(animation.Image, img)
		animation.Delay = append(animation.Delay, 4)
	}
	animation.Delay[len(animation.Delay)-1] = 80
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := gif.EncodeAll(f, animation); err != nil {
		t.Fatal(err)
	}
}
