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

// topUpFloors raises the portions of the proposal's plates on every complete
// day that holds fewer kcal than the policy's floor (H9), up to the floor
// itself (a light day above it stays light). The heuristic picks recipes by
// closeness to a meal's kcal share and never counts a day's total, so a day
// of small recipes (or a routine breakfast plus one dinner) would fall under
// the floor. Only mains and carb sides that eat a proposed session within
// its keep days grow (their session grows by the same amount, so H3 holds);
// other sides, frozen links and fixed sessions stay. A day still under the
// floor gets a verified no-cook food (S2's candidates). A day that cannot
// reach the floor that way stays as it is; the checker reports it (H9).
func (h *planner) topUpFloors() {
	k := h.s.Profile.KcalTarget
	if k == nil || *k <= 0 || len(h.prop.Plates) == 0 {
		return
	}
	merged := Merge(h.s, h.in.Target, h.prop)
	y := newIndex(merged, h.in.Target)
	for _, dk := range y.dayAccounts(y.days) {
		if !dk.Complete || dk.NutritionMissing || dk.Kcal <= 0 {
			continue
		}
		floor := y.pol.Floor(dk.Target, dk.FloorShare())
		if y.lightOf(dk.Date) == LightScopeMeal {
			// A light meal leaves the day's target as it was: the other
			// meals carry the rest, up to the band's low end.
			floor = max(floor, dk.Adjusted*y.pol.DayLow)
		}
		if dk.Kcal >= floor {
			continue
		}
		h.topUpDay(dk.Date, floor-dk.Kcal)
	}
	for _, dk := range y.dayAccounts(y.days) {
		for range maxFloorAdds {
			if !h.addNoCook(dk.Date, false, nil) {
				break
			}
		}
	}
	h.closeProteinGaps()
}

// maxProteinAdds bounds the protein foods added to one day, one per plate.
const maxProteinAdds = 2

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
		used := map[int64]bool{}
		for range maxProteinAdds {
			if !h.addNoCook(dk.Date, true, used) {
				break
			}
		}
	}
}

// maxFloorAdds bounds the no-cook foods added to one day.
const maxFloorAdds = 3

// addNoCook adds the best verified no-cook food to a still too light day
// (protein: a day still under its protein target; the candidates and the
// re-check of S2's and S3's options) and reports whether it did. Raising a
// component is left to topUpDay: it would have to move the component's
// session too. A protein add goes on a plate whose slot is not in used (the
// slot is marked), on the day's last plate first, and never repeats a food
// of the day.
func (h *planner) addNoCook(date time.Time, protein bool, used map[int64]bool) bool {
	merged := Merge(h.s, h.in.Target, h.prop)
	y := newIndex(merged, h.in.Target)
	var dk *DayKcal
	for _, a := range y.dayAccounts([]time.Time{date}) {
		dk = &a
	}
	if dk == nil || !dk.Complete || dk.NutritionMissing || dk.Kcal <= 0 {
		return false
	}
	ev := Evaluate(merged, h.in.Target)
	var d DayEval
	for _, e := range ev.Days {
		if e.Date == date.Format(time.DateOnly) {
			d = e
		}
	}
	need, band := y.pol.Floor(dk.Target, dk.FloorShare())-dk.Kcal, dk.Target
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
	cands := slices.DeleteFunc(y.candidates(date, need), func(c optCand) bool { return c.comp >= 0 })
	if protein {
		cands = spreadProteinAdds(y, merged, date, cands, used)
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
				if protein {
					used[at.SlotID] = true
				}
				return true
			}
		}
	}
	return false
}

// spreadProteinAdds puts every add candidate on each of the day's editable
// plates whose slot has no protein add yet (the day's last plate first) and
// drops foods the day already holds on any plate.
func spreadProteinAdds(y *index, merged State, date time.Time, cands []optCand, used map[int64]bool) []optCand {
	var plates []int
	for _, pi := range y.editablePlates(date) {
		if !used[merged.Plates[pi].SlotID] {
			plates = append(plates, pi)
		}
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
func (h *planner) topUpDay(date time.Time, need float64) {
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
			raised := math.Ceil(c.Portions*factor*4-epsilon) / 4
			h.prop.Sessions[si].Portions += eaters * (raised - c.Portions)
			c.Portions = raised
		}
	}
}
