// Package finds is the find pipeline (FIND-1..17): search the enabled sites,
// fetch three versions, rank, cross-check, adapt, match, gate, and store the
// find in its FIND-4 state.
package finds

import (
	"strings"
	"unicode/utf8"
)

// maxFieldRunes bounds each query field.
const maxFieldRunes = 80

// Query is everything a web search may say about a find (FIND-5, NFR-5): the
// dish, its cuisine, the diet and the language, never anything about the
// household. Keep it that way: a new field here is a privacy decision.
type Query struct {
	Dish     string
	Cuisine  string
	Diet     string
	Language string
}

// Text is the search request sent to the provider. Empty fields are left out.
func (q Query) Text() string {
	parts := []string{"recipe: " + clean(q.Dish)}
	for _, f := range []struct{ label, value string }{
		{"cuisine", q.Cuisine}, {"diet", q.Diet}, {"language", q.Language},
	} {
		if v := clean(f.value); v != "" {
			parts = append(parts, f.label+": "+v)
		}
	}
	return strings.Join(parts, "; ")
}

// clean collapses whitespace and cuts the value to maxFieldRunes.
func clean(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > maxFieldRunes {
		s = string([]rune(s)[:maxFieldRunes])
	}
	return s
}
