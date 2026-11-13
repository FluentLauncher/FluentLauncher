package app

import (
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/plugin"
	"fluentlauncher/locales"
)

type marketplaceState struct {
	view                 int
	query, detail        string
	diagnostics          bool
	operation            *packageOperation
	feedback, feedbackID string
	failed               bool
	icons                map[string]*ui.Bitmap
	iconPending          map[string]bool
	iconSlots            chan struct{}
	configKey            string
	configLoading        bool
	configValues         map[string]*pluginSettingValue
	configEdits          map[string]*atomic.Uint64
	configWrites         map[string]*pluginSettingWrite
	browseScroll         [3]ui.ScrollState
	detailScroll         ui.ScrollState
}

func (a *App) openPluginDetails(id string) {
	a.market.detail = id
	a.market.diagnostics = false
	a.market.detailScroll.X, a.market.detailScroll.Y = 0, 0
	a.market.configKey = ""
}

func (a *App) marketItems() []plugin.MarketItem {
	return plugin.Marketplace(a.officialPlugins(), a.plugins.Statuses())
}

func (a *App) pluginsPage(c *ui.Context) {
	theme := *c.Theme()
	a.applyUITheme(&theme)
	theme.FontSize = settingsFontSize
	c.SetTheme(&theme)
	if a.pluginRepo == nil || a.plugins == nil {
		ui.Text(c, locales.SettingsPluginsUninitialized()).TextColor(theme.TextMuted)
		return
	}
	items := a.marketItems()
	ui.Column(c).Gap(theme.Space(4)).FillWidth().Children(func() {
		a.marketFeedback(c, items)
		if a.market.detail != "" {
			for _, item := range items {
				if item.ID == a.market.detail {
					c.Key("detail:" + item.ID)
					ui.Column(c).Gap(theme.Space(4)).Children(func() { a.pluginDetails(c, item) })
					return
				}
			}
			a.market.detail = ""
		}
		c.Key("intro")
		ui.Text(c, locales.MarketIntro()).TextColor(theme.TextMuted)
		installed, updates := 0, 0
		for _, item := range items {
			if item.Installed != nil {
				installed++
			}
			if item.UpdateAvailable() {
				updates++
			}
		}
		c.Key("tabs")
		ui.Segmented(c, &a.market.view, locales.MarketDiscover(), fmt.Sprintf("%s %d", locales.MarketInstalled(), installed), fmt.Sprintf("%s %d", locales.MarketUpdates(), updates)).Label(locales.SettingsPagePlugins()).AlignSelf(ui.Start)
		width, _ := c.Size()
		c.Key("toolbar")
		ui.Column(c).Gap(theme.Space(2)).Children(func() {
			ui.Row(c).Gap(theme.Space(2)).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c, &a.market.query).Grow(1).MinWidth(0).Height(theme.Space(8)).Placeholder(locales.MarketSearch()).Label(locales.MarketSearch())
				if width >= 800 {
					a.marketToolbar(c)
				}
			})
			if width < 800 {
				ui.Row(c).Gap(theme.Space(2)).Children(func() { a.marketToolbar(c) })
			}
		})
		if a.official.message != "" {
			ui.Text(c, a.official.message).FontSize(theme.FontSize * 0.9).TextColor(theme.TextMuted).Wrap()
		}
		if a.official.installing {
			ui.Text(c, locales.MarketBusyHint()).FontSize(theme.FontSize * 0.9).TextColor(theme.TextMuted).Wrap()
		}
		query := strings.ToLower(strings.TrimSpace(a.market.query))
		visible := []plugin.MarketItem{}
		for _, item := range items {
			if a.market.view == 1 && item.Installed == nil || a.market.view == 2 && !item.UpdateAvailable() {
				continue
			}
			if query != "" && !strings.Contains(strings.ToLower(item.Name+" "+item.Summary+" "+item.ID+" "+item.Author), query) {
				continue
			}
			visible = append(visible, item)
		}
		columns := 1
		if width >= 1060 {
			columns = 2
		}
		c.Key("plugins")
		ui.Grid(c).Columns(columns).Gap(theme.Space(3)).FillWidth().Children(func() {
			for _, item := range visible {
				c.Key(item.ID)
				a.marketPluginCard(c, item)
			}
		})
		if len(visible) == 0 {
			message := locales.SettingsPluginsEmptyTitle()
			if query != "" {
				message = locales.MarketNoMatch()
			} else if a.market.view == 2 {
				message = locales.MarketNoUpdates()
			}
			ui.Text(c, message).Padding(theme.Space(6), 0).TextColor(theme.TextMuted).Wrap()
		}
		if a.market.view == 1 {
			a.pluginProblems(c)
		}
	})
}

func (a *App) marketToolbar(c *ui.Context) {
	if compactButton(c, locales.MarketRefresh()).Disabled(a.official.busy).Tooltip(locales.PluginCatalogRefresh()).Clicked() {
		a.refreshOfficialCatalog(true)
	}
	compactButton(c, locales.MarketLocalInstall()).Disabled(a.official.installing).PopoverMenuButton(func(m *ui.Menu) {
		if m.Item(locales.SettingsPluginsInstallZip()).Chosen() {
			a.installPlugin(false)
		}
		if m.Item(locales.SettingsPluginsInstallDir()).Chosen() {
			a.installPlugin(true)
		}
		m.Separator()
		if m.Item(locales.CmdOpenFileLocation()).Chosen() {
			directory := a.pluginRepo.Dir()
			go mygo.Shell.OpenPath(directory)
		}
	})
}

