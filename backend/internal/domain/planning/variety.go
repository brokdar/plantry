package planning

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Dish families: "Turkey chili", "Chili con carne" and "Chili sin carne" are
// one family. A day regenerate must not put a dish of a neighbouring day's
// family next to it, nor two dishes of one family on the same day, when the
// library has other mains (soft: a tie-break of the heuristic, an advisory for
// the model, a line in the week effects of the result).

// familyStop are the name words that make no family: dish types, cooking
// words and prepositions in the languages of the library.
var familyStop = map[string]bool{
	"carne": true, "with": true, "mit": true, "aus": true, "dem": true, "und": true, "from": true, "cooker": true,
	"slow": true, "easy": true, "baked": true, "fried": true, "mixed": true, "salad": true, "salat": true, "soup": true,
	"suppe": true, "pasta": true, "rice": true, "reis": true, "stew": true, "eintopf": true, "bowl": true, "pfanne": true,
}

// familyWords are the words of a dish name that identify its family.
func familyWords(name string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(name), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if len([]rune(w)) >= 5 && !familyStop[w] {
			out = append(out, w)
		}
	}
	return out
}

// sameFamily reports whether two dish names share a family word.
func sameFamily(a, b string) bool {
	wb := familyWords(b)
	return slices.ContainsFunc(familyWords(a), func(w string) bool { return slices.Contains(wb, w) })
}

// mainCell is a lunch or dinner main: its date, slot and food.
type mainCell struct {
	date   time.Time
	slot   int64
	foodID int64
}

// weekMains lists the mains of the full-meal plates of s's window.
func (x *index) weekMains() []mainCell {
	var out []mainCell
	for _, p := range x.s.Plates {
		if !x.s.Window.Contains(p.Date) || x.mealOf(p.SlotID) != mealFull {
			continue
		}
		if id := x.s.mainOf(p); id != 0 {
			out = append(out, mainCell{p.Date, p.SlotID, id})
		}
	}
	return out
}

// familyRepeats names the repeats of the window's plan in plain words: the
// same dish at lunch and dinner of a day, and dishes of one family on the same
// or adjacent days that are not the same batch (a batch's leftovers are meant).
func (x *index) familyRepeats() []string {
	mains := x.weekMains()
	slices.SortFunc(mains, func(a, b mainCell) int {
		if c := a.date.Compare(b.date); c != 0 {
			return c
		}
		return x.s.SlotOrder[a.slot] - x.s.SlotOrder[b.slot]
	})
	var out []string
	seen := map[string]bool{}
	add := func(a, b mainCell) {
		na, nb := foodName(x.s, a.foodID), foodName(x.s, b.foodID)
		line := fmt.Sprintf("%s %s and %s %s: %s / %s", a.date.Weekday(), slotName(x.s, a.slot), b.date.Weekday(), slotName(x.s, b.slot), na, nb)
		if !seen[line] {
			seen[line] = true
			out = append(out, line)
		}
	}
	for i, a := range mains {
		for _, b := range mains[i+1:] {
			gap := dayOf(b.date) - dayOf(a.date)
			if b.date.Sub(a.date) > 24*time.Hour || gap < 0 {
				continue
			}
			switch {
			case a.foodID == b.foodID && gap == 0:
				add(a, b)
			case a.foodID != b.foodID && sameFamily(foodName(x.s, a.foodID), foodName(x.s, b.foodID)):
				add(a, b)
			}
		}
	}
	return out
}

// nearFamily reports whether a main of f's family sits on the cell's day or
// an adjacent one in the fixed plates or the proposal so far (the same dish
// on an adjacent day is a batch's leftover, not a repeat), or the same dish
// on the cell's own day.
func (h *planner) nearFamily(f FoodFacts, c plate.CellKey) bool {
	near := func(d time.Time, slot int64, id int64) bool {
		if d.Sub(c.Date) > 24*time.Hour || c.Date.Sub(d) > 24*time.Hour || (dayOf(d) == dayOf(c.Date) && slot == c.SlotID) {
			return false
		}
		if id == f.ID {
			return dayOf(d) == dayOf(c.Date)
		}
		return sameFamily(f.Name, foodName(h.s, id))
	}
	for _, p := range h.s.Plates {
		if id := h.s.mainOf(p); id != 0 && h.s.Window.Contains(p.Date) && near(p.Date, p.SlotID, id) {
			return true
		}
	}
	for _, p := range h.prop.Plates {
		for _, comp := range p.Components {
			if ff, ok := h.s.Foods[comp.FoodID]; ok && ff.Composed() && ff.Role == food.RoleMain && near(p.Date, p.SlotID, comp.FoodID) {
				return true
			}
		}
	}
	return false
}

// WeekEffect is what a plan change did to the week it sits in: the meat of
// the window against its limit, and the repeats the plan holds.
type WeekEffect struct {
	MeatG   float64
	LimitG  float64
	Repeats []string
}

// WeekEffectOf reads the window of s as persisted. LimitG is 0 when the meat
// rule does not apply (a diet without meat, fewer than five planned days).
func WeekEffectOf(s State) WeekEffect {
	x := newIndex(s, WindowTarget(s))
	var w WeekEffect
	for _, p := range s.Plates {
		if s.Window.Contains(p.Date) {
			w.MeatG += x.plateGroups(p).MeatG
		}
	}
	if b, ok := MeatBudgetOf(s, Target{}); ok {
		w.LimitG = b.LimitG
	}
	w.Repeats = x.familyRepeats()
	return w
}
