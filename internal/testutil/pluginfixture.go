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
	"time"

	"fluentlauncher/internal/core/shortcuts"
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
	if strings.HasPrefix(id, "vendor.commands") {
		manifest.Commands = []protocol.Command{
			{ID: "write", Title: "Write marker", Scope: shortcuts.Global, Activation: "invoke"},
			{ID: "slow", Title: "Slow marker", Scope: shortcuts.Global, Activation: "invoke"},
			{ID: "fail", Title: "Fail", Scope: shortcuts.Global, Activation: "invoke"},
		}
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
		var rpcErr *protocol.RPCError
		switch req.Method {
		case "initialize":
			initialized := protocol.InitializeResult{ProtocolVersion: 1, PluginID: registration.Manifest.ID}
			if strings.HasPrefix(registration.Manifest.ID, "vendor.commands") && registration.Manifest.ID != "vendor.commands.old" {
				initialized.Capabilities = []string{"executeCommand"}
			}
			result = initialized
		case "executeCommand":
			var command protocol.ExecuteCommandRequest
			if json.Unmarshal(req.Params, &command) != nil {
				return
			}
			if command.CommandID == "slow" {
				time.Sleep(200 * time.Millisecond)
			}
			if command.CommandID == "fail" {
				rpcErr = &protocol.RPCError{Code: -32000, Message: "command failed"}
			} else {
				file, err := os.OpenFile(filepath.Join(registration.Directory, "commands.txt"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					return
				}
				_, _ = file.WriteString(command.CommandID + "\n")
				_ = file.Close()
			}
		case "ping":
			result = "pong"
		case "query":
			var q protocol.Query
			if json.Unmarshal(req.Params, &q) != nil {
				return
			}
			if strings.Contains(registration.Manifest.ID, ".queue") {
				file, err := os.OpenFile(filepath.Join(registration.Directory, "queries.txt"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					return
				}
				_, _ = file.WriteString(q.Text + "\n")
				_ = file.Close()
				if q.Text == "slow" {
					time.Sleep(150 * time.Millisecond)
				}
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
		if rpcErr != nil {
			result = nil
		}
		if err := protocol.WriteJSON(os.Stdout, protocol.Response{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rpcErr}); err != nil {
			return
		}
	}
}
