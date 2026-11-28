package planning

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// fixQuick keeps a wish's time limit (State.MaxMinutes, V72) after the
// finishing steps, which may swap a dish for less meat or more variety: a
// proposed batch cooked in a limited cell with a dish over its minutes is
// cooked, batch by batch, as a quick main instead (not in the plan, not
// recent, closest in energy, same cook cell). The plan after it must break no
// rule it did not break before, repeat no more dishes and cook on no more days.
// A batch no quick main can replace stays (the plan keeps the slow dish).
func fixQuick(in ChooseInput, prop Proposal) (Proposal, []string) {
	if len(in.State.MaxMinutes) == 0 {
		return prop, nil
	}
	// A quick swap never brings back a food the run avoids or the user removed.
	skip := idSet(slices.Concat(in.Exclude, in.Avoid, in.AvoidBatches))
	for id, on := range in.State.Avoid {
		if on {
			skip[id] = true
		}
	}
	var notes []string
	tried := map[string]bool{}
	for range maxGuardSteps {
		key, limit := slowBatch(in.State, prop, tried)
		if key == "" {
			break
		}
		tried[key] = true
		if next, note, ok := quickFor(in, prop, key, limit, skip); ok {
			prop = next
			notes = append(notes, note)
		}
	}
	prop, sideNotes := fixQuickSides(in, prop, skip)
	return prop, append(notes, sideNotes...)
}

// fixQuickBalanced is fixQuick followed, when it changed the plan, by the
// balance pass: a side left out or a dish swapped for the limit takes its kcal
// with it, and the days it left under their band are lifted again within the
// limit (R6, planner-13: Tuesday at -24 %).
func fixQuickBalanced(in ChooseInput, prop Proposal) (Proposal, []string) {
	prop, notes := fixQuick(in, prop)
	if len(notes) == 0 {
		return prop, nil
	}
	prop, balNotes := balancePlan(in, prop)
	return prop, append(notes, balNotes...)
}

// fixQuickSides keeps a wish's limit for the whole cooking time of a cell:
// where the sides cooked there (put in by plate fit, add-ons or a swap) take
// it past the limit, the longest side is replaced by one that fits (not in the
// plan, closest in energy), else left out. A change that breaks a hard rule
// the plan did not break before is not made.
func fixQuickSides(in ChooseInput, prop Proposal, skip map[int64]bool) (Proposal, []string) {
	var notes []string
	tried := map[string]bool{}
	for range maxGuardSteps {
		key, limit := overSideCell(in, prop, tried)
		if key == "" {
			break
		}
		tried[key] = true
		if next, note, ok := quickSide(in, prop, key, limit, skip); ok {
			prop = next
			notes = append(notes, note)
		}
	}
	return prop, notes
}

// overSideCell is the longest side session (by key) of the first limited cook
// cell whose whole cooking time is over its limit, and that limit.
func overSideCell(in ChooseInput, prop Proposal, tried map[string]bool) (string, int) {
	merged := Merge(in.State, in.Target, prop)
	for _, ss := range prop.Sessions {
		cell := plate.CellKey{Date: ss.CookDate, SlotID: ss.CookSlotID}
		limit, ok := merged.MaxMinutes[idOf(cell)]
		if !ok || merged.cellMinutes(cell, 0) <= limit {
			continue
		}
		best, bestMin := "", 0
		for _, o := range prop.Sessions {
			f := merged.Foods[o.FoodID]
			if o.CookSlotID != cell.SlotID || dayOf(o.CookDate) != dayOf(cell.Date) || tried[o.Key] || f.Role == food.RoleMain || !f.Composed() || f.Minutes <= bestMin {
				continue
			}
			best, bestMin = o.Key, f.Minutes
		}
		if best != "" {
			return best, limit
		}
	}
	return "", 0
}

