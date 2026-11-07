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
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/internal/store"
)

var settingPages = []string{"常规", "外观", "文件", "音量", "站点搜索", "启动项", "插件", "其他"}

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
	a.settingsPage = min(max(a.settingsPage, 0), len(settingPages)-1)

	a.drawShell(c, &theme, func() {
		a.captionBar(c, &theme, "FluentLauncher 设置", a.settingsWin)
		ui.Row(c).Grow(1).FillWidth().Children(func() {
			a.settingsSidebar(c, &theme)
			ui.Column(c).Grow(1).FillHeight().Children(func() {
				ui.Row(c).Padding(20, 24).AlignItems(ui.Center).Children(func() {
					ui.Text(c, settingPages[a.settingsPage]).FontSize(20).Bold().TextColor(theme.Text).Grow(1)
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
				ui.Row(c).Padding(12, 24).Gap(8).AlignItems(ui.Center).Children(func() {
					if a.settingsError != "" {
						ui.Text(c, a.settingsError).FontSize(12).TextColor(theme.Danger).Grow(1)
					} else {
						ui.Column(c).Grow(1)
					}
					if compactButton(c, "取消").Clicked() {
						a.cancelSettings()
					}
					if compactPrimaryButton(c, "保存").Clicked() {
						a.saveSettings()
					}
				})
			})
		})
	})
}

// settingsSidebar is the navigation column: transparent over the window
// material, with the same selected/hover treatment as the result rows. The
// mark and the window buttons live in the caption bar above it.
func (a *App) settingsSidebar(c *ui.Context, theme *ui.Theme) {
	hover := a.settingsHover
	next := -1
	ui.Column(c).Width(208).FillHeight().Padding(8, 12).Gap(2).Children(func() {
		for i, name := range settingPages {
			background := ui.Transparent
			text := theme.Text
			if i == a.settingsPage {
				background = theme.Accent.Alpha(0.85)
				text = theme.AccentText
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
						background = theme.Accent.Alpha(0.85)
						text = theme.AccentText
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
// keeps the system color and can be restored with 重置. A background color
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
			shown := "跟随系统"
			if *value != "" {
				shown = hexOf(picked)
			}
			ui.Text(c, shown).FontSize(12).TextColor(theme.TextMuted).Width(90).SingleLine()
			if *value != "" && compactButton(c, "重置").Clicked() {
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

var placementChoices = []struct {
	label string
	value settings.WindowPlacement
}{
	{"活动屏幕", settings.PlacementActiveScreen},
	{"鼠标所在屏幕", settings.PlacementMouseScreen},
	{"主屏幕", settings.PlacementPrimaryScreen},
}

var themeModeChoices = []struct {
	label string
	value settings.ThemeMode
}{
	{"跟随系统", settings.ThemeSystem},
	{"浅色", settings.ThemeLight},
	{"深色", settings.ThemeDark},
}

var backgroundChoices = []struct {
	label string
	value settings.BackgroundMode
}{
	{"亚克力", settings.BackgroundAcrylic},
	{"云母", settings.BackgroundMica},
	{"纯色", settings.BackgroundSolid},
	{"渐变", settings.BackgroundGradient},
	{"图片", settings.BackgroundImage},
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

func (a *App) generalPage(c *ui.Context) {
	settingCard(c, "唤醒与启动", func(c *ui.Context) {
		hotkeyHint := ""
		if a.hotkeyActive != "" && a.hotkeyActive != a.settingsDraft.Hotkey {
			hotkeyHint = "当前生效：" + a.hotkeyActive + "（所选热键被占用或未保存）"
		}
		a.themedSelect(c, "hotkey", "热键", hotkeyHint, []string{"Alt+Space", "Ctrl+Alt+Space", "Ctrl+Space"},
			a.settingsDraft.Hotkey, func(label string) { a.settingsDraft.Hotkey = label })
		a.switchRow(c, "启动时隐藏搜索框", "关闭后开机进入桌面即显示面板", &a.settingsDraft.HideOnStartup)
		a.switchRow(c, "开机启动", "登录系统后自动在后台运行", &a.settingsDraft.RunAtLogin)
	})
	settingCard(c, "窗口与结果", func(c *ui.Context) {
		placementLabel := placementChoices[0].label
		for _, choice := range placementChoices {
			if choice.value == a.settingsDraft.Placement {
				placementLabel = choice.label
			}
		}
		a.themedSelect(c, "placement", "弹出位置", "呼出面板时所在的屏幕", choiceLabels(placementChoices), placementLabel, func(label string) {
			for _, choice := range placementChoices {
				if choice.label == label {
					a.settingsDraft.Placement = choice.value
				}
			}
		})
		a.panelSizePreview(c)
		a.numberRow(c, "最大结果数", "每次搜索最多显示的条目", &a.settingsDraft.MaxResults, 5, 200, 5)
		a.switchRow(c, "空搜索时显示最近打开", "未输入关键词时列出最近使用的条目", &a.settingsDraft.ShowRecentEntries)
	})
}

func (a *App) appearancePage(c *ui.Context) {
	draft := &a.settingsDraft.Theme
	background := draft.Background
	settingCard(c, "模式与背景", func(c *ui.Context) {
		modeLabel := themeModeChoices[0].label
		for _, choice := range themeModeChoices {
			if choice.value == draft.Mode {
				modeLabel = choice.label
			}
		}
		a.themedSelect(c, "mode", "模式", "浅色、深色或跟随系统", choiceLabels(themeModeChoices), modeLabel, func(label string) {
			for _, choice := range themeModeChoices {
				if choice.label == label {
					draft.Mode = choice.value
				}
			}
		})
		backgroundLabel := backgroundChoices[0].label
		for _, choice := range backgroundChoices {
			if choice.value == background {
				backgroundLabel = choice.label
			}
		}
		a.themedSelect(c, "background", "背景材质", "亚克力与云母模糊透出下层窗口", choiceLabels(backgroundChoices), backgroundLabel, func(label string) {
			for _, choice := range backgroundChoices {
				if choice.label == label {
					draft.Background = choice.value
				}
			}
		})
	})
	settingCard(c, "颜色", func(c *ui.Context) {
		theme := c.Theme()
		a.colorRow(c, "强调色", "按钮与选中行", &draft.AccentColor, theme.Accent, false)
		a.colorRow(c, "文字色", "留空跟随系统", &draft.ForegroundColor, theme.Text, false)
		switch background {
		case settings.BackgroundSolid, settings.BackgroundGradient:
			a.colorRow(c, "背景色", "纯色与渐变起点，留空跟随系统", &draft.BackgroundColor, theme.Background, true)
		case settings.BackgroundAcrylic, settings.BackgroundMica:
			a.colorRow(c, "背景色", "模糊背景上的着色，留空跟随系统", &draft.BackgroundColor, theme.Background, true)
		}
		if background == settings.BackgroundGradient {
			a.colorRow(c, "渐变终点", "仅渐变背景使用", &draft.GradientColor, theme.Surface, true)
		}
	})
	settingCard(c, "不透明度", func(c *ui.Context) {
		hint := "0% 完全透明（只看到下层窗口），100% 完全不透明"
		switch background {
		case settings.BackgroundAcrylic, settings.BackgroundMica:
			hint = "模糊材质的着色强度：0% 只保留模糊后景，100% 完全不透明"
		case settings.BackgroundImage:
			hint = "图片整体的不透明度，蒙版另算"
		}
		a.sliderRow(c, "不透明度", hint, &draft.Opacity, 0, 1, func(v float64) string {
			return fmt.Sprintf("%.0f%%", v*100)
		})
	})
	if background == settings.BackgroundImage {
		settingCard(c, "背景图片", func(c *ui.Context) {
			settingRow(c, "图片路径", "背景材质为“图片”时使用", func(c *ui.Context) {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c, &draft.ImagePath).Placeholder("选择一张图片").Width(260).FontSize(13)
					if compactButton(c, "选择…").Clicked() {
						go func() {
							path, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "选择背景图片"})
							if err == nil && len(path) > 0 {
								a.settingsWin.Update(func() { draft.ImagePath = path[0] })
							}
						}()
					}
				})
			})
			a.switchRow(c, "图片铺满", "关闭则完整显示整张图片", &draft.ImageCover)
			a.sliderRow(c, "遮罩不透明度", "图片上的蒙版：越高图片越淡，0% 为原图", &draft.ImageMaskOpacity, 0, 0.9, func(v float64) string {
				return fmt.Sprintf("%.0f%%", v*100)
			})
			if draft.ImageBlur != a.blurSliderInt {
				a.blurSlider = float64(draft.ImageBlur)
				a.blurSliderInt = draft.ImageBlur
			} else {
				a.blurSliderInt = int(a.blurSlider)
				draft.ImageBlur = a.blurSliderInt
			}
			a.sliderRow(c, "高斯模糊", "0 – 24，0 为不模糊", &a.blurSlider, 0, 24, func(v float64) string {
				return fmt.Sprintf("%.0f", v)
			})
		})
	}
	settingCard(c, "字体与圆角", func(c *ui.Context) {
		a.textRow(c, "字体", "留空使用系统字体", "Segoe UI Variable", &draft.FontFamily, 240)
		a.floatRow(c, "字号", "12 – 32", &draft.FontSize, 12, 32, 1)
		a.floatRow(c, "搜索框高度", "36 – 120，搜索行的整体高度", &draft.SearchHeight, 36, 120, 2)
		a.floatRow(c, "结果行高度", "2 – 24，每个候选行的上下内边距，越大行越高", &draft.RowPadding, 2, 24, 1)
		a.floatRow(c, "圆角", "0 – 32", &draft.CornerRadius, 0, 32, 1)
	})
	settingCard(c, "主题文件", func(c *ui.Context) {
		ui.Row(c).Gap(8).Children(func() {
			if compactButton(c, "导出主题…").Clicked() {
				go a.exportTheme()
			}
			if compactButton(c, "导入主题…").Clicked() {
				go a.importTheme()
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
	settingCard(c, "索引目录", func(c *ui.Context) {
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, "目录（每行一个本地绝对路径）").FontSize(12).TextColor(c.Theme().TextMuted)
			directories := strings.Join(a.settingsDraft.Files.Directories, "\n")
			if ui.TextArea(c, &directories).Lines(4, 8).Height(110).Changed() {
				a.settingsDraft.Files.Directories = splitLines(directories)
			}
		})
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, "排除目录").FontSize(12).TextColor(c.Theme().TextMuted)
			excluded := strings.Join(a.settingsDraft.Files.ExcludedDirectories, "\n")
			if ui.TextArea(c, &excluded).Lines(2, 6).Height(70).Changed() {
				a.settingsDraft.Files.ExcludedDirectories = splitLines(excluded)
			}
		})
		a.numberRow(c, "最大条目数", "1000 – 500000", &a.settingsDraft.Files.MaxEntries, 1000, 500000, 1000)
		a.switchRow(c, "索引目录本身", "目录也作为可搜索结果", &a.settingsDraft.Files.IncludeDirectories)
		a.switchRow(c, "启用 Everything 连接器", "安装 Everything 时直接复用其索引", &a.settingsDraft.Files.EnableEverything)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if compactButton(c, "立即重建索引").Clicked() && a.fileIndex != nil {
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
	settingCard(c, "屏幕边缘滚轮调节音量", func(c *ui.Context) {
		a.switchRow(c, "启用", "在屏幕边缘滚动滚轮调节系统音量", &a.settingsDraft.VolumeWheel.Enabled)
		edges := a.settingsDraft.VolumeWheel.Edges
		top, bottom := edges&settings.EdgeTop != 0, edges&settings.EdgeBottom != 0
		left, right := edges&settings.EdgeLeft != 0, edges&settings.EdgeRight != 0
		settingRow(c, "触发边缘", "与相邻显示器相接的边不触发", func(c *ui.Context) {
			ui.Row(c).Gap(16).Children(func() {
				ui.Checkbox(c, &top, "上")
				ui.Checkbox(c, &bottom, "下")
				ui.Checkbox(c, &left, "左")
				ui.Checkbox(c, &right, "右")
			})
		})
		a.settingsDraft.VolumeWheel.Edges = edgeMask(top, bottom, left, right)
		a.numberRow(c, "热区厚度", "1 – 32 像素", &a.settingsDraft.VolumeWheel.ThicknessDip, 1, 32, 1)
		a.numberRow(c, "每次步进", "1 – 10%", &a.settingsDraft.VolumeWheel.StepPercent, 1, 10, 1)
	})
}

func (a *App) sitesPage(c *ui.Context) {
	settingCard(c, "站点搜索", func(c *ui.Context) {
		ui.Text(c, "输入“关键词 搜索词”即可在结果中追加该站点的搜索链接，模板中 {query} 会被替换为搜索词。").
			FontSize(12).TextColor(c.Theme().TextMuted)
		for i := range a.settingsDraft.WebSites {
			site := &a.settingsDraft.WebSites[i]
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &site.Keyword).Placeholder("关键词").Width(90)
				ui.TextInput(c, &site.Name).Placeholder("名称").Width(130)
				ui.TextInput(c, &site.URLTemplate).Placeholder("https://…/{query}").Grow(1)
				ui.Switch(c, &site.Enabled)
				if compactButton(c, "删除").Clicked() {
					a.settingsDraft.WebSites = append(a.settingsDraft.WebSites[:i], a.settingsDraft.WebSites[i+1:]...)
					return
				}
			})
		}
		if compactButton(c, "添加站点").Clicked() {
			a.settingsDraft.WebSites = append(a.settingsDraft.WebSites, settings.WebSearchSite{Enabled: true})
		}
	})
}

func (a *App) launchItemsPage(c *ui.Context) {
	settingCard(c, "自定义启动项", func(c *ui.Context) {
		for i := range a.settingsDraft.CustomItems {
			item := &a.settingsDraft.CustomItems[i]
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &item.Name).Placeholder("名称").Width(140)
				ui.TextInput(c, &item.Target).Placeholder("目标路径或网址").Grow(1)
				ui.TextInput(c, &item.Arguments).Placeholder("参数").Width(120)
				ui.Switch(c, &item.Enabled)
				if compactButton(c, "删除").Clicked() {
					a.settingsDraft.CustomItems = append(a.settingsDraft.CustomItems[:i], a.settingsDraft.CustomItems[i+1:]...)
					return
				}
			})
		}
		if compactButton(c, "添加启动项").Clicked() {
			a.settingsDraft.CustomItems = append(a.settingsDraft.CustomItems, settings.CustomLaunchItem{
				ID: settings.NewCustomItemID(), Enabled: true,
			})
		}
	})
	settingCard(c, "工作区", func(c *ui.Context) {
		ui.Text(c, "工作区按顺序打开下列启动项 ID 对应的程序。").FontSize(12).TextColor(c.Theme().TextMuted)
		for i := range a.settingsDraft.Workspaces {
			workspace := &a.settingsDraft.Workspaces[i]
			items := strings.Join(workspace.ItemIDs, ",")
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &workspace.Name).Placeholder("名称").Width(140)
				ui.TextInput(c, &workspace.Aliases).Placeholder("别名").Width(100)
				if ui.TextInput(c, &items).Placeholder("启动项 ID，逗号分隔").Changed() {
					workspace.ItemIDs = splitComma(items)
				}
				ui.Switch(c, &workspace.Enabled)
				if compactButton(c, "删除").Clicked() {
					a.settingsDraft.Workspaces = append(a.settingsDraft.Workspaces[:i], a.settingsDraft.Workspaces[i+1:]...)
					return
				}
			})
		}
		if compactButton(c, "添加工作区").Clicked() {
			a.settingsDraft.Workspaces = append(a.settingsDraft.Workspaces, settings.Workspace{ID: settings.NewWorkspaceID(), Enabled: true})
		}
	})
}

