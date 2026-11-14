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
	chinese := fmt.Sprintf("%d年%d月%d日", today.Year(), today.Month(), today.Day())
	wantDates := []string{today.Format("2006-01-02"), today.Format("2006/01/02"), today.Format("20060102"),
		chinese, chinese + " " + chineseWeekdays[today.Weekday()], today.Format("Monday, January 2, 2006")}
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
		[]string{"2026-02-28", "2026/02/28", "20260228", "2026年2月28日", "2026年2月28日 星期六", "Saturday, February 28, 2026"}) {
		t.Fatalf("date math formats = %q", titles(shifted))
	}
	clock, _ := p.Search(context.Background(), Query{Text: "time", Scope: ScopeAll}, 10)
	for i, pattern := range []string{`^\d{2}:\d{2}:\d{2}$`, `^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`, `^\d{2}:\d{2}$`,
		`^\d{1,2}:\d{2}:\d{2} [AP]M$`, `^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}$`, `.`, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`} {
		if len(clock) != 7 || !regexp.MustCompile(pattern).MatchString(clock[i].Entry.Title) {
			t.Fatalf("time row %d = %q", i, titles(clock))
		}
	}
	if _, err := time.Parse(time.RFC3339, clock[5].Entry.Title); err != nil {
		t.Fatalf("iso time %q: %v", clock[5].Entry.Title, err)
	}
	stamps, _ := p.Search(context.Background(), Query{Text: "timestamp", Scope: ScopeAll}, 10)
	if len(stamps) != 2 || len(stamps[0].Entry.CopyText) < 10 || len(stamps[1].Entry.CopyText) < 13 {
		t.Fatalf("timestamp = %+v", stamps)
	}
	uuid, _ := p.Search(context.Background(), Query{Text: "uuid", Scope: ScopeAll}, 10)
	if len(uuid) != 5 || !uuidRe.MatchString(uuid[0].Entry.Title) {
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
	// Several UUIDs copy one per line, in every format.
	batch, _ := p.Search(context.Background(), Query{Text: "uuid 3", Scope: ScopeAll}, 10)
	if len(batch) != 5 {
		t.Fatalf("uuid 3 = %+v", batch)
	}
	lines := strings.Split(batch[0].Entry.CopyText, "\r\n")
	if len(lines) != 3 || lines[0] == lines[1] || !uuidRe.MatchString(lines[2]) || !strings.HasPrefix(batch[0].Entry.Title, lines[0]) {
		t.Fatalf("uuid 3 copies %q", batch[0].Entry.CopyText)
	}
	if got := strings.Split(batch[1].Entry.CopyText, "\r\n"); len(got) != 3 || got[2] != strings.ReplaceAll(lines[2], "-", "") {
		t.Fatalf("uuid 3 without hyphens copies %q", got)
	}
	for _, text := range []string{"uuid 0", "uuid 21", "uuid x"} {
		if got, _ := p.Search(context.Background(), Query{Text: text, Scope: ScopeAll}, 10); len(got) != 0 {
			t.Fatalf("%s = %+v", text, got)
		}
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

	// The removed tools no longer answer: the developer tools plugin has
	// the encodings and colors, and their keywords are its own again.
	for _, text := range []string{"10kg lb", "b64 hi", "urlencode a b", "#36a", "rgb 51, 102, 170"} {
		if got := search(text); len(got) != 0 {
			t.Fatalf("%q still answers: %+v", text, got)
		}
	}
	// A full URL still opens.
	if got := search("https://example.com/x"); len(got) != 1 || got[0].Entry.Kind != model.URL {
		t.Fatalf("url open = %+v", got)
	}

	// Date math, from a date or from today, several steps at once.
	today := time.Now()
	for text, want := range map[string]string{
		"date 2026-01-31 +1d":     "2026-02-01",
		"date 2026-01-31 +1m":     "2026-02-28",
		"date 2026/01/31 +1y -1w": "2027-01-24",
		"date 20260131 +2w":       "2026-02-14",
		"date +3d":                today.AddDate(0, 0, 3).Format("2006-01-02"),
		"date tomorrow":           today.AddDate(0, 0, 1).Format("2006-01-02"),
	} {
		if got := search(text); len(got) == 0 || got[0].Entry.Title != want {
			t.Fatalf("%s = %q, want %s", text, titles(got), want)
		}
	}
	for _, text := range []string{"date 2026-02-31", "date 2026-01-31 +1x"} {
		if got := search(text); len(got) != 0 {
			t.Fatalf("%s matched: %+v", text, got)
		}
	}

	// Days from today, between two dates, and in weeks.
	if got := search("days " + today.AddDate(0, 0, 1).Format("2006-01-02")); len(got) != 1 || got[0].Entry.Title != "1" {
		t.Fatalf("days tomorrow = %+v", got)
	}
	if got := search("days 2026-01-01 2026-03-01"); len(got) != 2 || got[0].Entry.Title != "59" ||
		got[1].Entry.Title != locales.FormatWeeksAndDays(8, 3) {
		t.Fatalf("days between = %q", titles(got))
	}
	if got := search("days 2026-03-01 2026-01-01"); len(got) == 0 || got[0].Entry.Title != "-59" {
		t.Fatalf("days backwards = %q", titles(got))
	}

	// The ISO week of today and of a date, with its Monday to Sunday.
	if got := search("week"); len(got) != 3 || !regexp.MustCompile(`^\d{4}-W\d{2}$`).MatchString(got[0].Entry.Title) {
		t.Fatalf("week = %+v", got)
	}
	if got := search("week 2026-01-01"); !slices.Equal(titles(got), []string{"2026-W01", "1", "2025-12-29 ~ 2026-01-04"}) {
		t.Fatalf("week of a date = %q", titles(got))
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
	pass := search("pass 12")
	if len(pass) != 3 {
		t.Fatalf("pass 12 = %+v", pass)
	}
	for i, pattern := range []string{`^[A-Za-z0-9!@#$%^&*_-]{12}$`, `^[A-Za-z0-9]{12}$`, `^[0-9]{12}$`} {
		if !regexp.MustCompile(pattern).MatchString(pass[i].Entry.CopyText) {
			t.Fatalf("pass row %d = %q", i, pass[i].Entry.CopyText)
		}
	}
	if got := search("pass 3"); len(got) != 0 {
		t.Fatalf("too-short password accepted: %+v", got)
	}
}