func (a *App) marketPluginCard(c *ui.Context, item plugin.MarketItem) {
	theme := c.Theme()
	ui.Column(c).Gap(theme.Space(3)).Padding(theme.Space(4)).MinWidth(0).Radius(theme.Radius*1.5).
		Background(theme.Surface).Border(1, theme.Border).Children(func() {
		ui.Row(c).Gap(theme.Space(3)).AlignItems(ui.Center).Children(func() {
			a.marketIcon(c, item, theme.Space(12))
			ui.Column(c).Grow(1).MinWidth(0).Gap(theme.Space(0.5)).Children(func() {
				button := ui.ButtonBase(c).Justify(ui.Start).Label(item.Name).OnClick(func() { a.openPluginDetails(item.ID) })
				button.Children(func() { ui.Text(c, item.Name).Bold().TextColor(theme.Text).SingleLine().Ellipsis("…") })
				author := item.Author
				if author == "" {
					if item.Official != nil {
						author = locales.MarketOfficial()
					} else {
						author = locales.MarketLocal()
					}
				}
				ui.Text(c, author).FontSize(theme.FontSize * 0.9).TextColor(theme.TextMuted).SingleLine()
			})
		})
		ui.Text(c, item.Summary).TextColor(theme.TextMuted).Wrap().MaxHeight(theme.Space(15)).MinHeight(theme.Space(10)).Tooltip(item.Summary)
		version := ""
		if item.Official != nil {
			version = item.Official.Version
		}
		if item.Installed != nil {
			version = locales.MarketInstalled() + " · " + item.Manifest().Version
			if item.UpdateAvailable() {
				version = item.Manifest().Version + " → " + item.Official.Version
			}
		}
		ui.Row(c).Gap(theme.Space(2)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, version).FontSize(theme.FontSize * 0.85).TextColor(theme.TextMuted).Grow(1).SingleLine().Ellipsis("…")
			label := locales.MarketDetails()
			if item.Installed != nil {
				label = locales.MarketManage()
			}
			compactButton(c, label).Label(label + ": " + item.Name).OnClick(func() { a.openPluginDetails(item.ID) })
			a.marketInstallButton(c, item)
		})
		if a.market.feedbackID == item.ID && a.market.failed {
			ui.Text(c, a.market.feedback).TextColor(theme.Danger).Wrap()
		}
	})
}

func (a *App) marketInstallButton(c *ui.Context, item plugin.MarketItem) {
	if op := a.market.operation; op != nil && op.id == item.ID {
		label := op.phase.String()
		if op.total > 0 && op.phase == packageDownloading {
			label += fmt.Sprintf(" %d%%", op.downloaded*100/op.total)
		}
		compactButton(c, label).Disabled(true)
		return
	}
	if item.Installed == nil || item.UpdateAvailable() {
		label := locales.CommonInstall()
		if item.UpdateAvailable() {
			label = locales.PluginCatalogUpgrade()
		}
		if a.market.failed && a.market.feedbackID == item.ID {
			label = locales.MarketRetry()
		}
		canInstall := item.Official != nil && item.Official.Version != "" && a.official.catalog != nil && a.official.client != nil && len(item.MissingServices()) == 0
		if compactPrimaryButton(c, label).Disabled(a.official.installing || !canInstall).Clicked() {
			a.installOfficialPlugin(*item.Official)
		}
		return
	}
	compactButton(c, locales.MarketUse()).Disabled(!item.Installed.Enabled).OnClick(func() { a.useMarketPlugin(item.ID) })
}

func (a *App) marketFeedback(c *ui.Context, items []plugin.MarketItem) {
	theme := c.Theme()
	if op := a.market.operation; op != nil {
		ui.Row(c).Gap(theme.Space(3)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, strings.TrimSpace(op.name+" · "+op.phase.String())).Grow(1).TextColor(theme.Text).Wrap()
			if compactButton(c, locales.CommonCancel()).Disabled(op.committing.Load()).Clicked() {
				if !op.committing.Load() {
					op.cancel()
				}
			}
		})
		if op.total > 0 && op.phase == packageDownloading {
			ui.Progress(c, float64(op.downloaded)/float64(op.total)).Label(op.phase.String())
		}
	}
	if a.market.feedback == "" {
		return
	}
	color := theme.Success
	if a.market.failed {
		color = theme.Danger
	}
	ui.Column(c).Gap(theme.Space(2)).Padding(theme.Space(3)).Radius(theme.Radius).Background(color.Alpha(0.12)).Children(func() {
		ui.Row(c).Gap(theme.Space(2)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.market.feedback).Grow(1).TextColor(theme.Text).Wrap()
			compactButton(c, locales.MarketNoticeClose()).OnClick(func() { a.market.feedback = "" })
		})
		for _, item := range items {
			if item.ID == a.market.feedbackID && item.Installed != nil && !a.market.failed {
				ui.Row(c).Gap(theme.Space(2)).Children(func() {
					compactButton(c, locales.MarketUse()).Disabled(!item.Installed.Enabled).OnClick(func() { a.useMarketPlugin(item.ID) })
					if len(item.Manifest().Commands) > 0 {
						compactButton(c, locales.ShortcutsConfigure()).OnClick(func() { a.shortcutFilter = item.Name; a.settingsPage = pageShortcuts })
					}
				})
			}
		}
	})
}

func (a *App) pluginProblems(c *ui.Context) {
	problems := a.pluginRepo.Problems()
	if len(problems) == 0 {
		return
	}
	ui.Collapsible(c, locales.SettingsPluginsProblemsCard(), &a.market.diagnostics, func() {
		for _, problem := range problems {
			ui.Text(c, problem.Directory+": "+problem.Message).TextColor(c.Theme().Danger).Wrap()
		}
	})
}
