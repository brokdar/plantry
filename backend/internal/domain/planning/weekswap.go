package planning

import (
	"cmp"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
)

// The weekly meat budget and the fish meal (S7) as checklist items: Go
// computes the budget, lists the meat plates, and offers verified swaps of a
// meat main for a main with less meat (or a fish main). The planner's
// evaluate_plan and its repair feedback carry them (EvaluateWeek); the last
// guard before saving applies them (guardWeek).

const (
	// maxWeekOptions is how many swaps a week issue lists (one per plate).
	maxWeekOptions = 4
	// maxGuardSteps bounds the swaps the guard applies to one plan (a week has at most 14 main cells).
	maxGuardSteps = 14
	// maxGuardTries bounds the full re-evaluations the guard spends per plate.
	maxGuardTries = 6
)

// MeatBudget is the week's meat allowance per eater over the days being
// planned: the DGE limit prorated to them, less the meat on plates that stay.
type MeatBudget struct {
	Days    int
	LimitG  float64
	FixedG  float64
	BudgetG float64
}

// MeatBudgetOf computes the budget of s for the draft t replaces: the planned
// days are the window's days with a plan cell that are not away. False when
// the rule does not apply (a diet without meat, fewer than minDGEDays days).
func MeatBudgetOf(s State, t Target) (MeatBudget, bool) {
	if !meatAllowed(s) {
		return MeatBudget{}, false
	}
	m := Merge(s, t, Proposal{})
	x := newIndex(m, t)
	var days []time.Time
	seen := map[int]bool{}
	for _, c := range m.Cells {
		if d := dayOf(c.Date); c.Mode == availability.ModePlan && m.Window.Contains(c.Date) && !seen[d] {
			seen[d] = true
			days = append(days, c.Date)
		}
	}
	counted := map[int]bool{}
	for _, dk := range x.dayAccounts(days) {
		if !dk.Away {
			counted[dayOf(dk.Date)] = true
		}
	}
	b := MeatBudget{Days: len(counted)}
	if b.Days < minDGEDays {
		return b, false
	}
	for _, p := range m.Plates {
		if counted[dayOf(p.Date)] && m.Window.Contains(p.Date) {
			b.FixedG += x.plateGroups(p).MeatG
		}
	}
	b.LimitG = DGEMeatMaxG * float64(b.Days) / weekDays
	b.BudgetG = math.Max(0, b.LimitG-b.FixedG)
	return b, true
}

// meatAllowed and fishAllowed: the household's diet type permits the food.
func meatAllowed(s State) bool {
	dt := diet.Class(s.Profile.DietType)
	return dt == "" || diet.Compatible(diet.Omnivore, dt)
}

func fishAllowed(s State) bool {
	dt := diet.Class(s.Profile.DietType)
	return dt == "" || diet.Compatible(diet.Pescetarian, dt)
}

// noFish matches an instruction that rules fish out (vegetarian, vegan, no
// fish): the guard then places no fish meal.
var noFish = regexp.MustCompile(`(?i)vegetar|vegan|fleischlos|meatless|(kein|ohne|no|without)\s+fisch|(no|without)\s+fish|fish.free`)

// plateGroups is the DGE food groups of a plate, per eater.
func (x *index) plateGroups(p Plate) FoodGroups {
	var g FoodGroups
	for _, c := range p.Components {
		if f, ok := x.s.Foods[c.FoodID]; ok {
			g = g.add(f.Groups.times(c.Multiplier()))
		}
	}
	return g
}

// weekCounted is the days (YYYY-MM-DD) the week ledger of ev counts.
func weekCounted(ev Evaluation) map[string]bool {
	withPlate := map[string]bool{}
	for _, p := range ev.Plates {
		withPlate[p.Date] = true
	}
	out := map[string]bool{}
	for _, d := range ev.Days {
		if d.Complete && !d.Away && withPlate[d.Date] && !d.NutritionMissing && d.Kcal > 0 && d.TargetKcal != nil && *d.TargetKcal > 0 {
			out[d.Date] = true
		}
	}
	return out
}

