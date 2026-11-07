package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/mistquay/fluentlauncher/internal/core/model"
	"github.com/mistquay/fluentlauncher/internal/core/settings"
	"github.com/mistquay/fluentlauncher/internal/plugin/protocol"
	"github.com/mistquay/fluentlauncher/internal/store"
)

var settingPages = []string{"常规", "外观", "文件", "音量", "站点搜索", "启动项", "插件", "其他"}

func (a *App) settingsView(c *ui.Context) {
	theme := *c.Theme()
	a.applyUITheme(&theme)
	c.SetTheme(&theme)
	a.drawBackground(c, &theme, false)
	a.settingsPage = min(max(a.settingsPage, 0), len(settingPages)-1)

	ui.Row(c).Fill().Children(func() {
		ui.Column(c).Width(180).FillHeight().Padding(12).Gap(4).Background(theme.Surface).Children(func() {
			ui.Text(c, "FluentLauncher").FontSize(16).Bold().Padding(8, 4)
			for i, name := range settingPages {
				background := ui.Transparent
				text := theme.Text
				if i == a.settingsPage {
					background = theme.Accent.Alpha(0.85)
					text = theme.AccentText
				}
				button := ui.Button(c, name).Background(background).TextColor(text).Width(150).Radius(6)
				if button.Clicked() {
					a.settingsPage = i
				}
			}
		})
		ui.Column(c).Grow(1).FillHeight().Padding(20).Gap(12).Children(func() {
			ui.Text(c, settingPages[a.settingsPage]).FontSize(20).Bold()
			ui.Divider(c)
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
			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "保存").Clicked() {
					a.saveSettings()
				}
				if ui.Button(c, "取消").Clicked() {
					a.settingsDraft = a.settings
					a.settingsWin.Hide()
				}
			})
			if a.settingsError != "" {
				ui.Text(c, a.settingsError).FontSize(12).TextColor(theme.Danger)
			}
		})
	})
}

func (a *App) generalPage(c *ui.Context) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "热键").Width(100)
		hotkey := a.settingsDraft.Hotkey
		ui.Select(c, &hotkey, []string{"Alt+Space", "Ctrl+Alt+Space", "Ctrl+Space"}).Width(220)
		a.settingsDraft.Hotkey = hotkey
	})
	if a.hotkeyActive != "" && a.hotkeyActive != a.settingsDraft.Hotkey {
		ui.Text(c, "当前生效："+a.hotkeyActive+"（所选热键被占用或未保存）").FontSize(11).TextColor(c.Theme().TextMuted)
	}
	width := float64(a.settingsDraft.Width)
	if ui.NumberInput(c, &width, 480, 1600, 10).Changed() {
		a.settingsDraft.Width = int(width)
	}
	height := float64(a.settingsDraft.Height)
	if ui.NumberInput(c, &height, 320, 1200, 10).Changed() {
		a.settingsDraft.Height = int(height)
	}
	maxResults := float64(a.settingsDraft.MaxResults)
	if ui.NumberInput(c, &maxResults, 5, 200, 5).Changed() {
		a.settingsDraft.MaxResults = int(maxResults)
	}
	ui.Switch(c, &a.settingsDraft.ShowRecentEntries).Label("空搜索时显示最近打开")
	ui.Switch(c, &a.settingsDraft.RunAtLogin).Label("开机启动")
	ui.Switch(c, &a.settingsDraft.HideOnStartup).Label("启动时隐藏搜索框")
	placement := string(a.settingsDraft.Placement)
	ui.Select(c, &placement, []string{string(settings.PlacementActiveScreen), string(settings.PlacementMouseScreen), string(settings.PlacementPrimaryScreen)}).Width(220)
	a.settingsDraft.Placement = settings.WindowPlacement(placement)
	ui.Text(c, "↑↓ 选择 · ↵ 打开 · Ctrl+K 动作 · Esc 清空/隐藏").FontSize(11).TextColor(c.Theme().TextMuted)
}

