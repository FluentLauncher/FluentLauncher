//go:build windows

package everything

import (
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"

	"fluentlauncher/internal/core/model"
)

const (
	// Everything returns matches in its index order, not by relevance, so a
	// wider candidate set is ranked locally. Name-prefix matches are fetched
	// separately because a broad query can push them past the first page.
	prefixLimit    = 300
	candidateLimit = 200
	// Ranked scores stay in 100–800, below every local match band (the
	// weakest local hit, a subtitle match, scores 1000), so applications,
	// system entries and the local index always come first.
	exactScore  = 600
	prefixScore = 450
	wordScore   = 350
	partScore   = 250
	otherScore  = 150
)

// queryText appends the configured exclusions to a user query.
func queryText(text string, excludes []string) string {
	var b strings.Builder
	b.WriteString(text)
	for _, exclude := range excludes {
		exclude = strings.TrimSpace(exclude)
		if expanded, err := registry.ExpandString(exclude); err == nil {
			exclude = expanded
		}
		if exclude == "" || strings.ContainsRune(exclude, '"') {
			continue
		}
		b.WriteString(` !"`)
		b.WriteString(exclude)
		b.WriteString(`"`)
	}
	return b.String()
}

// prefixQuery restricts a plain single word to file names starting with it.
// Queries using Everything syntax are passed through untouched.
func prefixQuery(text string) (string, bool) {
	if text == "" || strings.ContainsAny(text, " \t:\"|!<>*?\\/") {
		return "", false
	}
	return "startwith:" + text, true
}

// rankResults removes duplicate paths, scores each result by how its name
// matches the query and returns the best limit results.
func rankResults(results []model.Result, text string, limit int) []model.Result {
	tokens := queryTokens(text)
	seen := make(map[string]bool, len(results))
	out := make([]model.Result, 0, len(results))
	for _, result := range results {
		key := strings.ToLower(result.Entry.LaunchTarget)
		if seen[key] {
			continue
		}
		seen[key] = true
		result.Score = fileScore(result.Entry, tokens)
		out = append(out, result)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return strings.ToLower(out[i].Entry.Title) < strings.ToLower(out[j].Entry.Title)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// queryTokens keeps the plain words; Everything functions and exclusions
// say nothing about how well a name matches.
func queryTokens(text string) []string {
	var tokens []string
	for _, token := range strings.Fields(strings.ToLower(text)) {
		token = strings.Trim(token, `"`)
		if token == "" || strings.ContainsAny(token, `:!<>|*?\/`) {
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens
}

func fileScore(entry model.Entry, tokens []string) int {
	name := strings.ToLower(entry.Title)
	stem := name
	if entry.Kind != model.Directory {
		stem = strings.TrimSuffix(name, filepath.Ext(name))
	}
	score := otherScore
	if len(tokens) > 0 {
		score = exactScore
		for _, token := range tokens {
			score = min(score, nameMatch(name, stem, token))
		}
	}
	switch {
	case entry.Kind == model.Directory:
		score += 150
	case launchableExt(name):
		score += 200
	}
	return score - min(strings.Count(entry.LaunchTarget, `\`), 10)*5
}

func nameMatch(name, stem, token string) int {
	switch {
	case name == token || stem == token:
		return exactScore
	case strings.HasPrefix(name, token):
		return prefixScore
	}
	i := strings.Index(name, token)
	switch {
	case i > 0 && strings.ContainsRune(" ._-()[]", rune(name[i-1])):
		return wordScore
	case i > 0:
		return partScore
	}
	return otherScore
}

func launchableExt(name string) bool {
	switch filepath.Ext(name) {
	case ".exe", ".lnk", ".com", ".bat", ".cmd", ".msc", ".cpl", ".appref-ms", ".url":
		return true
	}
	return false
}
