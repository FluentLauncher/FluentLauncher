//go:build windows

package everything

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/core/settings"
)

// This opt-in probe bypasses MyGo app discovery, which requires a live UI
// lifecycle. It starts the selected installed client and leaves it running.
func TestEverythingLiveIntegration(t *testing.T) {
	if os.Getenv("FL_TEST_EVERYTHING_LIVE") != "1" {
		t.Skip("set FL_TEST_EVERYTHING_LIVE=1 for real client integration")
	}
	options := settings.DefaultFileIndexOptions()
	options.EnableEverything = true
	options.EverythingPath = os.Getenv("FL_TEST_EVERYTHING_PATH")
	p := NewProvider(func() settings.FileIndexOptions { return options }, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := p.Connect(ctx); err != nil {
		t.Fatalf("real IPC connection probe: %v", err)
	}
	results, err := p.Search(ctx, search.Query{Text: "Everything.exe", Scope: search.ScopeFiles}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status().State != Connected {
		t.Fatal("real client did not connect")
	}
	t.Logf("connected to real client; %d result(s)", len(results))
	for _, text := range []string{"Everything.exe", "fluentlauncher-no-such-file-7a39e2", "文件", "go"} {
		ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
		start := time.Now()
		results, err := query(ctx, text, 5)
		cancel()
		if err != nil {
			t.Fatalf("query %q: %v", text, err)
		}
		t.Logf("query %q: %d result(s) in %s", text, len(results), time.Since(start))
	}
	// Exercise a cancellation during native IPC, then verify the next query.
	for range 20 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		_, _ = query(ctx, "go", 5)
		cancel()
	}
	if _, err := p.Search(ctx, search.Query{Text: "Everything.exe"}, 5); err != nil {
		t.Fatalf("query after rapid cancellations: %v", err)
	}
}

func TestStartupIsSharedAndSurvivesCanceledSearch(t *testing.T) {
	var running atomic.Bool
	var launches atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	c := &connection{
		running: running.Load,
		resolve: func(string) (string, error) { close(entered); <-release; return "Everything.exe", nil },
		launch: func(_ string, show bool) error {
			if show {
				t.Error("background connection opened a window")
			}
			launches.Add(1)
			running.Store(true)
			return nil
		},
	}
	options := settings.DefaultFileIndexOptions()
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() { first <- c.ensure(ctx, options, false) }()
	<-entered
	cancel()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if c.snapshot().State != Starting {
		t.Fatal("canceled keystroke stopped the shared startup")
	}
	const waiters = 8
	results := make(chan error, waiters)
	for range waiters {
		go func() { results <- c.ensure(context.Background(), options, false) }()
	}
	close(release)
	for range waiters {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if launches.Load() != 1 || c.snapshot().State != Disconnected {
		t.Fatal("startup was duplicated or failed")
	}
}

func TestStartupFailureBackoffAndManualRetry(t *testing.T) {
	var resolves atomic.Int32
	c := &connection{running: func() bool { return false }, resolve: func(string) (string, error) {
		resolves.Add(1)
		return "", errors.New("missing executable")
	}}
	options := settings.DefaultFileIndexOptions()
	for range 3 {
		if err := c.ensure(context.Background(), options, false); err == nil {
			t.Fatal("missing executable succeeded")
		}
	}
	if resolves.Load() != 1 || c.snapshot().State != ConnectionFailed {
		t.Fatal("failed startup was not backed off")
	}
	_ = c.ensure(context.Background(), options, true)
	if resolves.Load() != 2 {
		t.Fatal("manual retry did not bypass backoff")
	}
	options.EverythingPath = "another.exe"
	_ = c.ensure(context.Background(), options, false)
	if resolves.Load() != 3 {
		t.Fatal("changed executable did not bypass old failure")
	}
}

func TestNoAutoStartAndCanceledSearchDoNotLaunch(t *testing.T) {
	var probes atomic.Int32
	c := &connection{running: func() bool { probes.Add(1); return false }}
	options := settings.DefaultFileIndexOptions()
	options.EverythingAutoStart = false
	if err := c.ensure(context.Background(), options, false); err == nil {
		t.Fatal("disconnected client succeeded")
	}
	if c.pending != nil {
		t.Fatal("auto-start disabled but a startup was scheduled")
	}
	p := NewProvider(func() settings.FileIndexOptions { return options }, nil)
	p.connection = c
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := probes.Load()
	if _, err := p.Search(ctx, search.Query{Text: "report"}, 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled search: %v", err)
	}
	if probes.Load() != before {
		t.Fatal("canceled query touched Everything")
	}
}

func TestQueuedCanceledSearchNeverSendsIPC(t *testing.T) {
	options := settings.DefaultFileIndexOptions()
	p := NewProvider(func() settings.FileIndexOptions { return options }, nil)
	var probes atomic.Int32
	p.connection.running = func() bool { probes.Add(1); return true }
	p.querySlot <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := p.search(ctx, "report", 10, options, false); done <- err }()
	cancel()
	// Cancellation must return while the native query slot is still occupied.
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled search was not discarded")
	}
	<-p.querySlot
	if probes.Load() != 0 {
		t.Fatal("queued canceled search touched Everything")
	}
}

