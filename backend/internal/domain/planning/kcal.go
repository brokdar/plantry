package planning

import (
	"fmt"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
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
	Target   float64
	Scaled   float64
	Gap      float64 // Scaled - Kcal; positive: under target
	Planned  int
	Meals    int
	// Complete: every planned (non-routine) meal of the day holds a plate.
	Complete bool
	// Proposed: the day holds a plate of the proposal.
	Proposed bool
	// NutritionMissing: a food on the day's plates lacks nutrition data, so
	// the macros understate the day.
	NutritionMissing bool
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
		for _, c := range s.Cells {
			if dayOf(c.Date) != dayOf(d) {
				continue
			}
			dk.Meals++
			if c.Mode == availability.ModeNone {
				continue
			}
			dk.Planned++
			if c.Mode != availability.ModeRoutine && len(x.byCell[idOf(c.Key())]) == 0 {
				dk.Complete = false
			}
		}
		if dk.Planned == 0 {
			continue
		}
		dk.Scaled = k * float64(dk.Planned) / float64(dk.Meals)
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
					dk.NutritionMissing = dk.NutritionMissing || !f.NutritionComplete
				}
			}
			dk.Proposed = dk.Proposed || (p.Proposed() && x.inTarget(p.Cell()))
		}
		dk.Gap = dk.Scaled - dk.Kcal
		out = append(out, dk)
	}
	return out
}

// writeDayBudgets tells the prompt which days have fewer planned meals than
// slots (a No-plate meal): their kcal target is scaled to the planned meals.
func writeDayBudgets(b *strings.Builder, x *index) {
	k := x.s.Profile.KcalTarget
	if k == nil || *k <= 0 {
		return
	}
	first := true
	for _, d := range x.days {
		meals, planned := 0, 0
		for _, c := range x.s.Cells {
			if dayOf(c.Date) == dayOf(d) {
				meals++
				if c.Mode != availability.ModeNone {
					planned++
				}
			}
		}
		if planned == 0 || planned == meals {
			continue
		}
		if first {
			b.WriteString("\nShort days (a No-plate meal): the kcal target of these days is scaled to the planned meals, per person:\n")
			first = false
		}
		fmt.Fprintf(b, "- %s: %d of %d meals planned, target %.0f kcal\n", weekdayDate(d), planned, meals, *k*float64(planned)/float64(meals))
	}
}

// kcalNote is the run-notes line of the proposed days that land below the
// S2 band ("" when none does). The gaps are offered to the user afterwards;
// the chooser is never asked to close them.
func kcalNote(s State, t Target) string {
	var parts []string
	pol := PolicyOf(s.Profile)
	for _, d := range DayKcals(s, t) {
		if d.Complete && d.Proposed && d.Kcal < d.Scaled*pol.DayLow {
			parts = append(parts, fmt.Sprintf("%s −%.0f kcal", d.Date.Weekday().String()[:3], d.Gap))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "Below target: " + strings.Join(parts, ", ")
}
