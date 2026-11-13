package protocol

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
)

// ValidResourcePath accepts portable paths relative to a package, never a URL.
func ValidResourcePath(name string) bool {
	return len(name) <= 1024 && fs.ValidPath(name) && name != "." && !strings.ContainsAny(name, `\:`)
}

// ResourcePath resolves links before checking the package boundary. Call on a
// worker: resolving a resource is filesystem I/O, not a view operation.
func ResourcePath(directory, name string) (string, error) {
	if !ValidResourcePath(name) {
		return "", errors.New("invalid plugin resource path")
	}
	root, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("plugin resource is outside its package")
	}
	return path, nil
}
