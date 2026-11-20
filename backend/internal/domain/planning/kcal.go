package planning

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// The S2 band (a complete day's kcal outside the policy's day band of its
// scaled target is a soft issue, Evaluate) and the floors come from the
// nutrition policy (policy.go).
const (
	// ProteinMarginPts is how far below the profile's protein share a day
	// may fall before S3 reports it.
	ProteinMarginPts = 5.0
)

// DayKcal is one day's account per person: the planned macros, the day's
// target scaled to the meals that are planned (a day with a No-plate meal
// asks less) and the gap. Target, Scaled and Gap are zero without a kcal
// target.
type DayKcal struct {
	Date     time.Time
	Kcal     float64
	ProteinG float64
	FatG     float64
	CarbsG   float64
	FiberG   float64
	// FiberMissing: a food on the day lacks fibre data, so FiberG understates.
	FiberMissing bool
	Target       float64
	Scaled       float64
	// Adjusted is Scaled, or LightDayShare of it on a light day, or Scaled
	// shifted by the day's light and hearty meals (see shiftedTarget): the
	// target the day band is centred on. The floor stays on Scaled.
	Adjusted float64
	Gap      float64 // Adjusted - Kcal; positive: under target
	Planned  int
	Meals    int
	// Complete: every planned (non-routine) meal of the day holds a plate.
	Complete bool
	// Proposed: the day holds a plate of the proposal.
	Proposed bool
	// NutritionMissing: a food on the day's plates lacks nutrition data, so
	// the macros understate the day.
	NutritionMissing bool
	// Unplated counts routine cells without a plate: they hold no kcal to
	// count, so like a No-plate cell they ask nothing of the day.
	Unplated int
	// Away: every cell of a meal-kind slot (lunch, dinner) that is not
	// routine is No plate, so the user eats elsewhere. H9, S2, the day S3
	// and the weekly averages do not judge the day (light and routine cells
	// do not count).
	Away bool
	// shares are the expected shares of the day's slots (SB2) and share the
	// planned part of the day: 1 minus the shares of the slots not planned (away,
	// default no plate, or routine without a plate). Zero share means an even
	// split over the day's cells (a state without slot bands).
	shares map[int64]float64
	share  float64
	// AwayShare is the sum of the expected shares of the slots not planned.
	AwayShare float64
}

// Floor is the lowest kcal per person the day may hold (H9): 75 % of the
// scaled (normal) target, whatever light or hearty meals or an eat-less day
// moved the day's target to; the absolute floor never moves.
func (d DayKcal) Floor(p Policy) float64 {
	return p.Floor(d.Target, d.Share())
}

// Active is the number of meals the day's targets are judged on: the
// planned meals less the routine cells that hold no plate.
func (d DayKcal) Active() int { return d.Planned - d.Unplated }

// Share is the planned part of the day (SB3): the share of the day's kcal and
// protein target and of the floor. With slot bands it is 1 minus the expected
// shares of the slots not planned; without, Active as a part of all the meals.
func (d DayKcal) Share() float64 {
	if d.share > 0 {
		return d.share
	}
	return float64(d.Active()) / float64(d.Meals)
}

// mealShare is the kcal of one meal of slot id at its expected share of the
// day's full target (SB2): what a light meal's cap and a filling meal's minimum
// are measured from, ignoring marks and aways. Without slot bands it is the
// even share of the planned meals.
func (d DayKcal) mealShare(slotID int64) float64 {
	if sh, ok := d.shares[slotID]; ok {
		return d.Target * sh
	}
	if d.Active() == 0 {
		return 0
	}
	return d.Scaled / float64(d.Active())
}

// DayKcals computes the account of every day with a cell in t that has at
// least one planned (non-No-plate) meal, proposed or not. Nil without a kcal
// target. A routine plate counts toward Kcal.
func DayKcals(s State, t Target) []DayKcal {
	k := s.Profile.KcalTarget
	if k == nil || *k <= 0 {
		return nil
	}
	x := newIndex(s, t)
	return x.dayAccounts(x.days)
}

