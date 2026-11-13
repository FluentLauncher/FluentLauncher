package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
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

type packageOperation struct {
	id, name, version   string
	phase               packagePhase
	downloaded, total   int64
	previouslyInstalled bool
	cancel              context.CancelFunc
	committing          atomic.Bool
}

type packagePhase uint8

const (
	packageDownloading packagePhase = iota
	packageVerifying
	packageInstalling
	packageSelecting
)

func (phase packagePhase) String() string {
	switch phase {
	case packageVerifying:
		return locales.MarketVerifying()
	case packageInstalling:
		return locales.MarketInstalling()
	case packageSelecting:
		return locales.SettingsDialogPluginPackage()
	default:
		return locales.MarketDownloading()
	}
}

func pluginInstalled(installed []protocol.Registration, id string) bool {
	for _, item := range installed {
		if item.Manifest.ID == id {
			return true
		}
	}
	return false
}

func (a *App) beginPluginOperation(id, name, version string) (*packageOperation, context.Context) {
	if a.official.installing {
		return nil, nil
	}
	a.flushPluginSettings()
	parent := a.updates.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	op := &packageOperation{id: id, name: name, version: version, phase: packageDownloading, cancel: cancel,
		previouslyInstalled: pluginInstalled(a.pluginRepo.Installed(), id)}
	a.official.installing = true
	a.market.operation = op
	a.market.feedback, a.market.feedbackID, a.market.failed = "", "", false
	return op, ctx
}

func (a *App) finishPluginOperation(op *packageOperation, err error) {
	if a.market.operation != op {
		return
	}
	op.cancel()
	a.official.installing = false
	a.market.operation = nil
	a.market.feedbackID = op.id
	if err != nil {
		a.market.failed = !errors.Is(err, context.Canceled)
		if a.market.failed {
			a.market.feedback = locales.PluginCatalogFailed(err)
		} else {
			a.market.feedback = locales.MarketCancelled()
		}
		return
	}
	if !op.previouslyInstalled {
		a.settingsDraft.Plugins.EnabledIDs = toggleID(a.settingsDraft.Plugins.EnabledIDs, op.id, true)
	}
	// Finish includes durable configuration. Keep a disabled plugin disabled
	// across updates and do not claim activation when saving failed.
	if a.config != nil {
		if err := a.applySettings(a.settingsDraft); err != nil {
			a.market.failed = true
			a.market.feedback = locales.MarketSaveFailed() + " " + err.Error()
			a.settingsDraft.Plugins.EnabledIDs = toggleID(a.settingsDraft.Plugins.EnabledIDs, op.id, containsFold(a.settings.Plugins.EnabledIDs, op.id))
			a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
			return
		}
	}
	a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
	a.market.failed = false
	a.market.feedback = op.name + " · " + op.version + " · " + locales.MarketInstalledSuccess()
	if op.previouslyInstalled {
		a.market.feedback = op.name + " · " + op.version + " · " + locales.MarketUpdatedSuccess()
	}
	a.settingsError = ""
}

func (a *App) officialPlugins() []plugin.OfficialPlugin {
	if a.official.catalog != nil {
		return a.official.catalog.Localized()
	}
	return plugin.OfficialPlugins() // Display metadata only; no unverified downloads.
}

func (a *App) refreshOfficialCatalog(force bool) {
	if a.official.client == nil || a.official.busy {
		return
	}
	a.official.loaded, a.official.busy = true, true
	a.official.message = locales.PluginCatalogLoading()
	client := a.official.client
	parent := a.updates.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
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
	if a.official.installing || a.official.catalog == nil || entry.Version == "" || a.official.client == nil {
		return
	}
	op, ctx := a.beginPluginOperation(entry.ID, entry.Name, entry.Version)
	client := a.official.client
	go func() {
		defer op.cancel()
		err := a.downloadAndInstallOfficial(ctx, client, entry, op)
		a.publishUpdate(func() { a.finishPluginOperation(op, err) })
	}()
}

func (a *App) downloadAndInstallOfficial(ctx context.Context, client *plugin.CatalogClient, entry plugin.OfficialPlugin, op *packageOperation) error {
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
	if err := client.DownloadWithProgress(ctx, entry, archive, func(downloaded, total int64) {
		a.publishUpdate(func() {
			if a.market.operation != op {
				return
			}
			op.downloaded, op.total = downloaded, total
			if downloaded == total {
				op.phase = packageVerifying
			}
		})
	}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	op.committing.Store(true)
	a.publishUpdate(func() {
		if a.market.operation == op {
			op.phase = packageInstalling
		}
	})
	return a.plugins.ReplacePackage(entry.ID, func() ([]protocol.Registration, error) {
		if _, err := a.pluginRepo.Install(archive, true); err != nil {
			return nil, err
		}
		return a.pluginRepo.Installed(), nil
	})
}
