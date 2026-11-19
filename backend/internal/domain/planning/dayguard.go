package planning

import (
	"fmt"
	"slices"
	"strings"
	"time"
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
		pi, other, ok := x.sameMainPair(d)
		if !ok {
			continue
		}
		next, note := x.distinctMain(base, t, out, merged, pi, other, skip)
		out = next
		notes = append(notes, note)
	}
	return out, notes
}

// sameMainPair finds a day whose lunch and dinner hold the same main and
// returns the plate to change (the later one, or the proposed one when only
// one is) and the main they share.
func (x *index) sameMainPair(d time.Time) (int, int64, bool) {
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
				return a, mainOf(a), true
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
				return a, mainOf(a), true
			}
		}
	}
	return 0, 0, false
}

// distinctMain replaces the main of plate pi (which repeats other) when a
// verified replacement exists; the note says what was done or why not.
func (x *index) distinctMain(base State, t Target, in Proposal, merged State, pi int, other int64, skip map[int64]bool) (Proposal, string) {
	p := merged.Plates[pi]
	label := fmt.Sprintf("%s: lunch and dinner both hold %s", p.Date.Weekday(), foodName(merged, other))
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
		return next, fmt.Sprintf("Day variety: %s: %s replaced the repeated %s (%s).", x.cellLabel(p), sw.c.food.Name, foodName(merged, other), how)
	}
	return in, fmt.Sprintf("Day variety: %s: kept, no other main fits (%s).", label, x.noDistinctReason(len(sws)))
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
