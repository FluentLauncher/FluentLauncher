package systemsearch

import (
	"strings"
	"testing"
)

func titles(matches []Match) []string {
	out := make([]string, len(matches))
	for i, match := range matches {
		out[i] = match.Entry.Title
	}
	return out
}

func hasTitle(matches []Match, title string) bool {
	for _, match := range matches {
		if match.Entry.Title == title {
			return true
		}
	}
	return false
}

func TestSearchEmptyQuery(t *testing.T) {
	for _, query := range []string{"", "   ", "\t", "\u3000"} {
		if got := Search(query, 10); len(got) != 0 {
			t.Errorf("Search(%q) = %v, want no results", query, titles(got))
		}
	}
	if got := Search("网络", 0); len(got) != 0 {
		t.Errorf("Search with limit 0 = %v, want no results", titles(got))
	}
}

func TestSearchFirstResult(t *testing.T) {
	tests := []struct {
		query string
		want  []string // the first result is one of these
	}{
		{"控制面板", []string{"控制面板"}},
		{"显示设置", []string{"显示设置"}},
		{"display", []string{"显示设置"}},
		{"wifi", []string{"Wi-Fi"}},
		{"WLAN", []string{"Wi-Fi"}},
		{"蓝牙", []string{"蓝牙"}},
		{"打印机", []string{"打印机和扫描仪"}},
		{"电源", []string{"电源和电池", "电源选项"}},
		{"声音", []string{"声音设置"}},
		{"注册表", []string{"注册表编辑器"}},
		{"任务管理器", []string{"任务管理器"}},
		{"ncpa", []string{"网络连接"}},
		{"卸载", []string{"已安装的应用", "程序和功能"}},
		{"刷新率", []string{"显示设置", "高级显示"}},
	}
	for _, tt := range tests {
		got := Search(tt.query, 10)
		if len(got) == 0 {
			t.Errorf("Search(%q) found nothing", tt.query)
			continue
		}
		first := got[0].Entry.Title
		matched := false
		for _, want := range tt.want {
			if first == want {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("Search(%q) first = %q, want one of %v (all: %v)",
				tt.query, first, tt.want, titles(got))
		}
	}
}

func TestSearchPinyinInitials(t *testing.T) {
	got := Search("sy", 20)
	if !hasTitle(got, "声音设置") {
		t.Errorf(`Search("sy") = %v, want 声音设置 via pinyin initials`, titles(got))
	}
	got = Search("sb", 20)
	if !hasTitle(got, "鼠标") && !hasTitle(got, "鼠标属性") {
		t.Errorf(`Search("sb") = %v, want 鼠标 via pinyin initials`, titles(got))
	}
	got = Search("cp", 20)
	if !hasTitle(got, "控制面板") {
		t.Errorf(`Search("cp") = %v, want 控制面板 via "Control Panel" initials`, titles(got))
	}
}

func TestSearchMultipleKeywords(t *testing.T) {
	got := Search("网络 代理", 10)
	if len(got) == 0 || got[0].Entry.Title != "代理" {
		t.Fatalf(`Search("网络 代理") = %v, want 代理 first`, titles(got))
	}
	if !got[0].Complete {
		t.Fatal("代理 matched every token, so it must be complete")
	}
	// Complete hits come first, partial ones after them, never in between.
	seenPartial := false
	for _, match := range got {
		if !match.Complete {
			seenPartial = true
			continue
		}
		if seenPartial {
			t.Fatalf("complete hit %q ranked after a partial one: %v", match.Entry.Title, titles(got))
		}
	}
	// The order of the tokens does not matter.
	reversed := Search("代理 网络", 10)
	if len(reversed) == 0 || reversed[0].Entry.Title != "代理" {
		t.Fatalf(`Search("代理 网络") = %v, want 代理 first`, titles(reversed))
	}
	// A second keyword never invents matches: every complete hit for
	// "网络 代理" is also found by "网络" alone.
	single := map[string]bool{}
	for _, match := range Search("网络", 100) {
		single[match.Entry.Title] = true
	}
	for _, match := range Search("网络 代理", 100) {
		if match.Complete && !single[match.Entry.Title] {
			t.Errorf("%q is complete for 网络 代理 but not found by 网络", match.Entry.Title)
		}
	}
}

func TestSearchNoMatch(t *testing.T) {
	for _, query := range []string{"zzzz", "没有这个命名空间", "qwertyuiop"} {
		if got := Search(query, 10); len(got) != 0 {
			t.Errorf("Search(%q) = %v, want no results", query, titles(got))
		}
	}
}

func TestSearchLimit(t *testing.T) {
	all := Search("设置", 100)
	if len(all) == 0 {
		t.Fatal("Search(设置) found nothing")
	}
	if got := Search("设置", 3); len(got) != 3 {
		t.Fatalf("limit 3 returned %d results", len(got))
	}
	if got := Search("设置", 1); len(got) != 1 {
		t.Fatalf("limit 1 returned %d results", len(got))
	}
	// The same matches come back in the same order every time.
	again := Search("设置", 100)
	for i := range all {
		if all[i].Entry.Title != again[i].Entry.Title {
			t.Fatalf("unstable order at %d: %q vs %q", i, all[i].Entry.Title, again[i].Entry.Title)
		}
	}
}

func TestCatalogCompleteness(t *testing.T) {
	all := All()
	if len(all) < 50 || len(all) > 80 {
		t.Fatalf("catalog has %d entries, want 50..80", len(all))
	}
	required := []string{
		"系统", "显示", "声音", "网络", "蓝牙与设备", "打印机", "电源",
		"更新与安全", "账户", "时间与语言", "应用与卸载", "鼠标键盘", "存储",
		"隐私", "开发者选项", "服务", "任务管理器", "注册表编辑器", "控制面板",
	}
	requiredSet := make(map[string]bool, len(required))
	for _, category := range required {
		requiredSet[category] = true
	}
	perCategory := map[string]int{}
	titles := map[string]bool{}
	targets := map[string]bool{}
	for _, entry := range all {
		if entry.Title == "" || entry.Latin == "" || entry.Category == "" {
			t.Errorf("entry %+v has an empty title, latin name or category", entry)
		}
		if len(entry.Keywords) < 3 {
			t.Errorf("%q has %d keywords, want at least 3", entry.Title, len(entry.Keywords))
		}
		if entry.Target.File == "" {
			t.Errorf("%q has no target", entry.Title)
		}
		if entry.Target.URI {
			if !strings.HasPrefix(entry.Target.File, "ms-settings:") {
				t.Errorf("%q has an unexpected URI target %q", entry.Title, entry.Target.File)
			}
			if entry.Target.Arguments != "" {
				t.Errorf("%q is a URI but carries arguments %q", entry.Title, entry.Target.Arguments)
			}
		} else if strings.Contains(entry.Target.File, ":") {
			t.Errorf("%q has a command target with a scheme: %q", entry.Title, entry.Target.File)
		}
		if titles[entry.Title] {
			t.Errorf("duplicate title %q", entry.Title)
		}
		titles[entry.Title] = true
		if targets[entry.Target.Display()] {
			t.Errorf("duplicate target %q", entry.Target.Display())
		}
		targets[entry.Target.Display()] = true
		if !requiredSet[entry.Category] {
			t.Errorf("%q has the unknown category %q", entry.Title, entry.Category)
		}
		perCategory[entry.Category]++
	}
	for _, category := range required {
		if perCategory[category] == 0 {
			t.Errorf("category %q has no entries", category)
		}
	}
	if len(perCategory) != len(required) {
		t.Errorf("catalog covers %d categories, want %d", len(perCategory), len(required))
	}
}

// TestCatalogEntriesAreSearchable is the data test that keeps the catalog
// useful: every entry must be found by its own title and by its English name.
func TestCatalogEntriesAreSearchable(t *testing.T) {
	for _, entry := range All() {
		byTitle := Search(entry.Title, 20)
		if !hasTitle(byTitle, entry.Title) {
			t.Errorf("title %q is not searchable (got %v)", entry.Title, titles(byTitle))
		}
		byLatin := Search(entry.Latin, 20)
		if !hasTitle(byLatin, entry.Title) {
			t.Errorf("%q is not found by its Latin name %q (got %v)",
				entry.Title, entry.Latin, titles(byLatin))
		}
	}
}
