//go:build windows

package everything

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/winapi/apps"
	"fluentlauncher/locales"
)

type ConnectionState uint8

const (
	Disconnected ConnectionState = iota
	Starting
	Connected
	ConnectionFailed
)

const retryDelay = 15 * time.Second

type ConnectionStatus struct {
	State ConnectionState
	Path  string
	Error string
	// NeedsAdmin is set when the resolved client is configured to "Run as
	// administrator", so every launch raises a UAC prompt; FixAdmin clears it.
	NeedsAdmin bool
}

// connection owns a single startup attempt, independent of keystroke cancellation.
// Everything remains an independently owned application after it is launched.
type connection struct {
	mu         sync.Mutex
	status     ConnectionStatus
	pending    chan struct{}
	retryAt    time.Time
	lastPath   string
	discovered string
	// autoBlocked stops automatic launches after one failed or skipped
	// attempt, so a declined UAC prompt or a slow start is never repeated on
	// every keystroke. A manual connection, a successful query or a changed
	// program path re-arms it.
	autoBlocked bool
	changed     func()
	running     func() bool
	resolve     func(string) (string, error)
	launch      func(string, bool) error
	needsAdmin  func(string) bool
	elevate     func(ctx context.Context, file, arguments string) error
}

func newConnection(changed func()) *connection {
	c := &connection{changed: changed, running: clientRunning, launch: launchEverything, needsAdmin: requiresAdmin, elevate: apps.RunElevatedAndWait}
	c.resolve = func(configured string) (string, error) {
		if configured == "" {
			c.mu.Lock()
			candidate := c.discovered
			c.mu.Unlock()
			if candidate != "" {
				if path, err := resolveExecutable(candidate); err == nil {
					return path, nil
				}
			}
		}
		return resolveExecutable(configured)
	}
	return c
}

func clientRunning() bool {
	name, _ := windows.UTF16PtrFromString(everythingClass)
	hwnd, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(name)), 0)
	return hwnd != 0
}

func (c *connection) snapshot() ConnectionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func (c *connection) notify() {
	if c.changed != nil {
		c.changed()
	}
}

func (c *connection) ensure(ctx context.Context, options settings.FileIndexOptions, force bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	if options.EverythingPath != c.lastPath {
		c.autoBlocked = false
	}
	// IPC can fail while the client window is still present. Check backoff
	// before probing that window, otherwise every keystroke clears the error
	// and retries the same unresponsive connection.
	if !force && c.pending == nil && options.EverythingPath == c.lastPath && time.Now().Before(c.retryAt) {
		err := c.status.Error
		c.mu.Unlock()
		return errors.New(err)
	}
	if c.pending == nil && c.running() {
		c.mu.Unlock()
		return nil
	}
	if !force && !options.EverythingAutoStart && c.pending == nil {
		c.status.State, c.status.Error = Disconnected, locales.EverythingClientNotRunning()
		c.mu.Unlock()
		return errors.New(locales.EverythingClientNotRunning())
	}
	if !force && c.autoBlocked && c.pending == nil {
		err := c.status.Error
		c.mu.Unlock()
		return errors.New(err)
	}
	if c.pending == nil {
		c.pending = make(chan struct{})
		c.lastPath = options.EverythingPath
		c.status = ConnectionStatus{State: Starting, NeedsAdmin: c.status.NeedsAdmin}
		go c.start(options.EverythingPath, c.pending, force)
	}
	pending := c.pending
	c.mu.Unlock()
	c.notify()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-pending:
		status := c.snapshot()
		if status.Error != "" {
			return errors.New(status.Error)
		}
		return nil
	}
}

func (c *connection) queryFinished(options settings.FileIndexOptions, err error) {
	c.mu.Lock()
	previous := c.status
	c.lastPath = options.EverythingPath
	c.status.State, c.status.Error = Connected, ""
	c.retryAt = time.Time{}
	if err != nil {
		c.status.State, c.status.Error = ConnectionFailed, err.Error()
		c.retryAt = time.Now().Add(retryDelay)
	} else {
		c.autoBlocked = false
	}
	changed := previous != c.status
	c.mu.Unlock()
	if changed {
		c.notify()
	}
}

func (c *connection) start(configured string, pending chan struct{}, force bool) {
	path, err := c.resolve(configured)
	admin := err == nil && c.needsAdmin != nil && c.needsAdmin(path)
	if err == nil && !c.running() {
		// An elevated client raises a UAC prompt; only the user's own
		// connect action may cause that, never a keystroke.
		if !force && admin {
			err = errors.New(locales.EverythingNeedsAdmin())
		} else {
			err = c.launch(path, false)
		}
	}
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for !c.running() && err == nil {
			select {
			case <-ctx.Done():
				err = errors.New(locales.EverythingStartTimeout())
			case <-ticker.C:
			}
		}
	}
	c.mu.Lock()
	// The process is ready, but only a successful query confirms IPC.
	c.status = ConnectionStatus{State: Disconnected, Path: path, NeedsAdmin: admin}
	if err != nil {
		c.status.State, c.status.Error = ConnectionFailed, err.Error()
		c.retryAt = time.Now().Add(retryDelay)
		c.autoBlocked = c.autoBlocked || !force
	} else {
		c.retryAt = time.Time{}
		c.autoBlocked = false
	}
	c.pending = nil
	close(pending)
	c.mu.Unlock()
	c.notify()
}

