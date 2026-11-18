package planning

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"
)

// The final balancing pass (E014): the model chooses dishes, days and batches;
// the quantities (portions, sides, add-ons) are set in Go, by the same
// machinery the heuristic uses for its own plans (topUpFloors and the verified
// no-cook adds). It runs after the model's plan and the portion and meat
// guards, on the plates the run created only.

// maxSideKcalShare bounds a side recipe the pass adds to a day, as a share of
// the day's target.
const maxSideKcalShare = 0.3

// balancePlan returns prop with its quantities balanced and one plain-words
// note per change. Nothing changes without a kcal target, and a result that
// adds a hard issue is dropped whole.
func balancePlan(in ChooseInput, prop Proposal) (Proposal, []string) {
	if k := in.State.Profile.KcalTarget; k == nil || *k <= 0 || len(prop.Plates) == 0 {
		return prop, nil
	}
	h := newPlanner(in)
	h.balance, h.sided = true, map[int]bool{}
	h.prop = cloneProposal(prop)
	before := Evaluate(Merge(in.State, in.Target, prop), in.Target)
	h.trimAddOns()
	h.topUpFloors()
	after := Evaluate(Merge(in.State, in.Target, h.prop), in.Target)
	hard := hardKeys(before.Issues)
	for k := range hardKeys(after.Issues) {
		if !hard[k] {
			return prop, nil
		}
	}
	return h.prop, balanceNotes(in.State, prop, h.prop)
}

func cloneProposal(p Proposal) Proposal {
	out := p
	out.Sessions = slices.Clone(p.Sessions)
	out.Plates = make([]ProposedPlate, len(p.Plates))
	for i, pl := range p.Plates {
		pl.Components = slices.Clone(pl.Components)
		out.Plates[i] = pl
	}
	return out
}

