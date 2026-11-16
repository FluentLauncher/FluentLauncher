//go:build windows

package shortcut

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// Resolving a shortcut used to release its object twice, which crashed the
// launcher at startup once desktop shortcuts were scanned. Many resolutions,
// from goroutines on different threads, exercise the reference counting.
func TestResolveRepeatedly(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe")
	link := filepath.Join(dir, "Notepad.lnk")
	createShortcut(t, link, target, "--flag")

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 50 {
				got, ok := Resolve(link)
				if !ok {
					t.Error("shortcut was not resolved")
					return
				}
				if !strings.EqualFold(got.Path, target) || got.Arguments != "--flag" {
					t.Errorf("target = %q %q, want %q --flag", got.Path, got.Arguments, target)
					return
				}
			}
		})
	}
	wg.Wait()

	if _, ok := Resolve(filepath.Join(dir, "missing.lnk")); ok {
		t.Error("a missing shortcut resolved")
	}
}

func createShortcut(t *testing.T, path, target, arguments string) {
	t.Helper()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err == nil {
		defer ole.CoUninitialize()
	}
	unknown, err := oleutil.CreateObject("WScript.Shell")
	if err != nil {
		t.Fatal(err)
	}
	defer unknown.Release()
	shell, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		t.Fatal(err)
	}
	defer shell.Release()
	result, err := oleutil.CallMethod(shell, "CreateShortcut", path)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Clear()
	link := result.ToIDispatch()
	for name, value := range map[string]string{"TargetPath": target, "Arguments": arguments} {
		if _, err := oleutil.PutProperty(link, name, value); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := oleutil.CallMethod(link, "Save"); err != nil {
		t.Fatal(err)
	}
}
