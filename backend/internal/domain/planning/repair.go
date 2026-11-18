package planning

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// maxFeedbackLines caps the problems one repair message lists.
const maxFeedbackLines = 10

// repairer renders the repair feedback of one proposal: what the checker
// found as rules in plain words with the numbers, the cells and keys
// involved, and a concrete fix. It reads the merged state the checker saw.
type repairer struct {
	in   ChooseInput
	prop Proposal
	s    State
	x    *index
}

func newRepairer(in ChooseInput, prop Proposal) *repairer {
	s := Merge(in.State, in.Target, prop)
	return &repairer{in: in, prop: prop, s: s, x: newIndex(s, in.Target)}
}

// advisories are the findings that are no hard rule: a lunch or dinner
// without a main sends the plan back like a violation does, and so does a
// side swap that changes the side's role. Calories are never a repair
// trigger (kcalNote).
func (r *repairer) advisories() []string {
	if r.in.Side != nil {
		return r.sideRoleChanged()
	}
	return append(r.missingMains(), r.weekAdvice()...)
}

// weekAdvice are the week's meat and fish advisories (weekAdvisories), kept
// only where the last guard would act on the plan: a swap that holds on the
// whole plan, not just on its own dry run.
func (r *repairer) weekAdvice() []string {
	if distinctDays(r.in.Target) < minDGEDays {
		return nil // a day or a plate regenerate does not own the week
	}
	meat := meatAllowed(r.in.State)
	if !meat && !fishAllowed(r.in.State) {
		return nil
	}
	adv := weekAdvisories(EvaluateWeek(r.s, r.in.Target), r.in.Instruction)
	if len(adv) == 0 {
		return nil
	}
	_, notes := guardWeek(r.in.State, r.in.Target, r.prop, idSet(r.in.Exclude), r.in.Instruction)
	return slices.DeleteFunc(adv, func(a string) bool {
		prefix := "Weekly fish"
		if strings.HasPrefix(a, "The week holds ") && !strings.HasPrefix(a, "The week holds no fish") {
			prefix = "Weekly meat"
		}
		return !slices.ContainsFunc(notes, func(n string) bool { return strings.HasPrefix(n, prefix) })
	})
}

// sideRoleChanged reports a side swap whose new side has another role than
// the replaced one (a veg side swapped for a second starch).
func (r *repairer) sideRoleChanged() []string {
	sw := r.in.Side
	if !roleKept(r.in) {
		return nil // no other side of that role exists: any side will do
	}
	var out []string
	for _, ss := range r.s.Sessions {
		if !ss.Proposed() {
			continue
		}
		if f, ok := r.x.food(ss.FoodID); ok && f.Role != sw.Role {
			out = append(out, fmt.Sprintf("%s has role %s, but the replaced side %s has role %s. Fix: keep the role; choose a candidate with role %s.",
				foodName(r.s, ss.FoodID), roleName(f.Role), foodName(r.s, sw.FoodID), sw.Role, sw.Role))
		}
	}
	return out
}

func roleName(r food.Role) string {
	if r == "" {
		return "none"
	}
	return string(r)
}

func (r *repairer) missingMains() []string {
	if !hasFullMains(r.s) {
		return nil
	}
	var out []string
	for _, p := range r.s.Plates {
		if !p.Proposed() || !r.x.inTarget(p.Cell()) || r.x.routine(p.Cell()) {
			continue
		}
		kind := r.x.mealOf(p.SlotID)
		hasMain, unknown := false, false
		var names []string
		for _, c := range p.Components {
			f, ok := r.x.food(c.FoodID)
			unknown = unknown || !ok
			names = append(names, foodName(r.s, c.FoodID))
			if ok && isMain(f.Role, kind) {
				hasMain = true
			}
		}
		if !hasMain && !unknown {
			out = append(out, fmt.Sprintf("%s has no main (only %s). Fix: every lunch and dinner plate needs a recipe with role main (a standalone recipe such as oats is a breakfast, not a main); add one (or eat a main from a session) and keep the side next to it.",
				r.cellLabel(p.Cell()), strings.Join(names, " + ")))
		}
	}
	return out
}

