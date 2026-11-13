package plugin

import (
	"context"
	"encoding/json"
	"fluentlauncher/internal/plugin/hostservices"
	"fluentlauncher/internal/plugin/protocol"
	"fmt"
)

type HostService struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

type HostConfiguration struct {
	APIVersion  int           `json:"apiVersion"`
	HostVersion string        `json:"hostVersion"`
	Locale      string        `json:"locale"`
	DataDir     string        `json:"dataDir"`
	CacheDir    string        `json:"cacheDir"`
	Services    []HostService `json:"services"`
}

// A provider belongs to one immutable registration and process session. The
// caller's identity is bound here, never accepted from plugin request params.
type HostProvider interface {
	Configuration() (HostConfiguration, error)
	Call(context.Context, string, json.RawMessage, bool) (any, error)
}

func ValidateHostRequirements(manifest protocol.Manifest) error {
	for _, required := range manifest.RequiredHostServices {
		if hostservices.Version(required.Name) < required.Version {
			return fmt.Errorf("plugin %s requires host service %s version %d; update FluentLauncher", manifest.ID, required.Name, required.Version)
		}
	}
	return nil
}
