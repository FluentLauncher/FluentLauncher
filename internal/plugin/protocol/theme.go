package protocol

// Theme contains resolved opaque sRGB colors (0xRRGGBB), without a UI dependency.
// Font size is intentionally owned by each plugin's tool UI.
type Theme struct {
	Dark         bool    `json:"dark"`
	Background   uint32  `json:"background"`
	Surface      uint32  `json:"surface"`
	SurfaceHover uint32  `json:"surfaceHover"`
	Border       uint32  `json:"border"`
	Text         uint32  `json:"text"`
	TextMuted    uint32  `json:"textMuted"`
	Accent       uint32  `json:"accent"`
	AccentText   uint32  `json:"accentText"`
	Danger       uint32  `json:"danger"`
	Inverse      uint32  `json:"inverse"`
	InverseText  uint32  `json:"inverseText"`
	Radius       float32 `json:"radius"`
	Spacing      float32 `json:"spacing"`
	// Optional surface appearance; missing fields keep legacy plugins opaque.
	Opacity     *float32 `json:"opacity,omitempty"`
	Backdrop    string   `json:"backdrop,omitempty"`
	GradientEnd *uint32  `json:"gradientEnd,omitempty"` // top-to-bottom, matching the launcher plate
}
