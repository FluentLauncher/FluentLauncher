package plugin

import (
	"context"
	"encoding/json"
	"fluentlauncher/internal/plugin/protocol"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestThemeColdStartUpdatesAndGracefulClose(t *testing.T) {
	m, r, _ := commandManager(t, "vendor.commands.theme")
	theme := protocol.Theme{Dark: true, Background: 0x121212, Text: 0xfafafa, Accent: 0x667788}
	m.SetTheme(theme)
	if m.Statuses()[0].Running {
		t.Fatal("theme eagerly started process")
	}
	if err := m.ExecuteCommand(context.Background(), r.Manifest.ID, "write"); err != nil {
		t.Fatal(err)
	}
	read := func() protocol.Theme {
		data, err := os.ReadFile(filepath.Join(r.Directory, "theme.json"))
		if err != nil {
			t.Fatal(err)
		}
		var got protocol.Theme
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := read(); got != theme {
		t.Fatal("cold-start theme", got)
	}
	theme.Background = 0xf0f0f0
	theme.Dark = false
	m.SetTheme(theme)
	deadline := time.Now().Add(3 * time.Second)
	for read() != theme {
		if time.Now().After(deadline) {
			t.Fatal("live theme not delivered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	m.Stop(r.Manifest.ID)
	if data, err := os.ReadFile(filepath.Join(r.Directory, "closed.txt")); err != nil || string(data) != "eof" {
		t.Fatal("host killed plugin before EOF cleanup", err)
	}
}
