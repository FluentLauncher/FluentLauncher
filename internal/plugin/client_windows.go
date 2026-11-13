//go:build windows

package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"fluentlauncher/internal/plugin/protocol"
	"fluentlauncher/internal/plugin/rpc"
	"fluentlauncher/locales"
)

const (
	// maxStderrBytes caps the error output kept per plugin: enough for a
	// panic, small enough that the page can print it.
	maxStderrBytes = 8192
	// handshakeTimeout bounds the initialize round trip, which is separate
	// from the query timeout: a plugin that starts slowly still gets a fair
	// chance to introduce itself.
	handshakeTimeout = 5 * time.Second
	// cancelGrace is how long a canceled request may take to answer before
	// its process is killed instead of being kept for the next search.
	cancelGrace = 400 * time.Millisecond
)

// Client runs one plugin executable and speaks the framed JSON-RPC protocol.
type Client struct {
	executable string
	id         string

	// The operation gate is cancellable while waiting. Explicit actions share
	// it with queries but are never superseded by a later search.
	opGate         chan struct{}
	queryMu        sync.Mutex
	querySequence  uint64
	pendingQuery   context.CancelFunc
	stderrMu       sync.Mutex
	cmd            *exec.Cmd
	stdin          io.WriteCloser
	stdout         *bufio.Reader
	job            *job
	stderr         string
	closed         bool
	peer           atomic.Pointer[rpc.Peer]
	host           HostProvider
	requiresHost   bool
	hostReady      atomic.Bool
	interactive    atomic.Bool
	settingsEvents atomic.Bool
	capabilities   []string
	themeMu        sync.Mutex
	theme          *protocol.Theme
	sentTheme      *protocol.Theme // operation-gate protected

	// statsMu guards stats alone, so the settings page can read a snapshot
	// while a query is in flight instead of waiting for it.
	statsMu sync.Mutex
	stats   ClientStats
}

func NewClient(executable, id string) *Client {
	return &Client{executable: executable, id: id, opGate: make(chan struct{}, 1)}
}

func (c *Client) lock(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case c.opGate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-c.opGate
			return err
		}
		return nil
	}
}

// Stats is a snapshot of the process, taken without waiting for a request in
// flight. It is what the settings page draws, so it never blocks on a plugin.
func (c *Client) Stats() ClientStats {
	c.statsMu.Lock()
	stats := c.stats
	c.statsMu.Unlock()
	c.stderrMu.Lock()
	stats.Stderr = c.stderr
	c.stderrMu.Unlock()
	return stats
}