// dayAccounts is the account of each of days that has a planned meal: the
// one place that sums a day's plates, with or without a kcal target.
func (x *index) dayAccounts(days []time.Time) []DayKcal {
	s := x.s
	var k float64
	if t := s.Profile.KcalTarget; t != nil && *t > 0 {
		k = *t
	}
	var out []DayKcal
	for _, d := range days {
		dk := DayKcal{Date: d, Target: k, Complete: true}
		meals, away, lightBase, heartyBase := 0, 0, 0.0, 0.0
		var cells []Cell
		for _, c := range s.Cells {
			if dayOf(c.Date) == dayOf(d) {
				cells = append(cells, c)
			}
		}
		dk.shares = daySlotShares(s, cells)
		totalShare, unplanned := 0.0, 0.0
		for _, c := range cells {
			dk.Meals++
			totalShare += dk.shares[c.SlotID]
			if c.Mode != availability.ModeRoutine && s.SlotKind[c.SlotID] != slot.KindLight {
				meals++
				if c.Mode == availability.ModeNone {
					away++
				}
			}
			if c.Mode == availability.ModeNone {
				unplanned += dk.shares[c.SlotID]
				continue
			}
			dk.Planned++
			switch {
			case s.lightMeal(c.Date, c.SlotID):
				lightBase += dk.Target * dk.shares[c.SlotID]
			case s.heartyMeal(c.Date, c.SlotID):
				heartyBase += dk.Target * dk.shares[c.SlotID]
			}
			if len(x.byCell[idOf(c.Key())]) == 0 {
				if c.Mode == availability.ModeRoutine {
					dk.Unplated++
					unplanned += dk.shares[c.SlotID]
				} else {
					dk.Complete = false
				}
			}
		}
		if dk.Active() == 0 {
			continue
		}
		if totalShare > 0 {
			dk.share = (totalShare - unplanned) / totalShare
			dk.AwayShare = unplanned / totalShare
		}
		dk.Away = meals > 0 && away == meals
		dk.Scaled = k * dk.Share()
		dk.Adjusted = dk.Scaled
		switch {
		case s.lightDay(d):
			dk.Adjusted *= LightDayShare
		case s.heartyDay(d):
			dk.Adjusted *= HeartyDayShare
		case lightBase+heartyBase > 0:
			dk.Adjusted = x.pol.shiftedTarget(dk.Scaled, lightBase, heartyBase)
		}
		if dk.Adjusted < dk.Scaled {
			// A light day is never asked to go below the floor, so the week
			// is not measured against a target the hard floor forbids.
			dk.Adjusted = min(dk.Scaled, max(dk.Adjusted, x.pol.Floor(k, dk.Share())))
		}
		for _, p := range s.Plates {
			if dayOf(p.Date) != dayOf(d) {
				continue
			}
			for _, c := range p.Components {
				if f, ok := x.food(c.FoodID); ok {
					m := c.Multiplier()
					dk.Kcal += f.Macros.Kcal * m
					dk.ProteinG += f.Macros.Protein * m
					dk.FatG += f.Macros.Fat * m
					dk.CarbsG += f.Macros.Carbs * m
					dk.FiberG += f.Macros.Fiber * m
					dk.FiberMissing = dk.FiberMissing || f.FiberMissing
					dk.NutritionMissing = dk.NutritionMissing || !f.NutritionComplete
				}
			}
			dk.Proposed = dk.Proposed || (p.Proposed() && x.inTarget(p.Cell()))
		}
		dk.Gap = dk.Adjusted - dk.Kcal
		out = append(out, dk)
	}
	return out
}

