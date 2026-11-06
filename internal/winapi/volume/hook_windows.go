//go:build windows

package volume

import (
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	whMouseLL      = 14
	wmMouseWheel   = 0x020A
	wmQuit         = 0x0012
	monitorAll     = 0
	monitorNearest = 2
	dpiEffective   = 0
)

// Edges selects the screen edges that take the wheel. Bottom is the
// default; the other edges are for the settings UI.
type Edges uint8

const (
	EdgeBottom Edges = 1 << iota
	EdgeTop
	EdgeLeft
	EdgeRight
)

// Options configures the wheel listener.
type Options struct {
	Edges        Edges
	ThicknessDIP int
	StepPercent  int
	OnChange     func(percent int)
}

// Controller owns the low-level mouse hook and its worker.
type Controller struct {
	hook     uintptr
	thread   uint32
	stop     chan struct{}
	stopOnce sync.Once
	ch       chan int16
	opts     Options
	ready    chan error
}

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	shcore   = windows.NewLazySystemDLL("shcore.dll")

	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procMonitorFromPoint    = user32.NewProc("MonitorFromPoint")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	procPostThreadMessageW  = user32.NewProc("PostThreadMessageW")
	procGetCurrentThreadId  = kernel32.NewProc("GetCurrentThreadId")
	procGetDpiForMonitor    = shcore.NewProc("GetDpiForMonitor")
)

type point struct {
	X, Y int32
}

type rect struct {
	Left, Top, Right, Bottom int32
}

type monitorInfo struct {
	CbSize    uint32
	RcMonitor rect
	RcWork    rect
	DwFlags   uint32
}

type msllhookstruct struct {
	Pt          point
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type msg struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

// Start installs the hook on a dedicated, locked OS thread. The callback
// only checks the pointer against the selected edges and queues the wheel
// delta; volume changes happen on a worker.
func Start(opts Options) (*Controller, error) {
	if opts.ThicknessDIP <= 0 {
		opts.ThicknessDIP = 4
	}
	if opts.StepPercent <= 0 {
		opts.StepPercent = StepPercent
	}
	if opts.Edges == 0 {
		opts.Edges = EdgeBottom
	}
	c := &Controller{
		stop:  make(chan struct{}),
		ch:    make(chan int16, 16),
		opts:  opts,
		ready: make(chan error, 1),
	}
	go c.worker()
	go c.loop()
	if err := <-c.ready; err != nil {
		c.Stop()
		return nil, err
	}
	return c, nil
}

// Stop unhooks and ends the message loop and worker.
func (c *Controller) Stop() {
	c.stopOnce.Do(func() {
		close(c.stop)
		if c.hook != 0 {
			procUnhookWindowsHookEx.Call(c.hook)
		}
		if c.thread != 0 {
			procPostThreadMessageW.Call(uintptr(c.thread), wmQuit, 0, 0)
		}
	})
}

func (c *Controller) loop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	c.thread = uint32(getCurrentThreadID())
	hmod := uintptr(0)
	hook, _, err := procSetWindowsHookExW.Call(whMouseLL,
		syscall.NewCallback(c.hookProc), hmod, 0)
	if hook == 0 {
		c.ready <- fmt.Errorf("SetWindowsHookExW: %w", err)
		return
	}
	c.hook = hook
	c.ready <- nil

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (c *Controller) hookProc(nCode int32, wParam uintptr, lParam unsafe.Pointer) uintptr {
	if nCode < 0 {
		return callNext(nCode, wParam, uintptr(lParam))
	}
	if wParam != wmMouseWheel {
		return callNext(nCode, wParam, uintptr(lParam))
	}
	ms := (*msllhookstruct)(lParam)
	delta := int16(ms.MouseData >> 16)
	if os.Getenv("FL_SPIKE_HOOK_DEBUG") == "1" {
		log.Printf("hook wheel at %d,%d delta=%d hit=%v", ms.Pt.X, ms.Pt.Y, delta, c.hit(ms.Pt))
	}
	if delta == 0 || !c.hit(ms.Pt) {
		return callNext(nCode, wParam, uintptr(lParam))
	}
	select {
	case c.ch <- delta:
	default:
		// The worker is behind; let this notch through rather than block
		// the system's hook thread.
		return callNext(nCode, wParam, uintptr(lParam))
	}
	return 1
}

func (c *Controller) hit(p point) bool {
	hmon, _, _ := procMonitorFromPoint.Call(
		uintptr(uint32(p.X))|uintptr(uint32(p.Y))<<32, monitorNearest)
	if hmon == 0 {
		return false
	}
	mi := monitorInfo{CbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := procGetMonitorInfoW.Call(hmon, uintptr(unsafe.Pointer(&mi))); ok == 0 {
		return false
	}
	dpi := uint32(96)
	var dx, dy uint32
	if r, _, _ := procGetDpiForMonitor.Call(hmon, dpiEffective,
		uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); r == 0 && dx > 0 {
		dpi = dx
	}
	thickness := int32(math.Round(float64(c.opts.ThicknessDIP) * float64(dpi) / 96))
	if thickness < 1 {
		thickness = 1
	}
	r := mi.RcMonitor
	switch {
	case c.opts.Edges&EdgeBottom != 0 && p.Y >= r.Bottom-thickness:
		return true
	case c.opts.Edges&EdgeTop != 0 && p.Y <= r.Top+thickness:
		return true
	case c.opts.Edges&EdgeLeft != 0 && p.X <= r.Left+thickness:
		return true
	case c.opts.Edges&EdgeRight != 0 && p.X >= r.Right-thickness:
		return true
	}
	return false
}

func (c *Controller) worker() {
	for {
		select {
		case <-c.stop:
			return
		case delta := <-c.ch:
			steps := float32(delta) / 120
			percent, err := Adjust(steps)
			if err != nil {
				log.Printf("volume: %v", err)
				continue
			}
			if c.opts.OnChange != nil {
				c.opts.OnChange(percent)
			}
		}
	}
}

func callNext(nCode int32, wParam, lParam uintptr) uintptr {
	r, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return r
}

func getCurrentThreadID() uint32 {
	r, _, _ := procGetCurrentThreadId.Call()
	return uint32(r)
}

// ErrUnsupported reports an unavailable screen edge configuration.
var ErrUnsupported = errors.New("volume wheel: no edge selected")
