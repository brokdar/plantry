package planning

import (
	"cmp"
	"math"
	"slices"
	"time"
)

// maxVaryKcal bounds how far a varied side may move its plate's kcal per
// eater: batch variation stays within nutrition (PLATE-6).
const maxVaryKcal = 100

// varyBatches gives the leftover plates of each proposed batch another side
// than the batch's cook plate (PLATE-6, chef.batch_variation): the repeated
// main stays, the side changes. A variant is verified like an alternative (no
// new issue or nutrition gap, no added cooking, the day held in its band),
// never clashes with the main (plate fit) and never repeats a side the batch
// already serves, and a side must make sense on the plate (addFits: no fruit
// or second protein beside the main). A leftover plate never gets a side that
// needs a new cook session, nor one the household would rather not get
// (State.RatherNot): keeping the batch's side is the better alternative.
// Among the variants: nutrition first (closest to the old side's kcal), then
// rotation (the side the week uses least), then plate fit (fewer raised
// gaps), the food id. A side another plate of the day serves is no variant. A plate with no variant keeps its side. Off, the proposal is
// returned as it is.
func varyBatches(base State, t Target, in Proposal, skip map[int64]bool) Proposal {
	if !base.Settings.BatchVariation {
		return in
	}
	out := in
	for _, ms := range in.Sessions {
		f, ok := base.Foods[ms.FoodID]
		if !ok || !f.Composed() || !isMain(f.Role, mealFull) {
			continue
		}
		var pis []int // the batch's plates, its cook plate first
		for i, p := range out.Plates {
			if p.PlateID == 0 && slices.ContainsFunc(p.Components, func(c ProposedComponent) bool { return c.SessionKey == ms.Key }) {
				if p.Date.Equal(ms.CookDate) && p.SlotID == ms.CookSlotID {
					pis = append([]int{i}, pis...)
				} else {
					pis = append(pis, i)
				}
			}
		}
		if len(pis) < 2 {
			continue
		}
		served := map[int64]bool{}
		for _, i := range pis {
			for _, c := range out.Plates[i].Components {
				served[c.FoodID] = true
			}
		}
		for _, i := range pis[1:] {
			if next, food, ok := varyPlate(base, t, out, i, served, skip); ok {
				out, served[food] = next, true
			}
		}
	}
	return out
}

// varyPlate swaps the shared side of proposal plate i for the best variant
// (varyBatches) and reports the new side.
func varyPlate(base State, t Target, in Proposal, i int, served, skip map[int64]bool) (Proposal, int64, bool) {
	pp := in.Plates[i]
	if base.lightMeal(pp.Date, pp.SlotID) {
		return in, 0, false
	}
	merged := Merge(base, t, in)
	pi := slices.IndexFunc(merged.Plates, func(p Plate) bool { return p.ID == -int64(i+1) })
	if pi < 0 {
		return in, 0, false
	}
	x := newIndex(merged, t)
	p := merged.Plates[pi]
	main := x.mainIndex(p)
	if main < 0 {
		return in, 0, false
	}
	si, _ := x.sideOf(p, main)
	if si < 0 || !sharedSession(in, p.Components[si].SessionKey, i) {
		return in, 0, false
	}
	oc := p.Components[si]
	oldKcal := merged.Foods[oc.FoodID].Macros.Kcal * oc.Multiplier()
	base0 := evaluateWith(merged, t, false, true)
	hard, soft := hardKeys(base0.Issues), softKeys(base0.Issues)
	cooked := map[int64]bool{}
	for _, ev := range x.cooks {
		if merged.Window.Contains(ev.cell.Date) {
			cooked[ev.foodID] = true
		}
	}
	before := 0.0
	for _, d := range base0.Days {
		if d.Date == p.Date.Format(time.DateOnly) && d.TargetKcal != nil && *d.TargetKcal > 0 {
			before = d.Kcal / *d.TargetKcal * 100
		}
	}
	used := map[int64]int{}
	for _, pl := range in.Plates {
		for _, c := range pl.Components {
			used[c.FoodID]++
		}
	}
	today := merged.dayFoods(p)
	type variant struct {
		sw     weekSwap
		dKcal  float64
		raised int
	}
	var vs []variant
	for _, c := range merged.altCandidates(x, p, si, AltRequest{What: AltSide}) {
		if served[c.food.ID] || today[c.food.ID] || skip[c.food.ID] || base.RatherNot[c.food.ID] || c.food.Macros.Kcal <= 0 || (c.session != nil && c.grow > 0) || c.cookAt != nil {
			continue
		}
		d := math.Abs(c.food.Macros.Kcal*c.comp.Multiplier() - oldKcal)
		if d > maxVaryKcal {
			continue
		}
		rest := withoutComp(p, si)
		if !merged.addFits(rest, c.food, slotName(merged, p.SlotID)) {
			continue // no fruit, muesli or second protein as a dinner's side
		}
		on, clash, raised := merged.plateFitOf(rest, c.food)
		if on && clash {
			continue
		}
		a, why, _ := merged.tryAlt(t, pi, si, nil, c, base0, hard, soft, len(x.occasions[dayOf(p.Date)]), cooked, false, false)
		if why != "" || len(a.NewIssues) > 0 || a.rank.addsCook || !x.dayHolds(a, before) {
			continue
		}
		vs = append(vs, variant{weekSwap{pi: pi, old: si, c: c, a: a}, d, raised})
	}
	slices.SortStableFunc(vs, func(a, b variant) int {
		return cmp.Or(
			cmp.Compare(math.Round(a.dKcal/kcalBucket), math.Round(b.dKcal/kcalBucket)),
			cmp.Compare(used[a.sw.c.food.ID], used[b.sw.c.food.ID]),
			cmp.Compare(a.raised, b.raised),
			cmp.Compare(a.sw.c.food.ID, b.sw.c.food.ID),
		)
	})
	for _, v := range vs {
		if next, ok := applySwap(in, merged, v.sw); ok {
			return next, v.sw.c.food.ID, true
		}
	}
	return in, 0, false
}

// sharedSession reports whether session key feeds a proposal plate other
// than plate i (a side the plate shares with its batch).
func sharedSession(in Proposal, key string, i int) bool {
	for j, p := range in.Plates {
		if j != i && slices.ContainsFunc(p.Components, func(c ProposedComponent) bool { return c.SessionKey == key }) {
			return true
		}
	}
	return false
}

// withoutComp is p without its component at i.
func withoutComp(p Plate, i int) Plate {
	p.Components = slices.Delete(slices.Clone(p.Components), i, i+1)
	return p
}

// dayFoods are the foods of the other plates of p's day.
func (s State) dayFoods(p Plate) map[int64]bool {
	out := map[int64]bool{}
	for _, pl := range s.Plates {
		if dayOf(pl.Date) == dayOf(p.Date) && pl.ID != p.ID {
			for _, c := range pl.Components {
				out[c.FoodID] = true
			}
		}
	}
	return out
}