func (a *App) appearancePage(c *ui.Context) {
	mode := string(a.settingsDraft.Theme.Mode)
	ui.Row(c).Gap(12).Children(func() {
		ui.Text(c, "模式").Width(100)
		ui.Select(c, &mode, []string{"System", "Light", "Dark"}).Width(180)
	})
	a.settingsDraft.Theme.Mode = settings.ThemeMode(mode)
	background := string(a.settingsDraft.Theme.Background)
	ui.Row(c).Gap(12).Children(func() {
		ui.Text(c, "背景").Width(100)
		ui.Select(c, &background, []string{"Acrylic", "Mica", "Solid", "Gradient", "Image"}).Width(180)
	})
	a.settingsDraft.Theme.Background = settings.BackgroundMode(background)
	ui.Row(c).Gap(12).Children(func() {
		ui.Text(c, "背景色").Width(100)
		ui.TextInput(c, &a.settingsDraft.Theme.BackgroundColor).Placeholder("#RRGGBB").Width(200)
		ui.Text(c, "强调色").Width(80)
		ui.TextInput(c, &a.settingsDraft.Theme.AccentColor).Placeholder("#RRGGBB").Width(200)
	})
	ui.Row(c).Gap(12).Children(func() {
		ui.Text(c, "文字色").Width(100)
		ui.TextInput(c, &a.settingsDraft.Theme.ForegroundColor).Placeholder("跟随系统").Width(200)
		ui.Text(c, "渐变终点").Width(80)
		ui.TextInput(c, &a.settingsDraft.Theme.GradientColor).Placeholder("#3366AA").Width(200)
	})
	fontSize := a.settingsDraft.Theme.FontSize
	if ui.NumberInput(c, &fontSize, 12, 32, 1).Changed() {
		a.settingsDraft.Theme.FontSize = fontSize
	}
	rowPadding := a.settingsDraft.Theme.RowPadding
	if ui.NumberInput(c, &rowPadding, 2, 24, 1).Changed() {
		a.settingsDraft.Theme.RowPadding = rowPadding
	}
	corner := a.settingsDraft.Theme.CornerRadius
	if ui.NumberInput(c, &corner, 0, 32, 1).Changed() {
		a.settingsDraft.Theme.CornerRadius = corner
	}
	ui.Row(c).Gap(12).Children(func() {
		ui.Text(c, "字体").Width(100)
		ui.TextInput(c, &a.settingsDraft.Theme.FontFamily).Width(240)
	})
	ui.Row(c).Gap(12).Children(func() {
		ui.Text(c, "图片").Width(100)
		ui.TextInput(c, &a.settingsDraft.Theme.ImagePath).Grow(1)
		if ui.Button(c, "选择…").Clicked() {
			go func() {
				path, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Title: "选择背景图片"})
				if err == nil && len(path) > 0 {
					a.settingsWin.Update(func() { a.settingsDraft.Theme.ImagePath = path[0] })
				}
			}()
		}
	})
	ui.Switch(c, &a.settingsDraft.Theme.ImageCover).Label("图片铺满（Cover）")
	opacity := a.settingsDraft.Theme.ImageMaskOpacity
	if ui.NumberInput(c, &opacity, 0, 0.9, 0.05).Changed() {
		a.settingsDraft.Theme.ImageMaskOpacity = opacity
	}
	blur := float64(a.settingsDraft.Theme.ImageBlur)
	if ui.NumberInput(c, &blur, 0, 24, 1).Changed() {
		a.settingsDraft.Theme.ImageBlur = int(blur)
	}
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, "导出主题…").Clicked() {
			go a.exportTheme()
		}
		if ui.Button(c, "导入主题…").Clicked() {
			go a.importTheme()
		}
	})
}

