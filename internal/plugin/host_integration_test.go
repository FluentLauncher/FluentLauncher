package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/internal/testutil"
)

type integrationHost struct {
	mu      sync.Mutex
	calls   []string
	block   bool
	entered chan struct{}
}

func TestRequiredHostServicesRejectMissingNegotiationAndUnsupportedInstall(t *testing.T) {
	source := testutil.PluginFixture(t, "vendor.required.queue")
	registration, err := protocol.ReadManifest(source)
	if err != nil {
		t.Fatal(err)
	}
	manifest := registration.Manifest
	manifest.HostServices = []string{"context.get"}
	manifest.RequiredHostServices = []protocol.ServiceRequirement{{Name: "context.get", Version: 1}}
	writeManifest := func() {
		t.Helper()
		data, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "plugin.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeManifest()
	repo := NewRepository(filepath.Join(t.TempDir(), "plugins"))
	if _, err := repo.Install(source, false); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	defer manager.Dispose()
	options := settings.DefaultPluginOptions()
	options.EnabledIDs = []string{manifest.ID}
	options.QueryTimeoutMs = 5000
	manager.Configure(repo.Installed(), options)
	results, err := manager.Search(context.Background(), search.Query{Text: "echo must not run", Scope: search.ScopeAll}, 10)
	if err != nil || len(results) != 1 || !strings.Contains(results[0].Entry.Subtitle, "requires negotiated") {
		t.Fatal(results, err)
	}
	if _, err := os.Stat(filepath.Join(repo.Installed()[0].Directory, "queries.txt")); !os.IsNotExist(err) {
		t.Fatal("business query ran without required host negotiation")
	}
	manifest.Version = "1.0.1"
	manifest.RequiredHostServices[0].Version = 2
	writeManifest()
	if _, err := repo.Install(source, true); err == nil {
		t.Fatal("unsupported required service was installed")
	}
	if repo.Installed()[0].Manifest.Version != "1.0.0" {
		t.Fatal("rejected update changed installed package")
	}
}

func (h *integrationHost) Configuration() (HostConfiguration, error) {
	return HostConfiguration{APIVersion: 1, Services: []HostService{{Name: "settings.get", Version: 1}, {Name: "ui.notify", Version: 1}}}, nil
}
func (h *integrationHost) Call(ctx context.Context, method string, params json.RawMessage, interactive bool) (any, error) {
	h.mu.Lock()
	h.calls = append(h.calls, method)
	h.mu.Unlock()
	if method == "host.settings.get" {
		return map[string]any{"values": map[string]any{"prefix": "host: ", "preview": true}}, nil
	}
	if method == "host.ui.notify" {
		if !interactive {
			return nil, context.Canceled
		}
		if h.block {
			close(h.entered)
			<-ctx.Done()
			return nil, ctx.Err()
		}
	}
	return map[string]any{}, nil
}

func TestIndependentPluginCallsHostAndStopsPendingService(t *testing.T) {
	directory := os.Getenv("FL_TEST_OFFICIAL_RELEASE_DIR")
	if directory == "" {
		t.Skip("set FL_TEST_OFFICIAL_RELEASE_DIR for SDK/host acceptance")
	}
	archives, err := filepath.Glob(filepath.Join(directory, "official.echo-*-windows-amd64.zip"))
	if err != nil || len(archives) != 1 {
		t.Fatal(archives, err)
	}
	repo := NewRepository(filepath.Join(t.TempDir(), "plugins"))
	if _, err := repo.Install(archives[0], false); err != nil {
		t.Fatal(err)
	}
	provider := &integrationHost{}
	manager := NewManager()
	defer manager.Dispose()
	manager.SetHostProvider(func(protocol.Registration) HostProvider { return provider })
	options := settings.DefaultPluginOptions()
	options.EnabledIDs = []string{"official.echo"}
	options.QueryTimeoutMs = 5000
	manager.Configure(repo.Installed(), options)
	results, err := manager.Search(context.Background(), search.Query{Text: "echo callback", Scope: search.ScopeAll}, 10)
	if err != nil || len(results) != 1 || results[0].Entry.Title != "host: callback" {
		t.Fatal(results, err)
	}
	entry := results[0].Entry
	if len(entry.Actions) < 2 {
		t.Fatal("new SDK did not receive host services")
	}
	if err := manager.Execute(context.Background(), entry, entry.Actions[1]); err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	calls := append([]string(nil), provider.calls...)
	provider.mu.Unlock()
	if len(calls) < 2 || calls[0] != "host.settings.get" || calls[1] != "host.ui.notify" {
		t.Fatal(calls)
	}
	// Restart with a deliberately pending host service. Disabling must stop
	// both the action and its callback without waiting for the action timeout.
	manager.Stop("official.echo")
	provider = &integrationHost{block: true, entered: make(chan struct{})}
	manager.SetHostProvider(func(protocol.Registration) HostProvider { return provider })
	results, err = manager.Search(context.Background(), search.Query{Text: "echo pending", Scope: search.ScopeAll}, 10)
	if err != nil || len(results) != 1 {
		t.Fatal(results, err)
	}
	entry = results[0].Entry
	done := make(chan error, 1)
	go func() { done <- manager.Execute(context.Background(), entry, entry.Actions[1]) }()
	<-provider.entered
	options.EnabledIDs = nil
	manager.Configure(repo.Installed(), options)
	if err := <-done; err == nil {
		t.Fatal("disabled process reported successful action")
	}
}
