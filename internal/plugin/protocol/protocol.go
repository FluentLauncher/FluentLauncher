// Package protocol defines the launcher's plugin wire format and manifest.
package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"fluentlauncher/locales"
)

// MaxMessageBytes is the largest v1 frame either side accepts. The independent
// FluentLauncher-plugins/sdk module implements the same documented wire format.
const MaxMessageBytes = 4 << 20

// ProtocolVersion is the wire version this launcher speaks. A package that
// declares another version is refused with a message naming both numbers,
// instead of failing at the initialization handshake with no explanation.
const ProtocolVersion = 1

// Action kinds the launcher understands. Any other kind is dropped from the
// results and reported in the plugin's diagnostics.
const (
	// KindCopyText copies Action.Value to the clipboard.
	KindCopyText = "copyText"
	// KindInvoke asks the plugin itself to handle the action through execute.
	KindInvoke = "invoke"
	// KindOpenURL opens an http or https URL from Action.Value.
	KindOpenURL = "openUrl"
	// KindOpenPath opens an absolute path from Action.Value.
	KindOpenPath = "openPath"
)

type Query struct {
	Text         string `json:"text"`
	MaxResults   int    `json:"maxResults"`
	QueryVersion int64  `json:"queryVersion"`
	Keyword      string `json:"keyword,omitempty"`
}

type Action struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type Result struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle"`
	Actions     []Action `json:"actions"`
	PreviewText string   `json:"previewText,omitempty"`
}

type QueryResult struct {
	Results []Result `json:"results"`
}

type ExecuteRequest struct {
	ResultID string `json:"resultId"`
	ActionID string `json:"actionId"`
}

type InitializeResult struct {
	ProtocolVersion int    `json:"protocolVersion"`
	PluginID        string `json:"pluginId"`
}

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return locales.PluginRPCError(e.Code, e.Message)
}

// Read reads one frame. A nil payload means the stream ended cleanly.
func Read(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, io.EOF
		}
		return nil, err
	}
	length := int(binary.BigEndian.Uint32(header[:]))
	if length <= 0 || length > MaxMessageBytes {
		return nil, errors.New(locales.PluginFrameLengthInvalid(length))
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func Write(w io.Writer, payload []byte) error {
	if len(payload) == 0 || len(payload) > MaxMessageBytes {
		return errors.New(locales.PluginFrameTooLarge())
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func WriteJSON(w io.Writer, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return Write(w, payload)
}

// Manifest is plugin.json.
type Manifest struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	ProtocolVersion int      `json:"protocolVersion"`
	Executable      string   `json:"executable"`
	EntryAssembly   string   `json:"entryAssembly"`
	Keywords        []string `json:"keywords"`
	Permissions     []string `json:"permissions"`
	Description     string   `json:"description"`
}

// Validate checks a manifest before the launcher runs the package. The
// messages name the field and the offending value: they are shown in the
// settings page to whoever built the plugin, who otherwise only sees that
// the package "is invalid".
func (m Manifest) Validate() error {
	if !ValidID(m.ID) {
		return errors.New(locales.PluginManifestIDInvalid(m.ID))
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New(locales.PluginManifestNameMissing())
	}
	if utf8.RuneCountInString(m.Name) > 200 {
		return errors.New(locales.PluginManifestNameTooLong(utf8.RuneCountInString(m.Name)))
	}
	if !validVersion(m.Version) {
		return errors.New(locales.PluginManifestVersionInvalid(m.Version))
	}
	if m.ProtocolVersion != ProtocolVersion {
		return errors.New(locales.PluginManifestProtocolVersion(m.ProtocolVersion, ProtocolVersion))
	}
	entry := m.Entry()
	if entry == "" {
		return errors.New(locales.PluginManifestEntryMissing())
	}
	if filepath.IsAbs(entry) || strings.ContainsAny(entry, `/\:`) || entry == "." || entry == ".." {
		return errors.New(locales.PluginManifestEntryInvalid(entry))
	}
	if len(m.Keywords) > 20 {
		return errors.New(locales.PluginManifestKeywordsTooMany(len(m.Keywords)))
	}
	seen := make(map[string]bool, len(m.Keywords))
	for _, keyword := range m.Keywords {
		if keyword == "" || utf8.RuneCountInString(keyword) > 32 || strings.ContainsAny(keyword, " \t\r\n") {
			return errors.New(locales.PluginManifestKeywordInvalid(keyword))
		}
		// The launcher matches keywords case-insensitively, so two spellings
		// of one word would make one of them unreachable.
		folded := strings.ToLower(keyword)
		if seen[folded] {
			return errors.New(locales.PluginManifestKeywordDuplicate(keyword))
		}
		seen[folded] = true
	}
	if len(m.Permissions) > 20 {
		return errors.New(locales.PluginManifestPermissionsTooMany(len(m.Permissions)))
	}
	if utf8.RuneCountInString(m.Description) > 10000 {
		return errors.New(locales.PluginManifestDescriptionTooLong())
	}
	return nil
}

// ValidID reports whether id can name a plugin and its package directory.
// A package directory is created from this value, so it is also the guard
// against a manifest pointing outside the plugin root.
func ValidID(id string) bool {
	if id == "" || len(id) > 100 || id == "." || id == ".." {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func (m Manifest) Entry() string {
	if m.Executable != "" {
		return m.Executable
	}
	return m.EntryAssembly
}

func validVersion(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) < 2 || len(parts) > 4 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

type Registration struct {
	Manifest  Manifest
	Directory string
}

func (r Registration) Path() string {
	return filepath.Join(r.Directory, r.Manifest.Entry())
}

// ReadManifest reads plugin.json from a package directory and checks that
// the entry it names is inside that directory. The errors are the ones the
// settings page shows, so they say what to fix rather than what failed in
// the file system.
func ReadManifest(dir string) (Registration, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Registration{}, errors.New(locales.PluginManifestFileMissing())
		}
		return Registration{}, errors.New(locales.PluginManifestReadFailed(err))
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Registration{}, errors.New(locales.PluginManifestInvalidJSON(err))
	}
	if err := manifest.Validate(); err != nil {
		return Registration{}, err
	}
	path := filepath.Join(dir, manifest.Entry())
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Registration{}, errors.New(locales.PluginEntryNotFound(manifest.Entry()))
		}
		return Registration{}, errors.New(locales.PluginEntryReadFailed(manifest.Entry(), err))
	}
	return Registration{Manifest: manifest, Directory: dir}, nil
}