func (a *App) pluginsPage(c *ui.Context) {
	if a.pluginRepo == nil || a.plugins == nil {
		ui.Text(c, "插件运行时未初始化。").FontSize(12).TextColor(c.Theme().TextMuted)
		return
	}
	settingCard(c, "已安装插件", func(c *ui.Context) {
		installed := a.pluginRepo.Installed()
		if len(installed) == 0 {
			ui.Text(c, "尚未安装插件。").FontSize(12).TextColor(c.Theme().TextMuted)
		}
		for _, registration := range installed {
			manifest := registration.Manifest
			enabled := containsFold(a.settingsDraft.Plugins.EnabledIDs, manifest.ID)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Switch(c, &enabled)
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, manifest.Name).FontSize(13).TextColor(c.Theme().Text)
					ui.Text(c, manifest.Version+" · "+strings.Join(manifest.Keywords, ", ")).FontSize(11).TextColor(c.Theme().TextMuted)
				})
				if compactButton(c, "卸载").Clicked() {
					a.plugins.Stop(manifest.ID)
					_ = a.pluginRepo.Uninstall(manifest.ID)
					a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
				}
			})
			if enabled != containsFold(a.settingsDraft.Plugins.EnabledIDs, manifest.ID) {
				a.settingsDraft.Plugins.EnabledIDs = toggleID(a.settingsDraft.Plugins.EnabledIDs, manifest.ID, enabled)
				a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
			}
		}
	})
	settingCard(c, "安装", func(c *ui.Context) {
		ui.Row(c).Gap(8).Children(func() {
			for _, example := range []struct{ id, name string }{{"official.echo", "回显示例"}, {"official.devtools", "开发工具"}} {
				if !pluginInstalled(a.pluginRepo.Installed(), example.id) && compactButton(c, "安装官方 "+example.name).Clicked() {
					a.installOfficial(example.id)
				}
			}
			if compactButton(c, "安装 ZIP…").Clicked() {
				go a.installPlugin(false)
			}
			if compactButton(c, "安装目录…").Clicked() {
				go a.installPlugin(true)
			}
		})
		for _, diagnostic := range a.plugins.Diagnostics() {
			ui.Text(c, fmt.Sprintf("%s · PID %d · 启动 %.0fms · 查询 %.0fms %s",
				diagnostic.ID, diagnostic.PID, diagnostic.StartupMs, diagnostic.QueryMs, diagnostic.Error)).
				FontSize(11).TextColor(c.Theme().TextMuted)
		}
		if warning := a.pluginRepo.Warning(); warning != "" {
			ui.Text(c, warning).FontSize(11).TextColor(c.Theme().Danger)
		}
	})
}

