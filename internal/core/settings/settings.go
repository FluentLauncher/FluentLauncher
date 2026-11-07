// Package settings holds the launcher's configuration model and its rules.
// It is pure Go: reading and writing settings.json lives in internal/store.
package settings

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

type ThemeMode string

const (
	ThemeSystem ThemeMode = "System"
	ThemeLight  ThemeMode = "Light"
	ThemeDark   ThemeMode = "Dark"
)

type BackgroundMode string

const (
	BackgroundAcrylic  BackgroundMode = "Acrylic"
	BackgroundMica     BackgroundMode = "Mica"
	BackgroundSolid    BackgroundMode = "Solid"
	BackgroundGradient BackgroundMode = "Gradient"
	BackgroundImage    BackgroundMode = "Image"
)

type WindowPlacement string

const (
	PlacementActiveScreen  WindowPlacement = "ActiveScreen"
	PlacementMouseScreen   WindowPlacement = "MouseScreen"
	PlacementPrimaryScreen WindowPlacement = "PrimaryScreen"
)

// ScreenEdges selects the screen edges for the wheel volume gesture.
type ScreenEdges uint8

const (
	EdgeNone   ScreenEdges = 0
	EdgeTop    ScreenEdges = 1
	EdgeBottom ScreenEdges = 2
	EdgeLeft   ScreenEdges = 4
	EdgeRight  ScreenEdges = 8
	EdgeAll    ScreenEdges = EdgeTop | EdgeBottom | EdgeLeft | EdgeRight
)

// UnmarshalJSON accepts the C# string form ("Bottom", "Top, Left") and the
// numeric form written by older Go builds.
func (e *ScreenEdges) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		var edges ScreenEdges
		for _, part := range strings.Split(text, ",") {
			switch strings.ToLower(strings.TrimSpace(part)) {
			case "", "none":
			case "top":
				edges |= EdgeTop
			case "bottom":
				edges |= EdgeBottom
			case "left":
				edges |= EdgeLeft
			case "right":
				edges |= EdgeRight
			case "all":
				edges |= EdgeAll
			default:
				return fmt.Errorf("无效的屏幕边缘：%s", part)
			}
		}
		*e = edges
		return nil
	}
	var number uint8
	if err := json.Unmarshal(data, &number); err != nil {
		return fmt.Errorf("屏幕边缘须为字符串或数字")
	}
	*e = ScreenEdges(number)
	return nil
}

func (e ScreenEdges) MarshalJSON() ([]byte, error) {
	if e == EdgeNone {
		return []byte(`"None"`), nil
	}
	parts := make([]string, 0, 4)
	if e&EdgeTop != 0 {
		parts = append(parts, "Top")
	}
	if e&EdgeBottom != 0 {
		parts = append(parts, "Bottom")
	}
	if e&EdgeLeft != 0 {
		parts = append(parts, "Left")
	}
	if e&EdgeRight != 0 {
		parts = append(parts, "Right")
	}
	if len(parts) == 0 {
		return json.Marshal(uint8(e))
	}
	return json.Marshal(strings.Join(parts, ", "))
}

// ThemeSettings is the look of the search window and the settings window.
type ThemeSettings struct {
	Mode             ThemeMode      `json:"Mode"`
	Background       BackgroundMode `json:"Background"`
	BackgroundColor  string         `json:"BackgroundColor"`
	ForegroundColor  string         `json:"ForegroundColor"`
	AccentColor      string         `json:"AccentColor"`
	GradientColor    string         `json:"GradientColor"`
	ImagePath        string         `json:"ImagePath"`
	ImageCover       bool           `json:"ImageCover"`
	ImageBlur        int            `json:"ImageBlur"`
	ImageMaskOpacity float64        `json:"ImageMaskOpacity"`
	Opacity          float64        `json:"Opacity"`
	FontFamily       string         `json:"FontFamily"`
	FontSize         float64        `json:"FontSize"`
	RowPadding       float64        `json:"RowPadding"`
	CornerRadius     float64        `json:"CornerRadius"`
}