// TestTimestampTool converts both ways in a fixed time zone.
func TestTimestampTool(t *testing.T) {
	zone := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 11, 14, 18, 30, 0, 0, zone)
	if got := timestampTool("", now); !slices.Equal(titles(got), []string{"1794652200", "1794652200000"}) {
		t.Fatalf("now = %q", titles(got))
	}
	if got := timestampTool("1794652200", now); !slices.Equal(titles(got),
		[]string{"2026-11-14 18:30:00", "2026-11-14 10:30:00", "2026-11-14T18:30:00+08:00", "1794652200000"}) ||
		got[0].Entry.Subtitle != locales.FormatLocalTime("UTC+08:00") {
		t.Fatalf("seconds = %q", titles(got))
	}
	if got := timestampTool("1794652200123", now); !slices.Equal(titles(got),
		[]string{"2026-11-14 18:30:00.123", "2026-11-14 10:30:00.123", "2026-11-14T18:30:00.123+08:00", "1794652200"}) {
		t.Fatalf("milliseconds = %q", titles(got))
	}
	for _, text := range []string{"2026-11-14 18:30", "2026/11/14 18:30:00", "2026-11-14T10:30:00Z"} {
		if got := timestampTool(text, now); !slices.Equal(titles(got), []string{"1794652200", "1794652200000"}) {
			t.Fatalf("%s = %q", text, titles(got))
		}
	}
	// The time tool converts a timestamp it is given the same way.
	if got := utilityTools("time 1794652200"); len(got) != 4 || got[3].Entry.Title != "1794652200000" {
		t.Fatalf("time 1794652200 = %q", titles(got))
	}
	if got := timestampTool("soon", now); got != nil {
		t.Fatalf("soon = %q", titles(got))
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

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// TestReroll keeps what the user asked for: a UUID rerolls in all its
// format rows together, a password only in the selected row, and the other
// rows stay as they are.
func TestReroll(t *testing.T) {
	results := append(UUIDResults("62152892-92c1-425f-b32e-075f31040bde", "0b9f2b8e-7c38-4f0e-9a51-1d2b3c4d5e6f"),
		model.Result{Entry: model.Entry{ID: "rand:int:7~9", Title: "8", CopyText: "8", SourceID: "utilities"}})
	if !Reroll(results, 2) {
		t.Fatal("nothing rerolled")
	}
	lines := strings.Split(results[0].Entry.CopyText, "\r\n")
	if len(lines) != 2 || lines[0] == "62152892-92c1-425f-b32e-075f31040bde" || !uuidRe.MatchString(lines[1]) {
		t.Fatalf("rerolled uuids = %q", lines)
	}
	if want := titles(UUIDResults(lines...)); !slices.Equal(titles(results[:5]), want) {
		t.Fatalf("rerolled formats = %q, want %q", titles(results[:5]), want)
	}
	if results[5].Entry.Title != "8" {
		t.Fatalf("a non-uuid row changed: %+v", results[5].Entry)
	}
	for range 20 {
		if !Reroll(results, 5) {
			t.Fatal("rand did not reroll")
		}
		if n, _ := strconv.Atoi(results[5].Entry.Title); n < 7 || n > 9 || results[5].Entry.CopyText != results[5].Entry.Title {
			t.Fatalf("rand left 7~9: %+v", results[5].Entry)
		}
	}

	pass := passTool("10")
	before := titles(pass)
	if !Reroll(pass, 2) || pass[2].Entry.Title == before[2] || !regexp.MustCompile(`^[0-9]{10}$`).MatchString(pass[2].Entry.Title) {
		t.Fatalf("pin reroll = %q", titles(pass))
	}
	if pass[0].Entry.Title != before[0] || pass[1].Entry.Title != before[1] {
		t.Fatal("rerolling one password changed the others")
	}
	if Reroll([]model.Result{{Entry: model.Entry{ID: "calc:1+1", SourceID: "utilities"}}}, 0) {
		t.Fatal("the calculator rerolled")
	}
}
