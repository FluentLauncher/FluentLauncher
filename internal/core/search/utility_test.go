package search

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

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

func TestUtilityToolsExtended(t *testing.T) {
	cfg := settings.Default()
	p := UtilityProvider{Settings: func() settings.LauncherSettings { return cfg }}
	search := func(text string) []model.Result {
		got, err := p.Search(context.Background(), Query{Text: text, Scope: ScopeAll}, 10)
		if err != nil {
			t.Fatalf("%q: %v", text, err)
		}
		return got
	}

	// Unit conversion, including temperature and Chinese unit names.
	if got := search("10kg lb"); len(got) != 1 || got[0].Entry.Title != "22.0462 lb" || got[0].Entry.CopyText != "22.0462" {
		t.Fatalf("10kg lb = %+v", got)
	}
	if got := search("32f c"); len(got) != 1 || got[0].Entry.Title != "0 c" {
		t.Fatalf("32f c = %+v", got)
	}
	if got := search("1公里 m"); len(got) != 1 || got[0].Entry.Title != "1000 m" {
		t.Fatalf("1公里 m = %+v", got)
	}
	if got := search("10kg c"); len(got) != 0 {
		t.Fatalf("cross-dimension conversion matched: %+v", got)
	}

	// Encodings.
	if got := search("b64 hi"); len(got) != 1 || got[0].Entry.CopyText != "aGk=" {
		t.Fatalf("b64 hi = %+v", got)
	}
	if got := search("unb64 aGk="); len(got) != 1 || got[0].Entry.CopyText != "hi" {
		t.Fatalf("unb64 = %+v", got)
	}
	if got := search("unb64 !!!"); len(got) != 0 {
		t.Fatalf("bad base64 matched: %+v", got)
	}
	if got := search("urlencode a b"); len(got) != 1 || got[0].Entry.CopyText != "a%20b" {
		t.Fatalf("urlencode = %+v", got)
	}
	if got := search("urldecode a%20b"); len(got) != 1 || got[0].Entry.CopyText != "a b" {
		t.Fatalf("urldecode = %+v", got)
	}
	// A full URL still opens instead of being "encoded".
	if got := search("https://example.com/x"); len(got) != 1 || got[0].Entry.Kind != model.URL {
		t.Fatalf("url open = %+v", got)
	}

	// Date math, days and week.
	if got := search("date 2026-01-31 +1d"); len(got) != 1 || got[0].Entry.Title != "2026-02-01" {
		t.Fatalf("date +1d = %+v", got)
	}
	if got := search("date 2026-01-31 +1m"); len(got) != 1 || got[0].Entry.Title != "2026-02-28" {
		t.Fatalf("date +1m clamps to the shorter month: %+v", got)
	}
	if got := search("date 2026-02-31"); len(got) != 0 {
		t.Fatalf("impossible date matched: %+v", got)
	}
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	if got := search("days " + tomorrow); len(got) != 1 || got[0].Entry.Title != "1" {
		t.Fatalf("days tomorrow = %+v", got)
	}
	if got := search("week"); len(got) != 1 || !regexp.MustCompile(`^\d{4}-W\d{2}$`).MatchString(got[0].Entry.Title) {
		t.Fatalf("week = %+v", got)
	}

	// Random values.
	if got := search("rand 5-5"); len(got) != 1 || got[0].Entry.Title != "5" {
		t.Fatalf("rand 5-5 = %+v", got)
	}
	if got := search("rand 1-1000"); len(got) != 1 {
		t.Fatalf("rand = %+v", got)
	} else if n, _ := strconv.Atoi(got[0].Entry.Title); n < 1 || n > 1000 {
		t.Fatalf("rand out of range: %v", got)
	}
	if got := search("pass 8"); len(got) != 1 || len(got[0].Entry.CopyText) != 8 {
		t.Fatalf("pass 8 = %+v", got)
	}
	if got := search("pass 3"); len(got) != 0 {
		t.Fatalf("too-short password accepted: %+v", got)
	}

	// Colors.
	if got := search("#36a"); len(got) != 1 || got[0].Entry.Title != "#3366AA" {
		t.Fatalf("#36a = %+v", got)
	}
	if got := search("rgb 51, 102, 170"); len(got) != 1 || got[0].Entry.Title != "#3366AA" ||
		!strings.Contains(got[0].Entry.Subtitle, "51, 102, 170") {
		t.Fatalf("rgb = %+v", got)
	}
	if got := search("rgb 300, 0, 0"); len(got) != 0 {
		t.Fatalf("out-of-range rgb accepted: %+v", got)
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
