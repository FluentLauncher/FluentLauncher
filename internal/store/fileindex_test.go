package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"fluentlauncher/internal/core/model"
)

func TestCanceledScanPreservesPersistedIndex(t *testing.T) {
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	entries := []model.Entry{FileEntry(`C:\Files\keep.txt`, "keep.txt", false)}
	if err := db.SaveFileIndex(context.Background(), entries); err != nil {
		t.Fatal(err)
	}
	index := NewFileIndex(db)
	defer index.Close()
	published := false
	index.SetOnUpdate(func([]model.Entry) { published = true })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := index.Rebuild(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled scan: %v", err)
	}
	loaded, err := db.LoadFileIndex(context.Background())
	if err != nil || len(loaded) != 1 || loaded[0].LaunchTarget != entries[0].LaunchTarget {
		t.Fatalf("canceled scan changed database: %+v %v", loaded, err)
	}
	if published {
		t.Fatal("canceled scan published incomplete results")
	}
}