func (a *App) aboutPage(c *ui.Context) {
	settingCard(c, "关于", func(c *ui.Context) {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if logo := a.brandBitmap(); logo != nil {
				ui.Image(c, logo).Size(40, 40).Radius(9)
			}
			ui.Column(c).Gap(2).Children(func() {
				ui.Text(c, "FluentLauncher 0.1.0").FontSize(14).Bold().TextColor(c.Theme().Text)
				ui.Text(c, "数据目录："+a.dataDir).FontSize(12).TextColor(c.Theme().TextMuted)
			})
		})
		hidden := 0
		for _, profile := range a.profileMap() {
			if profile.Hidden {
				hidden++
			}
		}
		ui.Text(c, fmt.Sprintf("隐藏条目：%d", hidden)).FontSize(12).TextColor(c.Theme().TextMuted)
	})
	settingCard(c, "配置与数据", func(c *ui.Context) {
		ui.Row(c).Gap(8).Children(func() {
			if compactButton(c, "导出配置…").Clicked() {
				go a.exportConfig()
			}
			if compactButton(c, "导入配置…").Clicked() {
				go a.importConfig()
			}
			if compactButton(c, "重置学习记录").Clicked() {
				a.resetLearning()
			}
			if compactButton(c, "恢复隐藏条目").Clicked() {
				for id, profile := range a.profileMap() {
					if profile.Hidden {
						a.changeProfile(id, func(p model.Profile) model.Profile {
							p.Hidden = false
							return p
						})
					}
				}
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
	options := mygo.OpenDialogOptions{Title: "选择插件包"}
	if directory {
		options.Directory = true
	} else {
		options.Filters = []mygo.FileFilter{{Name: "插件包", Extensions: []string{"zip"}}}
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
	if _, err := a.pluginRepo.Install(paths[0], check.IsUpgrade()); err != nil {
		a.settingsWin.Update(func() { a.settingsError = err.Error() })
		return
	}
	a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
	a.settingsWin.Update(func() { a.settingsError = "" })
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
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Title: "导出配置", DefaultPath: "FluentLauncher-config.json"})
	if err != nil || path == "" {
		return
	}
	pkg := store.ConfigurationPackage{Settings: a.settings, Profiles: a.profileSlice(), IncludesLearning: true}
	if err := a.config.Export(path, pkg); err != nil {
		a.settingsWin.Update(func() { a.settingsError = err.Error() })
	}
}

func (a *App) importConfig() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "导入配置", Filters: []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}}})
	if err != nil || len(paths) == 0 {
		return
	}
	pkg, err := a.config.Import(paths[0])
	if err != nil {
		a.settingsWin.Update(func() { a.settingsError = err.Error() })
		return
	}
	if err := a.applySettings(pkg.Settings); err != nil {
		a.settingsWin.Update(func() { a.settingsError = err.Error() })
		return
	}
	if a.db != nil {
		if err := a.db.ImportProfiles(context.Background(), pkg.Profiles, pkg.IncludesLearning); err == nil {
			if profiles, err := a.db.LoadProfiles(context.Background()); err == nil {
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
		}
	}
	a.settingsWin.Update(func() { a.settingsError = "" })
}

