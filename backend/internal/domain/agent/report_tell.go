package agent

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

const (
	maxTell     = 16
	maxTellText = 260
)

// tellQty splits "Name 1.5 portions" / "Name 80 g" into name and quantity.
var tellQty = regexp.MustCompile(`^(.*?) ([\d.]+ (?:portions?|g))$`)

func splitQty(s string) (name, qty string) {
	if m := tellQty.FindStringSubmatch(s); m != nil {
		return m[1], m[2]
	}
	return s, ""
}

// mealChange says what happened to one meal's foods in plain words.
func mealChange(m reportMeal) string {
	where := m.Day + " " + m.Meal
	before, after := map[string]string{}, map[string]string{}
	var bn, an []string
	for _, s := range m.Before {
		n, q := splitQty(s)
		before[n] = q
		bn = append(bn, n)
	}
	for _, s := range m.After {
		n, q := splitQty(s)
		after[n] = q
		an = append(an, n)
	}
	var removed, added, resized []string
	for _, n := range bn {
		if _, ok := after[n]; !ok {
			removed = append(removed, n)
		}
	}
	for _, n := range an {
		if _, ok := before[n]; !ok {
			added = append(added, n)
		} else if before[n] != after[n] && before[n] != "" && after[n] != "" {
			resized = append(resized, fmt.Sprintf("%s from %s to %s", n, before[n], after[n]))
		}
	}
	var parts []string
	switch {
	case len(removed) == 1 && len(added) == 1:
		parts = append(parts, fmt.Sprintf("%s replaced by %s", removed[0], added[0]))
	default:
		if len(removed) > 0 {
			parts = append(parts, "removed "+strings.Join(removed, ", "))
		}
		if len(added) > 0 {
			parts = append(parts, "added "+strings.Join(added, ", "))
		}
	}
	parts = append(parts, resized...)
	if m.MovedFrom != "" {
		if len(parts) == 0 {
			parts = append(parts, strings.Join(bn, " + ")+" moved here from "+m.MovedFrom)
		} else {
			parts = append(parts, "moved here from "+m.MovedFrom)
		}
	}
	if len(m.After) == 0 {
		parts = []string{"no meal any more"}
	}
	s := where + ": " + strings.Join(parts, "; ")
	if m.KcalFrom != nil && m.KcalTo != nil && *m.KcalFrom != *m.KcalTo {
		s += fmt.Sprintf(" (%d → %d kcal)", *m.KcalFrom, *m.KcalTo)
	}
	return s + "."
}

