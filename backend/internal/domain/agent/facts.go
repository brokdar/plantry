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
// Classes 2 to 5 together show at most maxBlock lines, besides a day under its
// minimum and what moved (movedTold), which are always told.
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

// movedTold are the kinds of class 3 (what moved) the reply always tells,
// outside the cap of maxBlock (spec v1.5 G4.3, V63 b): cooking now elsewhere,
// leftovers moved or resized, guest portions, an away slot's assumed energy.
var movedTold = map[string]bool{kindCook: true, kindLeftover: true, kindGuest: true, kindAway: true}

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
	// Must marks a line the reply always tells, outside the cap of classes 2
	// to 5: a day the change left under its minimum (SF1(c), G4.4).
	Must bool `json:"must,omitempty"`
	// Day, on a leftover line that begins "<day>: ", lets the lines of one day
	// merge into one (G4).
	Day string `json:"day,omitempty"`
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

// addDay adds a leftover line that begins "<day>: ", so it can merge with the
// others of its day.
func (s *factSink) addDay(day string, class int, kind, status, en, de string) {
	if s == nil {
		return
	}
	n := len(s.items)
	s.add(class, kind, status, en, de)
	if len(s.items) > n {
		s.items[n].Day = day
	}
}

// mergeDays joins the leftover lines of one day into one line ("Wed 2 Dec: a; b."),
// at the place of the first.
func mergeDays(told []fact) []fact {
	var out []fact
	at := map[string]int{}
	for _, f := range told {
		if f.Day == "" {
			out = append(out, f)
			continue
		}
		i, ok := at[f.Day]
		if !ok {
			at[f.Day] = len(out)
			out = append(out, f)
			continue
		}
		out[i].EN = strings.TrimSuffix(out[i].EN, ".") + "; " + strings.TrimPrefix(f.EN, f.Day+": ")
		out[i].DE = strings.TrimSuffix(out[i].DE, ".") + "; " + strings.TrimPrefix(f.DE, deDay(f.Day)+": ")
	}
	return out
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

// addMust adds a must line (always told) for the figure key.
func (s *factSink) addMust(key string, class int, kind, status, en, de string) {
	if s == nil {
		return
	}
	n := len(s.items)
	s.addKey(key, class, kind, status, en, de)
	if len(s.items) > n {
		s.items[n].Must = true
	}
}

// emptyFact matches a line with nothing after its label ("Thu 26 Nov breakfast: ").
var emptyFact = regexp.MustCompile(`^([^:]*:)?\s*[.;,]*\s*$`)

// selectBlock is the facts a reply shows (a line is never cut: a fact is told
// whole or not at all): no empty line, no line twice. The must lines (a day under
// its minimum) and the what-moved lines (movedTold: cooking, leftovers, guests,
// a day's meals out) are never cut; leftover lines of one day are merged into one
// line (mergeDays), and buildFacts already merges a day's meals out, assumed
// energy and "nothing is cooked" into one. Of the rest (classes 2 to 5) at most
// maxBlock stay: the changed meals first (at least one always keeps its place),
// then the day outside its range, other worse figures, new warnings and the side
// figures. The result is in class order; class 1 is never cut and comes first.
func selectBlock(all []fact) []fact {
	var refused, must, rest []fact
	seen := map[string]bool{}
	// One fact per figure: of several lines with one key the one of the earliest class stays.
	best := map[string]int{}
	for _, f := range all {
		if f.Key == "" {
			continue
		}
		c := f.Class
		if f.Must {
			c = 0 // a must line is the one of its figure that stays
		}
		if b, ok := best[f.Key]; !ok || c < b {
			best[f.Key] = c
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
			c := f.Class
			if f.Must {
				c = 0
			}
			if c != best[f.Key] || keyed[f.Key] {
				continue
			}
			keyed[f.Key] = true
		}
		switch {
		case f.Class == classRefused:
			refused = append(refused, f)
		case f.Must:
			must = append(must, f)
		default:
			rest = append(rest, f)
		}
	}
	// spec v1.5 G4 (V63 b): what the household must know is never cut. A day
	// under its minimum (a must line) and every line of what moved (cooking now
	// elsewhere, leftovers, guests, an away slot's energy) are told outside the
	// cap, like the refused part. The cap of maxBlock holds for the rest:
	// changed meals, worse figures, new warnings and the other side figures.
	var told []fact
	rest = slices.DeleteFunc(rest, func(f fact) bool {
		if f.Class == classMoved && movedTold[f.Kind] {
			told = append(told, f)
			return true
		}
		return false
	})
	if len(rest) > maxBlock {
		// What stays when the rest is cut: the meals that changed (at least one
		// always keeps its place), the day outside its range, the other worse
		// figures, new warnings, and last the side figures of a change.
		prio := func(f fact) int {
			switch {
			case f.Class == classChanged && (f.Kind == kindMeal || f.Kind == kindAway):
				return 0
			case f.Class == classWorse && f.Kind == kindDay:
				return 1
			case f.Class == classWorse:
				return 2
			case f.Class == classWarn:
				return 3
			}
			return 4
		}
		keep := slices.Clone(rest)
		slices.SortStableFunc(keep, func(a, b fact) int { return prio(a) - prio(b) })
		keep = keep[:maxBlock]
		rest = slices.DeleteFunc(rest, func(f fact) bool { return !slices.Contains(keep, f) })
	}
	rest = append(rest, mergeDays(told)...)
	rest = append(rest, must...)
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
