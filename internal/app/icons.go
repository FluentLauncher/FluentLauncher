package app

import (
	"container/list"
	"context"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/model"
)

const (
	maxCachedIcons = 512
	iconWorkers    = 4
	iconQueueSize  = 64
)

type iconKey struct {
	path   string
	pixels int
}

type cachedIcon struct {
	key    iconKey
	bitmap *ui.Bitmap
}

// iconCache is used under iconLoader.mu. A nil bitmap is a cached failure,
// preventing an inaccessible path from being retried on every build pass.
type iconCache struct {
	capacity int
	items    map[iconKey]*list.Element
	recent   list.List
}

func (c *iconCache) get(key iconKey) (*ui.Bitmap, bool) {
	item, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.recent.MoveToFront(item)
	return item.Value.(cachedIcon).bitmap, true
}

func (c *iconCache) put(key iconKey, bitmap *ui.Bitmap) {
	if c.items == nil {
		c.items = make(map[iconKey]*list.Element)
	}
	if item, ok := c.items[key]; ok {
		item.Value = cachedIcon{key, bitmap}
		c.recent.MoveToFront(item)
		return
	}
	c.items[key] = c.recent.PushFront(cachedIcon{key, bitmap})
	if c.recent.Len() > c.capacity {
		last := c.recent.Back()
		delete(c.items, last.Value.(cachedIcon).key)
		c.recent.Remove(last)
	}
}

type iconLoader struct {
	mu      sync.Mutex
	cache   iconCache
	pending map[iconKey]bool
	queue   chan iconKey
	ctx     context.Context
	cancel  context.CancelFunc
}

func newIconLoader(load func(iconKey) *ui.Bitmap, changed func()) *iconLoader {
	ctx, cancel := context.WithCancel(context.Background())
	l := &iconLoader{cache: iconCache{capacity: maxCachedIcons}, pending: map[iconKey]bool{}, queue: make(chan iconKey, iconQueueSize), ctx: ctx, cancel: cancel}
	for range iconWorkers {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case key := <-l.queue:
					if ctx.Err() != nil {
						return
					}
					bitmap := load(key)
					l.mu.Lock()
					if ctx.Err() != nil {
						l.mu.Unlock()
						return
					}
					l.cache.put(key, bitmap)
					delete(l.pending, key)
					l.mu.Unlock()
					changed()
				}
			}
		}()
	}
	return l
}

func (l *iconLoader) get(key iconKey) *ui.Bitmap {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.ctx.Err() != nil {
		return nil
	}
	if bitmap, ok := l.cache.get(key); ok {
		return bitmap
	}
	if !l.pending[key] {
		select {
		case l.queue <- key:
			l.pending[key] = true
		default: // A completion invalidates the view, which retries visible misses.
		}
	}
	return nil
}

func (l *iconLoader) close() {
	l.cancel()
	l.mu.Lock()
	l.cache = iconCache{capacity: maxCachedIcons}
	clear(l.pending)
	l.mu.Unlock()
}

func (a *App) resultIcon(entry model.Entry) *ui.Bitmap {
	path := entry.IconPath
	if path == "" {
		path = entry.LaunchTarget
	}
	if path == "" || entry.Kind == model.Calculation || entry.Kind == model.URL || a.panel == nil {
		return nil
	}
	if a.icons == nil {
		win := a.panel
		a.icons = newIconLoader(func(key iconKey) *ui.Bitmap {
			data, err := mygo.Shell.FileIcon(key.path, key.pixels)
			if err != nil {
				return nil
			}
			bitmap, _ := ui.DecodeBitmap(data)
			return bitmap
		}, func() {
			if !win.IsDestroyed() {
				win.Invalidate()
			}
		})
	}
	pixels := a.iconPixels
	if pixels <= 0 {
		pixels = 32
	}
	return a.icons.get(iconKey{path, pixels})
}
