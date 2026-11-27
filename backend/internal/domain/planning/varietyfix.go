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

// fixVariety settles H8 in the preview the way the guards settle the week's
// meat: a recipe the plan cooks within the variety weeks of its last meal (or
// of another cook in the plan) is replaced, batch by batch, by a main that is
// not recent and not in the plan yet, closest in energy, on the same cook
// cell. The plan after it must break no rule it did not break before. A repeat
// no candidate can replace stays for the model, with the suggestion of
// varietyHints.
func fixVariety(in ChooseInput, prop Proposal) (Proposal, []string) {
	span := 7 * in.State.Settings.VarietyWeeks
	if span <= 0 || len(prop.Plates) == 0 {
		return prop, nil
	}
	skip := idSet(in.Exclude)
	var notes []string
	for range maxGuardSteps {
		merged := Merge(in.State, in.Target, prop)
		ev := evaluateLean(merged, in.Target)
		var bad *Issue
		for i, is := range ev.Issues {
			if is.Code == CodeVarietyRepeat && is.Severity == SeverityViolation {
				bad = &ev.Issues[i]
				break
			}
		}
		if bad == nil {
			break
		}
		hard := hardKeys(ev.Issues)
		next, note, ok := variantFor(in, merged, prop, *bad, skip, hard, ev)
		if !ok {
			// This repeat has no replacement: look no further (the model gets the hint).
			break
		}
		prop = next
		notes = append(notes, note)
	}
	return prop, notes
}

// variantFor replaces the recipe of one H8 issue by a verified candidate.
func variantFor(in ChooseInput, merged State, prop Proposal, is Issue, skip map[int64]bool, hard map[string]bool, before Evaluation) (Proposal, string, bool) {
	oldID := int64(anyF(is.Params["food_id"]))
	old, ok := merged.Foods[oldID]
	if !ok {
		return prop, "", false
	}
	key, _ := is.Params["session_key"].(string)
	span := 7 * merged.Settings.VarietyWeeks
	cookDate, err := time.Parse(time.DateOnly, is.Date)
	if err != nil {
		return prop, "", false
	}
	inPlan := map[int64]bool{}
	for _, ss := range prop.Sessions {
		inPlan[merged.dish(ss.FoodID)] = true
	}
	for _, pl := range prop.Plates {
		for _, c := range pl.Components {
			if c.SessionKey == "" && c.SessionID == 0 && c.Portions > 0 {
				inPlan[merged.dish(c.FoodID)] = true
			}
		}
	}
	for _, p := range merged.Plates {
		if p.Confirmed && merged.Window.Contains(p.Date) {
			for _, c := range p.Components {
				inPlan[merged.dish(c.FoodID)] = true
			}
		}
	}
	household := merged.Profile.DietType
	// A wish's time limit and GATE-2 hold for the swap: a batch is judged
	// where it cooks, a single meal where it is eaten.
	cell := plate.CellKey{Date: cookDate}
	if is.SlotID != nil {
		cell.SlotID = *is.SlotID
	}
	if si := slices.IndexFunc(prop.Sessions, func(s ProposedSession) bool { return key != "" && s.Key == key }); si >= 0 {
		cell = plate.CellKey{Date: prop.Sessions[si].CookDate, SlotID: prop.Sessions[si].CookSlotID}
	}
	var cands []FoodFacts
	for id, f := range merged.Foods {
		if id == oldID || merged.tooSlow(f, cell) || merged.slotBlocked(f, cell, merged.householdPlates(f)) || skip[id] || inPlan[merged.dish(id)] || !f.Composed() || f.Archived || f.Never || !varietyRole(f.Role) || !isMain(f.Role, mealFull) || f.Macros.Kcal <= 0 ||
			!f.NutritionComplete || (DietRestricts(household) && !fitsDiet(f.DietClass, diet.Class(household))) {
			continue
		}
		if le := f.LastEaten; le != nil && !le.Before(cookDate.AddDate(0, 0, -span)) {
			continue
		}
		if f.Role != old.Role && f.Role != food.RoleMain {
			continue
		}
		cands = append(cands, f)
	}
	slices.SortStableFunc(cands, func(a, b FoodFacts) int {
		return cmp.Or(cmp.Compare(a.Groups.MeatG, b.Groups.MeatG), cmp.Compare(math.Abs(a.Macros.Kcal-old.Macros.Kcal), math.Abs(b.Macros.Kcal-old.Macros.Kcal)), cmp.Compare(a.ID, b.ID))
	})
	if old.Groups.MeatG <= 0 {
		// A meat-free dish stays meat-free before it stays close in energy.
		slices.SortStableFunc(cands, func(a, b FoodFacts) int {
			return cmp.Or(cmp.Compare(b2i(a.Groups.MeatG > 0), b2i(b.Groups.MeatG > 0)), cmp.Compare(math.Abs(a.Macros.Kcal-old.Macros.Kcal), math.Abs(b.Macros.Kcal-old.Macros.Kcal)), cmp.Compare(a.ID, b.ID))
		})
	}
	repeats := h8Count(before)
	for tries, f := range cands {
		if tries >= maxGuardTries*3 {
			break
		}
		next := cloneProposal(prop)
		changed := false
		si := slices.IndexFunc(next.Sessions, func(s ProposedSession) bool { return key != "" && s.Key == key })
		if si >= 0 {
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
					changed = true
				}
			}
		} else if is.PlateID != nil && *is.PlateID < 0 {
			pi := int(-*is.PlateID) - 1
			if pi >= 0 && pi < len(next.Plates) {
				for ci, c := range next.Plates[pi].Components {
					if c.FoodID == oldID && c.SessionKey == "" && c.SessionID == 0 {
						p := clamp(math.Round(old.Macros.Kcal*c.Portions/f.Macros.Kcal/roundPortions)*roundPortions, optPortionMin, MaxDishPortions)
						next.Plates[pi].Components[ci].FoodID, next.Plates[pi].Components[ci].Portions = f.ID, p
						changed = true
					}
				}
			}
		}
		if !changed {
			return prop, "", false
		}
		after := evaluateLean(Merge(in.State, in.Target, next), in.Target)
		if newHard(hard, after) != "" || h8Count(after) >= repeats || cookDays(after) > cookDays(before) {
			continue
		}
		return next, fmt.Sprintf("Variety: %s replaced by %s (%s was eaten within the last %d weeks).", old.Name, f.Name, old.Name, merged.Settings.VarietyWeeks), true
	}
	return prop, "", false
}