func DefaultTheme() ThemeSettings {
	return ThemeSettings{
		Mode:             ThemeSystem,
		Background:       BackgroundAcrylic,
		GradientColor:    "#3366AA",
		ImageCover:       true,
		ImageMaskOpacity: 0.3,
		Opacity:          0.55,
		FontFamily:       "Segoe UI Variable",
		FontSize:         17,
		RowPadding:       10,
		CornerRadius:     8,
	}
}

func (t *ThemeSettings) UnmarshalJSON(data []byte) error {
	type alias ThemeSettings
	v := alias(DefaultTheme())
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*t = ThemeSettings(v)
	return nil
}

// Preset names the four built-in themes.
func Preset(name string) ThemeSettings {
	t := DefaultTheme()
	switch name {
	case "Light":
		t.Mode = ThemeLight
	case "Dark":
		t.Mode = ThemeDark
	case "Compact":
		t.FontSize = 15
		t.RowPadding = 4
	}
	return t
}

type CatalogOptions struct {
	PortableDirectories []string `json:"PortableDirectories"`
	IncludeStoreApps    bool     `json:"IncludeStoreApps"`
	IncludePath         bool     `json:"IncludePath"`
}

func DefaultCatalogOptions() CatalogOptions {
	return CatalogOptions{IncludeStoreApps: true}
}

func (c *CatalogOptions) UnmarshalJSON(data []byte) error {
	type alias CatalogOptions
	v := alias(DefaultCatalogOptions())
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*c = CatalogOptions(v)
	return nil
}

type FileIndexOptions struct {
	Directories         []string `json:"Directories"`
	ExcludedDirectories []string `json:"ExcludedDirectories"`
	MaxEntries          int      `json:"MaxEntries"`
	IncludeDirectories  bool     `json:"IncludeDirectories"`
	EnableEverything    bool     `json:"EnableEverything"`
}

func DefaultFileIndexOptions() FileIndexOptions {
	return FileIndexOptions{MaxEntries: 100000, IncludeDirectories: true}
}

func (f *FileIndexOptions) UnmarshalJSON(data []byte) error {
	type alias FileIndexOptions
	v := alias(DefaultFileIndexOptions())
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*f = FileIndexOptions(v)
	return nil
}

type PluginOptions struct {
	EnabledIDs         []string `json:"EnabledIds"`
	GlobalQueryIDs     []string `json:"GlobalQueryIds"`
	QueryTimeoutMs     int      `json:"QueryTimeoutMs"`
	MaxResults         int      `json:"MaxResults"`
	IdleSeconds        int      `json:"IdleSeconds"`
	MaxActiveProcesses int      `json:"MaxActiveProcesses"`
}

func DefaultPluginOptions() PluginOptions {
	return PluginOptions{
		EnabledIDs:         []string{},
		GlobalQueryIDs:     []string{},
		QueryTimeoutMs:     1500,
		MaxResults:         20,
		IdleSeconds:        120,
		MaxActiveProcesses: 3,
	}
}

func (p *PluginOptions) UnmarshalJSON(data []byte) error {
	type alias PluginOptions
	v := alias(DefaultPluginOptions())
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*p = PluginOptions(v)
	return nil
}

type VolumeWheelOptions struct {
	Enabled      bool        `json:"Enabled"`
	Edges        ScreenEdges `json:"Edges"`
	ThicknessDip int         `json:"ThicknessDip"`
	StepPercent  int         `json:"StepPercent"`
}

func DefaultVolumeWheelOptions() VolumeWheelOptions {
	return VolumeWheelOptions{Edges: EdgeBottom, ThicknessDip: 4, StepPercent: 2}
}

