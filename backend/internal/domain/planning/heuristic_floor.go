package planning

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

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
	for _, is := range Evaluate(s, t).Issues {
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
			h.liftDay(dk, floor, true)
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
			h.liftDay(dk, limit, false)
		}
	}
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
		h.liftDay(*worst, worst.Kcal+min(worst.Gap, gap), false)
	}
}

// dayAccounts is the account of the days S2 and S4 judge (complete, with
// nutrition data and some kcal) over the proposal as planned so far.
func (h *planner) dayAccounts() (*index, []DayKcal) {
	y := newIndex(Merge(h.s, h.in.Target, h.prop), h.in.Target)
	days := slices.DeleteFunc(y.dayAccounts(y.days), func(dk DayKcal) bool {
		return !dk.Complete || dk.Away || dk.NutritionMissing || dk.Kcal <= 0
	})
	return y, days
}

// liftDay raises a day from its kcal to limit: its growable plates, then
// verified no-cook foods.
func (h *planner) liftDay(dk DayKcal, limit float64, roundUp bool) {
	h.topUpDay(dk.Date, limit-dk.Kcal, roundUp)
	for h.addNoCook(dk.Date, false, limit) {
	}
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
		for range maxProteinAdds {
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
	if (protein && adds.protein >= maxProteinAdds) || (!protein && adds.filler >= maxFillerAdds) {
		return false
	}
	merged := Merge(h.s, h.in.Target, h.prop)
	y := newIndex(merged, h.in.Target)
	var dk *DayKcal
	for _, a := range y.dayAccounts([]time.Time{date}) {
		dk = &a
	}
	if dk == nil || !dk.Complete || dk.Away || dk.NutritionMissing || dk.Kcal <= 0 {
		return false
	}
	ev := Evaluate(merged, h.in.Target)
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
	if protein {
		// Week band: the add must not push the running average above it.
		if w := ev.Week; w != nil && w.TargetKcal > 0 {
			cands = slices.DeleteFunc(cands, func(c optCand) bool {
				return (w.AvgKcal+c.kcal/float64(w.Days))/w.TargetKcal > y.pol.WeekHigh+1e-9
			})
		}
	}
	slices.SortStableFunc(cands, func(a, b optCand) int {
		if protein {
			// Staying at or under the day's target first, then the most
			// protein, then the best fit.
			over := func(c optCand) int {
				if dk.Kcal+c.kcal > dk.Adjusted+1e-9 {
					return 1
				}
				return 0
			}
			return cmp.Or(cmp.Compare(over(a), over(b)), cmp.Compare(b.protein, a.protein), cmp.Compare(math.Abs(need-a.kcal), math.Abs(need-b.kcal)))
		}
		// Sweets are out and the protein pass has its own add, so a filler
		// is the food that fits the kcal gap best.
		return cmp.Compare(math.Abs(need-a.kcal), math.Abs(need-b.kcal))
	})
	for _, c := range cands {
		if _, ok := y.verify(c, h.in.Target, base, d, band, protein); !ok {
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
			c.plate = pi
			out = append(out, c)
		}
	}
	return out
}

// topUpDay adds need kcal per eater to the day's growable components.
func (h *planner) topUpDay(date time.Time, need float64, roundUp bool) {
	sessions := map[string]int{}
	for i, ss := range h.prop.Sessions {
		sessions[ss.Key] = i
	}
	// growable reports whether a component eats a proposed session within
	// its keep days (not a frozen link), and the session's index.
	growable := func(p ProposedPlate, c ProposedComponent) (int, bool) {
		i, ok := sessions[c.SessionKey]
		if c.SessionKey == "" || !ok || c.Portions <= 0 || h.s.lightMeal(p.Date, p.SlotID) {
			return 0, false
		}
		ss := h.prop.Sessions[i]
		f, known := h.s.Foods[c.FoodID]
		if !known {
			return 0, false
		}
		last := ss.CookDate.AddDate(0, 0, h.x.keepDays(c.FoodID))
		return i, !p.Date.After(last) && f.Macros.Kcal > 0 && (f.Role == food.RoleMain || f.Role == food.RoleSideStarch)
	}
	var raisable float64
	for _, p := range h.prop.Plates {
		if dayOf(p.Date) != dayOf(date) {
			continue
		}
		for _, c := range p.Components {
			if _, ok := growable(p, c); ok {
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
			if _, seen := h.topUpBase[key]; !seen {
				h.topUpBase[key] = c.Portions
			}
			raised = max(min(raised, math.Floor(h.topUpBase[key]*maxTopUp*4+epsilon)/4, MaxDishPortions), c.Portions)
			h.prop.Sessions[si].Portions += eaters * (raised - c.Portions)
			c.Portions = raised
		}
	}
}
