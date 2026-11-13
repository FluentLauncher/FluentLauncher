package search

import (
	"context"
	"reflect"
	"slices"
	"time"

	"fluentlauncher/internal/core/model"
)

type Update struct {
	Results []model.Result
	Batches []Batch
}

// Collect publishes the first results immediately, then coalesces provider
// completions over interval. All merging stays on the caller's worker. Empty
// completions that change neither results nor errors do not rebuild the UI.
// An initial empty update after one interval clears the previous query's rows
// even when external providers are still pending.
func Collect(ctx context.Context, input <-chan Batch, limit int, interval time.Duration, publish func(Update)) {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	tick := timer.C
	var batches []Batch
	var previous []model.Result
	var previousErrors []string
	sent, dirty := false, false
	emit := func(allowEmpty bool) {
		if !dirty && sent {
			return
		}
		matches := Merge(batches, limit)
		if !sent && !allowEmpty && len(matches) == 0 {
			return
		}
		var errors []string
		for _, batch := range batches {
			if batch.Error != "" {
				errors = append(errors, batch.SourceID+": "+batch.Error)
			}
		}
		dirty = false
		if sent && reflect.DeepEqual(previous, matches) && slices.Equal(previousErrors, errors) {
			return
		}
		if ctx.Err() != nil {
			return
		}
		publish(Update{matches, append([]Batch(nil), batches...)})
		previous, previousErrors, sent = matches, errors, true
	}
	for {
		select {
		case <-ctx.Done():
			return
		case batch, ok := <-input:
			if !ok {
				emit(true)
				return
			}
			batches = append(batches, batch)
			dirty = true
			if !sent {
				emit(false)
			}
			if sent && tick == nil {
				timer.Reset(interval)
				tick = timer.C
			}
		case <-tick:
			tick = nil
			emit(true)
		}
	}
}
