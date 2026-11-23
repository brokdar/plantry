package agent

import (
	"regexp"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
)

// A cooking claim of the model's text stays when the block's cooking facts
// back it (V77 lint, like P-7 for numbers; tune-v2b: 7 of 8 all-dropped write
// turns were true, block-backed cooking sentences): every weekday and meal it
// names is in the block, every dish it names is in a cooking line, a day it
// says something is cooked on is a day the block cooks on, and it denies no
// cooking. Anything else is dropped as before.

var (
	// cookVerb finds the cooking words a claim can tie to a day.
	cookVerb = regexp.MustCompile(`(?i)cook\w*|gekocht\w*|koch\w*`)
	// weekdayName is a full weekday name, also inside a compound
	// ("Sonntagabend").
	weekdayName = regexp.MustCompile(`(?i)monday|tuesday|wednesday|thursday|friday|saturday|sunday|montag|dienstag|mittwoch|donnerstag|freitag|samstag|sonntag`)
	// mealWordRe is a meal word, also inside a compound, by its key.
	mealWordRe = regexp.MustCompile(`(?i)breakfast|lunch|dinner|supper|fruehstueck|mittag|abend`)
	// cookDenial is a claim that something is not or no longer cooked.
	cookDenial = regexp.MustCompile(`(?i)\b(not|no longer|nothing|nicht|nichts|kein\w*|nie)\b`)
)

var mealKey = map[string]string{"breakfast": "breakfast", "fruehstueck": "breakfast", "lunch": "lunch", "mittag": "lunch", "dinner": "dinner", "supper": "dinner", "abend": "dinner"}

// mealIn: the block text names the meal (EN or DE form).
func mealIn(block, key string) bool {
	switch key {
	case "breakfast":
		return strings.Contains(block, "breakfast") || strings.Contains(block, "fruehstueck")
	case "lunch":
		return strings.Contains(block, "lunch") || strings.Contains(block, "mittag")
	}
	return strings.Contains(block, "dinner") || strings.Contains(block, "abend")
}

// cookingBacked reports whether the block backs every cooking claim of text.
func cookingBacked(text string, t replyTurn) bool {
	if t.block == nil || cookDenial.MatchString(text) {
		return false
	}
	var cookLines, all strings.Builder
	cookDays := map[string]string{} // weekday key → the day's cooking lines
	for _, f := range t.block.facts {
		all.WriteString(" " + f.EN + " " + f.DE)
		if f.Class != classMoved || !movedTold[f.Kind] {
			continue
		}
		cookLines.WriteString(" " + f.EN + " " + f.DE)
		if f.Kind == kindCook && strings.Contains(f.EN, "you cook") {
			if d, _ := cookLineCell(f); d != "" {
				cookDays[weekdayKey(d)] += " " + f.EN
			}
		}
		if f.Kind == kindLeftover && strings.Contains(f.EN, ": cook ") {
			if d, _, ok := strings.Cut(f.EN, ":"); ok {
				cookDays[weekdayKey(d)] += " " + f.EN
			}
		}
	}
	cooking := cookLines.String()
	if strings.TrimSpace(cooking) == "" {
		return false
	}
	blockAll := diet.Fold(all.String())
	for _, w := range weekdayName.FindAllString(text, -1) {
		if !namesWeekday(cooking, weekdayKey(w)) {
			return false
		}
	}
	for _, m := range mealWordRe.FindAllString(diet.Fold(text), -1) {
		if !mealIn(blockAll, mealKey[strings.ToLower(m)]) {
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
	// A day the text ties to a cooking word (within a few words either way)
	// is a day the block cooks on.
	for _, v := range cookVerb.FindAllStringIndex(text, -1) {
		lo, hi := max(0, v[0]-24), min(len(text), v[1]+24)
		best, dist := "", len(text)
		for _, d := range weekdayName.FindAllStringIndex(text[lo:hi], -1) {
			s, e := d[0]+lo, d[1]+lo
			gap := v[0] - e
			if s >= v[1] {
				gap = s - v[1]
			}
			if gap < dist {
				best, dist = text[s:e], gap
			}
		}
		if best == "" {
			continue
		}
		lines, cooks := cookDays[weekdayKey(best)]
		if !cooks {
			return false
		}
		// A dish the text names is cooked on that day.
		if len(named) > 0 && !slices.ContainsFunc(named, func(n string) bool { return namesFood(lines, n) }) {
			return false
		}
	}
	return true
}
