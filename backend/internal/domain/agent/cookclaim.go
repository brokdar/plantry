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
	dishStop = map[string]bool{"with": true, "and": true, "the": true, "for": true, "mit": true, "und": true, "von": true, "fuer": true}
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

// looselyNames reports a text that shares a word with a dish name but names
// no dish whole.
func looselyNames(text string, foods []string) bool {
	words := map[string]bool{}
	for _, w := range strings.FieldsFunc(diet.Fold(text), func(r rune) bool { return r < 'a' || r > 'z' }) {
		if len(w) >= 4 && !dishStop[w] {
			words[w] = true
		}
	}
	for _, name := range foods {
		if namesFood(text, name) {
			continue
		}
		for _, w := range strings.FieldsFunc(diet.Fold(name), func(r rune) bool { return r < 'a' || r > 'z' }) {
			if len(w) >= 4 && !dishStop[w] && words[w] {
				return true
			}
		}
	}
	return false
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
	if cookPast.MatchString(text) || (len(named) == 0 && looselyNames(text, t.foods)) {
		return false
	}
	days := weekdayName.FindAllStringIndex(text, -1)
	tiedDays := map[int]bool{} // index into days
	for _, v := range verbs {
		for i, d := range days {
			if d[1] > v[0]-24 && d[0] < v[1]+24 {
				tiedDays[i] = true
			}
		}
	}
	if len(tiedDays) == 0 && len(named) == 0 {
		return false // a cooking claim about no day and no dish
	}
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
	return true
}
