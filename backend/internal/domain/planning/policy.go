package planning

import (
	"fmt"
	"math"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/profile"
)

// Policy is the nutrition policy derived from the profile: which deviations
// from the user's kcal target are tolerated and where the safety floor is.
// It is the one place for these numbers; Evaluate, the checker, the prompts
// and the heuristic read it.
//
// The kcal target stays the user's own number; the goal decides the bands
// around it. Day bands are looser than week bands on purpose: day-to-day
// intake varies by 20-30 % without an effect on weight, and energy balance
// is a weekly matter, so the weekly average is the real goal.
//
// Sources (figures marked * came from secondary sources and were not checked
// against the primary pages):
//   - Energy need = resting energy expenditure x PAL, DGE reference values
//     (dge.de/wissenschaft/referenzwerte/energie/); body weight is the
//     control parameter.
//   - Predictive equations (Mifflin-St Jeor) and TDEE estimates are good to
//     about +-10-20 %*, so a day band tighter than +-10 % would be false
//     precision. The band widths are professional-practice inference; no
//     source states daily tolerances.
//   - Floors: 1200 kcal women / 1500 kcal men per day without medical
//     supervision* (Academy of Nutrition and Dietetics, NICE CG189
//     summaries); below that only under supervision. 75 % of the target is
//     the planner's own limit for a deliberately light day.
//   - Protein in g per kg body weight: DGE reference value 0.8 g/kg for
//     adults and 1.0 g/kg from 65 (dge.de/wissenschaft/referenzwerte/protein/),
//     EFSA PRI 0.83 g/kg; ISSN position stand 1.4-2.0 g/kg for exercising
//     people, and ~1.6 g/kg is the plateau of the benefit for muscle gain
//     (Morton 2018*), which is why lose and gain default to 1.6. The user's
//     own factor is accepted from 0.8 to 2.2 g/kg; 2.2 is an upper bound
//     that was not verified against a primary source. A day under 90 % of
//     its target and a weekly average under 100 % are soft issues: day to
//     day intake varies, the average is what counts.
//   - Carbs and fat as ranges of energy share, judged on the weekly average
//     only: AMDR carbs 45-65 %, fat 20-35 % (protein 10-35 %). A share the
//     user set themselves is the centre of a +-5 point range. Fibre: DGE
//     asks at least 30 g per day; it is displayed, never an issue.
type Policy struct {
	Goal profile.Goal
	// DayLow and DayHigh bound a complete day's kcal as a share of its
	// scaled target (soft issue S2). WeekLow and WeekHigh bound the average
	// of the complete days (soft issue S4).
	DayLow, DayHigh   float64
	WeekLow, WeekHigh float64
	// FloorShare is the hard floor as a share of the scaled target;
	// FloorKcal the absolute floor per person for a full day (0: sex
	// unknown).
	FloorShare float64
	FloorKcal  float64
	// ProteinGPerKg is the protein target per kg body weight, 0 when the
	// profile has no weight (the profile's ProteinPct share applies then).
	ProteinGPerKg float64
	WeightKg      float64
	// CarbsLow..FatHigh are the weekly ranges of the energy shares, in %.
	CarbsLow, CarbsHigh float64
	FatLow, FatHigh     float64
}

// Protein and macro figures (see Policy).
const (
	// ProteinDayMin and ProteinWeekMin are the shares of the protein target a
	// day and the weekly average must reach.
	ProteinDayMin  = 0.9
	ProteinWeekMin = 1.0
	// MacroRangePts is the half width of a range around a share the user set.
	MacroRangePts = 5.0
	// FiberTargetG is the DGE fibre target per day.
	FiberTargetG = 30.0
	seniorAge    = 65
	// seniorProteinPerKg is the protein floor in g per kg from seniorAge.
	seniorProteinPerKg = 1.0
)

