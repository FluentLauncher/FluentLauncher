package search

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fluentlauncher/internal/core/model"
	"fluentlauncher/locales"
)

// Most instant tools answer with one row per format of their value, so that
// the arrows pick the format and Enter copies it. A row's ID is
// "<tool>:<format>:<key>": the format names the row, the key is what the
// rows share (the UUIDs, the date, the input).

// formatRow is one format of a tool's value: its name in the ID, the text,
// and the subtitle saying what format it is.
type formatRow struct {
	format, value, hint string
}

// formatResults lists the rows in order: the scores step down from the
// tools' usual one, so the first format stays the default selection.
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

// splitFormatID splits a row's ID into its tool, format and key.
func splitFormatID(id string) (tool, format, key string, ok bool) {
	parts := strings.SplitN(id, ":", 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

var (
	offsetRe = regexp.MustCompile(`^([+-]\d+)([dwmy])$`)
	rangeRe  = regexp.MustCompile(`^(-?\d+)\s*[-–~]\s*(-?\d+)$`)
	// boundsRe reads a random integer's range back from its row's ID.
	boundsRe = regexp.MustCompile(`^(-?\d+)~(-?\d+)$`)
)

// ---- UUID ----

const maxUUIDs = 20

// uuidTool answers "uuid" with one UUID and "uuid 5" with five, each in
// every format.
func uuidTool(arg string) []model.Result {
	count := 1
	if arg != "" {
		n, err := strconv.Atoi(arg)
		if err != nil || n < 1 || n > maxUUIDs {
			return nil
		}
		count = n
	}
	values := newUUIDs(count)
	if values == nil {
		return nil
	}
	return UUIDResults(values...)
}

func newUUIDs(count int) []string {
	values := make([]string, count)
	for i := range values {
		if values[i] = NewUUID(); values[i] == "" {
			return nil
		}
	}
	return values
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

// UUIDResults lists UUIDs in their formats, canonical first. Several UUIDs
// copy one per line; the row shows the first.
func UUIDResults(values ...string) []model.Result {
	plain := func(v string) string { return strings.ReplaceAll(v, "-", "") }
	formats := []struct {
		name, hint string
		apply      func(string) string
	}{
		{"std", locales.ResultUUIDHint(), func(v string) string { return v }},
		{"plain", locales.FormatNoHyphens(), plain},
		{"upper", locales.FormatUppercase(), strings.ToUpper},
		{"upper-plain", locales.FormatUpperNoHyphens(), func(v string) string { return strings.ToUpper(plain(v)) }},
		{"braces", locales.FormatBraces(), func(v string) string { return "{" + strings.ToUpper(v) + "}" }},
	}
	rows := make([]formatRow, len(formats))
	for i, f := range formats {
		out := make([]string, len(values))
		for j, v := range values {
			out[j] = f.apply(v)
		}
		rows[i] = formatRow{f.name, strings.Join(out, "\r\n"), f.hint}
	}
	results := formatResults("uuid", strings.Join(values, ","), rows)
	if len(values) > 1 {
		for i := range results {
			entry := &results[i].Entry
			entry.Title = strings.SplitN(entry.CopyText, "\r\n", 2)[0] + " …"
			entry.Subtitle = locales.FormatUUIDCount(len(values)) + " · " + entry.Subtitle
		}
	}
	return results
}

// ---- Random values ----

// randTool draws one integer in the asked range (inclusive), 1-100 by
// default. The range rides in the ID so a reroll keeps it.
func randTool(arg string) []model.Result {
	lo, hi := 1, 100
	if arg != "" {
		m := rangeRe.FindStringSubmatch(arg)
		if m == nil {
			return nil
		}
		lo, _ = strconv.Atoi(m[1])
		hi, _ = strconv.Atoi(m[2])
		if lo > hi {
			lo, hi = hi, lo
		}
	}
	value := randInRange(lo, hi)
	if value == "" {
		return nil
	}
	return formatResults("rand", fmt.Sprintf("%d~%d", lo, hi), []formatRow{{"int", value, locales.ResultRandHint()}})
}

// randInRange draws one integer in [lo, hi] in decimal, or "" when the
// system randomness fails.
func randInRange(lo, hi int) string {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(hi-lo+1)))
	if err != nil {
		return ""
	}
	return strconv.FormatInt(n.Int64()+int64(lo), 10)
}

// The character sets leave out look-alikes (0/O, 1/l/I) so a copied
// password reads back unambiguously; a PIN takes every digit.
var passCharsets = map[string]string{
	"mixed":  "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%^&*-_",
	"alnum":  "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789",
	"digits": "0123456789",
}

// passTool draws a password of the asked length, 16 by default, once with
// symbols, once letters and digits only, once digits only. The length rides
// in the ID so a reroll keeps it.
func passTool(arg string) []model.Result {
	length := 16
	if arg != "" {
		n, err := strconv.Atoi(arg)
		if err != nil || n < 4 || n > 64 {
			return nil
		}
		length = n
	}
	var rows []formatRow
	for _, kind := range []struct{ name, hint string }{
		{"mixed", locales.ResultPassHint()},
		{"alnum", locales.FormatPassAlnum()},
		{"digits", locales.FormatPassDigits()},
	} {
		value := randomString(passCharsets[kind.name], length)
		if value == "" {
			return nil
		}
		rows = append(rows, formatRow{kind.name, value, kind.hint})
	}
	return formatResults("pass", strconv.Itoa(length), rows)
}

// randomString draws length characters from charset without the modulo
// bias, or returns "" when the system randomness fails.
func randomString(charset string, length int) string {
	limit := 256 - 256%len(charset)
	out := make([]byte, 0, length)
	buf := make([]byte, length*2)
	for len(out) < length {
		if _, err := rand.Read(buf); err != nil {
			return ""
		}
		for _, b := range buf {
			if int(b) < limit && len(out) < length {
				out = append(out, charset[int(b)%len(charset)])
			}
		}
	}
	return string(out)
}

// Reroll draws the random value of results[i] again and keeps what the user
// asked for: the range, the length and kind of password, how many UUIDs.
// The rows of a UUID reroll together, as they show one value; a password's
// rows are independent draws, so only the selected one changes. It reports
// whether any row changed.
func Reroll(results []model.Result, i int) bool {
	if i < 0 || i >= len(results) || results[i].Entry.SourceID != "utilities" {
		return false
	}
	tool, format, key, ok := splitFormatID(results[i].Entry.ID)
	if !ok {
		return false
	}
	set := func(entry *model.Entry, value string) {
		entry.Title, entry.CopyText = value, value
	}
	switch tool {
	case "uuid":
		values := newUUIDs(strings.Count(key, ",") + 1)
		if values == nil {
			return false
		}
		fresh := map[string]model.Entry{}
		for _, row := range UUIDResults(values...) {
			_, f, _, _ := splitFormatID(row.Entry.ID)
			fresh[f] = row.Entry
		}
		changed := false
		for j := range results {
			t, f, _, ok := splitFormatID(results[j].Entry.ID)
			if entry, known := fresh[f]; ok && known && t == "uuid" && results[j].Entry.SourceID == "utilities" {
				results[j].Entry.ID, results[j].Entry.Subtitle = entry.ID, entry.Subtitle
				set(&results[j].Entry, entry.CopyText)
				results[j].Entry.Title = entry.Title
				changed = true
			}
		}
		return changed
	case "rand":
		m := boundsRe.FindStringSubmatch(key)
		if m == nil {
			return false
		}
		lo, _ := strconv.Atoi(m[1])
		hi, _ := strconv.Atoi(m[2])
		value := randInRange(lo, hi)
		if value == "" {
			return false
		}
		set(&results[i].Entry, value)
		return true
	case "pass":
		length, err := strconv.Atoi(key)
		charset, known := passCharsets[format]
		if err != nil || !known || length < 4 || length > 64 {
			return false
		}
		value := randomString(charset, length)
		if value == "" {
			return false
		}
		set(&results[i].Entry, value)
		return true
	}
	return false
}

// ---- Dates and times ----

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// parseDate accepts an ISO, slashed or compact date, or today, tomorrow
// and yesterday by name; it rejects impossible dates like 2026-02-31.
func parseDate(text string, now time.Time) *time.Time {
	today := midnight(now)
	switch strings.ToLower(text) {
	case "today", "今天":
		return &today
	case "tomorrow", "明天":
		t := today.AddDate(0, 0, 1)
		return &t
	case "yesterday", "昨天":
		t := today.AddDate(0, 0, -1)
		return &t
	}
	for _, layout := range []string{"2006-01-02", "2006/01/02", "20060102"} {
		if t, err := time.ParseInLocation(layout, text, now.Location()); err == nil {
			return &t
		}
	}
	return nil
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

// dateTool answers "date" with today, and "date [<date>] <+3d|-2w|+1m|+1y>…"
// with the shifted date, from today when the date is left out.
func dateTool(trimmed string, now time.Time) []model.Result {
	fields := strings.Fields(trimmed)[1:]
	if len(fields) == 0 {
		return dateResults(midnight(now), locales.ResultDateHint())
	}
	base := midnight(now)
	if !offsetRe.MatchString(fields[0]) {
		parsed := parseDate(fields[0], now)
		if parsed == nil {
			return nil
		}
		base, fields = *parsed, fields[1:]
	}
	for _, field := range fields {
		m := offsetRe.FindStringSubmatch(field)
		if m == nil {
			return nil
		}
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "d":
			base = base.AddDate(0, 0, n)
		case "w":
			base = base.AddDate(0, 0, n*7)
		case "m":
			base = addMonthsClamped(base, n)
		case "y":
			base = addMonthsClamped(base, n*12)
		}
	}
	return dateResults(base, locales.ResultDateCalcHint())
}

var chineseWeekdays = [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}

// dateResults lists a date in its formats; hint names the first row, today
// or the result of date math.
func dateResults(t time.Time, hint string) []model.Result {
	iso := t.Format("2006-01-02")
	chinese := fmt.Sprintf("%d年%d月%d日", t.Year(), t.Month(), t.Day())
	return formatResults("date", iso, []formatRow{
		{"iso", iso, hint},
		{"slash", t.Format("2006/01/02"), locales.FormatSlashes()},
		{"compact", t.Format("20060102"), locales.FormatCompact()},
		{"zh", chinese, locales.FormatChinese()},
		{"zh-week", chinese + " " + chineseWeekdays[t.Weekday()], locales.FormatChineseWeekday()},
		{"en", t.Format("Monday, January 2, 2006"), locales.FormatEnglish()},
	})
}

func timeResults(now time.Time) []model.Result {
	return formatResults("time", now.Format("15:04:05"), []formatRow{
		{"hms", now.Format("15:04:05"), locales.ResultTimeHint()},
		{"datetime", now.Format("2006-01-02 15:04:05"), locales.FormatWithDate()},
		{"hm", now.Format("15:04"), locales.FormatNoSeconds()},
		{"12h", now.Format("3:04:05 PM"), locales.Format12Hour()},
		{"ms", now.Format("2006-01-02 15:04:05.000"), locales.FormatMilliseconds()},
		{"iso", now.Format(time.RFC3339), locales.FormatISO8601()},
		{"utc", now.UTC().Format("2006-01-02T15:04:05Z"), locales.FormatUTC()},
	})
}

// zoneName is a time's UTC offset, as UTC+08:00.
func zoneName(t time.Time) string {
	_, seconds := t.Zone()
	sign := "+"
	if seconds < 0 {
		sign, seconds = "-", -seconds
	}
	return fmt.Sprintf("UTC%s%02d:%02d", sign, seconds/3600, seconds%3600/60)
}

// timestampTool answers the bare keyword with the current Unix time,
// a Unix time in seconds or milliseconds with the moment it names, and a
// date or date and time with its Unix time.
func timestampTool(arg string, now time.Time) []model.Result {
	stamps := func(t time.Time, key string) []model.Result {
		return formatResults("timestamp", key, []formatRow{
			{"s", strconv.FormatInt(t.Unix(), 10), locales.ResultTimestampSec()},
			{"ms", strconv.FormatInt(t.UnixMilli(), 10), locales.ResultTimestampMs()},
		})
	}
	if arg == "" {
		return stamps(now, strconv.FormatInt(now.Unix(), 10))
	}
	if n, err := strconv.ParseInt(arg, 10, 64); err == nil {
		// Thirteen digits are milliseconds; seconds reach that far only
		// after the year 5000.
		milliseconds := n >= 1e11 || n <= -1e11
		moment, layout := time.Unix(n, 0), "2006-01-02 15:04:05"
		other := formatRow{"to-ms", strconv.FormatInt(n*1000, 10), locales.ResultTimestampMs()}
		if milliseconds {
			moment, layout = time.UnixMilli(n), "2006-01-02 15:04:05.000"
			other = formatRow{"to-s", strconv.FormatInt(n/1000, 10), locales.ResultTimestampSec()}
		}
		local := moment.In(now.Location())
		return formatResults("timestamp", arg, []formatRow{
			{"local", local.Format(layout), locales.FormatLocalTime(zoneName(local))},
			{"utc", moment.UTC().Format(layout), locales.FormatUTC()},
			{"iso", local.Format(time.RFC3339Nano), locales.FormatISO8601()},
			other,
		})
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02",
		"2006/01/02 15:04:05", "2006/01/02 15:04", "2006/01/02"} {
		if t, err := time.ParseInLocation(layout, arg, now.Location()); err == nil {
			return stamps(t, arg)
		}
	}
	if t, err := time.Parse(time.RFC3339Nano, arg); err == nil {
		return stamps(t, arg)
	}
	return nil
}

// weekTool answers the ISO week of today, or of the given date.
func weekTool(arg string, now time.Time) []model.Result {
	day, hint := midnight(now), locales.ResultWeekHint()
	if arg != "" {
		parsed := parseDate(arg, now)
		if parsed == nil {
			return nil
		}
		day, hint = *parsed, locales.FormatWeekOfDate()
	}
	year, week := day.ISOWeek()
	monday := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
	iso := fmt.Sprintf("%d-W%02d", year, week)
	return formatResults("week", iso, []formatRow{
		{"iso", iso, hint},
		{"number", strconv.Itoa(week), locales.FormatWeekNumber()},
		{"range", monday.Format("2006-01-02") + " ~ " + monday.AddDate(0, 0, 6).Format("2006-01-02"), locales.FormatWeekRange()},
	})
}

// daysTool says how many days a date is from today, future positive, or
// how many lie between two dates.
func daysTool(arg string, now time.Time) []model.Result {
	fields := strings.Fields(arg)
	if len(fields) == 0 || len(fields) > 2 {
		return nil
	}
	from, hint := midnight(now), locales.ResultDaysHint()
	to := parseDate(fields[0], now)
	if to == nil {
		return nil
	}
	if len(fields) == 2 {
		from, hint = *to, locales.FormatDaysBetween()
		if to = parseDate(fields[1], now); to == nil {
			return nil
		}
	}
	// Calendar days, whatever daylight saving does to the hours between.
	utc := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }
	days := int(utc(*to).Sub(utc(from)).Hours() / 24)
	rows := []formatRow{{"count", strconv.Itoa(days), hint}}
	if span := max(days, -days); span >= 7 {
		rows = append(rows, formatRow{"weeks", locales.FormatWeeksAndDays(span/7, span%7), locales.FormatInWeeks()})
	}
	return formatResults("days", strings.Join(fields, ".."), rows)
}