func (a *App) exportTheme() {
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Title: "导出主题", DefaultPath: "FluentLauncher-theme.zip",
		Filters: []mygo.FileFilter{{Name: "主题包", Extensions: []string{"zip"}}}})
	if err != nil || path == "" {
		return
	}
	if err := a.themes.Export(a.settingsDraft.Theme, path); err != nil {
		a.settingsWin.Update(func() { a.settingsError = err.Error() })
	}
}

func (a *App) importTheme() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "导入主题",
		Filters: []mygo.FileFilter{{Name: "主题包", Extensions: []string{"zip"}}}})
	if err != nil || len(paths) == 0 {
		return
	}
	theme, err := a.themes.Import(paths[0])
	if err != nil {
		a.settingsWin.Update(func() { a.settingsError = err.Error() })
		return
	}
	a.settingsDraft.Theme = theme
	a.settingsWin.Update(func() { a.settingsError = "" })
}

func (a *App) resetLearning() {
	if a.db == nil {
		return
	}
	if err := a.db.ResetLearning(context.Background()); err != nil {
		a.settingsError = err.Error()
		return
	}
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
	a.updateResults()
}

func (a *App) saveSettings() {
	if err := a.applySettings(a.settingsDraft); err != nil {
		a.settingsError = err.Error()
		return
	}
	a.settingsError = ""
	a.settingsWin.Hide()
}

// cancelSettings drops the draft and puts back the saved theme and panel
// size, which the preview had applied while the user was editing.
func (a *App) cancelSettings() {
	a.settingsDraft = a.settings
	a.settingsError = ""
	a.lastPreview = a.settings.Theme
	if a.settingsWin != nil {
		a.settingsWin.Hide()
	}
	a.applyThemeFrom(a.settings.Theme)
	a.applyWindowSize()
	a.positionPanel()
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
