package apps

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"fluentlauncher/internal/core/systemsearch"
)

var resolvedSystemTargets = sync.OnceValue(func() map[systemsearch.Target]string {
	systemDir, _ := windows.GetSystemDirectory()
	windowsDir, _ := windows.GetWindowsDirectory()
	probe := systemTargetProbe{systemDir: systemDir, windowsDir: windowsDir, canonicalNames: controlPanelNames()}
	resolved := make(map[systemsearch.Target]string)
	for _, entry := range systemsearch.All() {
		if path, ok := probe.resolve(entry.Target); ok {
			resolved[entry.Target] = path
		}
	}
	return resolved
})

func ResolveSystemTarget(target systemsearch.Target) (string, bool) {
	path, ok := resolvedSystemTargets()[target]
	return path, ok
}

type systemTargetProbe struct {
	systemDir, windowsDir string
	canonicalNames        map[string]bool
}

func (probe systemTargetProbe) resolve(target systemsearch.Target) (string, bool) {
	if target.URI {
		return target.File, true
	}
	if !probe.validArguments(target) {
		return "", false
	}
	for _, directory := range []string{probe.systemDir, probe.windowsDir} {
		if hasSystemFile(directory, target.File) {
			return filepath.Join(directory, target.File), true
		}
	}
	return "", false
}

func (probe systemTargetProbe) validArguments(target systemsearch.Target) bool {
	file, args := strings.ToLower(target.File), strings.ToLower(target.Arguments)
	switch file {
	case "control.exe":
		if args == "" || args == "keyboard" {
			return true
		}
		if name, ok := strings.CutPrefix(args, "/name "); ok {
			return probe.canonicalNames[name]
		}
		return strings.HasSuffix(args, ".cpl") && hasSystemFile(probe.systemDir, args)
	case "rundll32.exe":
		return args == "sysdm.cpl,editenvironmentvariables" && hasSystemFile(probe.systemDir, "sysdm.cpl")
	default:
		return args == ""
	}
}

func hasSystemFile(directory, name string) bool {
	if directory == "" || filepath.Base(name) != name {
		return false
	}
	info, err := os.Stat(filepath.Join(directory, name))
	return err == nil && !info.IsDir()
}

func controlPanelNames() map[string]bool {
	namespace, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\ControlPanel\NameSpace`, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer namespace.Close()
	classIDs, _ := namespace.ReadSubKeyNames(-1)
	names := make(map[string]bool, len(classIDs))
	for _, classID := range classIDs {
		classKey, err := registry.OpenKey(registry.CLASSES_ROOT, `CLSID\`+classID, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		name, _, err := classKey.GetStringValue("System.ApplicationName")
		classKey.Close()
		if err == nil && name != "" {
			names[strings.ToLower(name)] = true
		}
	}
	return names
}
