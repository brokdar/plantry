package gates

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// words splits a folded string into letter/digit words.
func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// containsAny reports whether the folded text contains any keyword.
func containsAny(s string, kws []string) bool {
	for _, k := range kws {
		if strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// wordMatch reports whether a word matches a keyword: "=x" exactly, "-x" as a
// suffix, otherwise as a substring.
func wordMatch(w, kw string) bool {
	switch {
	case strings.HasPrefix(kw, "="):
		return w == kw[1:]
	case strings.HasPrefix(kw, "-"):
		return strings.HasSuffix(w, kw[1:])
	}
	return strings.Contains(w, kw)
}

func anyWord(ws []string, kws []string) bool {
	for _, w := range ws {
		for _, k := range kws {
			if wordMatch(w, k) {
				return true
			}
		}
	}
	return false
}

func appendUnique(dst []string, add ...string) []string {
	for _, a := range add {
		if !slices.Contains(dst, a) {
			dst = append(dst, a)
		}
	}
	return dst
}

func containsStr(list []string, s string) bool { return slices.Contains(list, s) }

var minutesRe = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*(min|minute|minuten|minutes|std|stunde|stunden|h|hour|hours|hr)\b`)

// maxMinutes is the longest duration a text states, in minutes.
func maxMinutes(s string) float64 {
	best := 0.0
	for _, m := range minutesRe.FindAllStringSubmatch(s, -1) {
		v := parseNum(m[1])
		switch m[2] {
		case "std", "stunde", "stunden", "h", "hour", "hours", "hr":
			v *= 60
		}
		best = max(best, v)
	}
	return best
}

func parseNum(s string) float64 {
	var whole, frac, div float64 = 0, 0, 1
	after := false
	for _, r := range s {
		switch {
		case r == '.' || r == ',':
			after = true
		case after:
			frac = frac*10 + float64(r-'0')
			div *= 10
		default:
			whole = whole*10 + float64(r-'0')
		}
	}
	return whole + frac/div
}
