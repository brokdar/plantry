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
	// cookExclusive is a claim that a day cooks once or only something.
	cookExclusive = regexp.MustCompile(`(?i)\b(one cooking time|a single cooking time|(?:only|just) (?:once|one (?:cooking|time|batch))|cooks? once|nur einmal|einmal gekocht|eine kochzeit|einzige kochzeit)\b`)
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
		// Words that praise a dish say nothing about which one ("Kartoffelgratin
		// – einfach und klassisch", "Easy chicken curry"; round 9 M3: "nicht
		// einfach streichen" is no loose dish name).
		"einfach": true, "klassisch": true, "easy": true, "simple": true, "classic": true, "quick": true, "schnell": true,
	}
)

var mealKey = map[string]string{"breakfast": "breakfast", "fruehstueck": "breakfast", "lunch": "lunch", "mittag": "lunch", "dinner": "dinner", "supper": "dinner", "abend": "dinner"}

// claimCell is one cooking time of the block: a day, the meal it is cooked at
// ("" for a day-wide line) and the dishes cooked then.
type claimCell struct {
	day, meal string
	dishes    []string
	// feeds are, per dish, the weekdays and meals it feeds ("for Mon 30 Nov
	// lunch, ..."): dish -> weekday -> meals ("*" when the line names none).
	feeds map[string]map[string]map[string]bool
}

