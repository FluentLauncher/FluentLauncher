package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/plugin"
	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/internal/store"
	"fluentlauncher/locales"
)

// settingPages returns the sidebar's page titles, localized per frame so a
// language change shows at once.
func settingPages() []string {
	return []string{
		locales.SettingsPageGeneral(),
		locales.SettingsPageAppearance(),
		locales.SettingsPageFiles(),
		locales.SettingsPageVolume(),
		locales.SettingsPageSites(),
		locales.SettingsPageLaunchItems(),
		locales.SettingsPagePlugins(),
		locales.SettingsPageOther(),
	}
}

// pageIcons are the sidebar's line icons, one per settings page.
var pageIcons = []*ui.SVG{
	ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="4" y1="7" x2="20" y2="7"/><circle cx="9" cy="7" r="2.5"/><line x1="4" y1="15" x2="20" y2="15"/><circle cx="15" cy="15" r="2.5"/></svg>`)),
	ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3a9 9 0 1 0 0 18c1.5 0 2-.9 2-2s-.7-2 .3-2.8c.8-.6 2.2-.2 3.5-.2 1.8 0 3.2-1.2 3.2-3C21 7.6 17 3 12 3z"/><circle cx="7.5" cy="10.5" r="1"/><circle cx="12" cy="7.5" r="1"/><circle cx="16.5" cy="10.5" r="1"/></svg>`)),
	ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>`)),
	ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"><path d="M11 5 6 9H3v6h3l5 4z"/><path d="M15.5 8.5a5 5 0 0 1 0 7"/><path d="M18.5 5.5a9 9 0 0 1 0 13"/></svg>`)),
	ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M3 12h18"/><path d="M12 3a14 14 0 0 1 0 18"/><path d="M12 3a14 14 0 0 0 0 18"/></svg>`)),
	ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round"><path d="M13 2 4 14h6l-1 8 9-12h-6z"/></svg>`)),
	ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 7V3"/><path d="M15 7V3"/><path d="M18 7v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V7z"/><path d="M12 16v5"/></svg>`)),
	ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><path d="M12 7.5h.01"/></svg>`)),
}

var chevronIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>`))

// settingsFontSize is the type size of the settings window, apart from the
// theme's own: that one sizes the search panel, where a large line reads at a
// glance, and would make every field of the settings dense pages oversized.
const settingsFontSize float32 = 13

func (a *App) settingsView(c *ui.Context) {
	theme := *c.Theme()
	a.applyUITheme(&theme)
	theme.FontSize = settingsFontSize
	c.SetTheme(&theme)
	pages := settingPages()
	a.settingsPage = min(max(a.settingsPage, 0), len(pages)-1)

	a.drawShell(c, &theme, func() {
		a.captionBar(c, &theme, locales.SettingsWindowTitle(), a.settingsWin)
		ui.Row(c).Grow(1).FillWidth().Children(func() {
			a.settingsSidebar(c, &theme, pages)
			ui.Column(c).Grow(1).FillHeight().Children(func() {
				ui.Row(c).Padding(20, 24).AlignItems(ui.Center).Children(func() {
					ui.Text(c, pages[a.settingsPage]).FontSize(20).Bold().TextColor(theme.Text).Grow(1)
				})
				ui.Divider(c)
				ui.Scroll(c).Grow(1).Children(func() {
					ui.Column(c).Padding(20, 24).Gap(16).Children(func() {
						switch a.settingsPage {
						case 0:
							a.generalPage(c)
						case 1:
							a.appearancePage(c)
						case 2:
							a.filesPage(c)
						case 3:
							a.volumePage(c)
						case 4:
							a.sitesPage(c)
						case 5:
							a.launchItemsPage(c)
						case 6:
							a.pluginsPage(c)
						default:
							a.aboutPage(c)
						}
					})
				})
				ui.Divider(c)
				// No save or cancel buttons: every change applies and is
				// saved a moment later, so this row only reports what went
				// wrong or reassures that there is nothing to do.
				ui.Row(c).Padding(8, 24).AlignItems(ui.Center).Children(func() {
					switch {
					case a.settingsError != "":
						ui.Text(c, a.settingsError).FontSize(12).TextColor(theme.Danger)
					case a.settingsNotice != "":
						ui.Text(c, a.settingsNotice).FontSize(12).TextColor(theme.Success)
					default:
						ui.Text(c, locales.SettingsAutoSave()).FontSize(11).TextColor(theme.TextMuted)
					}
				})
			})
		})
	})
	if a.settingsWin != nil && a.draftChanged() {
		a.scheduleSettingsSave()
	}
}

// settingsSidebar is the navigation column: transparent over the window
// material, with the same selected/hover treatment as the result rows. The
// mark and the window buttons live in the caption bar above it.
func (a *App) settingsSidebar(c *ui.Context, theme *ui.Theme, pages []string) {
	hover := a.settingsHover
	next := -1
	ui.Column(c).Width(208).FillHeight().Padding(8, 12).Gap(2).Children(func() {
		for i, name := range pages {
			background := ui.Transparent
			text := theme.Text
			if i == a.settingsPage {
				background = selectionTint(theme)
				text = theme.Text
			} else if i == hover {
				background = theme.Surface.Alpha(0.7)
			}
			row := ui.Row(c).Padding(9, 12).Gap(10).AlignItems(ui.Center).Radius(8).Background(background)
			row.Children(func() {
				ui.Icon(c, pageIcons[i]).FontSize(16).TextColor(text)
				ui.Text(c, name).FontSize(13).TextColor(text).Grow(1)
			})
			if row.Clicked() {
				a.settingsPage = i
			}
			if row.Hovered() {
				next = i
			}
		}
	})
	a.settingsHover = next
}

// settingCard groups related controls on a translucent plate, so the
// window material stays visible between groups.
func settingCard(c *ui.Context, title string, body func(c *ui.Context)) {
	theme := c.Theme()
	ui.Column(c).Gap(12).Padding(16, 18).Radius(10).Background(theme.Surface.Alpha(0.8)).Children(func() {
		ui.Text(c, title).FontSize(14).Bold().TextColor(theme.Text)
		body(c)
	})
}

// settingRow puts a visible label and hint left of a control; every
// control in the settings pages goes through it.
func settingRow(c *ui.Context, label, hint string, control func(c *ui.Context)) {
	theme := c.Theme()
	ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(13).TextColor(theme.Text)
			if hint != "" {
				ui.Text(c, hint).FontSize(11).TextColor(theme.TextMuted)
			}
		})
		control(c)
	})
}

