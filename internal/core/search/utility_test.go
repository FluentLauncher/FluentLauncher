package search

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/locales"
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
	got, err := p.Search(context.Background(), Query{Text: "=2+2", Scope: ScopeAll}, 10)
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
	today := time.Now()
	wantDates := []string{today.Format("2006-01-02"), today.Format("2006/01/02"), today.Format("20060102"),
		fmt.Sprintf("%d年%d月%d日", today.Year(), today.Month(), today.Day())}
	if got := titles(date); !slices.Equal(got, wantDates) {
		t.Fatalf("date formats = %q, want %q", got, wantDates)
	}
	for _, row := range date {
		if row.Entry.CopyText != row.Entry.Title {
			t.Fatalf("date row copies something else than it shows: %+v", row)
		}
	}
	if zh, _ := p.Search(context.Background(), Query{Text: "今天", Scope: ScopeAll}, 10); len(zh) != len(date) || zh[0].Entry.ID != date[0].Entry.ID {
		t.Fatalf("今天 = %+v", zh)
	}
	if shifted, _ := p.Search(context.Background(), Query{Text: "date 2026-01-31 +1m", Scope: ScopeAll}, 10); !slices.Equal(titles(shifted),
		[]string{"2026-02-28", "2026/02/28", "20260228", "2026年2月28日"}) {
		t.Fatalf("date math formats = %q", titles(shifted))
	}
	clock, _ := p.Search(context.Background(), Query{Text: "time", Scope: ScopeAll}, 10)
	if len(clock) != 4 || !regexp.MustCompile(`^\d{2}:\d{2}:\d{2}$`).MatchString(clock[0].Entry.Title) ||
		!regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`).MatchString(clock[1].Entry.Title) ||
		!regexp.MustCompile(`^\d{2}:\d{2}$`).MatchString(clock[2].Entry.Title) {
		t.Fatalf("time = %q", titles(clock))
	}
	if _, err := time.Parse(time.RFC3339, clock[3].Entry.Title); err != nil {
		t.Fatalf("iso time %q: %v", clock[3].Entry.Title, err)
	}
	stamps, _ := p.Search(context.Background(), Query{Text: "timestamp", Scope: ScopeAll}, 10)
	if len(stamps) != 2 || len(stamps[0].Entry.CopyText) < 10 || len(stamps[1].Entry.CopyText) < 13 {
		t.Fatalf("timestamp = %+v", stamps)
	}
	uuid, _ := p.Search(context.Background(), Query{Text: "uuid", Scope: ScopeAll}, 10)
	if len(uuid) != 5 || !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(uuid[0].Entry.Title) {
		t.Fatalf("uuid = %+v", uuid)
	}
	canonical := uuid[0].Entry.Title
	plain := strings.ReplaceAll(canonical, "-", "")
	if want := []string{canonical, plain, strings.ToUpper(canonical), strings.ToUpper(plain), "{" + strings.ToUpper(canonical) + "}"}; !slices.Equal(titles(uuid), want) {
		t.Fatalf("uuid formats = %q, want %q", titles(uuid), want)
	}
	if again, _ := p.Search(context.Background(), Query{Text: "uuid", Scope: ScopeAll}, 10); again[0].Entry.Title == uuid[0].Entry.Title {
		t.Fatal("the uuid did not change between searches")
	}
	// Merge keeps the formats in order: each row ranks just below the one
	// before it, so the canonical form stays the default selection.
	if merged := Merge([]Batch{{Results: uuid}}, 10); !slices.Equal(titles(merged), titles(uuid)) {
		t.Fatalf("merged uuid order = %q", titles(merged))
	}
	// The tools belong to the unscoped query; "web" scope keeps searching.
	if web, _ := p.Search(context.Background(), Query{Text: "date", Scope: ScopeWeb}, 10); len(web) != 1 || web[0].Entry.Kind != model.URL {
		t.Fatalf("web scope leaked into the tools: %+v", web)
	}
}

func TestUtilityToolsExtended(t *testing.T) {
	cfg := settings.Default()
	// Automatic recognition of conversion/color is an explicit preference.
	for i := range cfg.Tools {
		if cfg.Tools[i].ID == "convert" || cfg.Tools[i].ID == "color" {
			cfg.Tools[i].GlobalQuery = true
		}
	}
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
	if got := search("date 2026-01-31 +1d"); len(got) == 0 || got[0].Entry.Title != "2026-02-01" {
		t.Fatalf("date +1d = %+v", got)
	}
	if got := search("date 2026-01-31 +1m"); len(got) == 0 || got[0].Entry.Title != "2026-02-28" {
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

func TestToolCommands(t *testing.T) {
	uuid := model.Entry{ID: "uuid:plain:62152892-92c1-425f-b32e-075f31040bde", Title: "6215289292c1425fb32e075f31040bde",
		Kind: model.Calculation, CopyText: "6215289292c1425fb32e075f31040bde", SourceID: "utilities"}
	regen := false
	for _, command := range CommandsFor(uuid, false, false) {
		if command.Command == CommandRegenerate {
			regen = true
		}
	}
	if !regen {
		t.Fatal("the uuid row offers no regenerate")
	}

	// Every date row, whatever its format, offers the ISO date's timestamp.
	date := model.Entry{ID: "date:zh:2026-11-08", Title: "2026年11月8日", Kind: model.Calculation, CopyText: "2026年11月8日", SourceID: "utilities"}
	values := map[string]string{}
	for _, command := range CommandsFor(date, false, false) {
		if command.Command == CommandCopyValue {
			values[command.Title] = command.Value
		}
	}
	want := strconv.FormatInt(time.Date(2026, 11, 8, 0, 0, 0, 0, time.Local).Unix(), 10)
	if got := values[locales.CmdCopyTimestamp()]; got != want {
		t.Fatalf("timestamp copy = %q, want %q", got, want)
	}

	color := model.Entry{ID: "color:#3366AA", Title: "#3366AA", Kind: model.Calculation, CopyText: "#3366AA", SourceID: "utilities"}
	commands := CommandsFor(color, false, false)
	values = map[string]string{}
	for _, command := range commands {
		if command.Command == CommandCopyValue {
			values[command.Title] = command.Value
		}
	}
	if got := values[locales.CmdCopyRGB()]; got != "rgb(51, 102, 170)" {
		t.Fatalf("rgb copy = %q", got)
	}
	if got := values[locales.CmdCopyHSL()]; got != "hsl(214, 54%, 43%)" {
		t.Fatalf("hsl copy = %q", got)
	}

	// The calculator keeps its single plain copy command.
	calc := model.Entry{ID: "calc:2+2", Title: "4", Kind: model.Calculation, CopyText: "4", SourceID: "utilities"}
	if commands := CommandsFor(calc, false, false); len(commands) != 1 || commands[0].Command != CommandOpen {
		t.Fatalf("calc commands = %+v", commands)
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

func titles(results []model.Result) []string {
	out := make([]string, len(results))
	for i, result := range results {
		out[i] = result.Entry.Title
	}
	return out
}

// TestRefreshUUIDs rerolls every UUID row together, each in its own format,
// and leaves the other rows alone.
func TestRefreshUUIDs(t *testing.T) {
	old := "62152892-92c1-425f-b32e-075f31040bde"
	results := append(UUIDResults(old), model.Result{Entry: model.Entry{ID: "rand:1-9:5", Title: "5", CopyText: "5"}})
	if !RefreshUUIDs(results) {
		t.Fatal("nothing refreshed")
	}
	canonical := results[0].Entry.Title
	if canonical == old {
		t.Fatal("the uuid did not change")
	}
	if want := titles(UUIDResults(canonical)); !slices.Equal(titles(results[:5]), want) {
		t.Fatalf("refreshed formats = %q, want %q", titles(results[:5]), want)
	}
	for _, row := range results[:5] {
		if row.Entry.CopyText != row.Entry.Title || !strings.HasSuffix(row.Entry.ID, ":"+canonical) {
			t.Fatalf("refreshed row is inconsistent: %+v", row.Entry)
		}
	}
	if results[5].Entry.Title != "5" {
		t.Fatalf("a non-uuid row changed: %+v", results[5].Entry)
	}
}
