package bundled

import (
	"crypto/sha256"
	"os"
	"testing"
)

func TestSQLiteRuntimeCacheIntegrity(t *testing.T) {
	cache := t.TempDir()
	path, err := SQLiteLibrary(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt cache"), 0600); err != nil {
		t.Fatal(err)
	}
	repaired, err := SQLiteLibrary(cache)
	if err != nil || repaired != path {
		t.Fatalf("repair: %s, %v", repaired, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(data) != sha256.Sum256(sqliteLibrary) {
		t.Fatal("DLL integrity check failed")
	}
}