func (a *App) switchRow(c *ui.Context, label, hint string, value *bool) {
	settingRow(c, label, hint, func(c *ui.Context) {
		ui.Switch(c, value)
	})
}

func (a *App) numberRow(c *ui.Context, label, hint string, value *int, low, high, step int) {
	settingRow(c, label, hint, func(c *ui.Context) {
		row := ui.Row(c).Gap(10).AlignItems(ui.Center)
		text := ui.Local[string](row, "text", func() string { return strconv.Itoa(*value) })
		row.Children(func() {
			field := ui.TextInput(c, text).Width(72).FontSize(13)
			if field.Changed() {
				if parsed, err := strconv.Atoi(strings.TrimSpace(*text)); err == nil && parsed >= low && parsed <= high {
					*value = parsed
				}
			}
			if !field.Focused() {
				*text = strconv.Itoa(*value)
			}
			current := float64(*value)
			if ui.Slider(c, &current, float64(low), float64(high)).Width(150).Changed() {
				rounded := int(math.Round(current/float64(step))) * step
				*value = clampInt(rounded, low, high)
			}
		})
	})
}

// floatRow edits a float with a small field for the exact value and a slider
// for the coarse change: the number input's own plus and minus buttons were
// heavy next to the labels.
func (a *App) floatRow(c *ui.Context, label, hint string, value *float64, low, high, step float64) {
	settingRow(c, label, hint, func(c *ui.Context) {
		row := ui.Row(c).Gap(10).AlignItems(ui.Center)
		digits := 0
		if step < 1 {
			digits = 1
		}
		format := func(v float64) string { return strconv.FormatFloat(v, 'f', digits, 64) }
		text := ui.Local[string](row, "text", func() string { return format(*value) })
		row.Children(func() {
			field := ui.TextInput(c, text).Width(72).FontSize(13)
			if field.Changed() {
				if parsed, err := strconv.ParseFloat(strings.TrimSpace(*text), 64); err == nil && parsed >= low && parsed <= high {
					*value = math.Round(parsed/step) * step
				}
			}
			if !field.Focused() {
				*text = format(*value)
			}
			current := *value
			if ui.Slider(c, &current, low, high).Width(150).Changed() {
				*value = math.Round(current/step) * step
			}
		})
	})
}

func (a *App) textRow(c *ui.Context, label, hint, placeholder string, value *string, width float32) {
	settingRow(c, label, hint, func(c *ui.Context) {
		ui.TextInput(c, value).Placeholder(placeholder).Width(width)
	})
}

// themedSelect is a dropdown drawn in the window, so its field and popup
// carry the theme like every other control; ui.Select's popup is a plain
// system-styled list.
func (a *App) themedSelect(c *ui.Context, key, label, hint string, options []string, current string, choose func(label string)) {
	settingRow(c, label, hint, func(c *ui.Context) {
		theme := c.Theme()
		open := a.selectOpen == key
		// The field matches the text inputs beside it: the same plate, the
		// same padding and the same text size, so a row does not mix two
		// looks.
		anchor := ui.Row(c).Width(220).Padding(theme.Space(1.5), theme.Space(2.5)).Gap(8).
			AlignItems(ui.Center).Radius(theme.Radius).Background(theme.Surface).Border(1, theme.Border)
		anchor.Children(func() {
			ui.Text(c, current).TextColor(theme.Text).Grow(1).SingleLine()
			ui.Icon(c, chevronIcon).FontSize(12).TextColor(theme.TextMuted)
		})
		if anchor.Clicked() {
			if open {
				a.selectOpen = ""
			} else {
				a.selectOpen = key
			}
			open = a.selectOpen == key
		}
		ui.Popover(c, anchor, &open, func() {
			ui.Column(c).Width(220).Padding(4).Gap(2).Children(func() {
				for _, option := range options {
					background := ui.Transparent
					text := theme.Text
					if option == current {
						background = selectionTint(theme)
						text = theme.Text
					}
					row := ui.Row(c).Padding(8, 12).Radius(6).Background(background)
					row.Children(func() {
						ui.Text(c, option).FontSize(13).TextColor(text).Grow(1).SingleLine()
					})
					if row.Clicked() {
						choose(option)
						a.selectOpen = ""
						open = false
					}
				}
			})
		})
		if !open && a.selectOpen == key {
			a.selectOpen = ""
		}
	})
}

// colorRow edits an optional hex color with a color well; an empty value
// keeps the system color and can be restored with the reset button. A background color
// is stored without alpha: the opacity slider is what makes the window
// see-through, so a color of its own cannot make it translucent twice.
func (a *App) colorRow(c *ui.Context, label, hint string, value *string, fallback ui.Color, opaque bool) {
	settingRow(c, label, hint, func(c *ui.Context) {
		theme := c.Theme()
		picked := fallback
		if parsed, ok := parseColor(*value); ok {
			picked = parsed
		}
		if opaque {
			picked.A = 255
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if ui.ColorWell(c, &picked).Changed() {
				if opaque {
					picked.A = 255
				}
				*value = hexOf(picked)
			}
			shown := locales.SettingsFollowSystem()
			if *value != "" {
				shown = hexOf(picked)
			}
			ui.Text(c, shown).FontSize(12).TextColor(theme.TextMuted).Width(90).SingleLine()
			if *value != "" && compactButton(c, locales.CommonReset()).Clicked() {
				*value = ""
			}
		})
	})
}

// compactButton is a settings button at a smaller size than the widget's
// default: the settings window is dense, and a default button next to
// thirteen-pixel labels reads as heavy.
func compactButton(c *ui.Context, label string) ui.Element {
	return ui.Button(c, label).Padding(4, 10).FontSize(13)
}

// compactPrimaryButton is compactButton for the action the page saves with.
func compactPrimaryButton(c *ui.Context, label string) ui.Element {
	return ui.PrimaryButton(c, label).Padding(4, 10).FontSize(13)
}

func hexOf(col ui.Color) string {
	if col.A == 255 {
		return fmt.Sprintf("#%02X%02X%02X", col.R, col.G, col.B)
	}
	return fmt.Sprintf("#%02X%02X%02X%02X", col.A, col.R, col.G, col.B)
}

// sliderRow edits a float with a slider and shows its current value.
func (a *App) sliderRow(c *ui.Context, label, hint string, value *float64, lo, hi float64, format func(float64) string) {
	settingRow(c, label, hint, func(c *ui.Context) {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Slider(c, value, lo, hi).Width(200)
			ui.Text(c, format(*value)).FontSize(12).TextColor(c.Theme().TextMuted).Width(52).SingleLine()
		})
	})
}

