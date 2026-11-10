package app

import (
	"testing"

	"fluentlauncher/internal/store"
)

func TestUnrelatedSettingsDoNotReconfigureSubsystems(t *testing.T) {
	a := newLocalizedApp()
	a.config = store.NewConfigStore(t.TempDir())
	a.appliedTheme = a.settings.Theme
	value := cloneSettings(a.settings)
	value.WebSearchTemplate = "https://example.com/?q={query}"
	value.Files.EnableEverything = !value.Files.EnableEverything
	// No native windows, catalog, plugins or hooks are provided. An unrelated
	// edit must only persist its settings and publish the new snapshot.
	if err := a.applySettings(value); err != nil {
		t.Fatal(err)
	}
	if a.currentSettings().WebSearchTemplate != value.WebSearchTemplate {
		t.Fatal("settings snapshot not published")
	}
	if a.settings.Theme != a.appliedTheme {
		t.Fatal("unrelated edit changed the theme")
	}
}
