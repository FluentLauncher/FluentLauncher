package plugin

import (
	"path/filepath"
	"testing"

	"fluentlauncher/internal/plugin/protocol"
)

// The install area lists the bundled examples before anything is installed, so
// their names and ids come from this catalog rather than from the packages.
// This keeps the catalog and the packages from drifting apart.
func TestOfficialPluginsMatchTheirPackages(t *testing.T) {
	official := OfficialPlugins()
	if len(official) == 0 {
		t.Fatal("the catalog is empty")
	}
	seen := map[string]bool{}
	for _, entry := range official {
		if seen[entry.ID] {
			t.Fatalf("%s listed twice", entry.ID)
		}
		seen[entry.ID] = true
		if entry.Name == "" || entry.Summary == "" {
			t.Fatalf("%s has no name or summary: %+v", entry.ID, entry)
		}
		source, err := filepath.Abs(filepath.Join("../../resources/examples", entry.ID))
		if err != nil {
			t.Fatal(err)
		}
		registration, err := protocol.ReadManifest(source)
		if err != nil {
			t.Fatalf("%s: %v", entry.ID, err)
		}
		if registration.Manifest.Name != entry.Name {
			t.Fatalf("%s is called %q in its package and %q in the catalog",
				entry.ID, registration.Manifest.Name, entry.Name)
		}
		if len(registration.Manifest.Keywords) == 0 {
			t.Fatalf("%s has no keywords, so no search would reach it", entry.ID)
		}
	}
}
