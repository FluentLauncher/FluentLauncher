package shortcuts

import (
	"errors"
	"reflect"
	"testing"
)

func TestNormalizationAndReservedKeys(t *testing.T) {
	chord, err := Parse(" shift + control + esc ")
	if err != nil || chord.String() != "Ctrl+Shift+Escape" {
		t.Fatalf("%+v %v", chord, err)
	}
	for _, key := range []string{"Ctrl", "Ctrl++K", "Ctrl+Ctrl+K", "Alt+F4", "Win+L", "Ctrl+Alt+Delete", "Ctrl+Shift+Escape", "A", "Shift+Space"} {
		if _, err := ValidateKey(key, Global); err == nil {
			t.Errorf("accepted global %q", key)
		}
	}
	for _, key := range []string{"Ctrl+C", "Ctrl+V", "Ctrl+X", "Ctrl+A", "Ctrl+Z", "Ctrl+Y", "Tab"} {
		if _, err := ValidateKey(key, Search); err == nil {
			t.Errorf("accepted editor key %s", key)
		}
	}
	for _, key := range KeyNames() {
		if _, err := Parse(key); err != nil {
			t.Errorf("unparseable supported key: %s", key)
		}
	}
}

func TestScopeConflictsAndPluginDefaults(t *testing.T) {
	commands := Builtins()
	if conflicts := Conflicts(commands, nil); len(conflicts) != 0 {
		t.Fatalf("defaults conflict: %+v", conflicts)
	}
	plugin := Command{ID: "plugin:vendor.tool:capture", Scope: Global, PluginID: "vendor.tool", Defaults: []string{"Ctrl+K"}, Available: true}
	commands = append(commands, plugin)
	if Effective(plugin, nil).Enabled {
		t.Fatal("plugin default unexpectedly enabled")
	}
	if len(Conflicts(commands, nil)) != 0 {
		t.Fatal("disabled plugin conflicts")
	}
	overrides := map[string]Binding{plugin.ID: {Keys: []string{"Ctrl+K"}, Enabled: true}}
	conflicts := Conflicts(commands, overrides)
	if len(conflicts) != 2 {
		t.Fatalf("global should overlap search and actions: %+v", conflicts)
	}
	commands[len(commands)-1].Available = false
	if len(Conflicts(commands, overrides)) != 0 {
		t.Fatal("unavailable plugin conflicts")
	}
	if b := Effective(commands[0], map[string]Binding{Toggle: {Keys: []string{}, Enabled: false}}); b.Enabled || len(b.Keys) > 0 {
		t.Fatal("explicit clear replaced by default")
	}
}

func TestRegistryRollbackAndNoOp(t *testing.T) {
	osKeys := map[string]string{}
	calls := 0
	r := Registry{
		Register: func(key, id string) error {
			calls++
			if key == "Ctrl+F12" {
				return errors.New("occupied")
			}
			if osKeys[key] != "" {
				return errors.New("duplicate")
			}
			osKeys[key] = id
			return nil
		},
		Unregister: func(key string) { calls++; delete(osKeys, key) },
	}
	before := map[string]string{"Alt+Space": "open", "Ctrl+F2": "tool"}
	if err := r.Apply(before, true); err != nil {
		t.Fatal(err)
	}
	n := calls
	if err := r.Apply(before, true); err != nil || calls != n {
		t.Fatal("unchanged settings reregistered keys")
	}
	if err := r.Apply(map[string]string{"Ctrl+F12": "open", "Ctrl+F2": "tool"}, true); err == nil {
		t.Fatal("occupied hotkey accepted")
	}
	if !reflect.DeepEqual(osKeys, before) || !reflect.DeepEqual(r.Active, before) {
		t.Fatalf("rollback: %+v %+v", osKeys, r.Active)
	}
	swap := map[string]string{"Alt+Space": "tool", "Ctrl+F2": "open"}
	if err := r.Apply(swap, true); err != nil || !reflect.DeepEqual(osKeys, swap) {
		t.Fatalf("swap: %+v %v", osKeys, err)
	}
	if err := r.Apply(map[string]string{"Ctrl+F12": "broken", "Ctrl+F3": "good"}, false); err == nil || osKeys["Ctrl+F3"] != "good" {
		t.Fatalf("best effort: %+v %v", osKeys, err)
	}
}
