package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// Kinds of must-say fact: what a household must hear after a saved change.
const (
	factSmall    = "small_meal"
	factKeep     = "keeps"
	factOdd      = "odd_name"
	factDay      = "day_range"
	factNoCook   = "nothing_cooked"
	factDropped  = "no_longer_cooked"
	factShorter  = "shorter_cooking"
	factTooOld   = "too_old"
	factResize   = "resized"
	factMoved    = "cook_moved"
	factMeat     = "week_meat"
	factEnergy   = "week_energy"
	factAwayMeat = "away_meat"
	factShare    = "week_share"
)

// factOrder is the order of the report's tell: facts are told in it.
var factOrder = []string{factSmall, factKeep, factOdd, factDay, factNoCook, factDropped, factShorter, factTooOld, factResize, factMoved, factMeat, factEnergy, factAwayMeat, factShare}

// mustFact is one fact of a saved change the final reply has to carry, as
// typed data (the report's tell sentence says the same in words). Days are
// labels like "Fri 27 Nov". Which fields a kind uses:
//
//	small_meal       Day, Meal, N (eaters), Kcal
//	keeps            Day, Meal, Dish, Day2 (cooked), Days (earlier), Keep, Last
//	odd_name         Dish
//	day_range        Day, Kcal, Side (below|above), Lo, Hi, Minimum
//	nothing_cooked   Day
//	no_longer_cooked Day, Dish
//	shorter_cooking  Day, Meal (cooking time), Dishes (still cooked), Time, WasTime, Gone
//	too_old          Day, Dish, Keep
//	resized          Day (cooked), Dish, N (portions), Was
//	cook_moved       Dish, Day, Meal, Day2, Meal2
//	week_meat        N (g), Was, Limit
//	week_energy      N (%), Was, Lo, Hi
//	away_meat        N (g), Limit
//	week_share       Share (fat|carbs), N (%), Lo, Hi
type mustFact struct {
	Kind    string     `json:"kind"`
	Day     string     `json:"day,omitempty"`
	Day2    string     `json:"day2,omitempty"`
	Meal    string     `json:"meal,omitempty"`
	Meal2   string     `json:"meal2,omitempty"`
	Dish    string     `json:"dish,omitempty"`
	Dishes  []string   `json:"dishes,omitempty"`
	Gone    []goneDish `json:"gone,omitempty"`
	Side    string     `json:"side,omitempty"`
	Share   string     `json:"share,omitempty"`
	Time    string     `json:"time,omitempty"`
	WasTime string     `json:"was_time,omitempty"`
	Last    string     `json:"last,omitempty"`
	N       float64    `json:"n,omitempty"`
	Was     float64    `json:"was,omitempty"`
	Lo      float64    `json:"lo,omitempty"`
	Hi      float64    `json:"hi,omitempty"`
	Limit   float64    `json:"limit,omitempty"`
	Kcal    float64    `json:"kcal,omitempty"`
	Keep    int        `json:"keep,omitempty"`
	Days    int        `json:"days,omitempty"`
	Minimum bool       `json:"minimum,omitempty"`
}

// goneDish is a dish a shorter cooking no longer cooks; Leftovers when the day
// still eats it from another cooking.
type goneDish struct {
	Dish      string `json:"dish"`
	Leftovers bool   `json:"leftovers,omitempty"`
}

// mustSink collects the facts while a report is built. Nil is a sink that
// drops them.
type mustSink struct{ facts []mustFact }

func (s *mustSink) add(f mustFact) {
	if s != nil {
		s.facts = append(s.facts, f)
	}
}

// sorted is the facts in the order of the tell (emission order within a kind).
func (s *mustSink) sorted() []mustFact {
	if s == nil {
		return nil
	}
	out := slices.Clone(s.facts)
	slices.SortStableFunc(out, func(a, b mustFact) int {
		return slices.Index(factOrder, a.Kind) - slices.Index(factOrder, b.Kind)
	})
	return out
}

// mustItem is a fact as the reply carries it: the line to add in English and
// German, and the test for a reply that already says it.
type mustItem struct {
	en, de string
	said   func(text string, german bool) bool
}

// mustTold returns the facts of a result's report the reply must carry, and
// whether the result is a saved change's report (a dry run's is renamed away).
func mustTold(view json.RawMessage) (facts []mustFact, saved bool) {
	var v struct {
		Report *struct {
			Must []mustFact `json:"must"`
		} `json:"report"`
	}
	if json.Unmarshal(view, &v) != nil || v.Report == nil {
		return nil, false
	}
	return v.Report.Must, true
}

