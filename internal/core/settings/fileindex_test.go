package settings

import "testing"

func TestEverythingChangesReuseLocalIndex(t *testing.T) {
	original := DefaultFileIndexOptions()
	original.Directories = []string{`C:\Files`}
	changed := original
	changed.EnableEverything = true
	changed.EverythingPath = `C:\Everything.exe`
	changed.EverythingAutoStart = !original.EverythingAutoStart
	if !original.SameIndex(changed) {
		t.Fatal("Everything change invalidated local file index")
	}
	for _, change := range []func(*FileIndexOptions){
		func(v *FileIndexOptions) { v.MaxEntries++ },
		func(v *FileIndexOptions) { v.IncludeDirectories = !v.IncludeDirectories },
		func(v *FileIndexOptions) { v.Directories = []string{`C:\Other`} },
		func(v *FileIndexOptions) { v.ExcludedDirectories = []string{`C:\Files\Cache`} },
	} {
		changed = original
		change(&changed)
		if original.SameIndex(changed) {
			t.Fatal("index option change did not invalidate index")
		}
	}
}
