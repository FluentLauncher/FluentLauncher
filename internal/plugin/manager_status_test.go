package plugin

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/plugin/protocol"
)

func protocolResult(id, title string) protocol.Result {
	return protocol.Result{ID: id, Title: title}
}

func protocolAction(kind, value string) protocol.Action {
	return protocol.Action{ID: "a", Title: "动作", Kind: kind, Value: value}
}

func echoOptions() settings.PluginOptions {
	return settings.PluginOptions{
		EnabledIDs:         []string{"official.echo"},
		QueryTimeoutMs:     3000,
		MaxResults:         20,
		IdleSeconds:        30,
		MaxActiveProcesses: 3,
	}
}

// The page has to show timings for a plugin that is not running, which means
// the numbers have to outlive the process: they are kept when it is recycled.
func TestStatusesKeepNumbersAfterStop(t *testing.T) {
	source, err := filepath.Abs("../../resources/examples/official.echo")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(filepath.Join(t.TempDir(), "plugins"))
	if _, err := repo.Install(source, false); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	defer manager.Dispose()
	manager.Configure(repo.Installed(), echoOptions())

	before := manager.Statuses()
	if len(before) != 1 || before[0].State() != StateStopped {
		t.Fatalf("before any search: %+v", before)
	}
	if before[0].RuntimeText() == "" {
		t.Fatal("no text for a plugin that has not run")
	}

	if _, err := manager.Search(context.Background(), search.Query{Text: "echo hi", Scope: search.ScopeAll}, 20); err != nil {
		t.Fatal(err)
	}
	running := manager.Statuses()[0]
	if running.State() != StateRunning {
		t.Fatalf("state after a search = %v", running.State())
	}
	if running.PID == 0 || running.StartupMs <= 0 || running.LastUsed.IsZero() {
		t.Fatalf("no runtime numbers: %+v", running)
	}

	manager.Stop("official.echo")
	stopped := manager.Statuses()[0]
	if stopped.Running || stopped.State() != StateStopped {
		t.Fatalf("state after stop = %v", stopped.State())
	}
	if stopped.StartupMs != running.StartupMs || !stopped.LastUsed.Equal(running.LastUsed) {
		t.Fatalf("timings lost with the process: %+v", stopped)
	}
	if stopped.PID != 0 {
		t.Fatalf("a stopped plugin still reports a pid: %+v", stopped)
	}
}

// Three failures pause a plugin; the page offers a restart, which is the only
// way out without editing the configuration file.
func TestFailuresPauseAndRestartClears(t *testing.T) {
	repo := NewRepository(filepath.Join(t.TempDir(), "plugins"))
	writePluginDir(t, filepath.Join(repo.Dir(), "vendor.broken"), "vendor.broken")
	if err := repo.Refresh(); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	defer manager.Dispose()
	manager.Configure(repo.Installed(), settings.PluginOptions{
		EnabledIDs:         []string{"vendor.broken"},
		QueryTimeoutMs:     500,
		MaxResults:         5,
		IdleSeconds:        5,
		MaxActiveProcesses: 1,
	})
	for i := 0; i < maxConsecutiveFailures; i++ {
		if _, err := manager.Search(context.Background(), search.Query{Text: "tool x", Scope: search.ScopeAll}, 5); err != nil {
			t.Fatal(err)
		}
	}
	paused := manager.Statuses()[0]
	if !paused.Paused || paused.State() != StatePaused {
		t.Fatalf("plugin not paused after %d failures: %+v", maxConsecutiveFailures, paused)
	}
	if paused.Error == "" {
		t.Fatal("the reason of a paused plugin is not kept")
	}

	manager.Restart("vendor.broken")
	restarted := manager.Statuses()[0]
	if restarted.Paused || restarted.State() != StateStopped {
		t.Fatalf("restart left the plugin paused: %+v", restarted)
	}
	if restarted.Error != "" {
		t.Fatalf("restart kept the old error: %+v", restarted)
	}
}

// Disabling a plugin from the page removes its session without blocking the
// caller on a query in flight, and the numbers it reported still show: the row
// reads "未启用" with the timings it had, not an empty one.
func TestConfigureKeepsNumbersOfDisabledPlugin(t *testing.T) {
	source, err := filepath.Abs("../../resources/examples/official.echo")
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(filepath.Join(t.TempDir(), "plugins"))
	if _, err := repo.Install(source, false); err != nil {
		t.Fatal(err)
	}
	manager := NewManager()
	defer manager.Dispose()
	manager.Configure(repo.Installed(), echoOptions())
	if _, err := manager.Search(context.Background(), search.Query{Text: "echo hi", Scope: search.ScopeAll}, 20); err != nil {
		t.Fatal(err)
	}
	options := echoOptions()
	options.EnabledIDs = nil
	manager.Configure(repo.Installed(), options)
	deadline := time.Now().Add(2 * time.Second)
	for {
		status := manager.Statuses()[0]
		if !status.Running && status.StartupMs > 0 {
			if status.State() != StateDisabled {
				t.Fatalf("state = %v", status.State())
			}
			if !strings.Contains(status.RuntimeText(), "启动") {
				t.Fatalf("timings missing from %q", status.RuntimeText())
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the process was not closed, or its numbers were dropped: %+v", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A plugin whose initialize reply disagrees with its manifest is refused with
// both values in the message: the old message said only that the handshake did
// not match, which left a plugin author with nothing to fix. (The examples read
// their id from plugin.json, so this is the path a hardcoded id takes.)
func TestHandshakeReportsTheMismatch(t *testing.T) {
	ok := protocol.InitializeResult{ProtocolVersion: protocol.ProtocolVersion, PluginID: "vendor.tool"}
	if err := handshakeError(ok, "vendor.tool"); err != nil {
		t.Fatalf("matching handshake refused: %v", err)
	}
	mismatch := handshakeError(protocol.InitializeResult{ProtocolVersion: protocol.ProtocolVersion, PluginID: "official.echo"}, "vendor.tool")
	if mismatch == nil {
		t.Fatal("a plugin naming another id was accepted")
	}
	for _, want := range []string{"official.echo", "vendor.tool"} {
		if !strings.Contains(mismatch.Error(), want) {
			t.Fatalf("message %q does not name %q", mismatch, want)
		}
	}
	oldProtocol := handshakeError(protocol.InitializeResult{ProtocolVersion: 2, PluginID: "vendor.tool"}, "vendor.tool")
	if oldProtocol == nil || !strings.Contains(oldProtocol.Error(), "协议版本") {
		t.Fatalf("protocol mismatch = %v", oldProtocol)
	}
}

// A result the launcher refuses is reported instead of disappearing: a plugin
// author sees which rule they broke.
func TestInvalidResultsAreReported(t *testing.T) {
	if reason, ok := validateResult(protocolResult("id", "")); ok || reason == "" {
		t.Fatalf("empty title accepted: %q %v", reason, ok)
	}
	if _, ok := validateResult(protocolResult("id", "标题")); !ok {
		t.Fatal("valid result refused")
	}
	if reason, ok := validateAction(protocolAction("openUrl", "file:///x")); ok || reason == "" {
		t.Fatalf("non-http url accepted: %q %v", reason, ok)
	}
	if reason, ok := validateAction(protocolAction("nonsense", "x")); ok || reason == "" {
		t.Fatalf("unknown kind accepted: %q %v", reason, ok)
	}
	if _, ok := validateAction(protocolAction("openPath", `C:\Windows\notepad.exe`)); !ok {
		t.Fatal("absolute path refused")
	}
}
