package planning

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// The planner's evaluation shows the plan as it will be saved (E020): the
// guards and the balancing pass run on a copy first, so what the model reads
// is true and only what it alone can fix is left open (dishes, days, batches).

// maxBalancedNotes caps the lines of what the code changed in a result.
const maxBalancedNotes = 8

// finishProposal is what a generated plan goes through after the chooser, in
// one place for the service and the evaluation: the portion cap, the day's
// distinct mains (a day regenerate), the meat and fish guard, the balancing pass, and the guard again for the portions the pass
// raised. It returns the finished proposal and one note per change. A side
// swap is left alone.
func finishProposal(in ChooseInput, prop Proposal) (Proposal, []string) {
	if in.Side != nil {
		return prop, nil
	}
	skip := idSet(in.Exclude)
	var notes []string
	prop, capNotes := capPortions(in.State, prop)
	notes = append(notes, capNotes...)
	prop, n := guardDayMains(in.State, in.Target, prop, skip)
	notes = append(notes, n...)
	prop, n = guardWeek(in.State, in.Target, prop, skip, in.Instruction)
	notes = append(notes, n...)
	prop, n = balancePlan(in, prop)
	notes = append(notes, n...)
	// Raised portions may have passed the week's meat limit again.
	prop, n = guardWeek(in.State, in.Target, prop, skip, in.Instruction)
	return prop, append(notes, n...)
}

// plannerView is the evaluate_plan result of a proposal: the evaluation of the
// finished plan, compact, without the options of the issues the balancing
// pass owns (energy, protein, add-ons), with the lines the code changed and,
// for a gap balancing could not close, the lever that would.
func plannerView(in ChooseInput, prop Proposal) (map[string]any, Evaluation, Proposal) {
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
	view := ev.Compact(ev.ProposedView())
	if len(notes) > 0 {
		if len(notes) > maxBalancedNotes {
			notes = append(notes[:maxBalancedNotes:maxBalancedNotes], fmt.Sprintf("and %d more changes of the same kind", len(notes)-maxBalancedNotes))
		}
		view["code_changes"] = notes
	}
	if levers := openLevers(ev, done); len(levers) > 0 {
		view["open_levers"] = levers
	}
	if distinctDays(in.Target) < minDGEDays {
		// A day or plate regenerate sits in a week: name the repeats it holds
		// (the same dish at lunch and dinner, a dish family on adjacent days).
		if repeats := newIndex(merged, in.Target).familyRepeats(); len(repeats) > 0 {
			view["repeats"] = repeats
		}
	}
	return view, ev, done
}

// openLevers says, from the evaluation after balancing, how a gap that the
// balancing could not close can be closed by the model: the days under their
// band and the week under its band, with the energy each needs.
func openLevers(ev Evaluation, prop Proposal) []string {
	type gap struct {
		date       string
		kcal, band float64 // under its target, under its band
	}
	var gaps []gap
	for _, d := range ev.Days {
		if d.Away || !d.Complete || d.TargetKcal == nil || d.Kcal >= d.BandLow-0.5 {
			continue
		}
		gaps = append(gaps, gap{d.Date, *d.TargetKcal - d.Kcal, d.BandLow - d.Kcal})
	}
	slices.SortStableFunc(gaps, func(a, b gap) int { return cmp.Compare(b.kcal, a.kcal) })
	day := func(g gap) string { return dateOf(g.date).Weekday().String() }
	cooking := ""
	if prop.MaxCookDays == 0 || cookDays(ev) < prop.MaxCookDays {
		cooking = ", or add a cooking day"
	} else {
		cooking = fmt.Sprintf(" (your limit of %d cooking days leaves no further cooking day)", prop.MaxCookDays)
	}
	var out []string
	if w := ev.Week; w != nil && w.Days >= MinWeekDays && len(gaps) > 0 && w.Pct < 95-1e-9 {
		out = append(out, fmt.Sprintf("After balancing the week is at %.0f %% of its energy target (%.0f kcal per person per day short): the dishes cannot carry more with portions at 1.5, the add-on and side caps and no new cooking days. Plan a higher-energy main on %s (about %.0f kcal more per person there)%s.",
			w.Pct, w.TargetKcal-w.AvgKcal, day(gaps[0]), gaps[0].kcal, cooking))
		gaps = gaps[1:]
	}
	for _, g := range gaps[:min(len(gaps), 3)] {
		out = append(out, fmt.Sprintf("%s stays %.0f kcal under its band after balancing: plan a higher-energy main there%s.", day(g), g.band, strings.TrimPrefix(cooking, ", or add a cooking day")))
	}
	return out
}