// findings is what one attempt's proposal left to repair: violations, the
// advisories, the corrections already applied in Go, and whether the
// proposal equals the previous attempt's.
type findings struct {
	violations []Issue
	advice     []string
	fixes      []string
	repeated   bool
}

// feedback is the repair message.
func (r *repairer) feedback(f findings) string {
	vs, adv, fixes, repeated := f.violations, f.advice, f.fixes, f.repeated
	var b strings.Builder
	if repeated {
		b.WriteString("Your last reply is identical to the one before it, so nothing was repaired. Change the cells named below; resubmitting the same plan fails again.\n")
	}
	if len(fixes) > 0 {
		b.WriteString("Already corrected for you (keep these in your next submit_plan):\n")
		for _, f := range fixes {
			fmt.Fprintf(&b, "- %s\n", f)
		}
	}
	seen := map[string]bool{}
	var lines []string
	for _, v := range vs {
		text := r.describe(v, seen)
		if text == "" || seen["text:"+text] {
			continue
		}
		seen["text:"+text] = true
		lines = append(lines, text)
	}
	lines = append(lines, adv...)
	if len(lines) > maxFeedbackLines {
		lines = append(lines[:maxFeedbackLines:maxFeedbackLines], fmt.Sprintf("and %d more problem(s) of the same kind; fix the ones above first.", len(lines)-maxFeedbackLines))
	}
	fmt.Fprintf(&b, "Your proposal has %d problem(s). Edit it, keeping everything that is not named, and call submit_plan again with the complete plan (every session and plate):\n", len(lines))
	for i, l := range lines {
		fmt.Fprintf(&b, "%d. %s\n", i+1, l)
	}
	b.WriteString("Reminder: portions on a plate component are per eater (eaters x portions is what it eats from a session); a session's portions must cover the sum of its plates, including the plate on its cook cell.")
	return b.String()
}

func (r *repairer) cellLabel(c plate.CellKey) string {
	return fmt.Sprintf("%s %s (slot_id %d)", weekdayDate(c.Date), slotName(r.s, c.SlotID), c.SlotID)
}

func (r *repairer) issueCell(v Issue) string {
	if v.Date == "" {
		return "the plan"
	}
	d, err := time.Parse(time.DateOnly, v.Date)
	if err != nil {
		return v.Date
	}
	if v.SlotID == nil {
		return weekdayDate(d)
	}
	return r.cellLabel(plate.CellKey{Date: d, SlotID: *v.SlotID})
}

func intParam(v Issue, k string) int64 {
	switch n := v.Params[k].(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	}
	return 0
}

