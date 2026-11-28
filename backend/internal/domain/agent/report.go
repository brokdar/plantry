package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/slot"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// changeReport is the facts of a change, computed from the plan states before
// and after it, in kitchen language: no codes, no ids, no ISO dates. The
// model's reply says these facts in its own words and states nothing about
// the plan that the report does not hold.
type changeReport struct {
	Changed []reportMeal `json:"changed,omitempty"`
	Days    []reportDay  `json:"days,omitempty"`
	Cooking []reportCook `json:"cooking,omitempty"`
	// EarlierWarnings counts the warnings that existed before and are
	// unaffected: do not list them.
	EarlierWarnings int `json:"earlier_warnings_unaffected"`
	// Facts is the facts block of the change: at most five lines of classes
	// 2 to 5 in class order (see selectBlock), each in English and German. The
	// code puts it into the reply (see composeReply); the model does not read it.
	Facts []fact `json:"facts,omitempty"`
	// Tell is the English lines of Facts, what the model frames its sentence on.
	Tell []string `json:"tell,omitempty"`
	// Offer is the one offer of the reply, rendered by code, or nil.
	Offer *offerLine `json:"offer,omitempty"`
	// Check is the touched days of the plan before and after, for checking the
	// facts against the stored plan; the model does not read it.
	Check *reportCheck `json:"check,omitempty"`
	// Note is set by an undo: the plan is back as it was before the change.
	Note string `json:"note,omitempty"`
	// NotCovered says what the report could not compute.
	NotCovered []string `json:"not_covered,omitempty"`
}

type reportMeal struct {
	Day       string   `json:"day"`
	Meal      string   `json:"meal"`
	MovedFrom string   `json:"moved_from,omitempty"`
	Before    []string `json:"before,omitempty"`
	After     []string `json:"after,omitempty"`
	// Kind is "cold plate, nothing to cook" for a meal of no-cook foods.
	Kind     string `json:"kind,omitempty"`
	KcalFrom *int   `json:"kcal_before,omitempty"`
	KcalTo   *int   `json:"kcal_after,omitempty"`
}

type reportDay struct {
	Day         string `json:"day"`
	KcalBefore  int    `json:"kcal_before"`
	KcalAfter   int    `json:"kcal_after"`
	Range       string `json:"healthy_range,omitempty"`
	Status      string `json:"status,omitempty"`
	MinimumKcal int    `json:"minimum_kcal,omitempty"`
}

type reportCook struct {
	Day          string       `json:"day"`
	CookingTimes int          `json:"cooking_times"`
	Cooked       []reportDish `json:"cooked,omitempty"`
}

type reportDish struct {
	At    string   `json:"at"`
	Dish  string   `json:"dish"`
	Eaten []string `json:"eaten"`
}

// reportInput is what a change leaves: the states and evaluations before and
// after, and the applied entries (for the knock-on changes).
type reportInput struct {
	beforeSt, afterSt planning.State
	before, after     planning.Evaluation
	applied           []map[string]any
	// facts collects the facts of the block while the report is built; nil drops them.
	facts *factSink
	// dayKcal is set for a lighter, heartier, light or filling change, which
	// tells the day's kcal also when the day is inside its range.
	dayKcal bool
	// undo is set for an undo: no warning is new or worse.
	undo bool
	// light is the call's own light marks: saved with the write, so the plan after
	// holds them (the state read inside the call has the stored ones only).
	light []planning.LightIntent
}

func dayLabel(d time.Time) string { return d.Format("Mon 2 Jan") }

func dayLabelStr(s string) string {
	if d, err := time.Parse(time.DateOnly, s); err == nil {
		return dayLabel(d)
	}
	return s
}

