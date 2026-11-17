package planning

import (
	"math"
	"time"
)

// maxTopUp caps how far the floor pass raises a component's portions: a
// plate of 2.5 portions of a small recipe is still a meal.
const maxTopUp = 2.5

// topUpFloors raises the portions of the proposal's plates on every complete
// day that holds fewer kcal than the policy's floor (H9), toward the lower
// edge of the day band. The heuristic picks recipes by closeness to a meal's
// kcal share and never counts a day's total, so a day of small recipes (or
// a routine breakfast plus one dinner) would fall under the floor. Only
// components that eat a proposed session within its keep days grow (their
// session grows by the same amount, so H3 holds); frozen links and fixed
// sessions stay. A day that cannot reach the floor within maxTopUp stays as
// it is; the checker reports it.
func (h *planner) topUpFloors() {
	k := h.s.Profile.KcalTarget
	if k == nil || *k <= 0 || len(h.prop.Plates) == 0 {
		return
	}
	merged := Merge(h.s, h.in.Target, h.prop)
	y := newIndex(merged, h.in.Target)
	for _, dk := range y.dayAccounts(y.days) {
		if !dk.Complete || dk.NutritionMissing || dk.Kcal <= 0 {
			continue
		}
		share := float64(dk.Planned) / float64(dk.Meals)
		floor := y.pol.Floor(dk.Target, share)
		if dk.Kcal >= floor {
			continue
		}
		h.topUpDay(dk.Date, math.Max(floor, y.pol.DayLow*dk.Scaled)-dk.Kcal)
	}
}

// topUpDay adds need kcal per eater to the day's growable components.
func (h *planner) topUpDay(date time.Time, need float64) {
	sessions := map[string]int{}
	for i, ss := range h.prop.Sessions {
		sessions[ss.Key] = i
	}
	// growable reports whether a component eats a proposed session within
	// its keep days (not a frozen link), and the session's index.
	growable := func(p ProposedPlate, c ProposedComponent) (int, bool) {
		i, ok := sessions[c.SessionKey]
		if c.SessionKey == "" || !ok || c.Portions <= 0 {
			return 0, false
		}
		ss := h.prop.Sessions[i]
		f, known := h.s.Foods[c.FoodID]
		if !known {
			return 0, false
		}
		last := ss.CookDate.AddDate(0, 0, h.x.keepDays(c.FoodID))
		return i, !p.Date.After(last) && f.Macros.Kcal > 0
	}
	var raisable float64
	for _, p := range h.prop.Plates {
		if dayOf(p.Date) != dayOf(date) {
			continue
		}
		for _, c := range p.Components {
			if _, ok := growable(p, c); ok {
				raisable += h.s.Foods[c.FoodID].Macros.Kcal * c.Portions
			}
		}
	}
	if raisable <= 0 {
		return
	}
	factor := math.Min(1+need/raisable, maxTopUp)
	for pi := range h.prop.Plates {
		p := &h.prop.Plates[pi]
		if dayOf(p.Date) != dayOf(date) {
			continue
		}
		eaters := p.eaters(h.s)
		for ci := range p.Components {
			c := &p.Components[ci]
			si, ok := growable(*p, *c)
			if !ok {
				continue
			}
			raised := math.Ceil(c.Portions*factor*4-epsilon) / 4
			h.prop.Sessions[si].Portions += eaters * (raised - c.Portions)
			c.Portions = raised
		}
	}
}
