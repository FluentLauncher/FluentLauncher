package app

import (
	"context"
	"reflect"
	"sync"
	"time"
	"unicode/utf8"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/diag"
)

type queryDelivery struct {
	generation int64
	update     search.Update
	preferred  string
	started    time.Time
	queued     time.Time
}

// latestQueryUpdate allows at most one queued UI callback across ALL queries.
// Even a canceled worker racing a newer generation cannot replace its result.
type latestQueryUpdate struct {
	mu     sync.Mutex
	latest queryDelivery
	posted bool
}

func (m *latestQueryUpdate) publish(value queryDelivery, post func(func()), apply func(queryDelivery)) {
	m.mu.Lock()
	if value.generation < m.latest.generation {
		m.mu.Unlock()
		return
	}
	m.latest = value
	if m.posted {
		m.mu.Unlock()
		return
	}
	m.posted = true
	m.mu.Unlock()
	post(func() {
		m.mu.Lock()
		value := m.latest
		m.latest.update = search.Update{} // do not retain plugin payloads after apply
		m.posted = false
		m.mu.Unlock()
		apply(value)
	})
}

func (a *App) deliverQuery(value queryDelivery) {
	if value.generation != a.queryGeneration.Load() || a.quitting {
		return
	}
	started := time.Now()
	first := !a.resultsReady
	if !reflect.DeepEqual(a.results, value.update.Results) {
		// Respect keyboard navigation made since this query started. Late
		// providers must not restore the selection captured at the first key.
		preferred := value.preferred
		if a.resultsReady {
			preferred = a.selectedID
		}
		a.setResults(value.update.Results, preferred)
	}
	a.resultsReady = true
	a.lastBatches = value.update.Batches
	applied := time.Now()
	if a.diagnostics != nil {
		a.diagnostics.Measure("ui_queue", started.Sub(value.queued), "query_id", value.generation)
		a.diagnostics.Measure("ui_apply", applied.Sub(started), "query_id", value.generation, "results", len(value.update.Results))
		if first {
			a.diagnostics.Measure("first_applied", applied.Sub(value.started), "query_id", value.generation, "results", len(value.update.Results))
		}
	}
}

type measuredProvider struct {
	search.Provider
	r          *diag.Recorder
	generation int64
}

func (p measuredProvider) Search(ctx context.Context, q search.Query, limit int) ([]model.Result, error) {
	start := time.Now()
	results, err := p.Provider.Search(ctx, q, limit)
	status := diag.ErrorKind(err)
	if ctx.Err() != nil {
		status = diag.ErrorKind(ctx.Err())
	}
	p.r.Measure("provider_"+p.ID(), time.Since(start), "query_id", p.generation, "status", status, "results", len(results))
	return results, err
}

func (a *App) measuredProviders(providers []search.Provider, q search.Query, generation int64) []search.Provider {
	if a.diagnostics == nil {
		return providers
	}
	a.diagnostics.Logger.Debug("query_started", "query_id", generation, "scope", q.Scope, "length", utf8.RuneCountInString(q.Text))
	wrapped := make([]search.Provider, len(providers))
	for i, p := range providers {
		wrapped[i] = measuredProvider{p, a.diagnostics, generation}
	}
	return wrapped
}
