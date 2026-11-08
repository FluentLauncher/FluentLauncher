//go:build windows

package volume

import (
	"testing"

	"fluentlauncher/internal/core/settings"
)

// TestEdgeBitsMatchSettings pins the contract the app relies on when it
// casts settings.ScreenEdges to Edges: a checkbox the user ticks on one
// edge must hit that same edge in the hook. Top and Bottom were once
// declared in the opposite order here, which swapped the two.
func TestEdgeBitsMatchSettings(t *testing.T) {
	for _, c := range []struct {
		name     string
		hook     Edges
		settings settings.ScreenEdges
	}{
		{"top", EdgeTop, settings.EdgeTop},
		{"bottom", EdgeBottom, settings.EdgeBottom},
		{"left", EdgeLeft, settings.EdgeLeft},
		{"right", EdgeRight, settings.EdgeRight},
	} {
		if c.hook != Edges(c.settings) {
			t.Errorf("%s: hook bit %d != settings bit %d", c.name, c.hook, c.settings)
		}
	}
}
