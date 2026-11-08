//go:build windows

// Package everything queries the Everything client through its Unicode
// WM_COPYDATA IPC, without shipping an SDK DLL.
package everything

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/search"
	"fluentlauncher/locales"
)

const (
	wmCopyData      = 0x004A
	everythingClass = "EVERYTHING_TASKBAR_NOTIFICATION"
	everythingTag   = 0x464C
	maxReplyBytes   = 4 << 20
	sendTimeout     = 1000
	queryTimeout    = 1500 * time.Millisecond
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassW      = user32.NewProc("RegisterClassW")
	procUnregisterClassW    = user32.NewProc("UnregisterClassW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procFindWindowW         = user32.NewProc("FindWindowW")
	procSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
)

// Provider is the optional Everything file-name source.
type Provider struct {
	Enabled func() bool
	mu      sync.Mutex
}

func (p *Provider) ID() string { return "everything" }

func (p *Provider) Supports(q search.Query) bool {
	return p.Enabled != nil && p.Enabled() && (q.Scope == search.ScopeAll || q.Scope == search.ScopeFiles) && q.Text != ""
}

func (p *Provider) Search(ctx context.Context, q search.Query, limit int) ([]model.Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	return query(ctx, q.Text, limit)
}

type wndClass struct {
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
}

type copyData struct {
	Tag    uintptr
	Length int32
	Data   unsafe.Pointer
}

type msg struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       struct{ X, Y int32 }
	LPrivate uint32
}

type queryState struct {
	limit   int
	results []model.Result
	err     error
	done    bool
}

// queryStates maps the query window to its state for the shared window
// procedure. The callback is registered once for the whole process: the
// runtime's callback table is capped (2000 slots, never recycled), and a
// NewCallback per search would exhaust it after a couple of thousand
// keystrokes with Everything enabled.
var (
	queryStatesMu sync.Mutex
	queryStates   = map[uintptr]*queryState{}
)

var queryCallback = syscall.NewCallback(func(hwnd uintptr, message uint32, wParam uintptr, lParam unsafe.Pointer) uintptr {
	if message != wmCopyData {
		r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, uintptr(lParam))
		return r
	}
	data := (*copyData)(lParam)
	queryStatesMu.Lock()
	state := queryStates[hwnd]
	queryStatesMu.Unlock()
	if state != nil && data.Tag == everythingTag {
		if data.Length < 28 || data.Length > maxReplyBytes || data.Data == nil {
			state.err = errors.New(locales.EverythingReplySizeInvalid())
		} else {
			payload := unsafe.Slice((*byte)(data.Data), int(data.Length))
			state.results, state.err = parseResults(append([]byte(nil), payload...), state.limit)
		}
		state.done = true
	}
	procPostMessageW.Call(hwnd, 0, 0, 0)
	return 1
})

func query(ctx context.Context, text string, limit int) ([]model.Result, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	everythingName, _ := windows.UTF16PtrFromString(everythingClass)
	everything, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(everythingName)), 0)
	if everything == 0 {
		return nil, errors.New(locales.EverythingClientNotRunning())
	}
	className := fmt.Sprintf("FluentLauncher.Everything.%d", time.Now().UnixNano())
	classPtr, _ := windows.UTF16PtrFromString(className)
	hinstance, _, _ := procGetModuleHandleW.Call(0)
	state := &queryState{limit: limit}
	class := wndClass{
		WndProc:   queryCallback,
		Instance:  hinstance,
		ClassName: classPtr,
	}
	if atom, _, err := procRegisterClassW.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
		return nil, fmt.Errorf("RegisterClassW: %v", err)
	}
	defer procUnregisterClassW.Call(uintptr(unsafe.Pointer(classPtr)), hinstance)

	hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(classPtr)), 0, 0,
		0, 0, 0, 0, ^uintptr(2), 0, hinstance, 0)
	if hwnd == 0 {
		return nil, fmt.Errorf("CreateWindowExW: %v", err)
	}
	queryStatesMu.Lock()
	queryStates[hwnd] = state
	queryStatesMu.Unlock()
	defer func() {
		queryStatesMu.Lock()
		delete(queryStates, hwnd)
		queryStatesMu.Unlock()
	}()
	defer procDestroyWindow.Call(hwnd)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			procPostMessageW.Call(hwnd, 0, 0, 0)
		case <-done:
		}
	}()

	// The payload carries the query as a terminated UTF-16 string. The
	// terminator is appended after encoding: StringToUTF16 panics on a NUL
	// inside its argument.
	encoded := append(windows.StringToUTF16(text), 0)
	payload := make([]byte, 20+len(encoded)*2)
	binary.LittleEndian.PutUint32(payload[0:], uint32(hwnd))
	binary.LittleEndian.PutUint32(payload[4:], everythingTag)
	binary.LittleEndian.PutUint32(payload[16:], uint32(limit))
	for i, r := range encoded {
		binary.LittleEndian.PutUint16(payload[20+i*2:], r)
	}
	data := copyData{Tag: 2, Length: int32(len(payload)), Data: unsafe.Pointer(&payload[0])}
	var accepted uintptr
	r, _, _ := procSendMessageTimeoutW.Call(everything, wmCopyData, hwnd,
		uintptr(unsafe.Pointer(&data)), 2, sendTimeout, uintptr(unsafe.Pointer(&accepted)))
	if r == 0 || accepted == 0 {
		return nil, errors.New(locales.EverythingQueryRejected())
	}

	for !state.done {
		if err := ctx.Err(); err != nil {
			procPostMessageW.Call(hwnd, 0, 0, 0)
			return nil, err
		}
		var m msg
		got, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(got) <= 0 {
			return nil, errors.New(locales.EverythingMessageLoopExited())
		}
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	if state.err != nil {
		return nil, state.err
	}
	return state.results, nil
}

func parseResults(payload []byte, limit int) ([]model.Result, error) {
	if len(payload) < 28 {
		return nil, errors.New(locales.EverythingResultHeaderIncomplete())
	}
	count := int(int32(binary.LittleEndian.Uint32(payload[20:])))
	if count < 0 || count > limit || 28+count*12 > len(payload) {
		return nil, errors.New(locales.EverythingResultCountInvalid())
	}
	results := make([]model.Result, 0, count)
	for i := 0; i < count; i++ {
		pos := 28 + i*12
		flags := binary.LittleEndian.Uint32(payload[pos:])
		name, err := readText(payload, int(int32(binary.LittleEndian.Uint32(payload[pos+4:]))), 28+count*12)
		if err != nil {
			return nil, err
		}
		dir, err := readText(payload, int(int32(binary.LittleEndian.Uint32(payload[pos+8:]))), 28+count*12)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(dir, name)
		if !filepath.IsAbs(path) {
			continue
		}
		kind := model.File
		if flags&1 != 0 {
			kind = model.Directory
		}
		results = append(results, model.Result{Entry: model.Entry{
			ID:           "file:" + strings.ToUpper(path),
			Title:        name,
			Subtitle:     path,
			LaunchTarget: path,
			Kind:         kind,
			SourceID:     "everything",
			IconPath:     path,
		}, Score: 5000})
	}
	return results, nil
}

func readText(data []byte, offset, minimum int) (string, error) {
	if offset < minimum || offset%2 != 0 || offset >= len(data) {
		return "", errors.New(locales.EverythingStringOffsetInvalid())
	}
	for end := offset; end+1 < len(data); end += 2 {
		if data[end] == 0 && data[end+1] == 0 {
			return windows.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&data[offset])), (end-offset)/2)), nil
		}
	}
	return "", errors.New(locales.EverythingStringNotTerminated())
}