func TestQueryFailureRetriesAfterBackoffOrPathChange(t *testing.T) {
	for _, change := range []string{"expired", "path"} {
		t.Run(change, func(t *testing.T) {
			options := settings.DefaultFileIndexOptions()
			p := NewProvider(func() settings.FileIndexOptions { return options }, nil)
			p.connection.running = func() bool { return true }
			p.connection.queryFinished(options, errors.New("unresponsive"))
			if change == "expired" {
				p.connection.retryAt = time.Now().Add(-time.Second)
			} else {
				options.EverythingPath = "another.exe"
			}
			p.runQuery = func(context.Context, string, int) ([]model.Result, error) { return nil, nil }
			if _, err := p.search(context.Background(), "report", 10, options, false); err != nil {
				t.Fatal(err)
			}
			if p.Status().State != Connected {
				t.Fatal("retry did not recover")
			}
		})
	}
}

func TestQueryFailureBackoffAndConnectionProbe(t *testing.T) {
	options := settings.DefaultFileIndexOptions()
	options.EnableEverything = true
	p := NewProvider(func() settings.FileIndexOptions { return options }, nil)
	p.connection.running = func() bool { return true }
	calls, fail := 0, true
	p.runQuery = func(_ context.Context, text string, limit int) ([]model.Result, error) {
		calls++
		if fail {
			return nil, context.DeadlineExceeded
		}
		if text != "" || limit != 1 {
			t.Fatal("manual connection did not use a minimal IPC probe")
		}
		return nil, nil
	}
	if err := p.Connect(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("client window without a reply appeared connected: %v", err)
	}
	for range 3 {
		if _, err := p.Search(context.Background(), search.Query{Text: "report"}, 10); err == nil {
			t.Fatal("unresponsive client succeeded")
		}
	}
	if calls != 1 || p.Status().State != ConnectionFailed {
		t.Fatalf("queries were not backed off: %d, %+v", calls, p.Status())
	}
	fail = false
	if err := p.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || p.Status().State != Connected || p.Status().Error != "" {
		t.Fatal("manual IPC probe did not clear the failure")
	}
}

func TestCanceledIPCDoesNotMarkConnectionFailed(t *testing.T) {
	options := settings.DefaultFileIndexOptions()
	p := NewProvider(func() settings.FileIndexOptions { return options }, nil)
	p.connection.running = func() bool { return true }
	p.connection.status.State = Connected
	ctx, cancel := context.WithCancel(context.Background())
	p.runQuery = func(context.Context, string, int) ([]model.Result, error) {
		cancel()
		return nil, context.Canceled
	}
	if _, err := p.Search(ctx, search.Query{Text: "report"}, 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled query: %v", err)
	}
	if p.Status().State != Connected || !p.connection.retryAt.IsZero() {
		t.Fatal("typing cancellation triggered connection failure/backoff")
	}
}

func TestExplicitExecutableSelection(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Everything.exe")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := resolveExecutable(path); err != nil || got != path {
		t.Fatalf("selected program: %q %v", got, err)
	}
	for _, path := range []string{"Everything.exe", root, filepath.Join(root, "missing.exe")} {
		if _, err := resolveExecutable(path); err == nil {
			t.Fatalf("invalid selected path accepted: %q", path)
		}
	}
}
