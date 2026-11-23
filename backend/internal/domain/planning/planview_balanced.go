package planning

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
)

// The planner's evaluation shows the plan as it will be saved (E020): the
// guards and the balancing pass run on a copy first, so what the model reads
// is true and only what it alone can fix is left open (dishes, days, batches).

// maxBalancedNotes caps the lines of what the code changed in a result.
const maxBalancedNotes = 8

// finishProposal is what a generated plan goes through after the chooser, in
// one place for the service and the evaluation: the portion cap, the cooking
// limit (side recipes only), the day's
// distinct mains (a day regenerate), the meat and fish guard, the balancing pass, and the guard again for the portions the pass
// raised. It returns the finished proposal and one note per change. A side
// swap is left alone.
func finishProposal(in ChooseInput, prop Proposal) (Proposal, []string) {
	if in.Side != nil {
		return prop, nil
	}
	// The light requests the proposal states stand for the passes below, like the checker's.
	in.State.Light = MergeLight(in.State.Light, prop.Light)
	skip := idSet(in.Exclude)
	var notes []string
	prop, capNotes := capPortions(in.State, prop)
	notes = append(notes, capNotes...)
	prop, n := fixCookLimit(in, prop)
	notes = append(notes, n...)
	prop, n = fixVariety(in, prop)
	notes = append(notes, n...)
	prop, n = trimLightMeals(in, prop)
	notes = append(notes, n...)
	prop, n = fillEmptyMeals(in, prop)
	notes = append(notes, n...)
	prop, n = guardDayMains(in.State, in.Target, prop, skip)
	notes = append(notes, n...)
	prop, n = guardWeek(in.State, in.Target, prop, skip, in.Instruction)
	notes = append(notes, n...)
	prop, n = balancePlan(in, prop)
	notes = append(notes, n...)
	// Raised portions may have passed the week's meat limit again.
	prop, n = guardWeek(in.State, in.Target, prop, skip, in.Instruction)
	notes = append(notes, n...)
	// A wish's time limit outlives the swaps above (V72).
	prop, n = fixQuick(in, prop)
	notes = append(notes, n...)
	// A binding cooking-day limit leaves days light: say which and by how much.
	ev := Evaluate(Merge(in.State, in.Target, prop), in.Target)
	if limitBinds(ev, prop) {
		if note := shortfallText(ev, gapDays(ev)); note != "" {
			notes = append(notes, note)
		}
	}
	return prop, notes
}

