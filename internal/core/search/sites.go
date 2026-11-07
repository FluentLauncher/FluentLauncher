package search

import (
	"context"
	"fmt"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
)

// SiteSearchProvider turns "gh term" into a named web search result.
type SiteSearchProvider struct {
	Settings Settings
}

func (p SiteSearchProvider) ID() string { return "sites" }

func (p SiteSearchProvider) Supports(q Query) bool {
	return q.Scope == ScopeAll && q.Text != ""
}

func (p SiteSearchProvider) Search(ctx context.Context, q Query, limit int) ([]model.Result, error) {
	site, term, ok := settings.MatchSite(p.Settings().WebSites, q.Text)
	if !ok {
		return nil, nil
	}
	target := site.BuildURL(term)
	return []model.Result{{Entry: model.Entry{
		ID:           fmt.Sprintf("site:%s:%s", site.Keyword, term),
		Title:        site.Name + "：" + term,
		Subtitle:     target,
		LaunchTarget: target,
		Kind:         model.URL,
		SourceID:     "sites",
	}, Score: 21000}}, nil
}
