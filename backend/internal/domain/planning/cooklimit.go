package planning

import (
	"fmt"
	"slices"
	"time"
)

// fixCookLimit closes a break of the cooking limit by the move the household
// accepts: a side recipe cooked fresh on its own plate (no batch) is dropped
// from a day that cooks on more cells than the limit allows, when that frees
// the cell's cooking occasion; the balancing pass lifts the day's energy again
// with sides and foods that need no cooking. A day still over the limit because
// an earlier meal cooks its own fresh main gets a cold plate there (the household's
// choice over a second cook, see coldFirstMeal). A cooking cell that holds a
// batch stays: that is a choice of dishes, and the model's.
func fixCookLimit(in ChooseInput, prop Proposal) (Proposal, []string) {
	limit := in.State.Settings.MaxCookOccasionsPerDay
	if limit <= 0 {
		return prop, nil
	}
	var notes []string
	for range maxGuardSteps {
		merged := Merge(in.State, in.Target, prop)
		x := newIndex(merged, in.Target)
		ev := Evaluate(merged, in.Target)
		hard := hardKeys(ev.Issues)
		moved := false
	days:
		for _, d := range x.days {
			occ := len(x.occasions[dayOf(d)])
			if occ <= limit {
				continue
			}
			for pi := len(prop.Plates) - 1; pi >= 0; pi-- {
				pl := prop.Plates[pi]
				if dayOf(pl.Date) != dayOf(d) || pl.PlateID != 0 {
					continue
				}
				lead := mainID(merged, pl)
				for ci, c := range pl.Components {
					f := merged.Foods[c.FoodID]
					if c.SessionKey != "" || c.SessionID != 0 || c.Portions <= 0 || !f.Composed() || c.FoodID == lead {
						continue
					}
					next := cloneProposal(prop)
					next.Plates[pi].Components = slices.Delete(next.Plates[pi].Components, ci, ci+1)
					nm := Merge(in.State, in.Target, next)
					if len(newIndex(nm, in.Target).occasions[dayOf(d)]) >= occ || newHard(hard, Evaluate(nm, in.Target)) != "" {
						continue
					}
					prop, moved = next, true
					notes = append(notes, fmt.Sprintf("Cooking limit: %s: %s dropped from %s (it needed a second cooking on the day).", d.Weekday(), f.Name, cellShort(merged, pl.Date, pl.SlotID)))
					break days
				}
			}
		}
		if !moved {
			prop, moved = coldForLimit(in, prop, notes2(&notes))
		}
		if !moved {
			break
		}
	}
	return prop, notes
}

func notes2(n *[]string) func(string) { return func(s string) { *n = append(*n, s) } }

// coldForLimit turns the earliest meal of a day over the cooking limit, one that
// cooks a fresh main for itself alone, into a cold plate, when the day then
// breaks no further rule.
func coldForLimit(in ChooseInput, prop Proposal, note func(string)) (Proposal, bool) {
	limit := in.State.Settings.MaxCookOccasionsPerDay
	merged := Merge(in.State, in.Target, prop)
	x := newIndex(merged, in.Target)
	hard := hardKeys(Evaluate(merged, in.Target).Issues)
	skip := idSet(in.Exclude)
	for _, d := range x.days {
		occ := len(x.occasions[dayOf(d)])
		if occ <= limit {
			continue
		}
		var dk *DayKcal
		for _, a := range x.dayAccounts([]time.Time{d}) {
			dk = &a
		}
		for pi, pl := range prop.Plates {
			if dayOf(pl.Date) != dayOf(d) || pl.PlateID != 0 || mainID(merged, pl) == 0 || x.mealOf(pl.SlotID) != mealFull {
				continue
			}
			if !slices.ContainsFunc(pl.Components, func(c ProposedComponent) bool {
				return c.SessionKey == "" && c.SessionID == 0 && c.Portions > 0 && c.FoodID == mainID(merged, pl)
			}) {
				continue
			}
			aim, dayTarget := 0.0, 0.0
			if dk != nil && dk.Adjusted > 0 {
				aim, dayTarget = dk.Adjusted/float64(max(dk.Planned, 1)), dk.Adjusted
				if merged.lightMeal(pl.Date, pl.SlotID) {
					aim *= LightMealShare
				}
			}
			comps, ok := merged.composeColdPlate(skip, aim, dayTarget)
			if !ok {
				continue
			}
			next := cloneProposal(prop)
			next.Plates[pi].Components = comps
			nm := Merge(in.State, in.Target, next)
			if len(newIndex(nm, in.Target).occasions[dayOf(d)]) >= occ || newHard(hard, Evaluate(nm, in.Target)) != "" {
				continue
			}
			note(fmt.Sprintf("Cooking limit: %s: %s made a cold plate instead of %s (a second cooking on the day).", d.Weekday(), cellShort(merged, pl.Date, pl.SlotID), foodName(merged, mainID(merged, pl))))
			return next, true
		}
	}
	return prop, false
}
