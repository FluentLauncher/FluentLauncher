package app

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in because this builds and replaces a separate Windows process. It never
// starts the launcher itself or opens the user's database or configuration.
func TestSignedNightlyUpdateIntegration(t *testing.T) {
	dir := os.Getenv("FL_TEST_SIGNED_RELEASE_DIR")
	if dir == "" {
		t.Skip("set FL_TEST_SIGNED_RELEASE_DIR and FL_TEST_UPDATE_PUBLIC_KEY for signed nightly acceptance")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	key := strings.TrimSpace(os.Getenv("FL_TEST_UPDATE_PUBLIC_KEY"))
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(decoded) != 32 {
		t.Fatal("invalid test update public key")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	exe := filepath.Join(t.TempDir(), "FluentLauncher.exe")
	flags := "-X github.com/egoist/mygo.production=1" +
		" -X github.com/egoist/mygo.packageUpdateFeed=https://github.com/example/launcher/releases/latest/download/update-windows-amd64.json" +
		" -X github.com/egoist/mygo.packageUpdateKey=" + key
	build := exec.CommandContext(ctx, "go", "build", "-ldflags", flags, "-o", exe, "./testdata/updateprobe")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build acceptance process: %v\n%s", err, out)
	}
	out, err := exec.CommandContext(ctx, exe, dir).CombinedOutput()
	if err != nil {
		t.Fatalf("nightly acceptance: %v\n%s", err, out)
	}
	t.Log(string(out))
}
