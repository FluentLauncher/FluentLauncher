package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/mistquay/fluentlauncher/internal/core/model"
	"github.com/mistquay/fluentlauncher/internal/core/settings"
)

const maxSettingsBytes = 1 << 20

// ConfigStore owns settings.json, hotkey.json and window-positions.json.
type ConfigStore struct {
	dir   string
	path  string
	mu    sync.Mutex
	value settings.LauncherSettings
	ready bool
}

func NewConfigStore(dir string) *ConfigStore {
	return &ConfigStore{dir: dir, path: filepath.Join(dir, "settings.json")}
}

type HotkeySettings struct {
	Hotkey string `json:"Hotkey"`
}

// Load reads settings.json, or the legacy hotkey.json on first run.
func (c *ConfigStore) Load() (settings.LauncherSettings, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if info, err := os.Stat(c.path); err == nil {
		if info.Size() > maxSettingsBytes {
			return settings.LauncherSettings{}, fmt.Errorf("配置文件超过 1 MiB")
		}
		data, err := os.ReadFile(c.path)
		if err != nil {
			return settings.LauncherSettings{}, err
		}
		value := settings.Default()
		if err := json.Unmarshal(data, &value); err != nil {
			return settings.LauncherSettings{}, err
		}
		value.Normalize()
		if err := value.Validate(); err != nil {
			return settings.LauncherSettings{}, err
		}
		c.value, c.ready = value, true
		return value, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return settings.LauncherSettings{}, err
	}
	value := settings.Default()
	if hotkey, err := c.loadLegacyHotkey(); err == nil && hotkey != "" {
		value.Hotkey = hotkey
	}
	if err := value.Validate(); err != nil {
		return settings.LauncherSettings{}, err
	}
	c.value, c.ready = value, true
	return value, nil
}

func (c *ConfigStore) loadLegacyHotkey() (string, error) {
	data, err := os.ReadFile(filepath.Join(c.dir, "hotkey.json"))
	if err != nil {
		return "", err
	}
	var h HotkeySettings
	if err := json.Unmarshal(data, &h); err != nil {
		return "", err
	}
	return h.Hotkey, nil
}

func (c *ConfigStore) Current() settings.LauncherSettings {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.value
}

func (c *ConfigStore) Save(value settings.LauncherSettings) error {
	if err := value.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.write(value); err != nil {
		return err
	}
	c.value, c.ready = value, true
	return nil
}

func (c *ConfigStore) SaveTheme(theme settings.ThemeSettings) error {
	if err := settings.ValidateTheme(theme); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	value := c.value
	value.Theme = theme
	if err := c.write(value); err != nil {
		return err
	}
	c.value = value
	return nil
}

func (c *ConfigStore) write(value settings.LauncherSettings) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxSettingsBytes {
		return fmt.Errorf("配置文件超过 1 MiB")
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(c.dir, "settings-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return replaceFile(name, c.path)
}

func replaceFile(source, destination string) error {
	if err := os.Rename(source, destination); err == nil {
		return nil
	}
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(source, destination)
}

// ConfigurationPackage is the JSON import/export shape used by the C#
// implementation.
type ConfigurationPackage struct {
	Settings         settings.LauncherSettings `json:"Settings"`
	Profiles         []model.Profile           `json:"Profiles"`
	IncludesLearning bool                      `json:"IncludesLearning"`
}

func (c *ConfigStore) Export(path string, pkg ConfigurationPackage) error {
	if err := pkg.Settings.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (c *ConfigStore) Import(path string) (ConfigurationPackage, error) {
	info, err := os.Stat(path)
	if err != nil {
		return ConfigurationPackage{}, err
	}
	if info.Size() > 16<<20 {
		return ConfigurationPackage{}, fmt.Errorf("配置包超过 16 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigurationPackage{}, err
	}
	var pkg ConfigurationPackage
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ConfigurationPackage{}, err
	}
	pkg.Settings.Normalize()
	if err := pkg.Settings.Validate(); err != nil {
		return ConfigurationPackage{}, err
	}
	if pkg.Profiles == nil || len(pkg.Profiles) > 20000 {
		return ConfigurationPackage{}, fmt.Errorf("配置包内容或个人规则数量无效")
	}
	seen := map[string]bool{}
	for _, profile := range pkg.Profiles {
		if profile.EntryID == "" || len(profile.EntryID) > 2048 || len(profile.Alias) > 500 ||
			profile.UseCount < 0 {
			return ConfigurationPackage{}, fmt.Errorf("个人规则包含无效字段")
		}
		key := profile.EntryID
		if seen[key] {
			return ConfigurationPackage{}, fmt.Errorf("个人规则包含重复条目")
		}
		seen[key] = true
	}
	return pkg, nil
}

// PopupAnchor is the window position as fractions of the work area.
type PopupAnchor struct {
	X float64 `json:"X"`
	Y float64 `json:"Y"`
}

type PositionStore struct {
	path string
	mu   sync.Mutex
	data map[string]PopupAnchor
}

func NewPositionStore(dir string) *PositionStore {
	return &PositionStore{path: filepath.Join(dir, "window-positions.json"), data: map[string]PopupAnchor{}}
}

func (p *PositionStore) Load() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	data, err := os.ReadFile(p.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var positions map[string]PopupAnchor
	if err := json.Unmarshal(data, &positions); err != nil {
		return err
	}
	p.data = map[string]PopupAnchor{}
	for key, anchor := range positions {
		if anchor.X >= 0 && anchor.X <= 1 && anchor.Y >= 0 && anchor.Y <= 1 {
			p.data[key] = anchor
		}
	}
	return nil
}

func (p *PositionStore) Get(monitor string) (PopupAnchor, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	anchor, ok := p.data[monitor]
	return anchor, ok
}

func (p *PositionStore) Save(monitor string, anchor PopupAnchor) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data[monitor] = anchor
	data, err := json.MarshalIndent(p.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(p.path), "positions-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return replaceFile(name, p.path)
}
