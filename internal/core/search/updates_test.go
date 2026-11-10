package search

import (
	"context"
	"fmt"
	"testing"
	"time"

	"fluentlauncher/internal/core/model"
)

func updateResult(id string, score int) Batch {
	return Batch{SourceID: id, Results: []model.Result{{Entry: model.Entry{ID: id, Title: id}, Score: score}}}
}

func receiveUpdate(t *testing.T, updates <-chan Update) Update {
	t.Helper()
	select {
	case update := <-updates:
		return update
	case <-time.After(3 * time.Second):
		t.Fatal("no result update")
		return Update{}
	}
}

func TestCollectPublishesFirstImmediatelyAndCoalescesBurst(t *testing.T) {
	input := make(chan Batch, 32)
	updates := make(chan Update, 32)
	done := make(chan struct{})
	go func() {
		Collect(context.Background(), input, 2, time.Hour, func(update Update) { updates <- update })
		close(done)
	}()
	input <- updateResult("fast", 1)
	if first := receiveUpdate(t, updates); len(first.Results) != 1 || first.Results[0].Entry.ID != "fast" {
		t.Fatal("first results delayed or lost")
	}
	for range 12 {
		input <- Batch{SourceID: "empty"}
	}
	input <- updateResult("best", 3)
	input <- updateResult("second", 2)
	input <- Batch{SourceID: "broken", Error: "unavailable"}
	close(input)
	last := receiveUpdate(t, updates)
	<-done
	if len(last.Results) != 2 || last.Results[0].Entry.ID != "best" || last.Results[1].Entry.ID != "second" || last.Batches[len(last.Batches)-1].Error != "unavailable" {
		t.Fatal("final flush lost results or errors")
	}
	if len(updates) != 0 {
		t.Fatal("burst published intermediate UI states")
	}
}

func TestCollectSkipsUnchangedEmptyCompletions(t *testing.T) {
	input := make(chan Batch, 32)
	input <- updateResult("same", 1)
	for range 20 {
		input <- Batch{SourceID: "empty"}
	}
	close(input)
	count := 0
	Collect(context.Background(), input, 20, time.Hour, func(Update) { count++ })
	if count != 1 {
		t.Fatalf("unchanged completions produced %d publications", count)
	}
}

func TestCollectFlushesWhileSlowSourceIsPending(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	input, updates := make(chan Batch, 4), make(chan Update, 4)
	done := make(chan struct{})
	go func() {
		Collect(ctx, input, 20, 5*time.Millisecond, func(update Update) { updates <- update })
		close(done)
	}()
	input <- updateResult("first", 1)
	receiveUpdate(t, updates)
	input <- updateResult("next", 2)
	if update := receiveUpdate(t, updates); len(update.Results) != 2 {
		t.Fatal("waited for slow provider instead of flushing frame")
	}
	cancel()
	<-done
	if len(updates) != 0 {
		t.Fatal("canceled query published late state")
	}
}

func TestCollectCompletesAnEmptyQuery(t *testing.T) {
	input := make(chan Batch, 1)
	input <- Batch{SourceID: "empty"}
	close(input)
	count := 0
	Collect(context.Background(), input, 20, time.Millisecond, func(update Update) {
		count++
		if len(update.Results) != 0 {
			t.Fatal("unexpected results")
		}
	})
	if count != 1 {
		t.Fatal("empty query never became ready")
	}
}

func BenchmarkProviderBurst(b *testing.B) {
	batches := make([]Batch, 20)
	for p := range batches {
		batches[p].SourceID = fmt.Sprint(p)
		for i := range 20 {
			id := fmt.Sprintf("%02d/%02d", p, i)
			batches[p].Results = append(batches[p].Results, model.Result{Entry: model.Entry{ID: id, Title: "Result " + id}, Score: p*100 + i})
		}
	}
	for _, coalesced := range []bool{false, true} {
		name := "Immediate"
		if coalesced {
			name = "Coalesced"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			publications := 0
			for b.Loop() {
				input := make(chan Batch, len(batches))
				for _, batch := range batches {
					input <- batch
				}
				close(input)
				if coalesced {
					Collect(context.Background(), input, 20, time.Hour, func(Update) { publications++ })
				} else {
					var received []Batch
					for batch := range input {
						received = append(received, batch)
						if len(Merge(received, 20)) > 0 {
							publications++
						}
					}
				}
			}
			b.ReportMetric(float64(publications)/float64(b.N), "updates/op")
		})
	}
}
