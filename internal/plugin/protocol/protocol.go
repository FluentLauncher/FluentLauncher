// Package protocol defines the launcher's plugin wire format and manifest.
package protocol

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const MaxMessageBytes = 4 << 20

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
	return fmt.Sprintf("plugin error %d: %s", e.Code, e.Message)
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
		return nil, fmt.Errorf("插件消息长度无效：%d", length)
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func Write(w io.Writer, payload []byte) error {
	if len(payload) == 0 || len(payload) > MaxMessageBytes {
		return fmt.Errorf("插件消息超过大小上限")
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

func (m Manifest) Validate() error {
	if len(m.ID) < 1 || len(m.ID) > 100 {
		return fmt.Errorf("插件标识无效")
	}
	for _, r := range m.ID {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_') {
			return fmt.Errorf("插件标识无效")
		}
	}
	if m.ID == "." || m.ID == ".." || strings.TrimSpace(m.Name) == "" || len(m.Name) > 200 ||
		m.ProtocolVersion != 1 || !validVersion(m.Version) {
		return fmt.Errorf("插件标识、名称、版本或协议版本无效")
	}
	entry := m.Executable
	if entry == "" {
		entry = m.EntryAssembly
	}
	if entry == "" || filepath.IsAbs(entry) || strings.ContainsAny(entry, `/\`) ||
		len(m.Keywords) > 20 {
		return fmt.Errorf("插件入口或触发词无效")
	}
	for _, keyword := range m.Keywords {
		if keyword == "" || len(keyword) > 32 || strings.ContainsAny(keyword, " \t\r\n") {
			return fmt.Errorf("插件触发词无效")
		}
	}
	if len(m.Permissions) > 20 || len(m.Description) > 10000 {
		return fmt.Errorf("插件描述无效")
	}
	return nil
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

// ReadManifest reads plugin.json from a package directory.
func ReadManifest(dir string) (Registration, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if err != nil {
		return Registration{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Registration{}, err
	}
	if err := manifest.Validate(); err != nil {
		return Registration{}, err
	}
	path := filepath.Join(dir, manifest.Entry())
	if _, err := os.Stat(path); err != nil {
		return Registration{}, fmt.Errorf("插件入口不存在：%w", err)
	}
	return Registration{Manifest: manifest, Directory: dir}, nil
}
