package planning

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// A regenerated day has different mains at lunch and dinner (E022). A model
// (or the heuristic's batch cover) that fills the day cooks one dish at lunch
// and eats it again at dinner when the day may cook only once. The guard
// replaces the second meal's main with, in this order: leftovers of an
// existing batch of the week, a no-cook dish, a fresh main when the day's
// cooking limit has room (never a second dish cooked on the first one's cell). Each is verified like a swap of the
// week guard (no new hard or soft issue, the day in its band, no new cooking
// day), keeps the week's meat within its limit and leaves fewer repeats. When
// none holds the plan keeps the dish and the run notes say why.

// guardDayMains applies the guard to a run that plans fewer than minDGEDays
// days (a day, plate or short re-plan); a week is the week guard's.
func guardDayMains(base State, t Target, in Proposal, skip map[int64]bool) (Proposal, []string) {
	if distinctDays(t) >= minDGEDays || len(in.Plates) == 0 {
		return in, nil
	}
	out := in
	var notes []string
	for _, d := range newIndex(Merge(base, t, in), t).days {
		merged := Merge(base, t, out)
		x := newIndex(merged, t)
		pi, early, other, ok := x.sameMainPair(d)
		if !ok {
			continue
		}
		next, note, replaced := x.distinctMain(base, t, out, merged, pi, other, skip)
		if !replaced {
			if cold, coldNote, ok := x.coldFirstMeal(base, t, out, merged, early, pi, other, skip); ok {
				next, note = cold, coldNote
			} else if x.hasNoColdFoods(merged, skip) {
				note += " The household has no suitable no-cook foods (a bread, a protein, a raw vegetable) for a cold plate."
			}
		}
		out = next
		notes = append(notes, note)
	}
	return out, notes
}

// sameMainPair finds a day whose lunch and dinner hold the same main and
// returns the plate to change (the later one, or the proposed one when only
// one is) and the main they share.
func (x *index) sameMainPair(d time.Time) (late, early int, shared int64, ok bool) {
	var pis []int
	for i, p := range x.s.Plates {
		if dayOf(p.Date) == dayOf(d) && !p.Confirmed && x.mealOf(p.SlotID) == mealFull && !x.routine(p.Cell()) && x.mainIndex(p) >= 0 {
			pis = append(pis, i)
		}
	}
	slices.SortStableFunc(pis, func(a, b int) int {
		return x.s.SlotOrder[x.s.Plates[b].SlotID] - x.s.SlotOrder[x.s.Plates[a].SlotID]
	})
	mainOf := func(i int) int64 {
		p := x.s.Plates[i]
		return p.Components[x.mainIndex(p)].FoodID
	}
	for i, a := range pis {
		for _, b := range pis[i+1:] {
			if mainOf(a) == mainOf(b) && x.s.Plates[a].Proposed() {
				return a, b, mainOf(a), true
			}
		}
	}
	// A confirmed plate may hold the other meal: it stays, the proposed one changes.
	for _, a := range pis {
		if !x.s.Plates[a].Proposed() {
			continue
		}
		for i, p := range x.s.Plates {
			if i != a && dayOf(p.Date) == dayOf(x.s.Plates[a].Date) && p.Confirmed && x.mealOf(p.SlotID) == mealFull && x.mainIndex(p) >= 0 &&
				p.Components[x.mainIndex(p)].FoodID == mainOf(a) {
				return a, i, mainOf(a), true
			}
		}
	}
	return 0, 0, 0, false
}