// Resolve only known installation metadata and bounded locations; never scan
// disks on the input path. An explicit user selection takes precedence.
func resolveExecutable(configured string) (string, error) {
	valid := func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && !info.IsDir() && strings.EqualFold(filepath.Ext(path), ".exe")
	}
	if configured != "" {
		if !filepath.IsAbs(configured) || !valid(configured) {
			return "", fmt.Errorf("%s: %s", locales.EverythingInvalidPath(), configured)
		}
		return filepath.Clean(configured), nil
	}
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			for _, subkey := range []string{`Software\Microsoft\Windows\CurrentVersion\App Paths\Everything.exe`, `Software\Microsoft\Windows\CurrentVersion\Uninstall\Everything`} {
				key, err := registry.OpenKey(root, subkey, registry.QUERY_VALUE|view)
				if err != nil {
					continue
				}
				for _, name := range []string{"", "InstallLocation"} {
					value, _, _ := key.GetStringValue(name)
					value = strings.Trim(value, "\"")
					if name == "InstallLocation" && value != "" {
						value = filepath.Join(value, "Everything.exe")
					}
					if filepath.IsAbs(value) && valid(value) {
						key.Close()
						return value, nil
					}
				}
				key.Close()
			}
		}
	}
	for _, root := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LOCALAPPDATA")} {
		if root != "" {
			path := filepath.Join(root, "Everything", "Everything.exe")
			if valid(path) {
				return path, nil
			}
		}
	}
	if path, err := exec.LookPath("Everything.exe"); err == nil && filepath.IsAbs(path) && valid(path) {
		return path, nil
	}
	return "", errors.New(locales.EverythingNotFound())
}

// requiresAdmin reports whether Everything is set to "Run as administrator",
// which makes the client relaunch itself elevated behind a UAC prompt.
// Settings live next to a portable executable, or in %APPDATA% when the
// local Everything.ini is absent or sets app_data=1.
func requiresAdmin(exe string) bool {
	local, err := readSmallFile(filepath.Join(filepath.Dir(exe), "Everything.ini"))
	if err == nil && iniValue(local, "app_data") != "1" {
		return iniValue(local, "run_as_admin") == "1"
	}
	roaming, err := readSmallFile(filepath.Join(os.Getenv("APPDATA"), "Everything", "Everything.ini"))
	return err == nil && iniValue(roaming, "run_as_admin") == "1"
}

func readSmallFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > 1<<20 {
		return "", errors.New("Everything.ini too large")
	}
	data, err := os.ReadFile(path)
	return string(data), err
}

// iniValue returns the first value of key in Everything.ini, which is a
// flat key=value list after its [Everything] section header.
func iniValue(text, key string) string {
	text = strings.TrimPrefix(text, "\uFEFF")
	for _, line := range strings.Split(text, "\n") {
		name, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.EqualFold(strings.TrimSpace(name), key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// fixAdminSteps are Everything's own switches for the supported setup that
// needs no elevation: the client runs as the user and the Everything service
// reads the volumes. Each requires administrator rights, so each raises one
// UAC prompt; -install-service also starts the service and sets it to start
// with Windows.
var fixAdminSteps = []string{"-disable-run-as-admin", "-install-service"}

// FixAdmin turns off "Run as administrator" and installs the service, then
// starts the client without elevation and verifies IPC. Everything ignores
// -disable-run-as-admin while it is running, so the client must be closed.
func (p *Provider) FixAdmin(ctx context.Context, options settings.FileIndexOptions) error {
	c := p.connection
	path, err := c.resolve(options.EverythingPath)
	if err != nil {
		return err
	}
	if c.running() {
		return errors.New(locales.EverythingFixAdminExitFirst())
	}
	for _, step := range fixAdminSteps {
		if err := c.elevate(ctx, path, step); err != nil {
			if errors.Is(err, apps.ErrUACCanceled) {
				return errors.New(locales.LaunchUACCanceled())
			}
			return fmt.Errorf("%s: %w", locales.EverythingFixAdminFailed(), err)
		}
	}
	c.mu.Lock()
	c.retryAt, c.autoBlocked = time.Time{}, false
	c.mu.Unlock()
	return p.ConnectWithOptions(ctx, options)
}

func launchEverything(path string, show bool) error {
	args := []string{"-startup"}
	if show {
		args = nil
	}
	command := exec.Command(path, args...)
	command.Dir = filepath.Dir(path)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: !show}
	if err := command.Start(); err != nil {
		return fmt.Errorf("%s: %w", locales.EverythingStartFailed(), err)
	}
	// Reap the process when it eventually exits, without owning its lifetime.
	go func() { _ = command.Wait() }()
	return nil
}
