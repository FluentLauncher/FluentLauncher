package protocol

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackageResourcesStayInsideTheirDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "assets", "icon.png")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := ResourcePath(root, "assets/icon.png"); err != nil || got != path {
		t.Fatal(got, err)
	}
	for _, invalid := range []string{"../icon.png", "/icon.png", `C:\icon.png`, `assets\icon.png`, "https://example.com/icon.png", "assets/../../icon.png"} {
		if _, err := ResourcePath(root, invalid); err == nil {
			t.Fatalf("unsafe resource accepted: %s", invalid)
		}
	}
}
