package planning

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
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
	before := evaluateLean(Merge(in.State, in.Target, prop), in.Target)
	hard := hardKeys(before.Issues)
	var best *planner
	var bestEv Evaluation
	var dropped string
	// Two variants: trimming the flagged add-ons first, and lifting only. The
	// week outweighs the add-on rule, so trimming that leaves the week short
	// loses to keeping the add-ons.
	for _, trim := range []bool{true, false} {
		h := newPlanner(in)
		h.balance, h.sided = true, map[int]bool{}
		// A model's plan gets sides that fit the plate, by the rule the chat uses (addOnFits).
		h.s.PairAddOns = pairInPlanner
		h.prop = cloneProposal(prop)
		if trim {
			h.trimAddOns()
		}
		// One pass leaves what its caps and order hid (a day lifted early can
		// free room for another); repeat while the week still gains.
		for range maxBalanceRounds {
			pct := weekPct(evaluateLean(Merge(in.State, in.Target, h.prop), in.Target))
			h.topUpFloors()
			if weekPct(evaluateLean(Merge(in.State, in.Target, h.prop), in.Target))-pct < 0.1 {
				break
			}
		}
		h.growShort()
		after := evaluateLean(Merge(in.State, in.Target, h.prop), in.Target)
		if k := newHard(hard, after); k != "" {
			dropped = "Balancing was dropped: it would have added a hard-rule issue (" + k + ")."
			continue
		}
		if cookDays(after) > cookDays(before) {
			dropped = "Balancing was dropped: it would have added cooking days."
			continue
		}
		if best == nil || balanceScore(after) > balanceScore(bestEv) {
			best, bestEv = h, after
		}
	}
	if best == nil {
		return prop, []string{dropped}
	}
	notes := append(balanceNotes(in.State, prop, best.prop), best.blockers(bestEv)...)
	if n := addOnExcess(bestEv); n > 0 {
		notes = append(notes, fmt.Sprintf("Add-on excess stays on %d plate(s): removing it left the week under its band, and the weekly energy ranks higher.", n))
	}
	return best.prop, notes
}

// newHard names a hard issue of ev that base lacks ("" when none).
func newHard(base map[string]bool, ev Evaluation) string {
	for k := range hardKeys(ev.Issues) {
		if !base[k] {
			return k
		}
	}
	return ""
}

func addOnExcess(ev Evaluation) int {
	n := 0
	for _, is := range ev.Issues {
		if is.Code == CodeAddOnExcess {
			n++
		}
	}
	return n
}

// balanceScore orders results: the week inside its band first, then fewer
// add-on excess issues, then the week closer to its target.
func balanceScore(ev Evaluation) float64 {
	score := -math.Abs(weekPct(ev) - 100)
	if weekPct(ev) >= 95 {
		score += 1000
	}
	for _, is := range ev.Issues {
		if is.Code == CodeAddOnExcess {
			score -= 10
		}
	}
	return score
}

// maxBalanceRounds bounds the repeats of the lift.
const maxBalanceRounds = 3

func weekPct(ev Evaluation) float64 {
	if ev.Week == nil {
		return 0
	}
	return ev.Week.Pct
}

// cookDays counts the days with a cooking occasion.
func cookDays(ev Evaluation) int {
	n := 0
	for _, d := range ev.Days {
		if d.CookOccasions > 0 {
			n++
		}
	}
	return n
}

