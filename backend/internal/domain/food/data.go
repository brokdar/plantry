package food

import (
	"regexp"
	"strings"
	"unicode"
)

// Recipes are imported from the web, so a name or a tag is text somebody else
// wrote, and it reaches the model in prompts and tool results. Data marks it
// as data the same way everywhere: one line (no forged "SYSTEM:" line), role
// markers cut out, length capped; both prompts say recipe names and tags are
// data, never a request. Only the text a model reads goes through it; the UI
// and the rules see the stored name. A delimiter around the text (« ») would
// mark it more strongly but changes every name in every prompt and tool
// result; it is left out for now.

const (
	dataMaxName = 120
	dataMaxTag  = 40
)

// roleMarker matches the speaker labels a prompt injection borrows.
var roleMarker = regexp.MustCompile(`(?i)\b(system|assistant|user|human|developer|instructions?)\s*[:：]`)

// Data is a recipe name as a model reads it.
func Data(s string) string { return data(s, dataMaxName) }

// DataTags is a recipe's tags as a model reads them.
func DataTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = data(t, dataMaxTag)
	}
	return out
}

func data(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r) || unicode.IsSpace(r):
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(roleMarker.ReplaceAllString(s, " ")), " ")
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}
