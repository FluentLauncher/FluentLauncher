package search

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"fluentlauncher/internal/core/model"
)

// Compare the bounded heap with a full materialization and sort. Mixed case,
// compatibility characters and equal titles protect the original tie order.
func TestTopKMatchesFullSort(t *testing.T) {
	e := NewEngine()
	entries := make([]model.Entry, 300)
	titles := []string{"AlphaApp", "alphaapp", "微信", "Visual Studio Code", "项目报告", "Ｃｏｄｅ", "Code"}
	profiles := map[string]model.Profile{}
	for i := range entries {
		id := fmt.Sprintf("entry:%03d", i)
		entries[i] = model.Entry{ID: id, Title: titles[i%len(titles)], Subtitle: "project tools", Aliases: []string{"应用"}}
		if i%11 == 0 {
			profiles[id] = model.Profile{EntryID: id, Alias: "ide;编辑器", Pinned: i%22 == 0, Hidden: i%33 == 0, UseCount: i % 20}
		}
	}
	e.ReplaceEntries(entries, profiles)
	for _, query := range []string{"", "alpha", "a", "wx", "vsc", "项目", "code", "应用", "ide", "项目 tools", "zzzzz"} {
		for _, limit := range []int{1, 20, 500} {
			tokens := strings.Fields(Normalize(query))
			fallback := make([]bool, len(tokens))
			for i, token := range tokens {
				fallback[i] = utf8.RuneCountInString(token) >= 3
			}
			var want []model.Result
			for i := range e.entries {
				entry := &e.entries[i]
				var profile model.Profile
				rule := entry.rule.Load()
				if rule != nil {
					profile = rule.profile
				}
				if profile.Hidden {
					continue
				}
				score := scoreEntry(entry, rule, tokens, fallback)
				if len(tokens) > 0 && score == 0 {
					continue
				}
				if profile.Pinned {
					if len(tokens) == 0 {
						score += 5000
					} else {
						score += pinnedBonus
					}
				}
				want = append(want, model.Result{Entry: entry.entry, Score: score + PersonalBonus(profile, time.Now())})
			}
			sort.Slice(want, func(i, j int) bool { return better(want[i], want[j]) })
			want = want[:min(limit, len(want))]
			got := e.Search(context.Background(), query, limit)
			if len(got) != len(want) || len(got) > 0 && !reflect.DeepEqual(got, want) {
				t.Fatalf("query=%q limit=%d: top-K differs from full sort", query, limit)
			}
		}
	}
}

func TestProfileSurvivesStaleIndexRefresh(t *testing.T) {
	e := NewEngine()
	entries := []model.Entry{{ID: "FILE:A", Title: "Original", Aliases: []string{"old-base"}}}
	e.ReplaceEntries(entries, nil)
	e.UpdateProfile(model.Profile{EntryID: "file:a", Alias: "personal", Pinned: true})
	entries[0].Title, entries[0].Aliases = "Renamed", []string{"new-base"}
	e.ReplaceEntries(entries, nil) // A scan's profile snapshot predates the launch.
	for _, query := range []string{"Renamed", "new-base", "personal"} {
		if got := e.Search(context.Background(), query, 1); len(got) != 1 {
			t.Fatalf("lost %q after refresh", query)
		}
	}
	if got := e.Search(context.Background(), "old-base", 1); len(got) != 0 {
		t.Fatal("old base alias survived rename")
	}
	e.UpdateProfiles(nil)
	if got := e.Search(context.Background(), "personal", 1); len(got) != 0 {
		t.Fatal("reset retained personal alias")
	}
	e.UpdateProfile(model.Profile{EntryID: "file:a", Hidden: true})
	if got := e.Search(context.Background(), "", 1); len(got) != 0 {
		t.Fatal("hidden rule not applied")
	}
}

func TestConcurrentIndexAndRuleUpdates(t *testing.T) {
	e := NewEngine()
	entries := []model.Entry{{ID: "a", Title: "Alpha"}, {ID: "b", Title: "Beta"}}
	e.ReplaceEntries(entries, nil)
	var wg sync.WaitGroup
	for worker := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 200 {
				switch worker {
				case 0:
					e.ReplaceEntries(entries, nil)
				case 1:
					e.UpdateProfile(model.Profile{EntryID: "a", UseCount: i, Alias: "first"})
				case 2:
					e.Search(context.Background(), "a", 20)
				case 3:
					e.UpdateProfiles(map[string]model.Profile{"b": {EntryID: "b", Hidden: i%2 == 0}})
				}
			}
		}()
	}
	wg.Wait()
	e.UpdateProfile(model.Profile{EntryID: "a", Alias: "latest"})
	if got := e.Search(context.Background(), "latest", 1); len(got) != 1 {
		t.Fatal("latest update lost")
	}
}
