package apps

import (
	"os"
	"path/filepath"
	"testing"

	"fluentlauncher/internal/core/systemsearch"
)

func TestSystemTargetProbe(t *testing.T) {
	root := t.TempDir()
	systemDir := filepath.Join(root, "System32")
	if err := os.Mkdir(systemDir, 0o755); err != nil {
		t.Fatal(err)
	}
	control := filepath.Join(systemDir, "control.exe")
	regedit := filepath.Join(root, "regedit.exe")
	for _, path := range []string{control, regedit, filepath.Join(systemDir, "ncpa.cpl")} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	probe := systemTargetProbe{
		systemDir:      systemDir,
		windowsDir:     root,
		canonicalNames: map[string]bool{"microsoft.fonts": true},
	}
	for _, test := range []struct {
		name   string
		target systemsearch.Target
		want   string
	}{
		{"settings URI", systemsearch.Target{File: "ms-settings:privacy-webcam", URI: true}, "ms-settings:privacy-webcam"},
		{"installed applet", systemsearch.Target{File: "control.exe", Arguments: "ncpa.cpl"}, control},
		{"missing applet", systemsearch.Target{File: "control.exe", Arguments: "missing.cpl"}, ""},
		{"mistyped applet", systemsearch.Target{File: "control.exe", Arguments: "ncpa.cp"}, ""},
		{"registered canonical name", systemsearch.Target{File: "control.exe", Arguments: "/name Microsoft.Fonts"}, control},
		{"unregistered canonical name", systemsearch.Target{File: "control.exe", Arguments: "/name Microsoft.NetworkConnections"}, ""},
		{"Windows directory executable", systemsearch.Target{File: "regedit.exe"}, regedit},
		{"missing optional executable", systemsearch.Target{File: "wfs.exe"}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := probe.resolve(test.target)
			if got != test.want || ok != (test.want != "") {
				t.Errorf("resolve(%+v) = %q, %v; want %q", test.target, got, ok, test.want)
			}
		})
	}
}
