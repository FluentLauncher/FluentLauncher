package plugin

import (
	"sort"
	"strings"

	"fluentlauncher/internal/plugin/hostservices"
	"fluentlauncher/internal/plugin/protocol"
)

// MarketItem joins catalogue and installed metadata by identity, not by name.
// Offline/local packages stay manageable without an online catalogue entry.
type MarketItem struct {
	ID, Name, Summary, Description, Usage, Author, Homepage string
	Official                                                *OfficialPlugin
	Installed                                               *Status
}

func (p MarketItem) UpdateAvailable() bool {
	return p.Official != nil && p.Installed != nil && p.Official.NewerThan(p.Installed.Registration.Manifest.Version)
}

func (p MarketItem) MissingServices() []string {
	var missing []string
	requirements := p.Manifest().RequiredHostServices
	if p.Official != nil {
		requirements = p.Official.RequiredHostServices
	}
	for _, required := range requirements {
		if hostservices.Version(required.Name) < required.Version {
			missing = append(missing, required.Name)
		}
	}
	return missing
}

func (p MarketItem) Services() []string {
	if p.Official != nil && len(p.Official.HostServices) > 0 {
		return p.Official.HostServices
	}
	return p.Manifest().HostServices
}

func Marketplace(official []OfficialPlugin, installed []Status) []MarketItem {
	items := make([]MarketItem, 0, len(official)+len(installed))
	indices := map[string]int{}
	for _, entry := range official {
		key := strings.ToLower(entry.ID)
		indices[key] = len(items)
		items = append(items, MarketItem{ID: entry.ID, Name: entry.Name, Summary: entry.Summary,
			Description: entry.Description, Usage: entry.Usage, Author: entry.Author, Homepage: entry.Homepage, Official: &entry})
	}
	for _, status := range installed {
		m := status.Registration.Manifest
		index, ok := indices[strings.ToLower(m.ID)]
		if !ok {
			index = len(items)
			indices[strings.ToLower(m.ID)] = index
			items = append(items, MarketItem{ID: m.ID, Name: m.Name, Summary: m.Description})
		}
		item := &items[index]
		item.Installed = &status
		if item.Description == "" {
			item.Description = m.Description
		}
		if item.Usage == "" {
			item.Usage = m.Usage
		}
		if item.Author == "" {
			item.Author = m.Author
		}
		if item.Homepage == "" {
			item.Homepage = m.Homepage
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (p MarketItem) Manifest() protocol.Manifest {
	if p.Installed != nil {
		return p.Installed.Registration.Manifest
	}
	return protocol.Manifest{ID: p.ID, Name: p.Name}
}
