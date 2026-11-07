package plugin

import (
	"strings"
	"testing"
	"time"
)

func TestStatusState(t *testing.T) {
	running := Status{Enabled: true, ClientStats: ClientStats{Running: true, PID: 42}}
	cases := []struct {
		name   string
		status Status
		want   State
	}{
		{"disabled wins over everything", Status{Enabled: false, Paused: true, Running: true}, StateDisabled},
		{"paused before running", Status{Enabled: true, Paused: true, ClientStats: ClientStats{Running: true}}, StatePaused},
		{"running", running, StateRunning},
		{"failed", Status{Enabled: true, ClientStats: ClientStats{Error: "启动失败"}}, StateFailed},
		{"idle", Status{Enabled: true, StartupMs: 12}, StateStopped},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.status.State(); got != c.want {
				t.Fatalf("state = %v, want %v", got, c.want)
			}
		})
	}
}

func TestStateString(t *testing.T) {
	for state, want := range map[State]string{
		StateDisabled: "未启用",
		StateStopped:  "已停止",
		StateRunning:  "运行中",
		StatePaused:   "已暂停",
		StateFailed:   "出错",
	} {
		if got := state.String(); got != want {
			t.Fatalf("state %d = %q, want %q", state, got, want)
		}
	}
}

func TestStatusRuntimeText(t *testing.T) {
	t.Run("never run", func(t *testing.T) {
		text := Status{Enabled: true}.RuntimeText()
		if !strings.Contains(text, "尚未运行") {
			t.Fatalf("text = %q", text)
		}
	})
	t.Run("disabled", func(t *testing.T) {
		text := Status{}.RuntimeText()
		if !strings.Contains(text, "未启用") {
			t.Fatalf("text = %q", text)
		}
	})
	t.Run("numbers survive a stopped process", func(t *testing.T) {
		text := Status{
			Enabled:     true,
			ClientStats: ClientStats{StartupMs: 120, QueryMs: 8, LastUsed: time.Now().Add(-3 * time.Minute)},
			Failures:    0,
			Dropped:     2,
		}.RuntimeText()
		for _, want := range []string{"启动 120 毫秒", "查询 8 毫秒", "上次使用 3 分钟前", "丢弃 2 条结果"} {
			if !strings.Contains(text, want) {
				t.Fatalf("text %q does not mention %q", text, want)
			}
		}
		if strings.Contains(text, "进程") {
			t.Fatalf("a stopped plugin reports a process: %q", text)
		}
	})
	t.Run("a query below the clock resolution", func(t *testing.T) {
		text := Status{Enabled: true, ClientStats: ClientStats{StartupMs: 40, LastUsed: time.Now()}}.RuntimeText()
		if !strings.Contains(text, "查询 <1 毫秒") {
			t.Fatalf("text = %q", text)
		}
	})
	t.Run("running and failing", func(t *testing.T) {
		text := Status{
			Enabled:     true,
			ClientStats: ClientStats{Running: true, PID: 1234, StartupMs: 2400},
			Failures:    2,
		}.RuntimeText()
		for _, want := range []string{"进程 1234", "启动 2.4 秒", "连续失败 2 次"} {
			if !strings.Contains(text, want) {
				t.Fatalf("text %q does not mention %q", text, want)
			}
		}
	})
}