// quickSide replaces side session key by a side that fits the cell's limit, or
// drops it.
func quickSide(in ChooseInput, prop Proposal, key string, limit int, skip map[int64]bool) (Proposal, string, bool) {
	si := slices.IndexFunc(prop.Sessions, func(s ProposedSession) bool { return s.Key == key })
	if si < 0 {
		return prop, "", false
	}
	merged := Merge(in.State, in.Target, prop)
	old := merged.Foods[prop.Sessions[si].FoodID]
	cell := plate.CellKey{Date: prop.Sessions[si].CookDate, SlotID: prop.Sessions[si].CookSlotID}
	room := limit - (merged.cellMinutes(cell, 0) - old.Minutes)
	before := evaluateLean(merged, in.Target)
	hard := hardKeys(before.Issues)
	inPlan := map[int64]bool{}
	for _, ss := range prop.Sessions {
		inPlan[merged.dish(ss.FoodID)] = true
	}
	household := merged.Profile.DietType
	var cands []FoodFacts
	for id, f := range merged.Foods {
		if skip[id] || inPlan[merged.dish(id)] || !f.Composed() || f.Archived || f.Never || !sideRole(f.Role) || f.Role != old.Role || f.Minutes <= 0 || f.Minutes > room ||
			f.Macros.Kcal <= 0 || !f.NutritionComplete || merged.RatherNot[id] || (DietRestricts(household) && !fitsDiet(f.DietClass, diet.Class(household))) {
			continue
		}
		cands = append(cands, f)
	}
	slices.SortStableFunc(cands, func(a, b FoodFacts) int {
		return cmp.Or(cmp.Compare(math.Abs(a.Macros.Kcal-old.Macros.Kcal), math.Abs(b.Macros.Kcal-old.Macros.Kcal)), cmp.Compare(a.ID, b.ID))
	})
	cookDate := cell.Date.Format(time.DateOnly)
	// A day already under its kcal floor takes no side that lowers its kcal.
	underFloor := slices.ContainsFunc(before.Issues, func(is Issue) bool {
		return is.Code == CodeKcalBelowFloor && is.Severity == SeverityViolation && is.Date == cookDate
	})
	for tries, f := range append(cands, FoodFacts{}) {
		if tries >= maxGuardTries*3 {
			break
		}
		if underFloor && f.Macros.Kcal < old.Macros.Kcal {
			continue
		}
		next := cloneProposal(prop)
		name := "left out"
		if f.ID != 0 {
			next.Sessions[si].FoodID = f.ID
			for pi := range next.Plates {
				for ci, c := range next.Plates[pi].Components {
					if c.SessionKey == key {
						next.Plates[pi].Components[ci].FoodID = f.ID
					}
				}
			}
			name = fmt.Sprintf("replaced by %s (%d min)", f.Name, f.Minutes)
		} else {
			next.Sessions = slices.Delete(next.Sessions, si, si+1)
			for pi := range next.Plates {
				next.Plates[pi].Components = slices.DeleteFunc(next.Plates[pi].Components, func(c ProposedComponent) bool { return c.SessionKey == key })
			}
		}
		after := evaluateLean(Merge(in.State, in.Target, next), in.Target)
		if newHard(hard, after) != "" {
			continue
		}
		return next, fmt.Sprintf("Quick meals: side %s (%d min) %s on %s, the cooking time stays within %d min.", old.Name, old.Minutes, name, cookDate, limit), true
	}
	return prop, "", false
}

// slowBatch is the first proposed batch (by key) cooked in a limited cell
// with a dish over its minutes, and that cell's limit. A leftover costs no
// time at the meal that eats it, so only the cook cell is judged.
func slowBatch(s State, prop Proposal, tried map[string]bool) (string, int) {
	for _, ss := range prop.Sessions {
		cell := plate.CellKey{Date: ss.CookDate, SlotID: ss.CookSlotID}
		limit, ok := s.MaxMinutes[idOf(cell)]
		if !ok || tried[ss.Key] || !s.tooSlow(s.Foods[ss.FoodID], cell) {
			continue
		}
		return ss.Key, limit
	}
	return "", 0
}

