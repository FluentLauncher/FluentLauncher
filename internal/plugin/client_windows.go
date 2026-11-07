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
)

// Client runs one plugin executable and speaks the framed JSON-RPC protocol.
type Client struct {
	executable string
	id         string

	opMu      sync.Mutex
	writeMu   sync.Mutex
	stderrMu  sync.Mutex
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    *bufio.Reader
	job       *job
	stderr    string
	errText   string
	lastUsed  time.Time
	startupMs float64
	queryMs   float64
	requestID int64
	closed    bool
}

func NewClient(executable, id string) *Client {
	return &Client{executable: executable, id: id}
}

func (c *Client) Query(ctx context.Context, query protocol.Query, timeout time.Duration) (protocol.QueryResult, error) {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	start := time.Now()
	defer func() { c.queryMs = float64(time.Since(start).Microseconds()) / 1000 }()
	if err := c.startLocked(ctx); err != nil {
		return protocol.QueryResult{}, err
	}
	raw, err := c.requestLocked(ctx, "query", query, timeout)
	if err != nil {
		c.errText = err.Error()
		c.stopLocked()
		return protocol.QueryResult{}, err
	}
	var result protocol.QueryResult
	if err := json.Unmarshal(raw, &result); err != nil {
		c.errText = err.Error()
		c.stopLocked()
		return protocol.QueryResult{}, err
	}
	if result.Results == nil {
		result.Results = []protocol.Result{}
	}
	c.errText = ""
	c.lastUsed = time.Now()
	return result, nil
}

func (c *Client) Execute(ctx context.Context, request protocol.ExecuteRequest, timeout time.Duration) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if c.cmd == nil || c.cmd.Process == nil {
		return errors.New("插件已回收，请重新查询后执行")
	}
	if _, err := c.requestLocked(ctx, "execute", request, timeout); err != nil {
		c.errText = err.Error()
		c.stopLocked()
		return err
	}
	c.lastUsed = time.Now()
	return nil
}

func (c *Client) startLocked(ctx context.Context) error {
	if c.closed {
		return errors.New("插件客户端已关闭")
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return nil
	}
	c.stopLocked()
	start := time.Now()
	command := exec.Command(c.executable)
	command.Dir = filepathDir(c.executable)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	stdin, err := command.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return err
	}
	job, err := newJob()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		job.Close()
		return err
	}
	if err := job.assign(command.Process); err != nil {
		job.Close()
		_ = command.Process.Kill()
		return err
	}
	c.cmd, c.stdin, c.stdout, c.job = command, stdin, bufio.NewReaderSize(stdout, 64*1024), job
	c.stderr = ""
	go c.readStderr(stderr)
	raw, err := c.requestLocked(ctx, "initialize", nil, 5*time.Second)
	if err != nil {
		c.stopLocked()
		return err
	}
	var initialized protocol.InitializeResult
	if err := json.Unmarshal(raw, &initialized); err != nil || initialized.ProtocolVersion != 1 || initialized.PluginID != c.id {
		c.stopLocked()
		return errors.New("插件身份或协议握手不匹配")
	}
	c.startupMs = float64(time.Since(start).Microseconds()) / 1000
	return nil
}

func (c *Client) requestLocked(ctx context.Context, method string, params any, timeout time.Duration) (json.RawMessage, error) {
	if c.cmd == nil || c.stdin == nil || c.stdout == nil {
		return nil, errors.New("插件进程未启动")
	}
	id := atomic.AddInt64(&c.requestID, 1)
	var rawParams json.RawMessage
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		rawParams = data
	}
	request := protocol.Request{JSONRPC: "2.0", ID: json.RawMessage(fmt.Sprintf("%d", id)), Method: method, Params: rawParams}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	c.writeMu.Lock()
	err = protocol.Write(c.stdin, payload)
	c.writeMu.Unlock()
	if err != nil {
		return nil, err
	}

	type readResult struct {
		payload []byte
		err     error
	}
	read := make(chan readResult, 1)
	go func() {
		payload, err := protocol.Read(c.stdout)
		read <- readResult{payload, err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-read:
		if result.err != nil {
			if errors.Is(result.err, io.EOF) {
				return nil, errors.New("插件已退出")
			}
			return nil, result.err
		}
		var response struct {
			JSONRPC string             `json:"jsonrpc"`
			ID      json.RawMessage    `json:"id"`
			Result  json.RawMessage    `json:"result"`
			Error   *protocol.RPCError `json:"error"`
		}
		if err := json.Unmarshal(result.payload, &response); err != nil {
			return nil, err
		}
		if response.JSONRPC != "2.0" || string(response.ID) != string(request.ID) {
			return nil, errors.New("插件响应 ID 不匹配")
		}
		if response.Error != nil {
			return nil, response.Error
		}
		return response.Result, nil
	case <-ctx.Done():
		c.cancelLocked(id, timeout)
		return nil, ctx.Err()
	case <-timer.C:
		c.cancelLocked(id, timeout)
		return nil, fmt.Errorf("插件 %s 超时", method)
	}
}

func (c *Client) cancelLocked(id int64, timeout time.Duration) {
	c.writeMu.Lock()
	_ = protocol.WriteJSON(c.stdin, protocol.Request{
		JSONRPC: "2.0",
		Method:  "$/cancelRequest",
		Params:  json.RawMessage(fmt.Sprintf(`{"id":%d}`, id)),
	})
	c.writeMu.Unlock()
	time.Sleep(250 * time.Millisecond)
	c.stopLocked()
	_ = timeout
}

func (c *Client) readStderr(reader io.Reader) {
	buffer := make([]byte, 1024)
	for {
		n, err := reader.Read(buffer)
		if n > 0 {
			c.stderrMu.Lock()
			c.stderr += string(buffer[:n])
			if len(c.stderr) > 8192 {
				c.stderr = c.stderr[len(c.stderr)-8192:]
			}
			c.stderrMu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (c *Client) Diagnostics() Diagnostics {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	pid := 0
	if c.cmd != nil && c.cmd.Process != nil {
		pid = c.cmd.Process.Pid
	}
	c.stderrMu.Lock()
	stderr := c.stderr
	c.stderrMu.Unlock()
	return Diagnostics{ID: c.id, PID: pid, StartupMs: c.startupMs, QueryMs: c.queryMs, Error: c.errText, Stderr: stderr}
}

func (c *Client) LastUsed() time.Time {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.lastUsed
}

func (c *Client) Close() {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.closed = true
	c.stopLocked()
}

func (c *Client) stopLocked() {
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
}

func filepathDir(path string) string {
	return filepath.Dir(path)
}

type Diagnostics struct {
	ID        string
	PID       int
	StartupMs float64
	QueryMs   float64
	Error     string
	Stderr    string
}