func fmtNum(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

func slotWord(st planning.State, id int64) string {
	n := strings.TrimPrefix(st.SlotNames[id], "slot.")
	if n == "" {
		return "meal"
	}
	return n
}

func cellLabel(st planning.State, d time.Time, slot int64) string {
	return dayLabel(d) + " " + slotWord(st, slot)
}

func foodName(st planning.State, id int64) string {
	if f, ok := st.Foods[id]; ok && f.Name != "" {
		return f.Name
	}
	return "a food"
}

func componentWords(st planning.State, p planning.Plate) []string {
	out := make([]string, 0, len(p.Components))
	for _, c := range p.Components {
		s := food.Data(foodName(st, c.FoodID))
		switch {
		case c.Portions != nil:
			s += " " + fmtNum(*c.Portions) + " portion"
			if *c.Portions != 1 {
				s += "s"
			}
		case c.Grams != nil:
			s += " " + fmtNum(*c.Grams) + " g"
		}
		out = append(out, s)
	}
	return out
}

// buildReport computes the report. A fact that cannot be computed is listed
// in NotCovered instead of being guessed.
func buildReport(in reportInput) changeReport {
	var r changeReport
	bs, as := in.beforeSt, in.afterSt
	bp, ap := map[int64]planning.Plate{}, map[int64]planning.Plate{}
	for _, p := range bs.Plates {
		bp[p.ID] = p
	}
	for _, p := range as.Plates {
		ap[p.ID] = p
	}
	be, ae := map[int64]planning.PlateEval{}, map[int64]planning.PlateEval{}
	for _, e := range in.before.Plates {
		be[e.PlateID] = e
	}
	for _, e := range in.after.Plates {
		ae[e.PlateID] = e
	}
	ids := map[int64]bool{}
	for id := range bp {
		ids[id] = true
	}
	for id := range ap {
		ids[id] = true
	}
	dates := map[string]bool{}
	type changedPlate struct {
		meal reportMeal
		date time.Time
		slot int64
	}
	var changed []changedPlate
	for id := range ids {
		b, hadB := bp[id]
		a, hasA := ap[id]
		if hadB && hasA && b.Cell() == a.Cell() && slices.Equal(componentWords(bs, b), componentWords(as, a)) {
			continue
		}
		m := reportMeal{}
		cur, st := a, as
		if !hasA {
			cur, st = b, bs
		}
		m.Day, m.Meal = dayLabel(cur.Date), slotWord(st, cur.SlotID)
		if hadB && hasA && b.Cell() != a.Cell() {
			m.MovedFrom = cellLabel(bs, b.Date, b.SlotID)
		}
		if hadB {
			m.Before = componentWords(bs, b)
			if e, ok := be[id]; ok {
				k := int(math.Round(e.Kcal))
				m.KcalFrom = &k
			}
		}
		if hasA {
			m.After = componentWords(as, a)
			if as.IsColdPlate(a) {
				m.Kind = "cold plate, nothing to cook"
			}
			if e, ok := ae[id]; ok {
				k := int(math.Round(e.Kcal))
				m.KcalTo = &k
			}
		}
		dates[cur.Date.Format(time.DateOnly)] = true
		if hadB {
			dates[b.Date.Format(time.DateOnly)] = true
		}
		changed = append(changed, changedPlate{m, cur.Date, cur.SlotID})
	}
	slices.SortFunc(changed, func(x, y changedPlate) int {
		if c := x.date.Compare(y.date); c != 0 {
			return c
		}
		return int(as.SlotOrder[x.slot]) - int(as.SlotOrder[y.slot])
	})
	for _, c := range changed {
		r.Changed = append(r.Changed, c.meal)
	}
	r.Changed = pairByCell(r.Changed)

	bd, ad := map[string]planning.DayEval{}, map[string]planning.DayEval{}
	for _, d := range in.before.Days {
		bd[d.Date] = d
	}
	for _, d := range in.after.Days {
		ad[d.Date] = d
	}
	for _, date := range sortedKeys(dates) {
		a, ok := ad[date]
		if !ok {
			continue
		}
		day := reportDay{Day: dayLabelStr(date), KcalBefore: int(math.Round(bd[date].Kcal)), KcalAfter: int(math.Round(a.Kcal))}
		if a.BandHigh > 0 {
			day.Range = fmt.Sprintf("%.0f-%.0f kcal", a.BandLow, a.BandHigh)
			switch {
			case a.Kcal < a.BandLow-0.5:
				day.Status = "under its range"
			case a.Kcal > a.BandHigh+0.5:
				day.Status = "over its range"
			default:
				day.Status = "inside its range"
			}
			if a.Kcal < a.FloorKcal {
				day.MinimumKcal = int(math.Round(a.FloorKcal))
				day.Status += ", under the day's minimum"
			}
		}
		r.Days = append(r.Days, day)
	}

	cookDates := map[string]bool{}
	for d := range dates {
		cookDates[d] = true
	}
	bc, ac := cookingByDay(bs), cookingByDay(as)
	for d := range bc {
		if !sameCooking(bc[d], ac[d]) {
			cookDates[d] = true
		}
	}
	for d := range ac {
		if !sameCooking(bc[d], ac[d]) {
			cookDates[d] = true
		}
	}
	for _, d := range sortedKeys(cookDates) {
		if !as.Window.Contains(mustDate(d)) {
			continue
		}
		rc := reportCook{Day: dayLabelStr(d), Cooked: ac[d]}
		if day, ok := ad[d]; ok {
			rc.CookingTimes = day.CookOccasions
		} else {
			seen := map[string]bool{}
			for _, c := range ac[d] {
				seen[c.At] = true
			}
			rc.CookingTimes = len(seen)
		}
		r.Cooking = append(r.Cooking, rc)
	}

	in.facts = &factSink{}
	leftovers(bs, as, in.facts, r.Changed)
	alsoChanged(as, in.applied, in.facts)
	weekFacts(in, bs, as)
	if !in.undo {
		r.EarlierWarnings = warnings(in, as, awayOnly(r.Changed), r.Days)
	}
	buildFacts(in, r)
	r.Facts = selectBlock(in.facts.items)
	r.Tell = tellOf(r.Facts)
	touched := map[string]bool{}
	for d := range dates {
		touched[d] = true
	}
	for d := range cookDates {
		touched[d] = true
	}
	for _, d := range changedDays(in) {
		touched[d] = true
	}
	r.Check = checkOf(in, sortedKeys(touched))
	return r
}

func mustDate(s string) time.Time {
	d, _ := time.Parse(time.DateOnly, s)
	return d
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// cookingByDay is what each day cooks: per cook cell the dish and every meal
// that eats it, from the state's batches and the components eating them.
func cookingByDay(st planning.State) map[string][]reportDish {
	eaters := map[int64][]plate.CellKey{}
	for _, p := range st.Plates {
		for _, c := range p.Components {
			if c.SessionID != 0 {
				eaters[c.SessionID] = append(eaters[c.SessionID], p.Cell())
			}
		}
	}
	out := map[string][]reportDish{}
	for _, s := range st.Sessions {
		cells := eaters[s.ID]
		if len(cells) == 0 || st.SlotKind[s.CookSlotID] == slot.KindLight {
			continue // a breakfast batch (oats) is not a cooking time
		}
		slices.SortFunc(cells, func(a, b plate.CellKey) int {
			if c := a.Date.Compare(b.Date); c != 0 {
				return c
			}
			return int(st.SlotOrder[a.SlotID]) - int(st.SlotOrder[b.SlotID])
		})
		cells = slices.Compact(cells)
		d := reportDish{At: slotWord(st, s.CookSlotID), Dish: foodName(st, s.FoodID)}
		for _, c := range cells {
			d.Eaten = append(d.Eaten, cellLabel(st, c.Date, c.SlotID))
		}
		key := s.CookDate.Format(time.DateOnly)
		out[key] = append(out[key], d)
	}
	for k := range out {
		slices.SortFunc(out[k], func(a, b reportDish) int { return strings.Compare(a.At+a.Dish, b.At+b.Dish) })
	}
	return out
}

func sameCooking(a, b []reportDish) bool {
	return slices.EqualFunc(a, b, func(x, y reportDish) bool {
		return x.At == y.At && x.Dish == y.Dish && slices.Equal(x.Eaten, y.Eaten)
	})
}

type batchFacts struct {
	portions, eaten, frozen float64
	links                   int
	dish                    string
	cooked                  time.Time
	cookedAt                string
}

// batchKey identifies a cooking by its dish and cook cell, not by its id: a
// regenerated or restored plan gives the same cooking a new id.
func batchKey(st planning.State, s planning.Session) string {
	return fmt.Sprintf("%s|%s|%d", foodName(st, s.FoodID), s.CookDate.Format(time.DateOnly), s.CookSlotID)
}

func batchesOf(st planning.State) map[string]batchFacts {
	out := map[string]batchFacts{}
	keyOf := map[int64]string{}
	for _, s := range st.Sessions {
		k := batchKey(st, s)
		keyOf[s.ID] = k
		b := out[k]
		b.portions += s.Portions
		b.frozen += s.FrozenPortions
		b.dish, b.cooked, b.cookedAt = foodName(st, s.FoodID), s.CookDate, slotWord(st, s.CookSlotID)
		out[k] = b
	}
	for _, p := range st.Plates {
		for _, c := range p.Components {
			if k, ok := keyOf[c.SessionID]; ok && c.Portions != nil {
				b := out[k]
				b.eaten += p.Eaters * *c.Portions
				b.links++
				out[k] = b
			}
		}
	}
	return out
}

func portionWord(v float64) string {
	if math.Abs(v-1) < 0.005 {
		return "1 portion"
	}
	return fmtNum(v) + " portions"
}

// leftovers says what the change did to a cooking that exists in the saved
// plan: cooked portions nobody eats, meals beyond the portions, a resize. A
// cooking the saved plan no longer has is said as "no longer cooked", unless a
// changed meal explains it (its dish left that meal) or the dish is cooked
// elsewhere now (the cooking-time fact says where).
func leftovers(bs, as planning.State, sink *factSink, changed []reportMeal) {
	b, a := batchesOf(bs), batchesOf(as)
	keys := make([]string, 0, len(a)+len(b))
	for k := range a {
		keys = append(keys, k)
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	cookedNow := map[string]bool{}
	for _, v := range a {
		cookedNow[v.dish] = true
	}
	explained := func(dish string) bool {
		for _, m := range changed {
			if slices.ContainsFunc(m.Before, func(s string) bool { n, _ := splitQty(s); return n == dish }) {
				return true
			}
		}
		return false
	}
	for _, k := range keys {
		was, wasOK := b[k]
		now, nowOK := a[k]
		switch {
		case wasOK && !nowOK:
			if was.eaten > 0.005 && !cookedNow[was.dish] && !explained(was.dish) {
				day := was.cooked.Format("Mon 2 Jan")
				sink.add(classMoved, kindLeftover, statusOK,
					fmt.Sprintf("The %s cooked on %s is no longer cooked.", food.Quote(was.dish), day),
					fmt.Sprintf("%s, gekocht am %s, wird nicht mehr gekocht.", deQuote(was.dish), deDay(day)))
			}
		case !wasOK:
		default:
			if math.Abs(was.eaten-now.eaten) < 0.005 && math.Abs(was.portions-now.portions) < 0.005 && was.links == now.links {
				continue
			}
			free := now.portions - now.eaten
			frozen, frozenDE := "", ""
			if now.frozen > 0.005 {
				frozen = fmt.Sprintf(" (%s are meant for the freezer)", portionWord(now.frozen))
				frozenDE = fmt.Sprintf(" (%s sind für den Gefrierschrank gedacht)", dePortions(now.frozen))
			}
			resized, resizedDE := "It was not resized.", "Die Menge wurde nicht geändert."
			if math.Abs(was.portions-now.portions) >= 0.005 {
				resized = fmt.Sprintf("It was resized from %s to %s.", portionWord(was.portions), portionWord(now.portions))
				resizedDE = fmt.Sprintf("Die Menge wurde von %s auf %s geändert.", dePortions(was.portions), dePortions(now.portions))
			}
			day := now.cooked.Format("Mon 2 Jan")
			switch {
			case math.Abs(free) <= 0.005 && math.Abs(was.portions-now.portions) >= 0.005:
				sink.addDay(day, classMoved, kindLeftover, statusOK,
					fmt.Sprintf("%s: cook %s of the %s (was %s).", day, portionWord(now.portions), food.Quote(now.dish), fmtNum(was.portions)),
					fmt.Sprintf("%s: %s von %s kochen (statt %s).", deDay(day), dePortions(now.portions), deQuote(now.dish), fmtComma(was.portions)))
			case free > 0.005:
				sink.add(classMoved, kindLeftover, statusOK,
					fmt.Sprintf("%s of the %s cooked on %s is now left over (it is cooked for %s%s, the meals eat %s). %s", portionWord(free), food.Quote(now.dish), day, portionWord(now.portions), frozen, fmtNum(now.eaten), resized),
					fmt.Sprintf("Von %s, gekocht am %s, bleiben %s übrig (gekocht für %s%s, die Mahlzeiten essen %s). %s", deQuote(now.dish), deDay(day), dePortions(free), dePortions(now.portions), frozenDE, fmtComma(now.eaten), resizedDE))
			case free < -0.005:
				sink.add(classMoved, kindLeftover, statusUnder,
					fmt.Sprintf("The meals need %s more of the %s cooked on %s than it holds (%s). %s", portionWord(-free), food.Quote(now.dish), day, portionWord(now.portions), resized),
					fmt.Sprintf("Die Mahlzeiten brauchen %s mehr von %s, gekocht am %s, als es enthält (%s). %s", dePortions(-free), deQuote(now.dish), deDay(day), dePortions(now.portions), resizedDE))
			}
		}
	}
}

func asInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	}
	return 0
}

func asDate(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, false
	}
	d, err := time.Parse(time.DateOnly, s)
	return d, err == nil
}