func h8Count(ev Evaluation) int {
	n := 0
	for _, is := range ev.Issues {
		if is.Code == CodeVarietyRepeat && is.Severity == SeverityViolation {
			n++
		}
	}
	return n
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// varietyHints suggest, for every dish that is the main of more meals than the
// user's limit (S8), which dishes could take one of its batches: mains not eaten
// within the variety weeks, not in the plan yet and close in energy.
func varietyHints(in ChooseInput, merged State, ev Evaluation) []string {
	var out []string
	span := 7 * merged.Settings.VarietyWeeks
	for _, is := range ev.Issues {
		if is.Code != CodeDishRepeats {
			continue
		}
		old, ok := merged.Foods[int64(anyF(is.Params["food_id"]))]
		if !ok {
			continue
		}
		used := map[int64]bool{}
		for _, p := range merged.Plates {
			if merged.Window.Contains(p.Date) {
				for _, c := range p.Components {
					used[c.FoodID] = true
				}
			}
		}
		var cands []FoodFacts
		for id, f := range merged.Foods {
			if used[id] || !f.Composed() || f.Archived || f.Never || !varietyRole(f.Role) || !isMain(f.Role, mealFull) || f.Macros.Kcal <= 0 || !f.NutritionComplete ||
				(DietRestricts(merged.Profile.DietType) && !fitsDiet(f.DietClass, diet.Class(merged.Profile.DietType))) ||
				(f.LastEaten != nil && !f.LastEaten.Before(VarietySince(merged.Window.Start, merged.Settings.VarietyWeeks))) || span <= 0 {
				continue
			}
			cands = append(cands, f)
		}
		slices.SortStableFunc(cands, func(a, b FoodFacts) int {
			return cmp.Or(cmp.Compare(math.Abs(a.Macros.Kcal-old.Macros.Kcal), math.Abs(b.Macros.Kcal-old.Macros.Kcal)), cmp.Compare(a.ID, b.ID))
		})
		if len(cands) == 0 {
			continue
		}
		names := []string{plainName(merged, cands[0].ID)}
		if len(cands) > 1 {
			names = append(names, plainName(merged, cands[1].ID))
		}
		out = append(out, fmt.Sprintf("%s is the main of %.0f meals, the limit is %.0f: let another dish take one of its batches, for example %s.", old.Name, anyF(is.Params["meals"]), anyF(is.Params["max"]), joinOr(names)))
	}
	return out
}

func joinOr(n []string) string {
	if len(n) == 1 {
		return n[0]
	}
	return n[0] + " or " + n[1]
}
