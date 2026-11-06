package app

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/mistquay/fluentlauncher/internal/core/model"
	"github.com/mistquay/fluentlauncher/internal/core/settings"
)

var searchIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>`))

func (a *App) view(c *ui.Context) {
	theme := *c.Theme()
	a.applyUITheme(&theme)
	c.SetTheme(&theme)
	a.drawBackground(c, &theme)

	if a.promptOpen {
		a.drawPrompt(c, &theme)
		return
	}
	if a.actionsOpen {
		a.drawActions(c, &theme)
		return
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

	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(16, 18).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, searchIcon).FontSize(20).TextColor(theme.TextMuted)
			in := ui.TextInputBase(c, &a.query).Bind(&a.input).Grow(1).FontSize(22).
				Placeholder("搜索应用、文件、命令").Label("搜索")
			if in.Changed() {
				a.notice = ""
				a.updateResults()
			}
			a.composing = in.Composing()
		})
		ui.Divider(c)
		a.resultsArea(c, &theme)
		ui.Divider(c)
		a.footer(c, &theme)
	})
}

func (a *App) resultsArea(c *ui.Context, theme *ui.Theme) {
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
	if index == a.chosen {
		background = theme.Accent.Alpha(0.85)
		text, muted = theme.AccentText, theme.AccentText.Alpha(0.75)
	}
	row := ui.Row(c).Padding(6, 10).Gap(12).AlignItems(ui.Center).Radius(8).Background(background)
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
		ui.Text(c, "↑↓ 选择 · ↵ 打开 · Ctrl+K 动作 · Esc 清空/隐藏").FontSize(11).TextColor(theme.TextMuted)
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
		field := ui.TextInput(c, &a.promptValue).AutoFocus().FontSize(18)
		if field.Submitted() || c.Shortcut(0, ui.KeyEnter) {
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
			for i, command := range a.actions {
				background := ui.Transparent
				text := theme.Text
				if i == a.actionIndex {
					background = theme.Accent.Alpha(0.85)
					text = theme.AccentText
				}
				row := ui.Row(c).Padding(10, 14).Radius(6).Background(background).Children(func() {
					ui.Text(c, command.Title).TextColor(text).Grow(1)
				})
				if row.Clicked() {
					a.actionIndex = i
					a.runAction()
					return
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
	value := a.settings.Theme
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

func (a *App) drawBackground(c *ui.Context, theme *ui.Theme) {
	root := c.Root()
	switch a.settings.Theme.Background {
	case settings.BackgroundImage:
		root.Background(ui.Transparent)
		if a.background != nil {
			ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).
				Opacity(float32(clampFloat(a.settings.Theme.ImageMaskOpacity, 0, 1))).
				Draw(func(p *ui.Painter, r ui.Rect) {
					if a.settings.Theme.ImageCover {
						p.Image(a.background, r, ui.Cover)
					} else {
						p.Image(a.background, r, ui.Contain)
					}
				})
		}
	case settings.BackgroundGradient:
		from, ok := parseColor(a.settings.Theme.BackgroundColor)
		if !ok {
			from = theme.Background
		}
		to, ok := parseColor(a.settings.Theme.GradientColor)
		if !ok {
			to = theme.Surface
		}
		root.Gradient(from, to, 180)
	case settings.BackgroundSolid:
		if colorValue, ok := parseColor(a.settings.Theme.BackgroundColor); ok {
			root.Background(colorValue)
		} else {
			root.Background(theme.Background)
		}
	default:
		if c.Vibrancy() {
			root.Background(ui.Transparent)
		} else {
			root.Background(theme.Background)
		}
	}
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

func trayIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	ink := color.NRGBA{R: 79, G: 124, B: 255, A: 255}
	for y := range 32 {
		for x := range 32 {
			dx, dy := float64(x)-16, float64(y)-16
			if dx*dx+dy*dy <= 13*13 {
				img.Set(x, y, ink)
			}
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		log.Printf("tray icon: %v", err)
	}
	return buffer.Bytes()
}
