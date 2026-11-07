package plugin

import (
	"strconv"
	"strings"
	"time"

	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/locales"
)

// ClientStats is what one plugin process last reported about itself. It is a
// snapshot: reading it never waits for a query in flight, so the settings page
// can keep drawing while a plugin is busy or wedged.
type ClientStats struct {
	Running   bool
	PID       int
	StartupMs float64
	QueryMs   float64
	LastUsed  time.Time
	// Error is the last failure this client hit, kept after the process is
	// gone: it is the only way the page can explain why a plugin stopped.
	Error string
	// Stderr is the tail of the process's error output, capped by the client.
	Stderr string
}

// Status is one row of the settings page's plugin list: the package as it was
// read from disk, whether the user turned it on, and the last known runtime
// state. A plugin whose process was recycled keeps the numbers it had, which
// is why timings survive a process that is no longer running.
type Status struct {
	Registration protocol.Registration
	Enabled      bool
	GlobalQuery  bool
	Paused       bool
	// Failures counts consecutive failed queries; the manager pauses a plugin
	// that keeps failing so one broken plugin cannot slow every search.
	Failures int
	// Dropped counts results the launcher refused because they broke the
	// protocol, with the first reason in DropNote. A plugin that shows nothing
	// in the search results usually finds its answer here.
	Dropped  int
	DropNote string
	ClientStats
}

// State is the one-word condition of a plugin, for the settings page's status
// pill. The page maps it to a theme color; the plugin package does not know
// about colors.
type State int

const (
	// StateDisabled means the user turned the plugin off: it is never queried.
	StateDisabled State = iota
	// StateStopped means no process is running; it starts on the next search.
	StateStopped
	// StateRunning means a process is answering.
	StateRunning
	// StatePaused means the plugin failed too often and was paused by the
	// manager, so searches skip it until it is restarted.
	StatePaused
	// StateFailed means the last attempt failed, for example at startup.
	StateFailed
)

func (s State) String() string {
	switch s {
	case StateDisabled:
		return locales.PluginStateDisabled()
	case StateRunning:
		return locales.PluginStateRunning()
	case StatePaused:
		return locales.PluginStatePaused()
	case StateFailed:
		return locales.PluginStateFailed()
	default:
		return locales.PluginStateStopped()
	}
}

// State reduces the row to the condition the page shows. The order matters:
// a disabled plugin is never queried, so its last error is history, not its
// state, and a paused plugin is skipped before its stale session is looked at.
func (s Status) State() State {
	switch {
	case !s.Enabled:
		return StateDisabled
	case s.Paused:
		return StatePaused
	case s.Running:
		return StateRunning
	case s.Error != "":
		return StateFailed
	default:
		return StateStopped
	}
}

// RuntimeText is the line under a plugin's description: what it costs and what
// it last did. It always returns something, because "nothing to show" is
// itself worth explaining: a plugin that has not run yet starts on the next
// search, which is not obvious from an empty line.
func (s Status) RuntimeText() string {
	parts := make([]string, 0, 5)
	if s.Running {
		parts = append(parts, locales.PluginProcessPID(s.PID))
	}
	if s.StartupMs > 0 {
		parts = append(parts, locales.PluginStartupTime(formatMillis(s.StartupMs)))
	}
	if !s.LastUsed.IsZero() {
		// Shown for anything that has answered once, even when the clock's
		// resolution makes the round trip measure zero.
		parts = append(parts, locales.PluginQueryTime(formatMillis(s.QueryMs)))
		parts = append(parts, locales.PluginLastUsed(elapsedText(time.Since(s.LastUsed))))
	}
	if s.Failures > 0 {
		parts = append(parts, locales.PluginConsecutiveFailures(s.Failures))
	}
	if s.Dropped > 0 {
		parts = append(parts, locales.PluginDroppedResults(s.Dropped))
	}
	if len(parts) == 0 {
		if s.State() == StateDisabled {
			return locales.PluginDisabledNote()
		}
		return locales.PluginNotRunNote()
	}
	return strings.Join(parts, " · ")
}

// formatMillis keeps the numbers short: a plugin that takes a second is
// reported in seconds, because "1234 ms" reads slower than it happens. A fast
// plugin may measure zero, which says "under a millisecond" and not "unknown".
func formatMillis(ms float64) string {
	switch {
	case ms < 1:
		return locales.PluginUnderOneMillis()
	case ms < 1000:
		return locales.PluginMillis(strconv.FormatFloat(ms, 'f', 0, 64))
	default:
		return locales.PluginSeconds(strconv.FormatFloat(ms/1000, 'f', 1, 64))
	}
}

func elapsedText(d time.Duration) string {
	switch {
	case d < time.Minute:
		return locales.PluginElapsedJustNow()
	case d < time.Hour:
		return locales.PluginElapsedMinutes(int(d.Minutes()))
	case d < 24*time.Hour:
		return locales.PluginElapsedHours(int(d.Hours()))
	default:
		return locales.PluginElapsedDays(int(d.Hours() / 24))
	}
}
