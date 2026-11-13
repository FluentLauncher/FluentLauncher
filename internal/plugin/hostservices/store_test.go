package hostservices

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"fluentlauncher/internal/plugin/protocol"
)

func TestPluginSettingsAreTypedPersistentAndIsolated(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	m := protocol.Manifest{ID: "vendor.first", Settings: []protocol.SettingDefinition{{ID: "enabled", Type: "bool", Default: json.RawMessage(`true`)}, {ID: "name", Type: "text", Default: json.RawMessage(`"default"`)}}}
	values, err := store.Get(m)
	if err != nil || string(values["enabled"]) != "true" {
		t.Fatal(values, err)
	}
	if err := store.Set(m, "enabled", json.RawMessage(`false`)); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(m, "name", json.RawMessage(`"changed"`)); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []json.RawMessage{json.RawMessage(`"not a bool"`), json.RawMessage(` null `)} {
		if store.Set(m, "enabled", invalid) == nil {
			t.Fatal("invalid type accepted")
		}
	}
	if store.Set(m, "unknown", json.RawMessage(`true`)) == nil {
		t.Fatal("undeclared key accepted")
	}
	fresh := NewStore(root)
	values, err = fresh.Get(m)
	if err != nil || string(values["enabled"]) != "false" || string(values["name"]) != `"changed"` {
		t.Fatal(values, err)
	}
	m.ID = "vendor.second"
	values, err = fresh.Get(m)
	if err != nil || string(values["name"]) != `"default"` {
		t.Fatal("other plugin settings leaked", values, err)
	}
	if _, _, err := store.Directories("../outside"); err == nil {
		t.Fatal("directory traversal accepted")
	}
	data, cache, err := store.Directories("vendor.first")
	if err != nil || data != filepath.Join(root, "plugin-data", "vendor.first") || cache != filepath.Join(root, "cache", "plugins", "vendor.first") {
		t.Fatal(data, cache, err)
	}
}

func TestObsoleteSettingWriteIsDiscarded(t *testing.T) {
	store := NewStore(t.TempDir())
	m := protocol.Manifest{ID: "vendor.example", Settings: []protocol.SettingDefinition{{ID: "text", Type: "text", Default: json.RawMessage(`""`)}}}
	if err := store.Set(m, "text", json.RawMessage(`"latest"`)); err != nil {
		t.Fatal(err)
	}
	if err := store.SetIf(m, "text", json.RawMessage(`"obsolete"`), func() bool { return false }); err != nil {
		t.Fatal(err)
	}
	values, err := store.Get(m)
	if err != nil || string(values["text"]) != `"latest"` {
		t.Fatal(values, err)
	}
	checks := 0
	if err := store.SetIf(m, "text", json.RawMessage(`"cancelled while writing"`), func() bool { checks++; return checks == 1 }); err != nil {
		t.Fatal(err)
	}
	values, err = store.Get(m)
	if err != nil || string(values["text"]) != `"latest"` {
		t.Fatal("cancelled write was committed", values, err)
	}
}
