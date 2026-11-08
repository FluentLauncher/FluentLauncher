package search

import (
	"strings"
	"unicode"

	gopinyin "github.com/mozillazg/go-pinyin"
	"golang.org/x/text/unicode/norm"
)

// Normalize applies NFKC and lower case, as the C# launcher does.
func Normalize(text string) string {
	return strings.ToLower(norm.NFKC.String(text))
}

// Phonetics returns the normalized title, full pinyin, initials and acronym.
func Phonetics(text string) (title, py, initials, acronym string) {
	title = Normalize(text)
	if !HasHan(text) {
		// Most file names are Han-free: skip the per-rune pinyin pass and
		// its two slice allocations entirely.
		short := Normalize(buildAcronym(text, nil, false))
		if short != "" && short != title {
			acronym = short
		} else {
			acronym = title
		}
		return title, title, title, acronym
	}
	runes := []rune(text)
	full := make([]string, len(runes))
	initial := make([]string, len(runes))
	args := gopinyin.NewArgs()
	hasHan := false
	for i, r := range runes {
		if !IsHan(r) {
			full[i] = string(r)
			initial[i] = string(r)
			continue
		}
		hasHan = true
		args.Style = gopinyin.Normal
		full[i] = strings.Join(gopinyin.LazyPinyin(string(r), args), "")
		args.Style = gopinyin.FirstLetter
		initial[i] = strings.Join(gopinyin.LazyPinyin(string(r), args), "")
	}
	if hasHan {
		py = Normalize(strings.Join(full, ""))
		initials = Normalize(strings.Join(initial, ""))
		acronym = buildAcronym(text, initial, true)
	} else {
		py = title
		initials = title
		acronym = buildAcronym(text, nil, false)
	}
	short := Normalize(acronym)
	switch short {
	case "", title:
		acronym = title
	case initials:
		acronym = initials
	default:
		acronym = short
	}
	return title, py, initials, acronym
}

func buildAcronym(text string, initials []string, hasHan bool) string {
	var b strings.Builder
	runes := []rune(text)
	for i, r := range runes {
		if IsHan(r) {
			if hasHan && len(initials) == len(runes) {
				b.WriteString(initials[i])
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			continue
		}
		prev := rune(' ')
		if i > 0 {
			prev = runes[i-1]
		}
		if !unicode.IsLetter(prev) && !unicode.IsDigit(prev) || IsHan(prev) ||
			(unicode.IsUpper(r) && unicode.IsLower(prev)) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// HasHan reports whether text contains a Han character.
func HasHan(text string) bool {
	for _, r := range text {
		if IsHan(r) {
			return true
		}
	}
	return false
}

// IsHan matches the ranges the C# implementation uses.
func IsHan(r rune) bool {
	return r >= 0x3400 && r <= 0x4DBF || r >= 0x4E00 && r <= 0x9FFF
}