// describe renders one violation; "" when it was already covered (a session
// is described once for all its H3/H4 findings).
func (r *repairer) describe(v Issue, seen map[string]bool) string {
	where := r.issueCell(v)
	switch v.Rule + "." + v.Code {
	case "H1." + CodeMissingPlate:
		return fmt.Sprintf("H1: %s is a fill cell without a plate. Fix: add one plate for it (date %s, slot_id %d).", where, v.Date, slotOf(v))
	case "H1." + CodeExtraPlate:
		return fmt.Sprintf("H1: %s holds %d plates, exactly one is allowed. Fix: keep one plate for that cell.", where, intParam(v, "count"))
	case "H1." + CodePlateInNoneCell:
		return fmt.Sprintf("H1: %s is a no-plate cell. Fix: remove the plate you put there.", where)
	case "H1." + CodeCookInNoneCell:
		return fmt.Sprintf("H1: %s is a no-plate cell, nobody is home to cook there. Fix: cook that session on a plan cell instead.", where)
	case "H2." + CodeTooManyCookOccasions:
		return r.describeH2(v)
	case "H3." + plate.WarnDeficit, "H3." + plate.WarnFrozenDeficit, "H4." + plate.WarnLeftoverBeforeCook, "H4." + plate.WarnPastKeepDays:
		return r.describeSession(v, seen)
	case "H5." + CodeDietIncompatible:
		return fmt.Sprintf("H5: %s uses %s (diet class %v) but the diet type is %v. Fix: replace it with a compatible recipe.",
			where, foodName(r.s, intParam(v, "food_id")), v.Params["diet_class"], v.Params["diet_type"])
	case "H5." + CodeMeatDaysExceeded:
		return fmt.Sprintf("H5: %d days of the week have an omnivore recipe, at most %d are allowed. Fix: replace the omnivore recipes of %d day(s) with vegetarian, vegan or pescetarian ones.",
			intParam(v, "days"), intParam(v, "max"), intParam(v, "days")-intParam(v, "max"))
	case "H8." + CodeVarietyRepeat:
		return fmt.Sprintf("H8: %s cooks %s again, last eaten or cooked %v (within %d weeks). Fix: choose a different main or standalone recipe there, with last_eaten empty or older.",
			where, foodName(r.s, intParam(v, "food_id")), v.Params["last_eaten"], r.s.Settings.VarietyWeeks)
	case "H9." + CodeKcalBelowFloor:
		return fmt.Sprintf("H9: %s holds %d kcal per person, below the safety floor of %d kcal for that day (its target is %d). This is a hard rule, never planned below. Fix: raise the portions of that day's plates or add a side or snack until the day reaches at least %d kcal.",
			v.Date, intParam(v, "kcal"), intParam(v, "floor_kcal"), intParam(v, "target_kcal"), intParam(v, "floor_kcal"))
	case "H7." + CodeUnknownFood:
		return fmt.Sprintf("H7: %s names food_id %d, which is not a candidate recipe. Fix: use only ids from the candidate table.", where, intParam(v, "food_id"))
	case "H7." + CodeNotComposed:
		return fmt.Sprintf("H7: %s uses food_id %d, which is not a recipe. Fix: use a candidate recipe id.", where, intParam(v, "food_id"))
	case "H7." + CodeUnknownSession:
		return fmt.Sprintf("H7: the plate at %s eats from session %v%v, which does not exist. Fix: define that session in sessions or use an existing session_id.",
			where, orEmpty(v.Params["session_key"]), orEmpty(v.Params["session_id"]))
	case "H7." + CodeSessionFoodMismatch:
		return fmt.Sprintf("H7: the plate at %s eats %s from a session of %s. Fix: a component must have the same food_id as its session.",
			where, foodName(r.s, intParam(v, "food_id")), foodName(r.s, intParam(v, "session_food_id")))
	case "H7." + CodeDuplicateSessionKey:
		return fmt.Sprintf("H7: session key %v is used more than once. Fix: give every session its own key.", v.Params["session_key"])
	case "H7." + CodeInvalidPortions:
		return fmt.Sprintf("H7: portions or eaters at %s are not positive, or frozen_portions is negative or above portions. Fix: use positive numbers and 0 <= frozen_portions <= portions.", where)
	case "H7." + CodePortionsOverCap:
		return fmt.Sprintf("H7: %s has %s at %v portions per person, the most allowed is %v. Fix: keep the dish at %v or less and close the energy gap with a side or a no-cook add-on (the S2/S3 options of the day list them).",
			where, foodName(r.s, intParam(v, "food_id")), v.Params["portions"], v.Params["max_portions"], v.Params["max_portions"])
	case "H7." + CodePlateWithoutComponent:
		return fmt.Sprintf("H7: the plate at %s has no component. Fix: give it at least one.", where)
	case "H7." + CodePlateOutsideTarget, "H6." + CodeSessionOutsideTarget:
		return fmt.Sprintf("%s: %s is not a fill cell. Fix: plates and cook sessions go only on cells marked fill.", v.Rule, where)
	}
	// Anything else keeps the plain line.
	line := fmt.Sprintf("%s %s at %s", v.Rule, v.Code, where)
	for _, k := range slices.Sorted(maps.Keys(v.Params)) {
		line += fmt.Sprintf(" %s=%v", k, v.Params[k])
	}
	return line + ". Fix: change the plan so this rule holds."
}

func slotOf(v Issue) int64 {
	if v.SlotID == nil {
		return 0
	}
	return *v.SlotID
}

func orEmpty(v any) any {
	if v == nil {
		return ""
	}
	return v
}

