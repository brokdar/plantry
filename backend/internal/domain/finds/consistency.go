package finds

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/gates"
	"github.com/jaltszeimer/plantry/backend/internal/domain/importer"
)

// CodeStepMentionsRemoved blocks an adapted draft whose steps still name an
// ingredient the adaptation replaced ("squeeze the lime" after lime became
// vinegar).
const CodeStepMentionsRemoved = "step_mentions_removed"

// genericWords name no ingredient on their own.
var genericWords = map[string]bool{
	"fresh": true, "dried": true, "ground": true, "chopped": true, "sliced": true, "diced": true, "large": true,
	"small": true, "medium": true, "juice": true, "powder": true, "sauce": true, "paste": true, "oil": true,
	"whole": true, "minced": true, "grated": true, "optional": true, "taste": true,
}

// words splits text into lowercase letter runs of four or more letters
// without a plural s.
func words(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len([]rune(w)) >= 4 {
			out = append(out, strings.TrimSuffix(w, "s"))
		}
	}
	return out
}

// removedMentions finds the steps of an adapted draft that still mention a
// replaced ingredient no remaining line has.
func removedMentions(base *importer.Draft, a adaptation) []gates.Finding {
	var left strings.Builder
	for _, in := range a.draft.Ingredients {
		left.WriteString(" " + in.Name + " " + in.RawText)
	}
	remaining := map[string]bool{}
	for _, w := range words(left.String()) {
		remaining[w] = true
	}
	var out []gates.Finding
	for _, ch := range a.changes {
		for _, in := range base.Ingredients {
			if in.RawText != ch.From {
				continue
			}
			for _, w := range words(in.Name) {
				if remaining[w] || genericWords[w] {
					continue
				}
				re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(w) + `(?:s|es)?\b`)
				for i, step := range a.draft.Instructions {
					if re.MatchString(step) {
						out = append(out, gates.Finding{
							Code: CodeStepMentionsRemoved, Ingredient: -1, Step: i,
							Params: map[string]string{"ingredient": in.Name},
							Reason: "step " + strconv.Itoa(i+1) + " still mentions " + w + ", which was replaced by " + ch.To,
						})
						break
					}
				}
			}
		}
	}
	return out
}

// fillerWords are in a dish name without being the dish.
var fillerWords = map[string]bool{
	"creamy": true, "speedy": true, "easy": true, "quick": true, "simple": true, "best": true, "classic": true,
	"with": true, "and": true, "style": true, "homemade": true, "perfect": true, "ultimate": true, "healthy": true,
	"fast": true, "authentic": true, "favorite": true, "mit": true, "und": true, "nach": true, "einfach": true,
	"schnell": true, "schnelle": true, "schneller": true, "cremig": true, "cremige": true, "klassisch": true,
}

func dishWords(name string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return !unicode.IsLetter(r) }) {
		w = strings.TrimSuffix(w, "s")
		if len([]rune(w)) > 2 && !fillerWords[w] && !fillerWords[w+"s"] {
			set[w] = true
		}
	}
	return set
}

// tooSimilar reports whether a dish name nearly duplicates a library name:
// the dish words (filler like creamy or easy ignored) of the shorter one
// are all but at most one in the other, with at least two dish words on the smaller side.
func tooSimilar(name string, library []string) bool {
	a := dishWords(name)
	for _, l := range library {
		b := dishWords(l)
		shared := 0
		for w := range a {
			if b[w] {
				shared++
			}
		}
		small := min(len(a), len(b))
		// One word is a match only against the same single word.
		if small < 2 && (len(a) != len(b) || shared != 1) {
			continue
		}
		if (shared >= 2 || shared == small) && float64(shared) >= 0.75*float64(small) {
			return true
		}
	}
	return false
}

// CodeMeatInMeatFree blocks a find asked for a meat-free main whose adapted
// recipe still has a meat or fish line.
const CodeMeatInMeatFree = "meat_in_meat_free"

// meatInMeatFree is the block for the first meat or fish line of a draft
// the dish asked meat-free (planningDishes), if any.
func meatInMeatFree(dish string, d *importer.Draft) []gates.Finding {
	if !strings.Contains(" "+dish+" ", " meat-free ") {
		return nil
	}
	for i, in := range d.Ingredients {
		if g := diet.GroupOf(in.Name); g == diet.GroupMeat || g == diet.GroupFish {
			return []gates.Finding{{
				Code: CodeMeatInMeatFree, Ingredient: i, Step: -1,
				Reason: in.Name + " is not meat-free",
			}}
		}
	}
	return nil
}
