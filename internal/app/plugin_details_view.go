package app

import (
	"net/url"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/plugin"
	"fluentlauncher/locales"
)

func (a *App) pluginDetails(c *ui.Context, item plugin.MarketItem) {
	theme := c.Theme()
	compactButton(c, locales.MarketBack()).AlignSelf(ui.Start).OnClick(func() { a.market.detail = "" })
	ui.Row(c).Gap(theme.Space(4)).AlignItems(ui.Center).Children(func() {
		a.marketIcon(c, item, theme.Space(16))
		ui.Column(c).Grow(1).MinWidth(0).Gap(theme.Space(1)).Children(func() {
			ui.Text(c, item.Name).FontSize(theme.FontSize * 1.5).Bold().TextColor(theme.Text).Wrap()
			ui.Text(c, item.Author).TextColor(theme.TextMuted).Wrap()
			if item.Installed != nil {
				ui.Text(c, locales.SettingsPluginsVersion(item.ID, item.Manifest().Version)).FontSize(theme.FontSize * 0.9).TextColor(theme.TextMuted).Wrap()
			}
		})
		a.marketInstallButton(c, item)
	})
	description := item.Description
	if description == "" {
		description = item.Summary
	}
	marketTextSection(c, locales.MarketDescription(), description)
	usage := item.Usage
	if usage == "" {
		usage = locales.MarketNoUsage()
	}
	marketTextSection(c, locales.MarketUsage(), usage)
	if services := item.Services(); len(services) > 0 {
		labels := make([]string, 0, len(services))
		for _, service := range services {
			label := hostServiceLabel(service)
			if !containsFold(labels, label) {
				labels = append(labels, label)
			}
		}
		marketTextSection(c, locales.MarketServices(), strings.Join(labels, " · "))
	}
	if missing := item.MissingServices(); len(missing) > 0 {
		ui.Text(c, locales.MarketUpgradeHost()+" "+strings.Join(missing, ", ")).TextColor(theme.Warning).Wrap()
		compactButton(c, locales.UpdateTitle()).OnClick(func() { a.settingsPage = pageOther })
	}
	manifest := item.Manifest()
	if len(manifest.Keywords) > 0 {
		ui.Text(c, locales.SettingsPluginsKeywords(strings.Join(manifest.Keywords, "、"))).TextColor(theme.TextMuted).Wrap()
	}
	if item.Installed != nil {
		status := *item.Installed
		ui.Divider(c)
		ui.Text(c, locales.MarketManage()).Bold().TextColor(theme.Text)
		ui.Column(c).Gap(theme.Space(4)).Children(func() {
			stateHint := status.State().String()
			if status.State() == plugin.StateStopped {
				stateHint = locales.PluginNotRunNote()
			}
			settingRow(c, locales.MarketEnable(), stateHint, func(c *ui.Context) {
				enabled := status.Enabled
				ui.Switch(c, &enabled).Label(locales.MarketEnable() + ": " + item.Name).Disabled(a.official.installing).OnClick(func() { a.setPluginEnabled(item.ID, enabled) })
			})
			settingRow(c, locales.MarketGlobal(), locales.MarketGlobalHint(), func(c *ui.Context) {
				global := status.GlobalQuery
				ui.Switch(c, &global).Label(locales.MarketGlobal() + ": " + item.Name).Disabled(a.official.installing || !status.Enabled).OnClick(func() { a.setPluginGlobalQuery(item.ID, global) })
			})
		})
		ui.Row(c).Gap(theme.Space(2)).Children(func() {
			if len(manifest.Commands) > 0 {
				compactButton(c, locales.ShortcutsConfigure()).OnClick(func() { a.shortcutFilter = item.Name; a.settingsPage = pageShortcuts })
			}
			compactButton(c, locales.CommonUninstall()).Disabled(a.official.installing).OnClick(func() { a.uninstallPlugin(item.ID) })
		})
		a.pluginSettings(c, manifest)
		if len(manifest.Commands) > 0 {
			ui.Text(c, locales.MarketCommands()).Bold().TextColor(theme.Text)
			for _, command := range manifest.Commands {
				ui.Text(c, command.Title).TextColor(theme.TextMuted).Wrap()
			}
		}
		ui.Collapsible(c, locales.MarketDiagnostics(), &a.market.diagnostics, func() {
			ui.Column(c).Gap(theme.Space(2)).Children(func() {
				ui.Text(c, status.RuntimeText()).TextColor(theme.TextMuted).Wrap()
				ui.Text(c, item.Installed.Registration.Directory).FontSize(theme.FontSize * 0.9).TextColor(theme.TextMuted).Wrap()
				if status.Error != "" {
					ui.Text(c, status.Error).TextColor(theme.Danger).Wrap()
				}
				if status.DropNote != "" {
					ui.Text(c, status.DropNote).TextColor(theme.Warning).Wrap()
				}
				if stderr := strings.TrimSpace(status.Stderr); stderr != "" {
					ui.Text(c, locales.SettingsPluginsStderr(tailText(stderr, 1600))).TextColor(theme.TextMuted).Wrap()
				}
				if status.State() == plugin.StatePaused || status.State() == plugin.StateFailed {
					compactButton(c, locales.SettingsPluginsRestart()).Disabled(a.official.installing).OnClick(func() {
						manager := a.plugins
						go func() { manager.Restart(item.ID); a.publishUpdate(func() {}) }()
					})
				}
			})
		})
	}
	if item.Official != nil && item.Official.ReleaseNotes != "" {
		marketTextSection(c, locales.MarketNotes(), item.Official.ReleaseNotes)
	}
	if address, err := url.Parse(item.Homepage); err == nil && address.Host != "" && (address.Scheme == "https" || address.Scheme == "http") {
		compactButton(c, locales.MarketWebsite()).AlignSelf(ui.Start).OnClick(func() { go mygo.Shell.OpenExternal(item.Homepage) })
	}
}

func marketTextSection(c *ui.Context, title, text string) {
	theme := c.Theme()
	ui.Column(c).Gap(theme.Space(2)).Children(func() {
		ui.Text(c, title).Bold().TextColor(theme.Text)
		ui.Text(c, text).TextColor(theme.TextMuted).Wrap()
	})
}

func hostServiceLabel(name string) string {
	switch name {
	case "ui.notify":
		return locales.MarketServiceNotify()
	case "launcher.show", "launcher.hide":
		return locales.MarketServiceLauncher()
	case "clipboard.readText":
		return locales.MarketServiceClipboardRead()
	case "clipboard.writeText":
		return locales.MarketServiceClipboardWrite()
	case "dialogs.open", "dialogs.save":
		return locales.MarketServiceDialogs()
	case "settings.get", "settings.set":
		return locales.MarketConfig()
	case "context.get":
		return locales.MarketServiceContext()
	case "shell.open", "shell.reveal":
		return locales.MarketServiceShell()
	}
	return name
}