func (v *VolumeWheelOptions) UnmarshalJSON(data []byte) error {
	type alias VolumeWheelOptions
	out := alias(DefaultVolumeWheelOptions())
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	*v = VolumeWheelOptions(out)
	return nil
}

// LauncherSettings is settings.json. JSON names stay PascalCase so the C#
// implementation and user-edited files keep working.
type LauncherSettings struct {
	Version            int             `json:"Version"`
	Hotkey             string          `json:"Hotkey"`
	ShowRecentEntries  bool            `json:"ShowRecentEntries"`
	ShowRecentSearches bool            `json:"ShowRecentSearches"`
	RunAtLogin         bool            `json:"RunAtLogin"`
	HideOnStartup      bool            `json:"HideOnStartup"`
	Placement          WindowPlacement `json:"Placement"`
	// AnchorX and AnchorY place the panel in the work area, as the share of
	// the room left around it: 0 is flush with the left or top edge, 1 with
	// the right or bottom edge, 0.5 centered. The settings preview sets them
	// when the user drags the panel; they stay as the user left them.
	AnchorX           float64            `json:"AnchorX"`
	AnchorY           float64            `json:"AnchorY"`
	Width             int                `json:"Width"`
	Height            int                `json:"Height"`
	MaxResults        int                `json:"MaxResults"`
	Applications      CatalogOptions     `json:"Applications"`
	Theme             ThemeSettings      `json:"Theme"`
	Files             FileIndexOptions   `json:"Files"`
	Plugins           PluginOptions      `json:"Plugins"`
	VolumeWheel       VolumeWheelOptions `json:"VolumeWheel"`
	CustomItems       []CustomLaunchItem `json:"CustomItems"`
	WebSites          []WebSearchSite    `json:"WebSites"`
	Workspaces        []Workspace        `json:"Workspaces"`
	WebSearchTemplate string             `json:"WebSearchTemplate"`
	EnableCalculator  bool               `json:"EnableCalculator"`
}

func Default() LauncherSettings {
	return LauncherSettings{
		Version:           1,
		Hotkey:            "Alt+Space",
		Placement:         PlacementActiveScreen,
		AnchorX:           0.5,
		AnchorY:           0.45,
		Width:             760,
		Height:            560,
		MaxResults:        50,
		Applications:      DefaultCatalogOptions(),
		Theme:             DefaultTheme(),
		Files:             DefaultFileIndexOptions(),
		Plugins:           DefaultPluginOptions(),
		VolumeWheel:       DefaultVolumeWheelOptions(),
		CustomItems:       []CustomLaunchItem{},
		WebSites:          []WebSearchSite{},
		Workspaces:        []Workspace{},
		WebSearchTemplate: "https://www.bing.com/search?q={query}",
		EnableCalculator:  true,
	}
}

func (s *LauncherSettings) UnmarshalJSON(data []byte) error {
	type alias LauncherSettings
	v := alias(Default())
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*s = LauncherSettings(v)
	s.Normalize()
	return nil
}

// Normalize fills groups and slices that older files lack, and migrates the
// removed ShowRecentSearches field.
func (s *LauncherSettings) Normalize() {
	s.ShowRecentEntries = s.ShowRecentEntries || s.ShowRecentSearches
	s.ShowRecentSearches = false
	s.AnchorX = clamp01(s.AnchorX, 0.5)
	s.AnchorY = clamp01(s.AnchorY, 0.45)
	if s.Applications.PortableDirectories == nil {
		s.Applications.PortableDirectories = []string{}
	}
	if s.Files.Directories == nil {
		s.Files.Directories = []string{}
	}
	if s.Files.ExcludedDirectories == nil {
		s.Files.ExcludedDirectories = []string{}
	}
	if s.Plugins.EnabledIDs == nil {
		s.Plugins.EnabledIDs = []string{}
	}
	if s.Plugins.GlobalQueryIDs == nil {
		s.Plugins.GlobalQueryIDs = []string{}
	}
	if s.CustomItems == nil {
		s.CustomItems = []CustomLaunchItem{}
	}
	if s.WebSites == nil {
		s.WebSites = []WebSearchSite{}
	}
	if s.Workspaces == nil {
		s.Workspaces = []Workspace{}
	}
	if s.Theme == (ThemeSettings{}) {
		s.Theme = DefaultTheme()
	}
	if s.VolumeWheel == (VolumeWheelOptions{}) {
		s.VolumeWheel = DefaultVolumeWheelOptions()
	}
}

