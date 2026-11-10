package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"fluentlauncher/internal/core/settings"
	"fluentlauncher/locales"
)

// State belongs to the UI thread. Network and disk work publishes through the
// main panel, which lives until app exit, even while settings are hidden.
type updateState struct {
	busy         bool
	message      string
	available    *mygo.Update
	ctx          context.Context
	installing   bool
	progress     int
	notesOpen    bool
	channel      string
	channelIndex int
}

func appVersion() string {
	if version := mygo.App.Version(); version != "" {
		return version
	}
	return "dev"
}

func (a *App) startUpdateChecks() {
	ctx, cancel := context.WithCancel(context.Background())
	a.updateCancel = cancel
	a.updates.ctx = ctx
	go func() {
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				a.panel.Update(func() {
					if !a.quitting {
						a.checkForUpdates(true)
					}
				})
				timer.Reset(24 * time.Hour)
			}
		}
	}()
}

func (a *App) publishUpdate(fn func()) {
	a.panel.Update(func() {
		if a.quitting {
			return
		}
		fn()
		if a.settingsWin != nil {
			a.settingsWin.Invalidate()
		}
	})
}

func (a *App) checkForUpdates(background bool) {
	if a.updates.busy {
		return
	}
	a.updates.busy = true
	a.updates.message = locales.UpdateChecking()
	channel := a.settings.EffectiveUpdateChannel(appVersion())
	mygo.Updater.SetPrereleases(channel == settings.UpdateChannelNightly)
	ctx, cancel := context.WithTimeout(a.updates.ctx, 45*time.Second)
	go func() {
		defer cancel()
		if exe, err := os.Executable(); err == nil && !strings.EqualFold(filepath.Base(exe), "FluentLauncher.exe") {
			a.publishUpdate(func() {
				a.updates.busy = false
				a.updates.message = locales.UpdateRenamed()
			})
			return
		}
		// Enabled probes directory permissions, so it also stays off the UI.
		if !mygo.Updater.Enabled() {
			a.publishUpdate(func() {
				a.updates.busy = false
				a.updates.message = locales.UpdateDisabled()
			})
			return
		}
		up, err := mygo.Updater.Check(ctx)
		a.publishUpdate(func() {
			a.updates.busy = false
			if channel != a.settings.EffectiveUpdateChannel(appVersion()) {
				a.updates.available = nil
				a.updates.notesOpen = false
				a.updates.message = ""
				return
			}
			if err != nil {
				a.updates.message = locales.UpdateFailed(err)
				return
			}
			previous := a.updates.available
			a.updates.available = up
			a.updates.message = locales.UpdateLatest()
			if up != nil {
				a.updates.message = ""
				if background && (previous == nil || previous.Version != up.Version) {
					a.settingsPage = len(settingPages()) - 1
					a.openSettings()
				}
			}
		})
	}()
}

func (a *App) installUpdate() {
	if a.updates.busy || a.updates.available == nil {
		return
	}
	up := a.updates.available
	a.updates.busy = true
	a.updates.installing = true
	a.updates.progress = 0
	a.updates.message = locales.UpdateDownloading(0)
	ctx, cancel := context.WithTimeout(a.updates.ctx, 15*time.Minute)
	go func() {
		defer cancel()
		last := -1
		err := up.Install(ctx, func(downloaded, total int64) {
			if total <= 0 {
				return
			}
			percent := int(downloaded * 100 / total)
			if percent != last {
				last = percent
				a.publishUpdate(func() {
					a.updates.progress = percent
					a.updates.message = locales.UpdateDownloading(percent)
				})
			}
		})
		a.publishUpdate(func() {
			a.updates.busy = false
			a.updates.installing = false
			if err != nil {
				a.updates.message = locales.UpdateFailed(err)
				return
			}
			// The framework's normal quit/relaunch sequence closes the database,
			// plugin processes and single-instance lock before starting the EXE.
			if a.settingsWin != nil {
				a.flushSettingsDraft()
			}
			mygo.App.Relaunch()
		})
	}()
}
