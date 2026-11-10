package app

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestIconLRUPreservesHotEntriesAndDPI(t *testing.T) {
	c := iconCache{capacity: 2}
	a, b, otherDPI := iconKey{"a", 32}, iconKey{"b", 32}, iconKey{"a", 48}
	c.put(a, nil)
	c.put(b, nil)
	if _, ok := c.get(a); !ok {
		t.Fatal("negative result was not cached")
	}
	c.put(otherDPI, nil)
	if _, ok := c.get(b); ok {
		t.Fatal("least recently used icon was retained")
	}
	for _, key := range []iconKey{a, otherDPI} {
		if _, ok := c.get(key); !ok {
			t.Fatal("hot icon or DPI variant was evicted")
		}
	}
}

func TestIconLoaderBoundsWorkAndStops(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, iconWorkers)
	var active, peak atomic.Int32
	l := newIconLoader(func(iconKey) *ui.Bitmap {
		n := active.Add(1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		started <- struct{}{}
		<-release
		active.Add(-1)
		return nil
	}, func() {})
	defer func() { l.close(); close(release) }()
	for i := range 500 {
		l.get(iconKey{fmt.Sprint(i), 32})
	}
	for range iconWorkers {
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("workers did not start")
		}
	}
	l.mu.Lock()
	pending, queued := len(l.pending), len(l.queue)
	l.mu.Unlock()
	if peak.Load() != iconWorkers || pending > iconWorkers+iconQueueSize || queued > iconQueueSize {
		t.Fatalf("unbounded icon work: peak=%d pending=%d queued=%d", peak.Load(), pending, queued)
	}
	l.close()
	for i := range 100 {
		l.get(iconKey{fmt.Sprint(i), 48})
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.pending) != 0 {
		t.Fatal("closed loader accepted more work")
	}
}

func TestIconLoaderDeduplicatesFailures(t *testing.T) {
	var mu sync.Mutex
	counts := map[iconKey]int{}
	changed := make(chan struct{}, 4)
	l := newIconLoader(func(key iconKey) *ui.Bitmap { mu.Lock(); counts[key]++; mu.Unlock(); return nil }, func() { changed <- struct{}{} })
	defer l.close()
	for _, pixels := range []int{32, 48} {
		key := iconKey{"missing", pixels}
		for range 100 {
			l.get(key)
		}
		select {
		case <-changed:
		case <-time.After(3 * time.Second):
			t.Fatal("icon did not complete")
		}
		for range 100 {
			l.get(key)
		}
		mu.Lock()
		count := counts[key]
		mu.Unlock()
		if count != 1 {
			t.Fatalf("retried cached failure %d times", count)
		}
	}
}
