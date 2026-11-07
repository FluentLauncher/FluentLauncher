package search

import (
	"context"
	"sort"
	"strings"
	"sync"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
)

type Scope uint8

const (
	ScopeAll Scope = iota
	ScopeApplications
	ScopeFiles
	ScopeWeb
	ScopePlugins
	ScopeWindows
)

type Query struct {
	Text  string
	Scope Scope
}

// ParseQuery recognizes the scope prefixes. "win" alone lists windows;
// other prefixes need a space so "window" is not swallowed.
func ParseQuery(text string) Query {
	trimmed := strings.TrimSpace(text)
	if strings.EqualFold(trimmed, "win") {
		return Query{Scope: ScopeWindows}
	}
	for _, prefix := range []struct {
		text  string
		scope Scope
	}{
		{"app ", ScopeApplications},
		{"file ", ScopeFiles},
		{"web ", ScopeWeb},
		{"win ", ScopeWindows},
		{">", ScopePlugins},
	} {
		if len(text) >= len(prefix.text) && strings.EqualFold(text[:len(prefix.text)], prefix.text) {
			return Query{Text: strings.TrimSpace(text[len(prefix.text):]), Scope: prefix.scope}
		}
	}
	return Query{Text: trimmed, Scope: ScopeAll}
}

type Batch struct {
	SourceID string
	Results  []model.Result
	Error    string
}

type Provider interface {
	ID() string
	Supports(Query) bool
	Search(context.Context, Query, int) ([]model.Result, error)
}

// Run queries supported providers concurrently and yields each batch as it
// finishes. A provider failure becomes a Batch error and never stops others.
func Run(ctx context.Context, providers []Provider, q Query, limit int) <-chan Batch {
	out := make(chan Batch)
	var wg sync.WaitGroup
	for _, provider := range providers {
		if !provider.Supports(q) {
			continue
		}
		wg.Add(1)
		go func(p Provider) {
			defer wg.Done()
			results, err := p.Search(ctx, q, limit)
			batch := Batch{SourceID: p.ID(), Results: results}
			if err != nil && ctx.Err() == nil {
				batch.Error = err.Error()
			}
			select {
			case out <- batch:
			case <-ctx.Done():
			}
		}(provider)
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// Merge orders all results by score and keeps the first of duplicate paths
// (unless a custom item, whose arguments make it distinct).
func Merge(batches []Batch, limit int) []model.Result {
	if limit <= 0 {
		return nil
	}
	all := make([]model.Result, 0)
	for _, batch := range batches {
		all = append(all, batch.Results...)
	}
	sort.SliceStable(all, func(i, j int) bool { return better(all[i], all[j]) })
	seen := map[string]bool{}
	out := make([]model.Result, 0, min(limit, len(all)))
	for _, result := range all {
		key := strings.ToLower(result.Entry.ID)
		if result.Entry.SourceID != settings.CustomSourceID &&
			(result.Entry.Kind == model.File || result.Entry.Kind == model.Directory) {
			key = "path:" + strings.ToLower(result.Entry.LaunchTarget)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, result)
		if len(out) >= limit {
			break
		}
	}
	return out
}
