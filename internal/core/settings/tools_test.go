package settings

import (
	"encoding/json"
	"testing"
)

func TestToolSettingsPersistAndLegacyDisableSurvives(t *testing.T) {
	var cfg LauncherSettings
	if err := json.Unmarshal([]byte(`{"EnableCalculator":false,"Files":{"EnableEverything":true}}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Tool("calc").Enabled || cfg.Tool("calc").Prefix != "=" || !cfg.Files.EverythingAutoStart {
		t.Fatalf("defaults or existing disabled preference lost: %+v", cfg)
	}
	for i := range cfg.Tools {
		if cfg.Tools[i].ID == "uuid" {
			cfg.Tools[i].Prefix = "@@"
			cfg.Tools[i].Keywords = []string{"uid"}
			cfg.Tools[i].GlobalQuery = false
		}
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var loaded LauncherSettings
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if tool := loaded.Tool("uuid"); tool.Prefix != "@@" || tool.GlobalQuery || tool.Keywords[0] != "uid" {
		t.Fatalf("tool config did not survive reload: %+v", tool)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestToolTriggersRejectAmbiguity(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*LauncherSettings)
	}{
		{"duplicate", func(s *LauncherSettings) { s.Tools[1].Keywords = []string{"date"} }},
		{"reserved", func(s *LauncherSettings) { s.Tools[1].Keywords = []string{"file"} }},
		{"canonical ID", func(s *LauncherSettings) { s.Tools[1].Keywords = []string{"week"} }},
		{"prefix overlap", func(s *LauncherSettings) { s.Tools[1].Prefix = "==" }},
		{"prefix shadows word", func(s *LauncherSettings) { s.Tools[0].Prefix = "uu" }},
		{"whitespace", func(s *LauncherSettings) { s.Tools[0].Prefix = "= " }},
		{"site", func(s *LauncherSettings) {
			s.WebSites = []WebSearchSite{{Keyword: "uuid", Name: "UUID site", URLTemplate: "https://example.com/?q={query}", Enabled: true}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := Default()
			test.edit(&cfg)
			if cfg.Validate() == nil {
				t.Fatal("ambiguous trigger was accepted")
			}
		})
	}
}

// TestRemovedToolsLeaveOldSettings loads a file written while unit
// conversion, color and the encodings were tools: they go, and the file
// still validates.
func TestRemovedToolsLeaveOldSettings(t *testing.T) {
	data := []byte(`{"Tools":[
		{"Id":"uuid","Enabled":true,"Prefix":"","GlobalQuery":true,"Keywords":["uid"]},
		{"Id":"convert","Enabled":true,"Prefix":"","Keywords":["convert","换算"]},
		{"Id":"color","Enabled":true,"Prefix":"","Keywords":["color"]},
		{"Id":"b64","Enabled":true,"Prefix":"","GlobalQuery":true,"Keywords":["b64","base64"]}
	]}`)
	var loaded LauncherSettings
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	for _, tool := range loaded.Tools {
		if tool.ID == "convert" || tool.ID == "color" || tool.ID == "b64" {
			t.Fatalf("removed tool kept: %+v", tool)
		}
	}
	if len(loaded.Tools) != len(DefaultTools()) || loaded.Tool("uuid").Keywords[0] != "uid" {
		t.Fatalf("tools after migration = %+v", loaded.Tools)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatal(err)
	}
}
