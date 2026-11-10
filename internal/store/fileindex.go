package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/locales"
)

// FileIndex scans the configured directories, writes only differences to
// SQLite, and publishes a new snapshot after a quiet period.
type FileIndex struct {
	db        *DB
	mu        sync.Mutex
	options   settings.FileIndexOptions
	persisted map[string]model.Entry
	warning   string
	onUpdate  func([]model.Entry)
	cancel    context.CancelFunc
	watchCtx  context.Context
	rebuild   sync.Mutex
	timer     *time.Timer
	closed    bool
}

func NewFileIndex(db *DB) *FileIndex {
	return &FileIndex{db: db}
}

func (f *FileIndex) SetOnUpdate(fn func([]model.Entry)) {
	f.mu.Lock()
	f.onUpdate = fn
	f.mu.Unlock()
}

func (f *FileIndex) Warning() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.warning
}

func (f *FileIndex) LoadCached(ctx context.Context, options settings.FileIndexOptions) ([]model.Entry, error) {
	if f.db == nil {
		return nil, errIndexClosed
	}
	entries, err := f.db.LoadFileIndex(ctx)
	if err != nil {
		return nil, err
	}
	snapshot := make(map[string]model.Entry, len(entries))
	for _, entry := range entries {
		snapshot[strings.ToLower(entry.LaunchTarget)] = entry
	}
	f.mu.Lock()
	f.persisted = snapshot
	f.mu.Unlock()
	out := make([]model.Entry, 0, len(entries))
	for _, entry := range entries {
		if !insideAny(options.Directories, entry.LaunchTarget) ||
			insideAny(options.ExcludedDirectories, entry.LaunchTarget) ||
			(!options.IncludeDirectories && entry.Kind == model.Directory) {
			continue
		}
		out = append(out, entry)
		if len(out) >= options.MaxEntries {
			break
		}
	}
	return out, nil
}

// Configure replaces the options, restarts the coarse watches, and rebuilds.
func (f *FileIndex) Configure(ctx context.Context, options settings.FileIndexOptions) error {
	if f.db == nil {
		return errIndexClosed
	}
	f.mu.Lock()
	if f.closed || ctx.Err() != nil {
		f.mu.Unlock()
		return context.Canceled
	}
	if f.cancel != nil && f.watchCtx.Err() == nil && f.options.SameIndex(options) {
		f.mu.Unlock()
		return nil
	}
	f.options = options
	if f.cancel != nil {
		f.cancel()
	}
	watchCtx, cancel := context.WithCancel(ctx)
	f.cancel = cancel
	f.watchCtx = watchCtx
	if f.timer != nil {
		f.timer.Stop()
	}
	f.mu.Unlock()

	for _, root := range options.Directories {
		if root == "" {
			continue
		}
		go f.watch(watchCtx, root)
	}
	return f.Rebuild(watchCtx)
}

func (f *FileIndex) watch(ctx context.Context, root string) {
	watcher, err := mygo.WatchFiles(ctx, root, mygo.FileWatchOptions{Recursive: true, HintsOnly: true})
	if err != nil {
		f.setWarning(locales.StoreFileIndexWatchFailed(root, err))
		return
	}
	defer watcher.Close()
	for {
		events, err := watcher.Next(ctx)
		if err != nil || len(events) == 0 {
			return
		}
		f.schedule(ctx)
	}
}

func (f *FileIndex) schedule(ctx context.Context) {
	f.mu.Lock()
	if f.closed || ctx.Err() != nil {
		f.mu.Unlock()
		return
	}
	if f.timer != nil {
		f.timer.Stop()
	}
	f.timer = time.AfterFunc(750*time.Millisecond, func() {
		if err := f.Rebuild(ctx); err != nil && ctx.Err() == nil {
			f.setWarning(locales.StoreFileIndexUpdateFailed(err))
		}
	})
	f.mu.Unlock()
}

