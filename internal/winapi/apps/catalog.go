//go:build windows

// Package apps discovers installed applications and launches entries.
package apps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/egoist/mygo"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/winapi/shortcut"
)

// Catalog lists Start menu, desktop and portable applications. Installed
// apps come from the shell; portable folders and PATH are scanned here.
type Catalog struct {
	mu      sync.Mutex
	options settings.CatalogOptions
	warning string
	changed func()
	cancel  context.CancelFunc
}

func NewCatalog(options settings.CatalogOptions) *Catalog {
	return &Catalog{options: options}
}

func (c *Catalog) SetOptions(options settings.CatalogOptions) {
	c.mu.Lock()
	c.options = options
	c.mu.Unlock()
}

func (c *Catalog) Warning() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.warning
}

func (c *Catalog) SetChanged(fn func()) {
	c.mu.Lock()
	c.changed = fn
	c.mu.Unlock()
}

// Discover reads the shell's installed apps and supplements them with
// portable folders and PATH. It must run off the main thread.
func (c *Catalog) Discover(ctx context.Context) ([]model.Entry, error) {
	c.mu.Lock()
	options := c.options
	c.mu.Unlock()

	entries := make([]model.Entry, 0, 256)
	seen := map[string]bool{}
	add := func(entry model.Entry) {
		if entry.ID == "" || seen[entry.ID] {
			return
		}
		seen[entry.ID] = true
		entries = append(entries, entry)
	}

	installed, err := mygo.Shell.InstalledApps()
	if err != nil {
		c.setWarning("系统应用列表暂不可用：" + err.Error())
	}
	for _, app := range installed {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if app.Name == "" || app.Path == "" {
			continue
		}
		add(model.Entry{
			ID:           appID(app.Path, ""),
			Title:        app.Name,
			Subtitle:     appSubtitle(app.Path),
			LaunchTarget: app.Path,
			Kind:         model.Application,
			SourceID:     "applications",
			IconPath:     app.Path,
			AppID:        app.ID,
		})
	}

	roots := make([]string, 0)
	roots = append(roots, options.PortableDirectories...)
	for _, root := range roots {
		if root == "" {
			continue
		}
		scanRoot(ctx, root, true, add)
	}
	if options.IncludePath {
		for _, root := range filepath.SplitList(os.Getenv("PATH")) {
			root = strings.Trim(root, "\" ")
			if root != "" {
				scanRoot(ctx, root, false, add)
			}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Title) < strings.ToLower(entries[j].Title)
	})
	return entries, nil
}

func (c *Catalog) setWarning(message string) {
	c.mu.Lock()
	c.warning = message
	c.mu.Unlock()
}

func scanRoot(ctx context.Context, root string, recursive bool, add func(model.Entry)) {
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return
	}
	walk := func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if path != root && !recursive {
				return filepath.SkipDir
			}
			if d.Type()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			return nil
		}
		if !launchable(path) {
			return nil
		}
		identity, arguments := path, ""
		var aliases []string
		if strings.EqualFold(filepath.Ext(path), ".lnk") {
			if target, ok := shortcut.Resolve(path); ok {
				identity, arguments = target.Path, target.Arguments
				name := strings.TrimSuffix(filepath.Base(target.Path), filepath.Ext(target.Path))
				if name != "" {
					aliases = []string{name}
				}
			}
		}
		add(model.Entry{
			ID:           appID(identity, arguments),
			Title:        strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
			Subtitle:     path,
			LaunchTarget: path,
			Aliases:      aliases,
			Kind:         model.Application,
			SourceID:     "applications",
			IconPath:     path,
		})
		return nil
	}
	_ = filepath.WalkDir(root, walk)
}

func launchable(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".lnk", ".appref-ms", ".exe":
		return true
	}
	return false
}

// appSubtitle shows a readable origin for an entry: packaged apps carry
// a shell:AppsFolder identifier, which reads as noise in a result row.
func appSubtitle(path string) string {
	const prefix = "shell:AppsFolder\\"
	if !strings.HasPrefix(path, prefix) {
		return path
	}
	name := strings.Split(strings.TrimPrefix(path, prefix), "!")[0]
	if i := strings.LastIndex(name, "_"); i > 0 {
		name = name[:i]
	}
	return "应用包 " + name
}

func appID(identity, arguments string) string {
	sum := sha256.Sum256([]byte(strings.ToUpper(identity) + "\x00" + arguments))
	return "app:" + hex.EncodeToString(sum[:])
}

func (c *Catalog) Close() {
	c.mu.Lock()
	cancel := c.cancel
	c.cancel = nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

var errNoApplication = errors.New("不是可启动的应用")

// Watch starts coarse hints-only watches on the application roots. A change
// means "rediscover", not which file changed; the app debounces it.
func (c *Catalog) Watch(ctx context.Context) {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return
	}
	watchCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	options := c.options
	c.mu.Unlock()

	roots := []struct {
		path      string
		recursive bool
	}{
		{filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs"), true},
		{filepath.Join(os.Getenv("ProgramData"), "Microsoft", "Windows", "Start Menu", "Programs"), true},
		{filepath.Join(os.Getenv("USERPROFILE"), "Desktop"), false},
		{filepath.Join(os.Getenv("PUBLIC"), "Desktop"), false},
	}
	for _, root := range options.PortableDirectories {
		roots = append(roots, struct {
			path      string
			recursive bool
		}{root, true})
	}
	for _, root := range roots {
		if root.path == "" {
			continue
		}
		go c.watchRoot(watchCtx, root.path, root.recursive)
	}
}

func (c *Catalog) watchRoot(ctx context.Context, root string, recursive bool) {
	watcher, err := mygo.WatchFiles(ctx, root, mygo.FileWatchOptions{
		Recursive: recursive,
		HintsOnly: true,
	})
	if err != nil {
		c.setWarning("无法监听应用目录 " + root + "：" + err.Error())
		return
	}
	defer watcher.Close()
	for {
		events, err := watcher.Next(ctx)
		if err != nil || len(events) == 0 {
			return
		}
		c.mu.Lock()
		changed := c.changed
		c.mu.Unlock()
		if changed != nil {
			changed()
		}
	}
}
