package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writePluginDir puts a minimal valid package, with the given manifest id, in
// dir: the entry file is what ReadManifest insists on.
func writePluginDir(t *testing.T, dir, id string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(
		`{"id":%q,"name":"工具","version":"1.0.0","protocolVersion":1,"executable":"tool.exe","keywords":["tool"],"description":"说明"}`,
		id)
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tool.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A broken package used to overwrite the warning of the previous one, so a
// single working plugin could hide the rest; every package now gets its own
// entry with its own reason.
func TestRepositoryReportsEveryBrokenPackage(t *testing.T) {
	root := t.TempDir()
	repo := NewRepository(filepath.Join(root, "plugins"))
	writePluginDir(t, filepath.Join(repo.Dir(), "vendor.good"), "vendor.good")
	if err := os.MkdirAll(filepath.Join(repo.Dir(), "vendor.empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeBrokenManifest(t, filepath.Join(repo.Dir(), "vendor.bad"), `{"id":"vendor.bad","protocolVersion":9}`)
	if err := repo.Refresh(); err != nil {
		t.Fatal(err)
	}
	if installed := repo.Installed(); len(installed) != 1 || installed[0].Manifest.ID != "vendor.good" {
		t.Fatalf("installed = %+v", installed)
	}
	problems := repo.Problems()
	if len(problems) != 2 {
		t.Fatalf("problems = %+v", problems)
	}
	// Sorted by directory, so the page's list does not shuffle between frames.
	if problems[0].Directory != "vendor.bad" || problems[1].Directory != "vendor.empty" {
		t.Fatalf("problems are not sorted: %+v", problems)
	}
	if problems[0].Message == problems[1].Message {
		t.Fatalf("two different packages report the same reason: %+v", problems)
	}
}

// A staging directory left behind by an interrupted install holds a valid
// plugin.json and must not be listed as an installed plugin.
func TestRepositoryIgnoresStagingDirectories(t *testing.T) {
	repo := NewRepository(filepath.Join(t.TempDir(), "plugins"))
	writePluginDir(t, filepath.Join(repo.Dir(), ".staging-123"), "vendor.tool")
	if err := repo.Refresh(); err != nil {
		t.Fatal(err)
	}
	if installed := repo.Installed(); len(installed) != 0 {
		t.Fatalf("staging directory listed as installed: %+v", installed)
	}
	if problems := repo.Problems(); len(problems) != 0 {
		t.Fatalf("staging directory reported as a problem: %+v", problems)
	}
}

// The folder name is not always the manifest id: a package copied in by hand
// can disagree. It still runs, so it is listed, and uninstalling has to remove
// the folder it actually lives in rather than a folder built from the id.
func TestUninstallRemovesTheRegisteredDirectory(t *testing.T) {
	repo := NewRepository(filepath.Join(t.TempDir(), "plugins"))
	directory := filepath.Join(repo.Dir(), "renamed")
	writePluginDir(t, directory, "vendor.tool")
	if err := repo.Refresh(); err != nil {
		t.Fatal(err)
	}
	installed := repo.Installed()
	if len(installed) != 1 || installed[0].Manifest.ID != "vendor.tool" {
		t.Fatalf("installed = %+v", installed)
	}
	problems := repo.Problems()
	if len(problems) != 1 || problems[0].Directory != "renamed" {
		t.Fatalf("mismatch not reported: %+v", problems)
	}
	if err := repo.Uninstall("vendor.tool"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("package directory still there: %v", err)
	}
	if len(repo.Installed()) != 0 || len(repo.Problems()) != 0 {
		t.Fatalf("repository still has %+v / %+v", repo.Installed(), repo.Problems())
	}
}

func TestUninstallRejectsUnsafeID(t *testing.T) {
	repo := NewRepository(filepath.Join(t.TempDir(), "plugins"))
	if err := repo.Uninstall("../escape"); err == nil {
		t.Fatal("unsafe id accepted")
	}
}

// Installing from a directory checks the entry it names, so a directory that
// would install a package that cannot start is refused before the copy.
func TestInstallDirectoryRequiresEntry(t *testing.T) {
	root := t.TempDir()
	repo := NewRepository(filepath.Join(root, "plugins"))
	source := filepath.Join(root, "package")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"id":"vendor.tool","name":"工具","version":"1.0.0","protocolVersion":1,"executable":"missing.exe"}`
	if err := os.WriteFile(filepath.Join(source, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Install(source, false); err == nil {
		t.Fatal("package without its entry was installed")
	}
	if len(repo.Installed()) != 0 {
		t.Fatalf("installed = %+v", repo.Installed())
	}
}

func writeBrokenManifest(t *testing.T, dir, manifest string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tool.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
}