// writeDayBudgets is the Go-computed day budget table of the planner prompt
// (p17): per day with a meal to plan, the kcal target after light and hearty
// requests, its band, the safety floor, the kcal already fixed (routine and
// confirmed plates), the open meals and what each needs to reach the floor
// and the target, and the light cap or hearty minimum of marked meals. The
// model reads the budget instead of doing the arithmetic.
func writeDayBudgets(b *strings.Builder, x *index) {
	k := x.s.Profile.KcalTarget
	if k == nil || *k <= 0 {
		return
	}
	open := map[int][]string{}
	for _, c := range x.s.Cells {
		if c.Mode == availability.ModePlan && x.inTarget(c.Key()) && len(x.byCell[idOf(c.Key())]) == 0 {
			open[dayOf(c.Date)] = append(open[dayOf(c.Date)], slotName(x.s, c.SlotID))
		}
	}
	first := true
	for _, dk := range x.dayAccounts(x.days) {
		meals := open[dayOf(dk.Date)]
		if dk.Away || (len(meals) == 0 && dk.Active() == dk.Meals) {
			continue
		}
		if first {
			b.WriteString("\nDay budgets per person (computed; do not redo the arithmetic). target is after light and hearty requests, band is the S2 range, floor is H9 (hard), fixed is routine and confirmed kcal, open lists the meals to plan with the kcal each needs for the floor and for the target:\n")
			first = false
		}
		fixed := math.Round(dk.Kcal)
		fmt.Fprintf(b, "- %s: target %.0f (band %.0f-%.0f), floor %.0f, fixed %.0f", weekdayDate(dk.Date), dk.Adjusted,
			dk.Adjusted*x.pol.DayLow, dk.Adjusted*x.pol.DayHigh, dk.Floor(x.pol), fixed)
		if dk.Active() != dk.Meals {
			fmt.Fprintf(b, ", %d of %d meals planned", dk.Active(), dk.Meals)
		}
		if n := float64(len(meals)); n > 0 {
			fmt.Fprintf(b, ", open %s: each needs >=%.0f for the floor, ~%.0f for the target", strings.Join(meals, "+"),
				math.Max(0, (dk.Floor(x.pol)-fixed)/n), math.Max(0, (dk.Adjusted-fixed)/n))
		}
		for _, l := range x.s.Light {
			if l.Date != dk.Date.Format(time.DateOnly) || l.Scope != LightScopeMeal {
				continue
			}
			if l.Hearty() {
				fmt.Fprintf(b, ", %s hearty >=%.0f", l.Slot, HeartyMealShare*dk.mealShare(x.s.slotByName(l.Slot)))
			} else {
				fmt.Fprintf(b, ", %s light <=%.0f", l.Slot, dk.mealCap(x.s.slotByName(l.Slot)))
			}
		}
		b.WriteByte('\n')
	}
}

// kcalNote is the run-notes line of the proposed days that land below the
// S2 band ("" when none does). The gaps are offered to the user afterwards;
// the chooser is never asked to close them.
func kcalNote(s State, t Target) string {
	var parts []string
	pol := PolicyOf(s.Profile)
	for _, d := range DayKcals(s, t) {
		if d.Complete && !d.Away && d.Proposed && !d.NutritionMissing && !s.lightDay(d.Date) && d.Kcal < d.Adjusted*pol.DayLow {
			parts = append(parts, fmt.Sprintf("%s −%.0f kcal", d.Date.Weekday().String()[:3], d.Gap))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "Below target: " + strings.Join(parts, ", ")
}

// floorNote is the run-notes line of the days left below the safety floor
// (H9) after the repairs ("" when none is).
func floorNote(s State, t Target) string {
	var parts []string
	for _, is := range newIndex(s, t).h9() {
		day, _ := time.Parse(time.DateOnly, is.Date)
		kcal, _ := is.Params["kcal"].(float64)
		floor, _ := is.Params["floor_kcal"].(float64)
		parts = append(parts, fmt.Sprintf("%s %.0f of %.0f kcal", day.Weekday().String()[:3], kcal, floor))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Below the daily minimum (the plan could not reach it, please add food to these days): " + strings.Join(parts, ", ")
}

// daySlotShares are the expected shares of the slots of one day's cells,
// normalised over them (SB2): the state's band shares, or an even split when
// the state has none.
func daySlotShares(s State, cells []Cell) map[int64]float64 {
	out := make(map[int64]float64, len(cells))
	total := 0.0
	for _, c := range cells {
		v, ok := s.SlotShare[c.SlotID]
		if !ok {
			v = 1
		}
		out[c.SlotID] = v
		total += v
	}
	for id := range out {
		if total > 0 {
			out[id] /= total
		}
	}
	return out
}

// slotByName is the id of the slot named name (lunch, dinner); 0 if none.
func (s State) slotByName(name string) int64 {
	for id := range s.SlotNames {
		if s.slotIs(id, name) {
			return id
		}
	}
	return 0
}
