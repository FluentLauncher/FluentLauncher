package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/internal/plugin/rpc"
)

func TestHostServicesBindIdentityAndEnforceDeclarations(t *testing.T) {
	a := newPluginsApp(t)
	m := protocol.Manifest{ID: "vendor.bound", HostServices: []string{"context.get", "settings.get", "settings.set", "launcher.show"}, Settings: []protocol.SettingDefinition{{ID: "name", Type: "text", Default: json.RawMessage(`"bound"`)}}}
	h := a.newPluginHost(protocol.Registration{Manifest: m})
	configuration, err := h.Configuration()
	if err != nil || len(configuration.Services) != 4 {
		t.Fatal(configuration, err)
	}
	if _, err := h.Call(context.Background(), "host.settings.set", json.RawMessage(`{"pluginId":"another.plugin","key":"name","value":"updated"}`), false); err != nil {
		t.Fatal(err)
	}
	values, err := a.pluginData.Get(m)
	if err != nil || string(values["name"]) != `"updated"` {
		t.Fatal(values, err)
	}
	m.ID = "another.plugin"
	values, err = a.pluginData.Get(m)
	if err != nil || string(values["name"]) != `"bound"` {
		t.Fatal("caller supplied identity was accepted")
	}
	for _, method := range []string{"host.clipboard.readText", "host.launcher.show", "host.unknown"} {
		_, err := h.Call(context.Background(), method, json.RawMessage(`{}`), false)
		var rpcErr *rpc.Error
		if !errors.As(err, &rpcErr) {
			t.Fatalf("%s was not rejected with an RPC error: %v", method, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Call(ctx, "host.context.get", nil, false); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled service ran", err)
	}
}
