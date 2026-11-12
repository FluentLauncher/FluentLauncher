package diag

import (
	"archive/zip"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func recorder(t *testing.T, limit int64) *Recorder {
	t.Helper()
	r, err := open(t.TempDir(), limit)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r
}

func TestRotationAndDrain(t *testing.T) {
	r := recorder(t, 600)
	for i := range 30 {
		r.Logger.Info("rotation", "index", i)
	}
	r.Close()
	count := 0
	for i := range logFiles {
		data, err := os.ReadFile(r.path(i))
		if err != nil {
			t.Fatal(err)
		}
		if len(data) > 600 {
			t.Fatalf("file over limit: %d", len(data))
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if !json.Valid([]byte(line)) {
				t.Fatalf("partial log record: %s", line)
			}
			count++
		}
	}
	if count >= 30 {
		t.Fatal("rotation did not remove oldest logs")
	}
	data, _ := os.ReadFile(r.path(0))
	if !strings.Contains(string(data), `"index":29`) {
		t.Fatal("close lost last queued event")
	}
}

func TestQueueNeverWaitsForDiskAndReportsDrops(t *testing.T) {
	r := recorder(t, maxLogBytes)
	blocked, release := make(chan struct{}), make(chan struct{})
	r.queue <- logRequest{barrier: func() { close(blocked); <-release }}
	<-blocked
	defer close(release)
	done := make(chan struct{})
	go func() {
		for range queueCapacity + 25 {
			r.Logger.Info("busy")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("logging waited on disk")
	}
	if r.dropped.Load() != 25+priorityReserve {
		t.Fatalf("drops = %d", r.dropped.Load())
	}
	r.Logger.Error("important")
	if r.dropped.Load() != 25+priorityReserve {
		t.Fatal("priority reserve did not accept error")
	}
}

func TestCrashCapture(t *testing.T) {
	if dir := os.Getenv("FL_TEST_CRASH_DIR"); dir != "" {
		r, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		r.Install()
		go func() { panic("diagnostic-test-crash") }()
		select {}
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCrashCapture$")
	cmd.Env = append(os.Environ(), "FL_TEST_CRASH_DIR="+dir)
	if err := cmd.Run(); err == nil {
		t.Fatal("crash child exited successfully")
	}
	if ctx.Err() != nil {
		t.Fatal("crash child timed out")
	}
	data, err := os.ReadFile(filepath.Join(dir, "crash-current.log"))
	if err != nil || !strings.Contains(string(data), "diagnostic-test-crash") {
		t.Fatalf("crash output not captured: %v", err)
	}
}

func TestLegacyPrivacyAndExportAllowlist(t *testing.T) {
	r := recorder(t, maxLogBytes)
	r.Start()
	_, _ = (legacyWriter{r}).Write([]byte(`plugin malicious: token=secret C:\Users\private\report.txt https://host/?key=secret`))
	r.Logger.Debug("query_started", "query_id", 1, "length", 8)
	r.Measure("ui_queue", 8*time.Millisecond)
	for _, name := range []string{"settings.json", "launcher.db", "unrelated.log", "crash-current.log"} {
		if err := os.WriteFile(filepath.Join(r.dir, name), []byte("private-marker"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "diagnostics.zip")
	if err := r.Export(context.Background(), path, map[string]string{"version": "test"}, nil, false); err != nil {
		t.Fatal(err)
	}
	files := readZip(t, path)
	for name, data := range files {
		if strings.Contains(data, "private") || strings.Contains(data, "secret") {
			t.Fatalf("leak in %s: %s", name, data)
		}
		if name != "summary.json" && !strings.HasSuffix(name, ".jsonl") {
			t.Fatalf("unexpected export %s", name)
		}
	}
	if !strings.Contains(files["summary.json"], "ui_queue") {
		t.Fatal("missing timing summary")
	}
	if err := r.Export(context.Background(), path, nil, map[string]string{"example": "private-marker"}, true); err != nil {
		t.Fatal(err)
	}
	files = readZip(t, path)
	if files["private/crash-current.log"] != "private-marker" || !strings.Contains(files["private/plugin-stderr.json"], "private-marker") {
		t.Fatal("explicit raw export missing")
	}
}

func readZip(t *testing.T, path string) map[string]string {
	t.Helper()
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	out := map[string]string{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(data)
	}
	return out
}

func TestExpiryRetentionAndConcurrentClose(t *testing.T) {
	r := recorder(t, maxLogBytes)
	r.Start()
	if !r.Active() {
		t.Fatal("diagnostics did not start")
	}
	r.until.Store(time.Now().Add(-time.Second).UnixNano())
	if r.Active() || r.Logger.Enabled(context.Background(), -4) {
		t.Fatal("expired diagnostics still enabled")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				r.Logger.Info("concurrent")
			}
		})
	}
	wg.Go(r.Close)
	wg.Wait()
	old := r.path(4)
	if err := os.WriteFile(old, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(old, when, when); err != nil {
		t.Fatal(err)
	}
	next, err := Open(r.dir)
	if err != nil {
		t.Fatal(err)
	}
	next.Close()
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("expired file survived startup cleanup")
	}
}
