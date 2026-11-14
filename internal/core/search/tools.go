package search

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/internal/core/settings"
	"fluentlauncher/locales"
)

// RouteTools resolves explicit intent once, before providers are dispatched.
// Prefixes consume their literal text ("=1+2"), words require a boundary.
func RouteTools(q Query, cfg settings.LauncherSettings) Query {
	if q.Scope == ScopeTools && q.ToolID == "" {
		for _, option := range cfg.Tools {
			for _, word := range append([]string{option.ID}, option.Keywords...) {
				if arg, ok := toolArgument(q.Text, word); ok {
					return Query{Text: arg, Scope: ScopeTools, ToolID: option.ID}
				}
			}
		}
		return q
	}
	if q.Scope != ScopeAll {
		return q
	}
	for _, option := range cfg.Tools {
		if !cfg.Tool(option.ID).Enabled {
			continue
		}
		if option.Prefix != "" {
			if len(q.Text) >= len(option.Prefix) && strings.EqualFold(q.Text[:len(option.Prefix)], option.Prefix) {
				return Query{Text: strings.TrimSpace(q.Text[len(option.Prefix):]), Scope: ScopeTools, ToolID: option.ID}
			}
			continue
		}
		if option.GlobalQuery {
			for _, word := range option.Keywords {
				if arg, ok := toolArgument(q.Text, word); ok {
					return Query{Text: arg, Scope: ScopeTools, ToolID: option.ID}
				}
			}
		}
	}
	return q
}

func toolArgument(text, word string) (string, bool) {
	if len(text) < len(word) || !strings.EqualFold(text[:len(word)], word) {
		return "", false
	}
	rest := text[len(word):]
	if rest != "" && !strings.HasPrefix(rest, " ") && !strings.HasPrefix(rest, "\t") {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

func ToolName(id string) string {
	names := map[string]string{
		"calc": locales.ToolCalc(), "uuid": locales.ToolUUID(), "date": locales.ToolDate(),
		"time": locales.ToolTime(), "timestamp": locales.ToolTimestamp(), "week": locales.ToolWeek(),
		"days": locales.ToolDays(), "rand": locales.ToolRand(), "pass": locales.ToolPass(),
	}
	return names[id]
}

func toolsSearch(q Query, cfg settings.LauncherSettings) ([]model.Result, error) {
	if q.ToolID == "" {
		if strings.TrimSpace(q.Text) != "" {
			return nil, fmt.Errorf("%s: %s", locales.ToolsUnknown(), q.Text)
		}
		var results []model.Result
		for _, option := range cfg.Tools {
			if !cfg.Tool(option.ID).Enabled {
				continue
			}
			results = append(results, model.Result{Entry: model.Entry{
				ID: "tool-query:" + option.ID, Title: ToolName(option.ID),
				Subtitle: "tool " + option.Keywords[0], Kind: model.Command, SourceID: "utilities",
				LaunchTarget: "tool " + option.Keywords[0] + " ",
			}, Score: 20000})
		}
		return results, nil
	}
	if !cfg.Tool(q.ToolID).Enabled {
		return nil, fmt.Errorf("%s: %s", locales.ToolsDisabled(), ToolName(q.ToolID))
	}
	return runTool(q.ToolID, q.Text), nil
}

func runTool(id, arg string) []model.Result {
	switch id {
	case "calc":
		if value, ok := Evaluate(arg); ok {
			formatted := strconv.FormatFloat(value, 'G', 15, 64)
			return []model.Result{{Entry: model.Entry{ID: "calc:" + arg, Title: formatted,
				Subtitle: locales.ResultCalcHint(arg), Kind: model.Calculation,
				CopyText: formatted, SourceID: "utilities"}, Score: 20000}}
		}
	default:
		// Existing handlers receive canonical names, never user aliases.
		return utilityTools(strings.TrimSpace(id + " " + arg))
	}
	return nil
}

// Automatic recognition is opt-in and never probes tools with a fixed prefix.
func automaticTools(text string, cfg settings.LauncherSettings) []model.Result {
	// Only the calculator recognizes its input without a keyword.
	option := cfg.Tool("calc")
	if !option.Enabled || !option.GlobalQuery || option.Prefix != "" ||
		strings.IndexFunc(text, unicode.IsDigit) < 0 { // cheap to reject before parsing
		return nil
	}
	return runTool("calc", text)
}
