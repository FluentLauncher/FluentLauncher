package app

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/locales"
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
			value := a.displayTheme()
			searchRow := ui.Row(c).Height(float32(clampFloat(value.SearchHeight, 36, 120))).
				Padding(0, 18).Gap(12).AlignItems(ui.Center)
			searchRow.Children(func() {
				ui.Icon(c, searchIcon).FontSize(20).TextColor(theme.TextMuted)
				in := ui.TextInputBase(c, &a.query).Bind(&a.input).Grow(1).FontSize(22).
					Placeholder(locales.PanelSearchPlaceholder()).Label(locales.PanelSearchLabel())
				if in.Changed() {
					a.notice = ""
					a.updateResults()
				}
				if in.Submitted() {
					a.executeSelected(model.Open)
				}
				a.composing = in.Composing()
				a.panelGrip(c, &theme)
			})
			ui.Divider(c)
			a.resultsArea(c, &theme)
			ui.Divider(c)
			a.footer(c, &theme)
		})
	})
}

// panelGrip is the handle that drags the panel around the desktop: the
// window moves with the pointer, and the app remembers where it was left so
// that the next one opens there.
func (a *App) panelGrip(c *ui.Context, theme *ui.Theme) {
	grip := ui.Box(c).Size(16, 22).Radius(4).Label(locales.PanelGripLabel()).Tooltip(locales.PanelGripLabel()).Cursor(ui.CursorMove)
	grip.Draw(func(p *ui.Painter, r ui.Rect) {
		color := theme.TextMuted.Alpha(0.65)
		for row := 0; row < 3; row++ {
			for column := 0; column < 2; column++ {
				p.Fill(ui.Rect{
					X: r.X + r.W/2 - 3 + float32(column)*4,
					Y: r.Y + r.H/2 - 6 + float32(row)*5,
					W: 2, H: 2,
				}, color, 1)
			}
		}
	})
	if grip.Hovered() {
		grip.Background(theme.Surface.Alpha(0.6))
	}
	grip.DragWindow()
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
	// The list paints the chosen row itself, in the theme's accent and accent
	// text. A theme for its rows where the accent is a tint and the text the
	// ordinary one keeps that row translucent, like the window around it,
	// instead of a solid blue plate that ignores the theme's colors.
	rowTheme := *theme
	rowTheme.Accent = selectionTint(theme)
	rowTheme.AccentText = theme.Text
	ui.Themed(c, &rowTheme).Grow(1).Background(ui.Transparent).Children(func() {
		list := ui.List(c, &a.list, len(a.results), func(i int) { a.row(c, theme, i) }).Grow(1).Padding(6).Children(func() {
			if len(a.results) == 0 {
				message := locales.PanelEmptySearch()
				if strings.TrimSpace(a.query) == "" {
					message = locales.PanelEmptyQuery()
				}
				if a.loading.Load() {
					message = locales.PanelLoading()
				}
				ui.Text(c, message).TextColor(theme.TextMuted).Padding(16)
			}
		})
		if list.Submitted() {
			a.executeSelected(model.Open)
		}
	})
}

