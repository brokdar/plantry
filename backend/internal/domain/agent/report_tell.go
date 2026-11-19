package agent

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
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
			resized = append(resized, fmt.Sprintf("%s from %s to %s%s", n, before[n], after[n], qtyDelta(before[n], after[n])))
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
	// A day away: its removed meals and the away line are one item.
	awayDays := map[string]bool{}
	for _, d := range r.Days {
		if strings.HasPrefix(d.Status, "away") {
			awayDays[d.Day] = true
		}
	}
	gone := map[string][]string{}
	var goneOrder []string
	for _, m := range r.Changed {
		if len(m.After) == 0 && awayDays[m.Day] {
			if _, ok := gone[m.Day]; !ok {
				goneOrder = append(goneOrder, m.Day)
			}
			gone[m.Day] = append(gone[m.Day], m.Meal)
			continue
		}
		add(mealChange(m))
	}
	for _, day := range goneOrder {
		add(fmt.Sprintf("%s is away: %s have no meal and the day is not counted.", day, strings.Join(gone[day], " and ")))
	}
	// Where the day's protein stands, in grams, when this change moved it by
	// 5 g or more.
	for _, a := range in.after.Days {
		for _, b := range in.before.Days {
			if b.Date != a.Date || a.Away || absf(a.ProteinG-b.ProteinG) < 5 || !touchedDate(r, a.Date) {
				continue
			}
			day := dayLabelStr(a.Date)
			switch {
			case a.ProteinTargetG > 0 && b.ProteinG >= a.ProteinTargetG && a.ProteinG >= b.ProteinG:
				add(fmt.Sprintf("%s already held %.0f g of protein, above the %.0f g it needs; now %.0f g.", day, b.ProteinG, a.ProteinTargetG, a.ProteinG))
			case a.ProteinTargetG > 0:
				add(fmt.Sprintf("%s: protein %.0f g → %.0f g (it needs %.0f g).", day, b.ProteinG, a.ProteinG, a.ProteinTargetG))
			default:
				add(fmt.Sprintf("%s: protein %.0f g → %.0f g.", day, b.ProteinG, a.ProteinG))
			}
		}
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
			why := "a meal is light or filling"
			var parts []string
			for _, lm := range d.LightMeals {
				if adj, slot, ok := strings.Cut(plainWords(lm), " "); ok {
					parts = append(parts, fmt.Sprintf("the %s is %s", slot, adj))
				}
			}
			if len(parts) > 0 {
				why = strings.Join(parts, " and ")
			}
			add(fmt.Sprintf("%s: the amount that suits the day is now %.0f kcal (was %.0f) because %s.", dayLabelStr(d.Date), *d.TargetKcal, *before, why))
		}
	}
	for _, d := range r.Days {
		switch {
		case strings.HasPrefix(d.Status, "away"):
			if _, said := gone[d.Day]; !said {
				add(d.Day + " is away: the day is not counted.")
			}
		default:
			s := fmt.Sprintf("%s holds %d kcal (was %d)", d.Day, d.KcalAfter, d.KcalBefore)
			if d.Range != "" && d.Status != "" {
				s += fmt.Sprintf(", %s (%s)", suitsPhrase(d.Status), d.Range)
			}
			add(s + ".")
		}
	}
	// Cooking: a day with a changed meal says everything cooked that day (what
	// stays too); any other day only a cooking whose amount changed.
	touched := map[string]bool{}
	for _, d := range r.Days {
		touched[d.Day] = true
	}
	for _, m := range r.Changed {
		touched[m.Day] = true
	}
	beforeCook, afterCook := cookingByDay(in.beforeSt), cookingByDay(in.afterSt)
	for _, c := range r.Cooking {
		if len(c.Cooked) == 0 {
			if !awayDays[c.Day] {
				add(c.Day + ": nothing is cooked.")
			}
			continue
		}
		if touched[c.Day] {
			var parts []string
			for _, d := range c.Cooked {
				parts = append(parts, fmt.Sprintf("%s at %s for %s", food.Quote(d.Dish), d.At, strings.Join(d.Eaten, ", ")))
			}
			add(fmt.Sprintf("%s you cook (%d cooking time%s): %s.", c.Day, c.CookingTimes, plural(c.CookingTimes), strings.Join(parts, "; ")))
			continue
		}
		add(otherDayCooking(c.Day, beforeCook, afterCook, strings.Join(r.Leftovers, "\n")))
	}
	// A cooking time that got longer, or one more dish in it, with the dishes
	// and the total time.
	for _, line := range longerCooking(in) {
		add(line)
	}
	moved := strings.Join(r.Leftovers, "\n")
	for _, s := range r.AlsoChanged {
		// A cooking that moved is said once, by the leftovers line; a resize
		// is said there too.
		if strings.Contains(s, "cooking was resized") || (strings.Contains(s, " is now cooked at ") && strings.Contains(moved, quotedDish(s)+" is now cooked on")) {
			continue
		}
		add(s)
	}
	for _, s := range r.Leftovers {
		add(s)
	}
	if len(r.Week) > 0 {
		add("For the week this means: " + strings.Join(r.Week, "; ") + ".")
	}
	add(r.Neutral)
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

// touchedDate reports a date the change itself touched (a changed meal sits on it).
func touchedDate(r changeReport, iso string) bool {
	day := dayLabelStr(iso)
	for _, m := range r.Changed {
		if m.Day == day {
			return true
		}
	}
	for _, d := range r.Days {
		if d.Day == day {
			return true
		}
	}
	return false
}

