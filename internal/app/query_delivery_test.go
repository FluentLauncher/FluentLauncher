package app

import (
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/search"
)

func TestQueryDeliveryKeepsLatestAcrossGenerations(t *testing.T) {
	var box latestQueryUpdate
	var posted []func()
	var applied []int64
	post := func(fn func()) { posted = append(posted, fn) }
	apply := func(value queryDelivery) { applied = append(applied, value.generation) }
	for i := int64(1); i <= 100; i++ {
		box.publish(queryDelivery{generation: i}, post, apply)
	}
	box.publish(queryDelivery{generation: 4}, post, apply)
	if len(posted) != 1 {
		t.Fatalf("queued %d callbacks for a busy UI", len(posted))
	}
	posted[0]()
	if len(applied) != 1 || applied[0] != 100 {
		t.Fatal("stale result replaced newest query")
	}
	box.publish(queryDelivery{generation: 101}, post, apply)
	if len(posted) != 2 {
		t.Fatal("mailbox did not schedule again after draining")
	}
	posted[1]()
}

func TestQueryDeliveryConcurrentPublish(t *testing.T) {
	var box latestQueryUpdate
	var callback func()
	var wg sync.WaitGroup
	for i := int64(1); i <= 100; i++ {
		wg.Go(func() {
			box.publish(queryDelivery{generation: i}, func(fn func()) { callback = fn }, func(v queryDelivery) {
				if v.generation != 100 {
					t.Error("wrong generation")
				}
			})
		})
	}
	wg.Wait()
	callback()
}

func TestLateProviderMovesDefaultSelectionToFirstResult(t *testing.T) {
	a := newLocalizedApp()
	a.query = "tele"
	a.queryGeneration.Store(1)
	early := model.Result{Entry: model.Entry{ID: "early", Title: "Telegram.lnk"}}
	first := model.Result{Entry: model.Entry{ID: "first", Title: "Telegram"}}
	a.deliverQuery(queryDelivery{generation: 1, query: a.query, update: search.Update{Results: []model.Result{early}}})
	tt := ui.NewTester(a.view, 760, 560)
	a.deliverQuery(queryDelivery{generation: 1, query: a.query, update: search.Update{Results: []model.Result{first, early}}})
	tt.Frame()
	entry, ok := a.selected()
	if !ok || entry.ID != first.Entry.ID {
		t.Fatalf("selected %q after results reordered, want %q", entry.ID, first.Entry.ID)
	}
	firstRow, ok := tt.Find(first.Entry.Title)
	hint, hasHint := tt.Find(a.primaryShortcutHint("search.actions"))
	if !ok || !hasHint || hint.Y < firstRow.Y || hint.Y >= firstRow.Y+firstRow.H {
		t.Fatalf("first result or its action hint is not visible: row=%+v, hint=%+v", firstRow, hint)
	}
}

func TestLateProviderPreservesKeyboardSelection(t *testing.T) {
	a := newLocalizedApp()
	a.query = "tele"
	a.queryGeneration.Store(2)
	first := model.Result{Entry: model.Entry{ID: "one", Title: "Telegram"}}
	second := model.Result{Entry: model.Entry{ID: "two", Title: "Telegram.lnk"}}
	a.deliverQuery(queryDelivery{generation: 2, query: a.query, update: search.Update{Results: []model.Result{first, second}}})
	tt := ui.NewTester(a.view, 760, 560)
	a.input.Focus()
	tt.Frame()
	tt.Key(0, ui.KeyDown)
	newFirst := model.Result{Entry: model.Entry{ID: "three", Title: "Telegram Desktop"}}
	items := []model.Result{newFirst, first, second}
	a.deliverQuery(queryDelivery{generation: 2, query: a.query, update: search.Update{Results: items}})
	if entry, _ := a.selected(); entry.ID != second.Entry.ID {
		t.Fatal("late provider reset user's selection")
	}
	a.deliverQuery(queryDelivery{generation: 1, update: search.Update{}})
	if len(a.results) != 3 {
		t.Fatal("obsolete results applied")
	}
	a.queryGeneration.Store(3)
	a.deliverQuery(queryDelivery{generation: 3, query: a.query, update: search.Update{Results: []model.Result{second, newFirst, first}}})
	if entry, _ := a.selected(); entry.ID != second.Entry.ID || a.chosen != 0 {
		t.Fatal("refresh of the same query did not retain user's selection")
	}
}

func TestNewQueryResetsSelectionAndScrollWithSameResults(t *testing.T) {
	a := newLocalizedApp()
	a.query = "tel"
	a.queryGeneration.Store(1)
	items := []model.Result{
		{Entry: model.Entry{ID: "one", Title: "Telegram"}},
		{Entry: model.Entry{ID: "two", Title: "Telegram.lnk"}},
		{Entry: model.Entry{ID: "three", Title: "Telegram Desktop"}},
		{Entry: model.Entry{ID: "four", Title: "Telegram Web"}},
	}
	a.deliverQuery(queryDelivery{generation: 1, query: a.query, update: search.Update{Results: items}})
	tt := ui.NewTester(a.view, 480, 160)
	a.choose(len(items) - 1)
	tt.Frame()
	if first, _ := a.list.Visible(); first == 0 {
		t.Fatal("test setup did not scroll past the first result")
	}
	a.query = "tele"
	a.queryGeneration.Store(2)
	a.deliverQuery(queryDelivery{generation: 2, query: a.query, update: search.Update{Results: items}})
	tt.Frame()
	if entry, _ := a.selected(); entry.ID != items[0].Entry.ID {
		t.Fatalf("new query selected %q, want %q", entry.ID, items[0].Entry.ID)
	}
	if first, _ := a.list.Visible(); first != 0 {
		t.Fatalf("new query first visible row = %d, want 0", first)
	}
}