// PolicyOf derives the policy of p; an empty goal is maintain.
func PolicyOf(p Profile) Policy {
	pol := Policy{
		Goal: profile.GoalMaintain, DayLow: 0.9, DayHigh: 1.1, WeekLow: 0.95, WeekHigh: 1.05,
		FloorShare: 0.75, FloorKcal: profile.MinDailyKcal(p.Sex),
		CarbsLow: 45, CarbsHigh: 65, FatLow: 20, FatHigh: 35,
	}
	if p.CarbsPct != nil {
		pol.CarbsLow, pol.CarbsHigh = max(*p.CarbsPct-MacroRangePts, 0), *p.CarbsPct+MacroRangePts
	}
	if p.FatPct != nil {
		pol.FatLow, pol.FatHigh = max(*p.FatPct-MacroRangePts, 0), *p.FatPct+MacroRangePts
	}
	if p.WeightKg != nil && *p.WeightKg > 0 {
		pol.WeightKg, pol.ProteinGPerKg = *p.WeightKg, proteinPerKg(p)
	}
	switch p.Goal {
	case profile.GoalLose:
		// The target is the deficit target: overshooting costs the deficit.
		pol.Goal, pol.DayHigh, pol.WeekHigh = profile.GoalLose, 1.05, 1.03
	case profile.GoalGain:
		pol.Goal, pol.DayLow, pol.WeekLow = profile.GoalGain, 0.95, 0.97
	}
	return pol
}

// proteinPerKg is the profile's protein factor: its own override, else the
// goal's default (maintain 1.0, lose and gain 1.6); from age 65 at least 1.0
// (the DGE advice), also over a lower override; never below the DGE floor.
func proteinPerKg(p Profile) float64 {
	f := 1.0
	if p.ProteinGPerKg != nil {
		f = *p.ProteinGPerKg
	} else if p.Goal == profile.GoalLose || p.Goal == profile.GoalGain {
		f = 1.6
	}
	if p.BirthYear != nil && time.Now().Year()-*p.BirthYear >= seniorAge {
		f = max(f, seniorProteinPerKg)
	}
	return max(f, profile.MinProteinGPerKg)
}

// ProteinTargetG is the protein target in g per person for a day whose
// planned meals are share of all meals (1 for a full day); 0 without a
// weight.
func (p Policy) ProteinTargetG(share float64) float64 {
	return p.WeightKg * p.ProteinGPerKg * share
}

// Floor is the lowest kcal per person a day may hold when a share of its
// meals (planned / all, 1 for a full day) is planned. The absolute floor
// scales with the share exactly like the target does, otherwise a day with a
// No-plate meal could never reach it.
func (p Policy) Floor(target, share float64) float64 {
	return max(p.FloorShare*target*share, p.FloorKcal*share)
}

// shiftedTarget is a day's scaled target moved by its light and hearty
// meals: a light meal by -(1-LightMealShare) and a hearty one by
// +(HeartyMealShare-1) of its normal share (scaled / active meals). The sum
// stays inside the day band, so a meal intent never takes a day further than
// a normal day may deviate; only the day scope goes beyond.
func (p Policy) shiftedTarget(scaled float64, active, light, hearty int) float64 {
	share := scaled / float64(active)
	shift := float64(hearty)*(HeartyMealShare-1)*share - float64(light)*(1-LightMealShare)*share
	return min(max(scaled+shift, p.DayLow*scaled), p.DayHigh*scaled)
}

// Describe is the policy as the prompts tell it to the model: the target is
// a target, the week matters most, the floors are hard.
func (p Policy) Describe() string {
	pct := func(v float64) int { return int(math.Round(v * 100)) }
	floor := fmt.Sprintf("%d%% of the day's target", pct(p.FloorShare))
	if p.FloorKcal > 0 {
		floor += fmt.Sprintf(" and never below %.0f kcal per person for a full day (scaled to the planned meals on a short day)", p.FloorKcal)
	}
	out := fmt.Sprintf("Energy policy (goal: %s). The kcal target is a target, not a limit. The week matters most: the average of the complete days should land at %d-%d%% of the target (soft issue S4); a single day may deviate within %d-%d%% of its target (soft issue S2). The floor is hard: never plan a day below %s (H9).",
		p.Goal, pct(p.WeekLow), pct(p.WeekHigh), pct(p.DayLow), pct(p.DayHigh), floor)
	out += fmt.Sprintf(" Priority when rules conflict: 1 hard rules, 2 weekly kcal, 3 day kcal band, 4 protein, 5 food-group rules (the DGE weekly food rules below, soft issues S7), 6 preferences and effort, 7 carb and fat ranges (soft issues S5: last, never at the cost of kcal or protein). Carbs and fat are judged on the weekly average only: carbs %.0f-%.0f%% and fat %.0f-%.0f%% of energy.",
		p.CarbsLow, p.CarbsHigh, p.FatLow, p.FatHigh)
	out += fmt.Sprintf(" Light requests are structured, never prose: a list of {date, scope meal|day, slot lunch|dinner for a meal}. \"Something light\" means scope meal: that lunch or dinner holds at most %d%% of its normal share of the day's target (soft issue S6 when it is not lighter). A light or hearty meal moves the day's target by that meal's share, within the normal day band; the safety floor stays on the normal target and so does protein. Only an explicit request to eat less for a day is scope day: that day's target becomes %d%% and its band is centred there. Never go below the floors (H9); when a floor stopped a lighter day, say so in the reply.",
		pct(LightMealShare), pct(LightDayShare))
	out += fmt.Sprintf(" The opposite is level hearty on the same entry: a hearty meal holds at least %d%% of its normal share (soft issue S6b when it does not) and raises the day's target the same way, a hearty day's target becomes %d%%.",
		pct(HeartyMealShare), pct(HeartyDayShare))
	out += " An explicit light or hearty meal request ranks above the day and week kcal bands (S2, S4), below the hard rules: top-ups never go on a light meal."
	out += " Add-ons (S9, ranked with preferences): within 2 servings, 200 g and 20% of the day's target, at most 3 a day, one food on at most 3 plates, none on a light meal or duplicating the main."
	out += " Dish repeats (soft issue S8 dish_repeats_exceeded, ranked with preferences): one dish should be the main of at most the user's limit of lunches and dinners a week."
	out += " DGE weekly food rules (soft issues week_meat_above_dge, week_fish_below_dge, week_legumes_below_dge; rule S7, priority 5: prefer them, never at the cost of kcal, protein or the user's wishes; they are judged only on 5 or more complete days): meat and sausage at most 300 g per person per week (prorated to the days), fish once a week or more, legumes (beans, lentils, chickpeas, peas, tofu) at least once a week (at least 50 g per person per meal), plenty of fruit and vegetables every day."
	if p.ProteinGPerKg > 0 {
		out += fmt.Sprintf(" Protein target: %.1f g per kg x %.0f kg = %.0f g per person per full day (scaled to the planned meals on a short day). A day under %d%% of it is soft issue S3, a weekly average under %d%% too: close protein gaps actively (find_foods with min_protein, or take an issue's options: skyr/quark, yoghurt, tofu, eggs, cheese, legumes, honouring the diet type and the kcal band) and write a note only when no option exists.",
			p.ProteinGPerKg, p.WeightKg, p.ProteinTargetG(1), pct(ProteinDayMin), pct(ProteinWeekMin))
	}
	return out
}

// WeekCode is the soft issue code of a weekly average (a share of the
// average scaled target), "" when it is inside the band.
func (p Policy) WeekCode(ratio float64) string {
	const eps = 1e-9
	switch {
	case ratio < p.WeekLow-eps:
		return CodeWeekKcalBelowBand
	case ratio > p.WeekHigh+eps:
		return CodeWeekKcalAboveBand
	}
	return ""
}
