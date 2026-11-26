package planning

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// proteinNote is the run-notes line of the days left under their protein
// target after the gap closing ("" when none is, or no g/kg target is
// active): no verified food could lift them.
func proteinNote(s State, t Target) string {
	if PolicyOf(s.Profile).ProteinGPerKg <= 0 {
		return ""
	}
	var parts []string
	for _, is := range evaluateLean(s, t).Issues {
		if is.Code != CodeProteinLow || is.Date == "" {
			continue
		}
		day, _ := time.Parse(time.DateOnly, is.Date)
		g, _ := is.Params["protein_g"].(float64)
		target, _ := is.Params["target_g"].(float64)
		parts = append(parts, fmt.Sprintf("%s %.0f of %.0f g", day.Weekday().String()[:3], g, target))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Protein below target (no suitable food fit these days): " + strings.Join(parts, ", ")
}

// maxTopUp caps how far the floor pass raises a component's portions: 1.5
// portions of a recipe is still a plate.
const maxTopUp = 1.5

// topUpFloors lifts the proposal's complete days in four steps, each growing
// plates first and then adding verified no-cook foods (S2's candidates):
//
//  1. every day under H9's floor goes up to it, rounding portions up (a hard
//     rule);
//  2. protein gap closing (closeProteinGaps) when the profile has a g/kg
//     target, so the protein foods are not used up by plain kcal fillers;
//  3. every day under its band's low end (the S2 band of its scaled day
//     target, 85 % of it on a light day) goes up to it, while the week
//     stays under its target;
//  4. while the week average is under the week's target, the days furthest
//     under their target go up toward it, never by more than the week still
//     lacks (portions round down, so a day does not overshoot), so the week
//     lands in the S4 band without pushing days past the day band.
//
// The heuristic picks recipes by closeness to a meal's kcal share and never
// counts a day's total, and library mains average well under a third of a
// normal target, so a day of small recipes (or a routine breakfast plus one
// dinner) falls short. Only mains and carb sides that eat a proposed session
// within its keep days grow (their session grows by the same amount, so H3
// holds), up to 1.5x; other sides, frozen links, fixed sessions and light
// meals stay. A no-cook add must bring the day closer to its target and
// never past the band top. A day that cannot reach the band stays as it is;
// the checker reports it (H9 below the floor) and the run notes say so
// (kcalNote, floorNote).
func (h *planner) topUpFloors() {
	k := h.s.Profile.KcalTarget
	if k == nil || *k <= 0 || len(h.prop.Plates) == 0 {
		return
	}
	h.topUpBase = map[[2]int]float64{}
	h.adds = map[int]*dayAdds{}
	y, days := h.dayAccounts()
	for _, dk := range days {
		if floor := dk.Floor(y.pol); dk.Kcal < floor {
			// The safety floor comes before how the plate looks: any verified food.
			pair := h.s.PairAddOns
			h.s.PairAddOns, h.floorLift = false, true
			h.liftDay(dk, floor, true, false)
			h.s.PairAddOns, h.floorLift = pair, false
		}
	}
	h.closeProteinGaps()
	_, days = h.dayAccounts()
	for _, d := range days {
		// The week outweighs a day: no lift takes the week past its target.
		_, cur := h.dayAccounts()
		dk, room := d, 0.0
		for _, c := range cur {
			room += c.Gap
			if dayOf(c.Date) == dayOf(d.Date) {
				dk = c
			}
		}
		if limit := min(dk.Adjusted*y.pol.DayLow, dk.Kcal+room); limit > dk.Kcal+epsilon {
			h.liftDay(dk, limit, false, h.s.Settings.PlateFit)
		}
	}
	h.liftWeek(h.s.Settings.PlateFit)
	if h.s.Settings.PlateFit {
		h.liftWeek(false)
		h.nutritionFill()
	}
}

// nutritionFill is the second filler pass with plate fit on (PLATE-4: hard
// rules, then nutrition, then plate fit). The passes above add only fillers
// that make sense on their plate, under the plate's side cap; where a day
// still has protein under its target or kcal under its band, or the week is
// under its band, this pass lifts it the way the planner did before plate
// fit (the pre-R1 ranking, no filler or side-cap filter; every hard rule
// still holds), only up to what the target needs. In a heuristic run each
// no-cook add it makes is logged as a nutrition_fill choice.
func (h *planner) nutritionFill() {
	h.s.Settings.PlateFit, h.nutritionPass = false, true
	defer func() { h.s.Settings.PlateFit, h.nutritionPass = true, false }()
	// The culinary passes' add counts do not hold the nutrition pass back;
	// its own caps are the pre-R1 ones. Plates that took an add stay last.
	for d, a := range h.adds {
		h.adds[d] = &dayAdds{used: a.used}
	}
	h.closeProteinGaps()
	y, days := h.dayAccounts()
	for _, d := range days {
		if dk := h.dayNow(d.Date); dk != nil && dk.Kcal < dk.Adjusted*y.pol.DayLow-epsilon {
			h.liftDay(*dk, dk.Adjusted*y.pol.DayLow, false, false)
		}
	}
	_, days = h.dayAccounts()
	kcal, target := 0.0, 0.0
	for _, dk := range days {
		kcal, target = kcal+dk.Kcal, target+dk.Adjusted
	}
	if kcal < target*y.pol.WeekLow-epsilon {
		h.liftWeek(false)
	}
}

// fatPctAfter is the week's fat share of energy, in %, after an add of kcal
// with fatKcal from fat.
func fatPctAfter(w WeekEval, fatKcal, kcal float64) float64 {
	total := w.AvgKcal * float64(w.Days)
	return (w.FatPct*total/100 + fatKcal) / (total + kcal) * 100
}

// dayNow is date's day account over the proposal as planned so far.
func (h *planner) dayNow(date time.Time) *DayKcal {
	_, days := h.dayAccounts()
	for i := range days {
		if dayOf(days[i].Date) == dayOf(date) {
			return &days[i]
		}
	}
	return nil
}

// liftWeek lifts the days furthest under their target while the week is
// under its own, each day once; with leanOnly, see liftDay.
func (h *planner) liftWeek(leanOnly bool) {
	_, days := h.dayAccounts()
	tried := map[int]bool{}
	for range days {
		_, days = h.dayAccounts()
		gap := 0.0
		var worst *DayKcal
		for i, dk := range days {
			gap += dk.Gap
			if dk.Gap > 0 && (worst == nil || dk.Gap > worst.Gap) && !tried[dayOf(dk.Date)] {
				worst = &days[i]
			}
		}
		if gap <= 0 || worst == nil {
			break
		}
		tried[dayOf(worst.Date)] = true
		h.liftDay(*worst, worst.Kcal+min(worst.Gap, gap), false, leanOnly)
	}
}

// dayAccounts is the account of the days S2 and S4 judge (complete, with
// nutrition data and some kcal) over the proposal as planned so far.
func (h *planner) dayAccounts() (*index, []DayKcal) {
	y := newIndex(Merge(h.s, h.in.Target, h.prop), h.in.Target)
	days := slices.DeleteFunc(y.dayAccounts(y.days), func(dk DayKcal) bool {
		return !dk.Complete || dk.NutritionMissing || dk.Kcal <= 0
	})
	return y, days
}

// liftDay raises a day from its kcal to limit: its growable plates, then
// verified no-cook foods. With plate fit on, the top-up rounds up to the
// quarter portion: a starch main without a second starch carries the
// plate's energy, not a filler of cheese on the remainder. With leanOnly, a
// fatty dish (fat share above the week's range) does not grow: lean dishes,
// sides and fillers come first and the fatty dishes only lift what the week
// still lacks after them (PLATE-4: nutrition before fit).
func (h *planner) liftDay(dk DayKcal, limit float64, roundUp, leanOnly bool) {
	if h.s.Settings.PlateFit && !h.floorLift {
		// Plate fit on: a side recipe carries the day's energy before the
		// portions grow (a varied plate, not a 1.5x main); the safety floor
		// lift grows first.
		if h.sided == nil {
			h.sided = map[int]bool{}
		}
		h.addSide(dk.Date, limit)
		if now := h.dayNow(dk.Date); now != nil {
			dk = *now
		}
	}
	h.topUpDay(dk.Date, limit-dk.Kcal, roundUp || h.s.Settings.PlateFit, leanOnly)
	h.growLight(dk.Date, limit)
	if h.balance || h.s.Settings.PlateFit {
		// Plate fit on: a side recipe of a batch the plan cooks anyway
		// carries energy better than a filler that does not belong there.
		if h.sided == nil {
			h.sided = map[int]bool{}
		}
		h.addSide(dk.Date, limit)
	}
	for h.addNoCook(dk.Date, false, limit) {
	}
}

// growLight raises the batch-eaten mains and starches of a short day's light
// meals by quarter portions while the meal stays within its light cap and
// the day under limit: topUpDay never grows a light meal, yet a light lunch
// under its cap has room the day needs (V73, g2-planner-04: light lunches at
// 482-570 kcal under a 737 cap left three days under their band). The batch
// cooks more where it already cooks: no new cooking time.
func (h *planner) growLight(date time.Time, limit float64) {
	for range 8 { // at most two portions over all of the day's light meals
		ev := evaluateLean(Merge(h.s, h.in.Target, h.prop), h.in.Target)
		day := slices.IndexFunc(ev.Days, func(d DayEval) bool { return d.Date == date.Format(time.DateOnly) })
		if day < 0 || ev.Days[day].Kcal >= limit-epsilon {
			return
		}
		if !h.growLightStep(date, ev, limit-ev.Days[day].Kcal) {
			return
		}
	}
}

// growLightStep grows one component of a light meal of date by a quarter
// portion when that keeps the meal within its cap and the step within need.
func (h *planner) growLightStep(date time.Time, ev Evaluation, need float64) bool {
	sessions := map[string]int{}
	for i, ss := range h.prop.Sessions {
		sessions[ss.Key] = i
	}
	for pi := range h.prop.Plates {
		p := &h.prop.Plates[pi]
		if dayOf(p.Date) != dayOf(date) || !h.s.lightMeal(p.Date, p.SlotID) {
			continue
		}
		pe := slices.IndexFunc(ev.Plates, func(e PlateEval) bool {
			return e.Date == p.Date.Format(time.DateOnly) && e.SlotID == p.SlotID
		})
		if pe < 0 || ev.Plates[pe].MaxKcal <= 0 {
			continue
		}
		room := ev.Plates[pe].MaxKcal - ev.Plates[pe].Kcal
		for ci := range p.Components {
			c := &p.Components[ci]
			si, ok := sessions[c.SessionKey]
			f, known := h.s.Foods[c.FoodID]
			if !ok || !known || c.Portions <= 0 || c.Portions+roundPortions > MaxDishPortions+epsilon ||
				f.Macros.Kcal <= 0 || (f.Role != food.RoleMain && f.Role != food.RoleSideStarch) {
				continue
			}
			step := f.Macros.Kcal * roundPortions
			if step > room+epsilon || step > need+f.Macros.Kcal*roundPortions/2 {
				continue
			}
			c.Portions += roundPortions
			h.prop.Sessions[si].Portions += p.eaters(h.s) * roundPortions
			return true
		}
	}
	return false
}

// The no-cook adds of one day over every pass (floor, protein, band, week):
// at most maxFillerAdds plain fillers and maxProteinAdds protein adds, each
// at most maxAddFillerGrams and fillerDayShare of the day's target, on
// different plates before any plate takes a second. Past that the day stays
// short and kcalNote says so.
const (
	maxFillerAdds     = 2
	maxProteinAdds    = 1
	maxAddFillerGrams = 200
	fillerDayShare    = 0.2
)

// proteinAdds is the day's protein add limit: plate fit on, one per meal
// (maxPlateFitProteinAdds; the plate's side cap keeps it from clutter and
// the day's add-on rule still holds), else maxProteinAdds.
func (h *planner) proteinAdds() int {
	if h.s.Settings.PlateFit {
		return maxPlateFitProteinAdds
	}
	return maxProteinAdds
}

// maxPlateFitProteinAdds: with plate fit on, a protein filler must fit its
// dish, so the day's protein comes from two small fitting adds (feta on
// the bowl, yogurt on the curry) rather than one that fits nowhere.
const maxPlateFitProteinAdds = 2

// dayAdds counts a day's no-cook adds and the plates (slots) they went to.
type dayAdds struct {
	filler, protein int
	used            map[int64]bool
}

func (h *planner) addsOf(date time.Time) *dayAdds {
	a := h.adds[dayOf(date)]
	if a == nil {
		a = &dayAdds{used: map[int64]bool{}}
		h.adds[dayOf(date)] = a
	}
	return a
}

// capAdd shrinks an add to the filler caps (grams, kcal) and reports false
// when even the smallest size is over them.
func (h *planner) capAdd(c optCand, maxKcal float64) (optCand, bool) {
	f := h.s.Foods[c.opt.FoodID]
	if f.Macros.Kcal <= 0 {
		return c, false
	}
	switch {
	case c.opt.Grams != nil:
		limit := addCapKcal(f, maxKcal)
		g := min(*c.opt.Grams, limit)
		if g < min(minAddGrams, limit) {
			return c, false
		}
		c.opt.Grams, c.add.Grams = &g, &g
		c.kcal, c.protein = f.Macros.Kcal*g/100, f.Macros.Protein*g/100
	case c.opt.Portions != nil:
		p := min(*c.opt.Portions, math.Floor(maxKcal/f.Macros.Kcal/roundPortions)*roundPortions)
		if p < minAddPortions {
			return c, false
		}
		c.opt.Portions, c.add.Portions = &p, &p
		c.kcal, c.protein = f.Macros.Kcal*p, f.Macros.Protein*p
	}
	return c, true
}

// proteinBucket groups protein adds of about the same protein (steps of
// 25 %), so the leaner one wins within a group and a vegetable or a yogurt
// never beats a cheese that brings more protein.
const proteinBucket = 1.25

// closeProteinGaps lifts every complete day under its g/kg protein target
// (100 %, so the week average can reach it too; the checker's day floor is
// 90 %) with up to maxProteinAdds verified no-cook foods, the most protein
// first (skyr, quark, tofu, eggs, cheese, legumes when the library has them),
// at most one per plate and spread over the day's plates (lunch and dinner)
// before any stacks. The candidates and the re-check are S3's options: diet
// type, the day's kcal band, the week band and every hard rule hold. A day
// no food can lift stays as it is; the checker reports it (S3).
func (h *planner) closeProteinGaps() {
	k := h.s.Profile.KcalTarget
	if k == nil || *k <= 0 || len(h.prop.Plates) == 0 || PolicyOf(h.s.Profile).ProteinGPerKg <= 0 {
		return
	}
	y := newIndex(Merge(h.s, h.in.Target, h.prop), h.in.Target)
	for _, dk := range y.dayAccounts(y.days) {
		for range h.proteinAdds() {
			if !h.addNoCook(dk.Date, true, 0) {
				break
			}
		}
	}
}

// addNoCook adds the best verified no-cook food to a day under limit kcal
// (protein: a day still under its protein target; the candidates and the
// re-check of S2's and S3's options) and reports whether it did. Raising a
// component is left to topUpDay: it would have to move the component's
// session too. An add goes on a plate with no add of the day yet (the day's
// last plate first; every plate once, then they stack), never repeats a food
// of the day and never is a sweet; the day's caps hold over all passes.
func (h *planner) addNoCook(date time.Time, protein bool, limit float64) bool {
	adds := h.addsOf(date)
	if (protein && adds.protein >= h.proteinAdds()) || (!protein && adds.filler >= maxFillerAdds) {
		return false
	}
	merged := Merge(h.s, h.in.Target, h.prop)
	y := newIndex(merged, h.in.Target)
	var dk *DayKcal
	for _, a := range y.dayAccounts([]time.Time{date}) {
		dk = &a
	}
	if dk == nil || !dk.Complete || dk.NutritionMissing || dk.Kcal <= 0 {
		return false
	}
	ev := evaluateLean(merged, h.in.Target)
	var d DayEval
	for _, e := range ev.Days {
		if e.Date == date.Format(time.DateOnly) {
			d = e
		}
	}
	need, band := limit-dk.Kcal, dk.Adjusted
	if protein {
		if d.ProteinTargetG <= 0 || d.ProteinG >= d.ProteinTargetG {
			return false
		}
		// Room up to the day band top; the sizes aim at the kcal gap.
		band = dk.Adjusted
		need = dk.Adjusted - dk.Kcal
		if need <= 0 {
			need = dk.Adjusted*y.pol.DayHigh - dk.Kcal
		}
	}
	if need <= 0 {
		return false
	}
	base := hardKeys(ev.Issues)
	cands := slices.DeleteFunc(y.candidates(date, need), func(c optCand) bool {
		return c.comp >= 0 || h.s.Foods[c.opt.FoodID].Sweet
	})
	for i := 0; i < len(cands); {
		var ok bool
		if cands[i], ok = h.capAdd(cands[i], dk.Adjusted*fillerDayShare); ok {
			i++
		} else {
			cands = slices.Delete(cands, i, i+1)
		}
	}
	cands = spreadAdds(y, merged, date, cands, adds.used)
	// A filler must make sense on its plate (fillerFits: no second starch, no
	// muesli or fruit beside a dinner, no second protein). The safety floor
	// lift may use any verified food, the fitting ones first. Plate fit off: no
	// filter.
	misfit := func(c optCand) bool {
		p := merged.Plates[c.plate]
		return !merged.fillerFits(p, merged.Foods[c.opt.FoodID], slotName(merged, p.SlotID))
	}
	// A food the household would rather not get beside a meal
	// (State.RatherNot) is no filler; only the floor lift may use it, after
	// every other food.
	ratherNot := func(c optCand) bool { return h.s.RatherNot[c.opt.FoodID] }
	if !h.floorLift {
		cands = slices.DeleteFunc(cands, func(c optCand) bool { return misfit(c) || ratherNot(c) })
	}
	if protein {
		// Week band: the add must not push the running average above it.
		if w := ev.Week; w != nil && w.TargetKcal > 0 {
			cands = slices.DeleteFunc(cands, func(c optCand) bool {
				return (w.AvgKcal+c.kcal/float64(w.Days))/w.TargetKcal > y.pol.WeekHigh+1e-9
			})
		}
	}
	raised := map[[2]int64]int{} // plate fit per (plate, food), when on
	served := map[int64]int{}    // plates of the window per food, when on
	if h.s.Settings.PlateFit && !protein {
		for _, p := range merged.Plates {
			if merged.Window.Contains(p.Date) {
				for _, c := range p.Components {
					served[c.FoodID]++
				}
			}
		}
		for _, c := range cands {
			if _, _, r := merged.plateFitOf(merged.Plates[c.plate], merged.Foods[c.opt.FoodID], false); r > 0 {
				raised[[2]int64{int64(c.plate), c.opt.FoodID}] = r
			}
		}
	}
	// With plate fit on, a filler serves the week before the plate's fit
	// (PLATE-4): a legume meal while the week has none, then, while its carb
	// share is below range, the most carb kcal (bread, not cheese).
	legume, carbs := func(optCand) float64 { return 0 }, func(optCand) float64 { return 0 }
	fat := carbs
	mult := func(c optCand) float64 {
		if f := h.s.Foods[c.opt.FoodID]; f.Macros.Kcal > 0 {
			return c.kcal / f.Macros.Kcal
		}
		return 0
	}
	if w := ev.Week; h.s.Settings.PlateFit && !h.floorLift && w != nil && w.Days > 0 {
		// A fitting add (cheese on the pasta) must not take the week's fat
		// share above its range or further above it: nutrition before fit
		// (PLATE-4). The nutrition pass (nutritionFill) covers what is left.
		cands = slices.DeleteFunc(cands, func(c optCand) bool {
			after := fatPctAfter(*w, h.s.Foods[c.opt.FoodID].Macros.Fat*9*mult(c), c.kcal)
			return after > y.pol.FatHigh+epsilon && after > w.FatPct+epsilon
		})
	}
	if h.s.Settings.PlateFit && protein {
		// The protein add that fits a plate is often a dairy (tofu and fish
		// rarely fit): a lean one (skyr, quark) before a full-fat one.
		fat = func(c optCand) float64 {
			return excessFat(h.s.Foods[c.opt.FoodID].Macros, mult(c), y.pol.FatHigh)
		}
	}
	if w := ev.Week; h.s.Settings.PlateFit && !protein && w != nil {
		if w.LegumeMeals == 0 {
			legume = func(c optCand) float64 {
				return -b2f(h.s.Foods[c.opt.FoodID].Groups.LegumeG*mult(c) >= minMealG)
			}
		}
		if w.FatPct > y.pol.FatHigh {
			fat = func(c optCand) float64 {
				return excessFat(h.s.Foods[c.opt.FoodID].Macros, mult(c), y.pol.FatHigh)
			}
		}
		switch {
		case w.CarbsPct < y.pol.CarbsLow:
			carbs = func(c optCand) float64 {
				return -math.Round(h.s.Foods[c.opt.FoodID].Macros.Carbs * 4 * mult(c) / 10)
			}
		case w.CarbsPct > y.pol.CarbsHigh:
			// Above range (the fitting fillers are mostly bread and vegetables
			// once fruit, muesli and a second protein are out): the fewest.
			carbs = func(c optCand) float64 {
				return math.Round(h.s.Foods[c.opt.FoodID].Macros.Carbs * 4 * mult(c) / 10)
			}
		}
	}
	slices.SortStableFunc(cands, func(a, b optCand) int {
		if protein {
			// Staying at or under the day's target first, then the most
			// protein (plate fit on: by proteinBucket, the leaner add within
			// one), then the best fit.
			over := func(c optCand) int {
				if dk.Kcal+c.kcal > dk.Adjusted+1e-9 {
					return 1
				}
				return 0
			}
			bucket := func(c optCand) float64 { return math.Round(math.Log(max(c.protein, 1)) / math.Log(proteinBucket)) }
			if !h.s.Settings.PlateFit {
				bucket = func(optCand) float64 { return 0 }
			}
			return cmp.Or(cmp.Compare(over(a), over(b)), cmp.Compare(bucket(b), bucket(a)), cmp.Compare(fat(a), fat(b)), cmp.Compare(b.protein, a.protein), cmp.Compare(math.Abs(need-a.kcal), math.Abs(need-b.kcal)))
		}
		// Sweets are out and the protein pass has its own add, so a filler
		// is the food that fits the kcal gap best. A model's plan gets, within 25 kcal
		// of each other, a vegetable or salad before bread before the rest (sideFitRank).
		// With plate fit on, within kcalBucket of each other the week's
		// needs (legume, carbs), then the food the week serves least (its
		// rotation), then the plate with fewer raised gaps come first
		// (PLATE-4: nutrition before fit).
		if h.s.Settings.PlateFit {
			ba, bb := math.Round(math.Abs(need-a.kcal)/kcalBucket), math.Round(math.Abs(need-b.kcal)/kcalBucket)
			ra, rb := raised[[2]int64{int64(a.plate), a.opt.FoodID}], raised[[2]int64{int64(b.plate), b.opt.FoodID}]
			if c := cmp.Or(cmp.Compare(ba, bb), cmp.Compare(legume(a), legume(b)), cmp.Compare(fat(a), fat(b)), cmp.Compare(carbs(a), carbs(b)),
				cmp.Compare(served[a.opt.FoodID], served[b.opt.FoodID]), cmp.Compare(ra, rb)); c != 0 {
				return c
			}
		}
		if h.balance {
			if ba, bb := math.Round(math.Abs(need-a.kcal)/25), math.Round(math.Abs(need-b.kcal)/25); ba != bb || sideFitRank(h.s.Foods[a.opt.FoodID]) != sideFitRank(h.s.Foods[b.opt.FoodID]) {
				return cmp.Or(cmp.Compare(ba, bb), cmp.Compare(sideFitRank(h.s.Foods[a.opt.FoodID]), sideFitRank(h.s.Foods[b.opt.FoodID])))
			}
		}
		return cmp.Compare(math.Abs(need-a.kcal), math.Abs(need-b.kcal))
	})
	if h.floorLift {
		// A fitting food first only when it reaches the floor: the day's few
		// adds must not go to a cucumber that leaves the day under it.
		// Among those, the one closest to the day's target: the floor
		// lift's add takes a plate's last place, so it serves the week too.
		short := func(c optCand) bool { return misfit(c) || c.kcal < need-epsilon }
		gap := math.Max(dk.Adjusted-dk.Kcal, need)
		slices.SortStableFunc(cands, func(a, b optCand) int {
			if c := cmp.Compare(b2f(ratherNot(a)), b2f(ratherNot(b))); c != 0 {
				return c
			}
			if c := cmp.Compare(b2f(short(a)), b2f(short(b))); c != 0 || short(a) {
				return c
			}
			return cmp.Compare(math.Abs(gap-a.kcal), math.Abs(gap-b.kcal))
		})
	}
	if h.floorLift || h.nutritionPass {
		// The passes that may break a plate's fit (plate fit on) still take a
		// second protein (tofu beside a lentil curry) only after every other
		// verified food; the floor lift's refused food stays last of all.
		on := h.nutritionPass || merged.Settings.PlateFit
		second := func(c optCand) bool {
			return on && merged.secondProteinAdd(merged.Plates[c.plate], merged.Foods[c.opt.FoodID])
		}
		refused := func(c optCand) bool { return h.floorLift && ratherNot(c) }
		slices.SortStableFunc(cands, func(a, b optCand) int {
			return cmp.Or(cmp.Compare(b2f(refused(a)), b2f(refused(b))), cmp.Compare(b2f(second(a)), b2f(second(b))))
		})
	}
	for _, c := range cands {
		o, ok := y.verify(c, h.in.Target, base, d, band, protein)
		if !ok || (h.balance && o.CookOccasions > len(y.occasions[dayOf(date)])) {
			// A model's plan keeps its cooking days: an add never cooks anew.
			continue
		}
		at := merged.Plates[c.plate]
		for i := range h.prop.Plates {
			p := &h.prop.Plates[i]
			if dayOf(p.Date) == dayOf(at.Date) && p.SlotID == at.SlotID {
				pc := ProposedComponent{FoodID: c.opt.FoodID}
				if c.opt.Portions != nil {
					pc.Portions = *c.opt.Portions
				}
				if c.opt.Grams != nil {
					pc.Grams = *c.opt.Grams
				}
				p.Components = append(p.Components, pc)
				adds.used[at.SlotID] = true
				if h.nutritionPass {
					h.choices = append(h.choices, heuristicChoice{Date: at.Date.Format(time.DateOnly), SlotID: at.SlotID, Foods: []int64{c.opt.FoodID}, Reason: "nutrition_fill"})
				}
				if protein {
					adds.protein++
				} else {
					adds.filler++
				}
				return true
			}
		}
	}
	return false
}

// spreadAdds puts every add candidate on each of the day's editable plates
// whose slot has no add yet (the day's last plate first; when every plate
// has one, on all of them) and drops foods the day already holds on any
// plate.
func spreadAdds(y *index, merged State, date time.Time, cands []optCand, used map[int64]bool) []optCand {
	var plates []int
	for _, pi := range y.editablePlates(date) {
		if !used[merged.Plates[pi].SlotID] {
			plates = append(plates, pi)
		}
	}
	if len(plates) == 0 {
		plates = y.editablePlates(date)
	}
	slices.SortStableFunc(plates, func(a, b int) int {
		return cmp.Compare(merged.SlotOrder[merged.Plates[b].SlotID], merged.SlotOrder[merged.Plates[a].SlotID])
	})
	held := map[int64]bool{}
	for _, p := range merged.Plates {
		if dayOf(p.Date) == dayOf(date) {
			for _, c := range p.Components {
				held[c.FoodID] = true
			}
		}
	}
	var out []optCand
	for _, pi := range plates {
		for _, c := range cands {
			if held[c.opt.FoodID] {
				continue
			}
			// The candidates were checked against the day's last plate: each plate has its own fit.
			if f := merged.Foods[c.opt.FoodID]; !f.Composed() && !merged.addOnFits(merged.Plates[pi], f, slotName(merged, merged.Plates[pi].SlotID)) {
				continue
			}
			c.plate = pi
			out = append(out, c)
		}
	}
	return out
}

// topUpDay adds need kcal per eater to the day's growable components,
// rounded up to the quarter portion when roundUp, down otherwise. With
// leanOnly, only the dishes whose fat share is within the week's range grow.
func (h *planner) topUpDay(date time.Time, need float64, roundUp, leanOnly bool) {
	full := h.fullMeals(date)
	if len(full) == 0 {
		h.topUpDaySkip(date, need, roundUp, leanOnly, nil)
		return
	}
	// Plate fit on: a meal at its share of the day grows only for what the
	// meals under their share cannot carry (a balanced day, not one heavy
	// meal beside a light one; nutrition still first).
	before := h.dayNow(date)
	h.topUpDaySkip(date, need, roundUp, leanOnly, full)
	if after := h.dayNow(date); before != nil && after != nil {
		if rest := need - (after.Kcal - before.Kcal); rest > epsilon {
			h.topUpDaySkip(date, rest, roundUp, leanOnly, nil)
		}
	}
}

// topUpDaySkip is topUpDay without growing the plates of the skip slots.
func (h *planner) topUpDaySkip(date time.Time, need float64, roundUp, leanOnly bool, skip map[int64]bool) {
	meatFull := h.balance && h.meatFull()
	sessions := map[string]int{}
	for i, ss := range h.prop.Sessions {
		sessions[ss.Key] = i
	}
	// growable reports whether a component eats a proposed session within
	// its keep days (not a frozen link), and the session's index.
	growable := func(p ProposedPlate, c ProposedComponent) (int, bool) {
		i, ok := sessions[c.SessionKey]
		if h.balance && c.SessionKey == "" && c.SessionID == 0 && c.Portions > 0 && !h.s.lightMeal(p.Date, p.SlotID) {
			// A component cooked fresh for its own plate has no session to grow.
			f, known := h.s.Foods[c.FoodID]
			return -1, known && f.Macros.Kcal > 0 && (f.Role == food.RoleMain || f.Role == food.RoleSideStarch) && !(meatFull && f.Groups.MeatG > 0)
		}
		if c.SessionKey == "" || !ok || c.Portions <= 0 || h.s.lightMeal(p.Date, p.SlotID) {
			return 0, false
		}
		ss := h.prop.Sessions[i]
		f, known := h.s.Foods[c.FoodID]
		if !known {
			return 0, false
		}
		last := ss.CookDate.AddDate(0, 0, h.x.keepDays(c.FoodID))
		role := f.Role == food.RoleMain || f.Role == food.RoleSideStarch || (h.s.Settings.PlateFit && f.Role == food.RoleSideVeg)
		return i, !p.Date.After(last) && f.Macros.Kcal > 0 && role && !(meatFull && f.Groups.MeatG > 0)
	}
	// capOf is how far every pass together raises a component: maxTopUp of
	// its planned portions.
	capOf := func(pi, ci int, c ProposedComponent) float64 {
		base, seen := h.topUpBase[[2]int{pi, ci}]
		if !seen {
			base = c.Portions
		}
		return min(math.Floor(base*maxTopUp*4+epsilon)/4, MaxDishPortions)
	}
	if leanOnly {
		all := growable
		growable = func(p ProposedPlate, c ProposedComponent) (int, bool) {
			i, ok := all(p, c)
			m := h.s.Foods[c.FoodID].Macros
			return i, ok && m.Fat*9 <= m.Kcal*h.x.pol.FatHigh/100
		}
	}
	if len(skip) > 0 {
		inner := growable
		growable = func(p ProposedPlate, c ProposedComponent) (int, bool) {
			if skip[p.SlotID] {
				return 0, false
			}
			return inner(p, c)
		}
	}
	var raisable float64
	for pi, p := range h.prop.Plates {
		if dayOf(p.Date) != dayOf(date) {
			continue
		}
		for ci, c := range p.Components {
			// Plate fit on: a component at its cap takes no share of the need.
			if _, ok := growable(p, c); ok && (!h.s.Settings.PlateFit || c.Portions < capOf(pi, ci, c)-epsilon) {
				raisable += h.s.Foods[c.FoodID].Macros.Kcal * c.Portions
			}
		}
	}
	if raisable <= 0 {
		return
	}
	factor := math.Min(1+need/raisable, maxTopUp)
	for pi := range h.prop.Plates {
		p := &h.prop.Plates[pi]
		if dayOf(p.Date) != dayOf(date) {
			continue
		}
		eaters := p.eaters(h.s)
		for ci := range p.Components {
			c := &p.Components[ci]
			si, ok := growable(*p, *c)
			if !ok {
				continue
			}
			raised := math.Floor(c.Portions*factor*4+epsilon) / 4
			if roundUp {
				raised = math.Ceil(c.Portions*factor*4-epsilon) / 4
			}
			// Every pass together stays within maxTopUp of the portions the
			// plate was planned with.
			key := [2]int{pi, ci}
			top := capOf(pi, ci, *c)
			if _, seen := h.topUpBase[key]; !seen {
				h.topUpBase[key] = c.Portions
			}
			raised = max(min(raised, top), c.Portions)
			if si >= 0 {
				h.prop.Sessions[si].Portions += eaters * (raised - c.Portions)
			}
			c.Portions = raised
		}
	}
}

// fullMeals are the slots of date's proposed full meals (lunch, dinner; not
// light) whose kcal per eater reach their share of the day (kcalShare), when
// another full meal of the day is under its share; nil with plate fit off,
// during the safety floor lift, on a day under its protein target, or when
// no meal of the day is under.
func (h *planner) fullMeals(date time.Time) map[int64]bool {
	if !h.s.Settings.PlateFit || h.floorLift {
		return nil
	}
	ev := evaluateLean(Merge(h.s, h.in.Target, h.prop), h.in.Target)
	for _, d := range ev.Days {
		if d.Date == date.Format(time.DateOnly) && d.ProteinTargetG > 0 && d.ProteinG < d.ProteinTargetG {
			return nil // protein before balance: the protein-rich meal may grow
		}
	}
	full, under := map[int64]bool{}, false
	for _, p := range h.prop.Plates {
		if dayOf(p.Date) != dayOf(date) || h.x.mealOf(p.SlotID) != mealFull || h.s.lightMeal(p.Date, p.SlotID) {
			continue
		}
		c := plate.CellKey{Date: p.Date, SlotID: p.SlotID}
		share := h.kcalShare(c)
		i := slices.IndexFunc(ev.Plates, func(e PlateEval) bool { return e.Date == p.Date.Format(time.DateOnly) && e.SlotID == p.SlotID })
		if share <= 0 || i < 0 {
			continue
		}
		if ev.Plates[i].Kcal >= share {
			full[p.SlotID] = true
		} else {
			under = true
		}
	}
	if !under {
		return nil
	}
	return full
}