// quickFor cooks batch key as a quick main (at most limit minutes).
func quickFor(in ChooseInput, prop Proposal, key string, limit int, skip map[int64]bool) (Proposal, string, bool) {
	si := slices.IndexFunc(prop.Sessions, func(s ProposedSession) bool { return s.Key == key })
	if si < 0 {
		return prop, "", false
	}
	merged := Merge(in.State, in.Target, prop)
	old := merged.Foods[prop.Sessions[si].FoodID]
	before := evaluateLean(merged, in.Target)
	hard := hardKeys(before.Issues)
	span := 7 * merged.Settings.VarietyWeeks
	cookDate := prop.Sessions[si].CookDate
	inPlan := map[int64]bool{}
	for _, ss := range prop.Sessions {
		inPlan[merged.dish(ss.FoodID)] = true
	}
	for _, p := range merged.Plates {
		if p.Confirmed && merged.Window.Contains(p.Date) {
			for _, c := range p.Components {
				inPlan[merged.dish(c.FoodID)] = true
			}
		}
	}
	household := merged.Profile.DietType
	var cands []FoodFacts
	for id, f := range merged.Foods {
		if skip[id] || inPlan[merged.dish(id)] || !f.Composed() || f.Archived || f.Never || !varietyRole(f.Role) || !isMain(f.Role, mealFull) || f.Macros.Kcal <= 0 || !f.NutritionComplete ||
			f.Minutes <= 0 || f.Minutes > limit || (f.Role != old.Role && f.Role != food.RoleMain) ||
			(DietRestricts(household) && !fitsDiet(f.DietClass, diet.Class(household))) {
			continue
		}
		if le := f.LastEaten; le != nil && span > 0 && !le.Before(cookDate.AddDate(0, 0, -span)) {
			continue
		}
		cands = append(cands, f)
	}
	slices.SortStableFunc(cands, func(a, b FoodFacts) int {
		return cmp.Or(cmp.Compare(b2i(a.Groups.MeatG > old.Groups.MeatG), b2i(b.Groups.MeatG > old.Groups.MeatG)),
			cmp.Compare(math.Abs(a.Macros.Kcal-old.Macros.Kcal), math.Abs(b.Macros.Kcal-old.Macros.Kcal)), cmp.Compare(a.ID, b.ID))
	})
	repeats := h8Count(before)
	for tries, f := range cands {
		if tries >= maxGuardTries*3 {
			break
		}
		next := cloneProposal(prop)
		next.Sessions[si].FoodID = f.ID
		for pi := range next.Plates {
			for ci, c := range next.Plates[pi].Components {
				if c.SessionKey != key {
					continue
				}
				p := clamp(math.Round(old.Macros.Kcal*c.Portions/f.Macros.Kcal/roundPortions)*roundPortions, optPortionMin, MaxDishPortions)
				if merged.lightMeal(next.Plates[pi].Date, next.Plates[pi].SlotID) {
					p = clamp(math.Floor(old.Macros.Kcal*c.Portions/f.Macros.Kcal/roundPortions)*roundPortions, optPortionMin, MaxDishPortions)
				}
				next.Sessions[si].Portions += next.Plates[pi].eaters(merged) * (p - c.Portions)
				next.Plates[pi].Components[ci].FoodID, next.Plates[pi].Components[ci].Portions = f.ID, p
			}
		}
		after := evaluateLean(Merge(in.State, in.Target, next), in.Target)
		if newHard(hard, after) != "" || h8Count(after) > repeats || cookDays(after) > cookDays(before) {
			continue
		}
		return next, fmt.Sprintf("Quick meals: %s (%d min) replaced by %s (%d min) on %s.", old.Name, old.Minutes, f.Name, f.Minutes, cookDate.Format(time.DateOnly)), true
	}
	return prop, "", false
}