// placementChoices returns the popup position options with labels for the
// current language; building them per frame keeps the label of each value in
// the same slice position.
func placementChoices() []struct {
	label string
	value settings.WindowPlacement
} {
	return []struct {
		label string
		value settings.WindowPlacement
	}{
		{locales.SettingsPlacementActiveScreen(), settings.PlacementActiveScreen},
		{locales.SettingsPlacementMouseScreen(), settings.PlacementMouseScreen},
		{locales.SettingsPlacementPrimaryScreen(), settings.PlacementPrimaryScreen},
	}
}

func themeModeChoices() []struct {
	label string
	value settings.ThemeMode
} {
	return []struct {
		label string
		value settings.ThemeMode
	}{
		{locales.SettingsFollowSystem(), settings.ThemeSystem},
		{locales.SettingsThemeLight(), settings.ThemeLight},
		{locales.SettingsThemeDark(), settings.ThemeDark},
	}
}

func backgroundChoices() []struct {
	label string
	value settings.BackgroundMode
} {
	return []struct {
		label string
		value settings.BackgroundMode
	}{
		{locales.SettingsBackgroundAcrylic(), settings.BackgroundAcrylic},
		{locales.SettingsBackgroundMica(), settings.BackgroundMica},
		{locales.SettingsBackgroundSolid(), settings.BackgroundSolid},
		{locales.SettingsBackgroundGradient(), settings.BackgroundGradient},
		{locales.SettingsBackgroundImage(), settings.BackgroundImage},
	}
}

func choiceLabels[T any](choices []struct {
	label string
	value T
}) []string {
	labels := make([]string, len(choices))
	for i, choice := range choices {
		labels[i] = choice.label
	}
	return labels
}

// languageChoices lists the interface languages, each named in itself.
var languageChoices = []struct {
	tag   string
	label string
}{
	{settings.LanguageZhHans, i18n.Autonym(settings.LanguageZhHans)},
	{settings.LanguageZhHant, i18n.Autonym(settings.LanguageZhHant)},
	{settings.LanguageEn, i18n.Autonym(settings.LanguageEn)},
}

func (a *App) generalPage(c *ui.Context) {
	settingCard(c, locales.SettingsLanguageCard(), func(c *ui.Context) {
		labels := make([]string, 0, len(languageChoices)+1)
		labels = append(labels, locales.SettingsLanguageSystem())
		for _, choice := range languageChoices {
			labels = append(labels, choice.label)
		}
		current := locales.SettingsLanguageSystem()
		for i, choice := range languageChoices {
			if choice.tag == a.settingsDraft.Language {
				current = labels[i+1]
			}
		}
		a.themedSelect(c, "language", locales.SettingsLanguage(), locales.SettingsLanguageHint(), labels, current, func(label string) {
			a.settingsDraft.Language = settings.LanguageSystem
			for i, choice := range languageChoices {
				if labels[i+1] == label {
					a.settingsDraft.Language = choice.tag
				}
			}
		})
	})
	settingCard(c, locales.SettingsGeneralWakeCard(), func(c *ui.Context) {
		hotkeyHint := ""
		if a.hotkeyActive != "" && a.hotkeyActive != a.settingsDraft.Hotkey {
			hotkeyHint = locales.SettingsHotkeyActive(a.hotkeyActive)
		}
		a.themedSelect(c, "hotkey", locales.SettingsHotkey(), hotkeyHint, []string{"Alt+Space", "Ctrl+Alt+Space", "Ctrl+Space"},
			a.settingsDraft.Hotkey, func(label string) { a.settingsDraft.Hotkey = label })
		a.switchRow(c, locales.SettingsHideOnStartup(), locales.SettingsHideOnStartupHint(), &a.settingsDraft.HideOnStartup)
		a.switchRow(c, locales.SettingsRunAtLogin(), locales.SettingsRunAtLoginHint(), &a.settingsDraft.RunAtLogin)
	})
	settingCard(c, locales.SettingsGeneralWindowCard(), func(c *ui.Context) {
		placements := placementChoices()
		placementLabel := placements[0].label
		for _, choice := range placements {
			if choice.value == a.settingsDraft.Placement {
				placementLabel = choice.label
			}
		}
		a.themedSelect(c, "placement", locales.SettingsPlacement(), locales.SettingsPlacementHint(), choiceLabels(placements), placementLabel, func(label string) {
			for _, choice := range placements {
				if choice.label == label {
					a.settingsDraft.Placement = choice.value
				}
			}
		})
		a.panelSizePreview(c)
		a.numberRow(c, locales.SettingsMaxResults(), locales.SettingsMaxResultsHint(), &a.settingsDraft.MaxResults, 5, 200, 5)
		a.switchRow(c, locales.SettingsShowRecent(), locales.SettingsShowRecentHint(), &a.settingsDraft.ShowRecentEntries)
	})
}