func (a *App) row(c *ui.Context, theme *ui.Theme, index int) {
	item := a.results[index]
	background := ui.Transparent
	text, muted := theme.Text, theme.TextMuted
	switch {
	case index == a.chosen:
		background = selectionTint(theme)
		text, muted = theme.Text, theme.TextMuted
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
			ui.Text(c, locales.PanelWorkspaceBadge()).FontSize(10).TextColor(muted)
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
	parts = append(parts, a.statusParts(a.lastBatches)...)
	if a.mem != "" {
		parts = append(parts, a.mem)
	}
	parts = append(parts, locales.PanelComposing(strconv.FormatBool(a.composing)))
	ui.Row(c).Padding(8, 16).Gap(12).Children(func() {
		ui.Text(c, strings.Join(parts, " · ")).FontSize(11).TextColor(theme.TextMuted).Grow(1).SingleLine()
		hint := locales.PanelHint()
		if a.hotkeyActive != "" {
			hint = locales.PanelHintHotkey(a.hotkeyActive)
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
			if ui.PrimaryButton(c, locales.CommonSave()).Clicked() {
				a.savePrompt()
			}
			if ui.Button(c, locales.CommonCancel()).Clicked() {
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
					background = selectionTint(theme)
					text = theme.Text
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
			ui.Text(c, locales.PanelActionsHint()).FontSize(11).TextColor(theme.TextMuted)
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
	if colorValue, ok := parseOpaqueColor(value.BackgroundColor); ok {
		t.Background = colorValue
	}
	// The fields, buttons and cards take a translucent plate of their own, so
	// that they read as part of the glass instead of opaque holes cut in it.
	// The plate stays visible at any window opacity, and follows the slider:
	// the more see-through the window, the lighter the plate.
	plate := 0.35 + 0.45*float32(clampFloat(value.Opacity, 0, 1))
	t.Surface = t.Surface.Alpha(plate)
	t.SurfaceHover = t.SurfaceHover.Alpha(min(plate+0.08, 1))
	t.SurfacePressed = t.SurfacePressed.Alpha(min(plate+0.16, 1))
	t.Border = t.Border.Alpha(min(plate+0.3, 1))
}

// drawShell paints the window's background behind its content: both go in
// the same cell of a grid, where a later sibling paints over an earlier one.
func (a *App) drawShell(c *ui.Context, theme *ui.Theme, content func()) {
	ui.Grid(c).Fill().ColumnTracks(ui.Fr(1)).RowTracks(ui.Fr(1)).Children(func() {
		a.drawBackground(c, theme)
		ui.Column(c).ColumnStart(1).RowStart(1).Fill().Children(content)
	})
}

// drawBackground paints the window's background behind the content: the
// image, the mask over it, and the plate of color or material, in that
// order. All three layers are built every time, even empty, so that changing
// a theme value never changes the view tree under the scroll: a child count
// that shifts would give the content a new identity and jump the scroll back
// to the top.
func (a *App) drawBackground(c *ui.Context, theme *ui.Theme) {
	value := a.displayTheme()
	opacity := float32(clampFloat(value.Opacity, 0, 1))
	// The window has per-pixel alpha: whatever the view leaves unpainted
	// shows what is behind the window (the desktop, blurred for acrylic and
	// mica), which is what makes the translucency real instead of a wash
	// towards a background color.
	c.Root().Background(ui.Transparent)
	tint := theme.Background
	if parsed, ok := parseOpaqueColor(value.BackgroundColor); ok {
		tint = parsed
	}
	image := value.Background == settings.BackgroundImage
	layer := func() ui.Element {
		return ui.Box(c).ColumnStart(1).RowStart(1).Fill().IgnoreInput(true)
	}

	// 1. The image, at the window's opacity: the picture itself shows what is
	// behind the window, beyond the mask over it.
	imageLayer := layer()
	if image && a.background != nil {
		if value.ImageBlur > 0 {
			imageLayer.Blur(float32(value.ImageBlur))
		}
		imageLayer.Opacity(opacity)
		imageLayer.Draw(func(p *ui.Painter, r ui.Rect) {
			if value.ImageCover {
				p.Image(a.background, r, ui.Cover)
			} else {
				p.Image(a.background, r, ui.Contain)
			}
		})
	}

	// 2. The mask over the image: the more opaque it is, the paler the image,
	// and at zero the image shows as it is.
	maskLayer := layer()
	if image {
		maskLayer.Background(theme.Background.Alpha(float32(clampFloat(value.ImageMaskOpacity, 0, 1))))
	}

	// 3. The plate of color: the solid, the gradient, or the tint of a
	// material over the blur the window turns on. One opacity slider fades
	// the background of every mode; a color's own alpha is ignored, so it
	// cannot make the window translucent twice.
	plate := layer()
	plate.Draw(func(p *ui.Painter, r ui.Rect) {
		switch value.Background {
		case settings.BackgroundImage:
		case settings.BackgroundGradient:
			to := theme.Surface
			if parsed, ok := parseOpaqueColor(value.GradientColor); ok {
				to = parsed
			}
			p.FillGradient(r, ui.LinearGradient{From: withAlpha(tint, opacity), To: withAlpha(to, opacity), Angle: 180}, 0)
		case settings.BackgroundSolid:
			p.Fill(r, withAlpha(tint, opacity), 0)
		default:
			// Acrylic and mica blur what is behind the window, which the
			// glass package turns on; the plate tints it. Mica takes the
			// wallpaper's color, as Windows' mica does; acrylic stays with
			// the theme's own background.
			plateTint := tint
			if value.Background == settings.BackgroundMica {
				plateTint = a.micaTint(theme)
			}
			p.Fill(r, withAlpha(plateTint, opacity), 0)
		}
	})
}

// micaTint is the plate color over a blurred background in the mica
// material: the wallpaper's average color, toned towards the theme's own
// background so that text keeps its contrast.
func (a *App) micaTint(theme *ui.Theme) ui.Color {
	if a.wallpaperTint != (ui.Color{}) {
		return a.wallpaperTint.Mix(theme.Background, 0.25)
	}
	return theme.Background.Mix(theme.Accent, 0.12)
}

// withAlpha scales a color's alpha by factor, for the theme's opacity.
func withAlpha(col ui.Color, factor float32) ui.Color {
	return col.Alpha(float32(col.A) / 255 * factor)
}

// maxCachedIcons bounds the result-icon cache; past it the map is dropped
// and repopulated from the visible rows.
const maxCachedIcons = 512

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
	// The cache only grows while results keep naming new paths; drop it all
	// past the cap and let the visible rows repopulate it asynchronously.
	if len(a.icons) >= maxCachedIcons {
		a.icons = map[string]*ui.Bitmap{}
	}
	if !a.iconLoading[path] {
		a.iconLoading[path] = true
		go a.loadIcon(path)
	}
	return nil
}

func (a *App) loadIcon(path string) {
	pixels := a.iconPixels
	if pixels <= 0 {
		pixels = 28
	}
	data, err := mygo.Shell.FileIcon(path, pixels)
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

// selectionTint is the plate of the item that is selected, in the results, the
// action panel, the settings sidebar and the dropdowns: a tint of the theme's
// accent rather than a solid fill, so that the window's own transparency shows
// through it. The text keeps the theme's color, so a light accent stays
// readable and the selection follows light and dark appearances.
func selectionTint(theme *ui.Theme) ui.Color {
	tint := theme.Accent
	if tint.A > 0x6B {
		tint.A = 0x6B
	}
	return tint
}

func parseColor(value string) (ui.Color, bool) {
	argb, ok := settings.TryParseColor(value)
	if !ok {
		return ui.Color{}, false
	}
	return ui.RGBA(uint8(argb>>16), uint8(argb>>8), uint8(argb), float32(argb>>24)/255), true
}

// parseOpaqueColor parses a theme color but ignores its alpha: the opacity
// slider is the only control over how transparent a background is, so a
// color with alpha of its own cannot make it translucent twice.
func parseOpaqueColor(value string) (ui.Color, bool) {
	color, ok := parseColor(value)
	if ok {
		color.A = 255
	}
	return color, ok
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
