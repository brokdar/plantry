package agent

import (
	"regexp"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
)

// A cooking claim of the model's text stays when the block's cooking cells
// back it (V77 lint, like P-7 for numbers; tune-v2b: 7 of 8 all-dropped write
// turns were true, block-backed cooking sentences). The claim is checked
// against the parsed cooking lines, the cells "<day> at <meal> you cook
// <dish>", never against the days a dish is only eaten or the whole block:
// every weekday tied to a cooking word (within 24 characters of it) is a day
// the block cooks on, a meal named next to that weekday is a meal cooked that
// day, a dish named is cooked on that day, a dish named only loosely ("the
// lasagne" for "Spinach lasagne") cannot be matched and drops the sentence,
// and it denies no cooking and is not in the past tense. Anything else is
// dropped (a heuristic on text fails safe: when unsure, the sentence goes).

var (
	// cookVerb finds the cooking words a claim can tie to a day.
	cookVerb = regexp.MustCompile(`(?i)cook\w*|gekocht\w*|koch\w*`)
	// weekdayName is a full weekday name, also inside a compound
	// ("Sonntagabend").
	weekdayName = regexp.MustCompile(`(?i)monday|tuesday|wednesday|thursday|friday|saturday|sunday|montag|dienstag|mittwoch|donnerstag|freitag|samstag|sonntag`)
	// mealWordRe is a meal word on a word start: "Mittag" is one, "Nachmittag"
	// is not; in a compound the span after the weekday starts on the word.
	mealWordRe = regexp.MustCompile(`(?i)\b(?:breakfast|lunch|dinner|supper|fruehstueck|mittag|abend)`)
	// cookDenial is a claim that something is not or no longer cooked.
	cookDenial = regexp.MustCompile(`(?i)\b(not|no longer|nothing|nicht|nichts|kein\w*|nie)\b`)
	// cookPast is a cooking claim about the past, which no cooking line backs.
	cookPast  = regexp.MustCompile(`(?i)\b(was|were|had|did|before|earlier|previously|already|yesterday|last|war|waren|wurde|wurden|hatte|hatten|vorher|zuvor|fr(?:ue|ü)her|bereits|gestern|letzte\w*)\b`)
	claimDish = regexp.MustCompile(`(?:you cook |; )"([^"]+)" for`)
	cookPort  = regexp.MustCompile(`cook [\d.]+ portions? of the "([^"]+)"`)
	// dishStop are words of a dish name that say nothing about the dish.
	dishStop = map[string]bool{
		"with": true, "and": true, "the": true, "for": true, "mit": true, "und": true, "von": true, "fuer": true,
		"der": true, "die": true, "das": true, "den": true, "dem": true, "ein": true, "eine": true, "einen": true, "oder": true, "bei": true, "aus": true, "from": true, "all": true, "both": true,
	}
)

var mealKey = map[string]string{"breakfast": "breakfast", "fruehstueck": "breakfast", "lunch": "lunch", "mittag": "lunch", "dinner": "dinner", "supper": "dinner", "abend": "dinner"}

// claimCell is one cooking time of the block: a day, the meal it is cooked at
// ("" for a day-wide line) and the dishes cooked then.
type claimCell struct {
	day, meal string
	dishes    []string
}

// claimCells parses the block's cooking lines into cells.
func claimCells(facts []fact) []claimCell {
	var cells []claimCell
	for _, f := range facts {
		if f.Class != classMoved || !movedTold[f.Kind] {
			continue
		}
		if f.Kind == kindCook && strings.Contains(f.EN, "you cook") {
			d, m := cookLineCell(f)
			if d == "" {
				continue
			}
			c := claimCell{day: weekdayKey(d)}
			if m != "?" {
				c.meal = mealKey[strings.ToLower(m)]
			}
			for _, x := range claimDish.FindAllStringSubmatch(f.EN, -1) {
				c.dishes = append(c.dishes, strings.ToLower(x[1]))
			}
			cells = append(cells, c)
		}
		if f.Kind == kindLeftover && strings.Contains(f.EN, ": cook ") {
			if d, _, ok := strings.Cut(f.EN, ":"); ok {
				c := claimCell{day: weekdayKey(d)}
				for _, x := range cookPort.FindAllStringSubmatch(f.EN, -1) {
					c.dishes = append(c.dishes, strings.ToLower(x[1]))
				}
				cells = append(cells, c)
			}
		}
	}
	return cells
}

// mealsNear are the meals a span of text names.
func mealsNear(span string) map[string]bool {
	out := map[string]bool{}
	for _, m := range mealWordRe.FindAllString(diet.Fold(span), -1) {
		out[mealKey[strings.ToLower(m)]] = true
	}
	return out
}

// dishStem folds a plural or singular ("salads", "Bohnen") to one stem.
func dishStem(w string) string {
	switch {
	case strings.HasSuffix(w, "ies") && len(w) > 5:
		return w[:len(w)-3] + "y"
	case (strings.HasSuffix(w, "en") || strings.HasSuffix(w, "es")) && len(w) > 5:
		return w[:len(w)-2]
	case (strings.HasSuffix(w, "s") || strings.HasSuffix(w, "n")) && len(w) > 4:
		return w[:len(w)-1]
	}
	return w
}

// dishWords are the stems of the content words of a text: folded, three
// letters or more, no stop word or article.
func dishWords(text string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(diet.Fold(text), func(r rune) bool { return r < 'a' || r > 'z' }) {
		if len(w) >= 3 && !dishStop[w] {
			out = append(out, dishStem(w))
		}
	}
	return out
}

