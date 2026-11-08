// Package search contains the launcher's matching, query routing, actions
// and built-in utility results. It does not touch the UI, storage or Windows.
package search

import (
	"container/heap"
	"context"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
)

const pinnedBonus = 800

type indexedEntry struct {
	entry    model.Entry
	id       string // strings.ToLower(entry.ID), computed once
	title    string
	pinyin   string
	initials string
	acronym  string
	subtitle string
	aliases  []string
	profile  model.Profile
}

// Engine is an immutable snapshot of entries and personal profiles.
type Engine struct {
	mu      sync.RWMutex
	entries []indexedEntry
	byID    map[string]int // lowered entry ID -> index in entries
}

func NewEngine() *Engine {
	return &Engine{}
}

// ReplaceEntries swaps the whole snapshot. Phonetics are reused when the
// title did not change, which keeps file-index refreshes cheap.
func (e *Engine) ReplaceEntries(entries []model.Entry, profiles map[string]model.Profile) {
	previous := map[string]indexedEntry{}
	e.mu.RLock()
	for _, old := range e.entries {
		previous[old.id] = old
	}
	e.mu.RUnlock()

	seen := map[string]bool{}
	next := make([]indexedEntry, 0, len(entries))
	byID := make(map[string]int, len(entries))
	for _, entry := range entries {
		key := strings.ToLower(entry.ID)
		if seen[key] {
			continue
		}
		seen[key] = true
		byID[key] = len(next)
		next = append(next, prepare(key, entry, profiles[key], previous[key]))
	}
	e.mu.Lock()
	e.entries = next
	e.byID = byID
	e.mu.Unlock()
}

// UpdateProfile replaces one entry's personal rule without rebuilding the
// phonetics of the rest of the snapshot.
func (e *Engine) UpdateProfile(profile model.Profile) {
	e.mu.Lock()
	defer e.mu.Unlock()
	i, ok := e.byID[strings.ToLower(profile.EntryID)]
	if !ok {
		return
	}
	next := make([]indexedEntry, len(e.entries))
	copy(next, e.entries)
	next[i].profile = profile
	next[i].aliases = prepareAliases(next[i].entry, profile)
	e.entries = next
}

func (e *Engine) UpdateProfiles(profiles map[string]model.Profile) {
	e.mu.Lock()
	defer e.mu.Unlock()
	next := make([]indexedEntry, len(e.entries))
	for i, old := range e.entries {
		profile := profiles[old.id]
		old.profile = profile
		old.aliases = prepareAliases(old.entry, profile)
		next[i] = old
	}
	e.entries = next
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
	for i := range entries {
		if err := ctx.Err(); err != nil {
			return nil
		}
		entry := &entries[i]
		if entry.profile.Hidden {
			continue
		}
		score := scoreEntry(entry, tokens, subseqOK)
		if len(tokens) > 0 && score == 0 {
			continue
		}
		if entry.profile.Pinned {
			if len(tokens) == 0 {
				score += 5000
			} else {
				score += pinnedBonus
			}
		}
		score += PersonalBonus(entry.profile, now)
		result := model.Result{Entry: entry.entry, Score: score}
		if h.Len() < limit {
			heap.Push(h, result)
		} else if better(result, (*h)[0]) {
			heap.Pop(h)
			heap.Push(h, result)
		}
	}
	out := make([]model.Result, h.Len())
	copy(out, *h)
	sort.Slice(out, func(i, j int) bool { return better(out[i], out[j]) })
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
	var title, py, initials, acronym string
	if previous.entry.Title == entry.Title && previous.entry.ID != "" {
		title, py, initials, acronym = previous.title, previous.pinyin, previous.initials, previous.acronym
	} else {
		title, py, initials, acronym = Phonetics(entry.Title)
	}
	return indexedEntry{
		entry:    entry,
		id:       id,
		title:    title,
		pinyin:   py,
		initials: initials,
		acronym:  acronym,
		subtitle: Normalize(entry.Subtitle),
		aliases:  prepareAliases(entry, profile),
		profile:  profile,
	}
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

func scoreEntry(entry *indexedEntry, tokens []string, subseqOK []bool) int {
	total := 0
	for j, token := range tokens {
		best := match(entry.title, token, 11000)
		for _, alias := range entry.aliases {
			best = max(best, match(alias, token, 12000))
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

type resultHeap []model.Result

func (h resultHeap) Len() int           { return len(h) }
func (h resultHeap) Less(i, j int) bool { return better(h[j], h[i]) }
func (h resultHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *resultHeap) Push(x any)        { *h = append(*h, x.(model.Result)) }
func (h *resultHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// Settings is used by providers that read the current configuration.
type Settings func() settings.LauncherSettings