func (a *App) appearancePage(c *ui.Context) {
	draft := &a.settingsDraft.Theme
	background := draft.Background
	settingCard(c, locales.SettingsThemeModeCard(), func(c *ui.Context) {
		modes := themeModeChoices()
		modeLabel := modes[0].label
		for _, choice := range modes {
			if choice.value == draft.Mode {
				modeLabel = choice.label
			}
		}
		a.themedSelect(c, "mode", locales.SettingsThemeMode(), locales.SettingsThemeModeHint(), choiceLabels(modes), modeLabel, func(label string) {
			for _, choice := range modes {
				if choice.label == label {
					draft.Mode = choice.value
				}
			}
		})
		backgrounds := backgroundChoices()
		backgroundLabel := backgrounds[0].label
		for _, choice := range backgrounds {
			if choice.value == background {
				backgroundLabel = choice.label
			}
		}
		a.themedSelect(c, "background", locales.SettingsThemeBackground(), locales.SettingsThemeBackgroundHint(), choiceLabels(backgrounds), backgroundLabel, func(label string) {
			for _, choice := range backgrounds {
				if choice.label == label {
					draft.Background = choice.value
				}
			}
		})
	})
	settingCard(c, locales.SettingsThemeColorsCard(), func(c *ui.Context) {
		theme := c.Theme()
		a.colorRow(c, locales.SettingsAccentColor(), locales.SettingsAccentColorHint(), &draft.AccentColor, theme.Accent, false)
		a.colorRow(c, locales.SettingsTextColor(), locales.SettingsTextColorHint(), &draft.ForegroundColor, theme.Text, false)
		switch background {
		case settings.BackgroundSolid, settings.BackgroundGradient:
			a.colorRow(c, locales.SettingsBackgroundColor(), locales.SettingsBackgroundColorSolidHint(), &draft.BackgroundColor, theme.Background, true)
		case settings.BackgroundAcrylic, settings.BackgroundMica:
			a.colorRow(c, locales.SettingsBackgroundColor(), locales.SettingsBackgroundColorBlurHint(), &draft.BackgroundColor, theme.Background, true)
		}
		if background == settings.BackgroundGradient {
			a.colorRow(c, locales.SettingsGradientEnd(), locales.SettingsGradientEndHint(), &draft.GradientColor, theme.Surface, true)
		}
	})
	settingCard(c, locales.SettingsOpacity(), func(c *ui.Context) {
		hint := locales.SettingsOpacityHint()
		switch background {
		case settings.BackgroundAcrylic, settings.BackgroundMica:
			hint = locales.SettingsOpacityBlurHint()
		case settings.BackgroundImage:
			hint = locales.SettingsOpacityImageHint()
		}
		a.sliderRow(c, locales.SettingsOpacity(), hint, &draft.Opacity, 0, 1, func(v float64) string {
			return fmt.Sprintf("%.0f%%", v*100)
		})
	})
	if background == settings.BackgroundImage {
		settingCard(c, locales.SettingsImageCard(), func(c *ui.Context) {
			settingRow(c, locales.SettingsImagePath(), locales.SettingsImagePathHint(), func(c *ui.Context) {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c, &draft.ImagePath).Placeholder(locales.SettingsImagePlaceholder()).Width(260).FontSize(13)
					if compactButton(c, locales.SettingsImageChoose()).Clicked() {
						go func() {
							path, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: locales.SettingsDialogBackgroundImage()})
							if err == nil && len(path) > 0 {
								a.settingsWin.Update(func() { draft.ImagePath = path[0] })
							}
						}()
					}
				})
			})
			a.switchRow(c, locales.SettingsImageCover(), locales.SettingsImageCoverHint(), &draft.ImageCover)
			a.sliderRow(c, locales.SettingsImageMask(), locales.SettingsImageMaskHint(), &draft.ImageMaskOpacity, 0, 0.9, func(v float64) string {
				return fmt.Sprintf("%.0f%%", v*100)
			})
			if draft.ImageBlur != a.blurSliderInt {
				a.blurSlider = float64(draft.ImageBlur)
				a.blurSliderInt = draft.ImageBlur
			} else {
				a.blurSliderInt = int(a.blurSlider)
				draft.ImageBlur = a.blurSliderInt
			}
			a.sliderRow(c, locales.SettingsImageBlur(), locales.SettingsImageBlurHint(), &a.blurSlider, 0, 24, func(v float64) string {
				return fmt.Sprintf("%.0f", v)
			})
		})
	}
	settingCard(c, locales.SettingsFontCard(), func(c *ui.Context) {
		a.textRow(c, locales.SettingsFont(), locales.SettingsFontHint(), "Segoe UI Variable", &draft.FontFamily, 240)
		a.floatRow(c, locales.SettingsFontSizeLabel(), locales.SettingsFontSizeHint(), &draft.FontSize, 12, 32, 1)
		a.floatRow(c, locales.SettingsSearchHeight(), locales.SettingsSearchHeightHint(), &draft.SearchHeight, 36, 120, 2)
		a.floatRow(c, locales.SettingsRowPadding(), locales.SettingsRowPaddingHint(), &draft.RowPadding, 2, 24, 1)
		a.floatRow(c, locales.SettingsCornerRadius(), locales.SettingsCornerRadiusHint(), &draft.CornerRadius, 0, 32, 1)
	})
	settingCard(c, locales.SettingsThemeFileCard(), func(c *ui.Context) {
		ui.Row(c).Gap(8).Children(func() {
			if compactButton(c, locales.SettingsExportTheme()).Clicked() {
				go a.exportTheme()
			}
			if compactButton(c, locales.SettingsImportTheme()).Clicked() {
				a.importTheme()
			}
		})
	})
	// Preview the draft as it changes: both windows redraw with it until
	// the dialog is saved or cancelled. Skipped headless, where theme
	// changes have no main loop to apply them on.
	if a.panel != nil && a.settingsDraft.Theme != a.lastPreview {
		a.lastPreview = a.settingsDraft.Theme
		a.applyThemeFrom(a.settingsDraft.Theme)
	}
}

func (a *App) filesPage(c *ui.Context) {
	settingCard(c, locales.SettingsFilesIndexCard(), func(c *ui.Context) {
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, locales.SettingsFilesDirectoriesHint()).FontSize(12).TextColor(c.Theme().TextMuted)
			directories := strings.Join(a.settingsDraft.Files.Directories, "\n")
			if ui.TextArea(c, &directories).Lines(4, 8).Height(110).Changed() {
				a.settingsDraft.Files.Directories = splitLines(directories)
			}
		})
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, locales.SettingsFilesExcluded()).FontSize(12).TextColor(c.Theme().TextMuted)
			excluded := strings.Join(a.settingsDraft.Files.ExcludedDirectories, "\n")
			if ui.TextArea(c, &excluded).Lines(2, 6).Height(70).Changed() {
				a.settingsDraft.Files.ExcludedDirectories = splitLines(excluded)
			}
		})
		a.numberRow(c, locales.SettingsFilesMaxEntries(), locales.SettingsFilesMaxEntriesHint(), &a.settingsDraft.Files.MaxEntries, 1000, 500000, 1000)
		a.switchRow(c, locales.SettingsFilesIncludeDirectories(), locales.SettingsFilesIncludeDirectoriesHint(), &a.settingsDraft.Files.IncludeDirectories)
		a.switchRow(c, locales.SettingsFilesEverything(), locales.SettingsFilesEverythingHint(), &a.settingsDraft.Files.EnableEverything)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if compactButton(c, locales.SettingsFilesRebuild()).Clicked() && a.fileIndex != nil {
				go func() {
					if err := a.fileIndex.Rebuild(context.Background()); err != nil {
						a.settingsWin.Update(func() { a.settingsError = err.Error() })
					}
				}()
			}
			if a.fileIndex != nil && a.fileIndex.Warning() != "" {
				ui.Text(c, a.fileIndex.Warning()).FontSize(11).TextColor(c.Theme().Danger)
			}
		})
	})
}