// describeH2 lists the cells that cook on the day.
func (r *repairer) describeH2(v Issue) string {
	d, _ := time.Parse(time.DateOnly, v.Date)
	type occ struct {
		cell  plate.CellKey
		cooks []string
	}
	var occs []occ
	add := func(c plate.CellKey, what string) {
		if r.x.routine(c) {
			return
		}
		for i := range occs {
			if occs[i].cell.Equal(c) {
				occs[i].cooks = append(occs[i].cooks, what)
				return
			}
		}
		occs = append(occs, occ{cell: c, cooks: []string{what}})
	}
	for _, ss := range r.s.Sessions {
		if dayOf(ss.CookDate) == dayOf(d) {
			add(ss.CookCell(), r.sessionLabel(ss))
		}
	}
	for _, p := range r.s.Plates {
		if dayOf(p.Date) != dayOf(d) {
			continue
		}
		for _, c := range p.Components {
			if _, linked := refOf(c); !linked && c.Portions != nil {
				add(p.Cell(), foodName(r.s, c.FoodID)+" without a session (cooked fresh)")
			}
		}
	}
	slices.SortFunc(occs, func(a, b occ) int { return plate.Compare(a.cell, b.cell, r.s.SlotOrder) })
	parts := make([]string, len(occs))
	for i, o := range occs {
		parts[i] = fmt.Sprintf("%s: %s", r.cellLabel(o.cell), strings.Join(o.cooks, ", "))
	}
	return fmt.Sprintf("H2: %s has %d cooking occasions, at most %d are allowed (an occasion is a distinct cook cell; a component without session_key/session_id cooks fresh and counts too). Cooking on that day: %s. Fix: cook all that day's sessions on one cell (several sessions on one cell are one occasion), and let the other plates eat from a session instead of cooking fresh.",
		weekdayDate(d), intParam(v, "count"), intParam(v, "max"), strings.Join(parts, "; "))
}

func (r *repairer) sessionLabel(ss Session) string {
	id := fmt.Sprintf("session_id %d", ss.ID)
	if ss.Proposed() {
		id = ss.Key
	}
	return fmt.Sprintf("%s (%s, food_id %d)", id, foodName(r.s, ss.FoodID), ss.FoodID)
}