// plannerView is the evaluate_plan result of a proposal: the evaluation of the
// finished plan, compact, without the options of the issues the balancing
// pass owns (energy, protein, add-ons), with the lines the code changed and,
// for a gap balancing could not close, the lever that would. Under a cooking-day
// limit that binds, a gap no richer main could close is accepted with a note
// (`accepted_shortfall`), not left as an open item; offered is true from the run's second evaluation on, when the richer-main lever has been given once.
func plannerView(in ChooseInput, prop Proposal, offered bool) (map[string]any, Evaluation, Proposal) {
	done, notes := finishProposal(in, prop)
	merged := Merge(in.State, in.Target, done)
	ev := EvaluateWeek(merged, in.Target)
	for i := range ev.Issues {
		switch ev.Issues[i].Rule {
		case "S2", "S3", "S4", "S5", "S9":
			ev.Issues[i].Options = nil
		}
	}
	if noFish.MatchString(in.Instruction) {
		// The instruction rules fish out and the guard places none: nothing to act on.
		ev.Issues = slices.DeleteFunc(ev.Issues, func(is Issue) bool { return is.Code == CodeWeekFishBelowDGE })
	}
	// A day a few kcal under its band is rounding, not a gap: not shown as one.
	near := map[string]bool{}
	for _, d := range ev.Days {
		if d.Complete && d.Kcal < d.BandLow-0.5 && d.Kcal >= d.BandLow-gapTolerance(d.BandLow) {
			near[d.Date] = true
		}
	}
	ev.Issues = slices.DeleteFunc(ev.Issues, func(is Issue) bool { return is.Code == CodeKcalBelowBand && near[is.Date] })
	levers, accepted, acceptedDates := openLevers(in, ev, merged, done, offered)
	if accepted != "" {
		ev.Issues = slices.DeleteFunc(ev.Issues, func(is Issue) bool {
			return (is.Code == CodeKcalBelowBand && slices.Contains(acceptedDates, is.Date)) || (is.Code == CodeWeekKcalBelowBand && len(levers) == 0)
		})
	}
	view := ev.Compact(ev.ProposedView())
	if len(notes) > 0 {
		if len(notes) > maxBalancedNotes {
			notes = append(notes[:maxBalancedNotes:maxBalancedNotes], fmt.Sprintf("and %d more changes of the same kind", len(notes)-maxBalancedNotes))
		}
		view["code_changes"] = notes
	}
	if len(levers) > 0 {
		view["open_levers"] = levers
	}
	if accepted != "" {
		view["accepted_shortfall"] = accepted
	}
	if hints := varietyHints(in, merged, ev); len(hints) > 0 {
		view["variety_hints"] = hints
	}
	if hints := cookingHints(in, merged); len(hints) > 0 {
		view["cooking_hints"] = hints
	}
	if distinctDays(in.Target) < minDGEDays {
		// A day or plate regenerate sits in a week: name the repeats it holds
		// (the same dish at lunch and dinner, a dish family on adjacent days).
		if repeats := newIndex(merged, in.Target).familyRepeats(); len(repeats) > 0 {
			view["repeats"] = repeats
		}
	}
	// What the code wrote for the model reads in the same kitchen words as the chat.
	for _, k := range []string{"code_changes", "open_levers", "cooking_hints", "variety_hints"} {
		if lines, ok := view[k].([]string); ok {
			out := make([]string, len(lines))
			for i, l := range lines {
				out[i] = kitchenProse(l)
			}
			view[k] = out
		}
	}
	if a, ok := view["accepted_shortfall"].(string); ok {
		view["accepted_shortfall"] = kitchenProse(a)
	}
	return view, ev, done
}

// gapDay is a day under its band after balancing.
type gapDay struct {
	date       string
	kcal, band float64 // under its target, under its band
}

// gapTolerance is the kcal a day may sit under its band before the evaluation
// calls it a gap: rounding of the sides (10 g steps), not a dish to change.
func gapTolerance(bandLow float64) float64 { return math.Max(25, 0.015*bandLow) }

// gapDays lists the days under their band, the furthest under target first.
func gapDays(ev Evaluation) []gapDay {
	var out []gapDay
	for _, d := range ev.Days {
		if !d.Complete || d.TargetKcal == nil || d.Kcal >= d.BandLow-gapTolerance(d.BandLow) {
			continue
		}
		out = append(out, gapDay{d.Date, *d.TargetKcal - d.Kcal, d.BandLow - d.Kcal})
	}
	slices.SortStableFunc(out, func(a, b gapDay) int { return cmp.Compare(b.kcal, a.kcal) })
	return out
}

// limitBinds reports whether the stated cooking-day limit is used up.
func limitBinds(ev Evaluation, prop Proposal) bool {
	return prop.MaxCookDays > 0 && cookDays(ev) >= prop.MaxCookDays
}

var countWords = []string{"zero", "one", "two", "three", "four", "five", "six", "seven"}

// shortfallText says in plain words which days are light and by how much.
func shortfallText(ev Evaluation, days []gapDay) string {
	if len(days) == 0 {
		return ""
	}
	slices.SortStableFunc(days, func(a, b gapDay) int { return cmp.Compare(a.date, b.date) })
	var parts []string
	for _, d := range days {
		parts = append(parts, fmt.Sprintf("%s about %.0f kcal under", dateOf(d.date).Weekday(), math.Round(d.kcal/10)*10))
	}
	n := cookDays(ev)
	cook := fmt.Sprint(n)
	if n < len(countWords) {
		cook = countWords[n]
	}
	out := strings.Join(parts, ", ") + ": " + cook + " cooking days cannot carry more"
	if w := ev.Week; w != nil && w.Days >= MinWeekDays {
		out += fmt.Sprintf(" (the week is at %.0f %% of its energy target)", w.Pct)
	}
	return out + "."
}

