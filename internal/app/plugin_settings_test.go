package app

import (
	"encoding/json"
	"testing"
	"time"

	"fluentlauncher/internal/plugin/protocol"
)

func TestPluginSettingsFlushKeepsLatestEditOnImmediateClose(t *testing.T) {
	a := newPluginsApp(t)
	manifest := protocol.Manifest{ID: "vendor.config", Name: "Configuration", Settings: []protocol.SettingDefinition{{ID: "text", Type: "text", Default: json.RawMessage(`""`)}}}
	a.savePluginSetting(manifest, "text", json.RawMessage(`"older"`))
	a.savePluginSetting(manifest, "text", json.RawMessage(`"latest"`))
	a.flushPluginSettings()
	values, err := a.pluginData.Get(manifest)
	if err != nil || string(values["text"]) != `"latest"` {
		t.Fatal(values, err)
	}
	if len(a.market.configWrites) != 0 {
		t.Fatal("saved edit still pending")
	}
	// Let the old debounce deadline pass: neither stopped nor already queued
	// workers may replace the value flushed during close.
	time.Sleep(450 * time.Millisecond)
	values, err = a.pluginData.Get(manifest)
	if err != nil || string(values["text"]) != `"latest"` {
		t.Fatal("obsolete edit overwrote close flush", values, err)
	}
}
