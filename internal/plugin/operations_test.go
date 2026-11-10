package plugin

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestRepositoryConcurrentRefreshAndReplacement(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "plugin.zip")
	makePluginZip(t, archive, "echo.exe", false)
	r := NewRepository(filepath.Join(t.TempDir(), "plugins"))
	if _, err := r.Install(archive, false); err != nil {
		t.Fatal(err)
	}
	errors := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 15; i++ {
			if _, err := r.Install(archive, true); err != nil {
				errors <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			if err := r.Refresh(); err != nil {
				errors <- err
				return
			}
			if got := r.Installed(); len(got) != 1 || got[0].Manifest.ID != "official.echo" {
				errors <- fmt.Errorf("scan observed a partial replacement: %+v", got)
				return
			}
		}
	}()
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}
