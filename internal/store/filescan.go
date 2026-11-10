package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
)

func Scan(ctx context.Context, options settings.FileIndexOptions) []model.Entry {
	entries, _ := scanPaths(ctx, options, coalesceDirectories(options.Directories), false)
	return entryValues(entries)
}

func scanPaths(ctx context.Context, options settings.FileIndexOptions, roots []string, includeRoots bool) (map[string]model.Entry, error) {
	entries := make(map[string]model.Entry)
	visited := map[string]bool{}
	pending := append([]string(nil), roots...)
	add := func(path, title string, directory bool) {
		entries[strings.ToLower(path)] = FileEntry(path, title, directory)
	}
	if includeRoots {
		topRoots := coalesceDirectories(options.Directories)
		for _, path := range roots {
			if !safeSubtree(options.Directories, path) || insideAny(options.ExcludedDirectories, path) {
				continue
			}
			info, err := os.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
				continue
			}
			if info.IsDir() && !options.IncludeDirectories {
				continue
			}
			topRoot := false
			for _, root := range topRoots {
				if strings.EqualFold(root, path) {
					topRoot = true
					break
				}
			}
			if !topRoot {
				add(path, filepath.Base(path), info.IsDir())
			}
		}
	}
	for len(pending) > 0 && len(entries) < options.MaxEntries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		directory := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		key := strings.ToLower(directory)
		if visited[key] || insideAny(options.ExcludedDirectories, directory) || includeRoots && !safeSubtree(options.Directories, directory) {
			continue
		}
		visited[key] = true
		info, err := os.Lstat(directory)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || !info.IsDir() {
			continue
		}
		items, err := os.ReadDir(directory)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if len(entries) >= options.MaxEntries {
				break
			}
			full := filepath.Join(directory, item.Name())
			if insideAny(options.ExcludedDirectories, full) {
				continue
			}
			info, err := item.Info()
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
				continue
			}
			if info.IsDir() {
				pending = append(pending, full)
				if options.IncludeDirectories {
					add(full, item.Name(), true)
				}
			} else {
				add(full, item.Name(), false)
			}
		}
	}
	return entries, ctx.Err()
}

// A hinted directory may have become a junction. Check ancestors before
// opening it so a partial scan cannot follow it outside configured roots.
func safeSubtree(roots []string, path string) bool {
	for _, root := range roots {
		root = filepath.Clean(root)
		if !insideAny([]string{root}, path) {
			continue
		}
		for current := path; ; current = filepath.Dir(current) {
			info, err := os.Lstat(current)
			if err == nil && info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
				return false
			}
			if strings.EqualFold(current, root) {
				return true
			}
			if filepath.Dir(current) == current {
				return false
			}
		}
	}
	return false
}

func coalesceDirectories(paths []string) []string {
	ordered := make([]string, 0, len(paths))
	for _, path := range paths {
		if abs, err := filepath.Abs(path); err == nil && path != "" {
			ordered = append(ordered, abs)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return len(ordered[i]) < len(ordered[j]) })
	var out []string
	for _, path := range ordered {
		if !insideAny(out, path) {
			out = append(out, path)
		}
	}
	return out
}

func insideAny(roots []string, path string) bool {
	if path == "" {
		return false
	}
	return withinRoots(roots, strings.ToLower(filepath.Clean(path)))
}

func withinRoots(roots []string, key string) bool {
	for _, root := range roots {
		if root == "" {
			continue
		}
		root = strings.ToLower(filepath.Clean(root))
		if root != "" && strings.HasPrefix(key, root) && (len(key) == len(root) || root[len(root)-1] == byte(filepath.Separator) || key[len(root)] == byte(filepath.Separator)) {
			return true
		}
	}
	return false
}

func indexChanges(previous, current map[string]model.Entry, scopes []string) ([]model.Entry, []string) {
	var added []model.Entry
	var removed []string
	for key, old := range previous {
		if scopes != nil && !withinRoots(scopes, key) {
			continue
		}
		if entry, ok := current[key]; !ok || entry.Title != old.Title || entry.Kind != old.Kind {
			removed = append(removed, old.LaunchTarget)
		}
	}
	for key, entry := range current {
		if old, ok := previous[key]; !ok || old.Title != entry.Title || old.Kind != entry.Kind {
			added = append(added, entry)
		}
	}
	return added, removed
}

func entryValues(entries map[string]model.Entry) []model.Entry {
	out := make([]model.Entry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	return out
}
