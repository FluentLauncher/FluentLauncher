// Package hostservices owns platform-neutral host service contracts and plugin
// configuration. Native implementations are supplied by the application.
package hostservices

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"fluentlauncher/internal/plugin/protocol"
)

var Names = []string{"ui.notify", "launcher.show", "launcher.hide", "clipboard.readText", "clipboard.writeText", "dialogs.open", "dialogs.save", "settings.get", "settings.set", "context.get", "shell.open", "shell.reveal"}

func Version(name string) int {
	for _, method := range Names {
		if method == name {
			return 1
		}
	}
	return 0
}

type Store struct {
	root string
	mu   sync.Mutex
}

func NewStore(root string) *Store { return &Store{root: root} }
func (s *Store) Directories(id string) (data, cache string, err error) {
	if !protocol.ValidID(id) {
		return "", "", errors.New("invalid plugin identity")
	}
	data, cache = filepath.Join(s.root, "plugin-data", id), filepath.Join(s.root, "cache", "plugins", id)
	for _, path := range []string{data, cache} {
		if err = os.MkdirAll(path, 0700); err != nil {
			return "", "", err
		}
	}
	return data, cache, nil
}
func (s *Store) Get(manifest protocol.Manifest) (map[string]json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.get(manifest)
}
func (s *Store) get(manifest protocol.Manifest) (map[string]json.RawMessage, error) {
	directory, _, err := s.Directories(manifest.ID)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(directory, "config.json")
	values := map[string]json.RawMessage{}
	if info, err := os.Stat(path); err == nil {
		if info.Size() > 1<<20 {
			return nil, errors.New("plugin settings exceed size limit")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &values); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	result := map[string]json.RawMessage{}
	for _, setting := range manifest.Settings {
		value, ok := values[setting.ID]
		if !ok || setting.ValidateValue(value) != nil {
			value = setting.Default
		}
		result[setting.ID] = append(json.RawMessage(nil), value...)
	}
	return result, nil
}
func (s *Store) Set(manifest protocol.Manifest, key string, value json.RawMessage) error {
	return s.SetIf(manifest, key, value, nil)
}

// SetIf lets a debounced editor discard obsolete writes after acquiring the
// storage lock, so a slow older edit cannot overwrite a newer value.
func (s *Store) SetIf(manifest protocol.Manifest, key string, value json.RawMessage, current func() bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current != nil && !current() {
		return nil
	}
	found := false
	for _, setting := range manifest.Settings {
		if setting.ID == key {
			found = true
			if err := setting.ValidateValue(value); err != nil {
				return err
			}
			break
		}
	}
	if !found {
		return errors.New("unknown plugin setting")
	}
	values, err := s.get(manifest)
	if err != nil {
		return err
	}
	values[key] = value
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	directory, _, err := s.Directories(manifest.ID)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(directory, ".config-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if current != nil && !current() {
		return nil
	}
	return os.Rename(f.Name(), filepath.Join(directory, "config.json"))
}
