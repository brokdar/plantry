package planning

import (
	"math"
	"slices"
	"time"
)

// Add-on rules (soft issue S9 add_on_excess). An add-on is a no-cook leaf on
// a plate that also holds a composed component (a plate component, not a
// child of a recipe): a snack or side the
// planner adds to a meal. The cap and the day limits are the heuristic's
// (dayAdds), so a heuristic plan never raises S9 and the options never
// create one.
const (
	// maxAddServings caps an add at this many servings of the food.
	maxAddServings = 2
	// maxAddOnPlates: the same add-on on more plates than this in the
	// window is a repeat.
	maxAddOnPlates = 3
	// dupMainGrams: an add-on already in the plate's main at this many
	// grams per portion duplicates it (tofu on the tofu stir-fry).
	dupMainGrams = 30
	// maxDayAddOns is the day's add-on limit: maxFillerAdds fillers plus
	// maxProteinAdds protein add. After the fact the two kinds cannot be told
	// apart, so S9 counts them together.
	maxDayAddOns = maxFillerAdds + maxProteinAdds
	// minLightAddKcal: an add-on under this many kcal per eater (raw
	// cucumber, tomato) does not weigh on a light meal.
	minLightAddKcal = 50
)

// S9 reasons (Params["reason"]).
const (
	AddOnLightMeal      = "light_meal"
	AddOnDuplicatesMain = "duplicates_main"
	AddOnOverCap        = "over_cap"
	AddOnDayLimit       = "day_limit"
	AddOnRepeated       = "repeated"
)

// addCapG is the most grams of f an add-on may hold on a day with this
// target: maxAddServings servings, maxAddFillerGrams, and fillerDayShare of
// the day's target (0: no target, no share cap). At least roundGrams.
func addCapG(f FoodFacts, dayTarget float64) float64 {
	return addCapKcal(f, dayTarget*fillerDayShare)
}

// addCapKcal is addCapG with the kcal budget given directly.
func addCapKcal(f FoodFacts, maxKcal float64) float64 {
	g := math.Min(maxAddServings*ServingGrams(f), maxAddFillerGrams)
	if maxKcal > 0 && f.Macros.Kcal > 0 {
		g = math.Min(g, math.Floor(maxKcal/f.Macros.Kcal*100/roundGrams)*roundGrams)
	}
	return math.Max(g, roundGrams)
}

// addOn is one no-cook leaf on a plate.
type addOn struct {
	plate Plate
	grams float64
	f     FoodFacts
}

// kcal is the add-on's energy per eater.
func (a addOn) kcal() float64 { return a.grams * a.f.Macros.Kcal / 100 }

// addOns lists the add-ons of the window's draft plates in plan order: a
// confirmed plate or one on a routine cell is the user's own and is never
// judged.
func (x *index) addOns() []addOn {
	var plates []Plate
	for _, p := range x.s.Plates {
		if p.Confirmed || !x.s.Window.Contains(p.Date) || x.routine(p.Cell()) {
			continue
		}
		plates = append(plates, p)
	}
	slices.SortStableFunc(plates, func(a, b Plate) int {
		if c := a.Date.Compare(b.Date); c != 0 {
			return c
		}
		return int(x.s.SlotOrder[a.SlotID]) - int(x.s.SlotOrder[b.SlotID])
	})
	var out []addOn
	for _, p := range plates {
		composed := false
		for _, c := range p.Components {
			if f, ok := x.food(c.FoodID); ok && f.Composed() {
				composed = true
			}
		}
		if !composed {
			continue // a plate of leaves only (yogurt, skyr) has no add-ons
		}
		for _, c := range p.Components {
			f, ok := x.food(c.FoodID)
			if !ok || f.Composed() || !f.NoCook || c.Grams == nil {
				continue
			}
			out = append(out, addOn{plate: p, grams: *c.Grams, f: f})
		}
	}
	return out
}

// addOnIssues reports S9 `add_on_excess` for every add-on that breaks one of
// the rules, with the first reason that applies: the plate is a light meal;
// the food is already in the plate's main (at least dupMainGrams per
// portion); it is over its cap (addCapG); it is past the day's limit of
// maxDayAddOns; or it is on more than maxAddOnPlates plates of the window.
// Params name the food and the reason; the issue sits on the plate.
func (x *index) addOnIssues() []Issue {
	adds := x.addOns()
	if len(adds) == 0 {
		return nil
	}
	targets := map[int]float64{}
	perDay := map[int]int{}
	var out []Issue
	for i := range adds {
		a := &adds[i]
		d := dayOf(a.plate.Date)
		if _, ok := targets[d]; !ok {
			for _, dk := range x.dayAccounts([]time.Time{a.plate.Date}) {
				targets[d] = dk.Adjusted
			}
		}
		perDay[d]++
		reason, limit := "", 0.0
		switch {
		case x.s.lightMeal(a.plate.Date, a.plate.SlotID) && a.kcal() >= minLightAddKcal:
			reason = AddOnLightMeal
		case x.duplicatesMain(a):
			reason = AddOnDuplicatesMain
		case a.grams > addCapG(a.f, targets[d])+1e-9:
			reason, limit = AddOnOverCap, addCapG(a.f, targets[d])
		case perDay[d] > maxDayAddOns:
			reason = AddOnDayLimit
		}
		if reason == "" && x.platesWith(adds[:i+1], a.f.ID) > maxAddOnPlates {
			reason = AddOnRepeated
		}
		if reason == "" {
			continue
		}
		is := issue("S9", CodeAddOnExcess, SeveritySoft).onPlate(a.plate).
			with("food_id", a.f.ID).with("name", a.f.Name).with("grams", math.Round(a.grams)).with("reason", reason)
		if limit > 0 {
			is = is.with("max_grams", limit)
		}
		out = append(out, is)
	}
	return out
}

// platesWith counts the distinct plates of adds that hold food id.
func (x *index) platesWith(adds []addOn, id int64) int {
	seen := map[[2]int64]bool{}
	for _, a := range adds {
		if a.f.ID == id {
			seen[[2]int64{int64(dayOf(a.plate.Date)), a.plate.SlotID}] = true
		}
	}
	return len(seen)
}

// duplicatesMain reports whether the add-on's food is a leaf of the plate's
// main with at least dupMainGrams per portion.
func (x *index) duplicatesMain(a *addOn) bool {
	main, ok := x.food(x.s.mainOf(a.plate))
	return ok && main.Leaves[a.f.ID] >= dupMainGrams
}
