package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/locales"
)

const indexDebounce = 750 * time.Millisecond
const maxHintDirectories = 64

type indexRequest struct {
	full    bool
	dirs    []string
	waiters []chan error
}

func (r indexRequest) empty() bool { return !r.full && len(r.dirs) == 0 }

// FileIndex owns one refresh worker. Events received during a scan are merged
// into one follow-up request; callers of Rebuild can wait for that same pass.
type FileIndex struct {
	db         *DB
	mu         sync.Mutex
	options    settings.FileIndexOptions
	persisted  map[string]model.Entry
	warning    string
	onUpdate   func([]model.Entry)
	cancel     context.CancelFunc
	watchCtx   context.Context
	timer      *time.Timer
	pending    indexRequest
	running    bool
	configured bool
	closed     bool
}

func NewFileIndex(db *DB) *FileIndex {
	ctx, cancel := context.WithCancel(context.Background())
	return &FileIndex{db: db, watchCtx: ctx, cancel: cancel}
}

func (f *FileIndex) SetOnUpdate(fn func([]model.Entry)) { f.mu.Lock(); f.onUpdate = fn; f.mu.Unlock() }
func (f *FileIndex) Warning() string                    { f.mu.Lock(); defer f.mu.Unlock(); return f.warning }

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
	if f.closed || ctx.Err() != nil {
		f.mu.Unlock()
		return nil, context.Canceled
	}
	f.persisted = snapshot
	f.mu.Unlock()
	out := make([]model.Entry, 0, min(len(entries), options.MaxEntries))
	for _, entry := range entries {
		if !insideAny(options.Directories, entry.LaunchTarget) || insideAny(options.ExcludedDirectories, entry.LaunchTarget) || (!options.IncludeDirectories && entry.Kind == model.Directory) {
			continue
		}
		out = append(out, entry)
		if len(out) >= options.MaxEntries {
			break
		}
	}
	return out, nil
}

func (f *FileIndex) Configure(ctx context.Context, options settings.FileIndexOptions) error {
	if f.db == nil {
		return errIndexClosed
	}
	f.mu.Lock()
	if f.closed || ctx.Err() != nil {
		f.mu.Unlock()
		return context.Canceled
	}
	if f.configured && f.watchCtx.Err() == nil && f.options.SameIndex(options) {
		f.mu.Unlock()
		return nil
	}
	f.cancel()
	for _, waiter := range f.pending.waiters {
		waiter <- context.Canceled
	}
	f.pending = indexRequest{}
	f.options, f.configured = options, true
	watchCtx, cancel := context.WithCancel(ctx)
	f.watchCtx, f.cancel = watchCtx, cancel
	if f.timer != nil {
		f.timer.Stop()
	}
	f.mu.Unlock()
	for _, root := range options.Directories {
		if root != "" {
			go f.watch(watchCtx, root, options.ExcludedDirectories)
		}
	}
	return f.Rebuild(watchCtx)
}

func (f *FileIndex) watch(ctx context.Context, root string, excluded []string) {
	watcher, err := mygo.WatchFiles(ctx, root, mygo.FileWatchOptions{
		Recursive: true, HintsOnly: true,
		Ignore: func(path string, isDir bool) bool {
			return isDir && insideAny(excluded, filepath.Join(root, filepath.FromSlash(path)))
		},
	})
	if err != nil {
		if ctx.Err() == nil {
			f.setWarning(locales.StoreFileIndexWatchFailed(root, err))
		}
		return
	}
	defer watcher.Close()
	for {
		events, err := watcher.Next(ctx)
		if err != nil || len(events) == 0 {
			return
		}
		f.schedule(ctx, root, events)
	}
}

func (f *FileIndex) schedule(ctx context.Context, root string, events []mygo.FileEvent) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed || ctx.Err() != nil {
		return
	}
	for _, event := range events {
		if event.Path == "." || event.Op != mygo.FileHint {
			f.pending.full = true
			f.pending.dirs = nil
			break
		}
		path := filepath.Clean(filepath.Join(root, filepath.FromSlash(event.Path)))
		if strings.EqualFold(path, filepath.Clean(root)) {
			f.pending.full = true
			f.pending.dirs = nil
			break
		}
		if !f.pending.full && insideAny([]string{root}, path) && !insideAny(f.options.ExcludedDirectories, path) && !insideAny(f.pending.dirs, path) {
			f.pending.dirs = append(f.pending.dirs, path)
			if len(f.pending.dirs) > maxHintDirectories {
				f.pending.full, f.pending.dirs = true, nil
				break
			}
		}
	}
	f.pending.dirs = coalesceDirectories(f.pending.dirs)
	if !f.running && !f.pending.empty() {
		f.armLocked()
	}
}

