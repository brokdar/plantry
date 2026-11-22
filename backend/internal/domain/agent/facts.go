package agent

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// The facts block of a write (spec G4). Code owns the facts: every fact is
// computed from the plan before and after, rendered once in English and once in
// German, and put into the reply by code. The model only writes the framing.
//
// Classes, in the order they are told:
//
//	1 refused: the guard's refused or skipped part (uncapped, outside the cap)
//	2 changed: the changed meals, before to after
//	3 moved: what moved because of it (cooking, leftovers, guests, away energy)
//	4 worse: a day outside its range, a week figure that got worse
//	5 warnings: new warnings
//
// Classes 2 to 5 together show at most maxBlock lines.
const (
	classRefused = 1
	classChanged = 2
	classMoved   = 3
	classWorse   = 4
	classWarn    = 5

	maxBlock = 5
)

// Status of a fact: whether what it states is fine, short of its limit, past
// it, or could not be judged.
const (
	statusOK      = "ok"
	statusUnder   = "under"
	statusOver    = "over"
	statusUnknown = "unknown"
)

// Kinds of fact; the contradiction lint reads them.
const (
	kindMeal     = "meal"     // a changed meal
	kindAway     = "away"     // a day or meal away, with the energy the slot is assumed to hold
	kindProtein  = "protein"  // a day's protein moved
	kindMeat     = "meat"     // the meat that left a day
	kindDay      = "day"      // a day's kcal against its range
	kindWeek     = "week"     // a week figure
	kindCook     = "cook"     // a cooking time that changed
	kindLeftover = "leftover" // portions left over, short, resized or no longer cooked
	kindGuest    = "guest"    // a plate's people
	kindKeep     = "keep"     // how long a batch keeps
	kindOdd      = "odd"      // a recipe name that reads like an instruction
	kindWarn     = "warning"  // a new or worsened warning
	kindRefused  = "refused"  // a part the guard left out
)