// weekPlates is the plates of the counted days with their meat grams, as
// the issue lists them.
func (x *index) weekPlates(counted map[string]bool) []map[string]any {
	var out []map[string]any
	for _, p := range x.s.Plates {
		if !counted[p.Date.Format(time.DateOnly)] || !x.s.Window.Contains(p.Date) {
			continue
		}
		g := x.plateGroups(p)
		if g.MeatG <= 0 {
			continue
		}
		main := ""
		if id := x.s.mainOf(p); id != 0 {
			main = foodName(x.s, id)
		}
		out = append(out, map[string]any{
			"date": p.Date.Format(time.DateOnly), "slot_id": p.SlotID, "slot": slotName(x.s, p.SlotID), "main": main,
			"meat_g": math.Round(g.MeatG), "fixed": p.Confirmed,
		})
	}
	slices.SortStableFunc(out, func(a, b map[string]any) int { return cmp.Compare(b["meat_g"].(float64), a["meat_g"].(float64)) })
	return out
}

// swapMode says what a swap is for.
type swapMode int

const (
	swapMeat swapMode = iota // less meat on the plate's main
	swapFish                 // a fish main
)

// weekSwap is one verified replacement of the main of the plate at index pi.
type weekSwap struct {
	pi, old   int
	c         altCand
	a         Alternative
	oldMeatG  float64 // the replaced main's meat per eater
	newMeatG  float64
	plateMeat float64
	rankMeat  float64
}

// swapPlates are the indices of the plates a swap may change, largest meat
// first (then date): draft plates of the counted days on a full-meal cell
// that hold a composed main. proposedOnly keeps the proposal's own plates.
func (x *index) swapPlates(counted map[string]bool, mode swapMode, proposedOnly bool) []int {
	var out []int
	for i, p := range x.s.Plates {
		if p.Confirmed || (proposedOnly && !p.Proposed()) || !x.s.Window.Contains(p.Date) || !counted[p.Date.Format(time.DateOnly)] ||
			x.routine(p.Cell()) || x.mealOf(p.SlotID) != mealFull || x.mainIndex(p) < 0 {
			continue
		}
		if mode == swapMeat && x.plateGroups(p).MeatG <= 0 {
			continue
		}
		out = append(out, i)
	}
	slices.SortStableFunc(out, func(a, b int) int {
		pa, pb := x.s.Plates[a], x.s.Plates[b]
		return cmp.Or(cmp.Compare(x.plateGroups(pb).MeatG, x.plateGroups(pa).MeatG), pa.Date.Compare(pb.Date), cmp.Compare(x.s.SlotOrder[pa.SlotID], x.s.SlotOrder[pb.SlotID]))
	})
	return out
}

// mainIndex is the index of the plate's composed main component, -1 without.
func (x *index) mainIndex(p Plate) int {
	for i := range p.Components {
		if f, ok := x.s.Foods[p.Components[i].FoodID]; ok && f.Composed() && isMainComp(x.s, p, i) {
			return i
		}
	}
	return -1
}

