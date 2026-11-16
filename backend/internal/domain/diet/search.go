package diet

import (
	"slices"
	"strings"
	"unicode"
)

// Fold lowercases s and folds umlauts and accents the way the classifier does
// ("Hähnchen" and "haehnchen" are the same word).
func Fold(s string) string { return fold.Replace(strings.ToLower(s)) }

// synonyms are groups of equivalent food words (en/de, folded), a small
// data table like the classifier's: a search for any word of a group also
// finds the others ("potato" finds "Kartoffelgratin").
var synonyms = [][]string{
	{"potato", "kartoffel", "erdapfel"},
	{"onion", "zwiebel"},
	{"rice", "reis"},
	{"chicken", "haehnchen", "huehnchen", "huhn", "poulet"},
	{"turkey", "pute", "truthahn"},
	{"beef", "rind"},
	{"pork", "schwein"},
	{"fish", "fisch"},
	{"salmon", "lachs"},
	{"bean", "bohne"},
	{"lentil", "linse"},
	{"pea", "erbse"},
	{"tomato", "tomate"},
	{"carrot", "karotte", "moehre"},
	{"cabbage", "kohl"},
	{"mushroom", "pilz", "champignon"},
	{"pepper", "paprika"},
	{"spinach", "spinat"},
	{"garlic", "knoblauch"},
	{"cheese", "kaese"},
	{"cream", "sahne"},
	{"milk", "milch"},
	{"noodle", "nudel", "pasta"},
	{"bread", "brot"},
	{"egg", "eier"},
	{"soup", "suppe"},
	{"salad", "salat"},
}

// pluralSuffixes are the endings a search word may carry beyond a synonym
// member ("eggs", "bohnen", "nudeln"); anything longer is another word
// ("peanut" is not a pea).
var pluralSuffixes = []string{"s", "n", "e", "en", "er", "es"}

// Alt is one alternative of a search word. Exact alternatives (the English
// member of a group) match a whole word, optionally plural ("pea", "peas",
// not "peanut" or "peach"; "egg", not "eggplant"). The others match the start
// of a word, which covers German compounds ("reis" finds "Reispfanne", not
// "Preiselbeeren").
type Alt struct {
	Word  string
	Exact bool
}

// SearchAlternatives returns the alternatives for one search word: the word
// itself plus the members of every synonym group it names (a member, or a
// member with a plural ending), so "bohnen" and "bean" both find the beans
// group. The first member of a group is its English word.
func SearchAlternatives(word string) []Alt {
	w := Fold(word)
	out := []Alt{{Word: w}}
	for _, g := range synonyms {
		hit := -1
		for i, m := range g {
			if namesMember(w, m) {
				hit = i
				break
			}
		}
		if hit < 0 {
			continue
		}
		out[0].Exact = out[0].Exact || hit == 0
		for i, m := range g {
			if m != w {
				out = append(out, Alt{Word: m, Exact: i == 0})
			}
		}
	}
	return out
}

func namesMember(w, m string) bool {
	rest, ok := strings.CutPrefix(w, m)
	return ok && (rest == "" || slices.Contains(pluralSuffixes, rest))
}

// MatchesWord reports whether the folded text matches alts (the result of
// SearchAlternatives, the typed word first): some word of the text must
// match one of them. A typed word of five or more letters outside the
// synonym table may also end a German compound ("suppe" finds "Linsensuppe").
func MatchesWord(folded string, alts []Alt) bool {
	if len(alts) == 1 && len(alts[0].Word) >= 5 && strings.Contains(folded, alts[0].Word) {
		return true
	}
	for _, w := range strings.FieldsFunc(folded, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		for _, a := range alts {
			if a.Exact {
				if namesMember(w, a.Word) {
					return true
				}
			} else if strings.HasPrefix(w, a.Word) {
				return true
			}
		}
	}
	return false
}
