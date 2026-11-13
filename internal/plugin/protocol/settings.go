package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type ServiceRequirement struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
}

type SettingDefinition struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Type        string          `json:"type"` // text, bool, choice, number
	Default     json.RawMessage `json:"default"`
	Options     []string        `json:"options,omitempty"`
	Min         *float64        `json:"min,omitempty"`
	Max         *float64        `json:"max,omitempty"`
}

func (s SettingDefinition) ValidateValue(value json.RawMessage) error {
	invalid := fmt.Errorf("invalid plugin setting: %s", s.ID)
	if len(value) > 16000 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return invalid
	}
	switch s.Type {
	case "text", "choice":
		var text string
		if json.Unmarshal(value, &text) != nil {
			return invalid
		}
		if s.Type == "choice" {
			for _, option := range s.Options {
				if option == text {
					return nil
				}
			}
			return invalid
		}
	case "bool":
		var boolean bool
		if json.Unmarshal(value, &boolean) != nil {
			return invalid
		}
	case "number":
		var number float64
		if json.Unmarshal(value, &number) != nil {
			return invalid
		}
		if s.Min != nil && number < *s.Min || s.Max != nil && number > *s.Max {
			return invalid
		}
	default:
		return invalid
	}
	return nil
}

func (m Manifest) validateHostMetadata() error {
	if len(m.HostServices) > 32 || len(m.RequiredHostServices) > 32 || len(m.Settings) > 50 {
		return errors.New("too much plugin host metadata")
	}
	seen := map[string]bool{}
	for _, name := range m.HostServices {
		if name == "" || len(name) > 100 || strings.ContainsAny(name, " /\\\t\r\n") || seen[name] {
			return fmt.Errorf("invalid host service declaration: %s", name)
		}
		seen[name] = true
	}
	for _, required := range m.RequiredHostServices {
		if !seen[required.Name] || required.Version < 1 {
			return fmt.Errorf("invalid required host service: %s", required.Name)
		}
	}
	seen = map[string]bool{}
	for _, setting := range m.Settings {
		if !ValidID(setting.ID) || seen[setting.ID] || strings.TrimSpace(setting.Title) == "" || len(setting.Title) > 400 || len(setting.Description) > 4000 || len(setting.Options) > 100 {
			return errors.New("invalid plugin settings schema")
		}
		if setting.Min != nil && setting.Max != nil && *setting.Min > *setting.Max {
			return errors.New("invalid setting range")
		}
		if err := setting.ValidateValue(setting.Default); err != nil {
			return err
		}
		seen[setting.ID] = true
	}
	return nil
}
