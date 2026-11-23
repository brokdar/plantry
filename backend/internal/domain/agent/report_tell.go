package agent

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
)

// tellQty splits "Name 1.5 portions" / "Name 80 g" into name and quantity.
var tellQty = regexp.MustCompile(`^(.*?) ([\d.]+ (?:portions?|g))$`)

func splitQty(s string) (name, qty string) {
	if m := tellQty.FindStringSubmatch(s); m != nil {
		return m[1], m[2]
	}
	return s, ""
}

// deQty says a quantity from splitQty in German: "1.5 portions" is "1,5 Portionen".
func deQty(q string) string {
	n, unit, _ := strings.Cut(q, " ")
	n = strings.Replace(n, ".", ",", 1)
	if strings.HasPrefix(unit, "portion") {
		if n == "1" {
			return "1 Portion"
		}
		return n + " Portionen"
	}
	return n + " " + unit
}

// mealChange says what happened to one meal's foods in plain words.
func mealChange(m reportMeal) string { return mealChangeL(m, langEN) }

// mealChangeL is mealChange in one language.
func mealChangeL(m reportMeal, lg lang) string {
	de := lg == langDE
	q := func(name string) string {
		if de {
			return deQuote(name)
		}
		return food.Quote(name)
	}
	where := m.Day + " " + m.Meal
	if de {
		where = deCell(where)
	}
	// Components are keyed by name and their place among the same-named ones, so
	// two components of one name stay two.
	type comp struct{ key, name, qty string }
	read := func(list []string) (out []comp) {
		seen := map[string]int{}
		for _, s := range list {
			n, q := splitQty(s)
			seen[n]++
			out = append(out, comp{key: fmt.Sprintf("%s#%d", n, seen[n]), name: n, qty: q})
		}
		return out
	}
	bc, ac := read(m.Before), read(m.After)
	before, after := map[string]comp{}, map[string]comp{}
	var bn []string
	for _, c := range bc {
		before[c.key] = c
		bn = append(bn, q(c.name))
	}
	for _, c := range ac {
		after[c.key] = c
	}
	var removed, added, resized []string
	for _, c := range bc {
		if _, ok := after[c.key]; !ok {
			removed = append(removed, q(c.name))
		}
	}
	for _, c := range ac {
		b, ok := before[c.key]
		switch {
		case !ok:
			added = append(added, q(c.name))
		case b.qty != c.qty && b.qty != "" && c.qty != "":
			if de {
				resized = append(resized, fmt.Sprintf("%s von %s auf %s%s", q(c.name), deQty(b.qty), deQty(c.qty), qtyDeltaL(b.qty, c.qty, lg)))
			} else {
				resized = append(resized, fmt.Sprintf("%s from %s to %s%s", q(c.name), b.qty, c.qty, qtyDeltaL(b.qty, c.qty, lg)))
			}
		}
	}
	var parts []string
	switch {
	case len(removed) == 1 && len(added) == 1:
		if de {
			parts = append(parts, fmt.Sprintf("%s ersetzt durch %s", removed[0], added[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%s replaced by %s", removed[0], added[0]))
		}
	default:
		if len(removed) > 0 {
			if de {
				parts = append(parts, "entfernt: "+strings.Join(removed, ", "))
			} else {
				parts = append(parts, "removed "+strings.Join(removed, ", "))
			}
		}
		if len(added) > 0 {
			if de {
				parts = append(parts, "hinzugefügt: "+strings.Join(added, ", "))
			} else {
				parts = append(parts, "added "+strings.Join(added, ", "))
			}
		}
	}
	parts = append(parts, resized...)
	if m.MovedFrom != "" {
		from, hereDE := m.MovedFrom, "hierher verschoben von "+deCell(m.MovedFrom)
		switch {
		case len(parts) == 0 && de:
			parts = append(parts, strings.Join(bn, " + ")+" "+hereDE)
		case len(parts) == 0:
			parts = append(parts, strings.Join(bn, " + ")+" moved here from "+from)
		case de:
			parts = append(parts, hereDE)
		default:
			parts = append(parts, "moved here from "+from)
		}
	}
	if len(m.After) == 0 {
		if de {
			parts = []string{"kein Essen mehr"}
		} else {
			parts = []string{"no meal any more"}
		}
	}
	if len(parts) == 0 {
		return "" // nothing about this meal changed in words
	}
	s := where + ": " + strings.Join(parts, "; ")
	if m.KcalFrom != nil && m.KcalTo != nil && *m.KcalFrom != *m.KcalTo {
		s += fmt.Sprintf(" (%d → %d kcal)", *m.KcalFrom, *m.KcalTo)
	}
	return s + "."
}

// buildFacts adds the facts of a change that come from the changed meals, days
// and cooking of the report (the others, leftovers, week and warnings, are added
// while the report is built): every fact one line of one class, in both
// languages. The caller selects the block (selectBlock).
func buildFacts(in reportInput, r changeReport) {
	s := in.facts
	// A day away: its removed meals and the away line are one fact.
	awayDays := map[string]bool{}
	for _, d := range r.Days {
		if strings.HasPrefix(d.Status, "away") {
			awayDays[d.Day] = true
		}
	}
	gone := map[string][]string{}
	var goneOrder []string
	out := dayOuts(in, r, awayDays)
	for _, m := range r.Changed {
		if len(m.After) == 0 && out[m.Day] != nil {
			continue // told in the day's one line below
		}
		if len(m.After) == 0 && awayDays[m.Day] {
			if _, ok := gone[m.Day]; !ok {
				goneOrder = append(goneOrder, m.Day)
			}
			gone[m.Day] = append(gone[m.Day], m.Meal)
			continue
		}
		s.add(classChanged, kindMeal, statusOK, mealChangeL(m, langEN), mealChangeL(m, langDE))
	}
	for _, day := range goneOrder {
		var de []string
		for _, m := range gone[day] {
			de = append(de, deSlot(m))
		}
		s.add(classChanged, kindAway, statusOK,
			fmt.Sprintf("%s is away: %s have no meal and the day is not counted.", day, strings.Join(gone[day], " and ")),
			fmt.Sprintf("%s ist abwesend: %s entfallen und der Tag wird nicht gezählt.", deDay(day), strings.Join(de, " und ")))
	}
	for _, d := range r.Days {
		if strings.HasPrefix(d.Status, "away") && gone[d.Day] == nil {
			s.add(classChanged, kindAway, statusOK, d.Day+" is away: the day is not counted.", deDay(d.Day)+" ist abwesend: der Tag wird nicht gezählt.")
		}
	}
	guestFacts(in)
	keepFacts(in, r.Changed)
	oddNameFacts(in, r.Changed)
	proteinFacts(in, r)
	meatLeaves(in, r)
	targetMoved(in, r)
	dayFacts(in, r)
	cookFacts(in, r)
	for _, c := range r.Cooking {
		if len(c.Cooked) == 0 && !awayDays[c.Day] {
			if o := out[c.Day]; o != nil {
				o.nothing = true
				continue
			}
			s.add(classMoved, kindCook, statusOK, c.Day+": nothing is cooked.", deDay(c.Day)+": es wird nichts gekocht.")
		}
	}
	for _, d := range in.after.Days {
		if o := out[dayLabelStr(d.Date)]; o != nil {
			o.tell(s)
		}
	}
}

// dayOut is what a day with meals out says in one line (G4): the meals out, the
// energy they are assumed to hold, and that nothing is cooked.
type dayOut struct {
	day     string
	meals   []string
	kcal    float64
	nothing bool
}

// dayOuts is the days with meals out and an assumed energy, by day label. A day
// whose meals out have no assumed energy is not in it: its meals keep their lines.
func dayOuts(in reportInput, r changeReport, awayDays map[string]bool) map[string]*dayOut {
	goneMeals := map[string][]string{}
	for _, m := range r.Changed {
		if len(m.After) == 0 && !awayDays[m.Day] {
			goneMeals[m.Day] = append(goneMeals[m.Day], m.Meal)
		}
	}
	out := map[string]*dayOut{}
	for _, d := range in.after.Days {
		day := dayLabelStr(d.Date)
		if len(goneMeals[day]) == 0 || d.AwayKcal <= 0 {
			continue
		}
		out[day] = &dayOut{day: day, meals: goneMeals[day], kcal: d.AwayKcal}
	}
	return out
}

// tell adds the day's one line.
func (o *dayOut) tell(s *factSink) {
	var de []string
	for _, m := range o.meals {
		de = append(de, deSlot(m))
	}
	en := fmt.Sprintf("%s: %s out (about %.0f kcal assumed)", o.day, strings.Join(o.meals, " and "), o.kcal)
	g := fmt.Sprintf("%s: %s außer Haus (etwa %.0f kcal angenommen)", deDay(o.day), deJoin(de), o.kcal)
	if o.nothing {
		en += ", nothing is cooked"
		g += ", es wird nichts gekocht"
	}
	s.add(classMoved, kindAway, statusOK, dot(en), dot(g))
}

// deJoin lists German meal names: "Mittag- und Abendessen" for two "-essen" meals.
func deJoin(names []string) string {
	n := len(names)
	if n == 1 {
		return names[0]
	}
	head := slices.Clone(names[:n-1])
	if last := names[n-1]; strings.HasSuffix(last, "essen") && strings.HasSuffix(head[n-2], "essen") {
		head[n-2] = strings.TrimSuffix(head[n-2], "essen") + "-"
	}
	return strings.Join(head, ", ") + " und " + names[n-1]
}

// proteinFacts says where a day's protein stands when this change moved it by 5 g or more.
func proteinFacts(in reportInput, r changeReport) {
	for _, a := range in.after.Days {
		for _, b := range in.before.Days {
			if b.Date != a.Date || absf(a.ProteinG-b.ProteinG) < 5 || !touchedDate(r, a.Date) {
				continue
			}
			day := dayLabelStr(a.Date)
			switch {
			case a.ProteinTargetG > 0 && b.ProteinG >= a.ProteinTargetG && a.ProteinG >= b.ProteinG:
				in.facts.add(classChanged, kindProtein, statusOK,
					fmt.Sprintf("%s already held %.0f g of protein, above the %.0f g it needs; now %.0f g.", day, b.ProteinG, a.ProteinTargetG, a.ProteinG),
					fmt.Sprintf("%s hatte schon %.0f g Eiweiß, über den nötigen %.0f g; jetzt %.0f g.", deDay(day), b.ProteinG, a.ProteinTargetG, a.ProteinG))
			case a.ProteinTargetG > 0:
				st := statusOK
				if a.ProteinG < a.ProteinTargetG {
					st = statusUnder
				}
				in.facts.addKey("protein:"+day, classChanged, kindProtein, st,
					fmt.Sprintf("%s: protein %.0f g → %.0f g (it needs %.0f g).", day, b.ProteinG, a.ProteinG, a.ProteinTargetG),
					fmt.Sprintf("%s: Eiweiß %.0f g → %.0f g (nötig sind %.0f g).", deDay(day), b.ProteinG, a.ProteinG, a.ProteinTargetG))
			default:
				in.facts.add(classChanged, kindProtein, statusUnknown,
					fmt.Sprintf("%s: protein %.0f g → %.0f g.", day, b.ProteinG, a.ProteinG),
					fmt.Sprintf("%s: Eiweiß %.0f g → %.0f g.", deDay(day), b.ProteinG, a.ProteinG))
			}
		}
	}
}

// meatLeaves says why a dish was swapped: the meat that left the day with it.
func meatLeaves(in reportInput, r changeReport) {
	bd, ad := map[string][]string{}, map[string][]string{}
	for _, d := range in.before.Days {
		bd[d.Date] = d.MeatFoods
	}
	for _, d := range in.after.Days {
		ad[d.Date] = d.MeatFoods
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
		for _, f := range bd[date] {
			if !slices.Contains(ad[date], f) {
				gone = append(gone, f)
			}
		}
		if len(gone) > 0 {
			en, de := make([]string, len(gone)), make([]string, len(gone))
			for i, g := range gone {
				en[i], de[i] = food.Quote(g), deQuote(g)
			}
			in.facts.add(classMoved, kindMeat, statusOK,
				fmt.Sprintf("Meat that leaves %s with this change: %s.", m.Day, strings.Join(en, ", ")),
				fmt.Sprintf("Fleisch, das %s mit dieser Änderung wegfällt: %s.", deDay(m.Day), strings.Join(de, ", ")))
		}
	}
}

// targetMoved says that a day's goal moved with a light or filling mark.
func targetMoved(in reportInput, r changeReport) {
	for _, d := range in.after.Days {
		if d.TargetKcal == nil {
			continue
		}
		var before *float64
		var beforeMarks []string
		known := false
		for _, x := range in.before.Days {
			if x.Date == d.Date {
				before, beforeMarks, known = x.TargetKcal, x.LightMeals, true
			}
		}
		if known && before != nil && absf(*d.TargetKcal-*before) >= 5 {
			day := dayLabelStr(d.Date)
			en, de := markChange(beforeMarks, d.LightMeals, langEN), markChange(beforeMarks, d.LightMeals, langDE)
			in.facts.add(classChanged, kindDay, statusOK,
				fmt.Sprintf("%s: the amount that suits the day is now %.0f kcal (was %.0f) because %s.", day, *d.TargetKcal, *before, en),
				fmt.Sprintf("%s: die passende Menge für den Tag liegt jetzt bei %.0f kcal (vorher %.0f), weil %s.", deDay(day), *d.TargetKcal, *before, de))
		}
	}
}

// dayFacts says the days against their range. A day outside its range or under
// its minimum is a fact that got worse; a day inside it is told only for a
// lighter, heartier, light or filling change (the day's kcal are part of it).
func dayFacts(in reportInput, r changeReport) {
	for _, d := range r.Days {
		if strings.HasPrefix(d.Status, "away") {
			continue
		}
		en := fmt.Sprintf("%s holds %d kcal (was %d)", d.Day, d.KcalAfter, d.KcalBefore)
		de := fmt.Sprintf("%s: %d kcal (vorher %d)", deDay(d.Day), d.KcalAfter, d.KcalBefore)
		status, class := statusOK, classChanged
		if d.Range != "" && d.Status != "" && !in.undo {
			side := strings.Fields(suitsPhrase(d.Status))[0]
			en += fmt.Sprintf(", %s (%s)", suitsPhrase(d.Status), d.Range)
			switch side {
			case "below":
				status, class = statusUnder, classWorse
				de += fmt.Sprintf(", unter dem passenden Bereich für den Tag (%s)", d.Range)
			case "above":
				status, class = statusOver, classWorse
				de += fmt.Sprintf(", über dem passenden Bereich für den Tag (%s)", d.Range)
			default:
				de += fmt.Sprintf(", im passenden Bereich für den Tag (%s)", d.Range)
			}
			if d.MinimumKcal > 0 {
				status, class = statusUnder, classWorse
				en += fmt.Sprintf(" and under the day's minimum of %d kcal", d.MinimumKcal)
				de += fmt.Sprintf(" und unter dem Mindestwert von %d kcal", d.MinimumKcal)
			}
		}
		// A day told as worse must be worse: further from its range than before;
		// a day the change left under its minimum is always told (G4.4, SF1(c)).
		if class == classWorse && !dayWorse(d) && !in.dayKcal && d.MinimumKcal == 0 {
			continue
		}
		if class == classChanged && !in.dayKcal && !in.undo {
			continue
		}
		if d.MinimumKcal > 0 {
			in.facts.addMust("day:"+d.Day, class, kindDay, status, dot(en), dot(de)) // SF1(c): always told
		} else {
			in.facts.addKey("day:"+d.Day, class, kindDay, status, dot(en), dot(de))
		}
	}
}

// dayWorse reports a day that is further from its range after the change than
// before it, or that the change took out of its range.
func dayWorse(d reportDay) bool {
	lo, hi, ok := strings.Cut(strings.TrimSuffix(d.Range, " kcal"), "-")
	if !ok {
		return false
	}
	l, h := float64(atoi(lo)), float64(atoi(hi))
	dist := func(k float64) float64 {
		switch {
		case k < l:
			return l - k
		case k > h:
			return k - h
		}
		return 0
	}
	return dist(float64(d.KcalAfter)) > dist(float64(d.KcalBefore))+0.5
}

// qtyDeltaL says how much a quantity changed: " (half a portion more)".
func qtyDeltaL(from, to string, lg lang) string {
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
	de := lg == langDE
	dir := "more"
	if de {
		dir = "mehr"
	}
	if d < 0 {
		dir, d = "less", -d
		if de {
			dir = "weniger"
		}
	}
	amount := fmtNum(d) + " g"
	if de {
		amount = fmtComma(d) + " g"
	}
	if strings.HasPrefix(ua, "portion") {
		switch {
		case d == 0.25 && de:
			amount = "eine Viertelportion"
		case d == 0.25:
			amount = "a quarter portion"
		case d == 0.5 && de:
			amount = "eine halbe Portion"
		case d == 0.5:
			amount = "half a portion"
		case d == 0.75 && de:
			amount = "drei Viertel einer Portion"
		case d == 0.75:
			amount = "three quarters of a portion"
		case d == 1 && de:
			amount = "eine Portion"
		case d == 1:
			amount = "one portion"
		case de:
			amount = fmtComma(d) + " Portionen"
		default:
			amount = fmtNum(d) + " portions"
		}
	}
	if de {
		return " (" + amount + " " + dir + ")"
	}
	return " (" + amount + " " + dir + ")"
}

// markChange names why a day's target moved: the marks the change gained or
// lost ("the lunch is no longer light"), or the marks it ends with when the
// change did not touch them.
func markChange(before, after []string, lg lang) string {
	de := lg == langDE
	phrase := func(lm string, lost bool) string {
		adj, slotName, ok := strings.Cut(plainWords(lm), " ")
		if !ok {
			return ""
		}
		if de {
			adj = map[string]string{"light": "leicht", "filling": "sättigend"}[adj]
			if adj == "" {
				return ""
			}
			if lost {
				return fmt.Sprintf("das %s nicht mehr %s ist", deSlot(slotName), adj)
			}
			return fmt.Sprintf("das %s %s ist", deSlot(slotName), adj)
		}
		if lost {
			return fmt.Sprintf("the %s is no longer %s", slotName, adj)
		}
		return fmt.Sprintf("the %s is %s", slotName, adj)
	}
	var parts []string
	for _, lm := range after {
		if !slices.Contains(before, lm) {
			parts = append(parts, phrase(lm, false))
		}
	}
	for _, lm := range before {
		if !slices.Contains(after, lm) {
			parts = append(parts, phrase(lm, true))
		}
	}
	if len(parts) == 0 {
		for _, lm := range after {
			parts = append(parts, phrase(lm, false))
		}
	}
	parts = slices.DeleteFunc(parts, func(p string) bool { return p == "" })
	if len(parts) == 0 {
		if de {
			return "eine Mahlzeit leicht oder sättigend ist"
		}
		return "a meal is light or filling"
	}
	if de {
		return strings.Join(parts, " und ")
	}
	return strings.Join(parts, " and ")
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
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
	if i := strings.Index(status, ", under the day's minimum"); i >= 0 {
		status = status[:i]
	}
	return status
}

// A cooking time of the plan: what is cooked on one cell, which meals eat it
// and how long the longest recipe takes (the recipe's total minutes).
type cookDish struct {
	name  string
	feeds []string
}

type cookCell struct {
	date    string
	slot    int64
	dishes  []cookDish
	maxMins int
}

// cookCells is, per cook cell (iso date + slot), the dishes cooked there for a
// meal; a breakfast batch is no cooking. Feeds are the meals eating each dish,
// in cell order.
func cookCells(st planning.State) map[string]*cookCell {
	feeds := map[int64][]string{}
	plates := slices.Clone(st.Plates)
	slices.SortFunc(plates, func(a, b planning.Plate) int {
		if c := a.Date.Compare(b.Date); c != 0 {
			return c
		}
		return int(st.SlotOrder[a.SlotID]) - int(st.SlotOrder[b.SlotID])
	})
	for _, p := range plates {
		for _, c := range p.Components {
			if c.SessionID == 0 {
				continue
			}
			label := cellLabel(st, p.Date, p.SlotID)
			if !slices.Contains(feeds[c.SessionID], label) {
				feeds[c.SessionID] = append(feeds[c.SessionID], label)
			}
		}
	}
	out := map[string]*cookCell{}
	sessions := slices.Clone(st.Sessions)
	slices.SortFunc(sessions, func(a, b planning.Session) int { return int(a.ID - b.ID) })
	for _, s := range sessions {
		if len(feeds[s.ID]) == 0 || st.SlotKind[s.CookSlotID] == slot.KindLight {
			continue
		}
		k := fmt.Sprintf("%s|%d", s.CookDate.Format("2006-01-02"), s.CookSlotID)
		c := out[k]
		if c == nil {
			c = &cookCell{date: s.CookDate.Format("2006-01-02"), slot: s.CookSlotID}
			out[k] = c
		}
		c.dishes = append(c.dishes, cookDish{name: foodName(st, s.FoodID), feeds: feeds[s.ID]})
		c.maxMins = max(c.maxMins, st.Foods[s.FoodID].Minutes)
	}
	return out
}

// signature is the dishes cooked there: which meals eat them is not where the
// cooking is.
func (c *cookCell) signature() string {
	if c == nil {
		return ""
	}
	parts := make([]string, len(c.dishes))
	for i, d := range c.dishes {
		parts[i] = d.name
	}
	slices.Sort(parts)
	return strings.Join(parts, ";")
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

// feedsList names the meals a cooking feeds, at most five.
func feedsList(feeds []string, de bool) string {
	show := feeds
	more := 0
	if len(show) > 5 {
		show, more = feeds[:5], len(feeds)-5
	}
	out := make([]string, len(show))
	for i, f := range show {
		out[i] = f
		if de {
			out[i] = deCell(f)
		}
	}
	s := strings.Join(out, ", ")
	switch {
	case more > 0 && de:
		s += fmt.Sprintf(" und %d weitere", more)
	case more > 0:
		s += fmt.Sprintf(" and %d more", more)
	}
	return s
}

// cookFacts is one fact per cooking time that changed: the cook cell of the
// saved plan whose dishes or meals differ from before, with its dishes, the day
// and meal, the meals they feed, where a dish was cooked before, and the
// longest recipe's total minutes. A dish whose cooking moved says where it was.
func cookFacts(in reportInput, r changeReport) {
	before, after := cookCells(in.beforeSt), cookCells(in.afterSt)
	// A dish that was cooked at a cell and no longer is moved from there.
	wasAt := map[string]string{}
	for k, c := range before {
		for _, d := range c.dishes {
			if now := after[k]; now != nil && slices.ContainsFunc(now.dishes, func(x cookDish) bool { return x.name == d.name }) {
				continue
			}
			label := dayLabelStr(c.date) + " " + slotWord(in.beforeSt, c.slot)
			if cur, ok := wasAt[d.name]; !ok || label < cur {
				wasAt[d.name] = label
			}
		}
	}
	keys := make([]string, 0, len(after))
	for k := range after {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		a := after[k]
		if a.signature() == before[k].signature() || !in.afterSt.Window.Contains(mustDate(a.date)) {
			continue
		}
		day, meal := dayLabelStr(a.date), slotWord(in.afterSt, a.slot)
		// One line per cooking time: each dish with the meals it feeds and, for a
		// moved dish, where it was cooked before; then the longest recipe's minutes.
		var en, de []string
		for _, d := range a.dishes {
			e := fmt.Sprintf("%s for %s", food.Quote(d.name), feedsList(d.feeds, false))
			g := fmt.Sprintf("%s für %s", deQuote(d.name), feedsList(d.feeds, true))
			if was, ok := wasAt[d.name]; ok && was != day+" "+meal {
				e += fmt.Sprintf(" (cooked %s before)", was)
				g += fmt.Sprintf(" (vorher %s gekocht)", deCell(was))
			}
			en, de = append(en, e), append(de, g)
		}
		enLine := fmt.Sprintf("%s at %s you cook %s", day, meal, strings.Join(en, "; "))
		deLine := fmt.Sprintf("%s (%s) kochst du %s", deDay(day), deSlot(meal), strings.Join(de, "; "))
		if a.maxMins > 0 {
			enLine += fmt.Sprintf("; the longest recipe takes about %s", duration(a.maxMins))
			deLine += fmt.Sprintf("; das längste Rezept braucht etwa %s", deDuration(a.maxMins))
		}
		in.facts.add(classMoved, kindCook, statusOK, dot(enLine), dot(deLine))
	}
}

// guestFacts says, for a plate that gained or lost people, how many portions of
// its main are cooked for them; on a light meal of its day it is the small one.
func guestFacts(in reportInput) {
	was := map[int64]float64{}
	for _, p := range in.beforeSt.Plates {
		was[p.ID] = p.Eaters
	}
	kcal := map[int64]float64{}
	for _, p := range in.after.Plates {
		kcal[p.PlateID] = p.Kcal
	}
	for _, p := range in.afterSt.Plates {
		b, ok := was[p.ID]
		if p.ID <= 0 || !ok || p.Eaters == b {
			continue
		}
		meal := slotWord(in.afterSt, p.SlotID)
		day := dayLabel(p.Date)
		var dishEN, dishDE string
		for _, c := range p.Components {
			if c.Portions != nil && c.SessionID != 0 {
				n := *c.Portions * p.Eaters
				dishEN = fmt.Sprintf(", %s of %s", portionWord(n), food.Quote(foodName(in.afterSt, c.FoodID)))
				dishDE = fmt.Sprintf(", %s von %s", dePortions(n), deQuote(foodName(in.afterSt, c.FoodID)))
				break
			}
		}
		peopleEN, peopleDE := peopleWords(p.Eaters)
		en := fmt.Sprintf("%s %s: now for %s%s.", day, meal, peopleEN, dishEN)
		de := fmt.Sprintf("%s %s: jetzt für %s%s.", deDay(day), deSlot(meal), peopleDE, dishDE)
		if k, has := kcal[p.ID]; has && p.Eaters > b {
			iso := p.Date.Format("2006-01-02")
			for _, d := range in.after.Days {
				if d.Date != iso {
					continue
				}
				for _, lm := range d.LightMeals {
					if plainWords(lm) == "light "+meal {
						en = fmt.Sprintf("%s %s: now for %s%s; it is the small %s (%.0f kcal each).", day, meal, peopleEN, dishEN, meal, k)
						de = fmt.Sprintf("%s %s: jetzt für %s%s; es ist das kleine %s (%.0f kcal pro Person).", deDay(day), deSlot(meal), peopleDE, dishDE, deSlot(meal), k)
					}
				}
			}
		}
		in.facts.add(classMoved, kindGuest, statusOK, en, de)
	}
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// peopleWords says how many people eat a plate: "1 person", "2 people".
func peopleWords(n float64) (en, de string) {
	if n > 0.995 && n < 1.005 {
		return "1 person", "1 Person"
	}
	return fmtNum(n) + " people", fmtComma(n) + " Personen"
}
