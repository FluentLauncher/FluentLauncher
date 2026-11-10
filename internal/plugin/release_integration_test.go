package plugin

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/core/settings"
)

// Optional cross-repository acceptance test. Normal host CI remains completely
// independent of the official plugin repo, the network and its executables.
func TestIndependentOfficialReleaseIntegration(t *testing.T) {
	dir := os.Getenv("FL_TEST_OFFICIAL_RELEASE_DIR")
	if dir == "" {
		t.Skip("set FL_TEST_OFFICIAL_RELEASE_DIR to a locally signed plugin release")
	}
	client := &CatalogClient{Repository: "mistquay/FluentLauncher-plugins", PublicKey: os.Getenv("FL_TEST_OFFICIAL_PUBLIC_KEY"), CachePath: filepath.Join(t.TempDir(), "catalog.json")}
	client.HTTP = &http.Client{Transport: catalogTransport(func(req *http.Request) (*http.Response, error) {
		f, err := os.Open(filepath.Join(dir, filepath.Base(req.URL.Path)))
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Body: f, Header: make(http.Header)}, nil
	})}
	catalog, err := client.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(filepath.Join(t.TempDir(), "plugins"))
	for _, entry := range catalog.Localized() {
		archive := filepath.Join(t.TempDir(), "plugin.zip")
		if err := client.Download(context.Background(), entry, archive); err != nil {
			t.Fatal(entry.ID, err)
		}
		if _, err := repo.Install(archive, false); err != nil {
			t.Fatal(entry.ID, err)
		}
	}
	if len(repo.Installed()) != 3 {
		t.Fatalf("installed %d official packages", len(repo.Installed()))
	}
	m := NewManager()
	defer m.Dispose()
	m.Configure(repo.Installed(), settings.PluginOptions{EnabledIDs: []string{"official.echo", "official.devtools"}, QueryTimeoutMs: 5000, MaxResults: 20, IdleSeconds: 30, MaxActiveProcesses: 3})
	for _, q := range []struct{ text, title string }{{"echo independent SDK", "independent SDK"}, {"hash abc", "SHA-256"}} {
		results, err := m.Search(context.Background(), search.Query{Text: q.text, Scope: search.ScopeAll}, 20)
		if err != nil || len(results) == 0 || results[0].Entry.Title != q.title {
			t.Fatalf("cross-repository query %q: %+v, %v", q.text, results, err)
		}
	}
}