func (f *FileIndex) armLocked() {
	if f.timer != nil {
		f.timer.Stop()
	}
	f.timer = time.AfterFunc(indexDebounce, func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if !f.running && !f.closed && !f.pending.empty() {
			f.running = true
			go f.work()
		}
	})
}

// Rebuild requests a full pass. Cancellation stops this caller's wait; the
// shared scan belongs to the current configuration and is canceled on change
// or Close. Requests arriving during a pass share one subsequent full pass.
func (f *FileIndex) Rebuild(ctx context.Context) error {
	if f.db == nil {
		return errors.New(locales.StoreFileIndexNoDatabase())
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	done := make(chan error, 1)
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return errIndexClosed
	}
	if ctx.Err() != nil {
		f.mu.Unlock()
		return ctx.Err()
	}
	f.pending.full, f.pending.dirs = true, nil
	f.pending.waiters = append(f.pending.waiters, done)
	if f.timer != nil {
		f.timer.Stop()
	}
	if !f.running {
		f.running = true
		go f.work()
	}
	f.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (f *FileIndex) work() {
	for {
		f.mu.Lock()
		if f.closed || f.pending.empty() {
			f.running = false
			f.mu.Unlock()
			return
		}
		request, ctx, options := f.pending, f.watchCtx, f.options
		f.pending = indexRequest{}
		f.mu.Unlock()
		err := f.refresh(ctx, options, request)
		for _, waiter := range request.waiters {
			waiter <- err
		}
		if err != nil && ctx.Err() == nil {
			f.setWarning(locales.StoreFileIndexUpdateFailed(err))
		}
		f.mu.Lock()
		if !f.closed && !f.pending.empty() && len(f.pending.waiters) == 0 {
			f.running = false
			f.armLocked()
			f.mu.Unlock()
			return
		}
		f.mu.Unlock()
	}
}

func (f *FileIndex) refresh(ctx context.Context, options settings.FileIndexOptions, request indexRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	previous := f.persisted
	f.mu.Unlock()
	full := request.full || previous == nil || len(previous) >= options.MaxEntries
	roots := coalesceDirectories(options.Directories)
	if !full {
		roots = coalesceDirectories(request.dirs)
	}
	if !full && len(roots) == 0 {
		return nil
	}
	current, err := scanPaths(ctx, options, roots, !full)
	if err != nil {
		return err
	}
	var scopes []string
	if !full {
		for _, root := range roots {
			scopes = append(scopes, strings.ToLower(root))
		}
	}
	added, removed := indexChanges(previous, current, scopes)
	if !full && (len(current) >= options.MaxEntries || len(previous)-len(removed)+len(added) > options.MaxEntries) {
		return f.refresh(ctx, options, indexRequest{full: true})
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(added) == 0 && len(removed) == 0 && previous != nil {
		f.mu.Lock()
		if !f.closed && ctx.Err() == nil {
			f.warning = ""
		}
		f.mu.Unlock()
		return ctx.Err()
	}
	if previous == nil {
		err = f.db.SaveFileIndex(ctx, entryValues(current))
	} else {
		err = f.db.UpdateFileIndex(ctx, added, removed)
	}
	if err != nil {
		return err
	}
	f.mu.Lock()
	// Even if cancellation followed COMMIT, remember what actually reached
	// SQLite. The next configuration must diff against that committed state.
	if full {
		f.persisted = current
	} else {
		for _, path := range removed {
			delete(previous, strings.ToLower(path))
		}
		for _, entry := range added {
			previous[strings.ToLower(entry.LaunchTarget)] = entry
		}
		f.persisted = previous
	}
	snapshot, callback := f.persisted, f.onUpdate
	active := !f.closed && ctx.Err() == nil
	if active {
		f.warning = ""
	}
	f.mu.Unlock()
	if active && callback != nil {
		callback(entryValues(snapshot))
	}
	return ctx.Err()
}

func (f *FileIndex) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	f.cancel()
	if f.timer != nil {
		f.timer.Stop()
	}
	for _, waiter := range f.pending.waiters {
		waiter <- context.Canceled
	}
	f.pending = indexRequest{}
}

func (f *FileIndex) setWarning(message string) { f.mu.Lock(); f.warning = message; f.mu.Unlock() }

var errIndexClosed = errors.New("file index closed")