// swapsFor lists the verified swaps for the plates pis (base is the
// evaluation of the state), plate by plate in the given order and, within a
// plate, best first. A swap replaces the plate's main by a main with less
// meat (swapMeat) or by a fish main (swapFish); it is verified by applying it
// to a copy and re-evaluating (tryAlt): no new hard issue and no new soft
// issue, the day stays in its band (or no farther from it than it was), the
// portion cap holds. skip are foods that may not be placed.
func (x *index) swapsFor(t Target, base Evaluation, pis []int, mode swapMode, skip map[int64]bool) []weekSwap {
	hard, soft := hardKeys(base.Issues), softKeys(base.Issues)
	cookedInWindow := map[int64]bool{}
	for _, ev := range x.cooks {
		if x.s.Window.Contains(ev.cell.Date) {
			cookedInWindow[ev.foodID] = true
		}
	}
	dayPct := map[string]float64{}
	for _, d := range base.Days {
		if d.TargetKcal != nil && *d.TargetKcal > 0 {
			dayPct[d.Date] = d.Kcal / *d.TargetKcal * 100
		}
	}
	var out []weekSwap
	for _, pi := range pis {
		p := x.s.Plates[pi]
		old := x.mainIndex(p)
		oc := p.Components[old]
		of := x.s.Foods[oc.FoodID]
		oldMeat := of.Groups.MeatG * oc.Multiplier()
		plateMeat := x.plateGroups(p).MeatG
		oldKcal := of.Macros.Kcal * oc.Multiplier()
		var found []weekSwap
		for _, c := range x.s.altCandidates(x, p, old, AltRequest{What: AltMain}) {
			if skip[c.food.ID] || (c.session != nil && c.grow > 0) {
				continue
			}
			for _, comp := range portionVariants(c.comp, c.food, oldKcal) {
				c.comp = comp
				m := comp.Multiplier()
				newMeat := c.food.Groups.MeatG * m
				switch {
				case mode == swapMeat && newMeat >= oldMeat-1e-9:
					continue
				case mode == swapFish && c.food.Groups.FishG*m < minMealG:
					continue
				}
				a, why, _ := x.s.tryAlt(t, pi, old, nil, c, base, hard, soft, len(x.occasions[dayOf(p.Date)]), cookedInWindow, false)
				if why != "" || len(a.NewIssues) > 0 || !x.dayHolds(a, dayPct[p.Date.Format(time.DateOnly)]) {
					continue
				}
				found = append(found, weekSwap{pi: pi, old: old, c: c, a: a, oldMeatG: oldMeat, newMeatG: newMeat, plateMeat: plateMeat})
			}
		}
		slices.SortStableFunc(found, func(a, b weekSwap) int {
			return cmp.Or(
				boolCmp(a.a.rank.addsCook, b.a.rank.addsCook),
				boolCmp(a.a.rank.weekCook, b.a.rank.weekCook),
				cmp.Compare(math.Round(a.newMeatG/50), math.Round(b.newMeatG/50)),
				cmp.Compare(math.Round(b.a.PlateProteinG/5), math.Round(a.a.PlateProteinG/5)),
				cmp.Compare(a.newMeatG, b.newMeatG),
				cmp.Compare(math.Abs(a.a.DayPct-100), math.Abs(b.a.DayPct-100)),
				cmp.Compare(a.a.rank.source, b.a.rank.source),
				cmp.Compare(a.c.food.ID, b.c.food.ID),
			)
		})
		// One swap per food and plate: the best of its portion variants and sources.
		listed := map[int64]bool{}
		found = slices.DeleteFunc(found, func(s weekSwap) bool {
			if listed[s.c.food.ID] {
				return true
			}
			listed[s.c.food.ID] = true
			return false
		})
		out = append(out, found...)
	}
	return out
}

// portionVariants are the portions a replacement main is tried at: the
// replaced main's, and the amount that keeps its kcal (a lighter dish needs
// more of it), within the portion cap.
func portionVariants(c Component, f FoodFacts, oldKcal float64) []Component {
	out := []Component{c}
	if f.Macros.Kcal <= 0 || oldKcal <= 0 || c.Portions == nil {
		return out
	}
	n := clamp(math.Round(oldKcal/f.Macros.Kcal/roundPortions)*roundPortions, optPortionMin, MaxDishPortions)
	if math.Abs(n-*c.Portions) < 1e-9 {
		return out
	}
	v := c
	v.Portions = &n
	return append(out, v)
}

// dayHolds reports whether the day after the swap is inside the policy band,
// or no farther from its target than before (pct of the target, 0: no target).
func (x *index) dayHolds(a Alternative, before float64) bool {
	if a.DayPct == 0 {
		return true
	}
	if a.DayPct >= x.pol.DayLow*100-1e-9 && a.DayPct <= x.pol.DayHigh*100+1e-9 {
		return true
	}
	return math.Abs(a.DayPct-100) <= math.Abs(before-100)+1e-9
}

