package search

import (
	"context"
	"math"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/locales"
)

// UtilityProvider supplies URL/path opening and the calculator.
type UtilityProvider struct {
	Settings Settings
}

func (p UtilityProvider) ID() string { return "utilities" }

func (p UtilityProvider) Supports(q Query) bool {
	return q.Scope == ScopeAll || q.Scope == ScopeWeb || q.Scope == ScopeTools
}

func (p UtilityProvider) Search(ctx context.Context, q Query, limit int) ([]model.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, nil
	}
	cfg := p.Settings()
	q = RouteTools(q, cfg)
	if q.Scope == ScopeTools {
		results, err := toolsSearch(q, cfg)
		return results[:min(len(results), limit)], err
	}
	if q.Text == "" || ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if q.Scope == ScopeWeb {
		target := strings.ReplaceAll(cfg.WebSearchTemplate, "{query}", escapeQuery(q.Text))
		return []model.Result{{Entry: model.Entry{
			ID:           "web:" + q.Text,
			Title:        locales.ResultSearchWeb(q.Text),
			Subtitle:     target,
			LaunchTarget: target,
			Kind:         model.URL,
			SourceID:     "utilities",
		}, Score: 20000}}, nil
	}
	if u, err := url.Parse(q.Text); err == nil && u.IsAbs() &&
		(u.Scheme == "http" || u.Scheme == "https") {
		return []model.Result{{Entry: model.Entry{
			ID:           "url:" + u.String(),
			Title:        q.Text,
			Subtitle:     locales.ResultOpenUrl(),
			LaunchTarget: u.String(),
			Kind:         model.URL,
			SourceID:     "utilities",
		}, Score: 20000}}, nil
	}
	if filepath.IsAbs(q.Text) && !strings.HasPrefix(q.Text, `\\`) {
		return []model.Result{{Entry: model.Entry{
			ID:           "path:" + q.Text,
			Title:        q.Text,
			Subtitle:     locales.ResultOpenLocalPath(),
			LaunchTarget: q.Text,
			Kind:         model.File,
			SourceID:     "utilities",
		}, Score: 20000}}, nil
	}
	if q.Scope == ScopeAll {
		return automaticTools(q.Text, cfg), nil
	}
	return nil, nil
}

func escapeQuery(text string) string {
	return strings.ReplaceAll(url.QueryEscape(text), "+", "%20")
}

// utilityTools answers the instant tools: dates and date math, the time,
// Unix timestamps, UUIDs, the ISO week, day counts and random values. Like
// the calculator they carry their value in CopyText, so Enter copies it;
// most answer with one row per format (instant.go).
func utilityTools(text string) []model.Result {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)
	now := time.Now()
	switch {
	case matchesTool(lower, "date", "today", "日期", "今天"):
		return dateTool(trimmed, now)
	case matchesTool(lower, "time", "now", "时间", "现在"):
		// "time 1794652200" converts as the timestamp tool does.
		if arg := argument(trimmed); arg != "" {
			return timestampTool(arg, now)
		}
		return timeResults(now)
	case matchesTool(lower, "timestamp", "unix", "时间戳"):
		return timestampTool(argument(trimmed), now)
	case matchesTool(lower, "uuid", "guid"):
		return uuidTool(argument(lower))
	case matchesTool(lower, "week", "周数"):
		return weekTool(argument(trimmed), now)
	case matchesTool(lower, "days", "天数"):
		return daysTool(argument(lower), now)
	case matchesTool(lower, "rand", "随机数"):
		return randTool(argument(lower))
	case matchesTool(lower, "pass", "password", "密码"):
		return passTool(argument(lower))
	}
	return nil
}

// argument is what follows the tool's keyword.
func argument(text string) string {
	if i := strings.IndexAny(text, " \t"); i >= 0 {
		return strings.TrimSpace(text[i:])
	}
	return ""
}

func matchesTool(lower string, keywords ...string) bool {
	for _, keyword := range keywords {
		if lower == keyword || strings.HasPrefix(lower, keyword+" ") {
			return true
		}
	}
	return false
}

// Evaluate parses only numbers and arithmetic; it never evaluates scripts,
// method calls or system commands.
func Evaluate(text string) (float64, bool) {
	if len(text) == 0 || len(text) > 512 ||
		!strings.ContainsAny(text, "+-*/%^()") {
		return 0, false
	}
	p := &parser{text: text}
	value, ok := p.expression()
	if !ok {
		return 0, false
	}
	p.space()
	if p.pos != len(text) || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

type parser struct {
	text  string
	pos   int
	depth int
}

func (p *parser) space() {
	for p.pos < len(p.text) && (p.text[p.pos] == ' ' || p.text[p.pos] == '\t' ||
		p.text[p.pos] == '\r' || p.text[p.pos] == '\n') {
		p.pos++
	}
}

func (p *parser) take(ch byte) bool {
	p.space()
	if p.pos >= len(p.text) || p.text[p.pos] != ch {
		return false
	}
	p.pos++
	return true
}

func (p *parser) expression() (float64, bool) {
	p.depth++
	if p.depth > 32 {
		return 0, false
	}
	defer func() { p.depth-- }()
	value, ok := p.term()
	if !ok {
		return 0, false
	}
	for {
		if p.take('+') {
			right, ok := p.term()
			if !ok {
				return 0, false
			}
			value += right
		} else if p.take('-') {
			right, ok := p.term()
			if !ok {
				return 0, false
			}
			value -= right
		} else {
			return value, true
		}
	}
}

func (p *parser) term() (float64, bool) {
	value, ok := p.power()
	if !ok {
		return 0, false
	}
	for {
		if p.take('*') {
			right, ok := p.power()
			if !ok {
				return 0, false
			}
			value *= right
		} else if p.take('/') {
			right, ok := p.power()
			if !ok {
				return 0, false
			}
			value /= right
		} else if p.take('%') {
			right, ok := p.power()
			if !ok {
				return 0, false
			}
			value = math.Mod(value, right)
		} else {
			return value, true
		}
	}
}

func (p *parser) power() (float64, bool) {
	value, ok := p.atom()
	if !ok {
		return 0, false
	}
	if p.take('^') {
		p.depth++
		if p.depth > 32 {
			return 0, false
		}
		right, ok := p.power()
		p.depth--
		if !ok {
			return 0, false
		}
		value = math.Pow(value, right)
	}
	return value, true
}

func (p *parser) atom() (float64, bool) {
	if p.take('+') {
		p.depth++
		if p.depth > 32 {
			return 0, false
		}
		v, ok := p.atom()
		p.depth--
		return v, ok
	}
	if p.take('-') {
		p.depth++
		if p.depth > 32 {
			return 0, false
		}
		v, ok := p.atom()
		p.depth--
		return -v, ok
	}
	if p.take('(') {
		v, ok := p.expression()
		if !ok || !p.take(')') {
			return 0, false
		}
		return v, true
	}
	p.space()
	start := p.pos
	for p.pos < len(p.text) && ((p.text[p.pos] >= '0' && p.text[p.pos] <= '9') || p.text[p.pos] == '.') {
		p.pos++
	}
	if start == p.pos {
		return 0, false
	}
	v, err := strconv.ParseFloat(p.text[start:p.pos], 64)
	return v, err == nil
}