func sessionDish(st planning.State, id int64) string {
	for _, s := range st.Sessions {
		if s.ID == id {
			return food.Quote(foodName(st, s.FoodID))
		}
	}
	return ""
}

// alsoChanged says a meal that now cooks its own dish instead of eating the
// batch cooked earlier. (A cook that moved is told by its cooking-time fact.)
func alsoChanged(st planning.State, applied []map[string]any, sink *factSink) {
	for _, e := range applied {
		list, ok := e["also_changed"].([]any)
		if !ok {
			continue
		}
		for _, x := range list {
			m, ok := x.(map[string]any)
			if !ok || m["what"] != "cooks_fresh" {
				continue
			}
			dish := sessionDish(st, asInt64(m["session_id"]))
			if dish == "" {
				dish = "a dish"
			}
			cell := ""
			if d, ok := asDate(m["date"]); ok {
				cell = cellLabel(st, d, asInt64(m["slot_id"]))
			}
			why := fmt.Sprint(m["why"])
			en := fmt.Sprintf("%s now cooks its own %s instead of eating the one cooked earlier", cell, dish)
			de := fmt.Sprintf("%s kocht jetzt selbst %s, statt das früher Gekochte zu essen", deCell(cell), deQuote(dish))
			reasonEN := map[string]string{"before_cook": "it comes before the batch is cooked", "past_keep_days": "the batch would be too old by then", "cooked": "the batch is already cooked"}[why]
			reasonDE := map[string]string{"before_cook": "es kommt vor dem Kochen", "past_keep_days": "es wäre dann zu alt", "cooked": "es ist schon gekocht"}[why]
			if reasonEN != "" {
				if why == "past_keep_days" {
					if keep := keepDaysOf(st, asInt64(m["session_id"])); keep > 0 {
						reasonEN += fmt.Sprintf("; it keeps %s in the fridge", countDays(keep, false))
						reasonDE += fmt.Sprintf("; es hält sich %s im Kühlschrank", countDays(keep, true))
					}
				}
				en += " (" + reasonEN + ")"
				de += " (" + reasonDE + ")"
			}
			sink.add(classMoved, kindLeftover, statusOK, dot(en), dot(de))
		}
	}
}

