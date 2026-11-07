package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
)

func TestProfilesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(context.Background(), filepath.Join(dir, "data", "launcher.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().Truncate(time.Millisecond)
	profile := modelProfile("vsc", "编辑器", true, false, 3, &now)
	if err := db.SaveProfile(context.Background(), profile); err != nil {
		t.Fatal(err)
	}
	profiles, err := db.LoadProfiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, ok := profiles["vsc"]
	if !ok || got.Alias != "编辑器" || !got.Pinned || got.UseCount != 3 || got.LastUsedAt == nil {
		t.Fatalf("profile = %+v", got)
	}
	if err := db.ResetLearning(context.Background()); err != nil {
		t.Fatal(err)
	}
	profiles, _ = db.LoadProfiles(context.Background())
	if profiles["vsc"].UseCount != 0 || profiles["vsc"].LastUsedAt != nil {
		t.Fatalf("learning not reset: %+v", profiles["vsc"])
	}
}

func TestFileIndexRoundTrip(t *testing.T) {
	dir := t.TempDir()
	db, err := OpenDB(context.Background(), filepath.Join(dir, "data", "launcher.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	entries := []model.Entry{
		FileEntry(`C:\Temp\a.txt`, "a.txt", false),
		FileEntry(`C:\Temp\folder`, "folder", true),
	}
	if err := db.SaveFileIndex(context.Background(), entries); err != nil {
		t.Fatal(err)
	}
	loaded, err := db.LoadFileIndex(context.Background())
	if err != nil || len(loaded) != 2 {
		t.Fatalf("loaded %+v, %v", loaded, err)
	}
	if err := db.UpdateFileIndex(context.Background(), []model.Entry{FileEntry(`C:\Temp\b.txt`, "b.txt", false)}, []string{`C:\Temp\a.txt`}); err != nil {
		t.Fatal(err)
	}
	loaded, _ = db.LoadFileIndex(context.Background())
	if len(loaded) != 2 {
		t.Fatalf("after update %+v", loaded)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewConfigStore(dir)
	value := settings.Default()
	value.Width = 820
	value.Theme.Mode = settings.ThemeDark
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Width != 820 || loaded.Theme.Mode != settings.ThemeDark {
		t.Fatalf("loaded %+v", loaded)
	}
	pkg := ConfigurationPackage{Settings: loaded, Profiles: []model.Profile{{EntryID: "x", Alias: "y"}}}
	path := filepath.Join(dir, "export.json")
	if err := store.Export(path, pkg); err != nil {
		t.Fatal(err)
	}
	imported, err := store.Import(path)
	if err != nil || imported.Profiles[0].Alias != "y" {
		t.Fatalf("imported %+v, %v", imported, err)
	}
}

func TestScanSkipsExcluded(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "keep"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "skip"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skip", "b.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := settings.DefaultFileIndexOptions()
	options.Directories = []string{root}
	options.ExcludedDirectories = []string{filepath.Join(root, "skip")}
	entries := Scan(context.Background(), options)
	for _, entry := range entries {
		if filepath.Base(entry.LaunchTarget) == "b.txt" {
			t.Fatal("excluded file was scanned")
		}
	}
	if len(entries) < 2 {
		t.Fatalf("entries = %+v", entries)
	}
}

func modelProfile(id, alias string, pinned, hidden bool, count int, used *time.Time) model.Profile {
	return model.Profile{EntryID: id, Alias: alias, Pinned: pinned, Hidden: hidden, UseCount: count, LastUsedAt: used}
}
