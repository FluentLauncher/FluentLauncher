package app

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"fluentlauncher/internal/core/model"
)

// builtinIcons are the line icons of the results the launcher makes up itself
// — the calculator, the instant tools, the settings entry — which have no file
// to take an icon from and would otherwise fall back to a letter avatar.
var builtinIcons = map[string]*ui.SVG{
	"calc":     ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="5" y="3" width="14" height="18" rx="2"/><line x1="8" y1="7" x2="16" y2="7"/><line x1="8" y1="12" x2="8" y2="12.01"/><line x1="12" y1="12" x2="12" y2="12.01"/><line x1="16" y1="12" x2="16" y2="12.01"/><line x1="8" y1="16" x2="8" y2="16.01"/><line x1="12" y1="16" x2="12" y2="16.01"/><line x1="16" y1="16" x2="16" y2="16.01"/></svg>`)),
	"clock":    ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><polyline points="12 7 12 12 15.5 14"/></svg>`)),
	"calendar": ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="5" width="18" height="16" rx="2"/><line x1="8" y1="3" x2="8" y2="7"/><line x1="16" y1="3" x2="16" y2="7"/><line x1="3" y1="10" x2="21" y2="10"/></svg>`)),
	"uuid":     ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="4" y1="9" x2="20" y2="9"/><line x1="4" y1="15" x2="20" y2="15"/><line x1="10" y1="3" x2="8" y2="21"/><line x1="16" y1="3" x2="14" y2="21"/></svg>`)),
	"globe":    ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="9"/><line x1="3" y1="12" x2="21" y2="12"/><path d="M12 3a14 14 0 0 1 0 18"/><path d="M12 3a14 14 0 0 0 0 18"/></svg>`)),
	"gear":     ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.87l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.87-.34 1.7 1.7 0 0 0-1 1.55V21a2 2 0 1 1-4 0v-.09a1.7 1.7 0 0 0-1-1.55 1.7 1.7 0 0 0-1.87.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.7 1.7 0 0 0 .34-1.87 1.7 1.7 0 0 0-1.55-1H3a2 2 0 1 1 0-4h.09a1.7 1.7 0 0 0 1.55-1 1.7 1.7 0 0 0-.34-1.87l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.7 1.7 0 0 0 1.87.34h.09a1.7 1.7 0 0 0 1-1.55V3a2 2 0 1 1 4 0v.09a1.7 1.7 0 0 0 1 1.55 1.7 1.7 0 0 0 1.87-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.7 1.7 0 0 0-.34 1.87v.09a1.7 1.7 0 0 0 1.55 1H21a2 2 0 1 1 0 4h-.09a1.7 1.7 0 0 0-1.55 1z"/></svg>`)),
	"layers":   ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m12 2 9 5-9 5-9-5 9-5z"/><path d="m3 12 9 5 9-5"/><path d="m3 17 9 5 9-5"/></svg>`)),
	"puzzle":   ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 7V4a2 2 0 0 0-4 0v3H7a2 2 0 0 0-2 2v3h3a2 2 0 0 1 0 4H5v3a2 2 0 0 0 2 2h3v-3a2 2 0 0 1 4 0v3h3a2 2 0 0 0 2-2v-3h3a2 2 0 0 0 0-4h-3V9a2 2 0 0 0-2-2z"/></svg>`)),
	"dice":     ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="4" y="4" width="16" height="16" rx="3"/><circle cx="9" cy="9" r="0.8"/><circle cx="15" cy="15" r="0.8"/><circle cx="15" cy="9" r="0.8"/><circle cx="9" cy="15" r="0.8"/></svg>`)),
	"key":      ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="8" cy="15" r="4"/><path d="m10.8 12.2 8.7-8.7"/><path d="m16 7 3 3"/><path d="m18.5 4.5 1.5 1.5"/></svg>`)),
}

// builtinIcon returns the icon of a result the launcher makes up itself, or
// nil when the row should keep the letter avatar. File-backed entries never
// reach it: their bitmap wins in resultIcon.
func builtinIcon(entry model.Entry) *ui.SVG {
	if entry.SourceID == "utilities" {
		switch {
		case strings.HasPrefix(entry.ID, "calc:"):
			return builtinIcons["calc"]
		case strings.HasPrefix(entry.ID, "date:"):
			return builtinIcons["calendar"]
		case strings.HasPrefix(entry.ID, "days:"), strings.HasPrefix(entry.ID, "week:"):
			return builtinIcons["calendar"]
		case strings.HasPrefix(entry.ID, "time:"):
			return builtinIcons["clock"]
		case strings.HasPrefix(entry.ID, "timestamp:"):
			return builtinIcons["clock"]
		case strings.HasPrefix(entry.ID, "uuid:"):
			return builtinIcons["uuid"]
		case strings.HasPrefix(entry.ID, "rand:"):
			return builtinIcons["dice"]
		case strings.HasPrefix(entry.ID, "pass:"):
			return builtinIcons["key"]
		case strings.HasPrefix(entry.ID, "web:"), strings.HasPrefix(entry.ID, "url:"):
			return builtinIcons["globe"]
		}
	}
	switch entry.Kind {
	case model.Calculation:
		return builtinIcons["calc"]
	case model.URL:
		return builtinIcons["globe"]
	case model.Command:
		return builtinIcons["gear"]
	case model.Workspace:
		return builtinIcons["layers"]
	case model.Plugin:
		return builtinIcons["puzzle"]
	}
	return nil
}
