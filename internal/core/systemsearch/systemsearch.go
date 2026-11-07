// Package systemsearch is a curated catalog of Windows settings pages, Control
// Panel items and system tools together with the matcher the launcher uses to
// find them by Chinese title, English name, synonym or initial.
//
// The package is pure Go: it never imports Windows APIs, the UI or storage, so
// the catalog and the matching rules stay testable on any platform. Opening the
// target is the caller's business; an entry only records how Windows expects to
// be asked.
package systemsearch

import (
	"sort"
	"strings"
	"unicode"

	gopinyin "github.com/mozillazg/go-pinyin"
)

// Target is how Windows opens one entry: a protocol URI such as
// "ms-settings:display", a program with fixed arguments such as
// control.exe /name Microsoft.NetworkConnections, or a *.cpl / *.msc module.
type Target struct {
	// File is the URI, executable or module to start.
	File string
	// Arguments are passed to File; they are empty for URIs.
	Arguments string
	// URI marks File as a protocol URI instead of a program or module.
	URI bool
}

// Display renders the target the way the result subtitle shows it.
func (t Target) Display() string {
	if t.Arguments == "" {
		return t.File
	}
	return t.File + " " + t.Arguments
}

// Entry is one Windows settings page or control panel item.
type Entry struct {
	// Title is the Chinese title the result row shows.
	Title string
	// Latin is the English name or alias, so "display" finds 显示设置.
	Latin string
	// Category groups the entry: 系统, 显示, 声音, 网络, …
	Category string
	// Keywords are the Chinese synonyms, English words and abbreviations.
	Keywords []string
	// Target is how Windows opens the item.
	Target Target
}

// Match is one search hit. Complete records whether every query token matched;
// partial hits always rank behind complete ones.
type Match struct {
	Entry    Entry
	Score    int
	Complete bool
}

// All returns the catalog. The slice is a copy, so callers cannot reorder or
// shorten the package's own data, but the Keyword slices are shared.
func All() []Entry {
	out := make([]Entry, len(catalog))
	copy(out, catalog)
	return out
}

// Search matches query against the catalog and returns at most limit matches,
// best first. An empty query returns nothing: the launcher only shows these
// entries for a real search, never as a default list.
//
// Tokens are split on whitespace. Every token must match for a complete hit;
// an entry that matches only a minority of the tokens is dropped, so that
// "网络 打印机" does not drown the printer entries in every network one.
func Search(query string, limit int) []Match {
	if limit <= 0 {
		return nil
	}
	tokens := strings.Fields(normalize(query))
	if len(tokens) == 0 {
		return nil
	}
	out := make([]Match, 0, 16)
	for i := range index {
		entry := &index[i]
		match, ok := entry.match(tokens)
		if !ok {
			continue
		}
		out = append(out, match)
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Complete != out[b].Complete {
			return out[a].Complete
		}
		if out[a].Score != out[b].Score {
			return out[a].Score > out[b].Score
		}
		// Same band: the shorter title is the tighter match, so "声音设置"
		// beats "声音控制面板" for the query "声音".
		la, lb := len([]rune(out[a].Entry.Title)), len([]rune(out[b].Entry.Title))
		if la != lb {
			return la < lb
		}
		if out[a].Entry.Category != out[b].Entry.Category {
			return out[a].Entry.Category < out[b].Entry.Category
		}
		return out[a].Entry.Title < out[b].Entry.Title
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Match bands. A band is wide enough that a hit in a stronger field beats any
// combination of weaker ones, which keeps "显示设置" above an entry that only
// mentions 设置 in its keywords.
const (
	titleExact       = 10000
	titlePrefix      = 9000
	titleContains    = 8000
	keywordExact     = 7800
	keywordPrefix    = 7200
	keywordContains  = 6600
	latinExact       = 7400
	latinPrefix      = 6800
	latinContains    = 6200
	initialsExact    = 5400
	initialsPrefix   = 4800
	acronymExact     = 5000
	acronymPrefix    = 4400
	categoryContains = 1200
)

// indexed is one catalog entry with the lowercase forms and phonetics that the
// matcher needs, computed once at startup instead of on every keystroke.
type indexed struct {
	entry    Entry
	title    string
	latin    string
	category string
	keywords []string
	initials string
	acronym  string
}

func (i *indexed) match(tokens []string) (Match, bool) {
	match := Match{Entry: i.entry}
	hits := 0
	for _, token := range tokens {
		score := i.tokenScore(token)
		if score == 0 {
			continue
		}
		hits++
		match.Score += score
	}
	if hits == 0 || hits*2 < len(tokens) {
		return Match{}, false
	}
	match.Complete = hits == len(tokens)
	return match, true
}

func (i *indexed) tokenScore(token string) int {
	best := fieldScore(i.title, token, titleExact, titlePrefix, titleContains)
	for _, keyword := range i.keywords {
		best = max(best, fieldScore(keyword, token, keywordExact, keywordPrefix, keywordContains))
	}
	best = max(best, fieldScore(i.latin, token, latinExact, latinPrefix, latinContains))
	best = max(best, fieldScore(i.initials, token, initialsExact, initialsPrefix, 0))
	if len(i.acronym) > 1 {
		best = max(best, fieldScore(i.acronym, token, acronymExact, acronymPrefix, 0))
	}
	if strings.Contains(i.category, token) {
		best = max(best, categoryContains)
	}
	return best
}

// fieldScore scores one normalized field. A zero "contains" band disables that
// band, which keeps short acronyms from matching in the middle of a word.
func fieldScore(value, token string, exact, prefix, contains int) int {
	switch {
	case value == "":
		return 0
	case value == token:
		return exact
	case strings.HasPrefix(value, token):
		return prefix
	case contains > 0 && strings.Contains(value, token):
		return contains
	}
	return 0
}

func normalize(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
}

func buildIndex(entries []Entry) []indexed {
	out := make([]indexed, len(entries))
	for i, entry := range entries {
		keywords := make([]string, 0, len(entry.Keywords))
		for _, keyword := range entry.Keywords {
			keywords = append(keywords, normalize(keyword))
		}
		out[i] = indexed{
			entry:    entry,
			title:    normalize(entry.Title),
			latin:    normalize(entry.Latin),
			category: normalize(entry.Category),
			keywords: keywords,
			initials: initials(entry.Title),
			acronym:  acronym(entry.Latin),
		}
	}
	return out
}

// initials turns a Chinese title into pinyin initials and keeps Latin letters,
// so 声音设置 gives "sysz" and "Wi-Fi" gives "wifi".
func initials(text string) string {
	var b strings.Builder
	args := gopinyin.NewArgs()
	args.Style = gopinyin.FirstLetter
	for _, r := range text {
		switch {
		case isHan(r):
			b.WriteString(strings.Join(gopinyin.LazyPinyin(string(r), args), ""))
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// acronym takes the first letter of every Latin word, so "Control Panel"
// gives "cp" and "Windows Update" gives "wu".
func acronym(text string) string {
	var b strings.Builder
	start := true
	for _, r := range normalize(text) {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			start = true
			continue
		}
		if start {
			b.WriteRune(r)
			start = false
		}
	}
	if len([]rune(b.String())) < 2 {
		return ""
	}
	return b.String()
}

// isHan reports whether r is a Han character; the same ranges the launcher's
// pinyin index uses.
func isHan(r rune) bool {
	return r >= 0x3400 && r <= 0x4DBF || r >= 0x4E00 && r <= 0x9FFF
}
