package diag

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxLogBytes     = 5 << 20
	logFiles        = 5
	queueCapacity   = 512
	priorityReserve = 64
)

type logRequest struct {
	data    []byte
	barrier func()
}

// Recorder owns a bounded, non-blocking log queue. Only its worker touches
// files; UI callers never wait for disk, rotation, or diagnostic exports.
type Recorder struct {
	dir         string
	queue       chan logRequest
	done        chan struct{}
	mu          sync.Mutex
	closed      bool
	dropped     atomic.Uint64
	writeErrors atomic.Uint64
	until       atomic.Int64
	Logger      *slog.Logger
	file        *os.File
	buffer      *bufio.Writer
	size        int64
	day         string
	limit       int64
	stop        chan struct{}
	timings     Timings
}

func Open(dir string) (*Recorder, error) {
	return open(dir, maxLogBytes)
}

func open(dir string, limit int64) (*Recorder, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	r := &Recorder{dir: dir, queue: make(chan logRequest, queueCapacity), done: make(chan struct{}), stop: make(chan struct{}), limit: limit}
	for i := 0; i < logFiles; i++ {
		path := r.path(i)
		if stat, err := os.Stat(path); err == nil && time.Since(stat.ModTime()) > 7*24*time.Hour {
			if err := os.Remove(path); err != nil {
				return nil, err
			}
		}
	}
	if err := r.openFile(); err != nil {
		return nil, err
	}
	r.Logger = slog.New(slog.NewJSONHandler(r, &slog.HandlerOptions{Level: diagnosticLevel{r}})).With("session", time.Now().UTC().Format("20060102T150405.000000000Z"))
	go r.run()
	return r, nil
}

type diagnosticLevel struct{ r *Recorder }

