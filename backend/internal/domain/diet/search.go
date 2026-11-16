package diet

import "strings"

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

// SearchAlternatives returns the folded alternatives for one search word:
// the word itself plus the members of every synonym group it belongs to (the
// word contains a member, or a member contains the word when it is at least
// four letters long, so "bohnen" and "bean" both find the beans group).
func SearchAlternatives(word string) []string {
	w := Fold(word)
	out := []string{w}
	for _, g := range synonyms {
		hit := false
		for _, m := range g {
			if strings.Contains(w, m) || (len(w) >= 4 && strings.Contains(m, w)) {
				hit = true
				break
			}
		}
		if hit {
			for _, m := range g {
				if m != w {
					out = append(out, m)
				}
			}
		}
	}
	return out
}