// feeding reports whether the dish (any dish when "") of the cell feeds the
// day, at every one of the meals.
func (c claimCell) feeding(dish, day string, meals map[string]bool) bool {
	for d, byDay := range c.feeds {
		if dish != "" && d != dish {
			continue
		}
		have := byDay[day]
		if len(have) == 0 {
			continue
		}
		ok := true
		for m := range meals {
			if !have[m] && !have["*"] {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// eatList is a cooked dish with the list of meals it is for: "for <...>" up
// to the next dish, the note in parentheses or the end.
var eatList = regexp.MustCompile(`(?:you cook |; )"([^"]+)" for ([^;(]+)`)

// eatMarker ends a span before a weekday that names a day the cooking is
// eaten on, not cooked on: "reicht für Montag", "covers Wednesday", "for
// Thursday lunch", "bis Freitag" (V81 L1, tune-v2c cook_elsewhere-12).
var eatMarker = regexp.MustCompile(`(?i)\b(covers?|covering|feeds?|feeding|lasts?|for|until|through|reicht|reichen|f(?:ü|ue)r|bis|eaten|isst|essen)\b[^.,;!?\d]{0,12}$`)

// cookOnDay is a preposition that puts the cooking on the day ("for 4 on
// Wednesday"): the weekday after it is a cooking day again.
var cookOnDay = regexp.MustCompile(`(?i)\b(on|at|am|an)\s*$`)

// movedTo ends a span before the weekday the cooking moved to ("moved to",
// "auf ... verschoben").
var (
	movedTo     = regexp.MustCompile(`(?i)\b(?:moved|moves|shifted|shifts)\s+(?:over\s+)?to\s+(?:the\s+)?$`)
	movedToDE   = regexp.MustCompile(`(?i)\b(?:auf|nach)\s+(?:den\s+)?$`)
	movedVerbDE = regexp.MustCompile(`(?i)^[^.,;!?]{0,24}\b(?:verschoben|verlegt)\b`)
	// eatAnd joins a weekday to the eating day before it.
	eatAnd = regexp.MustCompile(`(?i)^\s*(?:(?:breakfast|lunch|dinner|supper|mittag\w*|abend\w*|fr(?:ü|ue)hst(?:ü|ue)ck)\s*)?(?:,|and|und|or|oder|&)\s*$`)
	// eatAfter is a weekday (and meal) that eats: the subject of "eats".
	eatAfter = regexp.MustCompile(`(?i)^\w*\s*(?:(?:breakfast|lunch|dinner|supper)\s+)?(?:eats?|isst|essen)\b`)
)

// listJoin is a comma, semicolon, colon or "and"/"or" between two days.
var listJoin = regexp.MustCompile(`(?i)[,;:]|\b(?:and|und|or|oder)\b`)

// afterPunct is the part of a span after its last comma, semicolon, colon,
// "and" or "or": a meal word before them belongs to the day before ("for
// Thursday dinner and Friday lunch").
func afterPunct(span string) string {
	if all := listJoin.FindAllStringIndex(span, -1); len(all) > 0 {
		return span[all[len(all)-1][1]:]
	}
	return span
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
			c.feeds = map[string]map[string]map[string]bool{}
			for _, l := range eatList.FindAllStringSubmatch(f.EN, -1) {
				dish := strings.ToLower(l[1])
				if c.feeds[dish] == nil {
					c.feeds[dish] = map[string]map[string]bool{}
				}
				for _, item := range strings.Split(l[2], ",") {
					w := weekdayRe.FindString(item)
					if w == "" {
						continue
					}
					day := weekdayKey(w)
					if c.feeds[dish][day] == nil {
						c.feeds[dish][day] = map[string]bool{}
					}
					meals := mealsNear(item)
					if len(meals) == 0 {
						meals["*"] = true
					}
					for m := range meals {
						c.feeds[dish][day][m] = true
					}
				}
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

// cookTimes is the number of times the day cooks at: its distinct cooking
// meals, and one for a day-wide line alone.
func cookTimes(cells []claimCell, day string) int {
	meals := map[string]bool{}
	seen := false
	for _, c := range cells {
		if c.day == day {
			seen = true
			if c.meal != "" {
				meals[c.meal] = true
			}
		}
	}
	if len(meals) == 0 && seen {
		return 1
	}
	return len(meals)
}

// countCells is the number of cooking cells the dish is cooked in.
func countCells(cells []claimCell, dish string) int {
	n := 0
	for _, c := range cells {
		if slices.Contains(c.dishes, dish) {
			n++
		}
	}
	return n
}

// nothingCookedBacked reports whether a sentence that denies cooking says only
// that a day the block gives as "nothing is cooked" cooks nothing: it names
// such days only, no dish, and is not in the past tense.
func nothingCookedBacked(text string, t replyTurn) bool {
	days := weekdayName.FindAllString(text, -1)
	if len(days) == 0 || cookPast.MatchString(text) || unnegatedDone(text) {
		return false
	}
	for _, name := range t.foods {
		if namesFood(text, name) {
			return false
		}
	}
	for _, w := range days {
		key := weekdayKey(w)
		if !slices.ContainsFunc(t.block.facts, func(f fact) bool {
			return f.Class == classMoved && strings.Contains(f.EN, "nothing is cooked") && namesWeekday(f.EN[:strings.IndexAny(f.EN+":", ":,")], key)
		}) {
			return false
		}
	}
	return true
}

// cookingBacked reports whether the block backs every cooking claim of text.
func cookingBacked(text string, t replyTurn) bool {
	if t.block == nil {
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
		if t.block.nothingSaved {
			return refusalBacked(text, t)
		}
		return false
	}
	if cookDenial.MatchString(text) {
		return nothingCookedBacked(text, t)
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
	eatDays := map[int]bool{}  // weekdays after an eating word: eaten, not cooked, then
	for i, d := range days {
		lo := 0
		if i > 0 {
			lo = days[i-1][1]
		}
		before := text[lo:d[0]]
		after := text[d[1]:]
		if i+1 < len(days) {
			after = text[d[1]:days[i+1][0]]
		}
		switch {
		case movedTo.MatchString(before) || movedToDE.MatchString(before) && movedVerbDE.MatchString(after):
			// The day the cooking moved to is a cooking day, however far
			// from the cooking word (round 9 M2, dev-v2d g2-away-17: "the
			// cooking for Friday dinner has moved to Friday lunch").
			tiedDays[i] = true
		case i > 0 && eatDays[i-1] && eatAnd.MatchString(before), eatAfter.MatchString(after):
			// "for Thursday dinner and Friday lunch", "Thursday dinner eats
			// from it": eaten, not cooked, then (round 9 M2, dev-v2d
			// g2-cook_elsewhere-17).
			eatDays[i] = true
		}
		if m := eatMarker.FindStringIndex(before); m != nil && !tiedDays[i] && !cookVerb.MatchString(before[m[0]:]) && !cookOnDay.MatchString(before) {
			eatDays[i] = true
		}
	}
	for _, v := range verbs {
		for i, d := range days {
			if !eatDays[i] && d[1] > v[0]-24 && d[0] < v[1]+24 {
				tiedDays[i] = true
			}
		}
	}
	// A day the cooking is eaten on is fed by a cooking line, at the meal the
	// sentence names, and by every dish it names (V81 review F1: "The chili is
	// cooked for Monday" when only the lasagne feeds Monday is false).
	for i := range eatDays {
		key := weekdayKey(text[days[i][0]:days[i][1]])
		lo, hi := max(0, days[i][0]-14), min(len(text), days[i][1]+20)
		if i > 0 {
			lo = max(lo, days[i-1][1])
		}
		if i+1 < len(days) {
			hi = min(hi, days[i+1][0])
		}
		before := text[lo:days[i][0]]
		if i > 0 && lo == days[i-1][1] {
			before = afterPunct(before) // the meal words of the day before stay its own
		}
		meals := mealsNear(before)
		for m := range mealsNear(text[days[i][1]:hi]) {
			meals[m] = true
		}
		feeds := func(dish string) bool {
			return slices.ContainsFunc(cells, func(c claimCell) bool { return c.feeding(dish, key, meals) })
		}
		if !feeds("") {
			return false
		}
		for _, n := range named {
			if !feeds(strings.ToLower(strings.TrimSpace(n))) {
				return false
			}
		}
		var fed []string // the dishes feeding that day
		for _, c := range cells {
			for _, d := range c.dishes {
				if c.feeding(d, key, nil) && !slices.Contains(fed, d) {
					fed = append(fed, d)
				}
			}
		}
		for _, phrase := range loose {
			if !uniqueDishFor(phrase, fed) {
				return false
			}
		}
	}
	if len(tiedDays) == 0 && len(named) == 0 && len(loose) == 0 {
		return false // a cooking claim about no day and no dish
	}
	// An exclusive claim ("one cooking time", "only once", "nur einmal") is
	// true when its day cooks at one time (V84; the sentence need not name
	// every dish cooked then). The dishes it names must be cooked at that
	// time, which the named-dish checks below enforce.
	exclusive := cookExclusive.MatchString(text)
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
		before := text[lo:days[i][0]]
		if i > 0 && lo == days[i-1][1] {
			before = afterPunct(before) // the meal words of the day before stay its own
		}
		meals := mealsNear(before)
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
		if exclusive && cookTimes(cells, key) > 1 {
			return false // the day cooks at more than one time
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
	if exclusive && len(tiedDays) == 0 {
		// No day tied: every day the sentence names cooks at one time; with no
		// day named, each dish it names is cooked at one time of the plan.
		for _, d := range days {
			if cookTimes(cells, weekdayKey(text[d[0]:d[1]])) > 1 {
				return false
			}
		}
		if len(days) == 0 {
			for _, n := range named {
				k := strings.ToLower(strings.TrimSpace(n))
				if countCells(cells, k) > 1 {
					return false
				}
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

// Refusal reasons (round 9 M3, dev-v2d g2-remove-22, g2-swap-23,
// h2-named-00, h2-cookref-03, g2-cook_elsewhere-03): on a turn that saved
// nothing, the block holds no cooking line, only the code-rendered refused
// part ("Left out: ... (it would create a problem on Mon 7 Dec: cooking times
// per day (2 cooking times, at most 1))", "Left out: cooking "X" on Tue 1 Dec
// lunch (Sat 28 Nov dinner ... eat it before then)"). A sentence with a
// cooking word is backed by those lines when it names only their days, meals
// and dishes, says nothing was done, and refuses ("can't", "geht nicht") or
// gives the refused line's own rule: cooking times for a cooking-times
// refusal, eating before the cook for a move_cook refusal. Anything else is
// dropped (G2: the refused part is told with its reason; RP6).
var (
	refusalPhrase  = regexp.MustCompile(`(?i)\b(?:can'?t|cannot|can not|could ?n[o']?t|not possible|isn'?t possible|won'?t work|doesn'?t work|nicht möglich|geht (?:\S+ ){0,3}nicht|klappt (?:\S+ ){0,3}nicht|(?:kann|könnte) ich (?:\S+ ){0,6}nicht|lässt sich (?:\S+ ){0,6}nicht)`)
	cookTimesWords = regexp.MustCompile(`(?i)cooking times?|cooking sessions?|kochzeit\w*|kochvorg(?:a|ä)ng\w*`)
	eatWords       = regexp.MustCompile(`(?i)\b(?:eat|eats|eaten|eating|essen|isst|gegessen|before|by then|vorher|davor|zuvor|bis dahin)\b`)
	// pastOfEating are the cookPast words a move_cook reason uses for the
	// meals that eat first ("already eaten", "essen schon vorher davon").
	pastOfEating = regexp.MustCompile(`(?i)\b(?:already|before|earlier|bereits|schon|vorher|zuvor|davor)\b`)
	// hypothetical is the mood of a change that was not made.
	hypothetical = regexp.MustCompile(`(?i)\b(?:would|could|würde\w*|wäre\w*|gäbe|bräuchte\w*|entstünde\w*|hätte\w*|käme\w*|müsste\w*|könnte\w*)\b|\w'd\b`)
	// mealCompound is a meal word inside a weekday compound ("Montagmittag").
	mealCompound = regexp.MustCompile(`(?i)(?:montag|dienstag|mittwoch|donnerstag|freitag|samstag|sonntag)(mittag|abend|fruehstueck)`)
)

// mealsIn are the meals a text names, also inside a weekday compound.
func mealsIn(text string) map[string]bool {
	out := mealsNear(text)
	for _, m := range mealCompound.FindAllStringSubmatch(diet.Fold(text), -1) {
		out[mealKey[strings.ToLower(m[1])]] = true
	}
	return out
}

// savedWords are the words that tell a change as done, wider than doneClaim.
var savedWords = regexp.MustCompile(`(?i)\b(?:saved|done|updated|planned|moved|swapped|replaced|added|removed|changed|(?:is|are) now|gespeichert|eingetragen|erledigt|aktualisiert|ergänzt|eingefügt|verschoben|getauscht|ersetzt|hinzugefügt|entfernt|geändert|ist jetzt)\b`)

// clauseSplit cuts a sentence into its clauses.
var clauseSplit = regexp.MustCompile(`(?i)[.!?,;:]|\b(?:but|aber|and|und)\b`)

// unnegatedDone reports a clause that tells a change as done without a
// negation of its own.
func unnegatedDone(text string) bool {
	for _, c := range clauseSplit.Split(text, -1) {
		if (savedWords.MatchString(c) || doneClaim.MatchString(c)) && !negation.MatchString(c) {
			return true
		}
	}
	return false
}

// refusalBacked reports whether the refused lines of a no-save block back a
// cooking sentence.
func refusalBacked(text string, t replyTurn) bool {
	var b strings.Builder
	for _, f := range t.block.facts {
		if f.Class == classRefused {
			b.WriteString(" " + f.EN + " " + f.DE)
		}
	}
	refused := b.String()
	if strings.TrimSpace(refused) == "" || unnegatedDone(text) {
		return false
	}
	times := strings.Contains(refused, "cooking times per day")
	eating := strings.Contains(refused, "eat it before then") || strings.Contains(refused, "past its keeping time")
	timesReason, eatReason := cookTimesWords.MatchString(text), eatWords.MatchString(text)
	// The past tense only of the meals that eat before the cook.
	pastOK := eating && !cookPast.MatchString(pastOfEating.ReplaceAllString(text, " "))
	switch {
	case timesReason && !times:
		return false // a cooking-times reason the refusal does not give
	case eatReason && !eating:
		return false // an eating claim the refusal does not make
	case cookPast.MatchString(text) && !pastOK:
		return false
	case timesReason && !hypothetical.MatchString(text):
		// The cooking times of a change that was not made, told like the
		// refused line, as what would be: "dann entstehen zwei Kochzeiten"
		// reads as the saved plan (tune-v2c g2-swap-12).
		return false
	case !refusalPhrase.MatchString(text) && !timesReason && !eatReason:
		return false // no refusal and no reason of the refused line
	}
	for _, w := range weekdayName.FindAllString(text, -1) {
		if !namesWeekday(refused, weekdayKey(w)) {
			return false
		}
	}
	have := mealsIn(refused)
	for m := range mealsIn(text) {
		if !have[m] {
			return false
		}
	}
	var named, dishes []string
	for _, name := range t.foods {
		if namesFood(text, name) {
			if !namesFood(refused, name) {
				return false
			}
			named = append(named, name)
		}
	}
	for _, q := range quotedRe.FindAllString(refused, -1) {
		if d := strings.Trim(q, `"„“”`); !slices.Contains(dishes, d) {
			dishes = append(dishes, d) // the EN and the DE line quote each dish
		}
	}
	for _, phrase := range loosePhrases(text, t.foods, named) {
		if !uniqueDishFor(phrase, dishes) {
			return false
		}
	}
	return true
}
