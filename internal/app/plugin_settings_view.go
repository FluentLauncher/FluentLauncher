package app

import (
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/locales"
)

type pluginSettingValue struct {
	text    string
	number  float64
	enabled bool
}

type pluginSettingWrite struct {
	manifest protocol.Manifest
	key      string
	value    json.RawMessage
	epoch    *atomic.Uint64
	timer    *time.Timer
}

func (a *App) pluginSettings(c *ui.Context, manifest protocol.Manifest) {
	if len(manifest.Settings) == 0 || a.pluginData == nil {
		return
	}
	key := manifest.ID + ":" + manifest.Version
	if a.market.configKey != key {
		a.market.configKey = key
		a.market.configValues = nil
		a.market.configLoading = true
		store := a.pluginData
		go func() {
			values, err := store.Get(manifest)
			a.publishUpdate(func() {
				if a.market.configKey != key {
					return
				}
				a.market.configLoading = false
				if err != nil {
					a.market.feedback = err.Error()
					a.market.failed = true
					return
				}
				a.market.configValues = map[string]*pluginSettingValue{}
				for _, pending := range a.market.configWrites {
					if pending.manifest.ID == manifest.ID {
						values[pending.key] = pending.value
					}
				}
				for _, setting := range manifest.Settings {
					value := &pluginSettingValue{}
					switch setting.Type {
					case "bool":
						_ = json.Unmarshal(values[setting.ID], &value.enabled)
					case "number":
						_ = json.Unmarshal(values[setting.ID], &value.number)
					default:
						_ = json.Unmarshal(values[setting.ID], &value.text)
					}
					a.market.configValues[setting.ID] = value
				}
			})
		}()
	}
	ui.Text(c, locales.MarketConfig()).Bold().TextColor(c.Theme().Text)
	if a.market.configLoading {
		ui.Text(c, locales.PluginCatalogLoading()).TextColor(c.Theme().TextMuted)
		return
	}
	for _, definition := range manifest.Settings {
		value := a.market.configValues[definition.ID]
		if value == nil {
			continue
		}
		c.Key(definition.ID)
		settingRow(c, definition.Title, definition.Description, func(c *ui.Context) {
			changed := func() {
				var next any
				switch definition.Type {
				case "bool":
					next = value.enabled
				case "number":
					next = value.number
				case "choice":
					next = value.text
				default:
					next = value.text
				}
				data, _ := json.Marshal(next)
				a.savePluginSetting(manifest, definition.ID, data)
			}
			switch definition.Type {
			case "bool":
				ui.Switch(c, &value.enabled).Label(definition.Title).OnChange(changed)
			case "choice":
				ui.Select(c, &value.text, definition.Options).Label(definition.Title).OnChange(changed)
			case "number":
				min, max := -1e9, 1e9
				if definition.Min != nil {
					min = *definition.Min
				}
				if definition.Max != nil {
					max = *definition.Max
				}
				ui.NumberInput(c, &value.number, min, max, 1).Width(150).Label(definition.Title).OnChange(changed)
			default:
				ui.TextInput(c, &value.text).Width(220).Label(definition.Title).OnChange(changed)
			}
		})
	}
}

func (a *App) savePluginSetting(manifest protocol.Manifest, key string, value json.RawMessage) {
	if a.market.configEdits == nil {
		a.market.configEdits = map[string]*atomic.Uint64{}
	}
	id := manifest.ID + ":" + key
	epoch := a.market.configEdits[id]
	if epoch == nil {
		epoch = &atomic.Uint64{}
		a.market.configEdits[id] = epoch
	}
	version := epoch.Add(1)
	if a.market.configWrites == nil {
		a.market.configWrites = map[string]*pluginSettingWrite{}
	}
	if previous := a.market.configWrites[id]; previous != nil {
		previous.timer.Stop()
	}
	job := &pluginSettingWrite{manifest: manifest, key: key, value: append(json.RawMessage(nil), value...), epoch: epoch}
	a.market.configWrites[id] = job
	store := a.pluginData
	job.timer = time.AfterFunc(400*time.Millisecond, func() {
		err := store.SetIf(manifest, key, job.value, func() bool { return epoch.Load() == version })
		if err == nil && epoch.Load() == version {
			a.plugins.SettingsChanged(manifest.ID)
		}
		if a.panel != nil {
			a.publishUpdate(func() {
				if epoch.Load() != version {
					return
				}
				if err != nil {
					a.market.feedback = manifest.Name + ": " + err.Error()
					a.market.failed = true
					return
				}
				if a.market.configWrites[id] == job {
					delete(a.market.configWrites, id)
				}
			})
		}
	})
}

// Like the main settings draft, pending plugin edits are durable before the
// settings window or application closes. The bounded writes never wait on a
// plugin; bumping the epoch prevents an older worker from overwriting them.
func (a *App) flushPluginSettings() {
	for id, job := range a.market.configWrites {
		job.timer.Stop()
		job.epoch.Add(1)
		if err := a.pluginData.Set(job.manifest, job.key, job.value); err != nil {
			a.market.feedback = job.manifest.Name + ": " + err.Error()
			a.market.failed = true
			continue
		}
		delete(a.market.configWrites, id)
		a.plugins.SettingsChanged(job.manifest.ID)
	}
}
