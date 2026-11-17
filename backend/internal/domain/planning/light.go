package planning

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// A light request ("something light on weekdays") is structured data, not
// free text: a list of LightIntent. A dietitian first makes a MEAL lighter
// and keeps the day total in its band; only an explicit "eat less" lowers
// the day. The intent belongs to the run that was given it (State.Light, the
// run log's plan.evaluation steps record it), not to the plan: a later
// evaluation or regenerate knows it only when it is told again.
const (
	LightScopeMeal = "meal"
	LightScopeDay  = "day"
	// LightMealShare: a light meal holds at most this share of the meal's
	// normal share of the day's target (the target split evenly over the
	// planned meals); the day's target is unchanged, the other meals carry
	// the rest.
	LightMealShare = 0.75
	// LightDayShare: a light day's target is this share of the normal one
	// and its band is centred there. The floors (H9) are not lowered.
	LightDayShare = 0.85
)

// LightIntent is one light request: a meal (Slot lunch or dinner) or the
// whole day of Date (YYYY-MM-DD).
type LightIntent struct {
	Date  string `json:"date"`
	Scope string `json:"scope"`
	Slot  string `json:"slot,omitempty"`
}

// lightSlots are the slots a meal intent may name.
var lightSlots = []string{"lunch", "dinner"}

// ValidateLight checks the intents and returns them without duplicates; a
// day intent drops its slot.
func ValidateLight(in []LightIntent) ([]LightIntent, error) {
	var out []LightIntent
	for i, l := range in {
		if _, err := time.Parse(time.DateOnly, l.Date); err != nil {
			return nil, fmt.Errorf("light[%d].date %q is not a YYYY-MM-DD date", i, l.Date)
		}
		switch l.Scope {
		case LightScopeDay:
			l.Slot = ""
		case LightScopeMeal:
			if !slices.Contains(lightSlots, l.Slot) {
				return nil, fmt.Errorf("light[%d]: scope meal needs slot lunch or dinner", i)
			}
		default:
			return nil, fmt.Errorf("light[%d].scope %q must be meal or day", i, l.Scope)
		}
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out, nil
}

// UnionLight is a followed by the intents of b that a lacks.
func UnionLight(a, b []LightIntent) []LightIntent {
	out := slices.Clone(a)
	for _, l := range b {
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// lightDay reports whether the day of date is a light day.
func (s State) lightDay(date time.Time) bool {
	return slices.Contains(s.Light, LightIntent{Date: date.Format(time.DateOnly), Scope: LightScopeDay})
}

// lightMeal reports whether the meal in slot on date is a light meal.
func (s State) lightMeal(date time.Time, slot int64) bool {
	d := date.Format(time.DateOnly)
	return slices.ContainsFunc(s.Light, func(l LightIntent) bool {
		return l.Scope == LightScopeMeal && l.Date == d && s.slotIs(slot, l.Slot)
	})
}

// slotIs reports whether slot id carries the short name (lunch, dinner).
func (s State) slotIs(id int64, name string) bool {
	n, ok := s.SlotNames[id]
	return ok && strings.TrimPrefix(n, "slot.") == name
}

// mealCap is the most kcal per eater a light meal may hold: LightMealShare
// of the day's normal target split evenly over its planned meals.
func (dk DayKcal) mealCap() float64 {
	if dk.Planned == 0 {
		return 0
	}
	return LightMealShare * dk.Scaled / float64(dk.Planned)
}

// lightIssues reports S6 `meal_not_light` for every light meal whose plate
// holds more kcal than its cap (soft: it never blocks saving).
func (x *index) lightIssues(ev Evaluation) []Issue {
	var out []Issue
	for _, l := range x.s.Light {
		day, err := time.Parse(time.DateOnly, l.Date)
		if l.Scope != LightScopeMeal || err != nil {
			continue
		}
		var dk *DayKcal
		for _, a := range x.dayAccounts([]time.Time{day}) {
			dk = &a
		}
		if dk == nil || dk.NutritionMissing || dk.Target <= 0 {
			continue
		}
		for _, p := range ev.Plates {
			if p.Date != l.Date || p.Kcal <= 0 || !x.s.slotIs(p.SlotID, l.Slot) {
				continue
			}
			if limit := dk.mealCap(); p.Kcal > limit+1e-9 {
				out = append(out, issue("S6", CodeMealNotLight, SeveritySoft).
					atCell(plate.CellKey{Date: day, SlotID: p.SlotID}).
					with("kcal", math.Round(p.Kcal)).with("max_kcal", math.Round(limit)).
					with("normal_share_kcal", math.Round(dk.Scaled/float64(dk.Planned))))
			}
		}
	}
	return out
}

// lightOf is "day" for a light day, "meal" when one of its meals is light,
// else "".
func (x *index) lightOf(date time.Time) string {
	switch {
	case x.s.lightDay(date):
		return LightScopeDay
	case slices.ContainsFunc(x.s.Cells, func(c Cell) bool { return dayOf(c.Date) == dayOf(date) && x.s.lightMeal(c.Date, c.SlotID) }):
		return LightScopeMeal
	}
	return ""
}
