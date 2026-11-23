// Package volume adjusts the default output device's master volume and can
// watch the configured screen edges for wheel input. Windows only.
package volume

import (
	"fmt"
	"math"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

var procCoInitializeEx = syscall.NewLazyDLL("ole32.dll").NewProc("CoInitializeEx")

// StepPercent is the default volume change for one wheel notch.
const StepPercent = 2

// State is the default output device's master volume as a percentage and
// whether the device is muted. A muted device keeps its level: the level
// still moves, but nothing plays until the mute is lifted.
type State struct {
	Percent int
	Muted   bool
}

// Adjust changes the default output device's master volume by deltaSteps
// notches (positive is louder), each notch moving stepPercent points, and
// returns the new state. Like the keyboard's volume keys, any change lifts
// the mute: otherwise the bar would move while the speakers stay silent.
// It re-reads the default device on every call, so a device change is
// followed without extra notifications.
func Adjust(deltaSteps float32, stepPercent int) (State, error) {
	var state State
	err := withEndpointVolume(func(aev *wca.IAudioEndpointVolume) error {
		var current float32
		if err := aev.GetMasterVolumeLevelScalar(&current); err != nil {
			return err
		}
		next := nextLevel(current, deltaSteps, stepPercent)
		if err := aev.SetMasterVolumeLevelScalar(next, nil); err != nil {
			return err
		}
		muted, err := getMute(aev)
		if err != nil {
			return err
		}
		if muted {
			if err := aev.SetMute(false, nil); err != nil {
				return err
			}
		}
		state = State{Percent: toPercent(next)}
		return nil
	})
	return state, err
}

// ToggleMute flips the default output device's mute, as clicking the speaker
// on the system's volume flyout does, and returns the new state.
func ToggleMute() (State, error) {
	var state State
	err := withEndpointVolume(func(aev *wca.IAudioEndpointVolume) error {
		var level float32
		if err := aev.GetMasterVolumeLevelScalar(&level); err != nil {
			return err
		}
		muted, err := getMute(aev)
		if err != nil {
			return err
		}
		if err := aev.SetMute(!muted, nil); err != nil {
			return err
		}
		state = State{Percent: toPercent(level), Muted: !muted}
		return nil
	})
	return state, err
}

// Get returns the default output device's master volume and mute state.
func Get() (State, error) {
	var state State
	err := withEndpointVolume(func(aev *wca.IAudioEndpointVolume) error {
		var level float32
		if err := aev.GetMasterVolumeLevelScalar(&level); err != nil {
			return err
		}
		muted, err := getMute(aev)
		if err != nil {
			return err
		}
		state = State{Percent: toPercent(level), Muted: muted}
		return nil
	})
	return state, err
}

func nextLevel(current, deltaSteps float32, stepPercent int) float32 {
	if stepPercent <= 0 {
		stepPercent = StepPercent
	}
	next := current + deltaSteps*float32(stepPercent)/100
	return float32(math.Max(0, math.Min(1, float64(next))))
}

func toPercent(level float32) int {
	return int(math.Round(float64(level) * 100))
}

// getMute reads the endpoint's mute flag. go-wca's GetMute hands a Go bool
// to a method that writes a 4-byte BOOL, overrunning it; read a uint32.
func getMute(aev *wca.IAudioEndpointVolume) (bool, error) {
	var value uint32
	hr, _, _ := syscall.SyscallN(aev.VTable().GetMute,
		uintptr(unsafe.Pointer(aev)), uintptr(unsafe.Pointer(&value)))
	if hr != 0 {
		return false, ole.NewError(hr)
	}
	return value != 0, nil
}

func withEndpointVolume(fn func(*wca.IAudioEndpointVolume) error) error {
	// COM initialization is thread-private and the caller is a plain
	// goroutine: pin it to one OS thread for the whole call, or the COM
	// calls can land on an uninitialized thread after the scheduler moves
	// the goroutine.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	owned, err := coInitialize()
	if err != nil {
		return fmt.Errorf("CoInitializeEx: %w", err)
	}
	if owned {
		defer ole.CoUninitialize()
	}

	var enumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, wca.CLSCTX_ALL,
		wca.IID_IMMDeviceEnumerator, &enumerator); err != nil {
		return fmt.Errorf("CoCreateInstance: %w", err)
	}
	defer enumerator.Release()

	var device *wca.IMMDevice
	if err := enumerator.GetDefaultAudioEndpoint(wca.ERender, wca.EConsole, &device); err != nil {
		return fmt.Errorf("GetDefaultAudioEndpoint: %w", err)
	}
	defer device.Release()

	var endpoint *wca.IAudioEndpointVolume
	if err := device.Activate(wca.IID_IAudioEndpointVolume, wca.CLSCTX_ALL, nil, &endpoint); err != nil {
		return fmt.Errorf("Activate IAudioEndpointVolume: %w", err)
	}
	defer endpoint.Release()

	return fn(endpoint)
}

// coInitialize accepts S_FALSE, which go-ole reports as error 1: the thread
// already has COM initialized, and CoUninitialize must not be called then.
func coInitialize() (owned bool, err error) {
	hr, _, _ := procCoInitializeEx.Call(0, ole.COINIT_APARTMENTTHREADED)
	switch uint32(hr) {
	case 0:
		return true, nil
	case 1, 0x80010106: // S_FALSE, RPC_E_CHANGED_MODE
		return false, nil
	default:
		return false, ole.NewError(hr)
	}
}
