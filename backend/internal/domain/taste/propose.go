package taste

import (
	"regexp"
	"slices"
	"strings"
)

// Proposal is one statement a free-text answer proposes (COLD-1): a taste
// item or, after "allergic", a health word. The user's tap stores it.
type Proposal struct {
	Kind     string   `json:"kind"` // taste or health
	Level    Level    `json:"level,omitempty"`
	Key      string   `json:"key,omitempty"`
	Polarity Polarity `json:"polarity,omitempty"`
	Word     string   `json:"word,omitempty"`
}

var (
	clauseSplit = regexp.MustCompile(`[,.;:!?\n]+|\b(?:and|but|und|aber|oder|or)\b`)
	wordSplit   = regexp.MustCompile(`[^\p{L}]+`)
)

// negative words turn a clause into "rather not"; health words into a health
// statement; filler words name nothing.
var (
	negative = []string{"no", "not", "dont", "don't", "hate", "hates", "avoid", "without", "kein", "keine", "keinen", "nicht", "ohne", "hasse", "vermeiden"}
	health   = []string{
		"allergic", "allergy", "celiac", "coeliac", "intolerant", "allergie", "unverträglich", "zöliakie",
	}
	filler = []string{"i", "we", "my", "our", "like", "likes", "love", "loves", "eat", "mostly", "really", "very", "much", "the", "a", "an", "to", "of", "is", "are", "food", "foods", "meals", "dishes", "dish", "ich", "wir", "mag", "mögen", "esse", "essen", "gerne", "gern", "liebe", "lieben", "sehr", "und", "der", "die", "das", "ein", "eine", "gerichte", "essen", "alles", "everything", "anything", "all", "kids", "kinder", "family", "familie", "are", "dont", "don't", "no", "not", "kein", "keine", "keinen", "nicht", "ohne", "hate", "hates", "avoid", "without", "hasse", "vermeiden", "have", "has", "am", "be", "with", "mit", "in", "on", "for", "für"}
)

// Propose reads a free-text answer about food ("we love thai and pasta,
// but no coriander; one of us is celiac") into taste statements, one per
// named item, without a model call. A clause with a negative word proposes
// "rather not"; any other names what the household loves. Each item once.
func Propose(text string) []Proposal {
	var out []Proposal
	seen := map[string]bool{}
	add := func(p Proposal) {
		id := p.Kind + "|" + string(p.Level) + "|" + p.Key + "|" + p.Word
		if !seen[id] {
			seen[id] = true
			out = append(out, p)
		}
	}
	for _, clause := range clauseSplit.Split(strings.ToLower(text), -1) {
		words := wordSplit.Split(clause, -1)
		words = slices.DeleteFunc(words, func(w string) bool { return w == "" })
		neg := slices.ContainsFunc(words, func(w string) bool { return slices.Contains(negative, w) })
		sick := slices.ContainsFunc(words, func(w string) bool { return slices.Contains(health, w) })
		if sick && (slices.Contains(words, "celiac") || slices.Contains(words, "coeliac") || slices.Contains(words, "zöliakie")) {
			add(Proposal{Kind: "health", Word: "gluten"})
			continue
		}
		if sick { // only what follows "allergic to ..." names the food
			at := slices.IndexFunc(words, func(w string) bool { return slices.Contains(health, w) })
			words = words[at+1:]
		}
		for _, w := range words {
			if len(w) < 3 || slices.Contains(filler, w) {
				continue
			}
			if sick {
				add(Proposal{Kind: "health", Word: w})
				continue
			}
			ref := Classify(w)
			if ref.Key == "" {
				continue
			}
			pol := Love
			if neg {
				pol = RatherNot
			}
			add(Proposal{Kind: "taste", Level: ref.Level, Key: ref.Key, Polarity: pol})
		}
	}
	return out
}
