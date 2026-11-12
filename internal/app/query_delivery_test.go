package app

import (
	"sync"
	"testing"

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

func TestLateProviderPreservesKeyboardSelection(t *testing.T) {
	a := newLocalizedApp()
	a.queryGeneration.Store(2)
	items := []model.Result{{Entry: model.Entry{ID: "one"}}, {Entry: model.Entry{ID: "two"}}}
	a.setResults(items, "two")
	a.resultsReady = true
	items = append(items, model.Result{Entry: model.Entry{ID: "three"}})
	a.deliverQuery(queryDelivery{generation: 2, preferred: "one", update: search.Update{Results: items}})
	if a.selectedID != "two" {
		t.Fatal("late provider reset user's selection")
	}
	a.deliverQuery(queryDelivery{generation: 1, update: search.Update{}})
	if len(a.results) != 3 {
		t.Fatal("obsolete results applied")
	}
}
