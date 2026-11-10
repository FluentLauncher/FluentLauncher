package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/internal/core/shortcuts"
	"fluentlauncher/locales"
)

const maxSettingsBytes = 1 << 20

// ConfigStore owns settings.json and the legacy hotkey.json.
type ConfigStore struct {
	dir   string
	path  string
	mu    sync.Mutex
	value settings.LauncherSettings
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
			return settings.LauncherSettings{}, errors.New(locales.StoreConfigTooLarge())
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
		c.value = value
		return value, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return settings.LauncherSettings{}, err
	}
	value := settings.Default()
	if hotkey, err := c.loadLegacyHotkey(); err == nil && hotkey != "" {
		value.Hotkey = hotkey
		value.Shortcuts[shortcuts.Toggle] = shortcuts.Binding{Keys: []string{hotkey}, Enabled: true}
	}
	if err := value.Validate(); err != nil {
		return settings.LauncherSettings{}, err
	}
	c.value = value
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

func (c *ConfigStore) Save(value settings.LauncherSettings) error {
	if err := value.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
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
		return errors.New(locales.StoreConfigTooLarge())
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
		return ConfigurationPackage{}, errors.New(locales.StoreConfigPackageTooLarge())
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
		return ConfigurationPackage{}, errors.New(locales.StoreConfigPackageInvalidCount())
	}
	seen := map[string]bool{}
	for _, profile := range pkg.Profiles {
		if profile.EntryID == "" || len(profile.EntryID) > 2048 || len(profile.Alias) > 500 ||
			profile.UseCount < 0 {
			return ConfigurationPackage{}, errors.New(locales.StoreProfileInvalidFields())
		}
		key := profile.EntryID
		if seen[key] {
			return ConfigurationPackage{}, errors.New(locales.StoreProfileDuplicate())
		}
		seen[key] = true
	}
	return pkg, nil
}