func (a *App) volumePage(c *ui.Context) {
	settingCard(c, locales.SettingsVolumeCard(), func(c *ui.Context) {
		a.switchRow(c, locales.CommonEnable(), locales.SettingsVolumeEnableHint(), &a.settingsDraft.VolumeWheel.Enabled)
		edges := a.settingsDraft.VolumeWheel.Edges
		top, bottom := edges&settings.EdgeTop != 0, edges&settings.EdgeBottom != 0
		left, right := edges&settings.EdgeLeft != 0, edges&settings.EdgeRight != 0
		settingRow(c, locales.SettingsVolumeEdges(), locales.SettingsVolumeEdgesHint(), func(c *ui.Context) {
			ui.Row(c).Gap(16).Children(func() {
				ui.Checkbox(c, &top, locales.SettingsVolumeEdgeTop())
				ui.Checkbox(c, &bottom, locales.SettingsVolumeEdgeBottom())
				ui.Checkbox(c, &left, locales.SettingsVolumeEdgeLeft())
				ui.Checkbox(c, &right, locales.SettingsVolumeEdgeRight())
			})
		})
		a.settingsDraft.VolumeWheel.Edges = edgeMask(top, bottom, left, right)
		a.numberRow(c, locales.SettingsVolumeThickness(), locales.SettingsVolumeThicknessHint(), &a.settingsDraft.VolumeWheel.ThicknessDip, 1, 32, 1)
		a.numberRow(c, locales.SettingsVolumeStep(), locales.SettingsVolumeStepHint(), &a.settingsDraft.VolumeWheel.StepPercent, 1, 10, 1)
	})
}

func (a *App) sitesPage(c *ui.Context) {
	settingCard(c, locales.SettingsSitesCard(), func(c *ui.Context) {
		ui.Text(c, locales.SettingsSitesHint()).
			FontSize(12).TextColor(c.Theme().TextMuted)
		remove := -1
		for i := range a.settingsDraft.WebSites {
			site := &a.settingsDraft.WebSites[i]
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &site.Keyword).Placeholder(locales.SettingsSitesKeyword()).Width(90)
				ui.TextInput(c, &site.Name).Placeholder(locales.SettingsSitesName()).Width(130)
				ui.TextInput(c, &site.URLTemplate).Placeholder(locales.SettingsSitesURLPlaceholder()).Grow(1)
				ui.Switch(c, &site.Enabled)
				if compactButton(c, locales.CommonDelete()).Clicked() {
					remove = i
					return
				}
			})
			if remove >= 0 {
				break
			}
		}
		if remove >= 0 {
			a.settingsDraft.WebSites = append(a.settingsDraft.WebSites[:remove], a.settingsDraft.WebSites[remove+1:]...)
		}
		if compactButton(c, locales.SettingsSitesAdd()).Clicked() {
			a.settingsDraft.WebSites = append(a.settingsDraft.WebSites, settings.WebSearchSite{Enabled: true})
		}
	})
}

func (a *App) launchItemsPage(c *ui.Context) {
	settingCard(c, locales.SettingsItemsCard(), func(c *ui.Context) {
		remove := -1
		for i := range a.settingsDraft.CustomItems {
			item := &a.settingsDraft.CustomItems[i]
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &item.Name).Placeholder(locales.SettingsItemsName()).Width(140)
				ui.TextInput(c, &item.Target).Placeholder(locales.SettingsItemsTarget()).Grow(1)
				ui.TextInput(c, &item.Arguments).Placeholder(locales.SettingsItemsArguments()).Width(120)
				ui.Switch(c, &item.Enabled)
				if compactButton(c, locales.CommonDelete()).Clicked() {
					remove = i
					return
				}
			})
			if remove >= 0 {
				break
			}
		}
		if remove >= 0 {
			a.settingsDraft.CustomItems = append(a.settingsDraft.CustomItems[:remove], a.settingsDraft.CustomItems[remove+1:]...)
		}
		if compactButton(c, locales.SettingsItemsAdd()).Clicked() {
			a.settingsDraft.CustomItems = append(a.settingsDraft.CustomItems, settings.CustomLaunchItem{
				ID: settings.NewCustomItemID(), Enabled: true,
			})
		}
	})
	settingCard(c, locales.SettingsWorkspacesCard(), func(c *ui.Context) {
		ui.Text(c, locales.SettingsWorkspacesHint()).FontSize(12).TextColor(c.Theme().TextMuted)
		remove := -1
		for i := range a.settingsDraft.Workspaces {
			workspace := &a.settingsDraft.Workspaces[i]
			items := strings.Join(workspace.ItemIDs, ",")
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &workspace.Name).Placeholder(locales.SettingsWorkspacesName()).Width(140)
				ui.TextInput(c, &workspace.Aliases).Placeholder(locales.SettingsWorkspacesAliases()).Width(100)
				if ui.TextInput(c, &items).Placeholder(locales.SettingsWorkspacesItemIDs()).Changed() {
					workspace.ItemIDs = splitComma(items)
				}
				ui.Switch(c, &workspace.Enabled)
				if compactButton(c, locales.CommonDelete()).Clicked() {
					remove = i
					return
				}
			})
			if remove >= 0 {
				break
			}
		}
		if remove >= 0 {
			a.settingsDraft.Workspaces = append(a.settingsDraft.Workspaces[:remove], a.settingsDraft.Workspaces[remove+1:]...)
		}
		if compactButton(c, locales.SettingsWorkspacesAdd()).Clicked() {
			a.settingsDraft.Workspaces = append(a.settingsDraft.Workspaces, settings.Workspace{ID: settings.NewWorkspaceID(), Enabled: true})
		}
	})
}

func (a *App) pluginsPage(c *ui.Context) {
	theme := c.Theme()
	if a.pluginRepo == nil || a.plugins == nil {
		settingCard(c, locales.SettingsPagePlugins(), func(c *ui.Context) {
			ui.Text(c, locales.SettingsPluginsUninitialized()).
				FontSize(12).TextColor(theme.TextMuted)
		})
		return
	}
	statuses := a.plugins.Statuses()
	a.installedPluginsCard(c, theme, statuses)
	a.installPluginsCard(c, theme)
	a.pluginProblemsCard(c, theme)
	a.pluginRuntimeCard(c, theme, statuses)
}

// installedPluginsCard lists what is installed, one plate per plugin: the
// name and its condition on the first line, then id, version, description and
// the numbers the runtime kept, so nothing has to be read out of one long line.
func (a *App) installedPluginsCard(c *ui.Context, theme *ui.Theme, statuses []plugin.Status) {
	settingCard(c, locales.SettingsPluginsInstalledCard(), func(c *ui.Context) {
		if len(statuses) == 0 {
			a.emptyPluginsHint(c, theme)
			return
		}
		for _, status := range statuses {
			a.pluginEntry(c, theme, status)
		}
	})
}

