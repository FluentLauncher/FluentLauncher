// Package volume adjusts the default output device's master volume and can
// watch the configured screen edges for wheel input. Windows only.
package volume

import (
	"fmt"
	"math"
	"syscall"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

var procCoInitializeEx = syscall.NewLazyDLL("ole32.dll").NewProc("CoInitializeEx")

// StepPercent is the default volume change for one wheel notch.
const StepPercent = 2

// Adjust changes the default output device's master volume by deltaSteps
// notches (positive is louder) and returns the new volume as a percentage.
// It keeps the mute state as it is and re-reads the default device on every
// call, so a device change is followed without extra notifications.
func Adjust(deltaSteps float32) (int, error) {
	current, err := get()
	if err != nil {
		return 0, err
	}
	next := current + deltaSteps*StepPercent/100
	next = float32(math.Max(0, math.Min(1, float64(next))))
	if err := set(next); err != nil {
		return 0, err
	}
	return int(math.Round(float64(next) * 100)), nil
}

// Get returns the default output device's master volume as a percentage.
func Get() (int, error) {
	v, err := get()
	if err != nil {
		return 0, err
	}
	return int(math.Round(float64(v) * 100)), nil
}

func get() (float32, error) {
	var value float32
	err := withEndpointVolume(func(aev *wca.IAudioEndpointVolume) error {
		return aev.GetMasterVolumeLevelScalar(&value)
	})
	return value, err
}

func set(value float32) error {
	return withEndpointVolume(func(aev *wca.IAudioEndpointVolume) error {
		return aev.SetMasterVolumeLevelScalar(value, nil)
	})
}

func withEndpointVolume(fn func(*wca.IAudioEndpointVolume) error) error {
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
