package plugin

import (
	"testing"

	"fluentlauncher/internal/plugin/protocol"
)

func TestMarketplaceKeepsInstalledAndLocalPackagesByID(t *testing.T) {
	items := Marketplace([]OfficialPlugin{{ID: "official.one", Name: "Same name", Version: "2.0.0"}, {ID: "official.two", Name: "Same name", Version: "1.0.0"}}, []Status{{Registration: protocol.Registration{Manifest: protocol.Manifest{ID: "official.one", Name: "Old name", Version: "1.0.0", Icon: "assets/icon.png"}}}, {Registration: protocol.Registration{Manifest: protocol.Manifest{ID: "vendor.local", Name: "Local", Version: "1.0.0", Description: "local description"}}}})
	if len(items) != 3 {
		t.Fatal(items)
	}
	var upgraded, local bool
	for _, item := range items {
		if item.ID == "official.one" {
			upgraded = item.UpdateAvailable() && item.Manifest().Icon == "assets/icon.png"
		}
		if item.ID == "vendor.local" {
			local = item.Installed != nil && item.Description == "local description"
		}
	}
	if !upgraded || !local {
		t.Fatal("identity merge lost installed metadata")
	}
	item := MarketItem{Official: &OfficialPlugin{RequiredHostServices: []protocol.ServiceRequirement{{Name: "future.service", Version: 1}, {Name: "context.get", Version: 2}}}}
	if len(item.MissingServices()) != 2 {
		t.Fatal("unsupported host requirements accepted")
	}
}