// emptyPluginsHint explains what a plugin is good for; a page that says only
// "nothing here" leaves the user with no reason to install one.
func (a *App) emptyPluginsHint(c *ui.Context, theme *ui.Theme) {
	ui.Column(c).Gap(4).Children(func() {
		ui.Text(c, locales.SettingsPluginsEmptyTitle()).FontSize(13).Bold().TextColor(theme.Text)
		ui.Text(c, locales.SettingsPluginsEmptyHint1()).
			FontSize(12).TextColor(theme.TextMuted).Wrap()
		ui.Text(c, locales.SettingsPluginsEmptyHint2()).
			FontSize(12).TextColor(theme.TextMuted).Wrap()
		ui.Text(c, locales.SettingsPluginsEmptyHint3()).
			FontSize(12).TextColor(theme.TextMuted).Wrap()
	})
}

// pluginEntry is one installed plugin. The plate keeps its lines together and
// apart from the next plugin, which a flat list of rows did not: a description
// and a version looked like the same block.
func (a *App) pluginEntry(c *ui.Context, theme *ui.Theme, status plugin.Status) {
	manifest := status.Registration.Manifest
	enabled := status.Enabled
	global := status.GlobalQuery
	ui.Column(c).Gap(6).Padding(12, 14).Radius(8).
		Background(theme.Surface.Alpha(0.5)).Border(1, theme.Border).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, manifest.Name).FontSize(13).Bold().TextColor(theme.Text).Grow(1).SingleLine()
			pluginStatePill(c, theme, status.State())
			if status.State() == plugin.StatePaused || status.State() == plugin.StateFailed {
				if compactButton(c, locales.SettingsPluginsRestart()).Clicked() {
					// Stop waits for a query in flight; keep it off the UI thread.
					opts := a.settingsDraft.Plugins
					go func() {
						a.plugins.Restart(manifest.ID)
						a.plugins.Configure(a.pluginRepo.Installed(), opts)
						a.settingsWin.Update(func() {})
					}()
				}
			}
			if compactButton(c, locales.CommonUninstall()).Clicked() {
				go a.uninstallPlugin(manifest.ID)
			}
			if ui.Switch(c, &enabled).Changed() {
				a.setPluginEnabled(manifest.ID, enabled)
			}
		})
		ui.Text(c, locales.SettingsPluginsVersion(manifest.ID, manifest.Version)).
			FontSize(11).TextColor(theme.TextMuted).SingleLine()
		if description := strings.TrimSpace(manifest.Description); description != "" {
			ui.Text(c, description).FontSize(12).TextColor(theme.TextMuted).Wrap()
		}
		if len(manifest.Keywords) > 0 {
			ui.Text(c, locales.SettingsPluginsKeywords(strings.Join(manifest.Keywords, "、"))).
				FontSize(11).TextColor(theme.TextMuted).Wrap()
		}
		ui.Text(c, status.RuntimeText()).FontSize(11).TextColor(theme.TextMuted).Wrap()
		if status.Dropped > 0 && status.DropNote != "" {
			ui.Text(c, status.DropNote).FontSize(11).TextColor(theme.Warning).Wrap()
		}
		if status.Error != "" && status.State() != plugin.StateRunning {
			ui.Text(c, status.Error).FontSize(11).TextColor(theme.Danger).Wrap()
		}
		if stderr := strings.TrimSpace(status.Stderr); stderr != "" {
			ui.Text(c, locales.SettingsPluginsStderr(tailText(stderr, 400))).
				FontSize(11).TextColor(theme.TextMuted).Wrap()
		}
		if enabled {
			// The switch in the header decides whether the plugin is asked at
			// all; this decides whether it needs its keywords first.
			if ui.Checkbox(c, &global, locales.SettingsPluginsGlobalQuery()).Changed() {
				a.setPluginGlobalQuery(manifest.ID, global)
			}
		}
	})
}

// pluginStatePill shows the condition of a plugin as a small tag: the color
// says it at a glance, the word says it exactly. Colors come from the theme,
// so the tag follows light, dark and custom themes.
func pluginStatePill(c *ui.Context, theme *ui.Theme, state plugin.State) {
	color := theme.TextMuted
	switch state {
	case plugin.StateRunning:
		color = theme.Success
	case plugin.StatePaused:
		color = theme.Warning
	case plugin.StateFailed:
		color = theme.Danger
	}
	ui.Row(c).Shrink(0).Padding(1, 8).Radius(999).Background(color.Alpha(0.16)).Children(func() {
		ui.Text(c, state.String()).FontSize(11).TextColor(color).SingleLine()
	})
}

// installPluginsCard is where a plugin comes from: the bundled ones behind a
// picker — one row however many official plugins ship — then the two ways to
// install a third-party package.
func (a *App) installPluginsCard(c *ui.Context, theme *ui.Theme) {
	settingCard(c, locales.SettingsPluginsInstallCard(), func(c *ui.Context) {
		ui.Text(c, locales.SettingsPluginsInstallHint()).
			FontSize(12).TextColor(theme.TextMuted).Wrap()
		installed := a.pluginRepo.Installed()
		pending := make([]plugin.OfficialPlugin, 0)
		for _, official := range plugin.OfficialPlugins() {
			if !pluginInstalled(installed, official.ID) {
				pending = append(pending, official)
			}
		}
		if len(pending) > 0 {
			names := make([]string, 0, len(pending))
			for _, official := range pending {
				names = append(names, official.Name)
			}
			pick := a.officialPick
			if !containsFold(names, pick) {
				pick = names[0]
			}
			summary := ""
			pickID := ""
			for _, official := range pending {
				if official.Name == pick {
					summary = official.Summary
					pickID = official.ID
				}
			}
			a.themedSelect(c, "officialPlugin", locales.SettingsPluginsOfficialPick(),
				locales.SettingsPluginsOfficialPickHint(), names, pick,
				func(label string) { a.officialPick = label })
			if summary != "" {
				ui.Text(c, summary).FontSize(11).TextColor(theme.TextMuted).Wrap()
			}
			if compactPrimaryButton(c, locales.CommonInstall()).Clicked() && pickID != "" {
				go a.installOfficialPlugin(pickID)
			}
		}
		ui.Divider(c)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if compactButton(c, locales.SettingsPluginsInstallZip()).Clicked() {
				go a.installPlugin(false)
			}
			if compactButton(c, locales.SettingsPluginsInstallDir()).Clicked() {
				go a.installPlugin(true)
			}
			ui.Text(c, a.pluginRepo.Dir()).FontSize(11).TextColor(theme.TextMuted).Grow(1).SingleLine()
		})
	})
}

