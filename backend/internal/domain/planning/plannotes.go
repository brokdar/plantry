package planning

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// The note a saved plan carries (E028) is the chooser's own words plus lines
// computed from the plan as it was saved, in kitchen words: dishes the code
// changed, sides it added (one line), days that are light or heavy by how
// much (only outside their range), cold plates, a dish that had to stay twice
// in a day. The balancing and guard log lines stay in the run log
// (FinishStep), where they are exact but read like a log.

// FinishStep is the run-log step of the lines the code wrote after the
// chooser: portion cap, guards, balancing, and the chooser's own appended notes.
const FinishStep = "plan.finish"

// maxSideGroups bounds the plates the sides line names.
const maxSideGroups = 3

// savedPlanNotes are the code-written note lines of the plan final, which the
// chooser proposed as orig. Only lines that are true of final.
func savedPlanNotes(in ChooseInput, orig, final Proposal) []string {
	merged := Merge(in.State, in.Target, final)
	ev := Evaluate(merged, in.Target)
	x := newIndex(merged, in.Target)
	var out []string
	add := func(lines ...string) {
		for _, l := range lines {
			if l != "" {
				out = append(out, l)
			}
		}
	}
	add(lightNote(merged.Light, in.Locale))
	add(dishChangeLines(merged, orig, final)...)
	add(sidesLine(merged, orig, final))
	add(dayGapLines(in, ev, final)...)
	add(sameDishLines(x, merged)...)
	add(newDishLines(in, merged)...)
	add(floorNote(merged, in.Target), mealNote(merged, in.Target), proteinNote(merged, in.Target))
	return out
}

// newDishWhy says in kitchen words why a recipe the chooser never saw is out
// (excludedFood reasons).
var newDishWhy = map[string]string{
	excludeNever:    "it holds something you never eat",
	excludeDiet:     "it does not suit your diet",
	excludeReplaced: "you asked for something else in its place",
	excludeSlotTime: "it takes longer than any meal this week has time for",
	excludeTrial:    "you already cook it on trial",
}

// newDishLines name the week's new dishes (untried explore recipes, placed
// ready finds among them) the plan merged leaves out while it holds fewer
// than New dishes (FIND-3), newest first, each with why it did not fit.
func newDishLines(in ChooseInput, merged State) []string {
	if in.ExploreQuota <= 0 {
		return nil
	}
	onPlan := map[int64]bool{}
	for _, p := range merged.Plates {
		if merged.Window.Contains(p.Date) {
			for _, c := range p.Components {
				onPlan[c.FoodID] = true
			}
		}
	}
	placed := 0
	var left []FoodFacts
	for _, f := range in.State.Foods {
		switch {
		case !f.Composed() || f.Archived || !f.Explore():
		case onPlan[f.ID]:
			placed++
		default:
			left = append(left, f)
		}
	}
	if placed >= in.ExploreQuota || len(left) == 0 {
		return nil
	}
	slices.SortFunc(left, func(a, b FoodFacts) int { return cmp.Compare(b.ID, a.ID) })
	_, excluded := candidateSplit(in)
	var out []string
	for _, f := range left[:min(len(left), in.ExploreQuota-placed)] {
		why := "no meal this week had room for it"
		if i := slices.IndexFunc(excluded, func(e excludedFood) bool { return e.ID == f.ID }); i >= 0 && newDishWhy[excluded[i].Reason] != "" {
			why = newDishWhy[excluded[i].Reason]
		}
		out = append(out, fmt.Sprintf("The new dish %s did not fit this week: %s.", f.Name, why))
	}
	return out
}

// cellShort names a cell the way a cook says it ("Thu dinner").
func cellShort(s State, d time.Time, slot int64) string {
	return d.Weekday().String()[:3] + " " + slotName(s, slot)
}

// mainID is the id of the composed main a proposed plate holds (0: none).
func mainID(s State, p ProposedPlate) int64 {
	for _, c := range p.Components {
		if s.countsAsMain(c.FoodID) {
			return c.FoodID
		}
	}
	return 0
}

// dishChangeLines say which dishes the code swapped in place of the chooser's
// ("Instead of Beef ragù: Egg fried rice (Sat dinner)") and which meals it
// made a cold plate.
func dishChangeLines(s State, orig, final Proposal) []string {
	type change struct{ from, to int64 }
	cells := map[change][]string{}
	var order []change
	var cold []string
	coldLine := func(fp ProposedPlate) string {
		var parts []string
		kcal := 0.0
		for _, c := range fp.Components {
			f := s.Foods[c.FoodID]
			parts = append(parts, fmt.Sprintf("%s %s g", strings.ToLower(f.Name), num(c.Grams)))
			kcal += f.Macros.Kcal * c.Grams / 100
		}
		return fmt.Sprintf("%s is a cold plate, nothing to cook: %s (about %.0f kcal).", cellShort(s, fp.Date, fp.SlotID), strings.Join(parts, ", "), math.Round(kcal/10)*10)
	}
	for i, fp := range final.Plates {
		if fp.PlateID != 0 {
			continue
		}
		if i >= len(orig.Plates) {
			// A meal the chooser left empty that a fallback filled with a cold plate (KR7).
			if fp.Reason == reasonColdFill {
				cold = append(cold, coldLine(fp))
			}
			continue
		}
		if !orig.Plates[i].Date.Equal(fp.Date) || orig.Plates[i].SlotID != fp.SlotID {
			continue
		}
		from, to := mainID(s, orig.Plates[i]), mainID(s, fp)
		if from == to || from == 0 {
			continue
		}
		if to == 0 {
			cold = append(cold, coldLine(fp))
			continue
		}
		k := change{from, to}
		if _, ok := cells[k]; !ok {
			order = append(order, k)
		}
		cells[k] = append(cells[k], cellShort(s, fp.Date, fp.SlotID))
	}
	var out []string
	var parts []string
	for _, k := range order {
		parts = append(parts, fmt.Sprintf("instead of %s: %s (%s)", plainName(s, k.from), plainName(s, k.to), strings.Join(cells[k], ", ")))
	}
	if len(parts) > 0 {
		out = append(out, "Dishes changed: "+strings.Join(parts, "; ")+".")
	}
	return append(out, cold...)
}

