package search

import (
	"context"
	"sync/atomic"
	"testing"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
)

func editTool(cfg *settings.LauncherSettings, id string, edit func(*settings.ToolOptions)) {
	for i := range cfg.Tools {
		if cfg.Tools[i].ID == id {
			edit(&cfg.Tools[i])
		}
	}
}

func TestFixedPrefixIsRequiredAndConfigurable(t *testing.T) {
	cfg := settings.Default()
	p := UtilityProvider{Settings: func() settings.LauncherSettings { return cfg }}
	for _, text := range []string{"1+2", "2026-11-10", "123", "calc 1+2"} {
		results, err := p.Search(context.Background(), ParseQuery(text), 10)
		if err != nil || len(results) != 0 {
			t.Fatalf("unprefixed %q produced results: %+v %v", text, results, err)
		}
	}
	for _, text := range []string{"=1+2", "= 1+2", "tool calc 1+2"} {
		results, err := p.Search(context.Background(), ParseQuery(text), 10)
		if err != nil || len(results) != 1 || results[0].Entry.CopyText != "3" {
			t.Fatalf("%q: %+v %v", text, results, err)
		}
	}
	editTool(&cfg, "calc", func(tool *settings.ToolOptions) { tool.Prefix = "@@" })
	if got, _ := p.Search(context.Background(), ParseQuery("=1+2"), 10); len(got) != 0 {
		t.Fatal("old prefix still executed")
	}
	if got, _ := p.Search(context.Background(), ParseQuery("@@1+2"), 10); len(got) != 1 || got[0].Entry.CopyText != "3" {
		t.Fatalf("custom prefix did not execute: %+v", got)
	}
}

func TestToolControlsPreventExecution(t *testing.T) {
	cfg := settings.Default()
	p := UtilityProvider{Settings: func() settings.LauncherSettings { return cfg }}
	editTool(&cfg, "uuid", func(tool *settings.ToolOptions) { tool.Keywords = []string{"uid"} })
	if got, _ := p.Search(context.Background(), ParseQuery("uuid"), 10); len(got) != 0 {
		t.Fatal("removed alias still generated a UUID")
	}
	if got, _ := p.Search(context.Background(), ParseQuery("uid"), 10); len(got) == 0 {
		t.Fatal("new alias did not execute")
	}
	editTool(&cfg, "uuid", func(tool *settings.ToolOptions) { tool.GlobalQuery = false })
	if got, _ := p.Search(context.Background(), ParseQuery("uid"), 10); len(got) != 0 {
		t.Fatal("tool excluded from normal search executed")
	}
	if got, _ := p.Search(context.Background(), ParseQuery("tool uid"), 10); len(got) == 0 {
		t.Fatal("explicit call stopped working")
	}
	editTool(&cfg, "uuid", func(tool *settings.ToolOptions) { tool.Enabled = false })
	if got, err := p.Search(context.Background(), ParseQuery("tool uid"), 10); len(got) != 0 || err == nil {
		t.Fatal("disabled explicit tool executed or did not explain why")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := p.Search(ctx, ParseQuery("=1+2"), 10); len(got) != 0 || err == nil {
		t.Fatal("canceled query executed")
	}
}

type countingSource struct{ calls atomic.Int32 }

func (*countingSource) ID() string            { return "external" }
func (*countingSource) Supports(q Query) bool { return q.Scope == ScopeAll || q.Scope == ScopeFiles }
func (s *countingSource) Search(context.Context, Query, int) ([]model.Result, error) {
	s.calls.Add(1)
	return nil, nil
}

func TestExplicitToolDoesNotDispatchExternalSources(t *testing.T) {
	cfg := settings.Default()
	external := &countingSource{}
	p := UtilityProvider{Settings: func() settings.LauncherSettings { return cfg }}
	for _, text := range []string{"=1+2", "tool uuid", "uuid", "week"} {
		q := RouteTools(ParseQuery(text), cfg)
		if q.Scope != ScopeTools {
			t.Fatalf("%q was not routed to a tool", text)
		}
		for batch := range Run(context.Background(), []Provider{external, p}, q, 10) {
			if batch.SourceID != "utilities" || batch.Error != "" || len(batch.Results) == 0 {
				t.Fatalf("unexpected dispatch: %+v", batch)
			}
		}
	}
	if external.calls.Load() != 0 {
		t.Fatal("tool commands queried external providers")
	}
	for _, text := range []string{"file uuid", "app =1+2", "web uuid", ">uuid", "uuidocument"} {
		q := RouteTools(ParseQuery(text), cfg)
		if q.Scope == ScopeTools {
			t.Fatalf("explicit scope or non-keyword was consumed: %q", text)
		}
	}
}

func TestAutomaticRecognitionIsOptIn(t *testing.T) {
	cfg := settings.Default()
	p := UtilityProvider{Settings: func() settings.LauncherSettings { return cfg }}
	for _, text := range []string{"10kg lb", "#36a", "2+2"} {
		if got, _ := p.Search(context.Background(), ParseQuery(text), 10); len(got) != 0 {
			t.Fatalf("automatic tool executed by default: %q", text)
		}
	}
	editTool(&cfg, "calc", func(tool *settings.ToolOptions) { tool.Prefix = ""; tool.GlobalQuery = true })
	if got, _ := p.Search(context.Background(), ParseQuery("2+2"), 10); len(got) != 1 {
		t.Fatal("opt-in calculation failed")
	}
	if got, err := p.Search(context.Background(), ParseQuery("tool calc 2+2"), 10); err != nil || len(got) != 1 {
		t.Fatalf("explicit tool failed: %+v %v", got, err)
	}
	// The removed tools say so instead of answering.
	for _, text := range []string{"tool convert 10kg lb", "tool color #36a", "tool b64 hi"} {
		if got, err := p.Search(context.Background(), ParseQuery(text), 10); err == nil || len(got) != 0 {
			t.Fatalf("removed tool answered: %q %+v", text, got)
		}
	}
	if got, err := p.Search(context.Background(), ParseQuery("tool"), 5); err != nil || len(got) != 5 {
		t.Fatalf("tool menu limit: %+v %v", got, err)
	}
}

func BenchmarkToolRouting(b *testing.B) {
	cfg := settings.Default()
	for _, text := range []string{"Visual Studio Code", "=1+2", "uuid", "file report"} {
		b.Run(text, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				RouteTools(ParseQuery(text), cfg)
			}
		})
	}
}
