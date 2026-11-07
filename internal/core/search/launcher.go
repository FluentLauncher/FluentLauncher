package search

import (
	"context"
	"sort"
	"strings"

	"fluentlauncher/internal/core/model"
)

const SettingsID = "launcher:settings"

// LauncherCommands exposes the built-in "设置" result.
type LauncherCommands struct{}

func (LauncherCommands) ID() string { return "launcher" }

func (LauncherCommands) Supports(q Query) bool {
	return q.Scope == ScopeAll || q.Scope == ScopeApplications
}

func (LauncherCommands) Search(ctx context.Context, q Query, limit int) ([]model.Result, error) {
	text := strings.TrimSpace(q.Text)
	if text == "" {
		return nil, nil
	}
	for _, keyword := range []string{"设置", "设置中心", "配置", "settings", "setting", "shezhi", "sz"} {
		if strings.HasPrefix(strings.ToLower(keyword), strings.ToLower(text)) {
			return []model.Result{{Entry: model.Entry{
				ID:       SettingsID,
				Title:    "设置",
				Subtitle: "FluentLauncher",
				Kind:     model.Command,
				SourceID: "launcher",
			}, Score: 30000}}, nil
		}
	}
	return nil, nil
}

const MaxRecent = 10

// RecentEntries returns the most recently opened entries. It reads only
// profiles; it never stores a separate query history.
func RecentEntries(enabled bool, profiles []model.Profile,
	find func(string) (model.Entry, bool), limit int) []model.Result {
	if !enabled || limit <= 0 {
		return nil
	}
	sorted := make([]model.Profile, 0, len(profiles))
	for _, profile := range profiles {
		if !profile.Hidden && profile.LastUsedAt != nil {
			sorted = append(sorted, profile)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].LastUsedAt.After(*sorted[j].LastUsedAt)
	})
	results := make([]model.Result, 0, min(limit, MaxRecent))
	for _, profile := range sorted {
		if len(results) >= min(limit, MaxRecent) {
			break
		}
		entry, ok := find(profile.EntryID)
		if !ok {
			continue
		}
		results = append(results, model.Result{Entry: entry, Score: 10000 - len(results)})
	}
	return results
}
