package plugin

import (
	"context"
	"path/filepath"
	"testing"

	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/testutil"
)

func TestManagerEcho(t *testing.T) {
	source := testutil.PluginFixture(t, "official.echo")
	repo := NewRepository(filepath.Join(t.TempDir(), "plugins"))
	if _, err := repo.Install(source, false); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	defer manager.Dispose()
	manager.Configure(repo.Installed(), settings.PluginOptions{
		EnabledIDs:         []string{"official.echo"},
		QueryTimeoutMs:     3000,
		MaxResults:         20,
		IdleSeconds:        30,
		MaxActiveProcesses: 3,
	})
	results, err := manager.Search(context.Background(), search.Query{Text: "echo hello", Scope: search.ScopeAll}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Entry.Title != "hello" {
		t.Fatalf("results = %+v", results)
	}
	entry := results[0].Entry
	if len(entry.Actions) != 1 {
		t.Fatalf("actions = %+v", entry.Actions)
	}
	if err := manager.Execute(context.Background(), entry, entry.Actions[0]); err != nil {
		t.Fatal(err)
	}
}
func TestManagerDevTools(t *testing.T) {
	source := testutil.PluginFixture(t, "official.devtools")
	repo := NewRepository(filepath.Join(t.TempDir(), "plugins"))
	if _, err := repo.Install(source, false); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	defer manager.Dispose()
	manager.Configure(repo.Installed(), settings.PluginOptions{
		EnabledIDs:         []string{"official.devtools"},
		QueryTimeoutMs:     3000,
		MaxResults:         20,
		IdleSeconds:        30,
		MaxActiveProcesses: 3,
	})
	results, err := manager.Search(context.Background(), search.Query{Text: "hash abc", Scope: search.ScopeAll}, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || results[0].Entry.Title != "SHA-256" {
		t.Fatalf("results = %+v", results)
	}
	if results[0].Entry.CopyText == "" {
		t.Fatal("copy text not mapped")
	}
}
