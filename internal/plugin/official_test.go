package plugin

import (
	"testing"

	"fluentlauncher/internal/plugin/protocol"
)

func TestFallbackMetadataHasNoUnverifiedDownloads(t *testing.T) {
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
		if !protocol.ValidID(entry.ID) || entry.Version != "" || entry.Package.URL != "" {
			t.Fatalf("fallback metadata must not offer unverified downloads: %+v", entry)
		}
	}
}
