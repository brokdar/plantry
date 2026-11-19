package planning

import (
	"fmt"
	"slices"
	"strings"
)

// fixCookLimit closes a break of the cooking limit by the move the household
// accepts: a side recipe cooked fresh on its own plate (no batch) is dropped
// from a day that cooks on more cells than the limit allows, when that frees
// the cell's cooking occasion; the balancing pass lifts the day's energy again
// with sides and foods that need no cooking. A cooking cell that holds a main or
// a batch stays, and so does which meals are leftovers: that is a choice of
// dishes and days, and the model's (cookingHints tell it which days cook twice).
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
			break
		}
	}
	return prop, notes
}

// cookingHints name, for each day still over the cooking limit, where it cooks
// and a batch whose leftovers the model can put on one of those meals instead.
func cookingHints(in ChooseInput, merged State) []string {
	limit := in.State.Settings.MaxCookOccasionsPerDay
	if limit <= 0 {
		return nil
	}
	x := newIndex(merged, in.Target)
	var out []string
	for _, d := range x.days {
		cells := x.occasions[dayOf(d)]
		if len(cells) <= limit {
			continue
		}
		var names []string
		for c := range cells {
			for _, p := range merged.Plates {
				if dayOf(p.Date) == c.day && p.SlotID == c.slot {
					names = append(names, slotName(merged, p.SlotID))
					break
				}
			}
		}
		slices.Sort(names)
		hint := fmt.Sprintf("%s cooks %d times (%s); the limit is %d.", d.Weekday(), len(cells), strings.Join(names, ", "), limit)
		var best *Session
		for i := range merged.Sessions {
			ss := &merged.Sessions[i]
			if !ss.Proposed() || dayOf(ss.CookDate) >= dayOf(d) || ss.CookDate.AddDate(0, 0, x.keepDays(ss.FoodID)).Before(d) {
				continue
			}
			if best == nil || ss.CookDate.After(best.CookDate) {
				best = ss
			}
		}
		if best != nil {
			hint += fmt.Sprintf(" Eat leftovers of %s (cooked %s) at one of those meals so only one of them cooks.", plainName(merged, best.FoodID), cellShort(merged, best.CookDate, best.CookSlotID))
		} else {
			hint += " Let one of those meals eat leftovers of a batch cooked on an earlier day so only one of them cooks."
		}
		out = append(out, hint)
	}
	return out
}
