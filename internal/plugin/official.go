package plugin

import (
	_ "embed"
	"encoding/json"
	"sync"

	"github.com/egoist/mygo/i18n"
)

//go:embed official_index.json
var officialIndexData []byte

// OfficialPlugin describes one of the plugins that ship with the launcher,
// named and summarised in the language the app shows. The install area lists
// them by this catalog before any of them is installed, which is text the
// page cannot take from the packages themselves: reading plugin.json for
// every frame would be file I/O on the UI thread.
type OfficialPlugin struct {
	// ID is both the manifest id and the package directory under
	// resources/examples.
	ID string
	// Name is what the page shows; it repeats the manifest's name.
	Name string
	// Summary is one sentence about what the plugin answers.
	Summary string
}

type officialIndexEntry struct {
	ID      string            `json:"id"`
	Name    map[string]string `json:"name"`
	Summary map[string]string `json:"summary"`
}

var (
	officialOnce  sync.Once
	officialIndex []officialIndexEntry
)

// OfficialPlugins returns the bundled plugins, in the order the settings page
// lists them. The list is data (official_index.json), not code: shipping one
// more official plugin is a folder under resources/examples plus an entry in
// the index, in every language at once.
func OfficialPlugins() []OfficialPlugin {
	officialOnce.Do(func() {
		if err := json.Unmarshal(officialIndexData, &officialIndex); err != nil {
			panic("plugin: official index: " + err.Error())
		}
	})
	locale := i18n.Locale()
	if locale == "" {
		locale = "zh-Hans"
	}
	out := make([]OfficialPlugin, 0, len(officialIndex))
	for _, entry := range officialIndex {
		out = append(out, OfficialPlugin{
			ID:      entry.ID,
			Name:    officialText(entry.Name, locale),
			Summary: officialText(entry.Summary, locale),
		})
	}
	return out
}

func officialText(texts map[string]string, locale string) string {
	if text, ok := texts[locale]; ok {
		return text
	}
	if text, ok := texts["zh-Hans"]; ok {
		return text
	}
	for _, text := range texts {
		return text
	}
	return ""
}
