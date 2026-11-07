package layout

import "testing"

func TestCalculateAnchor(t *testing.T) {
	work := WorkArea{X: 100, Y: 50, Width: 1920, Height: 1040}
	rect := Calculate(work, 1, 800, 500, &Anchor{X: 0, Y: 0})
	if rect.X != work.X+16 || rect.Y != work.Y+16 {
		t.Fatalf("top left anchor = %+v", rect)
	}
	rect = Calculate(work, 1, 800, 500, &Anchor{X: 1, Y: 1})
	if rect.X != work.X+work.Width-16-800 || rect.Y != work.Y+work.Height-16-500 {
		t.Fatalf("bottom right anchor = %+v", rect)
	}
	rect = Calculate(work, 1, 800, 500, &Anchor{X: 0.5, Y: 0.5})
	if rect.X != work.X+(work.Width-800)/2 {
		t.Fatalf("centered anchor = %+v", rect)
	}
	// A panel larger than the work area stays inside it.
	rect = Calculate(work, 1, 5000, 5000, &Anchor{X: 0.5, Y: 0.5})
	if rect.Width > work.Width || rect.Height > work.Height {
		t.Fatalf("oversized panel = %+v", rect)
	}
}

func TestCaptureRoundTrip(t *testing.T) {
	work := WorkArea{Width: 1600, Height: 900}
	rect := Calculate(work, 1, 800, 500, &Anchor{X: 1, Y: 0})
	anchor := Capture(work, rect.X, rect.Y, rect.Width, rect.Height)
	if anchor.X < 0.95 || anchor.Y > 0.05 {
		t.Fatalf("captured anchor = %+v", anchor)
	}
}
