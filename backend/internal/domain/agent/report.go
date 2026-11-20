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
	// Leftovers are cooked portions without a meal and portions a meal needs
	// beyond its batch, each with what happened to the batch.
	Leftovers []string `json:"-"` // said in Tell
	// AlsoChanged is every change outside the requested meals.
	AlsoChanged []string `json:"-"` // said in Tell
	// Week holds only what this change made better or worse for the week.
	Week []string `json:"-"` // said in Tell
	// NewWarnings are the warnings and problems that did not exist before the
	// change. Worse holds an earlier one that got worse by a real step (see
	// worseStep), with its figure before and after.
	NewWarnings []string `json:"-"` // said in Tell
	Worse       []string `json:"-"` // said in Tell
	// EarlierWarnings counts the warnings that existed before and are
	// unaffected: do not list them.
	EarlierWarnings int `json:"earlier_warnings_unaffected"`
	// Tell is the ordered list of plain sentences the reply must convey,
	// computed from the saved plan (see buildTell).
	Tell []string `json:"tell,omitempty"`
	// Neutral is the one line for a week figure that only moved because a day
	// went away (the week is counted over fewer days).
	Neutral string `json:"-"`
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
		s := foodName(st, c.FoodID)
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
		if a.Away {
			day.Status = "away: the day is not counted"
		} else if a.BandHigh > 0 {
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

	r.Leftovers = leftovers(bs, as)
	r.AlsoChanged = alsoChanged(as, in.applied)
	r.Week = weekFacts(in, bs, as)
	r.NewWarnings, r.Worse, r.EarlierWarnings, r.Neutral = warnings(in, as, awayOnly(r.Changed))
	r.Tell = buildTell(in, r)
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
// cooking the saved plan no longer has is said as "no longer cooked".
func leftovers(bs, as planning.State) []string {
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
	// A dish whose cooking is gone from one day and new on another moved.
	movedTo := map[string]string{}
	for k, v := range a {
		if _, ok := b[k]; !ok {
			movedTo[v.dish] = v.cooked.Format("Mon 2 Jan") + " at " + v.cookedAt
		}
	}
	var out []string
	for _, k := range keys {
		was, wasOK := b[k]
		now, nowOK := a[k]
		switch {
		case wasOK && !nowOK:
			if was.eaten > 0.005 {
				if to, ok := movedTo[was.dish]; ok {
					out = append(out, fmt.Sprintf("The %s is now cooked on %s instead of %s at %s.", food.Quote(was.dish), to, was.cooked.Format("Mon 2 Jan"), was.cookedAt))
				} else {
					out = append(out, fmt.Sprintf("The %s cooked on %s is no longer cooked.", food.Quote(was.dish), was.cooked.Format("Mon 2 Jan")))
				}
			}
		case !wasOK:
		default:
			if math.Abs(was.eaten-now.eaten) < 0.005 && math.Abs(was.portions-now.portions) < 0.005 && was.links == now.links {
				continue
			}
			free := now.portions - now.eaten
			frozen := ""
			if now.frozen > 0.005 {
				frozen = fmt.Sprintf(" (%s are meant for the freezer)", portionWord(now.frozen))
			}
			resized := "It was not resized."
			if math.Abs(was.portions-now.portions) >= 0.005 {
				resized = fmt.Sprintf("It was resized from %s to %s.", portionWord(was.portions), portionWord(now.portions))
			}
			day := now.cooked.Format("Mon 2 Jan")
			switch {
			case math.Abs(free) <= 0.005 && math.Abs(was.portions-now.portions) >= 0.005:
				out = append(out, fmt.Sprintf("%s: cook %s of the %s (was %s).", day, portionWord(now.portions), food.Quote(now.dish), fmtNum(was.portions)))
			case free > 0.005:
				out = append(out, fmt.Sprintf("%s of the %s cooked on %s is now left over (it is cooked for %s%s, the meals eat %s). %s", portionWord(free), food.Quote(now.dish), day, portionWord(now.portions), frozen, fmtNum(now.eaten), resized))
			case free < -0.005:
				out = append(out, fmt.Sprintf("The meals need %s more of the %s cooked on %s than it holds (%s). %s", portionWord(-free), food.Quote(now.dish), day, portionWord(now.portions), resized))
			}
		}
	}
	return out
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

// alsoChanged says each knock-on change the operations reported: moved cooks,
// meals that now cook fresh, resized batches and the automatic fixes.
func alsoChanged(st planning.State, applied []map[string]any) []string {
	var out []string
	cell := func(m map[string]any, dk, sk string) string {
		if d, ok := asDate(m[dk]); ok {
			return cellLabel(st, d, asInt64(m[sk]))
		}
		return ""
	}
	for _, e := range applied {
		if list, ok := e["also_changed"].([]any); ok {
			for _, x := range list {
				m, ok := x.(map[string]any)
				if !ok {
					continue
				}
				dish := sessionDish(st, asInt64(m["session_id"]))
				if dish == "" {
					dish = "a dish"
				}
				switch m["what"] {
				case "cook_moved", "became_cooking_meal":
					s := fmt.Sprintf("The %s is now cooked at %s", dish, cell(m, "date", "slot_id"))
					if from := cell(m, "from_date", "from_slot_id"); from != "" {
						s += " instead of " + from
					}
					out = append(out, s+".")
				case "cooks_fresh":
					why := map[string]string{"before_cook": "it comes before the batch is cooked", "past_keep_days": "the batch would be too old by then", "cooked": "the batch is already cooked"}[fmt.Sprint(m["why"])]
					s := fmt.Sprintf("%s now cooks its own %s instead of eating the one cooked earlier", cell(m, "date", "slot_id"), dish)
					if why != "" {
						if m["why"] == "past_keep_days" {
							why += keepTooOld(st, asInt64(m["session_id"]))
						}
						s += " (" + why + ")"
					}
					out = append(out, s+".")
				case "batch_resized":
					out = append(out, fmt.Sprintf("The %s cooking was resized.", dish))
				}
			}
		}
		if e["auto_fix"] != nil {
			dish := sessionDish(st, asInt64(e["session_id"]))
			if dish == "" {
				dish = "a dish"
			}
			s := fmt.Sprintf("To keep one cooking time a day, %s is now cooked at %s", dish, cell(e, "cook_date", "cook_slot_id"))
			if from := cell(e, "from_date", "from_slot_id"); from != "" {
				s += " instead of " + from
			}
			out = append(out, s+".")
		}
	}
	return out
}

// mainDishes is the dish that is the main on each lunch or dinner plate of the
// window: dish name to its meals.
func mainDishes(st planning.State) map[string][]string {
	out := map[string][]string{}
	for _, p := range st.Plates {
		if !st.Window.Contains(p.Date) || p.Proposed() {
			continue
		}
		for _, c := range p.Components {
			if c.Position == plate.PositionMain && c.Portions != nil {
				n := foodName(st, c.FoodID)
				out[n] = append(out[n], cellLabel(st, p.Date, p.SlotID))
				break
			}
		}
	}
	return out
}

// weekFacts is what this change did to the week: the figures that moved, with
// the numbers, and the dishes whose repeats changed.
func weekFacts(in reportInput, bs, as planning.State) []string {
	var out []string
	bw, aw := in.before.Week, in.after.Week
	if bw != nil && aw != nil {
		limit := 300 * float64(aw.Days) / 7
		// A figure is told only when this change moved it past a real step
		// (25 g meat, 3 points of energy, 5 g protein) or across a line.
		if math.Abs(aw.MeatG-bw.MeatG) >= 25 || (aw.MeatG > limit) != (bw.MeatG > 300*float64(bw.Days)/7) {
			s := fmt.Sprintf("meat is %.0f g (was %.0f g, limit %.0f g)", aw.MeatG, bw.MeatG, limit)
			if aw.Days < 7 {
				s = fmt.Sprintf("meat is %.0f g (was %.0f g, limit for the %d days you are home: %.0f g)", aw.MeatG, bw.MeatG, aw.Days, limit)
			}
			out = append(out, s)
		}
		pol := planning.PolicyOf(as.Profile)
		inWeek := func(p float64) bool { return p >= pol.WeekLow*100-0.5 && p <= pol.WeekHigh*100+0.5 }
		if math.Abs(aw.Pct-bw.Pct) >= 3 || inWeek(aw.Pct) != inWeek(bw.Pct) {
			out = append(out, fmt.Sprintf("average energy is %.0f%% of the target (was %.0f%%); the week's range is %.0f–%.0f%%", aw.Pct, bw.Pct, pol.WeekLow*100, pol.WeekHigh*100))
		}
		if aw.TargetProteinG > 0 && math.Abs(aw.AvgProteinG-bw.AvgProteinG) >= 5 {
			out = append(out, fmt.Sprintf("protein averages %.0f g a day against %.0f g wanted (was %.0f g)", aw.AvgProteinG, aw.TargetProteinG, bw.AvgProteinG))
		}
		if (aw.FishMeals == 0) != (bw.FishMeals == 0) {
			out = append(out, fmt.Sprintf("fish is on the plan %s (was %s)", timesWord(aw.FishMeals), timesWord(bw.FishMeals)))
		}
		if (aw.LegumeMeals == 0) != (bw.LegumeMeals == 0) {
			out = append(out, fmt.Sprintf("beans or lentils are on the plan %s (was %s)", timesWord(aw.LegumeMeals), timesWord(bw.LegumeMeals)))
		}
	}
	if in.after.MeatDaysMax != nil && (in.after.MeatDays > *in.after.MeatDaysMax) != (in.before.MeatDays > *in.after.MeatDaysMax) {
		out = append(out, fmt.Sprintf("meat is on %d days (was %d, at most %d)", in.after.MeatDays, in.before.MeatDays, *in.after.MeatDaysMax))
	}
	bm, am := mainDishes(bs), mainDishes(as)
	dishes := make([]string, 0, len(am))
	for d := range am {
		dishes = append(dishes, d)
	}
	slices.Sort(dishes)
	for _, d := range dishes {
		if len(am[d]) >= 2 && !slices.Equal(am[d], bm[d]) {
			out = append(out, fmt.Sprintf("%s is eaten as the main dish at %d meals: %s", food.Quote(d), len(am[d]), strings.Join(am[d], ", ")))
		}
	}
	return out
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

// warnings are the issues that did not exist before the change, the earlier
// ones that got worse by a real step, each as one plain sentence, and the
// number of earlier ones left as they were (or moved by less than a step).
func warnings(in reportInput, as planning.State, away bool) (out, worse []string, unaffected int, neutral string) {
	was := issueMap(in.before.Issues)
	issues := slices.Clone(in.after.Issues)
	slices.SortFunc(issues, func(a, b planning.Issue) int { return strings.Compare(a.Date+a.Code, b.Date+b.Code) })
	for _, is := range issues {
		b, ok := was[issueKey(is)]
		switch {
		case !ok:
			out = append(out, issueSentence(is, as))
		case worsened(b, is) && away && is.Date == "" && is.Code != planning.CodeWeekFatRange && is.Code != planning.CodeWeekCarbsRange:
			// Only the day count changed: the remaining days eat what they ate.
			unaffected++
			if neutral == "" || is.Code == planning.CodeWeekMeatAboveDGE {
				neutral = awayNeutral(in, as, is)
			}
		case worsened(b, is):
			bb, _ := badness(b)
			ab, _ := badness(is)
			worse = append(worse, fmt.Sprintf("%s It was already open and got worse: %s beyond its limit before, %s now.", issueSentence(is, as), badnessWords(is.Code, bb), badnessWords(is.Code, ab)))
		default:
			unaffected++
		}
	}
	return out, worse, unaffected, neutral
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

// awayNeutral is the one neutral line for a week figure that was already open
// and only changed because the week is counted over fewer days.
func awayNeutral(in reportInput, as planning.State, is planning.Issue) string {
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
	who := "a day"
	if len(away) > 0 {
		who = strings.Join(away, " and ")
	}
	if is.Code == planning.CodeWeekMeatAboveDGE {
		return fmt.Sprintf("With %s away the week is counted over %d days; the meat limit for those days is %.0f g and the plan holds %.0f g, as before%s.", who, days, paramNum(is, "max_g"), paramNum(is, "meat_g"), overBy(paramNum(is, "meat_g")-paramNum(is, "max_g")))
	}
	return fmt.Sprintf("With %s away the week is counted over %d days: %s It was already open, as before.", who, days, issueSentence(is, as))
}

// overBy says how far over a limit, "; that is 64 g over", or nothing.
func overBy(g float64) string {
	if g < 0.5 {
		return ""
	}
	return fmt.Sprintf("; that is %.0f g over", g)
}

// badnessWords says a badness value with its unit.
func badnessWords(code string, v float64) string {
	switch code {
	case planning.CodeKcalBelowBand, planning.CodeKcalAboveBand, planning.CodeKcalBelowFloor, planning.CodeMealNotLight, planning.CodeMealNotHearty:
		return fmtNum(math.Round(v)) + " kcal"
	case planning.CodeProteinLow, planning.CodeWeekProteinLow, planning.CodeWeekMeatAboveDGE:
		return fmtNum(math.Round(v)) + " g"
	case planning.CodeWeekKcalBelowBand, planning.CodeWeekKcalAboveBand, planning.CodeWeekCarbsRange, planning.CodeWeekFatRange:
		return fmtNum(math.Round(v*10)/10) + " points"
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
// and the numbers.
func issueSentence(is planning.Issue, st planning.State) string {
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
	if where == "" {
		where = "The week"
	}
	n := func(k string) float64 { return paramNum(is, k) }
	switch is.Code {
	case planning.CodeTooManyCookOccasions:
		return fmt.Sprintf("%s has %.0f separate cooking times; the limit is %.0f.", where, n("count"), n("max"))
	case planning.CodeKcalBelowFloor:
		return fmt.Sprintf("%s holds only %.0f kcal, under the day's minimum of %.0f kcal.", where, n("kcal"), n("floor_kcal"))
	case planning.CodeKcalBelowBand, planning.CodeKcalAboveBand:
		side := "under"
		if is.Code == planning.CodeKcalAboveBand {
			side = "over"
		}
		if side == "under" {
			side = "below"
		} else {
			side = "above"
		}
		return fmt.Sprintf("%s holds %.0f kcal, %s a good amount for the day (%.0f–%.0f kcal).", where, n("kcal"), side, n("band_low_kcal"), n("band_high_kcal"))
	case planning.CodeProteinLow:
		return fmt.Sprintf("%s has %.0f g protein; the minimum is %.0f g.", where, n("protein_g"), n("min_g"))
	case planning.CodeMealNotLight:
		return fmt.Sprintf("%s is meant to be light (at most %.0f kcal) but holds %.0f kcal.", where, n("max_kcal"), n("kcal"))
	case planning.CodeMealNotHearty:
		return fmt.Sprintf("%s is meant to be filling (at least %.0f kcal) but holds %.0f kcal.", where, n("min_kcal"), n("kcal"))
	case planning.CodeWeekMeatAboveDGE:
		if d := n("days"); d > 0 && d < 7 {
			return fmt.Sprintf("The week's meat is %.0f g; the limit for the %.0f days you are home is %.0f g (300 g a week).", n("meat_g"), d, n("max_g"))
		}
		return fmt.Sprintf("The week's meat is %.0f g; the limit is %.0f g.", n("meat_g"), n("max_g"))
	case planning.CodeWeekKcalBelowBand, planning.CodeWeekKcalAboveBand:
		return fmt.Sprintf("The week's average is %.0f kcal a day, %.0f%% of the target; the week's range is %.0f-%.0f%%.", n("avg_kcal"), n("pct"), n("band_low_pct"), n("band_high_pct"))
	case planning.CodeWeekProteinLow:
		return fmt.Sprintf("The week's average protein is %.0f g a day; the target is %.0f g.", n("avg_protein_g"), n("target_protein_g"))
	case planning.CodeWeekCarbsRange, planning.CodeWeekFatRange:
		return fmt.Sprintf("%s: %.0f%% of the week's energy; the range is %.0f-%.0f%%.", cap1(planning.PlainText(is.Code).Label), n("pct"), n("range_low_pct"), n("range_high_pct"))
	case planning.CodeDishRepeats:
		return fmt.Sprintf("%s is the main dish of %.0f meals; the limit is %.0f.", food.Quote(fmt.Sprint(is.Params["name"])), n("meals"), n("max"))
	case planning.CodeMeatDaysExceeded:
		return fmt.Sprintf("%.0f days of the week have meat; the limit is %.0f.", n("days"), n("max"))
	case planning.CodeAddOnExcess:
		why := map[string]string{
			"light_meal": "added to a light meal", "duplicates_main": "already in the main", "over_cap": fmt.Sprintf("over the usual serving of %.0f g", n("max_grams")),
			"day_limit": "more sides that day than usual", "repeated": "the same side on too many meals",
		}[fmt.Sprint(is.Params["reason"])]
		return fmt.Sprintf("%s: %s %.0f g is %s.", where, food.Quote(fmt.Sprint(is.Params["name"])), n("grams"), why)
	}
	switch is.Code {
	case planning.CodeVarietyRepeat, planning.CodeVarietyRelaxed, planning.CodeVarietyRepeatWeek:
		if dish := issueDish(is, st); dish != "" {
			if is.Code == planning.CodeVarietyRepeatWeek {
				if meals := sessionMealLabels(st, is); len(meals) > 1 {
					return fmt.Sprintf("%s is cooked more than once this week: it is eaten at %s.", dish, strings.Join(meals, ", "))
				}
				return fmt.Sprintf("%s: %s is cooked more than once this week.", where, dish)
			}
			return fmt.Sprintf("%s: %s was cooked or eaten too recently before.", where, dish)
		}
	}
	return where + ": " + planning.PlainText(is.Code).Message
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

func cap1(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// reportOf reads the plan after the ops (inside the unit of work, so a check
// still sees them) and builds the report. A state that cannot be read leaves
// a report that says so, never a guess.
func reportOf(ctx context.Context, svc Services, w window.Window, beforeSt planning.State, beforeErr error, before, after planning.Evaluation, applied []map[string]any) *changeReport {
	if beforeErr != nil {
		return &changeReport{NotCovered: []string{"the plan before the change could not be read"}}
	}
	afterSt, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return &changeReport{NotCovered: []string{"the plan after the change could not be read"}}
	}
	r := buildReport(reportInput{beforeSt: beforeSt, afterSt: afterSt, before: before, after: after, applied: applied})
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
		r := buildReport(reportInput{beforeSt: beforeSt, afterSt: afterSt, before: beforeEv, after: afterEv})
		if undo {
			// An undo restores the plan exactly as it was before the undone
			// change: every warning open now existed then, so none is new or
			// worse; they are all earlier ones.
			r.NewWarnings, r.Worse, r.EarlierWarnings = nil, nil, len(afterEv.Issues)
			r.Note = "Undone: the plan is exactly as it was before that change, and the warnings open now were open then. Say what is back; list no warning."
			for i := range r.Days {
				r.Days[i].Status = ""
			}
			r.Tell = buildTell(reportInput{beforeSt: beforeSt, afterSt: afterSt, before: beforeEv, after: afterEv}, r)
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
