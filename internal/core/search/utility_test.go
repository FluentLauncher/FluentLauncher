package search

import (
	"context"
	"testing"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
)

func TestEvaluate(t *testing.T) {
	tests := []struct {
		text string
		want float64
		ok   bool
	}{
		{"1+2*3", 7, true},
		{"(1+2)^2", 9, true},
		{"10%4", 2, true},
		{"2*-3", -6, true},
		{"1/0", 0, false},
		{"abc", 0, false},
		{"1+", 0, false},
		{"1+2*3", 7, true},
	}
	for _, tt := range tests {
		got, ok := Evaluate(tt.text)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("Evaluate(%q) = %v, %v; want %v, %v", tt.text, got, ok, tt.want, tt.ok)
		}
	}
}

func TestUtilityProvider(t *testing.T) {
	cfg := settings.Default()
	p := UtilityProvider{Settings: func() settings.LauncherSettings { return cfg }}
	got, err := p.Search(context.Background(), Query{Text: "2+2", Scope: ScopeAll}, 10)
	if err != nil || len(got) == 0 || got[0].Entry.Kind != model.Calculation || got[0].Entry.CopyText != "4" {
		t.Fatalf("calculator = %+v, %v", got, err)
	}
	got, _ = p.Search(context.Background(), Query{Text: "https://example.com/a%20b", Scope: ScopeAll}, 10)
	if len(got) == 0 || got[0].Entry.Kind != model.URL {
		t.Fatalf("url = %+v", got)
	}
	got, _ = p.Search(context.Background(), Query{Text: "hello", Scope: ScopeWeb}, 10)
	if len(got) == 0 || got[0].Entry.LaunchTarget != "https://www.bing.com/search?q=hello" {
		t.Fatalf("web = %+v", got)
	}
}

func TestSiteProvider(t *testing.T) {
	cfg := settings.Default()
	cfg.WebSites = []settings.WebSearchSite{{Keyword: "gh", Name: "GitHub", URLTemplate: "https://github.com/search?q={query}", Enabled: true}}
	p := SiteSearchProvider{Settings: func() settings.LauncherSettings { return cfg }}
	got, _ := p.Search(context.Background(), Query{Text: "gh go lang", Scope: ScopeAll}, 10)
	if len(got) == 0 || got[0].Entry.LaunchTarget != "https://github.com/search?q=go%20lang" {
		t.Fatalf("site = %+v", got)
	}
	if got, _ := p.Search(context.Background(), Query{Text: "gh", Scope: ScopeAll}, 10); len(got) != 0 {
		t.Fatalf("keyword alone triggered: %+v", got)
	}
}

func TestParseQuery(t *testing.T) {
	tests := []struct {
		text string
		want Query
	}{
		{"win", Query{Scope: ScopeWindows}},
		{"win notepad", Query{Text: "notepad", Scope: ScopeWindows}},
		{"window", Query{Text: "window", Scope: ScopeAll}},
		{"file report", Query{Text: "report", Scope: ScopeFiles}},
		{">echo", Query{Text: "echo", Scope: ScopePlugins}},
	}
	for _, tt := range tests {
		if got := ParseQuery(tt.text); got != tt.want {
			t.Errorf("ParseQuery(%q) = %+v, want %+v", tt.text, got, tt.want)
		}
	}
}

func TestCommands(t *testing.T) {
	entry := model.Entry{ID: "x", Title: "x", Kind: model.Application, LaunchTarget: `C:\Windows\notepad.exe`}
	commands := CommandsFor(entry, false, true)
	if len(commands) < 6 {
		t.Fatalf("commands = %+v", commands)
	}
	url := model.Entry{ID: "u", Title: "u", Kind: model.URL, LaunchTarget: "https://example.com"}
	commands = CommandsFor(url, false, false)
	for _, command := range commands {
		if command.Command == CommandRunAsAdministrator || command.Command == CommandSetAlias {
			t.Fatalf("URL got inapplicable command: %+v", commands)
		}
	}
}
