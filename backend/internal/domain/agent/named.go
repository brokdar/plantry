package agent

import (
	"fmt"
	"regexp"
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

// dayKcalNumber is a kcal number in a message ("600 kcal", "650 kalorien").
var dayKcalNumber = regexp.MustCompile(`\b(\d{3,4})\s*(?:kcal|kilokalorien|kalorien|calories|cal)\b`)

// namedDayKcal are the days whose energy the message sets to a number: one
// weekday of the window and one kcal number, and no meal word ("Saturday
// around 600 kcal", "Dienstag etwa 650 kcal"), as date → kcal. A meal word
// makes the number the meal's. Set from the request text, never from a model
// argument (C-11: a day number under the day's minimum changes nothing,
// whichever op the model picks).
func namedDayKcal(text string, w window.Window) map[string]float64 {
	msg := " " + diet.Fold(text) + " "
	for _, words := range mealWords {
		for _, x := range words {
			if strings.Contains(msg, " "+x) {
				return nil
			}
		}
	}
	nums := dayKcalNumber.FindAllStringSubmatch(msg, -1)
	if len(nums) != 1 {
		return nil
	}
	var days []string
	for d := w.Start; !d.After(w.End); d = d.AddDate(0, 0, 1) {
		for _, x := range weekdayWords[d.Weekday()] {
			if strings.Contains(msg, " "+x) {
				days = append(days, d.Format(time.DateOnly))
				break
			}
		}
	}
	if len(days) != 1 {
		return nil
	}
	n, err := strconv.ParseFloat(nums[0][1], 64)
	if err != nil {
		return nil
	}
	return map[string]float64{days[0]: n}
}