// addAlsoChangedSaid gives each cook_moved entry of the applied results a
// plain sentence: the cooking moved, the meals did not (the model told a
// moved cook as a moved meal).
func addAlsoChangedSaid(st planning.State, applied []map[string]any) {
	for _, e := range applied {
		list, _ := e["also_changed"].([]any)
		for _, x := range list {
			m, ok := x.(map[string]any)
			if !ok || m["what"] != plate.AlsoCookMoved {
				continue
			}
			to, okTo := asDate(m["date"])
			from, okFrom := asDate(m["from_date"])
			dish := sessionDish(st, asInt64(m["session_id"]))
			if !okTo || !okFrom || dish == "" {
				continue
			}
			m["said"] = fmt.Sprintf("The cooking of %s moved from %s to %s; the meals stay where they are.",
				dish, cellLabel(st, from, asInt64(m["from_slot_id"])), cellLabel(st, to, asInt64(m["slot_id"])))
		}
	}
}

// weekFacts is what this change made worse for the week, with the numbers: a
// figure that crossed its limit or moved further past it, or that fell short
// (beans, fish, protein). A figure that moved without crossing is no news.
func weekFacts(in reportInput, bs, as planning.State) {
	bw, aw := in.before.Week, in.after.Week
	if in.undo {
		return
	}
	add := func(key, status, en, de string) {
		in.facts.addKey("week:"+key, classWorse, kindWeek, status, "For the week: "+en+".", "Für die Woche: "+de+".")
	}
	if bw != nil && aw != nil {
		limit := 300 * float64(aw.Days) / 7
		wasLimit := 300 * float64(bw.Days) / 7
		if aw.MeatG > limit && (aw.MeatG >= bw.MeatG+25 || bw.MeatG <= wasLimit) {
			en := fmt.Sprintf("meat is %.0f g (was %.0f g, limit %.0f g)", aw.MeatG, bw.MeatG, limit)
			de := fmt.Sprintf("Fleisch %.0f g (vorher %.0f g, Grenze %.0f g)", aw.MeatG, bw.MeatG, limit)
			if aw.Days < 7 {
				en = fmt.Sprintf("meat is %.0f g (was %.0f g, limit for the %d days you are home: %.0f g)", aw.MeatG, bw.MeatG, aw.Days, limit)
				de = fmt.Sprintf("Fleisch %.0f g (vorher %.0f g, Grenze für die %d Tage, an denen du da bist: %.0f g)", aw.MeatG, bw.MeatG, aw.Days, limit)
			}
			add("meat", statusOver, en, de)
		}
		pol := planning.PolicyOf(as.Profile)
		lo, hi := pol.WeekLow*100, pol.WeekHigh*100
		inWeek := func(p float64) bool { return p >= lo-0.5 && p <= hi+0.5 }
		if !inWeek(aw.Pct) && (inWeek(bw.Pct) || absf(aw.Pct-lo) > absf(bw.Pct-lo) && aw.Pct < lo || aw.Pct > hi && aw.Pct > bw.Pct) && absf(aw.Pct-bw.Pct) >= 3 {
			st := statusUnder
			if aw.Pct > hi {
				st = statusOver
			}
			add("energy", st, fmt.Sprintf("average energy is %.0f%% of the target (was %.0f%%); the week's range is %.0f–%.0f%%", aw.Pct, bw.Pct, lo, hi),
				fmt.Sprintf("durchschnittliche Energie %.0f %% des Ziels (vorher %.0f %%); der Bereich der Woche liegt bei %.0f–%.0f %%", aw.Pct, bw.Pct, lo, hi))
		}
		if aw.TargetProteinG > 0 && aw.AvgProteinG < aw.TargetProteinG && aw.AvgProteinG <= bw.AvgProteinG-5 {
			add("protein", statusUnder, fmt.Sprintf("protein averages %.0f g a day against %.0f g wanted (was %.0f g)", aw.AvgProteinG, aw.TargetProteinG, bw.AvgProteinG),
				fmt.Sprintf("Eiweiß liegt im Schnitt bei %.0f g pro Tag, gewünscht sind %.0f g (vorher %.0f g)", aw.AvgProteinG, aw.TargetProteinG, bw.AvgProteinG))
		}
		if aw.FishMeals == 0 && bw.FishMeals != 0 {
			add("fish", statusUnder, fmt.Sprintf("fish is on the plan %s (was %s)", timesWord(aw.FishMeals), timesWord(bw.FishMeals)),
				fmt.Sprintf("Fisch steht %s auf dem Plan (vorher %s)", deTimes(aw.FishMeals), deTimes(bw.FishMeals)))
		}
		if aw.LegumeMeals == 0 && bw.LegumeMeals != 0 {
			add("legumes", statusUnder, fmt.Sprintf("beans or lentils are on the plan %s (was %s)", timesWord(aw.LegumeMeals), timesWord(bw.LegumeMeals)),
				fmt.Sprintf("Bohnen oder Linsen stehen %s auf dem Plan (vorher %s)", deTimes(aw.LegumeMeals), deTimes(bw.LegumeMeals)))
		}
	}
	if in.after.MeatDaysMax != nil && in.after.MeatDays > *in.after.MeatDaysMax && in.before.MeatDays <= *in.after.MeatDaysMax {
		add("meatdays", statusOver, fmt.Sprintf("meat is on %d days (was %d, at most %d)", in.after.MeatDays, in.before.MeatDays, *in.after.MeatDaysMax),
			fmt.Sprintf("Fleisch steht an %d Tagen auf dem Plan (vorher %d, höchstens %d)", in.after.MeatDays, in.before.MeatDays, *in.after.MeatDaysMax))
	}
}