func (s LauncherSettings) Validate() error {
	if s.Version != 1 {
		return fmt.Errorf("配置版本不受支持")
	}
	switch s.Hotkey {
	case "Alt+Space", "Ctrl+Alt+Space", "Ctrl+Space":
	default:
		return fmt.Errorf("请选择受支持的热键")
	}
	if s.Width < 480 || s.Width > 1600 || s.Height < 320 || s.Height > 1200 ||
		s.MaxResults < 5 || s.MaxResults > 200 {
		return fmt.Errorf("窗口宽度须为 480–1600，高度 320–1200，结果数 5–200")
	}
	if !finite(s.AnchorX) || s.AnchorX < 0 || s.AnchorX > 1 ||
		!finite(s.AnchorY) || s.AnchorY < 0 || s.AnchorY > 1 {
		return fmt.Errorf("弹出位置须在 0–1 之间")
	}
	switch s.Placement {
	case PlacementActiveScreen, PlacementMouseScreen, PlacementPrimaryScreen:
	default:
		return fmt.Errorf("窗口定位模式无效")
	}
	if err := ValidateTheme(s.Theme); err != nil {
		return err
	}
	if s.VolumeWheel.Edges&^EdgeAll != 0 || (s.VolumeWheel.Enabled && s.VolumeWheel.Edges == EdgeNone) {
		return fmt.Errorf("屏幕边缘调音量须至少选择一条有效边缘")
	}
	if s.VolumeWheel.ThicknessDip < 1 || s.VolumeWheel.ThicknessDip > 32 ||
		s.VolumeWheel.StepPercent < 1 || s.VolumeWheel.StepPercent > 10 {
		return fmt.Errorf("音量触发带厚度须为 1–32，步长须为 1–10")
	}
	if len(s.CustomItems) > 200 {
		return fmt.Errorf("自定义启动项不能超过 200 条")
	}
	if len(s.WebSites) > 50 {
		return fmt.Errorf("站点搜索不能超过 50 条")
	}
	if len(s.Workspaces) > 50 {
		return fmt.Errorf("工作区不能超过 50 个")
	}
	ids := map[string]bool{}
	for i := range s.Workspaces {
		if err := s.Workspaces[i].Validate(); err != nil {
			return err
		}
		key := strings.ToLower(s.Workspaces[i].ID)
		if ids[key] {
			return fmt.Errorf("工作区标识重复")
		}
		ids[key] = true
	}
	for i := range s.WebSites {
		if err := s.WebSites[i].Validate(); err != nil {
			return err
		}
	}
	ids = map[string]bool{}
	for i := range s.CustomItems {
		if err := s.CustomItems[i].Validate(); err != nil {
			return err
		}
		key := strings.ToLower(s.CustomItems[i].ID)
		if ids[key] {
			return fmt.Errorf("自定义启动项标识重复")
		}
		ids[key] = true
	}
	for _, dirs := range [][]string{s.Applications.PortableDirectories, s.Files.Directories, s.Files.ExcludedDirectories} {
		if err := ValidateDirectories(dirs); err != nil {
			return err
		}
	}
	if s.Files.MaxEntries < 1000 || s.Files.MaxEntries > 500000 {
		return fmt.Errorf("文件索引上限须为 1,000–500,000")
	}
	if s.Plugins.QueryTimeoutMs < 100 || s.Plugins.QueryTimeoutMs > 30000 ||
		s.Plugins.MaxResults < 1 || s.Plugins.MaxResults > 100 ||
		s.Plugins.IdleSeconds < 10 || s.Plugins.IdleSeconds > 3600 ||
		s.Plugins.MaxActiveProcesses < 1 || s.Plugins.MaxActiveProcesses > 8 {
		return fmt.Errorf("插件超时、结果数、空闲回收或进程数量超出允许范围")
	}
	if s.WebSearchTemplate == "" || !strings.Contains(s.WebSearchTemplate, "{query}") {
		return fmt.Errorf("网页搜索模板须为包含 {query} 的 HTTP/HTTPS 地址")
	}
	target := strings.ReplaceAll(s.WebSearchTemplate, "{query}", "test")
	if u, err := url.Parse(target); err != nil || u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("网页搜索模板须为包含 {query} 的 HTTP/HTTPS 地址")
	}
	return nil
}

