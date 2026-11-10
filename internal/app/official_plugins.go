package app

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"fluentlauncher/internal/plugin"
	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/locales"
)

type officialState struct {
	client     *plugin.CatalogClient
	catalog    *plugin.Catalog
	loaded     bool
	busy       bool
	installing bool
	message    string
}

func (a *App) officialPlugins() []plugin.OfficialPlugin {
	if a.official.catalog != nil {
		return a.official.catalog.Localized()
	}
	return plugin.OfficialPlugins() // Names only; no unverified download buttons.
}

func (a *App) refreshOfficialCatalog(force bool) {
	if a.official.client == nil || a.official.busy || a.official.installing {
		return
	}
	a.official.loaded, a.official.busy = true, true
	a.official.message = locales.PluginCatalogLoading()
	client := a.official.client
	ctx, cancel := context.WithTimeout(a.updates.ctx, 45*time.Second)
	go func() {
		defer cancel()
		if !client.Configured() {
			a.publishUpdate(func() { a.official.busy = false; a.official.message = locales.PluginCatalogUnconfigured() })
			return
		}
		if !force {
			if cached, err := client.Cached(); err == nil {
				a.publishUpdate(func() { a.official.catalog = cached })
			}
		}
		catalog, err := client.Refresh(ctx)
		a.publishUpdate(func() {
			a.official.busy = false
			if err != nil {
				a.official.message = locales.PluginCatalogFailed(err)
				return
			}
			a.official.catalog = catalog
			a.official.message = locales.PluginCatalogReady()
		})
	}()
}

func (a *App) installOfficialPlugin(entry plugin.OfficialPlugin) {
	if a.official.installing || a.official.catalog == nil || entry.Version == "" {
		return
	}
	a.official.installing = true
	a.official.message = locales.PluginCatalogInstalling(entry.Name)
	client := a.official.client
	ctx, cancel := context.WithTimeout(a.updates.ctx, 5*time.Minute)
	go func() {
		defer cancel()
		err := a.downloadAndInstallOfficial(ctx, client, entry)
		a.publishUpdate(func() {
			a.official.installing = false
			if err != nil {
				a.official.message = locales.PluginCatalogFailed(err)
				return
			}
			a.settingsDraft.Plugins.EnabledIDs = toggleID(a.settingsDraft.Plugins.EnabledIDs, entry.ID, true)
			a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
			a.official.message = locales.PluginCatalogInstalled(entry.Name, entry.Version)
			a.settingsError = ""
		})
	}()
}

func (a *App) downloadAndInstallOfficial(ctx context.Context, client *plugin.CatalogClient, entry plugin.OfficialPlugin) error {
	cache := filepath.Join(a.dataDir, "cache")
	if err := os.MkdirAll(cache, 0700); err != nil {
		return err
	}
	work, err := os.MkdirTemp(cache, "plugin-install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	archive := filepath.Join(work, "plugin.zip")
	if err := client.Download(ctx, entry, archive); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return a.plugins.ReplacePackage(entry.ID, func() ([]protocol.Registration, error) {
		if _, err := a.pluginRepo.Install(archive, true); err != nil {
			return nil, err
		}
		return a.pluginRepo.Installed(), nil
	})
}
