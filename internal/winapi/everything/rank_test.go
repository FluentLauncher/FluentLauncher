//go:build windows

package everything

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/search"
	"fluentlauncher/internal/core/settings"
)

func fileResult(path string, dir bool) model.Result {
	kind := model.File
	if dir {
		kind = model.Directory
	}
	return model.Result{Entry: model.Entry{ID: "file:" + strings.ToUpper(path), Title: filepath.Base(path),
		LaunchTarget: path, Kind: kind, SourceID: "everything"}}
}

// Everything answers in index order; the most useful hits for "winscp" were
// beyond the first ten and lost behind extension scripts.
func TestRankResultsPrefersNameMatchesAndLaunchables(t *testing.T) {
	results := []model.Result{
		fileResult(`D:\Program Files (x86)\WinSCP\Extensions\ArchiveDownload.WinSCPextension.ps1`, false),
		fileResult(`D:\Program Files (x86)\WinSCP\Extensions\BatchRename.WinSCPextension.ps1`, false),
		fileResult(`D:\Program Files (x86)\WinSCP\Translations\WinSCP.ar`, false),
		fileResult(`C:\Windows\Prefetch\WINSCP.EXE-F57ABB25.pf`, false),
		fileResult(`D:\Program Files (x86)\WinSCP`, true),
		fileResult(`D:\Program Files (x86)\WinSCP\WinSCP.exe`, false),
		fileResult(`D:\Program Files (x86)\WinSCP\WinSCP.exe`, false), // from both queries
	}
	ranked := rankResults(results, "winscp", 4)
	want := []string{"WinSCP.exe", "WinSCP", "WinSCP.ar", "WINSCP.EXE-F57ABB25.pf"}
	if len(ranked) != len(want) {
		t.Fatalf("ranked = %+v", ranked)
	}
	for i, title := range want {
		if ranked[i].Entry.Title != title {
			t.Fatalf("rank %d = %s, want %s (%+v)", i, ranked[i].Entry.Title, title, ranked)
		}
	}
}

// Everything results must never outrank a local match: the weakest local
// band (an application found only through its subtitle) scores 1000.
func TestRankedScoresStayBelowLocalMatches(t *testing.T) {
	best := fileResult(`C:\a.exe`, false)
	worst := fileResult(`C:\1\2\3\4\5\6\7\8\9\10\11\12\x.txt`, false)
	ranked := rankResults([]model.Result{best, worst}, "a", 10)
	if ranked[0].Score >= 1000 || ranked[len(ranked)-1].Score <= 0 {
		t.Fatalf("scores escaped their band: %+v", ranked)
	}
	app := model.Result{Entry: model.Entry{ID: "app", Title: "文件资源管理器", Kind: model.Application}, Score: 1000}
	merged := search.Merge([]search.Batch{{Results: ranked}, {Results: []model.Result{app}}}, 1)
	if merged[0].Entry.ID != "app" {
		t.Fatal("Everything result outranked a local application")
	}
}

