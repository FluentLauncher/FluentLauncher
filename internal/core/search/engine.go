// Package search contains the launcher's matching, query routing, actions
// and built-in utility results. It does not touch the UI, storage or Windows.
package search

import (
	"container/heap"
	"context"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
)

const pinnedBonus = 800

type indexedEntry struct {
	entry model.Entry
	id    string // strings.ToLower(entry.ID), computed once
	// Keep the lexical fields together: misses do not need the sort key.
	title     string
	pinyin    string
	initials  string
	acronym   string
	subtitle  string
	aliases   []string
	rule      *atomic.Pointer[personalRule]
	sortTitle string
}

type personalRule struct {
	profile model.Profile
	aliases []string
}

// Engine publishes immutable entry snapshots. Each entry's personal rule is
// independently replaced, so recording a launch never copies the whole index.
type Engine struct {
	mu      sync.RWMutex
	entries []indexedEntry
	byID    map[string]int // lowered entry ID -> index in entries
	rebuild sync.Mutex
	// Most file indexes have no personal aliases. In that case a lexical
	// miss can skip loading its separate rule altogether. This flag remains
	// enabled after alias removal, avoiding coordination with active queries.
	hasAliasRules atomic.Bool
}

func NewEngine() *Engine {
	return &Engine{}
}

// ReplaceEntries swaps the whole snapshot. Phonetics are reused when the
// title did not change. Existing personal rules survive refreshes; callers use
// UpdateProfiles when replacing rules, including resetting learning.
func (e *Engine) ReplaceEntries(entries []model.Entry, profiles map[string]model.Profile) {
	e.rebuild.Lock()
	defer e.rebuild.Unlock()
	e.mu.RLock()
	previous, previousIDs := e.entries, e.byID
	e.mu.RUnlock()

	next := make([]indexedEntry, 0, len(entries))
	byID := make(map[string]int, len(entries))
	for _, entry := range entries {
		key := strings.ToLower(entry.ID)
		if _, exists := byID[key]; exists {
			continue
		}
		var old indexedEntry
		if i, ok := previousIDs[key]; ok {
			old = previous[i]
		}
		byID[key] = len(next)
		next = append(next, prepare(key, entry, profiles[key], old))
		if profiles[key].Alias != "" {
			e.hasAliasRules.Store(true)
		}
	}
	e.mu.Lock()
	e.entries = next
	e.byID = byID
	e.mu.Unlock()
}

// UpdateProfile replaces one entry's personal rule without rebuilding the
// phonetics of the rest of the snapshot.
func (e *Engine) UpdateProfile(profile model.Profile) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	i, ok := e.byID[strings.ToLower(profile.EntryID)]
	if !ok {
		return
	}
	if profile.Alias != "" {
		e.hasAliasRules.Store(true)
	}
	e.entries[i].rule.Store(makePersonalRule(profile))
}

func (e *Engine) UpdateProfiles(profiles map[string]model.Profile) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for i := range e.entries {
		entry := &e.entries[i]
		if profiles[entry.id].Alias != "" {
			e.hasAliasRules.Store(true)
		}
		entry.rule.Store(makePersonalRule(profiles[entry.id]))
	}
}

// Search returns the best matches, hidden entries excluded.
func (e *Engine) Search(ctx context.Context, query string, limit int) []model.Result {
	if limit <= 0 {
		return nil
	}
	tokens := strings.Fields(Normalize(query))
	// The subsequence fallback only applies to tokens of 3+ runes; decide
	// once per token, not once per entry.
	subseqOK := make([]bool, len(tokens))
	for i, token := range tokens {
		subseqOK[i] = utf8.RuneCountInString(token) >= 3
	}
	now := time.Now()

	e.mu.RLock()
	entries := e.entries
	e.mu.RUnlock()

	h := &resultHeap{}
	heap.Init(h)
	hasAliases := e.hasAliasRules.Load()
	for i := range entries {
		if err := ctx.Err(); err != nil {
			return nil
		}
		entry := &entries[i]
		var rule *personalRule
		if hasAliases {
			rule = entry.rule.Load()
		}
		score := scoreEntry(entry, rule, tokens, subseqOK)
		if len(tokens) > 0 && score == 0 {
			continue
		}
		if !hasAliases {
			rule = entry.rule.Load()
		}
		if rule != nil {
			if rule.profile.Hidden {
				continue
			}
			if rule.profile.Pinned {
				if len(tokens) == 0 {
					score += 5000
				} else {
					score += pinnedBonus
				}
			}
			score += PersonalBonus(rule.profile, now)
		}
		result := candidate{entry: entry, score: score}
		if h.Len() < limit {
			heap.Push(h, result)
		} else if candidateBetter(result, (*h)[0]) {
			(*h)[0] = result
			heap.Fix(h, 0)
		}
	}
	sort.Slice(*h, func(i, j int) bool { return candidateBetter((*h)[i], (*h)[j]) })
	out := make([]model.Result, h.Len())
	for i, result := range *h {
		out[i] = model.Result{Entry: result.entry.entry, Score: result.score}
	}
	return out
}

func (e *Engine) Find(id string) (model.Entry, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if i, ok := e.byID[strings.ToLower(id)]; ok {
		return e.entries[i].entry, true
	}
	return model.Entry{}, false
}