func timesWord(n int) string {
	switch n {
	case 0:
		return "not at all"
	case 1:
		return "once"
	}
	return fmt.Sprintf("%d times", n)
}

// issueFigure names the figure an issue states, so a fact of the change and a
// warning about the same figure are told once ("day:Sat 21 Nov", "week:meat").
func issueFigure(is planning.Issue) string {
	day := ""
	if is.Date != "" {
		day = dayLabelStr(is.Date)
	}
	switch is.Code {
	case planning.CodeKcalBelowBand, planning.CodeKcalAboveBand, planning.CodeKcalBelowFloor:
		return "day:" + day
	case planning.CodeProteinLow:
		return "protein:" + day
	case planning.CodeWeekMeatAboveDGE:
		return "week:meat"
	case planning.CodeWeekKcalBelowBand, planning.CodeWeekKcalAboveBand:
		return "week:energy"
	case planning.CodeWeekProteinLow:
		return "week:protein"
	case planning.CodeWeekFishBelowDGE:
		return "week:fish"
	case planning.CodeWeekLegumesBelow:
		return "week:legumes"
	case planning.CodeMeatDaysExceeded:
		return "week:meatdays"
	}
	return ""
}

// statusOfIssue says on which side of its limit an issue is.
func statusOfIssue(code string) string {
	switch code {
	case planning.CodeKcalBelowBand, planning.CodeKcalBelowFloor, planning.CodeProteinLow, planning.CodeWeekProteinLow,
		planning.CodeWeekKcalBelowBand, planning.CodeMealNotHearty:
		return statusUnder
	case planning.CodeKcalAboveBand, planning.CodeWeekKcalAboveBand, planning.CodeMealNotLight, planning.CodeWeekMeatAboveDGE,
		planning.CodeMeatDaysExceeded, planning.CodeDishRepeats, planning.CodeAddOnExcess, planning.CodeTooManyCookOccasions:
		return statusOver
	}
	return statusUnknown
}

// warnings adds the issues that did not exist before the change as class 5
// facts, and the earlier ones that got worse by a real step as class 4 facts. It
// returns the number of earlier ones left as they were (or moved by less than a
// step). A week figure that only moved because a day went away is no fact,
// unless the week's meat is over its limit.
func warnings(in reportInput, as planning.State, away bool, days []reportDay) (unaffected int) {
	// A day outside its range has its day line (class 4): no second line for the
	// same figures.
	dayTold := map[string]bool{}
	for _, d := range days {
		if d.Range != "" {
			dayTold[d.Day] = true
		}
	}
	was := issueMap(in.before.Issues)
	issues := slices.Clone(in.after.Issues)
	slices.SortFunc(issues, func(a, b planning.Issue) int { return strings.Compare(a.Date+a.Code, b.Date+b.Code) })
	for _, is := range issues {
		if is.Date != "" && dayTold[dayLabelStr(is.Date)] && slices.Contains([]string{planning.CodeKcalBelowBand, planning.CodeKcalAboveBand, planning.CodeKcalBelowFloor}, is.Code) {
			continue
		}
		b, ok := was[issueKey(is)]
		switch {
		case !ok:
			en, de := issueSentence(is, as)
			in.facts.addKey(issueFigure(is), classWarn, kindWarn, statusOfIssue(is.Code), "New warning: "+en, "Neue Warnung: "+de)
		case worsened(b, is) && away && is.Date == "" && is.Code != planning.CodeWeekFatRange && is.Code != planning.CodeWeekCarbsRange:
			// Only the day count changed: the remaining days eat what they ate.
			unaffected++
			if is.Code == planning.CodeWeekMeatAboveDGE && paramNum(is, "meat_g") > paramNum(is, "max_g") {
				en, de := awayNeutral(in, is)
				in.facts.addKey("week:meat", classWorse, kindWeek, statusOver, en, de)
			}
		case worsened(b, is):
			bb, _ := badness(b)
			ab, _ := badness(is)
			en, de := issueSentence(is, as)
			in.facts.addKey(issueFigure(is), classWorse, kindWarn, statusOfIssue(is.Code),
				fmt.Sprintf("Worse: %s It was already open and got worse: %s beyond its limit before, %s now.", en, badnessWords(is.Code, bb), badnessWords(is.Code, ab)),
				fmt.Sprintf("Schlimmer: %s Es war schon offen und ist schlimmer geworden: vorher %s über der Grenze, jetzt %s.", de, badnessWordsL(is.Code, bb, langDE), badnessWordsL(is.Code, ab, langDE)))
		default:
			unaffected++
		}
	}
	return unaffected
}

// awayOnly reports a change that only removed meals (a day away): the food of
// the remaining days is as it was.
func awayOnly(changed []reportMeal) bool {
	if len(changed) == 0 {
		return false
	}
	for _, m := range changed {
		if len(m.After) > 0 {
			return false
		}
	}
	return true
}

