// Package bundled contains only the launcher's own runtime assets.
package bundled

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed icon.png
var Icon []byte

//go:generate go run ../../scripts/icons -source ../../assets/brand/source.png -out .

//go:embed tray.png
var TrayIcon []byte

//go:embed mygo-sqlite3.dll
var sqliteLibrary []byte

// SQLiteLibrary extracts the pinned runtime to a content-addressed user cache.
// Different runtime versions get different paths, leaving loaded DLLs intact.
func SQLiteLibrary(cache string) (string, error) {
	digest := sha256.Sum256(sqliteLibrary)
	dir := filepath.Join(cache, "sqlite", fmt.Sprintf("%x", digest))
	path := filepath.Join(dir, "mygo-sqlite3.dll")
	if data, err := os.ReadFile(path); err == nil && sha256.Sum256(data) == digest {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "sqlite-*.tmp")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(sqliteLibrary)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return "", err
	}
	return path, nil
}