// sidesLine is one line of what was added to plates as sides, by plate.
func sidesLine(s State, orig, final Proposal) string {
	var groups []string
	for i, fp := range final.Plates {
		if i >= len(orig.Plates) || fp.PlateID != 0 || !orig.Plates[i].Date.Equal(fp.Date) || orig.Plates[i].SlotID != fp.SlotID || mainID(s, fp) == 0 {
			continue
		}
		var names []string
		for _, c := range fp.Components {
			if c.FoodID == mainID(s, fp) || slices.ContainsFunc(orig.Plates[i].Components, func(o ProposedComponent) bool { return o.FoodID == c.FoodID }) {
				continue
			}
			names = append(names, strings.ToLower(plainName(s, c.FoodID)))
		}
		if len(names) > 0 {
			groups = append(groups, cellShort(s, fp.Date, fp.SlotID)+" "+strings.Join(names, ", "))
		}
	}
	if len(groups) == 0 {
		return ""
	}
	more := ""
	if len(groups) > maxSideGroups {
		more = fmt.Sprintf("; and %d more meals", len(groups)-maxSideGroups)
		groups = groups[:maxSideGroups]
	}
	return "Sides added: " + strings.Join(groups, "; ") + more + "."
}

// dayGapLines name the days of the run that are light or heavy, and by how
// much (about, to ten kcal), when they sit outside their range.
func dayGapLines(in ChooseInput, ev Evaluation, final Proposal) []string {
	inTarget := map[string]bool{}
	for _, c := range in.Target.Cells {
		inTarget[c.Date.Format(time.DateOnly)] = true
	}
	var light, heavy []string
	for _, d := range ev.Days {
		if !inTarget[d.Date] || !d.Complete || d.TargetKcal == nil || *d.TargetKcal <= 0 {
			continue
		}
		part := func() string {
			return fmt.Sprintf("%s about %.0f kcal", dateOf(d.Date).Weekday(), math.Round(math.Abs(d.Kcal-*d.TargetKcal)/10)*10)
		}
		switch {
		case d.Kcal < d.BandLow-0.5:
			light = append(light, part())
		case d.Kcal > d.BandHigh+0.5:
			heavy = append(heavy, part())
		}
	}
	var out []string
	if len(light) > 0 {
		line := "Light days (less food than planned for): " + strings.Join(light, ", ") + "."
		if limitBinds(ev, final) {
			n := cookDays(ev)
			word := fmt.Sprint(n)
			if n < len(countWords) {
				word = countWords[n]
			}
			line += " " + strings.ToUpper(word[:1]) + word[1:] + " cooking days cannot carry more."
		}
		out = append(out, line)
	}
	if len(heavy) > 0 {
		out = append(out, "Heavy days (more food than planned for): "+strings.Join(heavy, ", ")+".")
	}
	return out
}

// sameDishLines name the days of a short run whose lunch and dinner still hold
// the same dish, with the reason in kitchen words.
func sameDishLines(x *index, merged State) []string {
	if distinctDays(x.t) >= minDGEDays {
		return nil
	}
	var out []string
	for _, d := range x.days {
		var cells []Plate
		for _, p := range merged.Plates {
			if dayOf(p.Date) == dayOf(d) && x.mealOf(p.SlotID) == mealFull && !x.routine(p.Cell()) && x.mainIndex(p) >= 0 {
				cells = append(cells, p)
			}
		}
		slices.SortStableFunc(cells, func(a, b Plate) int { return cmp.Compare(merged.SlotOrder[a.SlotID], merged.SlotOrder[b.SlotID]) })
		for i, a := range cells {
			for _, b := range cells[i+1:] {
				ma, mb := a.Components[x.mainIndex(a)].FoodID, b.Components[x.mainIndex(b)].FoodID
				if ma == mb {
					out = append(out, fmt.Sprintf("%s: %s and %s are both %s. No other dish fits: %s.", d.Weekday(), slotName(merged, a.SlotID), slotName(merged, b.SlotID), plainName(merged, ma), x.kitchenSameDishReason()))
				}
			}
		}
	}
	return out
}

// kitchenSameDishReason says in kitchen words why a day keeps one dish twice.
func (x *index) kitchenSameDishReason() string {
	why := []string{"every batch of the week is full", "no main is a no-cook dish"}
	if n := x.s.Settings.MaxCookOccasionsPerDay; n > 0 {
		why = append(why, fmt.Sprintf("another dish would mean cooking more than %s a day", map[bool]string{true: "once", false: fmt.Sprintf("%d times", n)}[n == 1]))
	}
	if _, ok := x.s.composeColdPlate(nil, 1, 0); !ok {
		why = append(why, "there are no suitable no-cook foods for a cold plate")
	}
	return strings.Join(why, ", ")
}

// plainName is a food's name for the user: as stored, not marked as data.
func plainName(s State, id int64) string {
	if f, ok := s.Foods[id]; ok {
		return f.Name
	}
	return fmt.Sprintf("food %d", id)
}