// awayNeutral is the line for the week's meat that was already over its limit
// and is counted over fewer days now.
func awayNeutral(in reportInput, is planning.Issue) (en, de string) {
	days := 0
	if in.after.Week != nil {
		days = in.after.Week.Days
	}
	var away []string
	for _, m := range in.beforeSt.Plates {
		found := false
		for _, a := range in.afterSt.Plates {
			if a.ID == m.ID {
				found = true
			}
		}
		if !found && !slices.Contains(away, dayLabel(m.Date)) {
			away = append(away, dayLabel(m.Date))
		}
	}
	who, whoDE := "a day", "einem Tag"
	if len(away) > 0 {
		who = strings.Join(away, " and ")
		d := make([]string, len(away))
		for i, a := range away {
			d[i] = deDay(a)
		}
		whoDE = strings.Join(d, " und ")
	}
	over := paramNum(is, "meat_g") - paramNum(is, "max_g")
	en = fmt.Sprintf("With %s away the week is counted over %d days; the meat limit for those days is %.0f g and the plan holds %.0f g, as before%s.", who, days, paramNum(is, "max_g"), paramNum(is, "meat_g"), overBy(over))
	de = fmt.Sprintf("Bei Abwesenheit an %s wird die Woche über %d Tage gezählt; die Fleischgrenze für diese Tage liegt bei %.0f g, der Plan hat %.0f g, wie vorher%s.", whoDE, days, paramNum(is, "max_g"), paramNum(is, "meat_g"), overByDE(over))
	return en, de
}

// overBy says how far over a limit, "; that is 64 g over", or nothing.
func overBy(g float64) string {
	if g < 0.5 {
		return ""
	}
	return fmt.Sprintf("; that is %.0f g over", g)
}

func overByDE(g float64) string {
	if g < 0.5 {
		return ""
	}
	return fmt.Sprintf("; das sind %.0f g darüber", g)
}

// badnessWords says a badness value with its unit.
func badnessWords(code string, v float64) string { return badnessWordsL(code, v, langEN) }

// badnessWordsL says a badness value with its unit in one language.
func badnessWordsL(code string, v float64, lg lang) string {
	switch code {
	case planning.CodeKcalBelowBand, planning.CodeKcalAboveBand, planning.CodeKcalBelowFloor, planning.CodeMealNotLight, planning.CodeMealNotHearty:
		return fmtNum(math.Round(v)) + " kcal"
	case planning.CodeProteinLow, planning.CodeWeekProteinLow, planning.CodeWeekMeatAboveDGE:
		return fmtNum(math.Round(v)) + " g"
	case planning.CodeWeekKcalBelowBand, planning.CodeWeekKcalAboveBand, planning.CodeWeekCarbsRange, planning.CodeWeekFatRange:
		if lg == langDE {
			return fmtComma(math.Round(v*10)/10) + " Punkte"
		}
		return fmtNum(math.Round(v*10)/10) + " points"
	}
	if lg == langDE {
		return fmtComma(v)
	}
	return fmtNum(v)
}

func paramNum(is planning.Issue, k string) float64 {
	switch v := is.Params[k].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	}
	return 0
}

