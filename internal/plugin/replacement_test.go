package plugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/internal/testutil"
)

func TestReplacementBlocksQueriesAndFailureKeepsOldPackage(t *testing.T) {
	source := testutil.PluginFixture(t, "official.echo")
	repo := NewRepository(filepath.Join(t.TempDir(), "plugins"))
	if _, err := repo.Install(source, false); err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	defer m.Dispose()
	m.Configure(repo.Installed(), echoOptions())
	query := func() int {
		t.Helper()
		results, err := m.Search(context.Background(), search.Query{Text: "echo hello", Scope: search.ScopeAll}, 10)
		if err != nil {
			t.Fatal(err)
		}
		return len(results)
	}
	if query() != 1 {
		t.Fatal("fixture did not start")
	}
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- m.ReplacePackage("official.echo", func() ([]protocol.Registration, error) {
			close(entered)
			<-release
			return nil, errors.New("installation failed")
		})
	}()
	<-entered
	if query() != 0 {
		t.Fatal("query restarted the plugin while files were being replaced")
	}
	close(release)
	if err := <-done; err == nil {
		t.Fatal("installation failure lost")
	}
	if query() != 1 {
		t.Fatal("old plugin did not recover after failed installation")
	}
	stale := repo.Installed()[0]
	newSource := testutil.PluginFixture(t, "official.echo")
	newRegistration, err := protocol.ReadManifest(newSource)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.ReplacePackage("official.echo", func() ([]protocol.Registration, error) {
		if err := os.RemoveAll(stale.Directory); err != nil {
			return nil, err
		}
		return []protocol.Registration{newRegistration}, nil
	}); err != nil {
		t.Fatal(err)
	}
	results := m.searchOne(context.Background(), stale, "queued", "echo", 10, 2, echoOptions())
	if len(results) != 1 || results[0].Entry.Title != "queued" {
		t.Fatal("queued query used the old executable path", results)
	}
}
