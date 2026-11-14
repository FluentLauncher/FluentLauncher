package search

import (
	"fmt"
	"strings"
	"time"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/locales"
)

// The UUID, time and date tools answer with one row per format, so that the
// arrows pick the format and Enter copies it. A row's ID is
// "<tool>:<format>:<key>": the format names the row, the key is what the
// rows share (the UUID, the time, the ISO date).

// formatRow is one format of a tool's value: its name in the ID, the text,
// and the subtitle saying what format it is.
type formatRow struct {
	format, value, hint string
}

// formatResults lists the rows in order: the scores step down from the
// tool's usual one, so the first format stays the default selection.
func formatResults(tool, key string, rows []formatRow) []model.Result {
	out := make([]model.Result, 0, len(rows))
	for i, row := range rows {
		out = append(out, model.Result{Entry: model.Entry{
			ID:       tool + ":" + row.format + ":" + key,
			Title:    row.value,
			Subtitle: row.hint,
			Kind:     model.Calculation,
			CopyText: row.value,
			SourceID: "utilities",
		}, Score: 20000 - i})
	}
	return out
}

// splitFormatID splits a format row's ID into its tool, format and key.
func splitFormatID(id string) (tool, format, key string, ok bool) {
	parts := strings.SplitN(id, ":", 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

func uuidRows(value string) []formatRow {
	plain := strings.ReplaceAll(value, "-", "")
	return []formatRow{
		{"std", value, locales.ResultUUIDHint()},
		{"plain", plain, locales.FormatNoHyphens()},
		{"upper", strings.ToUpper(value), locales.FormatUppercase()},
		{"upper-plain", strings.ToUpper(plain), locales.FormatUpperNoHyphens()},
		{"braces", "{" + strings.ToUpper(value) + "}", locales.FormatBraces()},
	}
}

// UUIDResults lists a UUID in its formats, canonical first.
func UUIDResults(value string) []model.Result {
	return formatResults("uuid", value, uuidRows(value))
}

// RefreshUUIDs gives every UUID row in results one new UUID, each row
// keeping its format, and reports whether there was any to refresh.
func RefreshUUIDs(results []model.Result) bool {
	value := NewUUID()
	if value == "" {
		return false
	}
	fresh := map[string]model.Entry{}
	for _, result := range UUIDResults(value) {
		_, format, _, _ := splitFormatID(result.Entry.ID)
		fresh[format] = result.Entry
	}
	changed := false
	for i := range results {
		tool, format, _, ok := splitFormatID(results[i].Entry.ID)
		entry, known := fresh[format]
		if !ok || tool != "uuid" || !known {
			continue
		}
		results[i].Entry.ID, results[i].Entry.Title, results[i].Entry.CopyText = entry.ID, entry.Title, entry.CopyText
		changed = true
	}
	return changed
}

func timeResults(now time.Time) []model.Result {
	return formatResults("time", now.Format("15:04:05"), []formatRow{
		{"hms", now.Format("15:04:05"), locales.ResultTimeHint()},
		{"datetime", now.Format("2006-01-02 15:04:05"), locales.FormatWithDate()},
		{"hm", now.Format("15:04"), locales.FormatNoSeconds()},
		{"iso", now.Format(time.RFC3339), locales.FormatISO8601()},
	})
}

// dateResults lists a date in its formats; hint names the first row, today
// or the result of date math.
func dateResults(t time.Time, hint string) []model.Result {
	iso := t.Format("2006-01-02")
	return formatResults("date", iso, []formatRow{
		{"iso", iso, hint},
		{"slash", t.Format("2006/01/02"), locales.FormatSlashes()},
		{"compact", t.Format("20060102"), locales.FormatCompact()},
		{"zh", fmt.Sprintf("%d年%d月%d日", t.Year(), t.Month(), t.Day()), locales.FormatChinese()},
	})
}
