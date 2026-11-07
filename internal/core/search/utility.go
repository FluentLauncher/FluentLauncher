package search

import (
	"context"
	"math"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"fluentlauncher/internal/core/model"
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
			Title:        "搜索网页：" + q.Text,
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
			Subtitle:     "打开网址",
			LaunchTarget: u.String(),
			Kind:         model.URL,
			SourceID:     "utilities",
		}, Score: 20000}}, nil
	}
	if filepath.IsAbs(q.Text) && !strings.HasPrefix(q.Text, `\\`) {
		return []model.Result{{Entry: model.Entry{
			ID:           "path:" + q.Text,
			Title:        q.Text,
			Subtitle:     "打开本地路径",
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
				Subtitle: q.Text + " · Enter 复制结果",
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