func (a *App) filesPage(c *ui.Context) {
	ui.Text(c, "目录（每行一个，本地绝对路径）").FontSize(12).TextColor(c.Theme().TextMuted)
	directories := strings.Join(a.settingsDraft.Files.Directories, "\n")
	if ui.TextArea(c, &directories).Lines(4, 8).Height(120).Changed() {
		a.settingsDraft.Files.Directories = splitLines(directories)
	}
	excluded := strings.Join(a.settingsDraft.Files.ExcludedDirectories, "\n")
	if ui.TextArea(c, &excluded).Lines(2, 6).Height(80).Changed() {
		a.settingsDraft.Files.ExcludedDirectories = splitLines(excluded)
	}
	maxEntries := float64(a.settingsDraft.Files.MaxEntries)
	if ui.NumberInput(c, &maxEntries, 1000, 500000, 1000).Changed() {
		a.settingsDraft.Files.MaxEntries = int(maxEntries)
	}
	ui.Switch(c, &a.settingsDraft.Files.IncludeDirectories).Label("索引目录")
	ui.Switch(c, &a.settingsDraft.Files.EnableEverything).Label("启用 Everything 连接器")
	if ui.Button(c, "立即重建索引").Clicked() && a.fileIndex != nil {
		go func() {
			if err := a.fileIndex.Rebuild(context.Background()); err != nil {
				a.settingsWin.Update(func() { a.settingsError = err.Error() })
			}
		}()
	}
	if a.fileIndex != nil && a.fileIndex.Warning() != "" {
		ui.Text(c, a.fileIndex.Warning()).FontSize(11).TextColor(c.Theme().Danger)
	}
}

func (a *App) volumePage(c *ui.Context) {
	ui.Switch(c, &a.settingsDraft.VolumeWheel.Enabled).Label("屏幕边缘滚轮调节音量")
	edges := a.settingsDraft.VolumeWheel.Edges
	top, bottom := edges&settings.EdgeTop != 0, edges&settings.EdgeBottom != 0
	left, right := edges&settings.EdgeLeft != 0, edges&settings.EdgeRight != 0
	ui.Row(c).Gap(16).Children(func() {
		ui.Checkbox(c, &top, "上边缘")
		ui.Checkbox(c, &bottom, "下边缘")
		ui.Checkbox(c, &left, "左边缘")
		ui.Checkbox(c, &right, "右边缘")
	})
	a.settingsDraft.VolumeWheel.Edges = edgeMask(top, bottom, left, right)
	thickness := float64(a.settingsDraft.VolumeWheel.ThicknessDip)
	if ui.NumberInput(c, &thickness, 1, 32, 1).Changed() {
		a.settingsDraft.VolumeWheel.ThicknessDip = int(thickness)
	}
	step := float64(a.settingsDraft.VolumeWheel.StepPercent)
	if ui.NumberInput(c, &step, 1, 10, 1).Changed() {
		a.settingsDraft.VolumeWheel.StepPercent = int(step)
	}
	ui.Text(c, "只在外侧边缘触发；与相邻显示器相接的边不触发。").FontSize(11).TextColor(c.Theme().TextMuted)
}

func (a *App) sitesPage(c *ui.Context) {
	for i := range a.settingsDraft.WebSites {
		site := &a.settingsDraft.WebSites[i]
		ui.Row(c).Gap(8).Children(func() {
			ui.TextInput(c, &site.Keyword).Placeholder("关键词").Width(110)
			ui.TextInput(c, &site.Name).Placeholder("名称").Width(140)
			ui.TextInput(c, &site.URLTemplate).Placeholder("https://…/{query}").Grow(1)
			ui.Switch(c, &site.Enabled)
			if ui.Button(c, "删除").Clicked() {
				a.settingsDraft.WebSites = append(a.settingsDraft.WebSites[:i], a.settingsDraft.WebSites[i+1:]...)
				return
			}
		})
	}
	if ui.Button(c, "添加站点").Clicked() {
		a.settingsDraft.WebSites = append(a.settingsDraft.WebSites, settings.WebSearchSite{Enabled: true})
	}
}

