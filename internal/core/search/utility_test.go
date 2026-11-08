package search

import (
	"context"
	"regexp"
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

func TestUtilityTools(t *testing.T) {
	cfg := settings.Default()
	p := UtilityProvider{Settings: func() settings.LauncherSettings { return cfg }}
	date, _ := p.Search(context.Background(), Query{Text: "date", Scope: ScopeAll}, 10)
	if len(date) != 1 || !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`).MatchString(date[0].Entry.Title) ||
		date[0].Entry.CopyText != date[0].Entry.Title {
		t.Fatalf("date = %+v", date)
	}
	if zh, _ := p.Search(context.Background(), Query{Text: "今天", Scope: ScopeAll}, 10); len(zh) != 1 || zh[0].Entry.ID != date[0].Entry.ID {
		t.Fatalf("今天 = %+v", zh)
	}
	clock, _ := p.Search(context.Background(), Query{Text: "time", Scope: ScopeAll}, 10)
	if len(clock) != 1 || !regexp.MustCompile(`^\d{2}:\d{2}:\d{2}$`).MatchString(clock[0].Entry.Title) {
		t.Fatalf("time = %+v", clock)
	}
	stamps, _ := p.Search(context.Background(), Query{Text: "timestamp", Scope: ScopeAll}, 10)
	if len(stamps) != 2 || len(stamps[0].Entry.CopyText) < 10 || len(stamps[1].Entry.CopyText) < 13 {
		t.Fatalf("timestamp = %+v", stamps)
	}
	uuid, _ := p.Search(context.Background(), Query{Text: "uuid", Scope: ScopeAll}, 10)
	if len(uuid) != 1 || !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(uuid[0].Entry.Title) {
		t.Fatalf("uuid = %+v", uuid)
	}
	if again, _ := p.Search(context.Background(), Query{Text: "uuid", Scope: ScopeAll}, 10); again[0].Entry.Title == uuid[0].Entry.Title {
		t.Fatal("the uuid did not change between searches")
	}
	// The tools belong to the unscoped query; "web" scope keeps searching.
	if web, _ := p.Search(context.Background(), Query{Text: "date", Scope: ScopeWeb}, 10); len(web) != 1 || web[0].Entry.Kind != model.URL {
		t.Fatalf("web scope leaked into the tools: %+v", web)
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
