package search

import (
	"context"
	"crypto/rand"
	"fmt"
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
	return q.Scope == ScopeAll || q.Scope == ScopeWeb
}

func (p UtilityProvider) Search(ctx context.Context, q Query, limit int) ([]model.Result, error) {
	if q.Text == "" {
		return nil, nil
	}
	cfg := p.Settings()
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
	if q.Scope == ScopeAll {
		if results := utilityTools(q.Text); results != nil {
			return results, nil
		}
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
	if cfg.EnableCalculator {
		if value, ok := Evaluate(strings.TrimSpace(strings.TrimPrefix(q.Text, "="))); ok {
			formatted := strconv.FormatFloat(value, 'G', 15, 64)
			return []model.Result{{Entry: model.Entry{
				ID:       "calc:" + q.Text,
				Title:    formatted,
				Subtitle: locales.ResultCalcHint(q.Text),
				Kind:     model.Calculation,
				CopyText: formatted,
				SourceID: "utilities",
			}, Score: 20000}}, nil
		}
	}
	return nil, nil
}

func escapeQuery(text string) string {
	return strings.ReplaceAll(url.QueryEscape(text), "+", "%20")
}

// utilityTools answers the instant tools: today's date, the current time,
// Unix timestamps and a fresh UUID. Like the calculator they carry their
// value in CopyText, so Enter copies it.
func utilityTools(text string) []model.Result {
	lower := strings.ToLower(strings.TrimSpace(text))
	now := time.Now()
	switch {
	case matchesTool(lower, "date", "today", "日期", "今天"):
		value := now.Format("2006-01-02")
		return toolResult("date:", value, locales.ResultDateHint())
	case matchesTool(lower, "time", "now", "时间", "现在"):
		value := now.Format("15:04:05")
		return toolResult("time:", value, locales.ResultTimeHint())
	case matchesTool(lower, "timestamp", "unix", "时间戳"):
		sec := strconv.FormatInt(now.Unix(), 10)
		ms := strconv.FormatInt(now.UnixMilli(), 10)
		return []model.Result{
			{Entry: model.Entry{ID: "timestamp:s", Title: sec, Subtitle: locales.ResultTimestampSec(),
				Kind: model.Calculation, CopyText: sec, SourceID: "utilities"}, Score: 20000},
			{Entry: model.Entry{ID: "timestamp:ms", Title: ms, Subtitle: locales.ResultTimestampMs(),
				Kind: model.Calculation, CopyText: ms, SourceID: "utilities"}, Score: 19000},
		}
	case matchesTool(lower, "uuid", "guid"):
		if value := newUUID(); value != "" {
			return toolResult("uuid:", value, locales.ResultUUIDHint())
		}
	}
	return nil
}

func matchesTool(lower string, keywords ...string) bool {
	for _, keyword := range keywords {
		if lower == keyword || strings.HasPrefix(lower, keyword+" ") {
			return true
		}
	}
	return false
}

func toolResult(prefix, value, subtitle string) []model.Result {
	return []model.Result{{Entry: model.Entry{
		ID:       prefix + value,
		Title:    value,
		Subtitle: subtitle,
		Kind:     model.Calculation,
		CopyText: value,
		SourceID: "utilities",
	}, Score: 20000}}
}

// newUUID returns a random version-4 UUID in its canonical form.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
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