func ValidateTheme(t ThemeSettings) error {
	switch t.Mode {
	case ThemeSystem, ThemeLight, ThemeDark:
	default:
		return fmt.Errorf("主题模式无效")
	}
	switch t.Background {
	case BackgroundAcrylic, BackgroundMica, BackgroundSolid, BackgroundGradient, BackgroundImage:
	default:
		return fmt.Errorf("主题模式无效")
	}
	if !finite(t.FontSize) || t.FontSize < 12 || t.FontSize > 32 ||
		!finite(t.RowPadding) || t.RowPadding < 2 || t.RowPadding > 24 ||
		!finite(t.Opacity) || t.Opacity < 0 || t.Opacity > 1 ||
		!finite(t.ImageMaskOpacity) || t.ImageMaskOpacity < 0 || t.ImageMaskOpacity > 0.9 ||
		!finite(t.CornerRadius) || t.CornerRadius < 0 || t.CornerRadius > 32 ||
		t.ImageBlur < 0 || t.ImageBlur > 24 {
		return fmt.Errorf("主题尺寸、着色强度或模糊参数超出允许范围")
	}
	if strings.TrimSpace(t.FontFamily) == "" || len(t.FontFamily) > 200 {
		return fmt.Errorf("字体名称无效")
	}
	for _, color := range []string{t.BackgroundColor, t.ForegroundColor, t.AccentColor, t.GradientColor} {
		if color != "" {
			if _, ok := TryParseColor(color); !ok {
				return fmt.Errorf("颜色须为 #RRGGBB 或 #AARRGGBB，留空表示跟随系统")
			}
		}
	}
	if t.ImagePath != "" {
		if !filepath.IsAbs(t.ImagePath) || strings.HasPrefix(t.ImagePath, `\\`) {
			return fmt.Errorf("图片背景须指定本地绝对路径")
		}
	}
	return nil
}

func ValidateDirectories(dirs []string) error {
	if len(dirs) > 100 {
		return fmt.Errorf("目录列表不能超过 100 项")
	}
	for _, path := range dirs {
		if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) || strings.HasPrefix(path, `\\`) {
			return fmt.Errorf("后台索引仅支持本地绝对目录路径，不递归扫描网络共享")
		}
	}
	return nil
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// clamp01 keeps v between 0 and 1, and falls back to fallback when it is
// not a number.
func clamp01(v, fallback float64) float64 {
	if !finite(v) {
		return fallback
	}
	return math.Min(math.Max(v, 0), 1)
}

// TryParseColor parses #RRGGBB or #AARRGGBB into 0xAARRGGBB.
func TryParseColor(color string) (uint32, bool) {
	if len(color) != 7 && len(color) != 9 {
		return 0, false
	}
	if color[0] != '#' {
		return 0, false
	}
	v, err := strconv.ParseUint(color[1:], 16, 32)
	if err != nil {
		return 0, false
	}
	if len(color) == 7 {
		v |= 0xFF000000
	}
	return uint32(v), true
}
