package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fluentlauncher/internal/plugin/protocol"
)

func TestActivityPreventsIdleRecycleUntilLastWindowCloses(t *testing.T) {
	m, r, _ := commandManager(t, "vendor.commands.activity")
	if err := os.WriteFile(filepath.Join(r.Directory, "active.txt"), []byte("true"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.ExecuteCommand(context.Background(), r.Manifest.ID, "write"); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	sess := m.sessions[r.Manifest.ID]
	m.mu.Unlock()
	for range 2 {
		m.recycleIdle(r.Manifest.ID, sess, sess.idleEpoch)
		if !m.Statuses()[0].Running {
			t.Fatal("live or hidden UI was recycled")
		}
	}
	if err := os.WriteFile(filepath.Join(r.Directory, "active.txt"), []byte("false"), 0600); err != nil {
		t.Fatal(err)
	}
	m.recycleIdle(r.Manifest.ID, sess, sess.idleEpoch)
	if m.Statuses()[0].Running {
		t.Fatal("inactive plugin was not recycled")
	}
}

func TestActivityIdleTimerChecksAgainAfterWindowCloses(t *testing.T) {
	m, r, opts := commandManager(t, "vendor.commands.activity")
	opts.IdleSeconds = 1 // Short test interval exercises the real timer callback.
	m.Configure([]protocol.Registration{r}, opts)
	path := filepath.Join(r.Directory, "active.txt")
	if err := os.WriteFile(path, []byte("true"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.ExecuteCommand(context.Background(), r.Manifest.ID, "write"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1300 * time.Millisecond)
	if !m.Statuses()[0].Running {
		t.Fatal("timer recycled active UI")
	}
	if err := os.WriteFile(path, []byte("false"), 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for m.Statuses()[0].Running {
		if time.Now().After(deadline) {
			t.Fatal("timer did not recycle closed UI")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestOldPluginIdleAndExplicitStopIgnoreActivity(t *testing.T) {
	for _, id := range []string{"vendor.commands", "vendor.commands.activity"} {
		m, r, _ := commandManager(t, id)
		if err := os.WriteFile(filepath.Join(r.Directory, "active.txt"), []byte("true"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := m.ExecuteCommand(context.Background(), id, "write"); err != nil {
			t.Fatal(err)
		}
		if id == "vendor.commands" {
			m.mu.Lock()
			sess := m.sessions[id]
			m.mu.Unlock()
			m.recycleIdle(id, sess, sess.idleEpoch)
		} else {
			m.Stop(id)
		}
		if m.Statuses()[0].Running {
			t.Fatalf("%s not stopped", id)
		}
	}
}

func TestStaleIdleCallbackDoesNotRecycleRecentRequest(t *testing.T) {
	m, r, _ := commandManager(t, "vendor.commands")
	if err := m.ExecuteCommand(context.Background(), r.Manifest.ID, "write"); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	sess := m.sessions[r.Manifest.ID]
	epoch := sess.idleEpoch
	m.scheduleIdleLocked(r.Manifest.ID, sess)
	m.mu.Unlock()
	m.recycleIdle(r.Manifest.ID, sess, epoch)
	if !m.Statuses()[0].Running {
		t.Fatal("stale timer recycled a recently used session")
	}
}
