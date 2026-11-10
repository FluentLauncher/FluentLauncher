// Package testutil provides a test-only plugin process. Host tests must not
// download official executables or depend on the separate plugin source repo.
package testutil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fluentlauncher/internal/plugin/protocol"
)

func init() {
	// Fixture copies the current test binary under this reserved name. Only
	// tests import this package; the launcher never includes this entry point.
	if strings.EqualFold(filepath.Base(os.Args[0]), "launcher-test-fixture.exe") {
		runFixture()
		os.Exit(0)
	}
}

func PluginFixture(t testing.TB, id string) string {
	t.Helper()
	dir := t.TempDir()
	manifest := protocol.Manifest{ID: id, Name: "回显示例", Version: "1.0.0", ProtocolVersion: 1,
		Executable: "launcher-test-fixture.exe", Keywords: []string{"echo"}, Description: "演示查询结果和复制动作"}
	if id == "official.devtools" {
		manifest.Name = "开发工具"
		manifest.Keywords = []string{"hash"}
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	dst, err := os.Create(filepath.Join(dir, manifest.Executable))
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		t.Fatal(copyErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	return dir
}

func runFixture() {
	executable, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	registration, err := protocol.ReadManifest(filepath.Dir(executable))
	if err != nil {
		os.Exit(1)
	}
	for {
		payload, err := protocol.Read(os.Stdin)
		if err != nil {
			return
		}
		var req protocol.Request
		if json.Unmarshal(payload, &req) != nil {
			return
		}
		var result any = map[string]any{}
		switch req.Method {
		case "initialize":
			result = protocol.InitializeResult{ProtocolVersion: 1, PluginID: registration.Manifest.ID}
		case "ping":
			result = "pong"
		case "query":
			var q protocol.Query
			if json.Unmarshal(req.Params, &q) != nil {
				return
			}
			title, text := q.Text, q.Text
			if registration.Manifest.ID == "official.devtools" {
				digest := sha256.Sum256([]byte(q.Text))
				title, text = "SHA-256", hex.EncodeToString(digest[:])
			}
			result = protocol.QueryResult{Results: []protocol.Result{{ID: "test", Title: title, Actions: []protocol.Action{{ID: "copy", Title: "复制", Kind: protocol.KindCopyText, Value: text}}}}}
		case "$/cancelRequest":
			continue
		}
		if err := protocol.WriteJSON(os.Stdout, protocol.Response{JSONRPC: "2.0", ID: req.ID, Result: result}); err != nil {
			return
		}
	}
}