// pluginProblemsCard lists the packages on disk that could not be loaded, one
// per line. The old page kept only the last warning, joined into a paragraph,
// so a broken package could hide behind a working one.
func (a *App) pluginProblemsCard(c *ui.Context, theme *ui.Theme) {
	problems := a.pluginRepo.Problems()
	if len(problems) == 0 {
		return
	}
	settingCard(c, locales.SettingsPluginsProblemsCard(), func(c *ui.Context) {
		for _, problem := range problems {
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, problem.Directory).FontSize(12).TextColor(theme.Text).SingleLine()
				ui.Text(c, problem.Message).FontSize(11).TextColor(theme.Danger).Wrap()
			})
		}
		ui.Text(c, locales.SettingsPluginsProblemsHint()).
			FontSize(11).TextColor(theme.TextMuted).Wrap()
	})
}

// pluginRuntimeCard sums up how the plugin runtime is set up and how much of it
// is in use, which is what one asks when a plugin "does not work".
func (a *App) pluginRuntimeCard(c *ui.Context, theme *ui.Theme, statuses []plugin.Status) {
	options := a.settingsDraft.Plugins
	running := 0
	for _, status := range statuses {
		if status.Running {
			running++
		}
	}
	enabled := 0
	for _, status := range statuses {
		if status.Enabled {
			enabled++
		}
	}
	settingCard(c, locales.SettingsPluginsRuntimeCard(), func(c *ui.Context) {
		ui.Text(c, locales.SettingsPluginsRuntimeStats(enabled, running, options.MaxActiveProcesses, options.IdleSeconds, options.QueryTimeoutMs)).
			FontSize(11).TextColor(theme.TextMuted).Wrap()
		ui.Text(c, locales.SettingsPluginsTrustHint()).
			FontSize(11).TextColor(theme.TextMuted).Wrap()
	})
}

// setPluginEnabled turns one plugin on or off and reconfigures the runtime at
// once, so the next search already follows the switch instead of the saved
// settings a moment later.
func (a *App) setPluginEnabled(id string, enabled bool) {
	a.settingsDraft.Plugins.EnabledIDs = toggleID(a.settingsDraft.Plugins.EnabledIDs, id, enabled)
	a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
}

// setPluginGlobalQuery chooses whether a plugin is asked on every search or
// only after its keywords. The setting existed but no page could reach it.
func (a *App) setPluginGlobalQuery(id string, global bool) {
	a.settingsDraft.Plugins.GlobalQueryIDs = toggleID(a.settingsDraft.Plugins.GlobalQueryIDs, id, global)
	a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
}

// uninstallPlugin stops the process, removes the package and forgets the id in
// the settings, so a later reinstall starts from a clean state instead of an
// "enabled" entry that points at nothing.
func (a *App) uninstallPlugin(id string) {
	a.plugins.Stop(id)
	if err := a.pluginRepo.Uninstall(id); err != nil {
		a.settingsWin.Update(func() { a.settingsError = err.Error() })
		return
	}
	a.settingsDraft.Plugins.EnabledIDs = toggleID(a.settingsDraft.Plugins.EnabledIDs, id, false)
	a.settingsDraft.Plugins.GlobalQueryIDs = toggleID(a.settingsDraft.Plugins.GlobalQueryIDs, id, false)
	a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
	a.settingsWin.Update(func() { a.settingsError = "" })
}

// installOfficialPlugin installs a bundled example and switches it on in one
// step: an installed but disabled plugin looks like a broken one, because the
// search stays empty until it is enabled.
func (a *App) installOfficialPlugin(id string) {
	a.installOfficial(id)
	if !pluginInstalled(a.pluginRepo.Installed(), id) {
		return
	}
	if !containsFold(a.settingsDraft.Plugins.EnabledIDs, id) {
		a.setPluginEnabled(id, true)
	}
}

// tailText keeps the end of a long message, where the actual error is, and
// marks what it cut.
func tailText(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return "…" + string(runes[len(runes)-limit:])
}

func (a *App) aboutPage(c *ui.Context) {
	settingCard(c, locales.SettingsAboutCard(), func(c *ui.Context) {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if logo := a.brandBitmap(); logo != nil {
				ui.Image(c, logo).Size(40, 40).Radius(9)
			}
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, "FluentLauncher "+appVersion()).FontSize(14).Bold().TextColor(c.Theme().Text)
				ui.Text(c, locales.SettingsAboutDataDir(a.dataDir)).FontSize(12).TextColor(c.Theme().TextMuted)
			})
		})
		ui.Text(c, locales.SettingsAboutHiddenCount(a.hiddenCount())).FontSize(12).TextColor(c.Theme().TextMuted)
	})
	a.updateCard(c)
	settingCard(c, locales.SettingsDataCard(), func(c *ui.Context) {
		ui.Row(c).Gap(8).Children(func() {
			if compactButton(c, locales.SettingsExportConfig()).Clicked() {
				go a.exportConfig()
			}
			if compactButton(c, locales.SettingsImportConfig()).Clicked() {
				a.importConfig()
			}
			if compactButton(c, locales.SettingsResetLearning()).Clicked() {
				a.resetLearning()
			}
			if compactButton(c, locales.SettingsRestoreHidden()).Clicked() {
				// One pass over the profiles, one engine refresh, one save
				// burst: per-entry changeProfile would scan every engine and
				// restart the search once per hidden entry.
				a.profilesMu.Lock()
				changed := make([]model.Profile, 0)
				for id, profile := range a.profiles {
					if profile.Hidden {
						profile.Hidden = false
						a.profiles[id] = profile
						changed = append(changed, profile)
					}
				}
				a.profilesMu.Unlock()
				if a.db != nil {
					go func() {
						for _, profile := range changed {
							_ = a.db.SaveProfile(context.Background(), profile)
						}
					}()
				}
				m := a.profileMap()
				a.appEngine.UpdateProfiles(m)
				a.fileEngine.UpdateProfiles(m)
				a.customEngine.UpdateProfiles(m)
				a.workspaceEngine.UpdateProfiles(m)
				a.settingsError = ""
				a.settingsNotice = locales.SettingsHiddenRestored(len(changed))
				a.updateResults()
			}
		})
	})
}

func (a *App) installOfficial(id string) {
	resources, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		a.settingsWin.Update(func() { a.settingsError = err.Error() })
		return
	}
	source := filepath.Join(resources, "examples", id)
	if _, err := a.pluginRepo.Install(source, false); err != nil {
		a.settingsWin.Update(func() { a.settingsError = err.Error() })
		return
	}
	a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
	a.settingsWin.Update(func() { a.settingsError = "" })
}

