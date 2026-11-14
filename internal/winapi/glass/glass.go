// Package glass turns the blur behind a window on and off, the way a
// launcher's acrylic does: the window keeps its pixels transparent and
// Windows blurs what is behind them.
//
// ACCENT_ENABLE_BLURBEHIND blurs the real windows behind a transparent surface,
// which the app uses for acrylic only. Mica uses Windows' native, opaque
// wallpaper-based material through MyGo's SetVibrancy.
package glass

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SetWindowCompositionAttribute is what applies an accent policy to a window.
var procSetWindowCompositionAttribute = windows.NewLazySystemDLL("user32.dll").NewProc("SetWindowCompositionAttribute")

const (
	// WCA_ACCENT_POLICY, the attribute carrying an ACCENT_POLICY.
	accentPolicyAttribute = 19

	// ACCENT_STATE values; 3 blurs what is behind the window and paints
	// nothing itself.
	accentDisabled   = 0
	accentBlurBehind = 3
)

// accentPolicy is ACCENT_POLICY.
type accentPolicy struct {
	State        uint32
	Flags        uint32
	GradientRGBA uint32
	AnimationID  uint32
}

// compositionAttribute is WINDOWCOMPOSITIONATTRIBDATA.
type compositionAttribute struct {
	Attribute  int32
	Data       uintptr
	SizeOfData uintptr
}

var procDwmSetWindowAttribute = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")

const (
	// DWMWA_BORDER_COLOR and DWMWA_COLOR_NONE (Windows 11 22000+).
	dwmBorderColorAttribute = 34
	dwmColorNone            = 0xFFFFFFFE
)

// HideBorder removes the 1px border Windows 11 draws around a window with
// rounded corners. Over a blurred, translucent surface that system line
// shows as a light rim whatever the theme. Older systems ignore it.
func HideBorder(hwnd uintptr) error {
	if hwnd == 0 {
		return errors.New("glass: the window has no native handle")
	}
	if err := procDwmSetWindowAttribute.Find(); err != nil {
		return err
	}
	color := uint32(dwmColorNone)
	if hr, _, _ := procDwmSetWindowAttribute.Call(hwnd, dwmBorderColorAttribute,
		uintptr(unsafe.Pointer(&color)), unsafe.Sizeof(color)); hr != 0 {
		return fmt.Errorf("glass: DwmSetWindowAttribute(border): HRESULT %#x", uint32(hr))
	}
	return nil
}

// SetBlur blurs what is behind hwnd while it is on, and stops when it is off.
// The window must keep the area it wants blurred transparent: the blur shows
// through its pixels.
func SetBlur(hwnd uintptr, on bool) error {
	if hwnd == 0 {
		return errors.New("glass: the window has no native handle")
	}
	if err := procSetWindowCompositionAttribute.Find(); err != nil {
		return err
	}
	state := uint32(accentDisabled)
	if on {
		state = accentBlurBehind
	}
	policy := accentPolicy{State: state}
	data := compositionAttribute{
		Attribute:  accentPolicyAttribute,
		Data:       uintptr(unsafe.Pointer(&policy)),
		SizeOfData: unsafe.Sizeof(policy),
	}
	ok, _, err := procSetWindowCompositionAttribute.Call(hwnd, uintptr(unsafe.Pointer(&data)))
	if ok == 0 {
		if err == nil {
			err = errors.New("glass: SetWindowCompositionAttribute failed")
		}
		return err
	}
	return nil
}
