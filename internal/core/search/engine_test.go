package search

import (
	"context"
	"testing"
	"time"

	"github.com/mistquay/fluentlauncher/internal/core/model"
)

func TestEngineMatchesBands(t *testing.T) {
	e := NewEngine()
	e.ReplaceEntries([]model.Entry{
		{ID: "vsc", Title: "Visual Studio Code", Subtitle: "Editor"},
		{ID: "wechat", Title: "微信", Subtitle: "聊天"},
		{ID: "qq", Title: "QQ浏览器", Subtitle: "浏览器"},
		{ID: "hidden", Title: "Hidden App", Subtitle: ""},
	}, map[string]model.Profile{
		"hidden": {EntryID: "hidden", Hidden: true},
	})

	tests := []struct {
		query string
		want  string
	}{
		{"vsc", "vsc"},
		{"visual", "vsc"},
		{"studio code", "vsc"},
		{"wx", "wechat"},
		{"qllq", "qq"},
		{"Visual Studio Code", "vsc"},
	}
	for _, tt := range tests {
		got := e.Search(context.Background(), tt.query, 10)
		if len(got) == 0 || got[0].Entry.ID != tt.want {
			t.Errorf("Search(%q) = %+v, want first %q", tt.query, got, tt.want)
		}
	}
	for _, result := range e.Search(context.Background(), "", 10) {
		if result.Entry.ID == "hidden" {
			t.Fatal("hidden entry returned")
		}
	}
}

func TestProfileAliasAndPinned(t *testing.T) {
	e := NewEngine()
	e.ReplaceEntries([]model.Entry{
		{ID: "vsc", Title: "Visual Studio Code"},
		{ID: "other", Title: "Other"},
	}, map[string]model.Profile{
		"vsc": {EntryID: "vsc", Alias: "编辑器;ide", Pinned: true},
	})
	got := e.Search(context.Background(), "编辑器", 10)
	if len(got) == 0 || got[0].Entry.ID != "vsc" {
		t.Fatalf("alias search = %+v", got)
	}
	e.UpdateProfile(model.Profile{EntryID: "vsc", Alias: "code", Pinned: false})
	if got := e.Search(context.Background(), "编辑器", 10); len(got) != 0 {
		t.Fatalf("old alias still matches: %+v", got)
	}
	if got := e.Search(context.Background(), "code", 10); len(got) == 0 || got[0].Entry.ID != "vsc" {
		t.Fatalf("new alias search = %+v", got)
	}
}

func TestPersonalBonus(t *testing.T) {
	now := time.Now()
	recent := model.Profile{UseCount: 1, LastUsedAt: &now}
	if PersonalBonus(recent, now) < 400 {
		t.Fatal("recent use not boosted")
	}
	old := now.Add(-40 * 24 * time.Hour)
	if PersonalBonus(model.Profile{UseCount: 1, LastUsedAt: &old}, now) > 10 {
		t.Fatal("old use still boosted")
	}
}
