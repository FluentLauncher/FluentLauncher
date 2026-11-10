package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"fluentlauncher/internal/core/settings"
	"fluentlauncher/locales"
)

func (a *App) updateCard(c *ui.Context) {
	theme := c.Theme()
	channel := a.settingsDraft.EffectiveUpdateChannel(appVersion())
	pending := channel != a.settings.EffectiveUpdateChannel(appVersion())
	ui.Column(c).Gap(theme.Space(3)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.UpdateTitle()).FontSize(theme.FontSize + 1).Bold().TextColor(theme.Text).Grow(1)
			if compactButton(c, locales.UpdateCheck()).Disabled(a.updates.busy || pending).Tooltip(locales.UpdateHint()).Clicked() {
				a.checkForUpdates(false)
			}
		})
		ui.Column(c).Radius(theme.Radius*1.5).Background(theme.Surface.Alpha(0.8)).Border(1, theme.Border).Children(func() {
			status := locales.UpdateSchedule()
			if a.updates.message != "" {
				status = a.updates.message
			} else if up := a.updates.available; up != nil {
				status = locales.UpdateAvailable(up.Version)
			}
			updateSettingRow(c, locales.UpdateVersion(), status, func(c *ui.Context) {
				ui.Row(c).Gap(theme.Space(3)).AlignItems(ui.Center).Children(func() {
					ui.Text(c, appVersion()).TextColor(theme.TextMuted).MaxWidth(theme.Space(52)).SingleLine().Ellipsis("…").Tooltip(appVersion())
					if a.updates.available != nil && compactPrimaryButton(c, locales.UpdateInstall()).Disabled(a.updates.busy || pending).Clicked() {
						a.installUpdate()
					}
				})
			})
			if a.updates.installing {
				ui.Progress(c, float64(a.updates.progress)/100).Height(theme.Space(0.75)).Label(locales.UpdateDownloading(a.updates.progress))
			}
			ui.Divider(c)
			hasNotes := a.updates.available != nil && strings.TrimSpace(a.updates.available.Notes) != ""
			notesHint := locales.UpdateNotesPending()
			if hasNotes {
				notesHint = locales.UpdateNotesHint()
			} else if a.updates.available != nil {
				notesHint = locales.UpdateNotesEmpty()
			}
			updateSettingRow(c, locales.UpdateNotes(), notesHint, func(c *ui.Context) {
				if compactButton(c, locales.CommonOpen()).Disabled(!hasNotes).Clicked() {
					a.updates.notesOpen = true
				}
			})
			ui.Divider(c)
			hint := locales.UpdateStableHint()
			if channel == settings.UpdateChannelNightly {
				hint = locales.UpdatePreviewHint()
			}
			updateSettingRow(c, locales.UpdateChannel(), hint, func(c *ui.Context) {
				// Keep the bound selection across frames, and resync only when
				// settings change outside this control (load, import or rollback).
				if a.updates.channel != channel {
					a.updates.channel = channel
					a.updates.channelIndex = 0
					if channel == settings.UpdateChannelNightly {
						a.updates.channelIndex = 1
					}
				}
				choice := ui.Segmented(c, &a.updates.channelIndex, locales.UpdateChannelStable(), locales.UpdateChannelNightly()).
					Label(locales.UpdateChannel()).Disabled(a.updates.busy).Tooltip(locales.UpdateChannelHint())
				if choice.Changed() {
					next := settings.UpdateChannelStable
					if a.updates.channelIndex == 1 {
						next = settings.UpdateChannelNightly
					}
					a.settingsDraft.UpdateChannel = next
					a.updates.channel = next
					a.updates.available = nil
					a.updates.notesOpen = false
					a.updates.message = ""
				}
			})
		})
	})
}

// Update rows use one shared edge, with secondary copy wrapping independently
// of the controls. A long nightly version never widens the settings window.
func updateSettingRow(c *ui.Context, label, hint string, control func(*ui.Context)) {
	theme := c.Theme()
	ui.Row(c).Padding(theme.Space(4), theme.Space(4.5)).Gap(theme.Space(4)).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(theme.Space(1)).Children(func() {
			ui.Text(c, label).TextColor(theme.Text)
			ui.Text(c, hint).FontSize(theme.FontSize * 0.9).TextColor(theme.TextMuted).Wrap()
		})
		ui.Column(c).Shrink(0).Children(func() { control(c) })
	})
}

func (a *App) updateNotesDialog(c *ui.Context) {
	up := a.updates.available
	if up == nil {
		a.updates.notesOpen = false
		return
	}
	theme := c.Theme()
	w, h := c.Size()
	ui.DialogBase(c, &a.updates.notesOpen, func(backdrop, panel ui.Element) {
		inverse := theme.Inverse
		if inverse.A == 0 {
			inverse = theme.Text
		}
		mask := inverse.Alpha(0.24)
		if theme.Dark {
			mask = theme.Background.Alpha(0.7)
		}
		backdrop.Background(mask)
		maxHeight := min(theme.Space(165), h-theme.Space(12))
		panel.Width(min(theme.Space(200), w-theme.Space(12))).MaxHeight(maxHeight).
			Background(theme.Background).Border(1, theme.Border).Radius(theme.Radius * 2).Label(locales.UpdateNotes())
		ui.Column(c).FillWidth().MaxHeight(maxHeight).Children(func() {
			ui.Column(c).Padding(theme.Space(5), theme.Space(6)).Gap(theme.Space(2)).Children(func() {
				ui.Text(c, locales.UpdateToVersion(up.Version)).FontSize(theme.FontSize + 3).Bold().TextColor(theme.Text).Wrap()
				ui.Text(c, locales.UpdateFromVersion(appVersion())).TextColor(theme.TextMuted).Wrap()
			})
			ui.Divider(c)
			ui.Scroll(c).MaxHeight(maxHeight - theme.Space(42)).Shrink(1).MinHeight(0).FillWidth().Children(func() {
				ui.Column(c).Padding(theme.Space(5), theme.Space(6)).FillWidth().Children(func() {
					markdown.View(c, up.Notes, nil)
				})
			})
			ui.Divider(c)
			ui.Column(c).Padding(theme.Space(4), theme.Space(6)).Gap(theme.Space(3)).Children(func() {
				if a.updates.message != "" {
					ui.Text(c, a.updates.message).TextColor(theme.TextMuted).Wrap()
				}
				if a.updates.installing {
					ui.Progress(c, float64(a.updates.progress)/100).Height(theme.Space(1)).Label(locales.UpdateDownloading(a.updates.progress))
				}
				ui.Row(c).Gap(theme.Space(3)).AlignItems(ui.Center).Children(func() {
					ui.Text(c, locales.UpdateDataKept()).FontSize(theme.FontSize * 0.9).TextColor(theme.TextMuted).Grow(1).Wrap()
					if compactButton(c, locales.UpdateLater()).Clicked() {
						a.updates.notesOpen = false
					}
					pending := a.settingsDraft.EffectiveUpdateChannel(appVersion()) != a.settings.EffectiveUpdateChannel(appVersion())
					if compactPrimaryButton(c, locales.UpdateInstall()).Disabled(a.updates.busy || pending).Clicked() {
						a.installUpdate()
					}
				})
			})
		})
	})
}
