package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/systemsearch"
	"fluentlauncher/locales"
)

// SystemProvider exposes the Windows settings, Control Panel and system tool
// catalog (internal/core/systemsearch). It answers only an unscoped search: the
// catalog is neither an application, a file nor a window, so "app 网络" or
// "win 网络" must not list settings pages.
type SystemProvider struct{}

func (SystemProvider) ID() string { return "system" }

func (SystemProvider) Supports(q Query) bool {
	return q.Scope == ScopeAll && strings.TrimSpace(q.Text) != ""
}

func (SystemProvider) Search(ctx context.Context, q Query, limit int) ([]model.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	matches := systemsearch.Search(q.Text, limit)
	if len(matches) == 0 {
		return nil, nil
	}
	results := make([]model.Result, 0, len(matches))
	for _, match := range matches {
		entry := match.Entry
		target := entry.Target
		result := model.Entry{
			ID:           systemID(target),
			Title:        systemsearch.DisplayTitle(entry),
			Subtitle:     systemsearch.DisplayCategory(entry) + " · " + targetKind(target),
			LaunchTarget: target.File,
			Arguments:    target.Arguments,
			IconPath:     targetIcon(target),
			SourceID:     "system",
			// Setting tells apps.Launch to open a protocol URI with the shell
			// or a tool with its arguments; the URL kind would drop them, and
			// the learning actions it suppresses do not apply here either.
			Kind: model.Setting,
		}
		results = append(results, model.Result{Entry: result, Score: match.Score})
	}
	return results, nil
}

// targetKind names the place a target opens, as the result subtitle shows it:
// a settings page, a control panel item or a system tool. The raw URI would
// read as an error message to a reader.
func targetKind(target systemsearch.Target) string {
	file := strings.ToLower(target.File)
	switch {
	case target.URI:
		if strings.HasPrefix(file, "ms-settings:") {
			return locales.ResultCategoryWindowsSettings()
		}
		if strings.HasPrefix(file, "ms-") || strings.HasPrefix(file, "shell:") {
			return locales.ResultCategoryWindowsTools()
		}
		return locales.ResultCategorySystemFeatures()
	case strings.HasSuffix(file, ".cpl"):
		return locales.ResultCategoryControlPanel()
	case strings.HasSuffix(file, ".msc"):
		return locales.ResultCategoryAdminTools()
	case strings.EqualFold(file, "control.exe"):
		return locales.ResultCategoryControlPanel()
	}
	return locales.ResultCategorySystemTools()
}

// targetIcon is the file whose icon the row shows. A bare program name such as
// control.exe is resolved under System32: the shell cannot find it by name, and
// the row would fall back to a letter badge.
func targetIcon(target systemsearch.Target) string {
	if target.URI {
		return ""
	}
	file := target.File
	if strings.ContainsAny(file, `\/:`) || !strings.HasSuffix(strings.ToLower(file), ".exe") {
		return file
	}
	if root := os.Getenv("SystemRoot"); root != "" {
		return filepath.Join(root, "System32", file)
	}
	return file
}

// systemID keys the entry's use counter. It is stable across restarts and
// unique per target, so "控制面板" and "网络连接" never share a profile.
func systemID(target systemsearch.Target) string {
	return "system:" + strings.ToLower(target.Display())
}