// describeSession explains every H3/H4 finding of the violation's session
// at once: the consumption table, the keep-days limits and the fix.
func (r *repairer) describeSession(v Issue, seen map[string]bool) string {
	var ref sessionRef
	if k, ok := v.Params["session_key"].(string); ok && k != "" {
		ref = sessionRef{key: k}
	} else if v.SessionID != nil {
		ref = sessionRef{id: *v.SessionID}
	}
	si, ok := r.x.byRef[ref]
	if !ok {
		return ""
	}
	label := fmt.Sprintf("%v/%d", ref.key, ref.id)
	if seen[label] {
		return ""
	}
	seen[label] = true

	ss := r.s.Sessions[si]
	set := r.s.Settings.BatchSettings
	keep := r.x.keepDays(ss.FoodID)
	useBy := plate.UseBy(ss.CookSession, keep)
	freezerLimit := ss.CookDate.AddDate(0, 0, set.FreezerKeepDays)
	links := plate.OrderLinks(ss.CookSession, r.x.links[ref], keep, r.s.SlotOrder)
	use := plate.Consumption(ss.CookSession, links, keep)
	f := r.s.Foods[ss.FoodID]

	var b strings.Builder
	fmt.Fprintf(&b, "H3/H4 %s: cooked %s with portions %s, frozen_portions %s. Plates eating from it (eaters x portions per eater = consumption):\n",
		r.sessionLabel(ss), r.cellLabel(ss.CookCell()), num(ss.Portions), num(ss.FrozenPortions))
	var frozenCells, lateCells, earlyCells []string
	for _, l := range links {
		note := string(l.Kind)
		switch {
		case plate.Before(l.Cell, ss.CookCell(), r.s.SlotOrder):
			note = "BEFORE the cook cell"
			earlyCells = append(earlyCells, l.Cell.Date.Format(time.DateOnly)+" slot_id "+fmt.Sprint(l.Cell.SlotID))
		case l.Kind == plate.KindFrozen:
			note = "frozen: after the keep days"
			frozenCells = append(frozenCells, l.Cell.Date.Format(time.DateOnly))
			if !set.FreezingEnabled || l.Cell.Date.After(freezerLimit) {
				lateCells = append(lateCells, l.Cell.Date.Format(time.DateOnly))
			}
		}
		fmt.Fprintf(&b, "     %s: %s x %s = %s (%s)\n", r.cellLabel(l.Cell), num(l.Eaters), num(l.PerEater), num(l.Portions), note)
	}
	fmt.Fprintf(&b, "   Consumption %s portions (of which frozen %s) against portions %s and frozen_portions %s. Keep days %d: plates up to %s eat it fresh or as leftovers; plates after that are frozen links.",
		num(use.Total), num(use.Frozen), num(ss.Portions), num(ss.FrozenPortions), keep, useBy.Format(time.DateOnly))
	b.WriteString("\n   Fix:")
	if len(earlyCells) > 0 {
		fmt.Fprintf(&b, " move the cook to the earliest plate (%s) or move that plate after the cook cell;", strings.Join(earlyCells, ", "))
	}
	if overBy(use.Total, ss.Portions) {
		fmt.Fprintf(&b, " portions must be at least %s;", num(use.Total))
	}
	switch {
	case len(frozenCells) == 0:
	case !set.FreezingEnabled:
		fmt.Fprintf(&b, " freezing is off, so plates after %s cannot eat from it (%s): give them another recipe, or cook a second session of %s later (e.g. on %s) for them;",
			useBy.Format(time.DateOnly), strings.Join(frozenCells, ", "), foodName(r.s, ss.FoodID), frozenCells[0])
	case !f.Freezable:
		fmt.Fprintf(&b, " %s is not freezable, so plates after %s (%s) cannot eat from it: give them another recipe, or cook a second session of it on %s;",
			foodName(r.s, ss.FoodID), useBy.Format(time.DateOnly), strings.Join(frozenCells, ", "), frozenCells[0])
	case len(lateCells) > 0:
		fmt.Fprintf(&b, " the freezer keeps %d days (until %s): plates on %s are too late, give them another recipe;",
			set.FreezerKeepDays, freezerLimit.Format(time.DateOnly), strings.Join(lateCells, ", "))
	default:
		fmt.Fprintf(&b, " each frozen plate must be covered by frozen_portions: set frozen_portions to at least %s (and portions to at least %s); or end the batch on %s and cook a second session on %s, or reduce the leftovers.",
			num(use.Frozen), num(use.Total), useBy.Format(time.DateOnly), frozenCells[0])
	}
	return strings.TrimSuffix(b.String(), ";") + "."
}

// autofix applies the unambiguous arithmetic corrections to a proposal: a
// proposed session whose plates consume more than it cooks grows its
// portions (and, for plates after the keep days, its frozen portions) to
// match, when its links are otherwise valid (after the cook cell, frozen
// ones only for a freezable recipe within the freezer's keep days) and the
// gap is plausible arithmetic: a plate eating more than maxAutofixPerEater
// per eater, or a consumption over maxAutofixGrowth times the session, is a
// model mistake (eaters used as portions) that is sent back, not hidden. It
// returns the corrected proposal and one user-facing line per change.
const (
	maxAutofixPerEater = 2.0
	maxAutofixGrowth   = 2.0
)