// loosePhrases are the runs of words in text that are words of a plan dish
// name (a run ends at a punctuation mark or a word no dish name has), except
// the runs inside a dish the text names whole: "the lasagne", "cucumber
// salad". Each is a phrase of a dish named loosely.
func loosePhrases(text string, foods, named []string) [][]string {
	vocab := map[string]bool{}
	for _, f := range foods {
		for _, w := range dishWords(f) {
			vocab[w] = true
		}
	}
	whole := map[string]bool{}
	for _, n := range named {
		for _, w := range dishWords(n) {
			whole[w] = true
		}
	}
	var out [][]string
	for _, chunk := range strings.FieldsFunc(text, func(r rune) bool { return strings.ContainsRune(",;.:()!?\"„“”", r) }) {
		var run []string
		flush := func() {
			if len(run) > 0 {
				out = append(out, run)
			}
			run = nil
		}
		for _, w := range strings.FieldsFunc(diet.Fold(chunk), func(r rune) bool { return r < 'a' || r > 'z' }) {
			switch {
			case dishStop[w] || len(w) < 3:
				flush()
			case vocab[dishStem(w)]:
				if !whole[dishStem(w)] {
					run = append(run, dishStem(w))
				} else {
					flush()
				}
			default:
				flush()
			}
		}
		flush()
	}
	return out
}

// uniqueDishFor reports that exactly one of the dishes has every word of the
// phrase.
func uniqueDishFor(phrase []string, dishes []string) bool {
	n := 0
	for _, d := range dishes {
		have := dishWords(d)
		if !slices.ContainsFunc(phrase, func(w string) bool { return !slices.Contains(have, w) }) {
			n++
		}
	}
	return n == 1
}

// cookingBacked reports whether the block backs every cooking claim of text.
func cookingBacked(text string, t replyTurn) bool {
	if t.block == nil || cookDenial.MatchString(text) {
		return false
	}
	var cookLines strings.Builder
	for _, f := range t.block.facts {
		if f.Class == classMoved && movedTold[f.Kind] {
			cookLines.WriteString(" " + f.EN + " " + f.DE)
		}
	}
	cooking := cookLines.String()
	if strings.TrimSpace(cooking) == "" {
		return false
	}
	cells := claimCells(t.block.facts)
	for _, w := range weekdayName.FindAllString(text, -1) {
		if !namesWeekday(cooking, weekdayKey(w)) {
			return false
		}
	}
	var named []string
	for _, name := range t.foods {
		if namesFood(text, name) {
			if !namesFood(cooking, name) {
				return false
			}
			named = append(named, name)
		}
	}
	verbs := cookVerb.FindAllStringIndex(text, -1)
	if len(verbs) == 0 {
		return true
	}
	if cookPast.MatchString(text) {
		return false
	}
	loose := loosePhrases(text, t.foods, named)
	days := weekdayName.FindAllStringIndex(text, -1)
	tiedDays := map[int]bool{} // index into days
	for _, v := range verbs {
		for i, d := range days {
			if d[1] > v[0]-24 && d[0] < v[1]+24 {
				tiedDays[i] = true
			}
		}
	}
	if len(tiedDays) == 0 && len(named) == 0 && len(loose) == 0 {
		return false // a cooking claim about no day and no dish
	}
	var scope []claimCell      // the cook cells a loosely named dish is looked up in
	onDay := map[string]bool{} // named dishes found on a tied day
	for i := range days {
		if !tiedDays[i] {
			continue
		}
		key := weekdayKey(text[days[i][0]:days[i][1]])
		lo, hi := max(0, days[i][0]-14), min(len(text), days[i][1]+14)
		if i > 0 {
			lo = max(lo, days[i-1][1])
		}
		if i+1 < len(days) {
			hi = min(hi, days[i+1][0])
		}
		meals := mealsNear(text[lo:days[i][0]])
		for m := range mealsNear(text[days[i][1]:hi]) {
			meals[m] = true
		}
		var day []claimCell
		for _, c := range cells {
			if c.day != key {
				continue
			}
			if len(meals) > 0 && !meals[c.meal] {
				continue
			}
			day = append(day, c)
		}
		if len(day) == 0 {
			return false // not a cooking day, or not at that meal
		}
		scope = append(scope, day...)
		found := false
		for _, n := range named {
			for _, c := range day {
				if slices.Contains(c.dishes, strings.ToLower(strings.TrimSpace(n))) {
					onDay[n] = true
					found = true
				}
			}
		}
		if len(named) > 0 && !found {
			return false
		}
	}
	if len(tiedDays) > 0 {
		for _, n := range named {
			if !onDay[n] {
				return false
			}
		}
	}
	if len(loose) == 0 {
		return true
	}
	// No day tied to a cooking word: the weekdays the sentence names scope it.
	if len(tiedDays) == 0 {
		for _, d := range days {
			key := weekdayKey(text[d[0]:d[1]])
			for _, c := range cells {
				if c.day == key {
					scope = append(scope, c)
				}
			}
		}
	}
	var dishes []string
	for _, c := range scope {
		for _, d := range c.dishes {
			if !slices.Contains(dishes, d) {
				dishes = append(dishes, d)
			}
		}
	}
	for _, phrase := range loose {
		if !uniqueDishFor(phrase, dishes) {
			return false // no dish, or several, cooked on that day has these words
		}
	}
	return true
}
