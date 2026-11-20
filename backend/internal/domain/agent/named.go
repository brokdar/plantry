package agent

import (
	"fmt"
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
		if len(n) >= 4 && strings.Contains(msg, n) {
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