// distinctMain replaces the main of plate pi (which repeats other) when a
// verified replacement exists; the note says what was done or why not.
func (x *index) distinctMain(base State, t Target, in Proposal, merged State, pi int, other int64, skip map[int64]bool) (Proposal, string, bool) {
	p := merged.Plates[pi]
	label := fmt.Sprintf("%s: lunch and dinner both hold %s", p.Date.Weekday(), food.Quote(foodName(merged, other)))
	ev := Evaluate(merged, t)
	hard, soft := hardKeys(ev.Issues), softKeys(ev.Issues)
	skipped := map[int64]bool{other: true}
	for id := range skip {
		skipped[id] = true
	}
	sws := x.swapsFor(t, ev, []int{pi}, swapDistinct, skipped)
	pref := func(sw weekSwap) int {
		switch sw.a.Source {
		case SourceSession:
			return 0
		case SourceLeaf:
			return 1
		}
		return 2
	}
	slices.SortStableFunc(sws, func(a, b weekSwap) int { return pref(a) - pref(b) })
	fams, meat := len(x.familyRepeats()), x.windowMeatG()
	limit, hasLimit := MeatBudgetOf(base, t)
	for i, sw := range sws {
		if i >= maxGuardTries {
			break
		}
		next, ok := applySwap(in, merged, sw)
		if !ok {
			continue
		}
		nm := Merge(base, t, next)
		after := Evaluate(nm, t)
		if !guardKeepsDay(ev, after, hard, soft) {
			continue
		}
		ny := newIndex(nm, t)
		if hasLimit && ny.windowMeatG() > max(limit.LimitG, meat)+1e-9 || len(ny.familyRepeats()) >= fams {
			continue
		}
		how := map[int]string{0: "leftovers of a batch the week already has", 1: "a no-cook dish", 2: "a fresh main within the day's cooking limit"}[pref(sw)]
		return next, fmt.Sprintf("Day variety: %s: %s replaced the repeated %s (%s).", x.cellLabel(p), food.Quote(sw.c.food.Name), food.Quote(foodName(merged, other)), how), true
	}
	return in, fmt.Sprintf("Day variety: %s: kept, no other main fits (%s).", label, x.noDistinctReason(len(sws))), false
}

// noDistinctReason says why no replacement was found. verified is the number
// of options that passed the candidate check and then failed the plan check.
func (x *index) noDistinctReason(verified int) string {
	why := []string{"no batch of the week has a spare portion for the meal", "no main is a no-cook dish"}
	if n := x.s.Settings.MaxCookOccasionsPerDay; n > 0 {
		why = append(why, fmt.Sprintf("another dish would need a second cooking occasion on a day limited to %d", n))
	}
	if verified > 0 {
		why = append(why, "the other options break the day's band, a rule or the week's meat limit")
	}
	return strings.Join(why, "; ")
}

// guardKeepsDay holds when the change adds no hard or soft issue and no
// cooking day.
func guardKeepsDay(before, after Evaluation, hard, soft map[string]bool) bool {
	for _, is := range after.Issues {
		switch is.Severity {
		case SeverityViolation:
			if !hard[issueID(is)] {
				return false
			}
		case SeveritySoft:
			if !soft[softKey(is)] {
				return false
			}
		}
	}
	return cookDays(after) <= cookDays(before)
}

// windowMeatG is the meat per eater on the plates of the state's window.
func (x *index) windowMeatG() float64 {
	g := 0.0
	for _, p := range x.s.Plates {
		if x.s.Window.Contains(p.Date) {
			g += x.plateGroups(p).MeatG
		}
	}
	return g
}

// hasNoColdFoods reports that the library cannot make a cold plate.
func (x *index) hasNoColdFoods(s State, skip map[int64]bool) bool {
	_, ok := s.composeColdPlate(skip, 1, 0)
	return !ok
}

