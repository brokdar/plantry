package planning

import (
	"fmt"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// A short run (a day replan) whose cooking cap left a meal empty fills it by
// KR7's fallbacks, in order (spec KR7, tune-v2c g2-replan-01: Sunday's lunch
// ate Saturday's batch, the replan avoided it and the one cooking of the day
// went to dinner, so lunch stayed empty and the day fell under its minimum):
//  1. a cold plate of the household's no-cook foods beside the one cooked meal;
//  2. the same dish twice: the meal eats the day's cooked dish (cooked at the
//     earlier of the two meals), only where no hard rule breaks, so never
//     where dish variety (H8) triggers.
// A batch of an earlier day with a spare portion is the chooser's own first
// pick (supplyFor), so it is no fallback here. A fill must break no hard rule
// the plan does not already break and leave the day at or above its minimum
// after balancing; a meal no fallback fills stays empty and the day run saves
// nothing (service: emptyDayMeals).

// The plate reasons of the two fallbacks (stored with the plate, shown on the board).
const (
	reasonColdFill     = "Cold plate, nothing to cook: the day cooks once"
	reasonSameDishFill = "The day's dish again: the day cooks once"
)

// fillEmptyMeals fills the short run's empty planned meals by KR7's fallbacks.
func fillEmptyMeals(in ChooseInput, prop Proposal) (Proposal, []string) {
	if distinctDays(in.Target) >= minDGEDays {
		return prop, nil
	}
	skip := idSet(in.Exclude)
	var notes []string
	for _, c := range emptyMeals(Merge(in.State, in.Target, prop), in.Target) {
		merged := Merge(in.State, in.Target, prop)
		x := newIndex(merged, in.Target)
		if next, ok := x.coldMeal(in, prop, merged, c, skip); ok {
			prop = next
			notes = append(notes, fmt.Sprintf("Empty meal: %s %s is a cold plate, nothing to cook (one cooking time that day).", c.Date.Weekday(), slotName(merged, c.SlotID)))
			continue
		}
		if next, ok := x.sameDishMeal(in, prop, c); ok {
			prop = next
			notes = append(notes, fmt.Sprintf("Empty meal: %s %s eats the day's cooked dish again (one cooking time that day).", c.Date.Weekday(), slotName(merged, c.SlotID)))
		}
	}
	return prop, notes
}

// emptyMeals are the target's planned lunch and dinner cells (not routine,
// not away) that hold no plate, in cell order.
func emptyMeals(s State, t Target) []plate.CellKey {
	x := newIndex(s, t)
	var out []plate.CellKey
	for _, c := range s.Cells {
		k := c.Key()
		if c.Mode != availability.ModePlan || !x.inTarget(k) || x.mealOf(c.SlotID) != mealFull || len(x.byCell[idOf(k)]) > 0 {
			continue
		}
		out = append(out, k)
	}
	slices.SortStableFunc(out, func(a, b plate.CellKey) int { return plate.Compare(a, b, s.SlotOrder) })
	return out
}

// fillHolds reports whether the plan with the fill breaks no hard rule the
// plan without it lacks, adds no cooking day, and keeps the day at or above its
// minimum once balanced; it returns the balanced plan. A warning never stops a
// fill: an empty meal is a hard rule broken, a warning is told.
func fillHolds(in ChooseInput, before State, next Proposal) (Proposal, bool) {
	ev := evaluateLean(before, in.Target)
	hard := hardKeys(ev.Issues)
	after := evaluateLean(Merge(in.State, in.Target, next), in.Target)
	for _, is := range after.Issues {
		// The day's energy is lifted by balancing: judged on the balanced plan below.
		if is.Severity == SeverityViolation && !hard[issueID(is)] && is.Code != CodeKcalBelowFloor {
			return next, false
		}
	}
	if cookDays(after) > cookDays(ev) {
		return next, false
	}
	bal, _ := balancePlan(in, next)
	if newHard(hard, evaluateLean(Merge(in.State, in.Target, bal), in.Target)) != "" {
		return next, false
	}
	return bal, true
}

// coldMeal puts a cold plate on the empty cell c (fallback 1).
func (x *index) coldMeal(in ChooseInput, prop Proposal, merged State, c plate.CellKey, skip map[int64]bool) (Proposal, bool) {
	aim, dayTarget := 0.0, 0.0
	for _, dk := range x.dayAccounts([]time.Time{c.Date}) {
		if dk.Adjusted > 0 {
			aim, dayTarget = dk.mealShare(c.SlotID), dk.Adjusted
			if merged.lightMeal(c.Date, c.SlotID) {
				aim *= LightMealShare
			}
		}
	}
	comps, ok := merged.composeColdPlate(skip, aim, dayTarget)
	if !ok {
		return prop, false
	}
	next := prop
	next.Plates = append(slices.Clone(prop.Plates), ProposedPlate{Date: c.Date, SlotID: c.SlotID, Components: comps, Reason: reasonColdFill})
	return fillHolds(in, merged, next)
}

// sameDishMeal lets the empty cell c eat the dish a proposed plate of the
// same day cooks (fallback 2): the batch grows by the meal's portions and
// cooks at the earlier of the two meals, so the day keeps one cooking time.
func (x *index) sameDishMeal(in ChooseInput, prop Proposal, c plate.CellKey) (Proposal, bool) {
	merged := Merge(in.State, in.Target, prop)
	pp := ProposedPlate{Date: c.Date, SlotID: c.SlotID, Eaters: in.EatersByDate[c.Date.Format(time.DateOnly)], Reason: reasonSameDishFill}
	eaters := pp.eaters(merged)
	for _, p := range prop.Plates {
		if p.PlateID != 0 || dayOf(p.Date) != dayOf(c.Date) || len(p.Components) == 0 || p.Components[0].SessionKey == "" {
			continue
		}
		next := prop
		next.Sessions = slices.Clone(prop.Sessions)
		ok := true
		for _, pc := range p.Components {
			k := slices.IndexFunc(next.Sessions, func(ss ProposedSession) bool { return ss.Key == pc.SessionKey })
			switch {
			case pc.SessionKey == "" && pc.Grams > 0: // a no-cook leaf goes along as it is
				continue
			case k < 0:
				ok = false
			default:
				ss := &next.Sessions[k]
				ss.Portions += eaters * pc.Portions
				if plate.Before(c, plate.CellKey{Date: ss.CookDate, SlotID: ss.CookSlotID}, merged.SlotOrder) {
					ss.CookDate, ss.CookSlotID = c.Date, c.SlotID
				}
			}
		}
		if !ok {
			continue
		}
		pp.Components = slices.Clone(p.Components)
		next.Plates = append(slices.Clone(prop.Plates), pp)
		if out, held := fillHolds(in, merged, next); held {
			return out, true
		}
	}
	return prop, false
}

// emptyDayMeals says what a day replan would leave open on its day: a planned
// meal without a plate, or the day under its minimum; "" when neither.
func emptyDayMeals(s State, t Target, date time.Time) string {
	for _, is := range openHard(s, t) {
		if is.Date != date.Format(time.DateOnly) {
			continue
		}
		switch is.Code {
		case CodeMissingPlate:
			meal := "a meal"
			if is.SlotID != nil {
				meal = slotName(s, *is.SlotID)
			}
			return fmt.Sprintf("%s %s would stay empty (the day cooks once, and neither a cold plate nor a second serving of the day's dish fits)", date.Weekday(), meal)
		case CodeKcalBelowFloor:
			return fmt.Sprintf("%s would stay under its minimum (%.0f of %.0f kcal)", date.Weekday(), anyF(is.Params["kcal"]), anyF(is.Params["floor_kcal"]))
		}
	}
	return ""
}
