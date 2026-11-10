package plugin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/internal/testutil"
)

func commandManager(t *testing.T, id string) (*Manager, protocol.Registration, settings.PluginOptions) {
	t.Helper()
	source := testutil.PluginFixture(t, id)
	registration, err := protocol.ReadManifest(source)
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager()
	t.Cleanup(m.Dispose)
	opts := settings.DefaultPluginOptions()
	opts.EnabledIDs = []string{id}
	opts.QueryTimeoutMs = 3000
	m.Configure([]protocol.Registration{registration}, opts)
	return m, registration, opts
}

func TestCommandColdStartAndDisabled(t *testing.T) {
	m, registration, opts := commandManager(t, "vendor.commands")
	if m.Statuses()[0].Running {
		t.Fatal("command declaration started process")
	}
	if err := m.ExecuteCommand(context.Background(), registration.Manifest.ID, "write"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(registration.Directory, "commands.txt"))
	if err != nil || string(data) != "write\n" {
		t.Fatalf("command not executed: %q %v", data, err)
	}
	if err := m.ExecuteCommand(context.Background(), registration.Manifest.ID, "missing"); err == nil {
		t.Fatal("undeclared command executed")
	}
	opts.EnabledIDs = nil
	m.Configure([]protocol.Registration{registration}, opts)
	if err := m.ExecuteCommand(context.Background(), registration.Manifest.ID, "write"); err == nil {
		t.Fatal("disabled plugin executed")
	}
}

func TestCommandCapabilityAndFailurePause(t *testing.T) {
	m, registration, _ := commandManager(t, "vendor.commands.old")
	if err := m.ExecuteCommand(context.Background(), registration.Manifest.ID, "write"); err == nil || !strings.Contains(err.Error(), "executeCommand") {
		t.Fatalf("missing capability not reported: %v", err)
	}
	m2, r2, _ := commandManager(t, "vendor.commands")
	for range maxConsecutiveFailures {
		if err := m2.ExecuteCommand(context.Background(), r2.Manifest.ID, "fail"); err == nil {
			t.Fatal("failed command succeeded")
		}
	}
	if !m2.Statuses()[0].Paused {
		t.Fatal("repeated command failure did not pause plugin")
	}
	if err := m2.ExecuteCommand(context.Background(), r2.Manifest.ID, "write"); err == nil {
		t.Fatal("paused plugin executed")
	}
	m2.Restart(r2.Manifest.ID)
	if err := m2.ExecuteCommand(context.Background(), r2.Manifest.ID, "write"); err != nil {
		t.Fatal(err)
	}
}

func TestCommandRepeatAndReplacement(t *testing.T) {
	m, r, _ := commandManager(t, "vendor.commands")
	done := make(chan error, 1)
	go func() { done <- m.ExecuteCommand(context.Background(), r.Manifest.ID, "slow") }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		m.mu.Lock()
		active := m.commandActive[r.Manifest.ID+"/slow"]
		m.mu.Unlock()
		if active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("command never started")
		}
		time.Sleep(time.Millisecond)
	}
	if err := m.ExecuteCommand(context.Background(), r.Manifest.ID, "slow"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(r.Directory, "commands.txt"))
	if string(data) != "slow\n" {
		t.Fatalf("repeat executed twice: %q", data)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	go func() {
		done <- m.ReplacePackage(r.Manifest.ID, func() ([]protocol.Registration, error) {
			close(entered)
			<-release
			return nil, errors.New("install failure")
		})
	}()
	<-entered
	if err := m.ExecuteCommand(context.Background(), r.Manifest.ID, "write"); err == nil {
		t.Fatal("command executed during replacement")
	}
	if !m.Statuses()[0].Replacing {
		t.Fatal("replacement not visible to shortcut registrar")
	}
	close(release)
	<-done
	if err := m.ExecuteCommand(context.Background(), r.Manifest.ID, "write"); err != nil {
		t.Fatal(err)
	}
}

func TestCommandProcessBudget(t *testing.T) {
	m, first, opts := commandManager(t, "vendor.commands.one")
	second, err := protocol.ReadManifest(testutil.PluginFixture(t, "vendor.commands.two"))
	if err != nil {
		t.Fatal(err)
	}
	opts.EnabledIDs = append(opts.EnabledIDs, second.Manifest.ID)
	opts.MaxActiveProcesses = 1
	m.Configure([]protocol.Registration{first, second}, opts)
	if err := m.ExecuteCommand(context.Background(), first.Manifest.ID, "write"); err != nil {
		t.Fatal(err)
	}
	if err := m.ExecuteCommand(context.Background(), second.Manifest.ID, "write"); err == nil {
		t.Fatal("process budget ignored")
	}
}
