// Package store persists the launcher's settings, personal rules and file
// index. SQLite comes from MyGo's purego plugin, so the app stays cgo-free.
package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/plugins/sqlite"

	"fluentlauncher/internal/core/model"
)

// DB owns the launcher database. It is safe for concurrent use; the plugin
// serializes statements on the connection.
type DB struct {
	db   *sqlite.DB
	path string
}

// OpenDB opens (and creates) the database, applies the additive schema and
// keeps one backup of an existing file before the first write.
func OpenDB(ctx context.Context, path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	backupDatabase(path)
	db, err := sqlite.Open(ctx, path, sqlite.OpenOptions{BusyTimeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}
	store := &DB{db: db, path: path}
	if err := store.initialize(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func backupDatabase(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return
	}
	backup := path + ".bak-0.1.0"
	if _, err := os.Stat(backup); err == nil {
		return
	}
	if data, err := os.ReadFile(path); err == nil {
		_ = os.WriteFile(backup, data, 0o600)
	}
}

func (d *DB) initialize(ctx context.Context) error {
	if _, err := d.db.Query(ctx, "PRAGMA journal_mode = WAL"); err != nil {
		return err
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS entry_profiles (
			entry_id TEXT PRIMARY KEY,
			alias TEXT NOT NULL DEFAULT '',
			pinned INTEGER NOT NULL DEFAULT 0,
			hidden INTEGER NOT NULL DEFAULT 0,
			use_count INTEGER NOT NULL DEFAULT 0,
			last_used_at TEXT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS file_index (path TEXT PRIMARY KEY, title TEXT NOT NULL, is_directory INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`,
		`DROP TABLE IF EXISTS query_history`,
	}
	for _, statement := range statements {
		if _, err := d.db.Execute(ctx, statement); err != nil {
			return err
		}
	}
	if _, err := d.db.Execute(ctx, `INSERT INTO schema_version(version)
		SELECT 1 WHERE NOT EXISTS (SELECT 1 FROM schema_version)`); err != nil {
		return err
	}
	return nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) LoadProfiles(ctx context.Context) (map[string]model.Profile, error) {
	rows, err := d.db.Query(ctx, `SELECT entry_id, alias, pinned, hidden, use_count, last_used_at FROM entry_profiles`)
	if err != nil {
		return nil, err
	}
	profiles := make(map[string]model.Profile, len(rows.Values))
	for _, row := range rows.Values {
		if len(row) != 6 {
			continue
		}
		profile := model.Profile{
			EntryID:  text(row[0]),
			Alias:    text(row[1]),
			Pinned:   integer(row[2]) != 0,
			Hidden:   integer(row[3]) != 0,
			UseCount: int(integer(row[4])),
		}
		if value := text(row[5]); value != "" {
			if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
				profile.LastUsedAt = &t
			}
		}
		profiles[profile.EntryID] = profile
	}
	return profiles, nil
}

func (d *DB) SaveProfile(ctx context.Context, profile model.Profile) error {
	var used any
	if profile.LastUsedAt != nil {
		used = profile.LastUsedAt.Format(time.RFC3339Nano)
	}
	_, err := d.db.Execute(ctx, `INSERT INTO entry_profiles(entry_id, alias, pinned, hidden, use_count, last_used_at)
		VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(entry_id) DO UPDATE SET alias=excluded.alias, pinned=excluded.pinned,
			hidden=excluded.hidden, use_count=excluded.use_count, last_used_at=excluded.last_used_at`,
		profile.EntryID, profile.Alias, boolInt(profile.Pinned), boolInt(profile.Hidden), profile.UseCount, used)
	return err
}

func (d *DB) ResetLearning(ctx context.Context) error {
	_, err := d.db.Execute(ctx, `UPDATE entry_profiles SET use_count = 0, last_used_at = NULL`)
	return err
}

// ImportProfiles replaces the personal rules in one transaction; learning
// data is kept unless includeLearning is set.
func (d *DB) ImportProfiles(ctx context.Context, profiles []model.Profile, includeLearning bool) error {
	statements := make([]sqlite.Statement, 0, len(profiles))
	for _, profile := range profiles {
		var used any
		if includeLearning && profile.LastUsedAt != nil {
			used = profile.LastUsedAt.Format(time.RFC3339Nano)
		}
		count := 0
		if includeLearning {
			count = profile.UseCount
		}
		statements = append(statements, sqlite.Statement{
			SQL: `INSERT INTO entry_profiles(entry_id, alias, pinned, hidden, use_count, last_used_at)
				VALUES(?, ?, ?, ?, ?, ?)
				ON CONFLICT(entry_id) DO UPDATE SET alias=excluded.alias, pinned=excluded.pinned,
					hidden=excluded.hidden,
					use_count=CASE WHEN ? THEN excluded.use_count ELSE entry_profiles.use_count END,
					last_used_at=CASE WHEN ? THEN excluded.last_used_at ELSE entry_profiles.last_used_at END`,
			Args: []any{profile.EntryID, profile.Alias, boolInt(profile.Pinned), boolInt(profile.Hidden),
				count, used, boolInt(includeLearning), boolInt(includeLearning)},
		})
	}
	_, err := d.db.Transaction(ctx, statements)
	return err
}

func (d *DB) LoadFileIndex(ctx context.Context) ([]model.Entry, error) {
	rows, err := d.db.Query(ctx, `SELECT path, title, is_directory FROM file_index`)
	if err != nil {
		return nil, err
	}
	entries := make([]model.Entry, 0, len(rows.Values))
	for _, row := range rows.Values {
		if len(row) != 3 {
			continue
		}
		entries = append(entries, FileEntry(text(row[0]), text(row[1]), integer(row[2]) != 0))
	}
	return entries, nil
}

// SaveFileIndex replaces the whole table in batches of 500 rows.
func (d *DB) SaveFileIndex(ctx context.Context, entries []model.Entry) error {
	if _, err := d.db.Execute(ctx, `DELETE FROM file_index`); err != nil {
		return err
	}
	for start := 0; start < len(entries); start += 500 {
		end := min(start+500, len(entries))
		statements := make([]sqlite.Statement, 0, end-start)
		for _, entry := range entries[start:end] {
			statements = append(statements, insertFile(entry))
		}
		if _, err := d.db.Transaction(ctx, statements); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) UpdateFileIndex(ctx context.Context, added []model.Entry, removed []string) error {
	statements := make([]sqlite.Statement, 0, len(removed)+len(added))
	for _, path := range removed {
		statements = append(statements, sqlite.Statement{SQL: `DELETE FROM file_index WHERE path=?`, Args: []any{path}})
	}
	for _, entry := range added {
		statements = append(statements, insertFile(entry))
	}
	_, err := d.db.Transaction(ctx, statements)
	return err
}

func insertFile(entry model.Entry) sqlite.Statement {
	return sqlite.Statement{
		SQL:  `INSERT OR REPLACE INTO file_index(path, title, is_directory) VALUES(?, ?, ?)`,
		Args: []any{entry.LaunchTarget, entry.Title, boolInt(entry.Kind == model.Directory)},
	}
}

// FileEntry builds the entry the file index stores.
func FileEntry(path, title string, directory bool) model.Entry {
	kind := model.File
	if directory {
		kind = model.Directory
	}
	return model.Entry{
		ID:           "file:" + upper(path),
		Title:        title,
		Subtitle:     path,
		LaunchTarget: path,
		Kind:         kind,
		SourceID:     "files",
		IconPath:     path,
	}
}

func text(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func integer(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case []byte:
		return 0
	}
	return 0
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func upper(s string) string {
	out := []byte(s)
	for i, b := range out {
		if b >= 'a' && b <= 'z' {
			out[i] = b - 32
		}
	}
	return string(out)
}
