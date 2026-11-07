package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
)

var searchIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>`))

func (a *App) view(c *ui.Context) {
	theme := *c.Theme()
	a.applyUITheme(&theme)
	c.SetTheme(&theme)

	if a.promptOpen {
		a.drawShell(c, &theme, func() { a.drawPrompt(c, &theme) })
		return
	}
	if a.actionsOpen {
		a.drawShell(c, &theme, func() { a.drawActions(c, &theme) })
		return
	}
	if a.pendingOpen {
		a.pendingOpen = false
		a.executeSelected(model.Open)
	}

	if c.Shortcut(0, ui.KeyEscape) {
		if a.query != "" {
			a.query = ""
			a.notice = ""
			a.updateResults()
		} else {
			a.hidePanel()
		}
	}
	if c.Shortcut(0, ui.KeyEnter) {
		a.executeSelected(model.Open)
	}
	if c.Shortcut(0, ui.KeyDown) {
		a.choose(1)
	}
	if c.Shortcut(0, ui.KeyUp) {
		a.choose(-1)
	}
	if c.Shortcut(ui.Ctrl, ui.KeyK) {
		a.openActions()
	}

	a.drawShell(c, &theme, func() {
		ui.Column(c).Fill().Children(func() {
			ui.Row(c).Padding(16, 18).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, searchIcon).FontSize(20).TextColor(theme.TextMuted)
				in := ui.TextInputBase(c, &a.query).Bind(&a.input).Grow(1).FontSize(22).
					Placeholder("搜索应用、文件、命令").Label("搜索")
				if in.Changed() {
					a.notice = ""
					a.updateResults()
				}
				if in.Submitted() {
					a.executeSelected(model.Open)
				}
				a.composing = in.Composing()
			})
			ui.Divider(c)
			a.resultsArea(c, &theme)
			ui.Divider(c)
			a.footer(c, &theme)
		})
	})
}

func (a *App) resultsArea(c *ui.Context, theme *ui.Theme) {
	a.listHover = a.listHoverNext
	a.listHoverNext = -1
	if a.hotkeyNotice != "" {
		ui.Row(c).Padding(12, 16).Children(func() {
			ui.Text(c, a.hotkeyNotice).FontSize(12).TextColor(theme.Warning)
		})
		ui.Divider(c)
	}
	if a.notice != "" {
		ui.Row(c).Padding(12, 16).Children(func() {
			ui.Text(c, a.notice).FontSize(12).TextColor(theme.Danger)
		})
		ui.Divider(c)
	}
	a.list.Selected = &a.chosen
	a.list.Key = func(i int) any { return a.results[i].Entry.ID }
	a.list.Label = func(i int) string { return a.results[i].Entry.Title }
	list := ui.List(c, &a.list, len(a.results), func(i int) { a.row(c, theme, i) }).Grow(1).Padding(6).Children(func() {
		if len(a.results) == 0 {
			message := "输入以搜索"
			if strings.TrimSpace(a.query) == "" {
				message = "输入以搜索应用、文件或命令"
			}
			if a.loading {
				message = "正在读取应用…"
			}
			ui.Text(c, message).TextColor(theme.TextMuted).Padding(16)
		}
	})
	if list.Submitted() {
		a.executeSelected(model.Open)
	}
}

func (a *App) row(c *ui.Context, theme *ui.Theme, index int) {
	item := a.results[index]
	background := ui.Transparent
	text, muted := theme.Text, theme.TextMuted
	switch {
	case index == a.chosen:
		background = theme.Accent.Alpha(0.85)
		text, muted = theme.AccentText, theme.AccentText.Alpha(0.75)
	case index == a.listHover:
		background = theme.Surface.Alpha(0.8)
	}
	vertical := float32(a.settings.Theme.RowPadding) / 2
	if vertical < 3 {
		vertical = 3
	}
	row := ui.Row(c).Padding(vertical, 10).Gap(12).AlignItems(ui.Center).Radius(8).Background(background)
	row.Children(func() {
		if bitmap := a.resultIcon(item.Entry); bitmap != nil {
			ui.Image(c, bitmap).Size(float32(a.iconSize), float32(a.iconSize))
		} else {
			ui.Avatar(c, item.Entry.Title, nil).Size(float32(a.iconSize), float32(a.iconSize))
		}
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, item.Entry.Title).TextColor(text).SingleLine()
			if item.Entry.Subtitle != "" {
				ui.Text(c, item.Entry.Subtitle).FontSize(11).TextColor(muted).SingleLine()
			}
		})
		if item.Entry.Kind == model.Workspace {
			ui.Text(c, "工作区").FontSize(10).TextColor(muted)
		}
	})
	if row.RightClicked() {
		a.chosen = index
		a.selectedID = item.Entry.ID
		a.openActions()
	}
	if row.Clicked() {
		a.chosen = index
		a.selectedID = item.Entry.ID
		a.pendingOpen = true
	}
	if row.Hovered() {
		a.listHoverNext = index
	}
}

func (a *App) footer(c *ui.Context, theme *ui.Theme) {
	parts := []string{}
	if a.status != "" {
		parts = append(parts, a.status)
	}
	if a.mem != "" {
		parts = append(parts, a.mem)
	}
	parts = append(parts, fmt.Sprintf("组合 %v", a.composing))
	ui.Row(c).Padding(8, 16).Gap(12).Children(func() {
		ui.Text(c, strings.Join(parts, " · ")).FontSize(11).TextColor(theme.TextMuted).Grow(1).SingleLine()
		hint := "↑↓ 选择 · ↵ 打开 · Ctrl+K 动作 · Esc 清空/隐藏"
		if a.hotkeyActive != "" {
			hint = a.hotkeyActive + " 呼出 · " + hint
		}
		ui.Text(c, hint).FontSize(11).TextColor(theme.TextMuted)
	})
}

func (a *App) drawPrompt(c *ui.Context, theme *ui.Theme) {
	if c.Shortcut(0, ui.KeyEscape) {
		a.promptOpen = false
		a.input.Focus()
		return
	}
	ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
		ui.Text(c, a.promptTitle).FontSize(18).Bold().TextColor(theme.Text)
		if a.promptReadOnly {
			ui.TextArea(c, &a.promptValue).ReadOnly(true).Height(260)
		} else {
			field := ui.TextInput(c, &a.promptValue).AutoFocus().FontSize(18)
			if field.Submitted() || c.Shortcut(0, ui.KeyEnter) {
				a.savePrompt()
				return
			}
		}
		if !a.promptReadOnly && c.Shortcut(0, ui.KeyEnter) {
			a.savePrompt()
			return
		}
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "保存").Clicked() {
				a.savePrompt()
			}
			if ui.Button(c, "取消").Clicked() {
				a.promptOpen = false
				a.input.Focus()
			}
		})
	})
}

func (a *App) drawActions(c *ui.Context, theme *ui.Theme) {
	if c.Shortcut(0, ui.KeyEscape) {
		a.hideActions()
		return
	}
	if c.Shortcut(0, ui.KeyDown) {
		a.actionIndex = min(a.actionIndex+1, len(a.actions)-1)
	}
	if c.Shortcut(0, ui.KeyUp) {
		a.actionIndex = max(a.actionIndex-1, 0)
	}
	if c.Shortcut(0, ui.KeyEnter) {
		a.runAction()
		return
	}
	entry, _ := a.selected()
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(16, 18).Children(func() {
			ui.Text(c, entry.Title).FontSize(18).Bold().TextColor(theme.Text)
		})
		ui.Divider(c)
		ui.Column(c).Fill().Padding(6).Bind(&a.actionsFocus).Focusable().AutoFocus().Children(func() {
			a.actionsHover = a.actionsHoverNext
			a.actionsHoverNext = -1
			for i, command := range a.actions {
				background := ui.Transparent
				text := theme.Text
				switch {
				case i == a.actionIndex:
					background = theme.Accent.Alpha(0.85)
					text = theme.AccentText
				case i == a.actionsHover:
					background = theme.Surface.Alpha(0.8)
				}
				row := ui.Row(c).Padding(10, 14).Radius(6).Background(background).Children(func() {
					ui.Text(c, command.Title).TextColor(text).Grow(1)
				})
				if row.Clicked() {
					a.actionIndex = i
					a.runAction()
					return
				}
				if row.Hovered() {
					a.actionsHoverNext = i
				}
			}
		})
		ui.Divider(c)
		ui.Row(c).Padding(8, 16).Children(func() {
			ui.Text(c, "↑↓ 选择 · ↵ 执行 · Esc 返回").FontSize(11).TextColor(theme.TextMuted)
		})
	})
}

func (a *App) applyUITheme(t *ui.Theme) {
	value := a.displayTheme()
	if value.FontFamily != "" {
		t.Font = value.FontFamily
	}
	if value.FontSize > 0 {
		t.FontSize = float32(value.FontSize)
	}
	if value.CornerRadius > 0 {
		t.Radius = float32(value.CornerRadius)
	}
	if colorValue, ok := parseColor(value.AccentColor); ok {
		t.Accent = colorValue
		t.AccentHover = colorValue
		t.AccentPressed = colorValue
	}
	if colorValue, ok := parseColor(value.ForegroundColor); ok {
		t.Text = colorValue
	}
	if colorValue, ok := parseColor(value.BackgroundColor); ok {
		t.Background = colorValue
	}
}

// drawShell paints the window's background behind its content: both go in
// the same cell of a grid, where a later sibling paints over an earlier one.
func (a *App) drawShell(c *ui.Context, theme *ui.Theme, content func()) {
	ui.Grid(c).Fill().ColumnTracks(ui.Fr(1)).RowTracks(ui.Fr(1)).Children(func() {
		a.drawBackground(c, theme)
		ui.Column(c).ColumnStart(1).RowStart(1).Fill().Children(content)
	})
}

func (a *App) drawBackground(c *ui.Context, theme *ui.Theme) {
	value := a.displayTheme()
	opacity := float32(clampFloat(value.Opacity, 0, 1))
	// The window has per-pixel alpha: whatever the view leaves unpainted
	// shows what is behind the window (the desktop, blurred for acrylic and
	// mica), which is what makes the translucency real instead of a wash
	// towards a background color.
	c.Root().Background(ui.Transparent)
	tint, ok := parseColor(value.BackgroundColor)
	if !ok {
		tint = theme.Background
	}
	// The layers go in the view's grid cell, before the content: a later
	// sibling paints over an earlier one, so the background stays behind.
	layer := func() ui.Element {
		return ui.Box(c).ColumnStart(1).RowStart(1).Fill().IgnoreInput(true)
	}
	switch value.Background {
	case settings.BackgroundImage:
		if a.background != nil {
			box := layer()
			if value.ImageBlur > 0 {
				box.Blur(float32(value.ImageBlur))
			}
			box.Draw(func(p *ui.Painter, r ui.Rect) {
				if value.ImageCover {
					p.Image(a.background, r, ui.Cover)
				} else {
					p.Image(a.background, r, ui.Contain)
				}
			})
		}
		// The mask sits over the image: the more opaque it is, the paler the
		// image, and at zero the image shows as it is.
		if mask := float32(clampFloat(value.ImageMaskOpacity, 0, 1)); mask > 0 {
			layer().Background(theme.Background.Alpha(mask))
		}
	case settings.BackgroundGradient:
		from, ok := parseColor(value.BackgroundColor)
		if !ok {
			from = theme.Background
		}
		to, ok := parseColor(value.GradientColor)
		if !ok {
			to = theme.Surface
		}
		layer().Gradient(withAlpha(from, opacity), withAlpha(to, opacity), 180)
	case settings.BackgroundSolid:
		layer().Background(withAlpha(tint, opacity))
	default:
		// Acrylic and mica blur what is behind the window, which the glass
		// package turns on; this is the plate of color over it. The opacity
		// fades that plate, so lowering it shows more of the blurred
		// background, not more of the background color.
		if value.Background == settings.BackgroundMica {
			tint = tint.Mix(theme.Accent, 0.1)
		}
		layer().Background(withAlpha(tint, opacity*materialTint))
	}
}

// materialTint is how opaque the plate of color over a blurred background is
// at full opacity: enough for text to read, thin enough to keep the blur.
const materialTint = 0.5

// withAlpha scales a color's alpha by factor, for the theme's opacity.
func withAlpha(col ui.Color, factor float32) ui.Color {
	return col.Alpha(float32(col.A) / 255 * factor)
}

func (a *App) resultIcon(entry model.Entry) *ui.Bitmap {
	path := entry.IconPath
	if path == "" {
		path = entry.LaunchTarget
	}
	if path == "" || entry.Kind == model.Calculation || entry.Kind == model.URL {
		return nil
	}
	if bitmap, ok := a.icons[path]; ok {
		return bitmap
	}
	if !a.iconLoading[path] {
		a.iconLoading[path] = true
		go a.loadIcon(path)
	}
	return nil
}

func (a *App) loadIcon(path string) {
	data, err := mygo.Shell.FileIcon(path, a.iconSize)
	var bitmap *ui.Bitmap
	if err == nil {
		bitmap, _ = ui.DecodeBitmap(data)
	}
	if a.panel != nil {
		a.panel.Update(func() {
			a.icons[path] = bitmap
			delete(a.iconLoading, path)
		})
	}
}

func parseColor(value string) (ui.Color, bool) {
	argb, ok := settings.TryParseColor(value)
	if !ok {
		return ui.Color{}, false
	}
	return ui.RGBA(uint8(argb>>16), uint8(argb>>8), uint8(argb), float32(argb>>24)/255), true
}

func clampFloat(v, low, high float64) float64 {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}
