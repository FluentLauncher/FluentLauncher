package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/diag"
	"fluentlauncher/locales"
)

func TestDiagnosticsCardStatesAndLocales(t *testing.T) {
	for _, language := range []string{"zh-Hans", "zh-Hant", "en"} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dark=%v", language, dark), func(t *testing.T) {
				setCatalogLocale(t, language)
				a := newLocalizedApp()
				r, err := diag.Open(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				defer r.Close()
				a.diagnostics = r
				tt := ui.NewTester(func(c *ui.Context) {
					theme := *c.Theme()
					a.applyUITheme(&theme)
					theme.FontSize = settingsFontSize
					c.SetTheme(&theme)
					ui.Column(c).Fill().Padding(theme.Space(4)).Children(func() { a.diagnosticsCard(c) })
				}, 720, 500)
				tt.SetDark(dark)
				if a.diagnosticRaw || !tt.HasText(locales.DiagnosticsPrivacy()) {
					t.Fatal("raw data was enabled or privacy hint missing")
				}
				if err := tt.Click(locales.DiagnosticsStart()); err != nil {
					t.Fatal(err)
				}
				if !r.Active() || !tt.HasText(locales.DiagnosticsStop()) {
					t.Fatal("start did not switch state")
				}
				if dir := os.Getenv("FL_DIAGNOSTIC_ARTIFACT_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
					f, err := os.Create(filepath.Join(dir, fmt.Sprintf("diagnostics-%s-dark-%v.png", language, dark)))
					if err != nil {
						t.Fatal(err)
					}
					if err := png.Encode(f, tt.Image()); err != nil {
						t.Fatal(err)
					}
					f.Close()
				}
				if err := tt.Click(locales.DiagnosticsStop()); err != nil {
					t.Fatal(err)
				}
				if r.Active() {
					t.Fatal("stop did not disable diagnostics")
				}
				a.diagnosticBusy = true
				tt.Frame()
				if !tt.HasText(locales.DiagnosticsExporting()) {
					t.Fatal("missing busy state")
				}
				a.diagnostics = nil
				tt.Frame()
				tt.Advance(time.Second)
				if !tt.HasText(locales.DiagnosticsUnavailable()) {
					t.Fatal("missing unavailable state")
				}
			})
		}
	}
}