// coldFirstMeal turns the day's earlier meal (plate early) into a cold plate
// when no other main fits (the household's choice over a second cook or the
// same dish twice): the later meal (plate late) keeps the dish, which then
// cooks on its own cell, and the earlier one holds no recipe. The plan after
// it must break no hard rule and add no soft issue the balancing pass does not
// own (energy, protein, carbs and fat).
func (x *index) coldFirstMeal(base State, t Target, in Proposal, merged State, early, late int, other int64, skip map[int64]bool) (Proposal, string, bool) {
	ep, lp := merged.Plates[early], merged.Plates[late]
	i := int(-ep.ID) - 1
	if !ep.Proposed() || i < 0 || i >= len(in.Plates) || in.Plates[i].PlateID != 0 {
		return in, "", false
	}
	var dk *DayKcal
	for _, a := range x.dayAccounts([]time.Time{ep.Date}) {
		dk = &a
	}
	// Without a kcal target the plate is one serving of each food.
	aim, dayTarget := 0.0, 0.0
	if dk != nil && dk.Adjusted > 0 {
		aim, dayTarget = dk.Adjusted/float64(max(dk.Planned, 1)), dk.Adjusted
		if merged.lightMeal(ep.Date, ep.SlotID) {
			aim *= LightMealShare
		}
	}
	comps, ok := merged.composeColdPlate(skip, aim, dayTarget)
	if !ok {
		return in, "", false
	}
	out := in
	out.Plates = slices.Clone(in.Plates)
	out.Sessions = slices.Clone(in.Sessions)
	old := out.Plates[i]
	out.Plates[i].Components = comps
	for _, c := range old.Components {
		if c.SessionKey == "" {
			continue
		}
		for k, ss := range out.Sessions {
			if ss.Key != c.SessionKey {
				continue
			}
			out.Sessions[k].Portions = ss.Portions - ep.Eaters*c.Portions
			out.Sessions[k].FrozenPortions = min(ss.FrozenPortions, max(out.Sessions[k].Portions, 0))
			// The cook no longer feeds the cold meal: it cooks on the cell that still eats it.
			if sameCell(ss.CookDate, ss.CookSlotID, ep.Date, ep.SlotID) && usesSession(out.Plates, ss.Key) {
				out.Sessions[k].CookDate, out.Sessions[k].CookSlotID = lp.Date, lp.SlotID
			}
			break
		}
	}
	out.Sessions = slices.DeleteFunc(out.Sessions, func(ss ProposedSession) bool { return ss.Portions <= epsilon })
	ev := Evaluate(merged, t)
	hard, soft := hardKeys(ev.Issues), softKeys(ev.Issues)
	nm := Merge(base, t, out)
	after := Evaluate(nm, t)
	for _, is := range after.Issues {
		switch is.Severity {
		case SeverityViolation:
			if !hard[issueID(is)] {
				return in, "", false
			}
		case SeveritySoft:
			// Energy, protein and the weekly shares are the balancing pass's to lift.
			if !soft[softKey(is)] && is.Rule != "S2" && is.Rule != "S3" && is.Rule != "S4" && is.Rule != "S5" {
				return in, "", false
			}
		}
	}
	if cookDays(after) > cookDays(ev) {
		return in, "", false
	}
	var parts []string
	kcal := 0.0
	for _, c := range comps {
		f := merged.Foods[c.FoodID]
		parts = append(parts, fmt.Sprintf("%s %s g", food.Quote(strings.ToLower(f.Name)), num(c.Grams)))
		kcal += f.Macros.Kcal * c.Grams / 100
	}
	dinner := "dinner is " + food.Quote(foodName(merged, other))
	if x.cooksAt(out, ep.Date, lp.SlotID) {
		dinner = "dinner is cooked fresh: " + food.Quote(foodName(merged, other))
	}
	return out, fmt.Sprintf("Day variety: %s: %s is a cold plate (about %.0f kcal, nothing to cook): %s; %s.", ep.Date.Weekday(), slotName(merged, ep.SlotID), kcal, strings.Join(parts, ", "), dinner), true
}

func sameCell(d1 time.Time, s1 int64, d2 time.Time, s2 int64) bool {
	return dayOf(d1) == dayOf(d2) && s1 == s2
}

// usesSession reports whether a proposed plate still eats the session.
func usesSession(plates []ProposedPlate, key string) bool {
	return slices.ContainsFunc(plates, func(p ProposedPlate) bool {
		return slices.ContainsFunc(p.Components, func(c ProposedComponent) bool { return c.SessionKey == key })
	})
}

// cooksAt reports whether the proposal cooks something on the cell.
func (x *index) cooksAt(p Proposal, d time.Time, slot int64) bool {
	return slices.ContainsFunc(p.Sessions, func(ss ProposedSession) bool { return sameCell(ss.CookDate, ss.CookSlotID, d, slot) }) ||
		slices.ContainsFunc(p.Plates, func(pl ProposedPlate) bool {
			return sameCell(pl.Date, pl.SlotID, d, slot) && slices.ContainsFunc(pl.Components, func(c ProposedComponent) bool { return c.SessionKey == "" && c.SessionID == 0 && c.Portions > 0 })
		})
}