// issueSentence says an issue in one plain sentence naming the day or meal
// and the numbers, in English and in German.
func issueSentence(is planning.Issue, st planning.State) (en, de string) {
	where := ""
	switch {
	case is.PlateID != nil:
		for _, p := range st.Plates {
			if p.ID == *is.PlateID {
				where = cellLabel(st, p.Date, p.SlotID)
			}
		}
	case is.SlotID != nil && is.Date != "":
		where = cellLabel(st, mustDate(is.Date), *is.SlotID)
	}
	if where == "" && is.Date != "" {
		where = dayLabelStr(is.Date)
	}
	week := where == ""
	if week {
		where = "The week"
	}
	whereDE := deCell(where)
	if week {
		whereDE = "Die Woche"
	} else if len(strings.Fields(where)) == 3 {
		whereDE = deDay(where)
	}
	n := func(k string) float64 { return paramNum(is, k) }
	both := func(e, d string) (string, string) { return e, d }
	switch is.Code {
	case planning.CodeTooManyCookOccasions:
		return both(fmt.Sprintf("%s has %.0f separate cooking times; the limit is %.0f.", where, n("count"), n("max")),
			fmt.Sprintf("%s hat %.0f getrennte Kochzeiten; erlaubt sind %.0f.", whereDE, n("count"), n("max")))
	case planning.CodeKcalBelowFloor:
		return both(fmt.Sprintf("%s holds only %.0f kcal, under the day's minimum of %.0f kcal.", where, n("kcal"), n("floor_kcal")),
			fmt.Sprintf("%s hat nur %.0f kcal, unter dem Mindestwert des Tages von %.0f kcal.", whereDE, n("kcal"), n("floor_kcal")))
	case planning.CodeKcalBelowBand, planning.CodeKcalAboveBand:
		side, sideDE := "below", "unter"
		if is.Code == planning.CodeKcalAboveBand {
			side, sideDE = "above", "über"
		}
		return both(fmt.Sprintf("%s holds %.0f kcal, %s a good amount for the day (%.0f–%.0f kcal).", where, n("kcal"), side, n("band_low_kcal"), n("band_high_kcal")),
			fmt.Sprintf("%s hat %.0f kcal, %s dem passenden Bereich für den Tag (%.0f–%.0f kcal).", whereDE, n("kcal"), sideDE, n("band_low_kcal"), n("band_high_kcal")))
	case planning.CodeProteinLow:
		return both(fmt.Sprintf("%s has %.0f g protein; the minimum is %.0f g.", where, n("protein_g"), n("min_g")),
			fmt.Sprintf("%s hat %.0f g Eiweiß; das Minimum sind %.0f g.", whereDE, n("protein_g"), n("min_g")))
	case planning.CodeMealNotLight:
		return both(fmt.Sprintf("%s is meant to be light (at most %.0f kcal) but holds %.0f kcal.", where, n("max_kcal"), n("kcal")),
			fmt.Sprintf("%s soll leicht sein (höchstens %.0f kcal), hat aber %.0f kcal.", whereDE, n("max_kcal"), n("kcal")))
	case planning.CodeMealNotHearty:
		return both(fmt.Sprintf("%s is meant to be filling (at least %.0f kcal) but holds %.0f kcal.", where, n("min_kcal"), n("kcal")),
			fmt.Sprintf("%s soll sättigend sein (mindestens %.0f kcal), hat aber %.0f kcal.", whereDE, n("min_kcal"), n("kcal")))
	case planning.CodeWeekMeatAboveDGE:
		if d := n("days"); d > 0 && d < 7 {
			return both(fmt.Sprintf("The week's meat is %.0f g; the limit for the %.0f days you are home is %.0f g (300 g a week).", n("meat_g"), d, n("max_g")),
				fmt.Sprintf("Das Fleisch der Woche beträgt %.0f g; die Grenze für die %.0f Tage, an denen du da bist, liegt bei %.0f g (300 g pro Woche).", n("meat_g"), d, n("max_g")))
		}
		return both(fmt.Sprintf("The week's meat is %.0f g; the limit is %.0f g.", n("meat_g"), n("max_g")),
			fmt.Sprintf("Das Fleisch der Woche beträgt %.0f g; die Grenze liegt bei %.0f g.", n("meat_g"), n("max_g")))
	case planning.CodeWeekKcalBelowBand, planning.CodeWeekKcalAboveBand:
		return both(fmt.Sprintf("The week's average is %.0f kcal a day, %.0f%% of the target; the week's range is %.0f-%.0f%%.", n("avg_kcal"), n("pct"), n("band_low_pct"), n("band_high_pct")),
			fmt.Sprintf("Der Wochendurchschnitt liegt bei %.0f kcal pro Tag, %.0f %% des Ziels; der Bereich der Woche liegt bei %.0f–%.0f %%.", n("avg_kcal"), n("pct"), n("band_low_pct"), n("band_high_pct")))
	case planning.CodeWeekProteinLow:
		return both(fmt.Sprintf("The week's average protein is %.0f g a day; the target is %.0f g.", n("avg_protein_g"), n("target_protein_g")),
			fmt.Sprintf("Das Eiweiß der Woche liegt im Schnitt bei %.0f g pro Tag; das Ziel sind %.0f g.", n("avg_protein_g"), n("target_protein_g")))
	case planning.CodeWeekCarbsRange, planning.CodeWeekFatRange:
		label, labelDE := "Weekly carbs", "Kohlenhydrate der Woche"
		if is.Code == planning.CodeWeekFatRange {
			label, labelDE = "Weekly fat", "Fett der Woche"
		}
		return both(fmt.Sprintf("%s: %.0f%% of the week's energy; the range is %.0f-%.0f%%.", label, n("pct"), n("range_low_pct"), n("range_high_pct")),
			fmt.Sprintf("%s: %.0f %% der Energie der Woche; der Bereich liegt bei %.0f–%.0f %%.", labelDE, n("pct"), n("range_low_pct"), n("range_high_pct")))
	case planning.CodeDishRepeats:
		name := fmt.Sprint(is.Params["name"])
		return both(fmt.Sprintf("%s is the main dish of %.0f meals; the limit is %.0f.", food.Quote(name), n("meals"), n("max")),
			fmt.Sprintf("%s ist das Hauptgericht von %.0f Mahlzeiten; erlaubt sind %.0f.", deQuote(name), n("meals"), n("max")))
	case planning.CodeMeatDaysExceeded:
		return both(fmt.Sprintf("%.0f days of the week have meat; the limit is %.0f.", n("days"), n("max")),
			fmt.Sprintf("An %.0f Tagen der Woche gibt es Fleisch; erlaubt sind %.0f.", n("days"), n("max")))
	case planning.CodeAddOnExcess:
		why := map[string][2]string{
			"light_meal":      {"added to a light meal", "zu einer leichten Mahlzeit dazugegeben"},
			"duplicates_main": {"already in the main", "schon im Hauptgericht enthalten"},
			"over_cap":        {fmt.Sprintf("over the usual serving of %.0f g", n("max_grams")), fmt.Sprintf("über der üblichen Portion von %.0f g", n("max_grams"))},
			"day_limit":       {"more sides that day than usual", "mehr Beilagen an dem Tag als üblich"},
			"repeated":        {"the same side on too many meals", "dieselbe Beilage zu zu vielen Mahlzeiten"},
		}[fmt.Sprint(is.Params["reason"])]
		name := fmt.Sprint(is.Params["name"])
		return both(fmt.Sprintf("%s: %s %.0f g is %s.", where, food.Quote(name), n("grams"), why[0]),
			fmt.Sprintf("%s: %s %.0f g ist %s.", whereDE, deQuote(name), n("grams"), why[1]))
	}
	switch is.Code {
	case planning.CodeVarietyRepeat, planning.CodeVarietyRelaxed, planning.CodeVarietyRepeatWeek, planning.CodeVarietyRepeatNamed:
		if dish := issueDish(is, st); dish != "" {
			dishDE := deQuoted(dish)
			if is.Code == planning.CodeVarietyRepeatWeek {
				if meals := sessionMealLabels(st, is); len(meals) > 1 {
					de := make([]string, len(meals))
					for i, m := range meals {
						de[i] = deCell(m)
					}
					return both(fmt.Sprintf("%s is cooked more than once this week: it is eaten at %s.", dish, strings.Join(meals, ", ")),
						fmt.Sprintf("%s wird diese Woche mehr als einmal gekocht: es wird gegessen am %s.", dishDE, strings.Join(de, ", ")))
				}
				return both(fmt.Sprintf("%s: %s is cooked more than once this week.", where, dish),
					fmt.Sprintf("%s: %s wird diese Woche mehr als einmal gekocht.", whereDE, dishDE))
			}
			if is.Code == planning.CodeVarietyRepeatNamed {
				return both(fmt.Sprintf("%s: %s is in the plan as you asked, but it was cooked or eaten recently before.", where, dish),
					fmt.Sprintf("%s: %s steht wie gewünscht im Plan, wurde aber vorher erst kürzlich gekocht oder gegessen.", whereDE, dishDE))
			}
			return both(fmt.Sprintf("%s: %s was cooked or eaten too recently before.", where, dish),
				fmt.Sprintf("%s: %s wurde vorher zu kürzlich gekocht oder gegessen.", whereDE, dishDE))
		}
	}
	msgDE, ok := issueMessageDE[is.Code]
	if !ok {
		msgDE = "eine Planungsregel ist nicht erfüllt."
	}
	return where + ": " + planning.PlainText(is.Code).Message, whereDE + ": " + msgDE
}

// issueDish is the main dish of the plate or batch an issue sits on.
func issueDish(is planning.Issue, st planning.State) string {
	if is.SessionID != nil {
		return sessionDish(st, *is.SessionID)
	}
	if is.PlateID != nil {
		for _, p := range st.Plates {
			if p.ID == *is.PlateID {
				for _, c := range p.Components {
					if c.Position == plate.PositionMain {
						return food.Quote(foodName(st, c.FoodID))
					}
				}
			}
		}
	}
	return ""
}