// suitsPhrase says a day status the way a cook does.
func suitsPhrase(status string) string {
	status = strings.Replace(status, "inside its range", "a good amount for the day", 1)
	status = strings.Replace(status, "under its range", "below a good amount for the day", 1)
	status = strings.Replace(status, "over its range", "above a good amount for the day", 1)
	return status
}

// qtyDelta says how much a quantity changed: " (half a portion more)".
func qtyDelta(from, to string) string {
	var a, b float64
	var ua, ub string
	if _, err := fmt.Sscanf(from, "%f %s", &a, &ua); err != nil {
		return ""
	}
	if _, err := fmt.Sscanf(to, "%f %s", &b, &ub); err != nil || strings.TrimSuffix(ua, "s") != strings.TrimSuffix(ub, "s") {
		return ""
	}
	d := b - a
	if d == 0 {
		return ""
	}
	dir := "more"
	if d < 0 {
		dir, d = "less", -d
	}
	amount := fmtNum(d) + " g"
	if strings.HasPrefix(ua, "portion") {
		switch d {
		case 0.25:
			amount = "a quarter portion"
		case 0.5:
			amount = "half a portion"
		case 0.75:
			amount = "three quarters of a portion"
		case 1:
			amount = "one portion"
		default:
			amount = fmtNum(d) + " portions"
		}
	}
	return " (" + amount + " " + dir + ")"
}

// otherDayCooking says the cooking that changed on a day no changed meal sits
// on: what it now cooks and what it no longer cooks.
func otherDayCooking(day string, before, after map[string][]reportDish, moved string) string {
	var now, gone []string
	for iso, dishes := range after {
		if dayLabelStr(iso) != day {
			continue
		}
		for _, d := range dishes {
			found := false
			for _, w := range before[iso] {
				found = found || (w.Dish == d.Dish && w.At == d.At)
			}
			if !found && !strings.Contains(moved, food.Quote(d.Dish)+" is now cooked on") {
				now = append(now, fmt.Sprintf("now cooks %s at %s for %s", food.Quote(d.Dish), d.At, strings.Join(d.Eaten, ", ")))
			}
		}
	}
	for iso, dishes := range before {
		if dayLabelStr(iso) != day {
			continue
		}
		for _, w := range dishes {
			found := false
			for _, d := range after[iso] {
				found = found || (w.Dish == d.Dish && w.At == d.At)
			}
			if !found && !strings.Contains(moved, food.Quote(w.Dish)+" is now cooked on") {
				gone = append(gone, "no longer cooks "+food.Quote(w.Dish))
			}
		}
	}
	slices.Sort(now)
	slices.Sort(gone)
	all := append(slices.Clone(now), gone...)
	if len(all) == 0 {
		return ""
	}
	return day + " cooking changes: " + strings.Join(all, "; ") + "."
}

type cookCell struct {
	dishes  []string
	minutes int
}

// cookCells is, per cook cell (iso date + slot), the dishes cooked there for a
// meal and their minutes (prep and cook) in all; a breakfast batch is no cooking.
func cookCells(st planning.State) map[string]cookCell {
	eaten := map[int64]bool{}
	for _, p := range st.Plates {
		for _, c := range p.Components {
			if c.SessionID != 0 {
				eaten[c.SessionID] = true
			}
		}
	}
	out := map[string]cookCell{}
	for _, s := range st.Sessions {
		if !eaten[s.ID] || st.SlotKind[s.CookSlotID] == slot.KindLight {
			continue
		}
		k := fmt.Sprintf("%s|%d", s.CookDate.Format("2006-01-02"), s.CookSlotID)
		c := out[k]
		c.dishes = append(c.dishes, food.Quote(foodName(st, s.FoodID)))
		c.minutes += st.Foods[s.FoodID].Minutes
		out[k] = c
	}
	return out
}

func duration(min int) string {
	switch {
	case min <= 0:
		return "no time"
	case min < 60:
		return fmt.Sprintf("%d min", min)
	case min%60 == 0:
		return fmt.Sprintf("%d h", min/60)
	}
	return fmt.Sprintf("%d h %d min", min/60, min%60)
}

// longerCooking lists the cook cells whose total time grew by 30 minutes or
// more, or that got another dish, with the dishes and the time.
func longerCooking(in reportInput) []string {
	before, after := cookCells(in.beforeSt), cookCells(in.afterSt)
	keys := make([]string, 0, len(after))
	for k := range after {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var out []string
	for _, k := range keys {
		a, b := after[k], before[k]
		if (a.minutes-b.minutes < 30 && len(a.dishes) <= len(b.dishes)) || a.minutes == 0 {
			continue
		}
		date, slotID := "", int64(0)
		if i := strings.IndexByte(k, '|'); i > 0 {
			date = k[:i]
			_, _ = fmt.Sscan(k[i+1:], &slotID)
		}
		was := "nothing"
		if b.minutes > 0 {
			was = duration(b.minutes)
		}
		out = append(out, fmt.Sprintf("%s at %s you now cook %s: about %s in all (was %s).", dayLabelStr(date), slotWord(in.afterSt, slotID), strings.Join(a.dishes, ", "), duration(a.minutes), was))
	}
	return out
}
