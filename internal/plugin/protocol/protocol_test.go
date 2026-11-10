package protocol_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fluentlauncher/internal/core/shortcuts"
	"fluentlauncher/internal/plugin/protocol"
)

func TestCommandManifestValidation(t *testing.T) {
	valid := protocol.Manifest{ID: "vendor.tool", Name: "Tool", Version: "1.0.0", ProtocolVersion: 1, Executable: "tool.exe"}
	command := protocol.Command{ID: "capture", Title: "Capture", Scope: shortcuts.Global, DefaultShortcuts: []string{"Ctrl+Alt+A"}, Activation: "invoke"}
	valid.Commands = []protocol.Command{command}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*protocol.Command){
		func(c *protocol.Command) { c.ID = "../capture" },
		func(c *protocol.Command) { c.Title = " " },
		func(c *protocol.Command) { c.Scope = shortcuts.Actions },
		func(c *protocol.Command) { c.Activation = "unknown" },
		func(c *protocol.Command) { c.Activation = "showQuery"; c.Query = " " },
		func(c *protocol.Command) { c.DefaultShortcuts = []string{"Ctrl+Alt+Delete"} },
		func(c *protocol.Command) { c.DefaultShortcuts = []string{"ctrl+alt+a", "Alt+Ctrl+A"} },
	} {
		broken := command
		change(&broken)
		valid.Commands = []protocol.Command{broken}
		if err := valid.Validate(); err == nil {
			t.Fatalf("invalid command accepted: %+v", broken)
		}
	}
	valid.Commands = []protocol.Command{command, command}
	if err := valid.Validate(); err == nil {
		t.Fatal("duplicate command ID accepted")
	}
	command.Activation = "showQuery"
	command.Query = "clip "
	valid.Commands = []protocol.Command{command}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestManifestValidateExplainsWhatIsWrong(t *testing.T) {
	valid := protocol.Manifest{
		ID: "vendor.tool", Name: "工具", Version: "1.2.0", ProtocolVersion: protocol.ProtocolVersion,
		Executable: "tool.exe", Keywords: []string{"tool"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid manifest refused: %v", err)
	}
	cases := []struct {
		name   string
		change func(*protocol.Manifest)
		want   string
	}{
		{"bad id", func(m *protocol.Manifest) { m.ID = "bad id" }, `插件标识 "bad id"`},
		{"no name", func(m *protocol.Manifest) { m.Name = "  " }, "缺少名称"},
		{"bad version", func(m *protocol.Manifest) { m.Version = "1" }, `插件版本 "1"`},
		{"wrong protocol", func(m *protocol.Manifest) { m.ProtocolVersion = 2 }, "协议版本为 2"},
		{"no entry", func(m *protocol.Manifest) { m.Executable = "" }, "缺少入口"},
		{"entry with path", func(m *protocol.Manifest) { m.Executable = `bin\tool.exe` }, "不能包含路径"},
		{"duplicate keyword", func(m *protocol.Manifest) {
			m.Keywords = []string{"tool", "TOOL"}
		}, `"TOOL" 重复`},
		{"keyword with space", func(m *protocol.Manifest) { m.Keywords = []string{"two words"} }, "不能含空白字符"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			manifest := valid
			manifest.Keywords = append([]string(nil), valid.Keywords...)
			c.change(&manifest)
			err := manifest.Validate()
			if err == nil {
				t.Fatal("invalid manifest accepted")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

func TestReadManifestReportsMissingFiles(t *testing.T) {
	dir := t.TempDir()
	if _, err := protocol.ReadManifest(dir); err == nil || !strings.Contains(err.Error(), "缺少 plugin.json") {
		t.Fatalf("empty directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"id":"vendor.tool"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.ReadManifest(dir); err == nil || !strings.Contains(err.Error(), "缺少名称") {
		t.Fatalf("incomplete manifest: %v", err)
	}
	manifest := `{"id":"vendor.tool","name":"工具","version":"1.0.0","protocolVersion":1,"executable":"tool.exe"}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.ReadManifest(dir); err == nil || !strings.Contains(err.Error(), "插件入口 tool.exe 不存在") {
		t.Fatalf("missing entry: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tool.exe"), []byte("MZ"), 0o644); err != nil {
		t.Fatal(err)
	}
	registration, err := protocol.ReadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if registration.Manifest.Keywords != nil {
		t.Fatalf("keywords = %v", registration.Manifest.Keywords)
	}
}

func TestRPCErrorMessageIsReadable(t *testing.T) {
	err := &protocol.RPCError{Code: -32603, Message: "boom"}
	if got := err.Error(); !strings.Contains(got, "boom") || !strings.Contains(got, "-32603") {
		t.Fatalf("error = %q", got)
	}
}

func TestValidID(t *testing.T) {
	for _, id := range []string{"official.echo", "vendor-tool_1"} {
		if !protocol.ValidID(id) {
			t.Fatalf("%q refused", id)
		}
	}
	for _, id := range []string{"", ".", "..", "a/b", `a\b`, "a b", strings.Repeat("a", 101)} {
		if protocol.ValidID(id) {
			t.Fatalf("%q accepted", id)
		}
	}
}
