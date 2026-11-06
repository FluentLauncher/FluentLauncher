package plugin

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryInstallAndUninstall(t *testing.T) {
	root := t.TempDir()
	repo := NewRepository(filepath.Join(root, "plugins"))
	source := filepath.Join(root, "official.echo.zip")
	makePluginZip(t, source, "echo.exe", false)
	check, err := repo.CheckUpdate(source)
	if err != nil {
		t.Fatal(err)
	}
	if check.Installed != nil || check.Package.ID != "official.echo" {
		t.Fatalf("check = %+v", check)
	}
	if _, err := repo.Install(source, false); err != nil {
		t.Fatal(err)
	}
	installed := repo.Installed()
	if len(installed) != 1 || installed[0].Manifest.ID != "official.echo" {
		t.Fatalf("installed = %+v", installed)
	}
	if err := repo.Uninstall("official.echo"); err != nil {
		t.Fatal(err)
	}
	if len(repo.Installed()) != 0 {
		t.Fatalf("uninstall left %+v", repo.Installed())
	}
}

func TestRepositoryRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	repo := NewRepository(filepath.Join(root, "plugins"))
	source := filepath.Join(root, "bad.zip")
	makePluginZip(t, source, "../evil.exe", true)
	if _, err := repo.Install(source, false); err == nil {
		t.Fatal("traversal package accepted")
	}
}

func makePluginZip(t *testing.T, path, executable string, traversal bool) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	archive := zip.NewWriter(file)
	manifest := `{"id":"official.echo","name":"Echo","version":"1.0.0","protocolVersion":1,"executable":"` + executable + `","keywords":["echo"],"permissions":[],"description":"x"}`
	entry, _ := archive.Create("plugin.json")
	if _, err := entry.Write([]byte(manifest)); err != nil {
		t.Fatal(err)
	}
	if traversal {
		entry, _ = archive.Create("../evil.exe")
	} else {
		entry, _ = archive.Create("echo.exe")
	}
	if _, err := entry.Write([]byte("MZ")); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
}
