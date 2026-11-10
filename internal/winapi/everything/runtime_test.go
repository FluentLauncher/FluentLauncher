//go:build windows

package everything

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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
	results, err := p.Search(ctx, search.Query{Text: "Everything.exe", Scope: search.ScopeFiles}, 5)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status().State != Connected {
		t.Fatal("real client did not connect")
	}
	t.Logf("connected to real client; %d result(s)", len(results))
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
	if launches.Load() != 1 || c.snapshot().State != Connected {
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
	p := &Provider{Options: func() settings.FileIndexOptions { return options }, connection: c}
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
	entered := make(chan struct{})
	var once sync.Once
	p := &Provider{Options: func() settings.FileIndexOptions { return options }, connection: &connection{running: func() bool { once.Do(func() { close(entered) }); return true }}}
	p.mu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := p.Search(ctx, search.Query{Text: "report"}, 10); done <- err }()
	// No actual Everything call is possible: the provider lock is held.
	<-entered
	cancel()
	p.mu.Unlock()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled search was not discarded")
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