// reportOf reads the plan after the ops (inside the unit of work, so a check
// still sees them) and builds the report. A state that cannot be read leaves
// a report that says so, never a guess.
func reportOf(ctx context.Context, svc Services, w window.Window, beforeSt planning.State, beforeErr error, before, after planning.Evaluation, applied []map[string]any, dayKcal bool) *changeReport {
	if beforeErr != nil {
		return &changeReport{NotCovered: []string{"the plan before the change could not be read"}}
	}
	afterSt, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return &changeReport{NotCovered: []string{"the plan after the change could not be read"}}
	}
	r := buildReport(reportInput{beforeSt: beforeSt, afterSt: afterSt, before: before, after: after, applied: applied, dayKcal: dayKcal, light: lightFrom(ctx)})
	return &r
}

// withReport adds a report to the result of a tool that changes the plan
// outside apply_changes (undo_last_change, regenerate): the plan is read
// before and after the call. A result that is not an object, a failed call or
// an unreadable plan returns the result as it is.
func withReport(svc Services, h ToolHandler, undo bool) ToolHandler {
	return func(ctx context.Context, in json.RawMessage) (json.RawMessage, ToolEffect, error) {
		w, ok := windowFrom(ctx)
		if !ok || svc.State == nil {
			return h(ctx, in)
		}
		beforeSt, e1 := svc.State(ctx, w, todayFrom(ctx))
		beforeEv, e2 := evalWindow(ctx, svc, w, false, nil)
		out, eff, err := h(ctx, in)
		if err != nil || e1 != nil || e2 != nil {
			return out, eff, err
		}
		afterSt, e3 := svc.State(ctx, w, todayFrom(ctx))
		afterEv, e4 := evalWindow(ctx, svc, w, false, nil)
		var obj map[string]json.RawMessage
		if e3 != nil || e4 != nil || json.Unmarshal(out, &obj) != nil || obj == nil {
			return out, eff, err
		}
		r := buildReport(reportInput{beforeSt: beforeSt, afterSt: afterSt, before: beforeEv, after: afterEv, undo: undo})
		if undo {
			// An undo restores the plan exactly as it was before the undone
			// change: every warning open now existed then, so none is new or
			// worse; they are all earlier ones.
			r.EarlierWarnings = len(afterEv.Issues)
			r.Note = "Undone: the plan is exactly as it was before that change, and the warnings open now were open then: list no warning."
			for i := range r.Days {
				r.Days[i].Status = ""
			}
		}
		if len(r.Changed) > 0 || len(r.Cooking) > 0 {
			obj["report"] = mustJSON(r)
			out = mustJSON(obj)
		}
		return out, eff, nil
	}
}

// pairByCell merges a removed plate and an added plate on the same cell into
// one change: a regenerated meal has a new plate id, but it is one meal that
// became another.
func pairByCell(in []reportMeal) []reportMeal {
	var out []reportMeal
	for _, m := range in {
		if len(m.Before) == 0 && len(m.After) > 0 && m.MovedFrom == "" {
			merged := false
			for i := range out {
				o := &out[i]
				if len(o.After) == 0 && len(o.Before) > 0 && o.Day == m.Day && o.Meal == m.Meal && o.MovedFrom == "" {
					o.After, o.KcalTo = m.After, m.KcalTo
					merged = true
					break
				}
			}
			if merged {
				continue
			}
		} else if len(m.After) == 0 && len(m.Before) > 0 {
			merged := false
			for i := range out {
				o := &out[i]
				if len(o.Before) == 0 && len(o.After) > 0 && o.Day == m.Day && o.Meal == m.Meal && o.MovedFrom == "" {
					o.Before, o.KcalFrom = m.Before, m.KcalFrom
					merged = true
					break
				}
			}
			if merged {
				continue
			}
		}
		out = append(out, m)
	}
	return out
}

// sessionMealLabels are the meals that eat the batch an issue sits on.
func sessionMealLabels(st planning.State, is planning.Issue) []string {
	if is.SessionID == nil {
		return nil
	}
	var cells []plate.CellKey
	for _, p := range st.Plates {
		for _, c := range p.Components {
			if c.SessionID == *is.SessionID {
				cells = append(cells, p.Cell())
			}
		}
	}
	slices.SortFunc(cells, func(a, b plate.CellKey) int {
		if c := a.Date.Compare(b.Date); c != 0 {
			return c
		}
		return int(st.SlotOrder[a.SlotID]) - int(st.SlotOrder[b.SlotID])
	})
	cells = slices.Compact(cells)
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = cellLabel(st, c.Date, c.SlotID)
	}
	return out
}

// issueMessageDE is the plain sentence of an issue code in German, for the codes
// issueSentence has no numbers for.
var issueMessageDE = map[string]string{
	planning.CodeTrialRepeat:         "Dieses Rezept ist auf Probe und wird diese Woche schon gekocht.",
	planning.CodeMealOutsideSlotBand: "Eine Mahlzeit hat mehr oder weniger von der Tagesenergie, als für diese Zeit gedacht ist.",
	planning.CodeWeekFishBelowDGE:    "In der Woche gibt es keine Fischmahlzeit.",
	planning.CodeWeekFishAboveDGE:    "In der Woche gibt es mehr als zwei Fischmahlzeiten.",
	planning.CodeWeekLegumesBelow:    "In der Woche gibt es keine Mahlzeit mit Hülsenfrüchten.",
	planning.CodeDietIncompatible:    "Ein Lebensmittel passt nicht zur Ernährungsweise.",
	planning.CodeDietUnknown:         "Bei einem Lebensmittel ist die Ernährungsweise unbekannt.",
	planning.CodeMissingPlate:        "Für eine Mahlzeit fehlt das Essen.",
	planning.CodeCookInNoneCell:      "Es wird zu einer Zeit gekocht, zu der niemand da ist.",
	"leftover_before_cook":           "Reste werden gegessen, bevor sie gekocht sind.",
	"past_keep_days":                 "Reste werden gegessen, nachdem sie nicht mehr haltbar sind.",
	"deficit":                        "Von einem Gericht wird mehr gegessen, als gekocht wurde.",
}
