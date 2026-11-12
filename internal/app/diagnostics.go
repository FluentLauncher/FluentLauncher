package app

import (
	"context"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/diag"
	"fluentlauncher/internal/winapi/sysinfo"
	"fluentlauncher/locales"
)

func (a *App) diagnosticsCard(c *ui.Context) {
	theme := c.Theme()
	settingCard(c, locales.DiagnosticsTitle(), func(c *ui.Context) {
		status := locales.DiagnosticsIdle()
		active := a.diagnostics != nil && a.diagnostics.Active()
		if a.diagnostics == nil || !a.diagnostics.Healthy() {
			status = locales.DiagnosticsUnavailable()
		} else if active {
			status = locales.DiagnosticsActive(a.diagnostics.Until().Format("15:04"))
		}
		ui.Text(c, status).TextColor(theme.Text).Wrap()
		ui.Text(c, locales.DiagnosticsPrivacy()).FontSize(theme.FontSize).TextColor(theme.TextMuted).Wrap()
		ui.Row(c).Gap(theme.Space(2)).Children(func() {
			label := locales.DiagnosticsStart()
			if active {
				label = locales.DiagnosticsStop()
			}
			if compactButton(c, label).Disabled(a.diagnostics == nil || a.diagnosticBusy).Clicked() {
				a.toggleDiagnostics()
			}
			if compactButton(c, locales.DiagnosticsOpen()).Disabled(a.diagnostics == nil).Clicked() {
				dir := a.diagnostics.Dir()
				go func() {
					if err := mygo.Shell.OpenPath(dir); err != nil {
						a.publishUpdate(func() { a.settingsError = locales.DiagnosticsFailed(diag.ErrorKind(err)) })
					}
				}()
			}
		})
		ui.Divider(c)
		ui.Checkbox(c, &a.diagnosticRaw, locales.DiagnosticsRaw()).Disabled(a.diagnosticBusy)
		ui.Text(c, locales.DiagnosticsRawHint()).FontSize(theme.FontSize).TextColor(theme.TextMuted).Wrap()
		label := locales.DiagnosticsExport()
		if a.diagnosticBusy {
			label = locales.DiagnosticsExporting()
		}
		if compactButton(c, label).Disabled(a.diagnostics == nil || a.diagnosticBusy).Clicked() {
			a.exportDiagnostics()
		}
	})
}

func (a *App) toggleDiagnostics() {
	if a.diagnosticTimer != nil {
		a.diagnosticTimer.Stop()
	}
	if a.diagnostics.Active() {
		a.diagnostics.Stop()
		return
	}
	a.diagnostics.Start()
	// Logging checks the deadline itself; this timer only refreshes the UI.
	if a.panel != nil {
		a.diagnosticTimer = time.AfterFunc(5*time.Minute, func() { a.publishUpdate(func() {}) })
	}
}

func (a *App) diagnosticResult(err error) {
	if a.panel == nil {
		return
	}
	a.publishUpdate(func() {
		a.diagnosticBusy = false
		if err != nil {
			a.settingsError = locales.DiagnosticsFailed(diag.ErrorKind(err))
			return
		}
		a.settingsError = ""
		a.settingsNotice = locales.DiagnosticsExported()
	})
}

func (a *App) exportDiagnostics() {
	a.diagnosticBusy = true
	a.settingsNotice, a.settingsError = "", ""
	// Explicit allowlist: no settings JSON, paths, query text, aliases or history.
	summary := map[string]any{
		"version": appVersion(), "max_results": a.settings.MaxResults,
		"everything_enabled": a.settings.Files.EnableEverything,
		"ui_scale":           a.scaleFactor, "background": a.settings.Theme.Background,
	}
	counts := map[string]int{}
	for id, engine := range map[string]*search.Engine{"applications": a.appEngine, "files": a.fileEngine, "custom": a.customEngine, "workspaces": a.workspaceEngine} {
		if engine != nil {
			counts[id] = engine.Count()
		}
	}
	summary["index_counts"] = counts
	if a.icons != nil {
		summary["icons"] = map[string]uint64{"hits": a.icons.hits.Load(), "misses": a.icons.misses.Load(), "queue_full": a.icons.full.Load()}
	}
	raw := map[string]string{}
	var plugins []map[string]any
	if a.plugins != nil {
		for _, p := range a.plugins.Statuses() {
			plugins = append(plugins, map[string]any{"id": p.Registration.Manifest.ID, "version": p.Registration.Manifest.Version, "enabled": p.Enabled, "running": p.Running, "paused": p.Paused, "failures": p.Failures, "startup_ms": p.StartupMs, "query_ms": p.QueryMs})
			if a.diagnosticRaw && p.Stderr != "" {
				raw[p.Registration.Manifest.ID] = p.Stderr
			}
		}
	}
	summary["plugins"] = plugins
	recorder, includeRaw := a.diagnostics, a.diagnosticRaw
	go func() {
		summary["windows_version"] = sysinfo.Version()
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Title: locales.DiagnosticsExport(), DefaultPath: "FluentLauncher-diagnostics-" + time.Now().Format("20060102-150405") + ".zip", Filters: []mygo.FileFilter{{Name: "ZIP", Extensions: []string{"zip"}}}})
		if err != nil {
			a.diagnosticResult(err)
			return
		}
		if path == "" {
			a.publishUpdate(func() { a.diagnosticBusy = false })
			return
		}
		if filepath.Ext(path) == "" {
			path += ".zip"
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		a.diagnosticResult(recorder.Export(ctx, path, summary, raw, includeRaw))
	}()
}
