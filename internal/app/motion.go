package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Keep feedback shorter than navigation. MyGo retargets these transitions
// during rapid input and follows the desktop's ReduceMotion preference.
var feedbackMotion = ui.Animation{Duration: 120 * time.Millisecond, Ease: ui.EaseOut}

var navigationMotion = ui.Animation{Duration: 180 * time.Millisecond, Ease: ui.EaseOut}

var feedbackTransition = ui.ElementTransition{Colors: true, Animation: &feedbackMotion}