func prepare(id string, entry model.Entry, profile model.Profile, previous indexedEntry) indexedEntry {
	var title, py, initials, acronym, sortTitle string
	if previous.entry.Title == entry.Title && previous.entry.ID != "" {
		title, py, initials, acronym = previous.title, previous.pinyin, previous.initials, previous.acronym
		sortTitle = previous.sortTitle
	} else {
		title, py, initials, acronym = Phonetics(entry.Title)
		sortTitle = strings.ToLower(entry.Title)
	}
	rule := previous.rule
	if rule == nil {
		rule = &atomic.Pointer[personalRule]{}
		rule.Store(makePersonalRule(profile))
	}
	subtitle, aliases := previous.subtitle, previous.aliases
	if previous.entry.ID == "" || previous.entry.Subtitle != entry.Subtitle {
		subtitle = Normalize(entry.Subtitle)
	}
	if previous.entry.ID == "" || !slices.Equal(previous.entry.Aliases, entry.Aliases) {
		aliases = prepareAliases(entry, model.Profile{})
	}
	return indexedEntry{
		entry:     entry,
		id:        id,
		title:     title,
		sortTitle: sortTitle,
		pinyin:    py,
		initials:  initials,
		acronym:   acronym,
		subtitle:  subtitle,
		aliases:   aliases,
		rule:      rule,
	}
}

func makePersonalRule(profile model.Profile) *personalRule {
	if profile.Alias == "" && !profile.Hidden && !profile.Pinned && profile.UseCount == 0 && profile.LastUsedAt == nil {
		return nil
	}
	return &personalRule{profile: profile, aliases: prepareAliases(model.Entry{}, profile)}
}

func prepareAliases(entry model.Entry, profile model.Profile) []string {
	all := append([]string{}, entry.Aliases...)
	all = append(all, strings.FieldsFunc(profile.Alias, func(r rune) bool { return r == ';' || r == '；' })...)
	out := make([]string, 0, len(all))
	seen := map[string]bool{}
	for _, alias := range all {
		alias = Normalize(strings.TrimSpace(alias))
		if alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		out = append(out, alias)
	}
	return out
}

func scoreEntry(entry *indexedEntry, rule *personalRule, tokens []string, subseqOK []bool) int {
	total := 0
	for j, token := range tokens {
		best := match(entry.title, token, 11000)
		for _, alias := range entry.aliases {
			best = max(best, match(alias, token, 12000))
		}
		if rule != nil {
			for _, alias := range rule.aliases {
				best = max(best, match(alias, token, 12000))
			}
		}
		if entry.pinyin != entry.title {
			best = max(best, match(entry.pinyin, token, 8500))
		}
		if entry.initials != entry.title {
			best = max(best, match(entry.initials, token, 8000))
		}
		if len(entry.acronym) > 1 && entry.acronym != entry.title && entry.acronym != entry.initials {
			best = max(best, match(entry.acronym, token, 8000))
		}
		if strings.Contains(entry.subtitle, token) {
			best = max(best, 1000)
		}
		if best == 0 && subseqOK[j] && subsequence(entry.title, token) {
			best = 2000
		}
		if best == 0 {
			return 0
		}
		total += best
	}
	return total
}

func match(value, token string, exact int) int {
	switch {
	case value == token:
		return exact
	case strings.HasPrefix(value, token):
		return exact - 2000
	case strings.Contains(value, token):
		return exact - 4000
	}
	return 0
}

func subsequence(text, token string) bool {
	rest := text
	for _, r := range token {
		i := strings.IndexRune(rest, r)
		if i < 0 {
			return false
		}
		rest = rest[i+utf8.RuneLen(r):]
	}
	return true
}

// PersonalBonus favors recent use within a match band; its maximum is below
// the 2000-point gap between bands.
func PersonalBonus(profile model.Profile, now time.Time) int {
	bonus := min(profile.UseCount, 20) * 8
	if profile.LastUsedAt == nil {
		return bonus
	}
	minutes := now.Sub(*profile.LastUsedAt).Minutes()
	switch {
	case minutes < 5:
		return bonus + 400
	case minutes < 60:
		return bonus + 300
	case minutes < 60*24:
		return bonus + 200
	case minutes < 60*24*7:
		return bonus + 120
	case minutes < 60*24*30:
		return bonus + 60
	}
	return bonus
}

func better(a, b model.Result) bool {
	if a.Score != b.Score {
		return a.Score > b.Score
	}
	at, bt := strings.ToLower(a.Entry.Title), strings.ToLower(b.Entry.Title)
	if at != bt {
		return at < bt
	}
	return a.Entry.ID < b.Entry.ID
}

type candidate struct {
	entry *indexedEntry
	score int
}

func candidateBetter(a, b candidate) bool {
	if a.score != b.score {
		return a.score > b.score
	}
	if a.entry.sortTitle != b.entry.sortTitle {
		return a.entry.sortTitle < b.entry.sortTitle
	}
	return a.entry.entry.ID < b.entry.entry.ID
}

type resultHeap []candidate

func (h resultHeap) Len() int           { return len(h) }
func (h resultHeap) Less(i, j int) bool { return candidateBetter(h[j], h[i]) }
func (h resultHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *resultHeap) Push(x any)        { *h = append(*h, x.(candidate)) }
func (h *resultHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// Settings is used by providers that read the current configuration.
type Settings func() settings.LauncherSettings
