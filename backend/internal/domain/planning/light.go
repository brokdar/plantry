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
// the day. The intents are stored per date (plan_light, LightStore) and
// loaded into State.Light by the context builder, so the board, the chat
// and every later run evaluate the plan under them; a run or chat call that
// is given light for a date replaces that date's stored intents.
const (
	LightScopeMeal = "meal"
	LightScopeDay  = "day"
	// LightScopeNone clears the stored intents of its date (input only; it
	// never reaches State.Light).
	LightScopeNone = "none"
	// MaxLightDays caps the day-scope intents in one plan window: a lighter
	// day is a deviation, a lasting calorie cut is the profile's goal lose.
	MaxLightDays = 3
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
		case LightScopeDay, LightScopeNone:
			l.Slot = ""
		case LightScopeMeal:
			if !slices.Contains(lightSlots, l.Slot) {
				return nil, fmt.Errorf("light[%d]: scope meal needs slot lunch or dinner", i)
			}
		default:
			return nil, fmt.Errorf("light[%d].scope %q must be meal, day or none", i, l.Scope)
		}
		if !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out, nil
}

// MergeLight is stored with every date that in names replaced by in's
// intents of that date (none intents only clear): the state's light after a
// call that was given in.
func MergeLight(stored, in []LightIntent) []LightIntent {
	var out []LightIntent
	for _, l := range stored {
		if !slices.ContainsFunc(in, func(i LightIntent) bool { return i.Date == l.Date }) {
			out = append(out, l)
		}
	}
	for _, l := range in {
		if l.Scope != LightScopeNone && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return out
}

// LightDates are the distinct dates of in, in order.
func LightDates(in []LightIntent) []string {
	var out []string
	for _, l := range in {
		if !slices.Contains(out, l.Date) {
			out = append(out, l.Date)
		}
	}
	return out
}

// ResolveLight checks validated intents against the state they apply to: a
// date outside the window, a meal slot the plan does not have, or more than
// MaxLightDays light days in the window once merged with the stored ones is
// an error for the caller to hand back to the model.
func ResolveLight(s State, in []LightIntent) error {
	for _, l := range in {
		d, _ := time.Parse(time.DateOnly, l.Date)
		if !s.Window.Contains(d) {
			return fmt.Errorf("light date %s is outside the plan window %s to %s", l.Date, s.Window.Start.Format(time.DateOnly), s.Window.End.Format(time.DateOnly))
		}
		if l.Scope == LightScopeMeal && !s.hasSlot(l.Slot) {
			return fmt.Errorf("light on %s: this plan has no %s slot, so the meal cannot be made light", l.Date, l.Slot)
		}
	}
	days := 0
	for _, l := range MergeLight(s.Light, in) {
		if l.Scope == LightScopeDay {
			days++
		}
	}
	if days > MaxLightDays {
		return fmt.Errorf("at most %d light days per plan window (this would make %d): a lasting calorie cut is the profile's goal lose, not light days; keep the days the user asked for and drop the rest", MaxLightDays, days)
	}
	return nil
}

// hasSlot reports whether the plan has a slot with the short name.
func (s State) hasSlot(name string) bool {
	for id := range s.SlotNames {
		if s.slotIs(id, name) {
			return true
		}
	}
	return false
}

// LightSchema is the `light` property of the tools that take light requests
// (submit_plan, evaluate_plan and the chat tools), one definition for all.
// withClear adds scope none, which the chat tools use to drop a date's
// stored request.
func LightSchema(withClear bool) string {
	scopes, clear := `"meal","day"`, ""
	if withClear {
		scopes = `"meal","day","none"`
		clear = " scope none: remove that date's light request (stored requests of other dates stay)."
	}
	return fmt.Sprintf(`"light":{
      "type":"array","maxItems":14,
      "description":"The user's light requests, structured, one entry per date; omit when there is none. Requests are stored: a date you list replaces that date's stored request, other dates keep theirs and every evaluation already stands under them, so list only what the user asked for now. scope meal: that lunch or dinner is lighter (at most 75%% of its even share of the day's target) and the day keeps its target. scope day: ONLY when the user explicitly asks to eat less that day; the day's target becomes 85%%; at most %d per plan window (a lasting cut is the profile's goal lose). Dates must lie in the plan window.%s",
      "items":{
        "type":"object","required":["date","scope"],
        "properties":{
          "date":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
          "scope":{"type":"string","enum":[%s]},
          "slot":{"type":"string","enum":["lunch","dinner"],"description":"Required for scope meal."}
        },
        "additionalProperties":false
      }
    }`, MaxLightDays, clear, scopes)
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
			if p.Date != l.Date || p.Kcal <= 0 || !x.s.slotIs(p.SlotID, l.Slot) || x.unfixable(p) {
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

// unfixable reports whether the plate of pe is one no run may change: a
// confirmed plate or one on a routine cell. S6 would never go away for it.
func (x *index) unfixable(pe PlateEval) bool {
	for _, p := range x.s.Plates {
		if p.ID == pe.PlateID {
			return p.Confirmed || x.routine(p.Cell())
		}
	}
	return false
}

// lightWords are the words of the run-notes line, per language.
var lightWords = map[string]struct {
	prefix   string
	day      string
	slots    map[string]string
	weekdays [7]string
}{
	"en": {"Light", "day", map[string]string{"lunch": "lunch", "dinner": "dinner"}, [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}},
	"de": {"Leicht", "Tag", map[string]string{"lunch": "Mittagessen", "dinner": "Abendessen"}, [7]string{"So", "Mo", "Di", "Mi", "Do", "Fr", "Sa"}},
}

// lightNote is the run-notes line of the light requests the plan stands
// under ("Light: Tue dinner, Thu day"; "" without any), in the notes'
// language.
func lightNote(light []LightIntent, locale string) string {
	if len(light) == 0 {
		return ""
	}
	w := lightWords["en"]
	if locale == "de" {
		w = lightWords["de"]
	}
	sorted := slices.Clone(light)
	slices.SortStableFunc(sorted, func(a, b LightIntent) int { return strings.Compare(a.Date, b.Date) })
	parts := make([]string, 0, len(sorted))
	for _, l := range sorted {
		d, err := time.Parse(time.DateOnly, l.Date)
		if err != nil {
			continue
		}
		what := w.day
		if l.Scope == LightScopeMeal {
			what = w.slots[l.Slot]
		}
		parts = append(parts, w.weekdays[d.Weekday()]+" "+what)
	}
	return w.prefix + ": " + strings.Join(parts, ", ")
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
