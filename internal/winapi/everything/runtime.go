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
	"fluentlauncher/locales"
)

type ConnectionState uint8

const (
	Disconnected ConnectionState = iota
	Starting
	Connected
	ConnectionFailed
)

type ConnectionStatus struct {
	State ConnectionState
	Path  string
	Error string
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
	changed    func()
	running    func() bool
	resolve    func(string) (string, error)
	launch     func(string, bool) error
}

func newConnection(changed func()) *connection {
	c := &connection{changed: changed, running: clientRunning, launch: launchEverything}
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
	if c.running() {
		changed := c.status.State != Connected
		c.status.State, c.status.Error = Connected, ""
		c.mu.Unlock()
		if changed {
			c.notify()
		}
		return nil
	}
	if !force && !options.EverythingAutoStart && c.pending == nil {
		c.status.State, c.status.Error = Disconnected, locales.EverythingClientNotRunning()
		c.mu.Unlock()
		return errors.New(locales.EverythingClientNotRunning())
	}
	if c.pending == nil {
		if !force && options.EverythingPath == c.lastPath && time.Now().Before(c.retryAt) {
			err := c.status.Error
			c.mu.Unlock()
			return errors.New(err)
		}
		c.pending = make(chan struct{})
		c.lastPath = options.EverythingPath
		c.status = ConnectionStatus{State: Starting}
		go c.start(options.EverythingPath, c.pending)
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

func (c *connection) start(configured string, pending chan struct{}) {
	path, err := c.resolve(configured)
	if err == nil && !c.running() {
		err = c.launch(path, false)
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
	c.status = ConnectionStatus{State: Connected, Path: path}
	if err != nil {
		c.status.State, c.status.Error = ConnectionFailed, err.Error()
		c.retryAt = time.Now().Add(15 * time.Second)
	} else {
		c.retryAt = time.Time{}
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