func (a *App) launchItemsPage(c *ui.Context) {
	for i := range a.settingsDraft.CustomItems {
		item := &a.settingsDraft.CustomItems[i]
		ui.Row(c).Gap(8).Children(func() {
			ui.TextInput(c, &item.Name).Placeholder("名称").Width(140)
			ui.TextInput(c, &item.Target).Placeholder("目标路径或网址").Grow(1)
			ui.TextInput(c, &item.Arguments).Placeholder("参数").Width(120)
			ui.Switch(c, &item.Enabled)
			if ui.Button(c, "删除").Clicked() {
				a.settingsDraft.CustomItems = append(a.settingsDraft.CustomItems[:i], a.settingsDraft.CustomItems[i+1:]...)
				return
			}
		})
	}
	if ui.Button(c, "添加启动项").Clicked() {
		a.settingsDraft.CustomItems = append(a.settingsDraft.CustomItems, settings.CustomLaunchItem{
			ID: settings.NewCustomItemID(), Enabled: true,
		})
	}
	ui.Divider(c)
	ui.Text(c, "工作区（按顺序打开启动项）").FontSize(14).Bold()
	for i := range a.settingsDraft.Workspaces {
		workspace := &a.settingsDraft.Workspaces[i]
		items := strings.Join(workspace.ItemIDs, ",")
		ui.Row(c).Gap(8).Children(func() {
			ui.TextInput(c, &workspace.Name).Placeholder("名称").Width(140)
			ui.TextInput(c, &workspace.Aliases).Placeholder("别名").Width(100)
			if ui.TextInput(c, &items).Placeholder("启动项 ID，逗号分隔").Changed() {
				workspace.ItemIDs = splitComma(items)
			}
			ui.Switch(c, &workspace.Enabled)
			if ui.Button(c, "删除").Clicked() {
				a.settingsDraft.Workspaces = append(a.settingsDraft.Workspaces[:i], a.settingsDraft.Workspaces[i+1:]...)
				return
			}
		})
	}
	if ui.Button(c, "添加工作区").Clicked() {
		a.settingsDraft.Workspaces = append(a.settingsDraft.Workspaces, settings.Workspace{ID: settings.NewWorkspaceID(), Enabled: true})
	}
}

func (a *App) pluginsPage(c *ui.Context) {
	if a.pluginRepo == nil || a.plugins == nil {
		ui.Text(c, "插件运行时未初始化。").FontSize(12).TextColor(c.Theme().TextMuted)
		return
	}
	installed := a.pluginRepo.Installed()
	if len(installed) == 0 {
		ui.Text(c, "尚未安装插件。").FontSize(12).TextColor(c.Theme().TextMuted)
	}
	for _, registration := range installed {
		manifest := registration.Manifest
		enabled := containsFold(a.settingsDraft.Plugins.EnabledIDs, manifest.ID)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Switch(c, &enabled).Label("启用")
			ui.Text(c, manifest.Name).Width(140)
			ui.Text(c, manifest.Version).FontSize(11).TextColor(c.Theme().TextMuted).Width(70)
			ui.Text(c, strings.Join(manifest.Keywords, ", ")).FontSize(11).TextColor(c.Theme().TextMuted).Grow(1)
			if ui.Button(c, "卸载").Clicked() {
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
	for _, example := range []struct{ id, name string }{{"official.echo", "回显示例"}, {"official.devtools", "开发工具"}} {
		if !pluginInstalled(installed, example.id) && ui.Button(c, "安装官方 "+example.name).Clicked() {
			a.installOfficial(example.id)
		}
	}
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, "安装 ZIP…").Clicked() {
			go a.installPlugin(false)
		}
		if ui.Button(c, "安装目录…").Clicked() {
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

func (a *App) aboutPage(c *ui.Context) {
	ui.Text(c, "FluentLauncher 0.1.0").FontSize(14).Bold()
	ui.Text(c, "数据目录："+a.dataDir).FontSize(12).TextColor(c.Theme().TextMuted)
	hidden := 0
	for _, profile := range a.profileMap() {
		if profile.Hidden {
			hidden++
		}
	}
	ui.Text(c, fmt.Sprintf("隐藏条目：%d", hidden)).FontSize(12).TextColor(c.Theme().TextMuted)
	ui.Row(c).Gap(8).Children(func() {
		if ui.Button(c, "导出配置…").Clicked() {
			go a.exportConfig()
		}
		if ui.Button(c, "导入配置…").Clicked() {
			go a.importConfig()
		}
		if ui.Button(c, "重置学习记录").Clicked() {
			a.resetLearning()
		}
	})
	if ui.Button(c, "恢复隐藏条目").Clicked() {
		for id, profile := range a.profileMap() {
			if profile.Hidden {
				a.changeProfile(id, func(p model.Profile) model.Profile {
					p.Hidden = false
					return p
				})
			}
		}
	}
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