// Rebuild scans and writes only added, removed or changed entries.
func (f *FileIndex) Rebuild(ctx context.Context) error {
	if f.db == nil {
		return errors.New(locales.StoreFileIndexNoDatabase())
	}
	f.rebuild.Lock()
	defer f.rebuild.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return errIndexClosed
	}
	options, watchCtx := f.options, f.watchCtx
	f.mu.Unlock()
	if watchCtx != nil {
		// Manual rebuilds also belong to this configuration. A later settings
		// change must cancel them before they can persist an obsolete scan.
		scanCtx, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(watchCtx, cancel)
		defer stop()
		defer cancel()
		ctx = scanCtx
		if watchCtx.Err() != nil {
			return watchCtx.Err()
		}
	}

	entries := Scan(ctx, options)
	// Scan may return a partial slice on cancellation. It must never delete
	// the unvisited part of the persisted index or publish it as complete.
	if err := ctx.Err(); err != nil {
		return err
	}
	next := make(map[string]model.Entry, len(entries))
	for _, entry := range entries {
		next[strings.ToLower(entry.LaunchTarget)] = entry
	}

	f.mu.Lock()
	previous := f.persisted
	f.mu.Unlock()

	if previous == nil {
		if err := f.db.SaveFileIndex(ctx, entries); err != nil {
			return err
		}
	} else {
		removed := make([]string, 0)
		for key, old := range previous {
			if current, ok := next[key]; !ok || current.Title != old.Title || current.Kind != old.Kind {
				removed = append(removed, old.LaunchTarget)
			}
		}
		added := make([]model.Entry, 0)
		for key, entry := range next {
			old, ok := previous[key]
			if !ok || old.Kind != entry.Kind || old.Title != entry.Title {
				added = append(added, entry)
			}
		}
		if len(removed) == 0 && len(added) == 0 {
			return nil
		}
		if err := f.db.UpdateFileIndex(ctx, added, removed); err != nil {
			return err
		}
	}
	f.mu.Lock()
	if f.closed || ctx.Err() != nil {
		f.mu.Unlock()
		return context.Canceled
	}
	f.persisted = next
	callback := f.onUpdate
	f.mu.Unlock()
	if callback != nil {
		callback(entries)
	}
	return nil
}

// Scan walks the roots iteratively, skipping reparse points, system entries
// and excluded directories.
func Scan(ctx context.Context, options settings.FileIndexOptions) []model.Entry {
	entries := make(map[string]model.Entry)
	visited := map[string]bool{}
	pending := make([]string, 0, len(options.Directories))
	for _, root := range options.Directories {
		if abs, err := filepath.Abs(root); err == nil {
			pending = append(pending, abs)
		}
	}
	for len(pending) > 0 && len(entries) < options.MaxEntries {
		if ctx.Err() != nil {
			break
		}
		directory := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if visited[strings.ToLower(directory)] || insideAny(options.ExcludedDirectories, directory) {
			continue
		}
		visited[strings.ToLower(directory)] = true
		info, err := os.Lstat(directory)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			continue
		}
		items, err := os.ReadDir(directory)
		if err != nil {
			continue
		}
		for _, item := range items {
			if ctx.Err() != nil {
				return nil
			}
			if len(entries) >= options.MaxEntries {
				break
			}
			full := filepath.Join(directory, item.Name())
			if insideAny(options.ExcludedDirectories, full) {
				continue
			}
			info, err := item.Info()
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
				continue
			}
			if info.IsDir() {
				pending = append(pending, full)
				if options.IncludeDirectories {
					entry := FileEntry(full, item.Name(), true)
					entries[strings.ToLower(full)] = entry
				}
			} else {
				entry := FileEntry(full, item.Name(), false)
				entries[strings.ToLower(full)] = entry
			}
		}
	}
	out := make([]model.Entry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	return out
}

func insideAny(roots []string, path string) bool {
	if path == "" {
		return false
	}
	path = strings.ToLower(filepath.Clean(path))
	for _, root := range roots {
		if root == "" {
			continue
		}
		root = strings.ToLower(filepath.Clean(root))
		if path == root || strings.HasPrefix(path, root+`\`) || strings.HasPrefix(path, root+"/") {
			return true
		}
	}
	return false
}

func (f *FileIndex) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	if f.cancel != nil {
		f.cancel()
		f.cancel = nil
	}
	if f.timer != nil {
		f.timer.Stop()
		f.timer = nil
	}
}

func (f *FileIndex) setWarning(message string) {
	f.mu.Lock()
	f.warning = message
	f.mu.Unlock()
}

var errIndexClosed = errors.New("file index closed")