func (c *Client) Query(ctx context.Context, query protocol.Query, timeout time.Duration) (protocol.QueryResult, error) {
	if err := ctx.Err(); err != nil {
		return protocol.QueryResult{}, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c.queryMu.Lock()
	if err := ctx.Err(); err != nil {
		c.queryMu.Unlock()
		return protocol.QueryResult{}, err
	}
	if c.pendingQuery != nil {
		c.pendingQuery()
	}
	c.querySequence++
	sequence := c.querySequence
	c.pendingQuery = cancel
	c.queryMu.Unlock()
	if err := c.lock(ctx); err != nil {
		return protocol.QueryResult{}, err
	}
	defer func() { <-c.opGate }()
	c.queryMu.Lock()
	current := sequence == c.querySequence
	if current {
		c.pendingQuery = nil
	}
	c.queryMu.Unlock()
	if !current {
		return protocol.QueryResult{}, context.Canceled
	}
	if err := c.startLocked(ctx); err != nil {
		c.fail(err, ctx)
		return protocol.QueryResult{}, err
	}
	start := time.Now()
	raw, err := c.requestLocked(ctx, "query", reqQuery, query, timeout)
	if err != nil {
		c.fail(err, ctx)
		if ctx.Err() == nil {
			c.stopLocked()
		}
		return protocol.QueryResult{}, err
	}
	var result protocol.QueryResult
	if err := json.Unmarshal(raw, &result); err != nil {
		wrapped := errors.New(locales.PluginQueryResultInvalid(err))
		c.fail(wrapped, ctx)
		c.stopLocked()
		return protocol.QueryResult{}, wrapped
	}
	if result.Results == nil {
		result.Results = []protocol.Result{}
	}
	c.succeed(time.Since(start))
	return result, nil
}

func (c *Client) Execute(ctx context.Context, request protocol.ExecuteRequest, timeout time.Duration) error {
	if err := c.lock(ctx); err != nil {
		return err
	}
	defer func() { <-c.opGate }()
	if err := c.startLocked(ctx); err != nil {
		c.fail(err, ctx)
		return errors.New(locales.PluginExecuteStartFailed(err))
	}
	c.interactive.Store(true)
	defer c.interactive.Store(false)
	if c.hostReady.Load() {
		timeout = max(timeout, 2*time.Minute)
	}
	if _, err := c.requestLocked(ctx, "execute", reqExecute, request, timeout); err != nil {
		c.fail(err, ctx)
		if ctx.Err() == nil {
			c.stopLocked()
		}
		return err
	}
	c.statsMu.Lock()
	c.stats.LastUsed = time.Now()
	c.stats.Error = ""
	c.statsMu.Unlock()
	return nil
}

func (c *Client) ExecuteCommand(ctx context.Context, commandID string, timeout time.Duration) error {
	if err := c.lock(ctx); err != nil {
		return err
	}
	defer func() { <-c.opGate }()
	if err := c.startLocked(ctx); err != nil {
		c.fail(err, ctx)
		return err
	}
	if !containsFold(c.capabilities, "executeCommand") {
		err := fmt.Errorf("plugin %s does not support executeCommand; update the plugin", c.id)
		c.fail(err, ctx)
		return err
	}
	c.interactive.Store(true)
	defer c.interactive.Store(false)
	if c.hostReady.Load() {
		timeout = max(timeout, 2*time.Minute)
	}
	_, err := c.requestLocked(ctx, "executeCommand", reqExecute, protocol.ExecuteCommandRequest{CommandID: commandID}, timeout)
	if err != nil {
		c.fail(err, ctx)
		if ctx.Err() == nil {
			c.stopLocked()
		}
		return err
	}
	c.statsMu.Lock()
	c.stats.LastUsed = time.Now()
	c.stats.Error = ""
	c.statsMu.Unlock()
	return nil
}

// HasActivity never starts a process. Older plugins retain the original idle
// policy; capable plugins may keep live UI until their last window is closed.
func (c *Client) HasActivity(ctx context.Context) (bool, error) {
	if err := c.lock(ctx); err != nil {
		return false, err
	}
	defer func() { <-c.opGate }()
	if c.cmd == nil || !containsFold(c.capabilities, "activity") {
		return false, nil
	}
	raw, err := c.requestLocked(ctx, "activity", reqExecute, nil, time.Second)
	if err != nil {
		c.fail(err, ctx)
		return false, err
	}
	var result struct {
		Active *bool `json:"active"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || result.Active == nil {
		return false, errors.New("invalid plugin activity response")
	}
	return *result.Active, nil
}

// succeed records a completed request. QueryMs is the round trip alone:
// startup is reported separately, so a slow first search does not look like
// a slow plugin.
func (c *Client) succeed(elapsed time.Duration) {
	c.statsMu.Lock()
	c.stats.QueryMs = float64(elapsed.Microseconds()) / 1000
	c.stats.LastUsed = time.Now()
	c.stats.Error = ""
	c.statsMu.Unlock()
}

// fail records why an attempt failed, so the page can still explain a plugin
// whose process is already gone. A canceled search is not a failure: the user
// simply typed on.
func (c *Client) fail(err error, ctx context.Context) {
	if err == nil || ctx.Err() != nil {
		return
	}
	c.statsMu.Lock()
	c.stats.Error = err.Error()
	c.statsMu.Unlock()
}

func (c *Client) startLocked(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.closed {
		return errors.New(locales.PluginClientClosed())
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return c.syncThemeLocked(ctx)
	}
	c.stopLocked()
	start := time.Now()
	command := exec.Command(c.executable)
	command.Dir = filepathDir(c.executable)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	stdin, err := command.StdinPipe()
	if err != nil {
		return errors.New(locales.PluginStdinPipeFailed(err))
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return errors.New(locales.PluginStdoutPipeFailed(err))
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return errors.New(locales.PluginStderrPipeFailed(err))
	}
	job, err := newJob()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		job.Close()
		return errors.New(locales.PluginProcessStartFailed(err))
	}
	if err := job.assign(command.Process); err != nil {
		job.Close()
		_ = command.Process.Kill()
		return err
	}
	c.cmd, c.stdin, c.stdout, c.job = command, stdin, bufio.NewReaderSize(stdout, 64*1024), job
	c.hostReady.Store(false)
	provider := c.host
	c.peer.Store(rpc.New(c.stdout, stdin, "h:", func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		if provider == nil || !c.hostReady.Load() {
			return nil, &rpc.Error{Code: -32601, Message: "host services are unavailable"}
		}
		return provider.Call(ctx, method, params, c.interactive.Load())
	}))
	c.stderrMu.Lock()
	c.stderr = ""
	c.stderrMu.Unlock()
	go c.readStderr(stderr)
	raw, err := c.requestLocked(ctx, "initialize", reqInitialize, nil, handshakeTimeout)
	if err != nil {
		c.stopLocked()
		return errors.New(locales.PluginHandshakeFailed(err))
	}
	var initialized protocol.InitializeResult
	if err := json.Unmarshal(raw, &initialized); err != nil {
		c.stopLocked()
		return errors.New(locales.PluginHandshakeReplyInvalid())
	}
	if err := handshakeError(initialized, c.id); err != nil {
		c.stopLocked()
		return err
	}
	c.capabilities = append([]string(nil), initialized.Capabilities...)
	if c.requiresHost && (c.host == nil || !containsFold(c.capabilities, "hostServices.v1")) {
		c.stopLocked()
		return fmt.Errorf("plugin %s requires negotiated host services; update the plugin and launcher", c.id)
	}
	c.settingsEvents.Store(containsFold(c.capabilities, "settings"))
	if c.host != nil && containsFold(c.capabilities, "hostServices.v1") {
		configuration, err := c.host.Configuration()
		if err != nil {
			c.stopLocked()
			return err
		}
		c.hostReady.Store(true)
		if _, err := c.requestLocked(ctx, "configureHost", reqInitialize, configuration, handshakeTimeout); err != nil {
			c.stopLocked()
			return err
		}
	}
	c.sentTheme = nil
	if err := c.syncThemeLocked(ctx); err != nil {
		c.stopLocked()
		return err
	}
	startupMs := float64(time.Since(start).Microseconds()) / 1000
	c.statsMu.Lock()
	c.stats.Running = true
	c.stats.PID = command.Process.Pid
	c.stats.StartupMs = startupMs
	c.stats.Error = ""
	c.statsMu.Unlock()
	return nil
}

// requestText is the set of messages one kind of request uses when it fails:
// the kind names the request in what the user sees, which is why it is
// separate from the wire method: "插件查询超时" tells more than "plugin query
// timed out".
type requestText struct {
	send    func(err error) string
	timeout func(ms int64) string
	cancel  func() string
}

var (
	reqQuery = requestText{
		send:    func(err error) string { return locales.PluginSendQueryFailed(err) },
		timeout: func(ms int64) string { return locales.PluginQueryTimeout(int(ms)) },
		cancel:  func() string { return locales.PluginQueryCanceled() },
	}
	reqExecute = requestText{
		send:    func(err error) string { return locales.PluginSendExecuteFailed(err) },
		timeout: func(ms int64) string { return locales.PluginExecuteTimeout(int(ms)) },
		cancel:  func() string { return locales.PluginExecuteCanceled() },
	}
	reqInitialize = requestText{
		send:    func(err error) string { return locales.PluginSendInitializeFailed(err) },
		timeout: func(ms int64) string { return locales.PluginInitializeTimeout(int(ms)) },
		cancel:  func() string { return locales.PluginInitializeCanceled() },
	}
)

// requestLocked waits on the session peer; only its reader consumes stdout.
func (c *Client) requestLocked(ctx context.Context, method string, text requestText, params any, timeout time.Duration) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	peer := c.peer.Load()
	if peer == nil {
		return nil, errors.New(locales.PluginProcessNotStarted())
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := peer.Call(requestCtx, method, params, cancelGrace)
	if err == nil {
		return result, nil
	}
	if errors.Is(err, rpc.ErrUnresponsive) {
		c.stopLocked()
	}
	if ctx.Err() != nil {
		return nil, errors.New(text.cancel())
	}
	if requestCtx.Err() != nil {
		c.stopLocked()
		return nil, errors.New(text.timeout(timeout.Milliseconds()))
	}
	if errors.Is(err, io.EOF) {
		return nil, errors.New(locales.PluginExitedNoResult())
	}
	return nil, err
}

func (c *Client) readStderr(reader io.Reader) {
	buffer := make([]byte, 1024)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			c.stderrMu.Lock()
			c.stderr += string(buffer[:n])
			if len(c.stderr) > maxStderrBytes {
				c.stderr = c.stderr[len(c.stderr)-maxStderrBytes:]
			}
			c.stderrMu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (c *Client) Close() {
	if peer := c.peer.Load(); peer != nil {
		peer.Close(io.EOF)
	}
	c.queryMu.Lock()
	if c.pendingQuery != nil {
		c.pendingQuery()
		c.pendingQuery = nil
	}
	c.queryMu.Unlock()
	_ = c.lock(context.Background())
	defer func() { <-c.opGate }()
	c.closed = true
	c.closeGracefullyLocked()
}

// Give UI plugins a bounded opportunity to release windows and temporary
// exports on EOF. Protocol failures/timeouts still use immediate stopLocked.
func (c *Client) closeGracefullyLocked() {
	if c.cmd == nil {
		c.stopLocked()
		return
	}
	if peer := c.peer.Swap(nil); peer != nil {
		peer.Close(io.EOF)
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
		c.stdin = nil
	}
	cmd := c.cmd
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	timer := time.NewTimer(cancelGrace)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		if c.job != nil {
			_ = c.job.Close()
			c.job = nil
		}
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
	}
	if c.job != nil {
		_ = c.job.Close()
		c.job = nil
	}
	c.cmd, c.stdin, c.stdout = nil, nil, nil
	c.statsMu.Lock()
	c.stats.Running = false
	c.stats.PID = 0
	c.statsMu.Unlock()
}

func (c *Client) stopLocked() {
	if peer := c.peer.Swap(nil); peer != nil {
		peer.Close(io.EOF)
	}
	if c.job != nil {
		_ = c.job.Close()
		c.job = nil
	}
	if c.cmd != nil {
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		_ = c.cmd.Wait()
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	c.cmd, c.stdin, c.stdout = nil, nil, nil
	c.statsMu.Lock()
	c.stats.Running = false
	c.stats.PID = 0
	c.statsMu.Unlock()
}

func filepathDir(path string) string {
	return filepath.Dir(path)
}