// richerMainExists reports whether the library holds a main that could lift a
// day by gap kcal: a diet-fitting main whose portion holds more energy than the
// day's mains by the gap shared over its main plates at the 1.5 portion cap.
func richerMainExists(in ChooseInput, merged State, date string, gap float64) bool {
	x := newIndex(merged, in.Target)
	best, plates := 0.0, 0
	held := map[int64]bool{}
	for _, p := range merged.Plates {
		if p.Date.Format(time.DateOnly) != date || x.mealOf(p.SlotID) != mealFull {
			continue
		}
		if id := merged.mainOf(p); id != 0 {
			plates++
			held[id] = true
			best = max(best, merged.Foods[id].Macros.Kcal)
		}
	}
	if plates == 0 {
		return false
	}
	var household diet.Class
	if DietRestricts(merged.Profile.DietType) {
		household = diet.Class(merged.Profile.DietType)
	}
	need := gap / (float64(plates) * maxTopUp)
	skip := idSet(in.Exclude)
	// The week's meat limit and the variety rule bind a swap too: a main they rule out is no lever.
	noMeat := false
	if w := EvaluateWeek(merged, in.Target).Week; w != nil && w.Days >= minDGEDays {
		noMeat = w.MeatG >= DGEMeatMaxG*float64(w.Days)/weekDays-1e-9
	}
	d, _ := time.Parse(time.DateOnly, date)
	span := time.Duration(7*merged.Settings.VarietyWeeks) * 24 * time.Hour
	recent := func(f FoodFacts) bool {
		return f.LastEaten != nil && d.Sub(*f.LastEaten) < span && d.After(*f.LastEaten)
	}
	for id, f := range merged.Foods {
		if f.Composed() && isMain(f.Role, mealFull) && !held[id] && !skip[id] && !recent(f) && !(noMeat && f.Groups.MeatG > 0) && fitsDiet(f.DietClass, household) && f.Macros.Kcal >= best+need {
			return true
		}
	}
	return false
}

// openLevers says, from the evaluation after balancing, how a gap that the
// balancing could not close can be closed by the model: a richer main on the
// day (and, without a binding cooking-day limit, a further cooking day). Under
// a binding limit it never proposes a cooking day; a gap no richer main can
// close is returned as accepted (with its text and dates) instead.
func openLevers(in ChooseInput, ev Evaluation, merged State, prop Proposal, offered bool) (levers []string, accepted string, acceptedDates []string) {
	gaps := gapDays(ev)
	day := func(g gapDay) string { return dateOf(g.date).Weekday().String() }
	binds := limitBinds(ev, prop)
	cooking := ", or add a cooking day"
	if binds {
		cooking = fmt.Sprintf(" (your limit of %d cooking days stays)", prop.MaxCookDays)
	}
	// A richer main is offered once (the first evaluation of the run); after that the gap is accepted.
	feasible := func(g gapDay) bool { return !binds || (!offered && richerMainExists(in, merged, g.date, g.band)) }
	if binds {
		var left, acc []gapDay
		for _, g := range gaps {
			if feasible(g) {
				left = append(left, g)
			} else {
				acc = append(acc, g)
				acceptedDates = append(acceptedDates, g.date)
			}
		}
		accepted = shortfallText(ev, acc)
		gaps = left
	}
	if w := ev.Week; w != nil && w.Days >= MinWeekDays && len(gaps) > 0 && w.Pct < 95-1e-9 {
		levers = append(levers, fmt.Sprintf("After balancing the week is at %.0f %% of its energy target (%.0f kcal per person per day short): the dishes cannot carry more with portions at 1.5 and the add-on and side caps. Plan a richer main on %s (about %.0f kcal more per person there)%s.",
			w.Pct, w.TargetKcal-w.AvgKcal, day(gaps[0]), gaps[0].kcal, cooking))
		gaps = gaps[1:]
	}
	for _, g := range gaps[:min(len(gaps), 3)] {
		levers = append(levers, fmt.Sprintf("%s stays %.0f kcal under its band after balancing: plan a richer main there%s.", day(g), g.band, strings.TrimPrefix(cooking, ", or add a cooking day")))
	}
	return levers, accepted, acceptedDates
}
