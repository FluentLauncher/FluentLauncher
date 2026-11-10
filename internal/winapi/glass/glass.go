// Package glass turns the blur behind a window on and off, the way a
// launcher's acrylic does: the window keeps its pixels transparent and
// Windows blurs what is behind them.
//
// Native Mica is a separate wallpaper-based, opaque system backdrop, selected
// through MyGo's SetVibrancy. ACCENT_ENABLE_BLURBEHIND blurs the real windows
// behind a transparent surface, which the app uses for acrylic only.
package glass

import (
	"errors"
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
