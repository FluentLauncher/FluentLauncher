package search

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math"
	"math/big"
	"net/url"
	"path/filepath"
	"regexp"
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

// utilityTools answers the instant tools: dates and date math, the current
// time, Unix timestamps, a fresh UUID, unit conversions, text encodings,
// random values and colors. Like the calculator they carry their value in
// CopyText, so Enter copies it.
func utilityTools(text string) []model.Result {
	lower := strings.ToLower(strings.TrimSpace(text))
	trimmed := strings.TrimSpace(text)
	now := time.Now()
	switch {
	case matchesTool(lower, "date", "today", "日期", "今天"):
		return dateTool(trimmed, now)
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
		if value := NewUUID(); value != "" {
			return toolResult("uuid:", value, locales.ResultUUIDHint())
		}
	case matchesTool(lower, "week", "周数"):
		year, week := now.ISOWeek()
		value := fmt.Sprintf("%d-W%02d", year, week)
		return toolResult("week:", value, locales.ResultWeekHint())
	case matchesTool(lower, "days", "天数"):
		return daysTool(lower, now)
	case matchesTool(lower, "rand", "随机数"):
		return randTool(lower)
	case matchesTool(lower, "pass", "password", "密码"):
		return passTool(lower)
	}
	if result := convertTool(trimmed); result != nil {
		return result
	}
	if result := encodeTool(trimmed); result != nil {
		return result
	}
	if result := colorTool(trimmed); result != nil {
		return result
	}
	return nil
}

// dateTool answers the bare keyword with today, and "date <iso> [+3d|-2w|+1m|+1y]"
// with the shifted date.
func dateTool(trimmed string, now time.Time) []model.Result {
	fields := strings.Fields(trimmed)
	if len(fields) == 1 {
		return toolResult("date:", now.Format("2006-01-02"), locales.ResultDateHint())
	}
	base := parseDate(fields[1], now)
	if base == nil {
		return nil
	}
	if len(fields) >= 3 {
		if m := offsetRe.FindStringSubmatch(fields[2]); m != nil {
			n, _ := strconv.Atoi(m[1])
			switch m[2] {
			case "d":
				*base = base.AddDate(0, 0, n)
			case "w":
				*base = base.AddDate(0, 0, n*7)
			case "m":
				*base = addMonthsClamped(*base, n)
			case "y":
				*base = addMonthsClamped(*base, n*12)
			}
		}
	}
	return toolResult("date:", base.Format("2006-01-02"), locales.ResultDateCalcHint())
}

