package planning

import (
	"fmt"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
)

// Calorie thresholds (per person, against the day's scaled target): a day
// below kcalLow triggers one repair attempt, a day above kcalHigh is only
// named in a repair that happens anyway. Neither is a hard rule.
const (
	kcalLow  = 0.7
	kcalHigh = 1.5
	// maxPortionPerEater is the largest portion per eater the prompt offers
	// to close a kcal gap.
	maxPortionPerEater = 1.5
)

// openMeal is a meal of a day the chooser can still change.
type openMeal struct {
	label string
	kcal  float64
	foods string
}

// DayKcal is one day's calorie account per person: the planned kcal, the
// day's target scaled to the meals that are planned (a day with a No-plate
// meal asks less), the gap and the meals the proposal can still change.
type DayKcal struct {
	Date    time.Time
	Kcal    float64
	Target  float64
	Scaled  float64
	Gap     float64 // Scaled - Kcal; positive: under target
	Planned int
	Meals   int
	open    []openMeal
}

// dayKcals computes the account of every day with a target cell whose
// planned meals all hold a plate and that holds a proposed plate. Nil without
// a kcal target.
func (r *repairer) dayKcals() []DayKcal {
	k := r.s.Profile.KcalTarget
	if k == nil || *k <= 0 {
		return nil
	}
	var out []DayKcal
	for _, d := range r.x.days {
		dk := DayKcal{Date: d, Target: *k}
		complete := true
		for _, c := range r.s.Cells {
			if dayOf(c.Date) != dayOf(d) {
				continue
			}
			dk.Meals++
			if c.Mode == availability.ModeNone {
				continue
			}
			dk.Planned++
			if c.Mode != availability.ModeRoutine && len(r.x.byCell[idOf(c.Key())]) == 0 {
				complete = false
			}
		}
		if !complete || dk.Planned == 0 {
			continue
		}
		dk.Scaled = *k * float64(dk.Planned) / float64(dk.Meals)
		for _, p := range r.s.Plates {
			if dayOf(p.Date) != dayOf(d) {
				continue
			}
			pk := 0.0
			var names []string
			for _, c := range p.Components {
				if f, ok := r.x.food(c.FoodID); ok {
					pk += f.Macros.Kcal * c.Multiplier()
				}
				names = append(names, foodName(r.s, c.FoodID))
			}
			dk.Kcal += pk
			if p.Proposed() && r.x.inTarget(p.Cell()) {
				dk.open = append(dk.open, openMeal{label: r.cellLabel(p.Cell()), kcal: pk, foods: strings.Join(names, " + ")})
			}
		}
		if len(dk.open) == 0 {
			continue
		}
		dk.Gap = dk.Scaled - dk.Kcal
		out = append(out, dk)
	}
	return out
}

func (d DayKcal) scaledNote() string {
	if d.Planned == d.Meals {
		return fmt.Sprintf("the target of %.0f", d.Target)
	}
	return fmt.Sprintf("the target of %.0f scaled to %d of %d meals", d.Target, d.Planned, d.Meals)
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

// kcalFindings are the calorie findings of one proposal: trigger lines (days
// below kcalLow of their scaled target, with the gap and the meals that can
// grow), over lines (days far above), and the run-note lines for the same
// days that stay unresolved at the end.
type kcalFindings struct {
	trigger []string
	over    []string
	notes   []string
}

func (r *repairer) kcalFindings() kcalFindings {
	var kf kcalFindings
	for _, d := range r.dayKcals() {
		day := weekdayDate(d.Date)
		switch {
		case d.Kcal < d.Scaled*kcalLow:
			meals := make([]string, len(d.open))
			for i, m := range d.open {
				meals[i] = fmt.Sprintf("%s (%s, %.0f kcal)", m.label, m.foods, m.kcal)
			}
			kf.trigger = append(kf.trigger, fmt.Sprintf("kcal (no rule, but fix it): %s has %.0f kcal per person against %s, %.0f short. Meals you can change: %s. Fix: give a main up to %s portions per eater (raise its session's portions to match), or add a side, dessert or drink recipe to one of these plates; the day then lands near %.0f.",
				day, d.Kcal, d.scaledNote(), d.Gap, strings.Join(meals, "; "), num(maxPortionPerEater), d.Scaled))
			kf.notes = append(kf.notes, fmt.Sprintf("%s is %.0f kcal under target (%.0f of %.0f).", d.Date.Weekday().String()[:3], d.Gap, d.Kcal, d.Scaled))
		case d.Kcal > d.Scaled*kcalHigh:
			kf.over = append(kf.over, fmt.Sprintf("kcal (no rule): %s has %.0f kcal per person against %s. Fix: swap a plate for a lighter recipe or drop a side so the day lands near %.0f.",
				day, d.Kcal, d.scaledNote(), d.Scaled))
			kf.notes = append(kf.notes, fmt.Sprintf("%s is %.0f kcal over target (%.0f of %.0f).", d.Date.Weekday().String()[:3], -d.Gap, d.Kcal, d.Scaled))
		}
	}
	return kf
}
