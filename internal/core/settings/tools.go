package settings

import (
	"fmt"
	"strings"
	"unicode"

	"fluentlauncher/locales"
)

// ToolOptions controls dispatch before any tool work is performed. A prefix
// replaces automatic recognition; GlobalQuery controls unprefixed recognition.
type ToolOptions struct {
	ID          string   `json:"Id"`
	Enabled     bool     `json:"Enabled"`
	Prefix      string   `json:"Prefix"`
	Keywords    []string `json:"Keywords"`
	GlobalQuery bool     `json:"GlobalQuery"`
}

func DefaultTools() []ToolOptions {
	return []ToolOptions{
		{ID: "calc", Enabled: true, Prefix: "=", Keywords: []string{"calc", "计算"}},
		{ID: "uuid", Enabled: true, GlobalQuery: true, Keywords: []string{"uuid", "guid"}},
		{ID: "date", Enabled: true, GlobalQuery: true, Keywords: []string{"date", "today", "日期", "今天"}},
		{ID: "time", Enabled: true, GlobalQuery: true, Keywords: []string{"time", "now", "时间", "现在"}},
		{ID: "timestamp", Enabled: true, GlobalQuery: true, Keywords: []string{"timestamp", "unix", "时间戳"}},
		{ID: "week", Enabled: true, GlobalQuery: true, Keywords: []string{"week", "周数"}},
		{ID: "days", Enabled: true, GlobalQuery: true, Keywords: []string{"days", "天数"}},
		{ID: "rand", Enabled: true, GlobalQuery: true, Keywords: []string{"rand", "随机数"}},
		{ID: "pass", Enabled: true, GlobalQuery: true, Keywords: []string{"pass", "password", "密码"}},
		{ID: "convert", Enabled: true, Keywords: []string{"convert", "换算"}},
		{ID: "b64", Enabled: true, GlobalQuery: true, Keywords: []string{"b64", "base64"}},
		{ID: "unb64", Enabled: true, GlobalQuery: true, Keywords: []string{"unb64"}},
		{ID: "urlencode", Enabled: true, GlobalQuery: true, Keywords: []string{"urlencode"}},
		{ID: "urldecode", Enabled: true, GlobalQuery: true, Keywords: []string{"urldecode"}},
		{ID: "color", Enabled: true, Keywords: []string{"color", "颜色"}},
	}
}

func (s LauncherSettings) Tool(id string) ToolOptions {
	for _, tool := range s.Tools {
		if tool.ID == id {
			if id == "calc" && !s.EnableCalculator {
				tool.Enabled = false
			}
			return tool
		}
	}
	return ToolOptions{ID: id}
}

func (s *LauncherSettings) normalizeTools() {
	for _, def := range DefaultTools() {
		found := false
		for _, tool := range s.Tools {
			found = found || tool.ID == def.ID
		}
		if !found {
			s.Tools = append(s.Tools, def)
		}
	}
	if !s.EnableCalculator {
		for i := range s.Tools {
			if s.Tools[i].ID == "calc" {
				s.Tools[i].Enabled = false
			}
		}
	}
}

func (s LauncherSettings) validateTools() error {
	known := map[string]bool{}
	for _, tool := range DefaultTools() {
		known[tool.ID] = true
	}
	ids, words := map[string]bool{}, map[string]string{}
	var prefixes []string
	for _, tool := range s.Tools {
		if !known[tool.ID] || ids[tool.ID] || len(tool.Keywords) == 0 || len(tool.Keywords) > 16 {
			return fmt.Errorf("%s: %s", locales.ToolsInvalid(), tool.ID)
		}
		ids[tool.ID] = true
		if len(tool.Prefix) > 32 || strings.IndexFunc(tool.Prefix, unicode.IsSpace) >= 0 || strings.ContainsRune(tool.Prefix, '\x00') {
			return fmt.Errorf("%s: %s", locales.ToolsInvalidPrefix(), tool.ID)
		}
		if tool.Enabled && tool.Prefix != "" {
			key := strings.ToLower(tool.Prefix)
			for _, reserved := range append(append([]string{}, ReservedKeywords...), ">", "tool") {
				if strings.HasPrefix(key, reserved) || strings.HasPrefix(reserved, key) {
					return fmt.Errorf("%s: %s", locales.ToolsConflict(), tool.Prefix)
				}
			}
			for _, prefix := range prefixes {
				if strings.HasPrefix(prefix, key) || strings.HasPrefix(key, prefix) {
					return fmt.Errorf("%s: %s / %s", locales.ToolsConflict(), prefix, key)
				}
			}
			prefixes = append(prefixes, key)
		}
		for _, word := range tool.Keywords {
			key := strings.ToLower(word)
			if len(word) == 0 || len(word) > 32 || strings.IndexFunc(word, unicode.IsSpace) >= 0 || strings.ContainsAny(word, "\x00=>") {
				return fmt.Errorf("%s: %s", locales.ToolsInvalid(), word)
			}
			for _, reserved := range ReservedKeywords {
				if key == reserved {
					return fmt.Errorf("%s: %s", locales.ToolsConflict(), word)
				}
			}
			if known[key] && key != tool.ID {
				return fmt.Errorf("%s: %s", locales.ToolsConflict(), word)
			}
			if owner := words[key]; owner != "" {
				return fmt.Errorf("%s: %s (%s / %s)", locales.ToolsConflict(), word, owner, tool.ID)
			}
			words[key] = tool.ID
		}
	}
	for _, tool := range s.Tools {
		if !tool.Enabled || tool.Prefix == "" {
			continue
		}
		for _, other := range s.Tools {
			if !other.Enabled || other.ID == tool.ID || !other.GlobalQuery || other.Prefix != "" {
				continue
			}
			for _, word := range other.Keywords {
				if strings.HasPrefix(strings.ToLower(word), strings.ToLower(tool.Prefix)) {
					return fmt.Errorf("%s: %s / %s", locales.ToolsConflict(), tool.Prefix, word)
				}
			}
		}
	}
	for _, site := range s.WebSites {
		if !site.Enabled {
			continue
		}
		for _, tool := range s.Tools {
			if !tool.Enabled {
				continue
			}
			for _, word := range tool.Keywords {
				if tool.GlobalQuery && tool.Prefix == "" && strings.EqualFold(word, site.Keyword) {
					return fmt.Errorf("%s: %s (%s / %s)", locales.ToolsConflict(), word, tool.ID, site.Name)
				}
			}
			if tool.Prefix != "" && strings.HasPrefix(strings.ToLower(site.Keyword), strings.ToLower(tool.Prefix)) {
				return fmt.Errorf("%s: %s / %s", locales.ToolsConflict(), tool.Prefix, site.Keyword)
			}
		}
	}
	return nil
}