// weekOptions are the issue options of the week's meat (swapMeat) or fish
// meal (swapFish): the best swap of each of up to maxWeekOptions plates.
func (x *index) weekOptions(ev Evaluation, t Target, mode swapMode) []Option {
	counted := weekCounted(ev)
	var opts []Option
	for _, pi := range x.swapPlates(counted, mode, x.merged) {
		swaps := x.swapsFor(t, ev, []int{pi}, mode, nil)
		if len(swaps) == 0 {
			continue
		}
		opts = append(opts, x.swapOption(swaps[0]))
		if len(opts) == maxWeekOptions {
			break
		}
	}
	if opts == nil {
		opts = []Option{}
	}
	return opts
}

// cellLabel is "Tuesday dinner".
func (x *index) cellLabel(p Plate) string {
	return p.Date.Weekday().String() + " " + slotName(x.s, p.SlotID)
}

// swapOption writes a swap as an issue option. A persisted plate gets the
// apply_changes ops; a proposed one (no id yet) an explicit replace op that
// names the cell, the recipe, its portions and where it cooks.
func (x *index) swapOption(sw weekSwap) Option {
	p := x.s.Plates[sw.pi]
	o := x.altOption(sw.a, p, 0)
	o.KcalDelta = 0
	if p.Proposed() {
		o.PlateID = nil
		op := OptionOp{Op: "replace", Date: o.Date, SlotID: o.SlotID, FoodID: sw.c.food.ID, Portions: sw.c.comp.Portions, SessionID: sw.a.SessionID, CookAt: sw.a.CookAt}
		o.Op, o.Ops = op, nil
	}
	o.Portions = sw.c.comp.Portions
	o.MeatSavedG = math.Round(sw.oldMeatG - sw.newMeatG)
	of := x.s.Foods[x.s.Plates[sw.pi].Components[sw.old].FoodID]
	how := "cook it fresh on that cell"
	switch {
	case sw.a.SessionID != nil:
		how = "eat it from the batch already cooked"
	case sw.a.CookAt != nil:
		how = fmt.Sprintf("cook it on %s %s", weekdayDate(dateOf(sw.a.CookAt.Date)), slotName(x.s, sw.a.CookAt.SlotID))
	}
	what := fmt.Sprintf("%.0f g meat", sw.newMeatG)
	if g := sw.c.food.Groups.FishG * sw.c.comp.Multiplier(); g >= minMealG {
		what = fmt.Sprintf("%.0f g fish, %s", g, what)
	}
	o.Why = fmt.Sprintf("%s: replace %s (%.0f g meat) by %s (%s, %s portions; %s); day %.0f kcal (%.0f%% of its target), no hard rule broken",
		x.cellLabel(p), of.Name, sw.oldMeatG, sw.c.food.Name, what, num(*sw.c.comp.Portions), how, sw.a.DayKcal, sw.a.DayPct)
	return o
}

func dateOf(s string) time.Time {
	d, _ := time.Parse(time.DateOnly, s)
	return d
}

// weekOptionsFor gives the week's meat and fish issues of ev their swap
// options or, when none is verified, no_fix_found and its reason. It runs once
// ev holds every issue, so the swaps are judged against all of them.
func (x *index) weekOptionsFor(ev *Evaluation, t Target) {
	for i := range ev.Issues {
		switch ev.Issues[i].Code {
		case CodeWeekMeatAboveDGE:
			ev.Issues[i].Params["plates"] = x.weekPlates(weekCounted(*ev))
			x.weekFix(*ev, t, &ev.Issues[i], swapMeat)
		case CodeWeekFishBelowDGE:
			x.weekFix(*ev, t, &ev.Issues[i], swapFish)
		}
	}
}