func anyF(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

// trimAddOns removes or shrinks the add-ons the add-on excess rule (S9) flags
// on the proposal's plates; the lifts that follow make up for the energy.
func (h *planner) trimAddOns() {
	y := newIndex(Merge(h.s, h.in.Target, h.prop), h.in.Target)
	for _, is := range y.addOnIssues() {
		if is.PlateID != nil { // a persisted plate: not the run's
			continue
		}
		pi := slices.IndexFunc(h.prop.Plates, func(p ProposedPlate) bool {
			return p.PlateID == 0 && p.Date.Format(time.DateOnly) == is.Date && is.SlotID != nil && p.SlotID == *is.SlotID
		})
		if pi < 0 {
			continue
		}
		food := int64(anyF(is.Params["food_id"]))
		p := &h.prop.Plates[pi]
		for ci, c := range p.Components {
			if c.FoodID != food || c.Grams <= 0 {
				continue
			}
			if mg := anyF(is.Params["max_grams"]); is.Params["reason"] == "over_cap" && mg > 0 && mg < c.Grams {
				p.Components[ci].Grams = mg
			} else {
				p.Components = slices.Delete(p.Components, ci, ci+1)
			}
			break
		}
	}
}

// addSide adds one side recipe of the library to a day still under limit kcal
// after its portions were raised: on an editable plate (never a light meal),
// verified like an alternative (no new hard or soft issue, closer to the day's
// target, not past the band top, at most maxSideKcalShare of the target).
func (h *planner) addSide(date time.Time, limit float64) {
	if h.sided[dayOf(date)] {
		return
	}
	merged := Merge(h.s, h.in.Target, h.prop)
	y := newIndex(merged, h.in.Target)
	var dk *DayKcal
	for _, a := range y.dayAccounts([]time.Time{date}) {
		dk = &a
	}
	if dk == nil || !dk.Complete || dk.Away || dk.NutritionMissing || dk.Kcal <= 0 || limit-dk.Kcal < minAddGrams*2 {
		return
	}
	need := limit - dk.Kcal
	base := Evaluate(merged, h.in.Target)
	hard, soft := hardKeys(base.Issues), softKeys(base.Issues)
	cooked := map[int64]bool{}
	for _, ev := range y.cooks {
		cooked[ev.foodID] = true
	}
	held := map[int64]bool{}
	for _, p := range merged.Plates {
		if dayOf(p.Date) == dayOf(date) {
			for _, c := range p.Components {
				held[c.FoodID] = true
			}
		}
	}
	type opt struct {
		pi int
		c  altCand
		a  Alternative
	}
	var opts []opt
	for _, pi := range y.editablePlates(date) {
		p := merged.Plates[pi]
		if !p.Proposed() || y.mealOf(p.SlotID) != mealFull {
			continue
		}
		for _, c := range merged.altCandidates(y, p, -1, AltRequest{What: AltAdd}) {
			f := c.food
			if !f.Composed() || !sideRole(f.Role) || f.Macros.Kcal <= 0 || held[f.ID] || h.exclude[f.ID] || (c.session != nil && c.grow > 0) || c.cookAt != nil ||
				f.Macros.Kcal*c.comp.Multiplier() > dk.Adjusted*maxSideKcalShare {
				continue
			}
			a, why, _ := merged.tryAlt(h.in.Target, pi, -1, nil, c, base, hard, soft, len(y.occasions[dayOf(p.Date)]), cooked, false)
			if why != "" || len(a.NewIssues) > 0 || a.rank.addsCook || a.DayKcal > dk.Adjusted*y.pol.DayHigh ||
				math.Abs(dk.Adjusted-a.DayKcal) >= math.Abs(dk.Adjusted-dk.Kcal) {
				continue
			}
			opts = append(opts, opt{pi, c, a})
		}
	}
	slices.SortStableFunc(opts, func(a, b opt) int {
		return cmp.Or(boolCmp(a.a.rank.addsCook, b.a.rank.addsCook), cmp.Compare(math.Abs(need-a.c.food.Macros.Kcal*a.c.comp.Multiplier()), math.Abs(need-b.c.food.Macros.Kcal*b.c.comp.Multiplier())),
			cmp.Compare(a.a.rank.source, b.a.rank.source), cmp.Compare(a.c.food.ID, b.c.food.ID))
	})
	for _, o := range opts {
		if next, ok := applyAdd(h.prop, merged, o.pi, o.c); ok {
			h.prop, h.sided[dayOf(date)] = next, true
			return
		}
	}
}

// applyAdd appends the recipe of c to the proposal's plate for merged plate pi
// (fresh, from a persisted batch, or on its own new session).
func applyAdd(in Proposal, merged State, pi int, c altCand) (Proposal, bool) {
	p := merged.Plates[pi]
	i := int(-p.ID) - 1
	if i < 0 || i >= len(in.Plates) || in.Plates[i].PlateID != 0 {
		return in, false
	}
	out := cloneProposal(in)
	nc := ProposedComponent{FoodID: c.food.ID, Portions: *c.comp.Portions}
	switch {
	case c.cookAt != nil:
		key := fmt.Sprintf("side-%d", len(out.Sessions)+1)
		for slices.ContainsFunc(out.Sessions, func(s ProposedSession) bool { return s.Key == key }) {
			key += "x"
		}
		out.Sessions = append(out.Sessions, ProposedSession{Key: key, FoodID: c.food.ID, CookDate: c.cookAt.Date, CookSlotID: c.cookAt.SlotID, Portions: nc.Portions * p.Eaters, Reason: "cooked for the added side"})
		nc.SessionKey = key
	case c.session != nil:
		nc.SessionID = c.session.ID
	}
	out.Plates[i].Components = append(out.Plates[i].Components, nc)
	return out, true
}

// balanceNotes describes what the pass changed, one plain-words line each.
func balanceNotes(s State, a, b Proposal) []string {
	var notes []string
	for i := range a.Plates {
		pa, pb := a.Plates[i], b.Plates[i]
		cell := pa.Date.Weekday().String() + " " + slotName(s, pa.SlotID)
		for _, ca := range pa.Components {
			cb, ok := findComp(pb, ca)
			switch {
			case !ok:
				notes = append(notes, fmt.Sprintf("Balanced: %s: %s removed (add-on excess).", cell, foodName(s, ca.FoodID)))
			case cb.Grams > 0 && cb.Grams < ca.Grams-epsilon:
				notes = append(notes, fmt.Sprintf("Balanced: %s: %s reduced from %s g to %s g (over the add-on cap).", cell, foodName(s, ca.FoodID), num(ca.Grams), num(cb.Grams)))
			case cb.Portions > ca.Portions+epsilon:
				notes = append(notes, fmt.Sprintf("Balanced: %s: %s portions raised from %s to %s to bring the day toward its target.", cell, foodName(s, ca.FoodID), num(ca.Portions), num(cb.Portions)))
			}
		}
		for _, cb := range pb.Components {
			if _, ok := findComp(pa, cb); ok {
				continue
			}
			amount := num(cb.Portions) + " portions"
			if cb.Grams > 0 {
				amount = num(cb.Grams) + " g"
			}
			notes = append(notes, fmt.Sprintf("Balanced: %s: %s (%s) added to bring the day toward its target.", cell, foodName(s, cb.FoodID), amount))
		}
	}
	return notes
}

func findComp(p ProposedPlate, c ProposedComponent) (ProposedComponent, bool) {
	for _, o := range p.Components {
		if o.FoodID == c.FoodID && (o.Grams > 0) == (c.Grams > 0) {
			return o, true
		}
	}
	return ProposedComponent{}, false
}