// addMonthsClamped shifts by whole months, pinning to the last day of a
// shorter target month: Jan 31 + 1m is Feb 28 (or 29), not Mar 3.
func addMonthsClamped(t time.Time, months int) time.Time {
	year, month, day := t.Date()
	target := time.Date(year, month+time.Month(months), 1, 0, 0, 0, 0, t.Location())
	if last := time.Date(target.Year(), target.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day(); day > last {
		day = last
	}
	return time.Date(target.Year(), target.Month(), day, 0, 0, 0, 0, t.Location())
}

// parseDate accepts an ISO date, or "today"/"今天" for the current one; it
// rejects calendar-impossible dates like 2026-02-31.
func parseDate(text string, now time.Time) *time.Time {
	lower := strings.ToLower(text)
	if lower == "today" || text == "今天" {
		t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return &t
	}
	t, err := time.ParseInLocation("2006-01-02", text, now.Location())
	if err != nil {
		return nil
	}
	return &t
}

// daysTool says how many days the given date is from today, future positive.
func daysTool(lower string, now time.Time) []model.Result {
	fields := strings.Fields(lower)
	if len(fields) < 2 {
		return nil
	}
	target := parseDate(fields[1], now)
	if target == nil {
		return nil
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	value := strconv.Itoa(int(target.Sub(today).Hours() / 24))
	return toolResult("days:", value, locales.ResultDaysHint())
}

// randTool draws one integer in the asked range (inclusive), 1-100 by
// default. The range rides in the entry ID so the action panel can reroll
// within the same bounds.
func randTool(lower string) []model.Result {
	lo, hi := 1, 100
	fields := strings.Fields(lower)
	if len(fields) >= 2 {
		m := rangeRe.FindStringSubmatch(fields[1])
		if m == nil {
			return nil
		}
		lo, _ = strconv.Atoi(m[1])
		hi, _ = strconv.Atoi(m[2])
	}
	value := RandInRange(lo, hi)
	if value == "" {
		return nil
	}
	return []model.Result{{Entry: model.Entry{
		ID:       fmt.Sprintf("rand:%d-%d:%s", lo, hi, value),
		Title:    value,
		Subtitle: locales.ResultRandHint(),
		Kind:     model.Calculation,
		CopyText: value,
		SourceID: "utilities",
	}, Score: 20000}}
}

// RandInRange draws one integer in [lo, hi] (inclusive, swapped if reversed)
// and returns it in decimal, or "" when the system randomness fails.
func RandInRange(lo, hi int) string {
	if lo > hi {
		lo, hi = hi, lo
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(hi-lo+1)))
	if err != nil {
		return ""
	}
	return strconv.FormatInt(n.Int64()+int64(lo), 10)
}

// passCharset leaves out look-alikes (0/O, 1/l/I) so a copied password reads
// back unambiguously.
const passCharset = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%^&*-_"

// passTool generates a random password of the asked length, 16 by default.
// The length rides in the entry ID so the action panel can reroll it.
func passTool(lower string) []model.Result {
	length := 16
	fields := strings.Fields(lower)
	if len(fields) >= 2 {
		n, err := strconv.Atoi(fields[1])
		if err != nil || n < 4 || n > 64 {
			return nil
		}
		length = n
	}
	value := RandomPassword(length)
	if value == "" {
		return nil
	}
	return []model.Result{{Entry: model.Entry{
		ID:       fmt.Sprintf("pass:%d:%s", length, value),
		Title:    value,
		Subtitle: locales.ResultPassHint(),
		Kind:     model.Calculation,
		CopyText: value,
		SourceID: "utilities",
	}, Score: 20000}}
}

// RandomPassword draws length characters from passCharset, or "" when the
// system randomness fails.
func RandomPassword(length int) string {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	out := make([]byte, length)
	for i, b := range buf {
		out[i] = passCharset[int(b)%len(passCharset)]
	}
	return string(out)
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

// NewUUID returns a random version-4 UUID in its canonical form, or "" when
// the system randomness fails.
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

var (
	offsetRe   = regexp.MustCompile(`^([+-]\d+)([dwmy])$`)
	rangeRe    = regexp.MustCompile(`^(-?\d+)\s*[-–]\s*(-?\d+)$`)
	convertRe  = regexp.MustCompile(`^([+-]?\d*\.?\d+)\s*([a-zA-Z°\x{4e00}-\x{9fff}]+)\s+([a-zA-Z°\x{4e00}-\x{9fff}]+)$`)
	hexColorRe = regexp.MustCompile(`^#([0-9a-fA-F]{6}|[0-9a-fA-F]{3})$`)
	rgbColorRe = regexp.MustCompile(`(?i)^rgb\s*\(?\s*(\d{1,3})\s*[,，\s]\s*(\d{1,3})\s*[,，\s]\s*(\d{1,3})\s*\)?$`)
)

// unitDef names one unit of a dimension; factor converts to the dimension's
// base unit (kilograms, meters, liters, bytes). Temperature ignores factor.
type unitDef struct {
	dim    string
	factor float64
}

var unitTable = map[string]unitDef{
	"mg": {"mass", 1e-6}, "g": {"mass", 1e-3}, "kg": {"mass", 1}, "t": {"mass", 1e3},
	"oz": {"mass", 0.028349523125}, "lb": {"mass", 0.45359237},
	"mm": {"length", 1e-3}, "cm": {"length", 1e-2}, "m": {"length", 1}, "km": {"length", 1e3},
	"in": {"length", 0.0254}, "ft": {"length", 0.3048}, "yd": {"length", 0.9144}, "mi": {"length", 1609.344},
	"ml": {"volume", 1e-3}, "l": {"volume", 1}, "gal": {"volume", 3.785411784},
	"b": {"data", 1}, "kb": {"data", 1 << 10}, "mb": {"data", 1 << 20}, "gb": {"data", 1 << 30}, "tb": {"data", 1 << 40},
	"c": {"temp", 0}, "f": {"temp", 0}, "k": {"temp", 0},
}

var unitAliases = map[string]string{
	"毫克": "mg", "克": "g", "公斤": "kg", "千克": "kg", "吨": "t", "盎司": "oz", "磅": "lb",
	"毫米": "mm", "厘米": "cm", "公分": "cm", "米": "m", "公尺": "m", "公里": "km", "千米": "km",
	"英寸": "in", "吋": "in", "英尺": "ft", "呎": "ft", "码": "yd", "英里": "mi", "哩": "mi",
	"毫升": "ml", "升": "l", "公升": "l", "加仑": "gal",
	"摄氏": "c", "摄氏度": "c", "华氏": "f", "华氏度": "f", "开尔文": "k",
}

func canonicalUnit(token string) (unitDef, string, bool) {
	if alias, ok := unitAliases[token]; ok {
		token = alias
	}
	key := strings.ToLower(strings.TrimSuffix(token, "°"))
	def, ok := unitTable[key]
	return def, key, ok
}

// convertTool answers "<value><unit> <unit>" such as "10kg lb" or "32f c".
func convertTool(trimmed string) []model.Result {
	m := convertRe.FindStringSubmatch(trimmed)
	if m == nil {
		return nil
	}
	value, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	src, srcKey, ok := canonicalUnit(m[2])
	if !ok {
		return nil
	}
	dst, dstKey, ok := canonicalUnit(m[3])
	if !ok || src.dim != dst.dim {
		return nil
	}
	var out float64
	if src.dim == "temp" {
		out = fromCelsius(toCelsius(value, srcKey), dstKey)
	} else {
		out = value * src.factor / dst.factor
	}
	shown := strconv.FormatFloat(out, 'G', 6, 64)
	return []model.Result{{Entry: model.Entry{
		ID:       "convert:" + trimmed,
		Title:    shown + " " + dstKey,
		Subtitle: locales.ResultConvertHint(m[1]+" "+srcKey, dstKey),
		Kind:     model.Calculation,
		CopyText: shown,
		SourceID: "utilities",
	}, Score: 20000}}
}

func toCelsius(v float64, unit string) float64 {
	switch unit {
	case "f":
		return (v - 32) * 5 / 9
	case "k":
		return v - 273.15
	}
	return v
}

func fromCelsius(v float64, unit string) float64 {
	switch unit {
	case "f":
		return v*9/5 + 32
	case "k":
		return v + 273.15
	}
	return v
}

// encodeTool answers "b64 <text>", "unb64 <text>", "urlencode <text>" and
// "urldecode <text>".
func encodeTool(trimmed string) []model.Result {
	fields := strings.SplitN(trimmed, " ", 2)
	if len(fields) != 2 {
		return nil
	}
	keyword := strings.ToLower(fields[0])
	arg := strings.TrimSpace(fields[1])
	if arg == "" {
		return nil
	}
	switch keyword {
	case "b64", "base64":
		return toolResult("b64:", base64.StdEncoding.EncodeToString([]byte(arg)), locales.ResultB64EncodeHint())
	case "unb64":
		decoded, err := base64.StdEncoding.DecodeString(arg)
		if err != nil {
			return nil
		}
		return toolResult("b64d:", string(decoded), locales.ResultB64DecodeHint())
	case "urlencode":
		return toolResult("urlenc:", escapeQuery(arg), locales.ResultUrlEncodeHint())
	case "urldecode":
		decoded, err := url.QueryUnescape(arg)
		if err != nil {
			return nil
		}
		return toolResult("urldec:", decoded, locales.ResultUrlDecodeHint())
	}
	return nil
}

// colorTool answers "#3366AA", "#36A" and "rgb 51, 102, 170"; Enter copies
// the six-digit hex form.
func colorTool(trimmed string) []model.Result {
	var r, g, b int
	if m := hexColorRe.FindStringSubmatch(trimmed); m != nil {
		hex := m[1]
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return nil
		}
		r, g, b = int(v>>16&0xff), int(v>>8&0xff), int(v&0xff)
	} else if m := rgbColorRe.FindStringSubmatch(trimmed); m != nil {
		r, _ = strconv.Atoi(m[1])
		g, _ = strconv.Atoi(m[2])
		b, _ = strconv.Atoi(m[3])
		if r > 255 || g > 255 || b > 255 {
			return nil
		}
	} else {
		return nil
	}
	hexValue := fmt.Sprintf("#%02X%02X%02X", r, g, b)
	h, s, l := rgbToHSL(r, g, b)
	subtitle := locales.ResultColorHint(
		fmt.Sprintf("%d, %d, %d", r, g, b),
		fmt.Sprintf("%d°, %d%%, %d%%", h, s, l))
	return []model.Result{{Entry: model.Entry{
		ID:       "color:" + hexValue,
		Title:    hexValue,
		Subtitle: subtitle,
		Kind:     model.Calculation,
		CopyText: hexValue,
		SourceID: "utilities",
	}, Score: 20000}}
}

func rgbToHSL(r, g, b int) (int, int, int) {
	rf, gf, bf := float64(r)/255, float64(g)/255, float64(b)/255
	mx := math.Max(rf, math.Max(gf, bf))
	mn := math.Min(rf, math.Min(gf, bf))
	l := (mx + mn) / 2
	if mx == mn {
		return 0, 0, int(math.Round(l * 100))
	}
	d := mx - mn
	s := d / (1 - math.Abs(2*l-1))
	var h float64
	switch mx {
	case rf:
		h = math.Mod((gf-bf)/d, 6)
	case gf:
		h = (bf-rf)/d + 2
	default:
		h = (rf-gf)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return int(math.Round(h)), int(math.Round(s * 100)), int(math.Round(l * 100))
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
