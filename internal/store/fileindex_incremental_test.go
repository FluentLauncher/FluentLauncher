package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
)

func newIndexFixture(t testing.TB) (*FileIndex, string, settings.FileIndexOptions) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "files")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(context.Background(), filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	f := NewFileIndex(db)
	t.Cleanup(func() { f.Close(); db.Close() })
	options := settings.DefaultFileIndexOptions()
	options.Directories = []string{root}
	f.options = options // No OS watcher: tests control exactly which hints arrive.
	return f, root, options
}

func writeIndexFile(t testing.TB, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
}

func flushIndexHints(t *testing.T, f *FileIndex) {
	t.Helper()
	done := make(chan error, 1)
	f.mu.Lock()
	if f.pending.empty() {
		f.mu.Unlock()
		t.Fatal("no pending directory hints")
	}
	if f.timer != nil {
		f.timer.Stop()
	}
	f.pending.waiters = append(f.pending.waiters, done)
	if !f.running {
		f.running = true
		go f.work()
	}
	f.mu.Unlock()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("index did not flush")
	}
}

func assertIndexMatchesFullScan(t *testing.T, f *FileIndex, options settings.FileIndexOptions) {
	t.Helper()
	got, err := f.db.LoadFileIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := Scan(context.Background(), options)
	sort.Slice(got, func(i, j int) bool { return got[i].ID < got[j].ID })
	sort.Slice(want, func(i, j int) bool { return want[i].ID < want[j].ID })
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("incremental/full mismatch: got %+v want %+v", got, want)
	}
}

func TestIncrementalIndexScansOnlyHintedSubtree(t *testing.T) {
	f, root, options := newIndexFixture(t)
	left, right := filepath.Join(root, "left"), filepath.Join(root, "right")
	writeIndexFile(t, filepath.Join(left, "old.txt"))
	writeIndexFile(t, filepath.Join(right, "keep.txt"))
	if err := f.Rebuild(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(left, "old.txt"), filepath.Join(left, "new.txt")); err != nil {
		t.Fatal(err)
	}
	writeIndexFile(t, filepath.Join(left, "child", "added.txt"))
	// This unrelated change has no hint yet. A full scan would remove it.
	if err := os.Remove(filepath.Join(right, "keep.txt")); err != nil {
		t.Fatal(err)
	}
	f.schedule(f.watchCtx, root, []mygo.FileEvent{{Op: mygo.FileHint, Path: "left"}, {Op: mygo.FileHint, Path: "left/child"}})
	flushIndexHints(t, f)
	got, err := f.db.LoadFileIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range got {
		if strings.HasSuffix(entry.LaunchTarget, `right\keep.txt`) {
			found = true
		}
	}
	if !found {
		t.Fatal("unrelated subtree was scanned")
	}
	f.schedule(f.watchCtx, root, []mygo.FileEvent{{Op: mygo.FileHint, Path: "right"}})
	flushIndexHints(t, f)
	assertIndexMatchesFullScan(t, f, options)
}

func TestIncrementalIndexRenameDeleteAndDirectoryOptions(t *testing.T) {
	for _, include := range []bool{false, true} {
		t.Run(fmt.Sprint(include), func(t *testing.T) {
			f, root, options := newIndexFixture(t)
			options.IncludeDirectories = include
			options.Directories = append(options.Directories, filepath.Join(root, "group")) // Overlapping roots.
			options.ExcludedDirectories = []string{filepath.Join(root, "group", "skip")}
			f.options = options
			writeIndexFile(t, filepath.Join(root, "group", "old", "a.txt"))
			writeIndexFile(t, filepath.Join(root, "group2", "b.txt"))
			writeIndexFile(t, filepath.Join(root, "group", "skip", "excluded.txt"))
			if err := f.Rebuild(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(filepath.Join(root, "group", "old"), filepath.Join(root, "group", "new")); err != nil {
				t.Fatal(err)
			}
			f.schedule(f.watchCtx, root, []mygo.FileEvent{{Op: mygo.FileHint, Path: "group"}})
			flushIndexHints(t, f)
			assertIndexMatchesFullScan(t, f, options)
			if err := os.Remove(filepath.Join(root, "group", "new", "a.txt")); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(root, "group", "new")); err != nil {
				t.Fatal(err)
			}
			f.schedule(f.watchCtx, root, []mygo.FileEvent{{Op: mygo.FileHint, Path: "group/new"}})
			flushIndexHints(t, f)
			assertIndexMatchesFullScan(t, f, options)
		})
	}
}