// blockers says, per day still under its band or the week under 96 %, what
// stopped the lift: the add-on limit used, no side that fits, portions at the
// cap, a light meal. Nothing when the plan is balanced.
func (h *planner) blockers(ev Evaluation) []string {
	var out []string
	for _, d := range ev.Days {
		if !d.Complete || d.TargetKcal == nil || d.Kcal >= d.BandLow-0.5 {
			continue
		}
		var why []string
		if len(h.s.Light) > 0 && (h.s.lightDay(dateOf(d.Date)) || len(d.LightMeals) > 0) {
			why = append(why, "a light meal takes no add-ons")
		}
		if a := h.adds[dayOf(dateOf(d.Date))]; a != nil && a.filler >= maxFillerAdds {
			why = append(why, "the day's add-on limit is used")
		}
		if !h.sided[dayOf(dateOf(d.Date))] {
			why = append(why, "no side recipe fits the plates without a new cooking occasion")
		}
		why = append(why, "portions are at the 1.5 cap or cook with a fixed batch")
		out = append(out, fmt.Sprintf("Balancing left %s %.0f kcal under its band: %s.", dateOf(d.Date).Weekday(), d.BandLow-d.Kcal, strings.Join(why, "; ")))
	}
	if w := ev.Week; w != nil && w.Pct < 96 && len(out) == 0 {
		out = append(out, fmt.Sprintf("Balancing left the week at %.1f %% of its target: every day is in its band and the add-on, side and portion limits are used.", w.Pct))
	}
	return out
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

// meatFull reports whether the plan already holds the week's meat budget: a
// dish with meat then keeps its portions (growing it would pass the limit).
func (h *planner) meatFull() bool {
	if !meatAllowed(h.s) {
		return false
	}
	ev := evaluateLean(Merge(h.s, h.in.Target, h.prop), h.in.Target)
	w := ev.Week
	return w != nil && w.Days >= minDGEDays && w.MeatG >= DGEMeatMaxG*float64(w.Days)/weekDays-1
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
	if dk == nil || !dk.Complete || dk.NutritionMissing || dk.Kcal <= 0 || limit-dk.Kcal < minAddGrams*2 {
		return
	}
	need := limit - dk.Kcal
	base := evaluateLean(merged, h.in.Target)
	hard, soft := hardKeys(base.Issues), softKeys(base.Issues)
	cooked := map[int64]bool{}
	for _, ev := range y.cooks {
		cooked[ev.foodID] = true
	}
	held := map[int64]bool{}
	served := map[int64]int{} // plates of the window per food: the week's side rotation
	for _, p := range merged.Plates {
		if dayOf(p.Date) == dayOf(date) {
			for _, c := range p.Components {
				held[c.FoodID] = true
			}
		}
		if merged.Window.Contains(p.Date) {
			for _, c := range p.Components {
				served[c.FoodID]++
			}
		}
	}
	type opt struct {
		pi     int
		c      altCand
		a      Alternative
		raised int
		prot   bool // a second protein: only the floor lift's last resort
	}
	var opts []opt
	for _, pi := range y.editablePlates(date) {
		p := merged.Plates[pi]
		if !p.Proposed() || y.mealOf(p.SlotID) != mealFull {
			continue
		}
		for _, c := range merged.altCandidates(y, p, -1, AltRequest{What: AltAdd}) {
			f := c.food
			if !f.Composed() || !sideRole(f.Role) || f.Macros.Kcal <= 0 || held[f.ID] || h.exclude[f.ID] || (h.s.RatherNot[f.ID] && !h.floorLift) || (c.session != nil && c.grow > 0) || c.cookAt != nil ||
				f.Macros.Kcal*c.comp.Multiplier() > dk.Adjusted*maxSideKcalShare {
				continue
			}
			if h.s.Settings.PlateFit && !h.floorLift && merged.sidesBeside(p, f) >= maxPlateSides {
				continue // no clutter: the main and at most two sides or add-ons
			}
			on, clash, raised := merged.plateFitOf(p, f, h.floorLift)
			if on && clash {
				continue
			}
			a, why, _ := merged.tryAlt(h.in.Target, pi, -1, nil, c, base, hard, soft, len(y.occasions[dayOf(p.Date)]), cooked, false, false)
			if why != "" || len(a.NewIssues) > 0 || a.rank.addsCook || a.DayKcal > dk.Adjusted*y.pol.DayHigh ||
				math.Abs(dk.Adjusted-a.DayKcal) >= math.Abs(dk.Adjusted-dk.Kcal) {
				continue
			}
			prot := on && h.floorLift && merged.secondProtein(merged.mainOf(p), plateFoods(p), f)
			opts = append(opts, opt{pi, c, a, raised, prot})
		}
	}
	slices.SortStableFunc(opts, func(a, b opt) int {
		// Every option holds the day's nutrition (verified above). A side
		// the household would rather not get is only an option for the
		// floor lift (filtered above), after every other option, before
		// plate fit.
		// A second protein (the floor lift only) comes after every other
		// side the household takes.
		if c := cmp.Or(boolCmp(h.s.RatherNot[a.c.food.ID], h.s.RatherNot[b.c.food.ID]), boolCmp(a.prot, b.prot)); c != 0 {
			return c
		}
		if h.s.Settings.PlateFit {
			// Plate fit after nutrition: the least fat above the week's
			// range (excessFat), then the side the week serves least and
			// the one eaten longest ago (its rotation), then within
			// kcalBucket of the need, the plate with fewer raised gaps first
			// (PLATE-4).
			ka, kb := math.Abs(need-a.c.food.Macros.Kcal*a.c.comp.Multiplier()), math.Abs(need-b.c.food.Macros.Kcal*b.c.comp.Multiplier())
			fa, fb := excessFat(a.c.food.Macros, a.c.comp.Multiplier(), y.pol.FatHigh), excessFat(b.c.food.Macros, b.c.comp.Multiplier(), y.pol.FatHigh)
			if c := cmp.Or(boolCmp(a.a.rank.addsCook, b.a.rank.addsCook), cmp.Compare(fa, fb), cmp.Compare(served[a.c.food.ID], served[b.c.food.ID]),
				cmp.Compare(lastEaten(a.c.food), lastEaten(b.c.food)), cmp.Compare(math.Round(ka/kcalBucket), math.Round(kb/kcalBucket)), cmp.Compare(a.raised, b.raised)); c != 0 {
				return c
			}
		}
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

// growShort is the last lift: what the passes above left under (the day-count
// and size caps of the adds were used on gaps that have since moved), by
// growing the adds the days already hold (growAddOns): the days under their
// band up to its low end, then, while the week is under its target, the days
// furthest under theirs, never by more than the week still lacks.
func (h *planner) growShort() {
	y, days := h.dayAccounts()
	for _, d := range days {
		if low := d.Adjusted * y.pol.DayLow; d.Kcal < low-epsilon {
			h.growAddOns(d.Date, low)
		}
	}
	for range days {
		_, days = h.dayAccounts()
		gap := 0.0
		var worst *DayKcal
		for i, d := range days {
			gap += d.Gap
			if d.Gap > 0 && (worst == nil || d.Gap > worst.Gap) {
				worst = &days[i]
			}
		}
		if gap <= 0 || worst == nil {
			return
		}
		before := worst.Kcal
		h.growAddOns(worst.Date, before+min(worst.Gap, gap))
		if _, after := h.dayAccounts(); slices.IndexFunc(after, func(a DayKcal) bool { return dayOf(a.Date) == dayOf(worst.Date) && a.Kcal > before+epsilon }) < 0 {
			return
		}
	}
}

// growAddOns raises the no-cook adds a day already holds, up to their caps
// (addCapG), while the day is under limit: the day's add-on count is used
// up, but its adds were sized to the gap of the moment and can carry more.
// The denser food grows first; a step that adds a hard or add-on issue, or
// takes the day past its band top, is dropped. Balancing a model's plan only.
func (h *planner) growAddOns(date time.Time, limit float64) {
	base := evaluateLean(Merge(h.s, h.in.Target, h.prop), h.in.Target)
	hard := hardKeys(base.Issues)
	for range maxDayAddOns * 2 {
		y, days := h.dayAccounts()
		var dk *DayKcal
		for i := range days {
			if dayOf(days[i].Date) == dayOf(date) {
				dk = &days[i]
			}
		}
		if dk == nil || limit-dk.Kcal < epsilon {
			return
		}
		type grow struct {
			pi, ci int
			grams  float64
			dens   float64
		}
		var opts []grow
		merged := Merge(h.s, h.in.Target, h.prop)
		for pi, p := range h.prop.Plates {
			if dayOf(p.Date) != dayOf(date) || p.PlateID != 0 || h.s.lightMeal(p.Date, p.SlotID) {
				continue
			}
			for ci, c := range p.Components {
				f, ok := h.s.Foods[c.FoodID]
				if !ok || c.Grams <= 0 || f.Macros.Kcal <= 0 || !f.NutritionComplete {
					continue
				}
				// An add that does not fit its plate (a nut beside a pasta) is not made bigger.
				if mi := slices.IndexFunc(merged.Plates, func(mp Plate) bool { return mp.ID == -int64(pi+1) }); mi >= 0 && !merged.addOnFits(merged.Plates[mi], f, slotName(merged, p.SlotID)) {
					continue
				}
				// Never past the usual serving (or where the add already was).
				capG := math.Max(c.Grams, math.Min(addCapG(f, dk.Adjusted), ServingGrams(f)))
				g := math.Min(capG, math.Ceil((c.Grams+(limit-dk.Kcal)*100/f.Macros.Kcal)/roundGrams)*roundGrams)
				if g > c.Grams+epsilon {
					opts = append(opts, grow{pi, ci, g, f.Macros.Kcal})
				}
			}
		}
		slices.SortStableFunc(opts, func(a, b grow) int { return cmp.Compare(b.dens, a.dens) })
		done := false
		for _, o := range opts {
			next := cloneProposal(h.prop)
			next.Plates[o.pi].Components[o.ci].Grams = o.grams
			merged := Merge(h.s, h.in.Target, next)
			ny := newIndex(merged, h.in.Target)
			var after *DayKcal
			for _, a := range ny.dayAccounts([]time.Time{date}) {
				after = &a
			}
			if after == nil || after.Kcal <= dk.Kcal || after.Kcal > dk.Adjusted*y.pol.DayHigh {
				continue
			}
			if newHard(hard, evaluateLean(merged, h.in.Target)) != "" {
				continue
			}
			if base := y.baseAddOnKeys(); slices.ContainsFunc(slices.Collect(maps.Keys(ny.addOnKeys())), func(k string) bool { return !base[k] }) {
				continue
			}
			h.prop, done = next, true
			break
		}
		if !done {
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

// lastEaten is when f was last eaten, in Unix seconds (0: never).
func lastEaten(f FoodFacts) int64 {
	if f.LastEaten == nil {
		return 0
	}
	return f.LastEaten.Unix()
}
