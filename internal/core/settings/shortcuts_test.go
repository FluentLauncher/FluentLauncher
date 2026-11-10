package settings

import (
	"encoding/json"
	"fluentlauncher/internal/core/shortcuts"
	"testing"
)

func TestShortcutMigrationAndExplicitClear(t *testing.T) {
	var value LauncherSettings
	if err := json.Unmarshal([]byte(`{"Hotkey":"Ctrl+Space"}`), &value); err != nil {
		t.Fatal(err)
	}
	binding := value.Shortcuts[shortcuts.Toggle]
	if !binding.Enabled || len(binding.Keys) != 1 || binding.Keys[0] != "Ctrl+Space" {
		t.Fatalf("migration: %+v", binding)
	}
	value.Shortcuts[shortcuts.Toggle] = shortcuts.Binding{Keys: []string{}, Enabled: false}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if value.Shortcuts[shortcuts.Toggle].Enabled {
		t.Fatal("old Hotkey resurrected cleared command")
	}
	delete(value.Shortcuts, shortcuts.Toggle)
	data, _ = json.Marshal(value)
	if err = json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	if len(value.Shortcuts) != 0 {
		t.Fatal("restoring defaults remigrated legacy field")
	}
}