func autofix(in ChooseInput, prop Proposal) (Proposal, []string) {
	s := Merge(in.State, in.Target, prop)
	x := newIndex(s, in.Target)
	set := s.Settings.BatchSettings
	out := prop
	out.Sessions = slices.Clone(prop.Sessions)
	var notes []string
	for i, ps := range out.Sessions {
		si, ok := x.byRef[sessionRef{key: ps.Key}]
		f, known := x.food(ps.FoodID)
		if !ok || !known || x.s.Sessions[si].FoodID != ps.FoodID || !(ps.Portions > 0) || ps.FrozenPortions < 0 || ps.FrozenPortions > ps.Portions {
			continue
		}
		ss := s.Sessions[si]
		keep := x.keepDays(ss.FoodID)
		links := x.links[ss.ref()]
		use := plate.Consumption(ss.CookSession, links, keep)
		if !overBy(use.Total, ps.Portions) && !overBy(use.Frozen, ps.FrozenPortions) {
			continue
		}
		valid := len(links) > 0
		for _, l := range links {
			switch {
			case plate.Before(l.Cell, ss.CookCell(), s.SlotOrder), !(l.PerEater > 0), !(l.Eaters > 0), l.PerEater > maxAutofixPerEater:
				valid = false
			case plate.Classify(l.Cell, ss.CookSession, keep) == plate.KindFrozen:
				valid = valid && set.FreezingEnabled && f.Freezable && !l.Cell.Date.After(ss.CookDate.AddDate(0, 0, set.FreezerKeepDays))
			}
		}
		if !valid || use.Total > maxAutofixGrowth*ps.Portions {
			continue
		}
		portions, frozen := max(ps.Portions, round2(use.Total)), max(ps.FrozenPortions, round2(use.Frozen))
		line := fmt.Sprintf("%s: portions raised from %s to %s", foodName(s, ps.FoodID), num(ps.Portions), num(portions))
		if frozen != ps.FrozenPortions {
			line += fmt.Sprintf(", frozen portions from %s to %s", num(ps.FrozenPortions), num(frozen))
		}
		notes = append(notes, line+" to cover its plates.")
		out.Sessions[i].Portions, out.Sessions[i].FrozenPortions = portions, frozen
	}
	return out, notes
}

// capPortions is the last guard before a generated plan is saved: a component
// above MaxDishPortions is lowered to it, and the proposed session it eats
// from shrinks by what the plate no longer eats, so coverage (H3) holds. The
// checker reports the violation first and the repair loop asks the model to
// fix it; this keeps an unrepaired one out of the saved plan. One user-facing
// line per change.
func capPortions(s State, in Proposal) (Proposal, []string) {
	out := in
	out.Plates = slices.Clone(in.Plates)
	out.Sessions = slices.Clone(in.Sessions)
	var notes []string
	for i, pl := range out.Plates {
		pl.Components = slices.Clone(pl.Components)
		changed := false
		for j, c := range pl.Components {
			if c.Portions <= MaxDishPortions+epsilon {
				continue
			}
			for k, ss := range out.Sessions {
				if c.SessionKey != "" && ss.Key == c.SessionKey {
					out.Sessions[k].Portions = max(ss.Portions-pl.eaters(s)*(c.Portions-MaxDishPortions), 0)
					out.Sessions[k].FrozenPortions = min(out.Sessions[k].FrozenPortions, out.Sessions[k].Portions)
				}
			}
			notes = append(notes, fmt.Sprintf("%s: portions lowered from %s to %s on %s (at most %s portions of a dish per person).",
				foodName(s, c.FoodID), num(c.Portions), num(MaxDishPortions), weekdayDate(pl.Date), num(MaxDishPortions)))
			pl.Components[j].Portions = MaxDishPortions
			changed = true
		}
		if changed {
			out.Plates[i] = pl
		}
	}
	return out, notes
}

func round2(v float64) float64 { return math.Ceil(v*100-1e-9) / 100 }

// signature identifies a proposal by its structure, without the texts, to
// tell a repeated reply from an edited one.
func signature(p Proposal) string {
	var b strings.Builder
	for _, s := range p.Sessions {
		fmt.Fprintf(&b, "S%s|%d|%s|%d|%g|%g;", s.Key, s.FoodID, s.CookDate.Format(time.DateOnly), s.CookSlotID, s.Portions, s.FrozenPortions)
	}
	for _, pl := range p.Plates {
		fmt.Fprintf(&b, "P%d|%s|%d|%g", pl.PlateID, pl.Date.Format(time.DateOnly), pl.SlotID, pl.Eaters)
		for _, c := range pl.Components {
			fmt.Fprintf(&b, "|%d,%g,%s,%d", c.FoodID, c.Portions, c.SessionKey, c.SessionID)
		}
		b.WriteByte(';')
	}
	return b.String()
}

// overBy mirrors plate's tolerance: a exceeds b by a visible amount.
func overBy(a, b float64) bool { return a-b >= 1e-6 }
