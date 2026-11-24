package agent

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// namedFoods are the foods the user's message names (SF3): a food whose whole
// name (folded, at least 4 letters) is in the text and that no other food of
// that name shares. A name inside a longer matched name does not count
// ("Reis" in "Reispfanne" when only the pan is named). It is set from the
// request text, never from a model argument.
func namedFoods(text string, foods map[int64]planning.FoodFacts) map[int64]bool {
	msg := " " + diet.Fold(text) + " "
	byName := map[string][]int64{}
	for id, f := range foods {
		n := diet.Fold(f.Name)
		if len(n) >= 4 && containsWord(msg, n) {
			byName[n] = append(byName[n], id)
		}
	}
	out := map[int64]bool{}
	for n, ids := range byName {
		if len(ids) != 1 {
			continue
		}
		inner := false
		for other := range byName {
			if other != n && strings.Contains(other, n) {
				inner = true
				break
			}
		}
		if !inner {
			out[ids[0]] = true
		}
	}
	return out
}

// containsWord reports whether name is in msg as whole words: neither side is
// a letter or digit ("Reis" is not in "Reiswaffeln").
func containsWord(msg, name string) bool {
	for from := 0; ; {
		i := strings.Index(msg[from:], name)
		if i < 0 {
			return false
		}
		i += from
		end := i + len(name)
		if (i == 0 || !isWordByte(msg[i-1])) && (end == len(msg) || !isWordByte(msg[end])) {
			return true
		}
		from = i + 1
	}
}

