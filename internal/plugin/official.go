package plugin

import (
	_ "embed"
	"encoding/json"
	"sync"

	"github.com/egoist/mygo/i18n"
)

//go:embed official_index.json
var officialIndexData []byte

// OfficialPlugin is one catalogue entry in the current UI language. Embedded
// fallback names have no Version or Package and cannot offer downloads.
type OfficialPlugin struct {
	// ID is both the manifest id and its installed package directory name.
	ID string
	// Name is what the page shows; it repeats the manifest's name.
	Name string
	// Summary is one sentence about what the plugin answers.
	Summary string
	Version string
	Package PluginPackage
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

// OfficialPlugins supplies names while the signed catalogue is unavailable.
// Download metadata and newly published plugins come from Catalog.Localized.
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
