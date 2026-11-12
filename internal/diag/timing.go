package diag

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// Fixed histogram buckets keep profiling memory bounded even during a long
// session. Counts and bucket upper bounds are exported, not misleading exact
// percentiles from a small or censored sample.
var durationBounds = [...]float64{1, 4, 8, 16, 32, 50, 100, 200, 500, 1000, 2000}

type Timing struct {
	Count   uint64     `json:"count"`
	TotalMS float64    `json:"total_ms"`
	MaxMS   float64    `json:"max_ms"`
	Buckets [12]uint64 `json:"buckets"`
}

type Timings struct {
	mu     sync.Mutex
	values map[string]Timing
}

func (r *Recorder) Measure(event string, duration time.Duration, attrs ...any) {
	active := r.Active()
	threshold := 100 * time.Millisecond
	if event == "query_complete" || event == "provider_everything" || event == "provider_plugins" {
		threshold = 500 * time.Millisecond
	}
	if strings.HasPrefix(event, "ui_") {
		threshold = 32 * time.Millisecond
	}
	canceled := false
	for i := 0; i+1 < len(attrs); i += 2 {
		if attrs[i] == "status" && attrs[i+1] == "canceled" {
			canceled = true
		}
	}
	if !active && (duration < threshold || canceled) {
		return
	}
	ms := float64(duration) / float64(time.Millisecond)
	if active {
		r.timings.mu.Lock()
		if r.timings.values == nil {
			r.timings.values = map[string]Timing{}
		}
		t := r.timings.values[event]
		t.Count++
		t.TotalMS += ms
		t.MaxMS = max(t.MaxMS, ms)
		i := 0
		for i < len(durationBounds) && ms > durationBounds[i] {
			i++
		}
		t.Buckets[i]++
		r.timings.values[event] = t
		r.timings.mu.Unlock()
	}
	level := slog.LevelDebug
	if duration >= threshold && !canceled {
		level = slog.LevelWarn
	}
	fields := append([]any{"duration_ms", ms}, attrs...)
	r.Logger.Log(context.Background(), level, event, fields...)
}

func (r *Recorder) timingSummary() any {
	r.timings.mu.Lock()
	defer r.timings.mu.Unlock()
	values := make(map[string]Timing, len(r.timings.values))
	for key, value := range r.timings.values {
		values[key] = value
	}
	return map[string]any{"bucket_upper_bounds_ms": durationBounds, "last_bucket": ">2000ms", "events": values}
}

func ErrorKind(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, os.ErrNotExist):
		return "not_found"
	case errors.Is(err, os.ErrPermission):
		return "permission"
	default:
		return "failed"
	}
}