func isWordByte(b byte) bool { return b >= 0x80 || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' }

var weekdayWords = [7][]string{
	time.Sunday: {"sunday", "sonntag"}, time.Monday: {"monday", "montag"}, time.Tuesday: {"tuesday", "dienstag"},
	time.Wednesday: {"wednesday", "mittwoch"}, time.Thursday: {"thursday", "donnerstag"},
	time.Friday: {"friday", "freitag"}, time.Saturday: {"saturday", "samstag"},
}

// mealWords are the words that name a slot, by the slot's name key.
var mealWords = map[string][]string{
	"breakfast": {"breakfast", "fruehstueck"},
	"lunch":     {"lunch", "mittagessen", "mittag"},
	"dinner":    {"dinner", "abendessen", "abendbrot", "supper"},
}

// namedCells are the meals the message names: a weekday of the window and a
// meal word together ("Thursday dinner", "Donnerstag Abendessen"), as
// "date|slot_id". A day alone or a meal alone names no cell. Set from the
// request text, never from a model argument (B-4: an op on a meal the user
// named is requested, not an offer).
func namedCells(text string, w window.Window, slotNames map[int64]string) map[string]bool {
	msg := " " + diet.Fold(text) + " "
	has := func(words []string) bool {
		for _, x := range words {
			if strings.Contains(msg, " "+x) {
				return true
			}
		}
		return false
	}
	var slots []int64
	for id, name := range slotNames {
		if words, ok := mealWords[strings.TrimPrefix(name, "slot.")]; ok && has(words) {
			slots = append(slots, id)
		}
	}
	out := map[string]bool{}
	if len(slots) == 0 {
		return out
	}
	for d := w.Start; !d.After(w.End); d = d.AddDate(0, 0, 1) {
		if has(weekdayWords[d.Weekday()]) {
			for _, s := range slots {
				out[fmt.Sprintf("%s|%d", d.Format(time.DateOnly), s)] = true
			}
		}
	}
	return out
}

// dayKcalNumber is a kcal number in a message with its unit: "600 kcal",
// "650 kalorien", "1,500 kcals", "2.000 kcal/day". The thousands separator is
// part of the number ("1,500" and "1.800" are 1500 and 1800).
var dayKcalNumber = regexp.MustCompile(`\b(\d{1,3}(?:[.,]\d{3})+|\d{1,4})\s*(?:kcals?|kilokalorien|kalorien|calories|cal)\b(\s*/\s*(?:day|tag))?`)

// eatNothing says a day of no food without a number ("gar nichts essen",
// "eat nothing"): a day of 0 kcal.
var eatNothing = regexp.MustCompile(`\b(?:eat (?:absolutely )?nothing|nothing to eat|(?:gar )?nichts (?:zu )?essen|nichts zu mir nehmen)\b`)

// awayStated says the message gives a reason the day has no meals at home:
// the user is away. Such a day may leave the plan (an away mark), and "eat
// nothing" then means no meal at home, not no food.
var awayStated = regexp.MustCompile(`\b(?:away|abwesend|verreis\w*|unterwegs|travel\w*|trip|urlaub|vacation|holiday|out of town|auswaerts|eat(?:ing)? out|not (?:at )?home|nicht (?:zu hause|daheim|da)|bin weg)\b`)

// kcalWord is a folded word run of a message (and "~").
var kcalWord = regexp.MustCompile(`[a-z0-9~]+`)

var (
	// absoluteCue before a kcal number says it is the day's target.
	absoluteCue = map[string]bool{
		"around": true, "about": true, "to": true, "only": true, "total": true, "auf": true, "etwa": true, "ca": true,
		"ungefaehr": true, "nur": true, "insgesamt": true, "approx": true, "approximately": true, "circa": true, "~": true,
	}
	// totalCue near a number says it is the whole day's, whatever meal is named.
	totalCue = map[string]bool{"total": true, "insgesamt": true, "gesamt": true, "day": true, "tag": true}
	// relativeWord next to a kcal number makes it an amount to add or take.
	relativeWord = map[string]bool{
		"by": true, "less": true, "more": true, "lighter": true, "heavier": true, "cut": true, "um": true, "weniger": true, "mehr": true,
		"leichter": true, "snack": true, "dessert": true, "side": true, "drink": true, "extra": true, "plus": true, "minus": true, "fewer": true,
		"under": true, "over": true, "below": true, "above": true, "unter": true, "ueber": true,
	}
)

func wordsOf(s string) []string { return kcalWord.FindAllString(s, -1) }

func lastWords(s string, n int) []string {
	w := wordsOf(s)
	return w[max(0, len(w)-n):]
}

func firstWords(s string, n int) []string {
	w := wordsOf(s)
	return w[:min(n, len(w))]
}

func anyIn(words []string, set map[string]bool) bool {
	return slices.ContainsFunc(words, func(w string) bool { return set[w] })
}

// shortWeekday are the abbreviations of the weekdays: three letters in
// English, two in German. A two-letter one counts only with a period or
// before a number ("Mo. 600 kcal", "So 1800 kcal"), since "so" and "do" are
// words.
var shortWeekday = map[string]time.Weekday{
	"mon": time.Monday, "tue": time.Tuesday, "tues": time.Tuesday, "wed": time.Wednesday, "thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday,
	"fri": time.Friday, "sat": time.Saturday, "sun": time.Sunday,
}

var shortWeekdayDE = map[string]time.Weekday{
	"mo": time.Monday, "di": time.Tuesday, "mi": time.Wednesday, "do": time.Thursday, "fr": time.Friday, "sa": time.Saturday, "so": time.Sunday,
}
var shortDayDE = regexp.MustCompile(`\b(mo|di|mi|do|fr|sa|so)(?:\.|\s+\d)`)

// namedDayKcal are the days whose energy the message sets to a number: one
// day (a weekday name or abbreviation of the window, or today/tomorrow/heute/
// morgen against today) and one kcal number that is the day's absolute target:
// after a cue word ("around", "to", "only", "total", "auf", "etwa", "ca.",
// "ungefähr", "nur", "insgesamt") or right after the day ("Saturday 600
// kcal"), and no relative amount ("by", "less", "more", "cut", "um", "weniger",
// "mehr", "leichter", a snack, dessert, side, drink or extra), as date → kcal.
// A meal word makes the number the meal's unless a cue says it is the day's
// total ("Saturday ~600 kcal total"). Two days or two kcal numbers are not
// read: it stays nil, so a guess never blocks a legitimate write. Set from the
// request text, never from a model argument (C-11: a day number under the
// day's minimum changes nothing, whichever op the model picks).
func namedDayKcal(text string, w window.Window, today time.Time) map[string]float64 {
	msg := " " + diet.Fold(text) + " "
	nums := dayKcalNumber.FindAllStringSubmatchIndex(msg, -1)
	if len(nums) == 0 && eatNothing.MatchString(msg) && !awayStated.MatchString(msg) && !namesMeal(msg) {
		if days := daysNamed(msg, w, today); len(days) == 1 {
			for d := range days {
				return map[string]float64{d: 0}
			}
		}
		return nil
	}
	if len(nums) != 1 {
		return nil
	}
	m := nums[0]
	before, after := msg[:m[0]], msg[m[1]:]
	if anyIn(lastWords(before, 3), relativeWord) || anyIn(firstWords(after, 2), relativeWord) {
		return nil
	}
	day := daysNamed(msg, w, today)
	if len(day) != 1 {
		return nil
	}
	total := m[4] >= 0 || anyIn(firstWords(after, 3), totalCue) || anyIn(lastWords(before, 2), totalCue)
	if !total && namesMeal(msg) {
		return nil
	}
	prev := lastWords(before, 2)
	dayBefore := len(prev) > 0 && isDayWord(prev[len(prev)-1])
	if !total && !anyIn(prev, absoluteCue) && !dayBefore {
		return nil
	}
	n, err := strconv.ParseFloat(strings.NewReplacer(".", "", ",", "").Replace(msg[m[2]:m[3]]), 64)
	if err != nil || n == 0 && awayStated.MatchString(msg) {
		return nil // "0 kcal" while away is no meal at home, not a day without food
	}
	for d := range day {
		return map[string]float64{d: n}
	}
	return nil
}

// namesMeal reports a folded message that has a meal word.
func namesMeal(msg string) bool {
	for _, words := range mealWords {
		for _, x := range words {
			if strings.Contains(msg, " "+x) {
				return true
			}
		}
	}
	return false
}

// daysNamed are the days of the window a folded message names: a weekday name
// or abbreviation, today or tomorrow against today.
func daysNamed(msg string, w window.Window, today time.Time) map[string]bool {
	day := map[string]bool{}
	for d := w.Start; !d.After(w.End); d = d.AddDate(0, 0, 1) {
		for _, x := range weekdayWords[d.Weekday()] {
			if strings.Contains(msg, " "+x) {
				day[d.Format(time.DateOnly)] = true
			}
		}
	}
	for _, wd := range wordsOf(msg) {
		for d := w.Start; !d.After(w.End); d = d.AddDate(0, 0, 1) {
			if sd, ok := shortWeekday[wd]; ok && sd == d.Weekday() {
				day[d.Format(time.DateOnly)] = true
			}
		}
		if !today.IsZero() {
			rel := map[string]int{"today": 0, "heute": 0, "tomorrow": 1, "morgen": 1}
			if n, ok := rel[wd]; ok {
				if d := today.AddDate(0, 0, n); !d.Before(w.Start) && !d.After(w.End) {
					day[d.Format(time.DateOnly)] = true
				}
			}
		}
	}
	for _, sm := range shortDayDE.FindAllStringSubmatch(msg, -1) {
		for d := w.Start; !d.After(w.End); d = d.AddDate(0, 0, 1) {
			if shortWeekdayDE[sm[1]] == d.Weekday() {
				day[d.Format(time.DateOnly)] = true
			}
		}
	}
	return day
}

// isDayWord reports a folded word that names a day: a weekday (full, or
// abbreviated), today or tomorrow.
func isDayWord(w string) bool {
	for _, names := range weekdayWords {
		for _, x := range names {
			if strings.HasPrefix(w, x) {
				return true
			}
		}
	}
	_, short := shortWeekday[w]
	_, shortDE := shortWeekdayDE[w]
	return short || shortDE || w == "today" || w == "heute" || w == "tomorrow" || w == "morgen"
}
