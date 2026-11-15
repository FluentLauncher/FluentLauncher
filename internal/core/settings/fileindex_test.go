package settings

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

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

func TestEverythingExcludesDefaultsAndValidation(t *testing.T) {
	var options FileIndexOptions
	if err := json.Unmarshal([]byte(`{"EnableEverything":true}`), &options); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(options.EverythingExcludes, DefaultEverythingExcludes()) {
		t.Fatalf("old configuration did not receive default exclusions: %q", options.EverythingExcludes)
	}
	if err := json.Unmarshal([]byte(`{"EverythingExcludes":[]}`), &options); err != nil || len(options.EverythingExcludes) != 0 {
		t.Fatal("an emptied exclusion list was not kept")
	}
	if err := ValidateEverythingExcludes(DefaultEverythingExcludes()); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{{" "}, {`C:\"x`}, {strings.Repeat("a", 261)}, make([]string, 65)} {
		if ValidateEverythingExcludes(bad) == nil {
			t.Fatalf("invalid exclusions accepted: %q", bad)
		}
	}
	changed := DefaultFileIndexOptions()
	changed.EverythingExcludes = nil
	if !DefaultFileIndexOptions().SameIndex(changed) {
		t.Fatal("Everything exclusions invalidated the local index")
	}
}