func (x *index) weekFix(ev Evaluation, t Target, is *Issue, mode swapMode) {
	opts := x.weekOptions(ev, t, mode)
	if len(opts) > 0 {
		is.Options = recommendFirst(opts)
		return
	}
	is.NoFixFound = true
	if mode == swapMeat {
		is.NoFixReason = "no main with less meat fits any plate of the week without breaking a hard rule or leaving its day's energy band; leave the meat as it is or ask the user"
		return
	}
	is.NoFixReason = "no fish main fits any plate of the week (a cell where a fresh cook fits, the day's energy band, the hard rules); leave it or ask the user"
}

// guardWeek is the last guard before a generated plan is saved (as capPortions
// is for the portion cap): while the week holds more meat than its budget it
// swaps the main of the plate with the most meat for a verified main with less
// meat (largest meat plate first), then places one fish meal when the week has
// none and a verified placement exists. Each swap is applied to the proposal
// and the whole plan re-evaluated: it is kept only when it adds no hard or
// soft issue, lowers the week's meat (or adds the fish meal) and keeps the
// weekly protein at 90 % of its target or no lower than it was. Only the
// proposal's own plates change. skip are foods the target may not hold. One
// plain-words note per change.
func guardWeek(base State, t Target, in Proposal, skip map[int64]bool, instruction string) (Proposal, []string) {
	out := in
	var notes []string
	step := func(mode swapMode) bool {
		merged := Merge(base, t, out)
		ev := Evaluate(merged, t)
		w := ev.Week
		if w == nil || w.Days < minDGEDays {
			return false
		}
		x := newIndex(merged, t)
		hard, soft := hardKeys(ev.Issues), softKeys(ev.Issues)
		for _, pi := range x.swapPlates(weekCounted(ev), mode, true) {
			tries := 0
			for _, sw := range x.swapsFor(t, ev, []int{pi}, mode, skip) {
				if tries++; tries > maxGuardTries {
					break
				}
				next, ok := applySwap(out, merged, sw)
				if !ok {
					continue
				}
				after := Evaluate(Merge(base, t, next), t)
				if !guardKeeps(ev, after, hard, soft, mode) {
					continue
				}
				out = next
				notes = append(notes, x.swapNote(sw, ev, after, mode))
				return true
			}
		}
		return false
	}
	budget := func() bool {
		ev := Evaluate(Merge(base, t, out), t)
		return ev.Week != nil && ev.Week.Days >= minDGEDays && meatAllowed(base) && ev.Week.MeatG > DGEMeatMaxG*float64(ev.Week.Days)/weekDays+1e-9
	}
	for i := 0; i < maxGuardSteps && budget(); i++ {
		if !step(swapMeat) {
			break
		}
	}
	if ev := Evaluate(Merge(base, t, out), t); fishAllowed(base) && !noFish.MatchString(instruction) && ev.Week != nil && ev.Week.Days >= minDGEDays && ev.Week.FishMeals == 0 {
		step(swapFish)
	}
	return out, notes
}

// guardKeeps reports whether the plan after a swap is better: no new hard or
// soft issue, less meat (or a fish meal), weekly protein not pushed below 90 %.
func guardKeeps(before, after Evaluation, hard, soft map[string]bool, mode swapMode) bool {
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
	b, a := before.Week, after.Week
	if a == nil || b == nil {
		return false
	}
	if a.TargetProteinG > 0 && a.ProteinPct < math.Min(b.ProteinPct, ProteinWeekMin*100)-1e-9 {
		return false
	}
	if mode == swapFish {
		return a.FishMeals > b.FishMeals
	}
	return a.MeatG < b.MeatG-1e-9
}

