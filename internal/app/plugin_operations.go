package app

import (
	"context"

	"github.com/egoist/mygo"

	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/locales"
)

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
	if a.official.installing {
		return
	}
	a.official.installing = true
	go func() {
		err := a.removePluginPackage(id)
		a.publishUpdate(func() { a.finishPluginUninstall(id, err) })
	}()
}

func (a *App) removePluginPackage(id string) error {
	return a.plugins.ReplacePackage(id, func() ([]protocol.Registration, error) {
		if err := a.pluginRepo.Uninstall(id); err != nil {
			return nil, err
		}
		return a.pluginRepo.Installed(), nil
	})
}

func (a *App) finishPluginUninstall(id string, err error) {
	a.official.installing = false
	a.market.feedbackID, a.market.failed = id, err != nil
	if err != nil {
		a.market.feedback = err.Error()
		return
	}
	a.settingsDraft.Plugins.EnabledIDs = toggleID(a.settingsDraft.Plugins.EnabledIDs, id, false)
	a.settingsDraft.Plugins.GlobalQueryIDs = toggleID(a.settingsDraft.Plugins.GlobalQueryIDs, id, false)
	a.plugins.Configure(a.pluginRepo.Installed(), a.settingsDraft.Plugins)
	a.market.feedback = locales.MarketUninstallDone()
	a.market.detail = ""
}

// Begin on the UI thread; dialog and package I/O stay on the worker.
func (a *App) installPlugin(directory bool) {
	if a.official.installing {
		return
	}
	op, ctx := a.beginPluginOperation("", "", "")
	op.phase = packageSelecting
	go func() {
		defer op.cancel()
		options := mygo.OpenDialogOptions{Title: locales.SettingsDialogPluginPackage(), Directory: directory}
		if !directory {
			options.Filters = []mygo.FileFilter{{Name: locales.SettingsDialogPluginPackageFilter(), Extensions: []string{"zip"}}}
		}
		paths, err := mygo.Dialog.Open(options)
		if err != nil || len(paths) == 0 {
			if err == nil {
				err = context.Canceled
			}
			a.publishUpdate(func() { a.finishPluginOperation(op, err) })
			return
		}
		check, err := a.pluginRepo.CheckUpdate(paths[0])
		if err == nil {
			err = ctx.Err()
		}
		if err != nil {
			a.publishUpdate(func() { a.finishPluginOperation(op, err) })
			return
		}
		a.publishUpdate(func() {
			op.id, op.name, op.version = check.Package.ID, check.Package.Name, check.Package.Version
			op.previouslyInstalled = check.IsUpgrade()
			op.phase = packageInstalling
		})
		op.committing.Store(true)
		err = a.plugins.ReplacePackage(check.Package.ID, func() ([]protocol.Registration, error) {
			if _, err := a.pluginRepo.Install(paths[0], check.IsUpgrade()); err != nil {
				return nil, err
			}
			return a.pluginRepo.Installed(), nil
		})
		a.publishUpdate(func() { a.finishPluginOperation(op, err) })
	}()
}

func (a *App) useMarketPlugin(itemID string) {
	for _, item := range a.marketItems() {
		if item.ID != itemID || item.Installed == nil || !item.Installed.Enabled {
			continue
		}
		m := item.Manifest()
		query := ""
		for _, command := range m.Commands {
			if command.Activation == "showQuery" {
				query = command.Query
				break
			}
		}
		if query == "" && len(m.Keywords) > 0 {
			query = m.Keywords[0] + " "
		}
		if query == "" {
			a.market.detail = itemID
			return
		}
		if a.panel != nil {
			a.show()
			a.query = query
			a.updateResults()
			a.input.Focus()
		}
		return
	}
}