// fact is one rendered line of one fact class.
type fact struct {
	Class  int    `json:"class"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
	EN     string `json:"en"`
	DE     string `json:"de"`
	// Key names the figure the line states ("day:Sat 21 Nov", "week:meat"): two
	// lines of one key are one fact, told once, in the earlier class.
	Key string `json:"key,omitempty"`
}

// lang is the language of a reply.
type lang int

const (
	langEN lang = iota
	langDE
)

// line is the fact in one language.
func (f fact) line(l lang) string {
	if l == langDE {
		return f.DE
	}
	return f.EN
}

// factSink collects the facts while a report is built. Nil drops them.
type factSink struct{ items []fact }

func (s *factSink) add(class int, kind, status, en, de string) {
	s.addKey("", class, kind, status, en, de)
}

// addKey adds a fact that states the figure key.
func (s *factSink) addKey(key string, class int, kind, status, en, de string) {
	if s == nil {
		return
	}
	en, de = strings.TrimSpace(en), strings.TrimSpace(de)
	if en == "" || de == "" {
		return
	}
	s.items = append(s.items, fact{Class: class, Kind: kind, Status: status, EN: en, DE: de, Key: key})
}

// emptyFact matches a line with nothing after its label ("Thu 26 Nov breakfast: ").
var emptyFact = regexp.MustCompile(`^([^:]*:)?\s*[.;,]*\s*$`)

// selectBlock is the facts a reply shows (a line is never cut: a fact is told whole or not at all): no empty line, no line twice, and of
// classes 2 to 5 at most maxBlock. When more were found the ones that got worse
// or are new warnings stay first, then the changed meals, then what moved; the
// result is in class order. Class 1 is never cut and comes first.
func selectBlock(all []fact) []fact {
	var refused, rest []fact
	seen := map[string]bool{}
	// One fact per figure: of several lines with one key the one of the earliest class stays.
	best := map[string]int{}
	for _, f := range all {
		if f.Key == "" {
			continue
		}
		if c, ok := best[f.Key]; !ok || f.Class < c {
			best[f.Key] = f.Class
		}
	}
	keyed := map[string]bool{}
	for _, f := range all {
		if emptyFact.MatchString(f.EN) || emptyFact.MatchString(f.DE) {
			continue
		}
		key := strings.ToLower(f.EN)
		if seen[key] {
			continue
		}
		seen[key] = true
		if f.Key != "" {
			if f.Class != best[f.Key] || keyed[f.Key] {
				continue
			}
			keyed[f.Key] = true
		}
		if f.Class == classRefused {
			refused = append(refused, f)
		} else {
			rest = append(rest, f)
		}
	}
	if len(rest) > maxBlock {
		// What the household must hear first when the block is cut: the meals
		// that changed and the cooking that moved (G4.2/G4.3; a day whose cooking
		// changed is always named), then the day outside its range or under its
		// minimum, the other worse figures, new warnings, and last the side
		// figures of a changed meal (protein, the day's moved target, meat).
		prio := func(f fact) int {
			switch {
			case f.Class == classChanged && (f.Kind == kindMeal || f.Kind == kindAway):
				return 0
			case f.Class == classMoved && (f.Kind == kindCook || f.Kind == kindLeftover):
				return 1
			case f.Class == classWorse && f.Kind == kindDay:
				return 2
			case f.Class == classWorse:
				return 3
			case f.Class == classWarn:
				return 4
			case f.Class == classMoved:
				return 5
			}
			return 6
		}
		keep := slices.Clone(rest)
		slices.SortStableFunc(keep, func(a, b fact) int { return prio(a) - prio(b) })
		keep = keep[:maxBlock]
		rest = slices.DeleteFunc(rest, func(f fact) bool { return !slices.Contains(keep, f) })
	}
	slices.SortStableFunc(rest, func(a, b fact) int { return a.Class - b.Class })
	return append(refused, rest...)
}

// tellOf is the English lines of a block (report.tell).
func tellOf(block []fact) []string {
	out := make([]string, len(block))
	for i, f := range block {
		out[i] = f.EN
	}
	return out
}

// Language of a reply: the language of the user's message. EN and DE only; a
// text that is neither (or a tie) is English.
var (
	englishWord = regexp.MustCompile(`(?i)\b(the|and|is|are|now|with|please|can|you|my|move|dinner|lunch|to|yes|no|instead|of|on|monday|tuesday|wednesday|thursday|friday|saturday|sunday)\b`)
	germanWord  = regexp.MustCompile(`(?i)\b(der|die|das|und|ist|wird|werden|mit|nicht|für|jetzt|auf|dem|den|bitte|kannst|du|ich|mein|meine|zum|zur|abendessen|mittagessen|frühstück|verschieb\w*|tausch\w*|leichter|mehr|weniger|ja|nein|statt|am|im|ein|eine|einen|nur|auch|noch|kein|keine|tag|woche|montag|dienstag|mittwoch|donnerstag|freitag|samstag|sonntag)\b|[äöüß]`)
)

// replyLangOf is the language of the first text that has more marker words of
// one language than of the other; false when none does (English then).
func replyLangOf(texts ...string) (lang, bool) {
	for _, t := range texts {
		en, de := len(englishWord.FindAllString(t, -1)), len(germanWord.FindAllString(t, -1))
		switch {
		case de > en:
			return langDE, true
		case en > de:
			return langEN, true
		}
	}
	return langEN, false
}

// German words and dates for the lines the code renders.

var deWeekday = map[string][2]string{
	"Mon": {"Montag", "Mo."}, "Tue": {"Dienstag", "Di."}, "Wed": {"Mittwoch", "Mi."},
	"Thu": {"Donnerstag", "Do."}, "Fri": {"Freitag", "Fr."}, "Sat": {"Samstag", "Sa."},
	"Sun": {"Sonntag", "So."},
}

// deMonth holds the German abbreviation per English month name.
var deMonth = map[string]string{
	"Jan": "Jan.", "Feb": "Feb.", "Mar": "Mär.", "Apr": "Apr.", "May": "Mai", "Jun": "Juni",
	"Jul": "Juli", "Aug": "Aug.", "Sep": "Sept.", "Oct": "Okt.", "Nov": "Nov.", "Dec": "Dez.",
}

// deDay renders "Fri 27 Nov" as "Freitag, 27. Nov."; an unknown label stays.
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
	return fmt.Sprintf("%s, %s. %s", w[0], p[1], m)
}

// deSlot is a meal name in German; a name of the household's own stays.
func deSlot(m string) string {
	switch strings.ToLower(m) {
	case "breakfast":
		return "Frühstück"
	case "lunch":
		return "Mittagessen"
	case "dinner":
		return "Abendessen"
	case "snack":
		return "Snack"
	}
	return m
}

// deCell is a cell label, "Fri 27 Nov dinner", as "Freitag, 27. Nov. Abendessen".
func deCell(en string) string {
	p := strings.Fields(en)
	if len(p) < 4 {
		return en
	}
	return deDay(strings.Join(p[:3], " ")) + " " + deSlot(strings.Join(p[3:], " "))
}

// deQuote is a dish name in German quotes.
func deQuote(name string) string {
	return "„" + strings.NewReplacer(`"`, "'", `\`, "/").Replace(food.Data(strings.Trim(name, `"„“`))) + "“"
}

// dot ends a rendered sentence with a full stop unless it ends on one already
// ("Freitag, 27. Nov." needs no second).
func dot(s string) string {
	if strings.HasSuffix(s, ".") {
		return s
	}
	return s + "."
}

// deQuoted turns the English quotes of a sentence into German ones.
var enQuoted = regexp.MustCompile(`"([^"\n]*)"`)

func deQuoted(s string) string { return enQuoted.ReplaceAllString(s, "„$1“") }

func fmtComma(v float64) string { return strings.Replace(fmtNum(v), ".", ",", 1) }

// dePortions says a portion count in German: "1 Portion", "1,5 Portionen".
func dePortions(v float64) string {
	if v > 0.995 && v < 1.005 {
		return "1 Portion"
	}
	return fmtComma(v) + " Portionen"
}

// deTimes says how often in German.
func deTimes(n int) string {
	switch n {
	case 0:
		return "gar nicht"
	case 1:
		return "einmal"
	}
	return fmt.Sprintf("%d Mal", n)
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

// deDuration says a cooking time in German.
func deDuration(min int) string {
	switch {
	case min <= 0:
		return "keine Zeit"
	case min < 60:
		return fmt.Sprintf("%d Min.", min)
	case min%60 == 0:
		return fmt.Sprintf("%d Std.", min/60)
	}
	return fmt.Sprintf("%d Std. %d Min.", min/60, min%60)
}