func TestQueryTextAndPrefix(t *testing.T) {
	t.Setenv("FL_TEST_ROOT", `C:\Win`)
	got := queryText("explor", []string{`%FL_TEST_ROOT%\WinSxS\`, " ", `\$Recycle.Bin\`})
	if got != `explor !"C:\Win\WinSxS\" !"\$Recycle.Bin\"` {
		t.Fatalf("query = %s", got)
	}
	if q, ok := prefixQuery("winscp"); !ok || q != "startwith:winscp" {
		t.Fatalf("prefix = %q %v", q, ok)
	}
	for _, text := range []string{"", "two words", "ext:exe", `C:\Temp`, "*.log"} {
		if _, ok := prefixQuery(text); ok {
			t.Fatalf("syntax query %q got a prefix query", text)
		}
	}
}

func TestSearchSendsPrefixAndExcludedQueries(t *testing.T) {
	options := settings.DefaultFileIndexOptions()
	options.EnableEverything = true
	options.EverythingExcludes = []string{`D:\skip\`}
	p := NewProvider(func() settings.FileIndexOptions { return options }, nil)
	p.connection.running = func() bool { return true }
	var sent []string
	p.runQuery = func(_ context.Context, text string, limit int) ([]model.Result, error) {
		sent = append(sent, text)
		return []model.Result{fileResult(`D:\a\winscp.exe`, false)}, nil
	}
	results, err := p.Search(context.Background(), search.Query{Text: "winscp"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sent) != 2 || sent[0] != `startwith:winscp !"D:\skip\"` || sent[1] != `winscp !"D:\skip\"` {
		t.Fatalf("queries = %q", sent)
	}
	if len(results) != 1 {
		t.Fatalf("duplicate path was not merged: %+v", results)
	}
}

func TestIniValueAndRequiresAdmin(t *testing.T) {
	if iniValue("\uFEFF[Everything]\r\nrun_as_admin=1\r\napp_data=0\r\n", "run_as_admin") != "1" {
		t.Fatal("ini value not read")
	}
	root := t.TempDir()
	t.Setenv("APPDATA", filepath.Join(root, "roaming"))
	exe := filepath.Join(root, "portable", "Everything.exe")
	write := func(path, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if requiresAdmin(exe) {
		t.Fatal("missing settings reported elevation")
	}
	write(filepath.Join(root, "portable", "Everything.ini"), "[Everything]\nrun_as_admin=1\n")
	if !requiresAdmin(exe) {
		t.Fatal("portable run_as_admin ignored")
	}
	// app_data=1 moves the settings to %APPDATA%.
	write(filepath.Join(root, "portable", "Everything.ini"), "[Everything]\napp_data=1\nrun_as_admin=1\n")
	write(filepath.Join(root, "roaming", "Everything", "Everything.ini"), "[Everything]\nrun_as_admin=0\n")
	if requiresAdmin(exe) {
		t.Fatal("app_data settings were not used")
	}
}

func TestElevatedClientIsNeverAutoStarted(t *testing.T) {
	var launches atomic.Int32
	c := &connection{
		running:    func() bool { return false },
		resolve:    func(string) (string, error) { return "Everything.exe", nil },
		needsAdmin: func(string) bool { return true },
		launch:     func(string, bool) error { launches.Add(1); return nil },
	}
	options := settings.DefaultFileIndexOptions()
	for range 3 {
		if err := c.ensure(context.Background(), options, false); err == nil {
			t.Fatal("elevated client was treated as available")
		}
		c.retryAt = c.retryAt.AddDate(0, 0, -1) // backoff expiry must not re-arm it
	}
	if launches.Load() != 0 || c.snapshot().State != ConnectionFailed {
		t.Fatalf("keystrokes launched an elevated client: %d", launches.Load())
	}
	// The user's own connect action may raise the prompt.
	_ = c.ensure(context.Background(), options, true)
	if launches.Load() != 1 {
		t.Fatal("manual connection did not start the client")
	}
}

func TestFailedAutoStartIsNotRepeated(t *testing.T) {
	var launches atomic.Int32
	c := &connection{
		running: func() bool { return false },
		resolve: func(string) (string, error) { return "Everything.exe", nil },
		launch:  func(string, bool) error { launches.Add(1); return errors.New("declined") },
	}
	options := settings.DefaultFileIndexOptions()
	for range 3 {
		_ = c.ensure(context.Background(), options, false)
		c.retryAt = c.retryAt.AddDate(0, 0, -1)
	}
	if launches.Load() != 1 {
		t.Fatalf("failed automatic start was repeated %d times", launches.Load())
	}
	options.EverythingPath = "another.exe"
	_ = c.ensure(context.Background(), options, false)
	if launches.Load() != 2 {
		t.Fatal("changed program path did not re-arm automatic start")
	}
}