func pluginInstalled(installed []protocol.Registration, id string) bool {
	for _, registration := range installed {
		if registration.Manifest.ID == id {
			return true
		}
	}
	return false
}

func (a *App) installPlugin(directory bool) {
	options := mygo.OpenDialogOptions{Title: locales.SettingsDialogPluginPackage()}
	if directory {
		options.Directory = true
	} else {
		options.Filters = []mygo.FileFilter{{Name: locales.SettingsDialogPluginPackageFilter(), Extensions: []string{"zip"}}}
	}
	paths, err := mygo.Dialog.Open(options)
	if err != nil || len(paths) == 0 {
		return
	}
	check, err := a.pluginRepo.CheckUpdate(paths[0])
	if err != nil {
		a.settingsWin.Update(func() { a.settingsError = err.Error() })
		return
	}
	if check.IsUpgrade() {
		a.plugins.Stop(check.Package.ID)
	}
	registration, err := a.pluginRepo.Install(paths[0], check.IsUpgrade())
	if err != nil {
		a.settingsWin.Update(func() { a.settingsError = err.Error() })
		return
	}
	// A freshly installed plugin is switched on: installing it and finding a
	// silent search is the same surprise as a broken package. This runs
	// off the UI thread, so the draft is touched inside Update like every
	// other change to it from here.
	id := registration.Manifest.ID
	a.settingsWin.Update(func() {
		a.settingsDraft.Plugins.EnabledIDs = toggleID(a.settingsDraft.Plugins.EnabledIDs, id, true)
		a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
		a.settingsError = ""
	})
}

func containsFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

func toggleID(items []string, id string, enabled bool) []string {
	out := make([]string, 0, len(items)+1)
	for _, item := range items {
		if !strings.EqualFold(item, id) {
			out = append(out, item)
		}
	}
	if enabled {
		out = append(out, id)
	}
	return out
}

func (a *App) exportConfig() {
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Title: locales.SettingsDialogExportConfig(), DefaultPath: "FluentLauncher-config.json"})
	if err != nil || path == "" {
		return
	}
	pkg := store.ConfigurationPackage{Settings: a.settings, Profiles: a.profileSlice(), IncludesLearning: true}
	if err := a.config.Export(path, pkg); err != nil {
		a.settingsWin.Update(func() { a.settingsError = err.Error() })
	}
}

func (a *App) importConfig() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: locales.SettingsDialogImportConfig(), Filters: []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}}})
	if err != nil || len(paths) == 0 {
		return
	}
	// The file and database work stays off the UI thread; everything that
	// touches app state applies inside the window's update, like every other
	// change from here.
	go func() {
		pkg, err := a.config.Import(paths[0])
		if err != nil {
			a.settingsWin.Update(func() { a.settingsError = err.Error() })
			return
		}
		var profiles map[string]model.Profile
		if a.db != nil {
			if err := a.db.ImportProfiles(context.Background(), pkg.Profiles, pkg.IncludesLearning); err == nil {
				profiles, _ = a.db.LoadProfiles(context.Background())
			}
		}
		a.settingsWin.Update(func() {
			if err := a.applySettings(pkg.Settings); err != nil {
				a.settingsError = err.Error()
				return
			}
			if profiles != nil {
				a.profilesMu.Lock()
				a.profiles = profiles
				a.profilesMu.Unlock()
				m := a.profileMap()
				a.appEngine.UpdateProfiles(m)
				a.fileEngine.UpdateProfiles(m)
				a.customEngine.UpdateProfiles(m)
				a.workspaceEngine.UpdateProfiles(m)
				a.updateResults()
			}
			a.settingsError = ""
		})
	}()
}

func (a *App) exportTheme() {
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Title: locales.SettingsDialogExportTheme(), DefaultPath: "FluentLauncher-theme.zip",
		Filters: []mygo.FileFilter{{Name: locales.SettingsDialogThemePackageFilter(), Extensions: []string{"zip"}}}})
	if err != nil || path == "" {
		return
	}
	if err := a.themes.Export(a.settingsDraft.Theme, path); err != nil {
		a.settingsWin.Update(func() { a.settingsError = err.Error() })
	}
}

func (a *App) importTheme() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: locales.SettingsDialogImportTheme(),
		Filters: []mygo.FileFilter{{Name: locales.SettingsDialogThemePackageFilter(), Extensions: []string{"zip"}}}})
	if err != nil || len(paths) == 0 {
		return
	}
	go func() {
		theme, err := a.themes.Import(paths[0])
		if err != nil {
			a.settingsWin.Update(func() { a.settingsError = err.Error() })
			return
		}
		a.settingsWin.Update(func() {
			a.settingsDraft.Theme = theme
			a.settingsError = ""
		})
	}()
}

func (a *App) resetLearning() {
	if a.db == nil {
		return
	}
	db := a.db
	go func() {
		if err := db.ResetLearning(context.Background()); err != nil {
			a.settingsWin.Update(func() { a.settingsError = err.Error() })
			return
		}
		a.settingsWin.Update(a.finishResetLearning)
	}()
}

// finishResetLearning clears the learned profiles in memory, refreshes the
// engines and reports the outcome. It runs on the UI thread.
func (a *App) finishResetLearning() {
	a.profilesMu.Lock()
	for id, profile := range a.profiles {
		profile.UseCount = 0
		profile.LastUsedAt = nil
		a.profiles[id] = profile
	}
	a.profilesMu.Unlock()
	m := a.profileMap()
	a.appEngine.UpdateProfiles(m)
	a.fileEngine.UpdateProfiles(m)
	a.customEngine.UpdateProfiles(m)
	a.workspaceEngine.UpdateProfiles(m)
	a.settingsError = ""
	a.settingsNotice = locales.SettingsLearningResetDone()
	a.updateResults()
}

func cloneSettings(value settings.LauncherSettings) settings.LauncherSettings {
	data, _ := json.Marshal(value)
	var out settings.LauncherSettings
	_ = json.Unmarshal(data, &out)
	return out
}

func splitComma(text string) []string {
	parts := strings.Split(text, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func splitLines(text string) []string {
	lines := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' })
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

func edgeMask(top, bottom, left, right bool) settings.ScreenEdges {
	var edges settings.ScreenEdges
	if top {
		edges |= settings.EdgeTop
	}
	if bottom {
		edges |= settings.EdgeBottom
	}
	if left {
		edges |= settings.EdgeLeft
	}
	if right {
		edges |= settings.EdgeRight
	}
	return edges
}
