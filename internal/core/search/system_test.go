package search

import (
	"context"
	"strings"
	"testing"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/systemsearch"
)

func TestSystemProviderSupports(t *testing.T) {
	p := SystemProvider{}
	tests := []struct {
		query Query
		want  bool
	}{
		{Query{Text: "网络", Scope: ScopeAll}, true},
		{Query{Text: "", Scope: ScopeAll}, false},
		{Query{Text: "   ", Scope: ScopeAll}, false},
		{Query{Text: "网络", Scope: ScopeApplications}, false},
		{Query{Text: "网络", Scope: ScopeFiles}, false},
		{Query{Text: "网络", Scope: ScopeWindows}, false},
	}
	for _, tt := range tests {
		if got := p.Supports(tt.query); got != tt.want {
			t.Errorf("Supports(%+v) = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestSystemProviderSearch(t *testing.T) {
	p := SystemProvider{}
	ctx := context.Background()

	got, err := p.Search(ctx, Query{Text: "控制面板", Scope: ScopeAll}, 10)
	if err != nil || len(got) == 0 {
		t.Fatalf("控制面板 = %+v, %v", got, err)
	}
	first := got[0]
	if first.Entry.Title != "控制面板" || first.Entry.SourceID != "system" {
		t.Fatalf("first = %+v", first.Entry)
	}
	if first.Entry.Kind != model.Setting || first.Entry.LaunchTarget != "control.exe" {
		t.Fatalf("control panel entry = %+v", first.Entry)
	}
	if !strings.Contains(first.Entry.Subtitle, "控制面板") {
		t.Fatalf("subtitle = %q, want the category in it", first.Entry.Subtitle)
	}

	got, err = p.Search(ctx, Query{Text: "网络连接", Scope: ScopeAll}, 10)
	if err != nil || len(got) == 0 {
		t.Fatalf("网络连接 = %+v, %v", got, err)
	}
	first = got[0]
	if first.Entry.Title != "网络连接" || first.Entry.Kind != model.Setting ||
		first.Entry.LaunchTarget != "control.exe" || first.Entry.Arguments != "ncpa.cpl" {
		t.Fatalf("network connections entry = %+v", first.Entry)
	}

	got, err = p.Search(ctx, Query{Text: "相机", Scope: ScopeAll}, 1)
	if err != nil || len(got) != 1 || got[0].Entry.LaunchTarget != "ms-settings:privacy-webcam" {
		t.Fatalf("相机 = %+v, %v", got, err)
	}

	// A settings page is a protocol URI without arguments; apps.Launch opens it
	// with the shell's "open" verb.
	got, _ = p.Search(ctx, Query{Text: "蓝牙", Scope: ScopeAll}, 10)
	if len(got) == 0 || got[0].Entry.Kind != model.Setting ||
		got[0].Entry.LaunchTarget != "ms-settings:bluetooth" {
		t.Fatalf("蓝牙 = %+v", got)
	}
	if got[0].Entry.Arguments != "" {
		t.Fatalf("URI entry carries arguments: %+v", got[0].Entry)
	}

	if got, err := p.Search(ctx, Query{Text: "", Scope: ScopeAll}, 10); err != nil || len(got) != 0 {
		t.Fatalf("empty query = %+v, %v", got, err)
	}
	if got, err := p.Search(ctx, Query{Text: "zzzz", Scope: ScopeAll}, 10); err != nil || len(got) != 0 {
		t.Fatalf("miss = %+v, %v", got, err)
	}
}

func TestSystemProviderFillsResultsAfterSkippingUnavailableTarget(t *testing.T) {
	provider := SystemProvider{Resolve: func(target systemsearch.Target) (string, bool) {
		return `C:\Windows\System32\` + target.File, target.File != "wfs.exe" && target.File != "ms-settings:printers"
	}}
	results, err := provider.Search(t.Context(), Query{Text: "打印机", Scope: ScopeAll}, 1)
	if err != nil || len(results) != 1 || results[0].Entry.LaunchTarget != `C:\Windows\System32\control.exe` ||
		results[0].Entry.Arguments != "/name Microsoft.DevicesAndPrinters" {
		t.Fatalf("打印机 = %+v, %v", results, err)
	}
	results, err = provider.Search(t.Context(), Query{Text: "wfs", Scope: ScopeAll}, 10)
	if err != nil || len(results) != 0 {
		t.Fatalf("unavailable wfs.exe = %+v, %v", results, err)
	}
}

func TestSystemProviderIDsAreUnique(t *testing.T) {
	p := SystemProvider{}
	got, err := p.Search(context.Background(), Query{Text: "设置", Scope: ScopeAll}, 100)
	if err != nil || len(got) == 0 {
		t.Fatalf("设置 = %+v, %v", got, err)
	}
	seen := map[string]bool{}
	for _, result := range got {
		if seen[result.Entry.ID] {
			t.Errorf("duplicate id %q", result.Entry.ID)
		}
		seen[result.Entry.ID] = true
	}
}

func TestSystemEntryCommands(t *testing.T) {
	entry := model.Entry{ID: "s", Title: "显示设置", Kind: model.Setting, LaunchTarget: "ms-settings:display"}
	commands := CommandsFor(entry, false, false)
	if len(commands) != 1 || commands[0].Command != CommandOpen {
		t.Fatalf("commands = %+v, want only 打开", commands)
	}
	if commands[0].Title != "打开" {
		t.Errorf("open title = %q, want 打开", commands[0].Title)
	}
}
