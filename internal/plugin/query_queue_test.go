package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/internal/testutil"
)

func waitForQuerySequence(t *testing.T, client *Client, sequence uint64) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		client.queryMu.Lock()
		current := client.querySequence
		client.queryMu.Unlock()
		if current >= sequence {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("query never entered pending slot")
}

func TestClientKeepsOnlyLatestPendingQueryAndPreservesActions(t *testing.T) {
	directory := testutil.PluginFixture(t, "vendor.commands.queue")
	client := NewClient(filepath.Join(directory, "launcher-test-fixture.exe"), "vendor.commands.queue")
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := client.Query(ctx, protocol.Query{Text: "warm"}, time.Second); err != nil {
		t.Fatal(err)
	}
	pid := client.Stats().PID
	if err := client.lock(ctx); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	release := func() { once.Do(func() { <-client.opGate }) }
	defer release()
	actionDone := make(chan error, 1)
	go func() { actionDone <- client.ExecuteCommand(ctx, "write", time.Second) }()
	errorsFromOld := make(chan error, 32)
	for i := range 32 {
		go func() {
			_, err := client.Query(ctx, protocol.Query{Text: fmt.Sprintf("obsolete-%d", i)}, time.Second)
			errorsFromOld <- err
		}()
		waitForQuerySequence(t, client, uint64(i+2))
	}
	latest := make(chan error, 1)
	go func() {
		result, err := client.Query(ctx, protocol.Query{Text: "latest"}, time.Second)
		if err == nil && (len(result.Results) != 1 || result.Results[0].Title != "latest") {
			err = errors.New("latest response missing")
		}
		latest <- err
	}()
	waitForQuerySequence(t, client, 34)
	// A delayed, already-canceled caller must not replace the live latest slot.
	stale, stop := context.WithCancel(ctx)
	stop()
	if _, err := client.Query(stale, protocol.Query{Text: "stale"}, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for range 32 {
		select {
		case err := <-errorsFromOld:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("old query: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("obsolete queries waited for active RPC")
		}
	}
	release()
	for _, done := range []<-chan error{actionDone, latest} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("latest query or action did not complete")
		}
	}
	data, err := os.ReadFile(filepath.Join(directory, "queries.txt"))
	if err != nil || string(data) != "warm\nlatest\n" {
		t.Fatalf("obsolete requests reached plugin: %q %v", data, err)
	}
	commands, err := os.ReadFile(filepath.Join(directory, "commands.txt"))
	if err != nil || !strings.Contains(string(commands), "write") {
		t.Fatal("query superseded explicit action")
	}
	if client.Stats().PID != pid {
		t.Fatal("queue cancellation restarted reusable process")
	}
}

func TestGlobalPluginQueryDebounceAvoidsProcessStartup(t *testing.T) {
	source := testutil.PluginFixture(t, "official.echo")
	repo := NewRepository(filepath.Join(t.TempDir(), "plugins"))
	if _, err := repo.Install(source, false); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	defer manager.Dispose()
	options := echoOptions()
	options.GlobalQueryIDs = []string{"official.echo"}
	manager.Configure(repo.Installed(), options)
	for range 4 {
		ctx, cancel := context.WithCancel(context.Background())
		timer := time.AfterFunc(5*time.Millisecond, cancel)
		results, _ := manager.Search(ctx, search.Query{Text: "typing", Scope: search.ScopeAll}, 20)
		timer.Stop()
		cancel()
		if len(results) != 0 {
			t.Fatal("canceled global query produced results")
		}
	}
	manager.mu.Lock()
	count := len(manager.sessions)
	manager.mu.Unlock()
	if count != 0 {
		t.Fatal("typing burst started plugin process")
	}
	if got := manager.Statuses()[0]; got.PID != 0 || got.Failures != 0 {
		t.Fatalf("debounce affected diagnostics: %+v", got)
	}
	results, err := manager.Search(context.Background(), search.Query{Text: "echo explicit", Scope: search.ScopeAll}, 20)
	if err != nil || len(results) != 1 || results[0].Entry.Title != "explicit" {
		t.Fatalf("explicit keyword route failed: %+v %v", results, err)
	}
}

func TestClientDrainsCanceledActiveQueryBeforeLatest(t *testing.T) {
	directory := testutil.PluginFixture(t, "vendor.commands.queue")
	client := NewClient(filepath.Join(directory, "launcher-test-fixture.exe"), "vendor.commands.queue")
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	old := make(chan error, 1)
	go func() { _, err := client.Query(ctx, protocol.Query{Text: "slow"}, 2*time.Second); old <- err }()
	deadline := time.Now().Add(5 * time.Second)
	started := false
	for time.Now().Before(deadline) {
		data, _ := os.ReadFile(filepath.Join(directory, "queries.txt"))
		if strings.Contains(string(data), "slow") {
			started = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !started {
		t.Fatal("slow RPC did not start")
	}
	pid := client.Stats().PID
	cancel()
	result, err := client.Query(context.Background(), protocol.Query{Text: "latest"}, 2*time.Second)
	if err != nil || len(result.Results) != 1 || result.Results[0].Title != "latest" {
		t.Fatalf("late frame consumed as latest response: %+v %v", result, err)
	}
	if err := <-old; err == nil {
		t.Fatal("active query was not canceled")
	}
	if pid == 0 || client.Stats().PID != pid {
		t.Fatal("cancellation restarted process instead of draining reply")
	}
}