// withoutMust removes the must facts from a result's reports: the model reads
// the tell, the code reads the facts.
func withoutMust(view json.RawMessage) json.RawMessage {
	if !strings.Contains(string(view), `"must"`) {
		return view
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(view, &obj) != nil {
		return view
	}
	changed := false
	for _, key := range []string{"report", "report_if_applied"} {
		var rep map[string]json.RawMessage
		if json.Unmarshal(obj[key], &rep) != nil || rep == nil {
			continue
		}
		if _, ok := rep["must"]; ok {
			delete(rep, "must")
			b, _ := json.Marshal(rep)
			obj[key], changed = b, true
		}
	}
	if !changed {
		return view
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return view
	}
	return b
}

// item renders a fact; ok is false for a fact of no known shape.
func (f mustFact) item() (mustItem, bool) {
	// Names reach the reply as the sentences quote them: cleaned.
	f.Dish = food.Data(f.Dish)
	f.Dishes = slices.Clone(f.Dishes)
	for i := range f.Dishes {
		f.Dishes[i] = food.Data(f.Dishes[i])
	}
	f.Gone = slices.Clone(f.Gone)
	for i := range f.Gone {
		f.Gone[i].Dish = food.Data(f.Gone[i].Dish)
	}
	switch f.Kind {
	case factResize:
		return resizeItem(f), true
	case factDay:
		return dayItem(f), true
	case factEnergy:
		return energyItem(f), true
	case factMeat, factAwayMeat:
		return meatItem(int(f.N+0.5), int(f.Limit+0.5)), true
	case factShare:
		return shareItem(f), true
	case factNoCook:
		return noCookItem(f), true
	case factDropped:
		return droppedItem(f), true
	case factMoved:
		return movedItem(f), true
	case factKeep:
		return keepItem(f), true
	case factTooOld:
		return tooOldItem(f), true
	case factOdd:
		return oddNameItem(f), true
	case factShorter:
		return shorterItem(f), true
	case factSmall:
		return smallItem(f), true
	}
	return mustItem{}, false
}

var deMeal = map[string]string{"breakfast": "zum Frühstück", "lunch": "zum Mittagessen", "dinner": "zum Abendessen"}

func deMealAt(m string) string {
	if s, ok := deMeal[m]; ok {
		return "(" + s + ")"
	}
	return ""
}

// countDays says "1 day" or "3 days"; the German form is "1 Tag", "3 Tage".
func countDays(n int, german bool) string {
	switch {
	case german && n == 1:
		return "1 Tag"
	case german:
		return fmt.Sprintf("%d Tage", n)
	case n == 1:
		return "1 day"
	}
	return fmt.Sprintf("%d days", n)
}

func fmtComma(v float64) string { return strings.Replace(fmtNum(v), ".", ",", 1) }

func resizeItem(f mustFact) mustItem {
	unit, deUnit := "portions", "Portionen"
	if f.N == 1 {
		unit, deUnit = "portion", "Portion"
	}
	return mustItem{
		en: fmt.Sprintf("%s: cook %s %s of the %s (was %s).", f.Day, fmtNum(f.N), unit, food.Quote(f.Dish), fmtNum(f.Was)),
		de: fmt.Sprintf("%s: %s %s „%s“ kochen (statt %s).", deDay(f.Day), fmtComma(f.N), deUnit, f.Dish, fmtComma(f.Was)),
		said: func(text string, _ bool) bool {
			return lineWith(text, func(l string) bool {
				// A number word counts next to the word portion only.
				n := digitsIn(l, fmtNum(f.N)) || ((wordsIn(l, fmtNum(f.N)) || (f.N == 1 && onePortion.MatchString(l))) && portionNoun.MatchString(l))
				// A line without the day names the dish and the portions.
				return n && (dayIn(l, f.Day) || (dishIn(l, f.Dish) && portionNoun.MatchString(l)))
			})
		},
	}
}

func dayItem(f mustFact) mustItem {
	de := "unter"
	if f.Side == "above" {
		de = "über"
	}
	kcal := fmt.Sprint(int(f.Kcal + 0.5))
	return mustItem{
		en: fmt.Sprintf("%s: %s kcal, %s a good amount for the day (%.0f–%.0f kcal).", f.Day, kcal, f.Side, f.Lo, f.Hi),
		de: fmt.Sprintf("%s: %s kcal, %s dem passenden Bereich für den Tag (%.0f–%.0f kcal).", deDay(f.Day), kcal, de, f.Lo, f.Hi),
		said: func(text string, _ bool) bool {
			// The day by name or date, or by the range the line states: a
			// reply that says "the day goes to 1774 kcal, below 1815–2218".
			return lineWith(text, func(l string) bool {
				return numberIn(l, kcal) && (dayIn(l, f.Day) || (numberIn(l, fmt.Sprint(int(f.Lo+0.5))) && numberIn(l, fmt.Sprint(int(f.Hi+0.5)))))
			})
		},
	}
}

func energyItem(f mustFact) mustItem {
	now, was, lo, hi := fmtNum(f.N), fmtNum(f.Was), fmtNum(f.Lo), fmtNum(f.Hi)
	re := regexp.MustCompile(`(^|[^\d.,])` + regexp.QuoteMeta(now) + `\s?(%|Prozent|percent)`)
	return mustItem{
		en:   fmt.Sprintf("Week: average energy is %s%% of the target (was %s%%); the week's range is %s–%s%%.", now, was, lo, hi),
		de:   fmt.Sprintf("Woche: Durchschnitt %s %% des Ziels (vorher %s %%); der Bereich liegt bei %s–%s %%.", now, was, lo, hi),
		said: func(text string, _ bool) bool { return re.MatchString(text) },
	}
}

var (
	againstWord = regexp.MustCompile(`(?i)\b(against|gegen|gegenüber)\b`)
	meatWord    = regexp.MustCompile(`(?i)meat|fleisch`)
	overWord    = regexp.MustCompile(`(?i)\b(over|above|exceed\w*|more than|über\w*|darüber|drüber|mehr)\b`)
)

// meatItem says the week's meat against its limit, with the grams over.
func meatItem(now, limit int) mustItem {
	over := now - limit
	if over <= 0 {
		return mustItem{
			en: fmt.Sprintf("Week: meat is %d g, the limit is %d g.", now, limit),
			de: fmt.Sprintf("Woche: Fleisch %d g, Grenze %d g.", now, limit),
			said: func(text string, _ bool) bool {
				return lineWith(text, func(l string) bool { return meatWord.MatchString(l) && numberIn(l, fmt.Sprint(now)) })
			},
		}
	}
	return mustItem{
		en: fmt.Sprintf("Week: meat is %d g against a limit of %d g, %d g over.", now, limit, over),
		de: fmt.Sprintf("Woche: Fleisch %d g bei höchstens %d g, also %d g darüber.", now, limit, over),
		said: func(text string, _ bool) bool {
			return lineWith(text, func(l string) bool {
				if numberIn(l, fmt.Sprint(over)) && overWord.MatchString(l) {
					return true
				}
				// "the week's meat is now 471 g, over the 300 g limit"
				for _, sent := range strings.Split(l, ". ") {
					if meatWord.MatchString(sent) && numberIn(sent, fmt.Sprint(now)) && numberIn(sent, fmt.Sprint(limit)) && (overWord.MatchString(sent) || againstWord.MatchString(sent)) {
						return true
					}
				}
				return false
			})
		},
	}
}

func shareItem(f mustFact) mustItem {
	en, de, re := "fat", "Fett", regexp.MustCompile(`(?i)fat|fett`)
	if f.Share == "carbs" {
		en, de, re = "carbohydrates", "Kohlenhydrate", regexp.MustCompile(`(?i)carb|kohlenhydrat`)
	}
	pct := fmtNum(f.N)
	return mustItem{
		en: fmt.Sprintf("Week: %s is %s%% of the energy (range %s–%s%%), further out than before.", en, pct, fmtNum(f.Lo), fmtNum(f.Hi)),
		de: fmt.Sprintf("Woche: %s machen %s %% der Energie aus (Bereich %s–%s %%), weiter außerhalb als vorher.", de, pct, fmtNum(f.Lo), fmtNum(f.Hi)),
		said: func(text string, _ bool) bool {
			return lineWith(text, func(l string) bool { return re.MatchString(l) && numberIn(l, pct) })
		},
	}
}

// onePortion is "one portion" said in words.
var onePortion = regexp.MustCompile(`(?i)\b(one|eine[rn]?) portion`)

var portionNoun = regexp.MustCompile(`(?i)portion`)

var noCookWord = regexp.MustCompile(`(?i)(nothing|no longer|not cooked|no cooking|leftover|left over|is gone|dropped|removed|nichts|nicht mehr|kein|reste|entfällt|entfallen|fällt weg|wegfall)`)

func noCookItem(f mustFact) mustItem {
	return mustItem{
		en: fmt.Sprintf("%s: nothing is cooked any more.", f.Day),
		de: fmt.Sprintf("%s: es wird nichts mehr gekocht.", deDay(f.Day)),
		said: func(text string, _ bool) bool {
			return lineWith(text, func(l string) bool { return dayIn(l, f.Day) && noCookWord.MatchString(l) })
		},
	}
}

func droppedItem(f mustFact) mustItem {
	return mustItem{
		en: fmt.Sprintf("%s: %s is no longer cooked.", f.Day, food.Quote(f.Dish)),
		de: fmt.Sprintf("%s: „%s“ wird nicht mehr gekocht.", deDay(f.Day), f.Dish),
		said: func(text string, _ bool) bool {
			return lineWith(text, func(l string) bool { return (dishIn(l, f.Dish) || dayIn(l, f.Day)) && noCookWord.MatchString(l) })
		},
	}
}

func movedItem(f mustFact) mustItem {
	return mustItem{
		en: fmt.Sprintf("The %s is now cooked on %s at %s instead of %s at %s.", food.Quote(f.Dish), f.Day, f.Meal, f.Day2, f.Meal2),
		de: fmt.Sprintf("„%s“ wird jetzt am %s %s gekocht, nicht mehr am %s %s.", f.Dish, deDay(f.Day), deMealAt(f.Meal), deDay(f.Day2), deMealAt(f.Meal2)),
		said: func(text string, _ bool) bool {
			return lineWith(text, func(l string) bool { return dayIn(l, f.Day) && dayIn(l, f.Day2) })
		},
	}
}

var shortWord = regexp.MustCompile(`(?i)(only|just|no longer|not cooked|leftover|left over|nur|noch|nicht mehr|reste|aufgewärmt)`)

// shorterItem says a day's cooking that shrank: what it still cooks and what
// it no longer cooks.
func shorterItem(f mustFact) mustItem {
	var still, enGone, deGone []string
	for _, d := range f.Dishes {
		still = append(still, food.Quote(d))
	}
	deStill := make([]string, len(f.Dishes))
	for i, d := range f.Dishes {
		deStill[i] = "„" + d + "“"
	}
	var first string
	for _, g := range f.Gone {
		if first == "" {
			first = g.Dish
		}
		if g.Leftovers {
			enGone = append(enGone, fmt.Sprintf("the %s is now eaten as leftovers", food.Quote(g.Dish)))
			deGone = append(deGone, fmt.Sprintf("„%s“ gibt es als Reste", g.Dish))
		} else {
			enGone = append(enGone, fmt.Sprintf("the %s is no longer cooked", food.Quote(g.Dish)))
			deGone = append(deGone, fmt.Sprintf("„%s“ wird nicht mehr gekocht", g.Dish))
		}
	}
	return mustItem{
		en: fmt.Sprintf("%s at %s you now cook only %s: about %s in all (was %s); %s.", f.Day, f.Meal, strings.Join(still, ", "), f.Time, f.WasTime, strings.Join(enGone, "; ")),
		de: fmt.Sprintf("%s: nur noch %s kochen, etwa %s statt %s; %s.", deDay(f.Day), strings.Join(deStill, ", "), f.Time, f.WasTime, strings.Join(deGone, "; ")),
		said: func(text string, _ bool) bool {
			return lineWith(text, func(l string) bool { return dayIn(l, f.Day) && dishIn(l, first) && shortWord.MatchString(l) })
		},
	}
}

var smallWord = regexp.MustCompile(`(?i)(small|light|lighter|smaller|klein|leicht)`)

// smallItem says that the person added to a light meal eats the small one.
func smallItem(f mustFact) mustItem {
	at, ok := deMeal[f.Meal]
	if !ok {
		at = "zu dieser Mahlzeit"
	}
	de := map[string]string{"breakfast": "Frühstück", "lunch": "Mittagessen", "dinner": "Abendessen"}[f.Meal]
	if de == "" {
		de = "Mahlzeit"
	}
	return mustItem{
		en: fmt.Sprintf("%s %s: now for %s people; it is the small %s (%.0f kcal each).", f.Day, f.Meal, fmtNum(f.N), f.Meal, f.Kcal),
		de: fmt.Sprintf("%s %s: jetzt für %s Personen; es ist das kleine %s (%.0f kcal pro Person).", deDay(f.Day), at, fmtComma(f.N), de, f.Kcal),
		said: func(text string, _ bool) bool {
			return lineWith(text, func(l string) bool {
				return dayIn(l, f.Day) && (smallWord.MatchString(l) || numberIn(l, fmt.Sprint(int(f.Kcal+0.5))))
			})
		},
	}
}

var (
	keepWord = regexp.MustCompile(`(?i)(keeps?|kept|fridge|lasts?|good until|h[aä]lt\w*|reicht|kühlschrank)`)
	oddWord  = regexp.MustCompile(`(?i)(odd|strange|unusual|weird|rename|instruction|seltsam|merkwürdig|ungewöhnlich|komisch|umbenenn\w*|anweisung)`)
	nameWord = regexp.MustCompile(`(?i)(name|benenn|rezept|recipe)`)
)

func keepItem(f mustFact) mustItem {
	keep := fmt.Sprint(f.Keep)
	return mustItem{
		en: fmt.Sprintf("%s: the %s keeps %s in the fridge, so %s is the last day.", f.Day, food.Quote(f.Dish), countDays(f.Keep, false), f.Last),
		de: fmt.Sprintf("%s: „%s“ ist %s im Kühlschrank haltbar, letzter Tag ist %s.", deDay(f.Day), f.Dish, countDays(f.Keep, true), strings.TrimSuffix(deDay(f.Last), ".")),
		said: func(text string, _ bool) bool {
			return lineWith(text, func(l string) bool {
				return numberIn(l, keep) && keepWord.MatchString(l) && (dayIn(l, f.Day) || dayIn(l, f.Last) || dishIn(l, f.Dish))
			})
		},
	}
}

func tooOldItem(f mustFact) mustItem {
	keep := fmt.Sprint(f.Keep)
	return mustItem{
		en: fmt.Sprintf("%s: the %s would be too old by then; it keeps %s in the fridge.", f.Day, food.Quote(f.Dish), countDays(f.Keep, false)),
		de: fmt.Sprintf("%s: „%s“ wäre dann zu alt; es hält sich %s im Kühlschrank.", deDay(f.Day), f.Dish, countDays(f.Keep, true)),
		said: func(text string, _ bool) bool {
			return lineWith(text, func(l string) bool { return numberIn(l, keep) && keepWord.MatchString(l) })
		},
	}
}

func oddNameItem(f mustFact) mustItem {
	return mustItem{
		en: fmt.Sprintf("The recipe name %s looks odd, like an instruction. I did not act on it; you may want to rename it in the recipe list.", food.Quote(f.Dish)),
		de: fmt.Sprintf("Der Rezeptname „%s“ sieht seltsam aus, wie eine Anweisung. Ich habe nichts davon ausgeführt; du kannst ihn in der Rezeptliste umbenennen.", f.Dish),
		said: func(text string, _ bool) bool {
			return lineWith(text, func(l string) bool { return oddWord.MatchString(l) && nameWord.MatchString(l) })
		},
	}
}

// deDay renders "Fri 27 Nov" as "Freitag, 27. Nov."; an unknown name stays.
func deDay(label string) string {
	p := strings.Fields(label)
	if len(p) != 3 {
		return label
	}
	w, ok1 := deWeekday[p[0]]
	m, ok2 := deMonth[p[2]]
	if !ok1 || !ok2 {
		return label
	}
	return fmt.Sprintf("%s, %s. %s", w[0], p[1], m.abb)
}

func lineWith(text string, f func(line string) bool) bool {
	for _, l := range strings.Split(text, "\n") {
		if f(l) {
			return true
		}
	}
	return false
}

// dishIn reports a line that names the dish: any word of it, of four letters
// or more, at the start of a word.
func dishIn(line, dish string) bool {
	for _, w := range strings.Fields(dish) {
		w = strings.Trim(w, `"„“.,()`)
		if len([]rune(w)) < 4 {
			continue
		}
		if regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}])` + regexp.QuoteMeta(w)).MatchString(line) {
			return true
		}
	}
	return false
}

var (
	enWeekdayFull = map[string]string{"Mon": "monday", "Tue": "tuesday", "Wed": "wednesday", "Thu": "thursday", "Fri": "friday", "Sat": "saturday", "Sun": "sunday"}
	enWeekdayAlt  = map[string]string{"Mon": "Mon", "Tue": "Tues?", "Wed": "Wed", "Thu": "Thu(rs?)?", "Fri": "Fri", "Sat": "Sat", "Sun": "Sun"}
	deWeekdayAbb  = map[string]string{"Mon": "Mo", "Tue": "Di", "Wed": "Mi", "Thu": "Do", "Fri": "Fr", "Sat": "Sa", "Sun": "So"}
	// weekdayRes match a weekday in a line: the English name, a capitalised
	// English short form ("Sun"), the German name with any ending ("Sonntags",
	// "Sonntagabend"), or a German short form with its dot ("So.").
	weekdayRes = func() map[string]*regexp.Regexp {
		out := map[string]*regexp.Regexp{}
		for wd, full := range enWeekdayFull {
			de := strings.ToLower(deWeekday[wd][0])
			out[wd] = regexp.MustCompile(`(?i:\b` + full + `\b)|(?i:\b` + de + `\w*)|\b` + enWeekdayAlt[wd] + `\b|\b` + deWeekdayAbb[wd] + `\.`)
		}
		return out
	}()
)

// dayIn reports a day ("Fri 27 Nov") named in a line, by weekday or date.
func dayIn(line, label string) bool {
	p := strings.Fields(label)
	if len(p) != 3 {
		return strings.Contains(line, label)
	}
	wd, dd, mon := p[0], p[1], p[2]
	if _, ok := enWeekdayFull[wd]; !ok {
		return strings.Contains(line, label)
	}
	if weekdayRes[wd].MatchString(line) {
		return true
	}
	m := deMonth[mon]
	prefixes := regexp.QuoteMeta(mon) + `|` + regexp.QuoteMeta(strings.TrimSuffix(m.abb, "."))
	date := regexp.MustCompile(`(?i)\b` + dd + `(st|nd|rd|th)?\.? ?(` + prefixes + `)\w*|\b(` + prefixes + `)\w* ` + dd + `\b|\b` + dd + `\.\s?0?` + fmt.Sprint(m.num) + `\.`)
	return date.MatchString(line)
}

var numberWords = map[int][]string{
	2: {"two", "zwei"}, 3: {"three", "drei"}, 4: {"four", "vier"}, 5: {"five", "fünf"}, 6: {"six", "sechs"},
	7: {"seven", "sieben"}, 8: {"eight", "acht"}, 9: {"nine", "neun"}, 10: {"ten", "zehn"}, 11: {"eleven", "elf"}, 12: {"twelve", "zwölf"},
}

// thousands matches a thousands separator in "1,698" and "1.698".
var thousands = regexp.MustCompile(`(\d)[.,](\d{3})\b`)

// fractionWords are the words for the portion counts people say as words.
var fractionWords = map[string]string{
	"0.5":  `half|halb`,
	"0.25": `quarter|viertel`,
	"0.75": `three[- ]quarters|drei[- ]?viertel`,
	"1.5":  `one and a half|anderthalb|eineinhalb`,
}

// numberIn matches num as a whole number in text, either decimal mark, or as
// a number word (English or German) for a whole number from 2 to 12.
func numberIn(text, num string) bool { return digitsIn(text, num) || wordsIn(text, num) }

// digitsIn matches num as a whole number in text, either decimal mark.
func digitsIn(text, num string) bool {
	text = thousands.ReplaceAllString(text, "$1$2")
	re := regexp.MustCompile(`(^|[^\d.,])` + strings.Replace(regexp.QuoteMeta(num), `\.`, `[.,]`, 1) + `($|[^\d]|[.,]\D|[.,]$)`)
	return re.MatchString(text)
}

// wordsIn matches num as a number word, or a fraction word for a portion.
func wordsIn(text, num string) bool {
	if ws, ok := fractionWords[num]; ok && regexp.MustCompile(`(?i)\b(`+ws+`)`).MatchString(text) {
		return true
	}
	if n, err := strconv.Atoi(num); err == nil {
		for _, w := range numberWords[n] {
			if regexp.MustCompile(`(?i)\b` + w + `\b`).MatchString(text) {
				return true
			}
		}
	}
	return false
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }
