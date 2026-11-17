package planning

import (
	"fmt"
	"math"

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
}

// PolicyOf derives the policy of p; an empty goal is maintain.
func PolicyOf(p Profile) Policy {
	pol := Policy{
		Goal: profile.GoalMaintain, DayLow: 0.9, DayHigh: 1.1, WeekLow: 0.95, WeekHigh: 1.05,
		FloorShare: 0.75, FloorKcal: profile.MinDailyKcal(p.Sex),
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

// Floor is the lowest kcal per person a day may hold when a share of its
// meals (planned / all, 1 for a full day) is planned. The absolute floor
// scales with the share exactly like the target does, otherwise a day with a
// No-plate meal could never reach it.
func (p Policy) Floor(target, share float64) float64 {
	return max(p.FloorShare*target*share, p.FloorKcal*share)
}

// Describe is the policy as the prompts tell it to the model: the target is
// a target, the week matters most, the floors are hard.
func (p Policy) Describe() string {
	pct := func(v float64) int { return int(math.Round(v * 100)) }
	floor := fmt.Sprintf("%d%% of the day's target", pct(p.FloorShare))
	if p.FloorKcal > 0 {
		floor += fmt.Sprintf(" and never below %.0f kcal per person for a full day (scaled to the planned meals on a short day)", p.FloorKcal)
	}
	return fmt.Sprintf("Energy policy (goal: %s). The kcal target is a target, not a limit. The week matters most: the average of the complete days should land at %d-%d%% of the target (soft issue S4); a single day may deviate within %d-%d%% of its target (soft issue S2). The floor is hard: never plan a day below %s (H9).",
		p.Goal, pct(p.WeekLow), pct(p.WeekHigh), pct(p.DayLow), pct(p.DayHigh), floor)
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