func (l diagnosticLevel) Level() slog.Level {
	if l.r.Active() {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

func (r *Recorder) Start() {
	r.timings.mu.Lock()
	r.timings.values = nil
	r.timings.mu.Unlock()
	r.until.Store(time.Now().Add(5 * time.Minute).UnixNano())
	r.Logger.Info("diagnostics_started", "duration_seconds", 300)
}
func (r *Recorder) Stop()            { r.until.Store(0); r.Logger.Info("diagnostics_stopped") }
func (r *Recorder) Active() bool     { return time.Now().UnixNano() < r.until.Load() }
func (r *Recorder) Until() time.Time { return time.Unix(0, r.until.Load()) }
func (r *Recorder) Dir() string      { return r.dir }
func (r *Recorder) Healthy() bool    { return r.writeErrors.Load() == 0 }

func (r *Recorder) Write(data []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return len(data), nil
	}
	// Bound individual records as well as the queue. Unknown legacy output is
	// filtered before reaching this writer; structured events contain no payloads.
	if len(data) > 16<<10 {
		r.dropped.Add(1)
		return len(data), nil
	}
	if len(r.queue) >= queueCapacity-priorityReserve && !strings.Contains(string(data), `"level":"WARN"`) && !strings.Contains(string(data), `"level":"ERROR"`) {
		r.dropped.Add(1)
		return len(data), nil
	}
	select {
	case r.queue <- logRequest{data: append([]byte(nil), data...)}:
	default:
		r.dropped.Add(1)
	}
	return len(data), nil
}

func (r *Recorder) path(i int) string {
	return filepath.Join(r.dir, fmt.Sprintf("launcher.%d.jsonl", i))
}
func (r *Recorder) openFile() error {
	f, err := os.OpenFile(r.path(0), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	r.file, r.buffer, r.size = f, bufio.NewWriterSize(f, 32<<10), stat.Size()
	r.day = stat.ModTime().UTC().Format("2006-01-02")
	return nil
}
func (r *Recorder) flush() {
	if r.buffer != nil {
		if err := r.buffer.Flush(); err != nil {
			r.writeErrors.Add(1)
		}
	}
}
func (r *Recorder) rotate() error {
	r.flush()
	if r.file != nil {
		r.file.Close()
		r.file, r.buffer = nil, nil
	}
	if err := os.Remove(r.path(logFiles - 1)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for i := logFiles - 2; i >= 0; i-- {
		if err := os.Rename(r.path(i), r.path(i+1)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return r.openFile()
}
func (r *Recorder) consume(req logRequest) {
	if req.barrier != nil {
		r.flush()
		req.barrier()
		return
	}
	if r.size+int64(len(req.data)) > r.limit || r.day != time.Now().UTC().Format("2006-01-02") {
		if err := r.rotate(); err != nil {
			r.writeErrors.Add(1)
			return
		}
	}
	if r.buffer == nil {
		r.writeErrors.Add(1)
		return
	}
	n, err := r.buffer.Write(req.data)
	r.size += int64(n)
	if err != nil {
		r.writeErrors.Add(1)
	}
}
func (r *Recorder) run() {
	defer close(r.done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastCleanup := time.Now()
	for {
		select {
		case req := <-r.queue:
			r.consume(req)
		case <-ticker.C:
			r.flush()
			if deadline := r.until.Load(); deadline != 0 && time.Now().UnixNano() >= deadline && r.until.CompareAndSwap(deadline, 0) {
				r.Logger.Info("diagnostics_expired")
			}
			if time.Since(lastCleanup) >= time.Hour {
				lastCleanup = time.Now()
				for i := 1; i < logFiles; i++ {
					if stat, err := os.Stat(r.path(i)); err == nil && time.Since(stat.ModTime()) > 7*24*time.Hour {
						_ = os.Remove(r.path(i))
					}
				}
			}
		case <-r.stop:
			for len(r.queue) > 0 {
				r.consume(<-r.queue)
			}
			r.flush()
			if r.file != nil {
				r.file.Close()
			}
			return
		}
	}
}

// Close drains queued records with a bounded wait, even on a failed disk.
func (r *Recorder) Close() {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.stop)
	}
	r.mu.Unlock()
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
	}
}

// Install redirects standard library/MyGo logs without retaining arbitrary
// error text, which can contain search input, URLs, file names or plugin data.
// Known numeric frame statistics are retained for explicitly enabled profiling.
func (r *Recorder) Install() func() {
	previous, flags, prefix, logger := log.Writer(), log.Flags(), log.Prefix(), slog.Default()
	slog.SetDefault(r.Logger)
	log.SetOutput(legacyWriter{r})
	log.SetFlags(0)
	log.SetPrefix("")
	crashPath := filepath.Join(r.dir, "crash-current.log")
	previousCrash := filepath.Join(r.dir, "crash-previous.log")
	for _, path := range []string{crashPath, previousCrash} {
		if stat, err := os.Stat(path); err == nil && time.Since(stat.ModTime()) > 7*24*time.Hour {
			_ = os.Remove(path)
		}
	}
	if stat, err := os.Stat(crashPath); err == nil && stat.Size() > 0 {
		_ = os.Remove(previousCrash)
		_ = os.Rename(crashPath, previousCrash)
	}
	crash, err := os.OpenFile(crashPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err == nil {
		err = debug.SetCrashOutput(crash, debug.CrashOptions{})
		crash.Close() // runtime holds its own descriptor
	}
	if err != nil {
		r.Logger.Warn("crash_capture_unavailable")
	}
	return func() {
		_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
		slog.SetDefault(logger)
		log.SetOutput(previous)
		log.SetFlags(flags)
		log.SetPrefix(prefix)
	}
}

type legacyWriter struct{ r *Recorder }

func (w legacyWriter) Write(p []byte) (int, error) {
	text := strings.TrimSpace(string(p))
	if strings.HasPrefix(text, "mygo: frame ") {
		w.r.Logger.Debug("frame", "stats", text)
	} else {
		component := "runtime"
		for _, name := range []string{"file index", "apps", "anchor", "tray", "open at login", "theme", "settings icon", "background", "volume", "shortcuts"} {
			if strings.HasPrefix(text, name+":") {
				component = name
				break
			}
		}
		w.r.Logger.Warn("legacy_event", "component", component, "detail", "omitted_for_privacy")
	}
	return len(p), nil
}

// snapshot runs reads on the writer to exclude rotation races. The caller must
// be a worker; cancellation also bounds waiting behind a slow disk.
func (r *Recorder) snapshot(ctx context.Context) (map[string][]byte, error) {
	result := make(chan map[string][]byte, 1)
	req := logRequest{barrier: func() {
		files := map[string][]byte{}
		for i := 0; i < logFiles; i++ {
			if f, err := os.Open(r.path(i)); err == nil {
				data, readErr := io.ReadAll(io.LimitReader(f, maxLogBytes))
				f.Close()
				if readErr == nil {
					files[filepath.Base(r.path(i))] = data
				}
			}
		}
		result <- files
	}}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, errors.New("diagnostics closed")
	}
	select {
	case r.queue <- req:
		r.mu.Unlock()
	default:
		r.mu.Unlock()
		return nil, errors.New("diagnostic queue busy; retry")
	}
	select {
	case files := <-result:
		return files, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *Recorder) health() json.RawMessage {
	data, _ := json.Marshal(map[string]any{"dropped_records": r.dropped.Load(), "write_errors": r.writeErrors.Load(), "diagnostics_active": r.Active()})
	return data
}