// applySwap writes sw into the proposal: the replaced main's component
// becomes the new recipe at its portions (fresh, from a batch, or cooked on
// its own new session), and the proposed session the old main ate from
// shrinks by what the plate no longer eats (or goes when nothing is left).
func applySwap(in Proposal, merged State, sw weekSwap) (Proposal, bool) {
	p := merged.Plates[sw.pi]
	i := int(-p.ID) - 1
	if i < 0 || i >= len(in.Plates) || in.Plates[i].PlateID != 0 || sw.old >= len(in.Plates[i].Components) {
		return in, false
	}
	out := in
	out.Plates = slices.Clone(in.Plates)
	out.Sessions = slices.Clone(in.Sessions)
	pl := out.Plates[i]
	pl.Components = slices.Clone(pl.Components)
	oc := pl.Components[sw.old]
	nc := ProposedComponent{FoodID: sw.c.food.ID, Portions: *sw.c.comp.Portions}
	switch {
	case sw.c.cookAt != nil:
		key := fmt.Sprintf("swap-%d", len(out.Sessions)+1)
		for slices.ContainsFunc(out.Sessions, func(s ProposedSession) bool { return s.Key == key }) {
			key += "x"
		}
		out.Sessions = append(out.Sessions, ProposedSession{
			Key: key, FoodID: sw.c.food.ID, CookDate: sw.c.cookAt.Date, CookSlotID: sw.c.cookAt.SlotID,
			Portions: nc.Portions * p.Eaters, Reason: "cooked for the swapped main",
		})
		nc.SessionKey = key
	case sw.c.session != nil:
		nc.SessionID = sw.c.session.ID
	}
	if oc.SessionKey != "" {
		for k, ss := range out.Sessions {
			if ss.Key != oc.SessionKey {
				continue
			}
			out.Sessions[k].Portions = ss.Portions - p.Eaters*oc.Portions
			out.Sessions[k].FrozenPortions = min(ss.FrozenPortions, max(out.Sessions[k].Portions, 0))
			if out.Sessions[k].Portions <= epsilon {
				out.Sessions = slices.Delete(out.Sessions, k, k+1)
			}
			break
		}
	}
	pl.Components[sw.old] = nc
	out.Plates[i] = pl
	return out, true
}

// swapNote is the run note of one guard swap.
func (x *index) swapNote(sw weekSwap, before, after Evaluation, mode swapMode) string {
	p := x.s.Plates[sw.pi]
	old := x.s.Foods[p.Components[sw.old].FoodID].Name
	if mode == swapFish {
		return fmt.Sprintf("Weekly fish: %s: %s replaced by %s so the week holds a fish meal (the weekly food rules ask for one).", x.cellLabel(p), old, sw.c.food.Name)
	}
	return fmt.Sprintf("Weekly meat: %s: %s replaced by %s to bring the week's meat from %.0f g toward its limit of %.0f g (now %.0f g).",
		x.cellLabel(p), old, sw.c.food.Name, before.Week.MeatG, DGEMeatMaxG*float64(before.Week.Days)/weekDays, after.Week.MeatG)
}

// weekAdvisories are the repair advisories of the week's meat and fish
// issues: each is raised only when a verified swap exists (ev from
// EvaluateWeek), and carries the swaps in words.
func weekAdvisories(ev Evaluation, instruction string) []string {
	var out []string
	for _, is := range ev.Issues {
		if is.Rule != "S7" || is.NoFixFound || len(is.Options) == 0 {
			continue
		}
		var lines []string
		for _, o := range is.Options {
			lines = append(lines, o.Why)
		}
		switch is.Code {
		case CodeWeekMeatAboveDGE:
			out = append(out, fmt.Sprintf("The week holds %v g of meat per person, above the %v g limit for its %v days (weekly meat rule). Verified fix, take swaps largest first until the week is within the limit (each keeps its day in band and breaks no rule): %s.",
				is.Params["meat_g"], is.Params["max_g"], is.Params["days"], strings.Join(lines, "; ")))
		case CodeWeekFishBelowDGE:
			if noFish.MatchString(instruction) {
				continue
			}
			out = append(out, fmt.Sprintf("The week holds no fish meal (weekly fish rule). Verified fix, take one of: %s.", strings.Join(lines, "; ")))
		}
	}
	return out
}