func TestIndexCapacityFallsBackToFullScan(t *testing.T) {
	f, root, options := newIndexFixture(t)
	options.MaxEntries, options.IncludeDirectories = 3, false
	f.options = options
	for i := range 8 {
		writeIndexFile(t, filepath.Join(root, fmt.Sprintf("dir%d", i%2), fmt.Sprintf("%d.txt", i)))
	}
	if err := f.Rebuild(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries, _ := f.db.LoadFileIndex(context.Background())
	if err := os.Remove(entries[0].LaunchTarget); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Base(filepath.Dir(entries[0].LaunchTarget))
	f.schedule(f.watchCtx, root, []mygo.FileEvent{{Op: mygo.FileHint, Path: dir}})
	flushIndexHints(t, f)
	assertIndexMatchesFullScan(t, f, options)
	entries, _ = f.db.LoadFileIndex(context.Background())
	if len(entries) != 3 {
		t.Fatal("saturated index was not refilled")
	}
}

func TestIndexHintsStayWithinRoots(t *testing.T) {
	f, root, options := newIndexFixture(t)
	options.ExcludedDirectories = []string{filepath.Join(root, "skip")}
	f.options = options
	for _, path := range []string{"../outside", "skip/child"} {
		f.schedule(f.watchCtx, root, []mygo.FileEvent{{Op: mygo.FileHint, Path: path}})
	}
	f.mu.Lock()
	empty := f.pending.empty()
	f.mu.Unlock()
	if !empty {
		t.Fatal("outside or excluded hint scheduled work")
	}
	f.schedule(f.watchCtx, root, []mygo.FileEvent{{Op: mygo.FileHint, Path: "."}})
	f.mu.Lock()
	full := f.pending.full
	f.mu.Unlock()
	if !full {
		t.Fatal("root/lost-event hint did not request full scan")
	}
	f.mu.Lock()
	f.pending = indexRequest{}
	f.mu.Unlock()
	var burst []mygo.FileEvent
	for i := range maxHintDirectories + 1 {
		burst = append(burst, mygo.FileEvent{Op: mygo.FileHint, Path: fmt.Sprintf("dir%d", i)})
	}
	f.schedule(f.watchCtx, root, burst)
	f.mu.Lock()
	bounded := f.pending.full && len(f.pending.dirs) == 0
	f.mu.Unlock()
	if !bounded {
		t.Fatal("directory hint burst grew an unbounded pending set")
	}
	if !insideAny([]string{filepath.VolumeName(root) + `\`}, root) {
		t.Fatal("drive root boundary rejected descendants")
	}
}

func TestIncrementalGrowthRespectsCapacity(t *testing.T) {
	f, root, options := newIndexFixture(t)
	options.MaxEntries, options.IncludeDirectories = 4, false
	f.options = options
	writeIndexFile(t, filepath.Join(root, "left", "original.txt"))
	writeIndexFile(t, filepath.Join(root, "right", "original.txt"))
	if err := f.Rebuild(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := range 8 {
		writeIndexFile(t, filepath.Join(root, "left", fmt.Sprintf("new%d.txt", i)))
	}
	f.schedule(f.watchCtx, root, []mygo.FileEvent{{Op: mygo.FileHint, Path: "left"}})
	flushIndexHints(t, f)
	assertIndexMatchesFullScan(t, f, options)
	entries, _ := f.db.LoadFileIndex(context.Background())
	if len(entries) != 4 {
		t.Fatal("incremental growth exceeded index capacity")
	}
}

func TestIndexBurstSharesOneFollowUpPass(t *testing.T) {
	f, root, _ := newIndexFixture(t)
	writeIndexFile(t, filepath.Join(root, "left", "first.txt"))
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var publications atomic.Int32
	f.SetOnUpdate(func([]model.Entry) {
		if publications.Add(1) == 1 {
			close(started)
			<-release
		}
	})
	first := make(chan error, 1)
	go func() { first <- f.Rebuild(context.Background()) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first pass did not start")
	}
	writeIndexFile(t, filepath.Join(root, "left", "next.txt"))
	done := make(chan error, 20)
	for range 20 {
		go func() { done <- f.Rebuild(context.Background()) }()
	}
	deadline := time.Now().Add(5 * time.Second)
	queued := 0
	for time.Now().Before(deadline) {
		f.mu.Lock()
		queued = len(f.pending.waiters)
		f.mu.Unlock()
		if queued == 20 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if queued != 20 {
		t.Fatal("rebuild callers did not queue")
	}
	for range 100 {
		f.schedule(f.watchCtx, root, []mygo.FileEvent{{Op: mygo.FileHint, Path: "left"}})
	}
	unblock()
	for range 20 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("queued rebuild stalled")
		}
	}
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if publications.Load() != 2 {
		t.Fatalf("burst produced %d snapshot publications", publications.Load())
	}
}

func TestPartialScanDoesNotFollowReplacedDirectoryLink(t *testing.T) {
	f, root, options := newIndexFixture(t)
	outside := t.TempDir()
	writeIndexFile(t, filepath.Join(outside, "outside.txt"))
	writeIndexFile(t, filepath.Join(outside, "child", "outside.txt"))
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		// Windows permits local directory junctions without the symbolic-link
		// privilege. They exercise the same reparse-point boundary we skip.
		if output, junctionErr := exec.Command("cmd", "/d", "/c", "mklink", "/J", link, outside).CombinedOutput(); junctionErr != nil {
			t.Skipf("directory links unavailable: %v; junction: %v %s", err, junctionErr, output)
		}
		t.Log("using a directory junction because symbolic-link privilege is unavailable")
	}
	t.Cleanup(func() {
		if err := os.Remove(link); err != nil {
			t.Error(err)
		}
	})
	for _, path := range []string{link, filepath.Join(link, "child")} {
		got, err := scanPaths(context.Background(), options, []string{path}, true)
		if err != nil || len(got) != 0 {
			t.Fatalf("followed directory link: %+v %v", got, err)
		}
	}
	_ = f
}

func TestIndexCloseReleasesQueuedRebuilds(t *testing.T) {
	f, root, _ := newIndexFixture(t)
	writeIndexFile(t, filepath.Join(root, "first.txt"))
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	f.SetOnUpdate(func([]model.Entry) { close(started); <-release })
	first, queued := make(chan error, 1), make(chan error, 1)
	go func() { first <- f.Rebuild(context.Background()) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not start")
	}
	go func() { queued <- f.Rebuild(context.Background()) }()
	deadline := time.Now().Add(5 * time.Second)
	pending := false
	for time.Now().Before(deadline) {
		f.mu.Lock()
		pending = len(f.pending.waiters) == 1
		f.mu.Unlock()
		if pending {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !pending {
		t.Fatal("follow-up rebuild did not queue")
	}
	f.Close()
	select {
	case err := <-queued:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued close result: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("close left queued caller blocked")
	}
	unblock()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("in-flight close result: %v", err)
	}
}

func BenchmarkFileIndexRefresh(b *testing.B) {
	f, root, options := newIndexFixture(b)
	options.MaxEntries = 20000
	f.options = options
	for i := range 10000 {
		writeIndexFile(b, filepath.Join(root, fmt.Sprintf("dir%03d", i/100), fmt.Sprintf("%05d.txt", i)))
	}
	if err := f.Rebuild(context.Background()); err != nil {
		b.Fatal(err)
	}
	for _, full := range []bool{true, false} {
		name := "Subtree100of10000"
		if full {
			name = "Full10000"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			request := indexRequest{full: full, dirs: []string{filepath.Join(root, "dir050")}}
			for b.Loop() {
				if err := f.refresh(context.Background(), options, request); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