// buildTell is the list the reply must convey, in order: what changed, why
// a dish was swapped (meat), the days against their goal, what is cooked when,
// cooking that moved, batches left over or no longer cooked, the week, the
// warnings. Every item is a plain sentence from the saved plan.
func buildTell(in reportInput, r changeReport) []string {
	var out []string
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			if len([]rune(s)) > maxTellText {
				s = string([]rune(s)[:maxTellText]) + "…"
			}
			out = append(out, s)
		}
	}
	for _, m := range r.Changed {
		add(mealChange(m))
	}
	// Why a dish was swapped: the meat that left the day with it.
	bd, ad := map[string]struct{ meat []string }{}, map[string]struct{ meat []string }{}
	for _, d := range in.before.Days {
		bd[d.Date] = struct{ meat []string }{d.MeatFoods}
	}
	for _, d := range in.after.Days {
		ad[d.Date] = struct{ meat []string }{d.MeatFoods}
	}
	for _, m := range r.Changed {
		if len(m.Before) == 0 || len(m.After) == 0 || m.MovedFrom != "" {
			continue
		}
		date := ""
		for _, p := range in.beforeSt.Plates {
			if dayLabel(p.Date) == m.Day && slotWord(in.beforeSt, p.SlotID) == m.Meal {
				date = p.Date.Format("2006-01-02")
			}
		}
		if date == "" {
			continue
		}
		var gone []string
		for _, f := range bd[date].meat {
			if !slices.Contains(ad[date].meat, f) {
				gone = append(gone, food.Data(f))
			}
		}
		if len(gone) > 0 {
			add(fmt.Sprintf("Meat that leaves %s with this change: %s.", m.Day, strings.Join(gone, ", ")))
		}
	}
	// The day's goal moved with a light or filling mark.
	for _, d := range in.after.Days {
		b, ok := bd[d.Date]
		_ = b
		if !ok || d.TargetKcal == nil {
			continue
		}
		var before *float64
		for _, x := range in.before.Days {
			if x.Date == d.Date {
				before = x.TargetKcal
			}
		}
		if before != nil && !d.Away && absf(*d.TargetKcal-*before) >= 5 {
			why := "a light or filling meal is marked"
			if len(d.LightMeals) > 0 {
				why = "the " + plainWords(strings.Join(d.LightMeals, " and the ")) + " is marked"
			}
			add(fmt.Sprintf("%s: the day's goal moved from %.0f to %.0f kcal because %s.", dayLabelStr(d.Date), *before, *d.TargetKcal, why))
		}
	}
	for _, d := range r.Days {
		switch {
		case strings.HasPrefix(d.Status, "away"):
			add(d.Day + " is away: the day is not counted.")
		default:
			s := fmt.Sprintf("%s holds %d kcal (was %d)", d.Day, d.KcalAfter, d.KcalBefore)
			if d.Range != "" && d.Status != "" {
				s += fmt.Sprintf(", %s (%s)", d.Status, d.Range)
			}
			add(s + ".")
			// Protein before and after, in grams, when the day's protein moved.
			for _, a := range in.after.Days {
				if dayLabelStr(a.Date) != d.Day {
					continue
				}
				for _, b := range in.before.Days {
					if b.Date == a.Date && absf(a.ProteinG-b.ProteinG) >= 5 {
						p := fmt.Sprintf("%s: protein %.0f g → %.0f g", d.Day, b.ProteinG, a.ProteinG)
						if a.ProteinTargetG > 0 {
							p += fmt.Sprintf(" (wanted %.0f g)", a.ProteinTargetG)
						}
						add(p + ".")
					}
				}
			}
		}
	}
	for _, c := range r.Cooking {
		if len(c.Cooked) == 0 {
			add(c.Day + ": nothing is cooked.")
			continue
		}
		var parts []string
		for _, d := range c.Cooked {
			parts = append(parts, fmt.Sprintf("%s at %s for %s", food.Quote(d.Dish), d.At, strings.Join(d.Eaten, ", ")))
		}
		add(fmt.Sprintf("%s you cook (%d cooking time%s): %s.", c.Day, c.CookingTimes, plural(c.CookingTimes), strings.Join(parts, "; ")))
	}
	moved := strings.Join(r.Leftovers, "\n")
	for _, s := range r.AlsoChanged {
		// A cooking that moved is said once, by the leftovers line; a resize
		// that fits is no news.
		if strings.Contains(s, "was resized") || (strings.Contains(s, " is now cooked at ") && strings.Contains(moved, quotedDish(s)+" is now cooked on")) {
			continue
		}
		add(s)
	}
	for _, s := range r.Leftovers {
		if !strings.Contains(s, "nothing is left over") { // a resize that fits is no news
			add(s)
		}
	}
	for _, s := range r.Week {
		add(s)
	}
	for _, s := range r.NewWarnings {
		add("New warning: " + s)
	}
	for _, s := range r.Worse {
		add("Worse: " + s)
	}
	if len(out) > maxTell {
		out = out[:maxTell]
	}
	return out
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// quotedDish is the first quoted name in s, with its quotes.
func quotedDish(s string) string {
	i := strings.IndexByte(s, '"')
	if i < 0 {
		return "\x00"
	}
	j := strings.IndexByte(s[i+1:], '"')
	if j < 0 {
		return "\x00"
	}
	return s[i : i+j+2]
}
