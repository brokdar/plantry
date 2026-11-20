package planning

import (
	"math"
	"slices"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// fitMealsToBands is PL7: after the day passes, the proposed lunches and
// dinners of a day get the portions of their mains moved on the quarter grid so
// as many as possible land inside their slot band (SB1, percent of the day's
// full target) while the day stays inside its own band and above its floor. A
// main's batch grows or shrinks by the same amount (H3 holds). The best
// combination of a day is kept only when it adds no hard issue. A light or
// hearty meal (marked, or on a light or hearty day) and a main that eats a
// persisted batch stay as they are.
func (h *planner) fitMealsToBands() {
	k := h.s.Profile.KcalTarget
	if k == nil || *k <= 0 || len(h.s.SlotBand) == 0 {
		return
	}
	y, days := h.dayAccounts()
	// The week outweighs a meal's place in its band (priority 2 over 7): meals
	// may give up kcal only while the week average stays inside its band.
	sumKcal, sumTarget := 0.0, 0.0
	for _, dk := range days {
		sumKcal += dk.Kcal
		sumTarget += dk.Adjusted
	}
	room := sumKcal - y.pol.WeekLow*1.01*sumTarget
	for _, dk := range days {
		if dk.Away {
			continue
		}
		h.fitDay(y, dk, &room)
	}
}

// bandMeal is a meal of one day whose main's portions may move.
type bandMeal struct {
	pi     int
	kcal   float64 // the plate, per eater
	lo, hi float64 // its band in kcal
	eaters float64
	moves  []bandMove // the changes to try: none first
}

// bandMove changes one component of a plate: a recipe's portions (its batch
// follows) or a leaf's grams (shrinking only), by dKcal per eater.
type bandMove struct {
	ci       int
	portions float64 // new portions (recipe), 0 for a leaf
	grams    float64 // new grams (leaf)
	dKcal    float64
}

func (h *planner) fitDay(y *index, dk DayKcal, room *float64) {
	merged := Merge(h.s, h.in.Target, h.prop)
	ev := evaluateLean(merged, h.in.Target)
	plateKcal := map[int64]float64{}
	for _, pe := range ev.Plates {
		plateKcal[pe.PlateID] = pe.Kcal
	}
	var meals []bandMeal
	for pi, p := range h.prop.Plates {
		if dayOf(p.Date) != dayOf(dk.Date) {
			continue
		}
		b, ok := h.s.SlotBand[p.SlotID]
		if !ok || h.x.mealOf(p.SlotID) != mealFull || h.x.routine(plate.CellKey{Date: p.Date, SlotID: p.SlotID}) ||
			h.s.lightDay(p.Date) || h.s.heartyDay(p.Date) {
			continue
		}
		lo, hi := dk.bandKcal(b, p.SlotID)
		switch {
		case h.s.lightMeal(p.Date, p.SlotID): // the explicit request outranks the band: the cap
			lo, hi = 0, LightMealShare*dk.mealShare(p.SlotID)
		case h.s.heartyMeal(p.Date, p.SlotID): // the filling minimum
			lo, hi = HeartyMealShare*dk.mealShare(p.SlotID), math.Inf(1)
		}
		m := bandMeal{pi: pi, kcal: plateKcal[int64(-(pi + 1))], lo: lo, hi: hi, eaters: p.eaters(h.s), moves: []bandMove{{ci: -1}}}
		for ci, c := range p.Components {
			f, ok := h.s.Foods[c.FoodID]
			if !ok || f.Macros.Kcal <= 0 || c.SessionID != 0 {
				continue
			}
			switch {
			case f.Composed() && c.Portions > 0 && (f.Role == "main" || f.Role == "side_starch" || f.Role == "side_veg"):
				floor := wayPortionShared
				if c.SessionKey == "" {
					floor = wayPortionMin
				}
				for n := floor; n <= MaxDishPortions+epsilon; n += 0.25 {
					if n != c.Portions {
						m.moves = append(m.moves, bandMove{ci: ci, portions: n, dKcal: (n - c.Portions) * f.Macros.Kcal})
					}
				}
			case !f.Composed() && c.Grams > 0 && f.Macros.Protein < 10: // protein stays: it carries the week target
				for _, k := range []float64{0.5, 0.75} {
					g := math.Round(c.Grams*k/roundGrams) * roundGrams
					if g < c.Grams {
						m.moves = append(m.moves, bandMove{ci: ci, grams: g, dKcal: f.Macros.Kcal * (g - c.Grams) / 100})
					}
				}
			}
		}
		meals = append(meals, m)
		if len(meals) == 3 {
			break
		}
	}
	if len(meals) == 0 {
		return
	}
	inBand := func(m bandMeal, mv bandMove) bool {
		v := m.kcal + mv.dKcal
		return v >= m.lo && v <= m.hi
	}
	dayLo, dayHi := dk.Adjusted*y.pol.DayLow, dk.Adjusted*y.pol.DayHigh
	floor := dk.Floor(y.pol)
	dayDist := func(v float64) float64 { return math.Max(0, math.Max(dayLo-v, v-dayHi)) }
	startIn := 0
	for _, m := range meals {
		if inBand(m, m.moves[0]) {
			startIn++
		}
	}
	type pick struct {
		mv     []bandMove
		in     int
		dayGap float64
	}
	best := pick{in: startIn, dayGap: dayDist(dk.Kcal)}
	var walk func(i int, mv []bandMove)
	walk = func(i int, mv []bandMove) {
		if i == len(meals) {
			day := dk.Kcal
			in := 0
			for j, m := range meals {
				day += mv[j].dKcal
				if inBand(m, mv[j]) {
					in++
				}
			}
			// A change never takes the day below its target when it was above it: the
			// week outweighs a meal's place in its band (priority 2 over 7).
			// A day gives up kcal only down to its own band and while the week has room.
			if day < floor || day < dayLo-epsilon && day < dk.Kcal || dk.Kcal-day > *room+epsilon || dayDist(day) > dayDist(dk.Kcal)+epsilon {
				return
			}
			if in > best.in || (in == best.in && in > startIn && dayDist(day) < best.dayGap-epsilon) {
				best = pick{mv: slices.Clone(mv), in: in, dayGap: dayDist(day)}
			}
			return
		}
		for _, x := range meals[i].moves {
			walk(i+1, append(mv, x))
		}
	}
	walk(0, nil)
	if best.in <= startIn {
		return
	}
	if h.applyMoves(meals, best.mv) {
		day := dk.Kcal
		for _, mv := range best.mv {
			day += mv.dKcal
		}
		*room -= dk.Kcal - day
	}
}

// applyMoves applies one move per meal (the recipe's batch by the same amount)
// and keeps them when no hard issue is added; else it restores the proposal.
func (h *planner) applyMoves(meals []bandMeal, moves []bandMove) bool {
	saved := h.prop
	plates := slices.Clone(h.prop.Plates)
	sessions := slices.Clone(h.prop.Sessions)
	was := len(openHard(Merge(h.s, h.in.Target, h.prop), h.in.Target))
	wasSoft := softKeys(evaluateLean(Merge(h.s, h.in.Target, h.prop), h.in.Target).Issues)
	for i, m := range meals {
		mv := moves[i]
		if mv.ci < 0 {
			continue
		}
		p := &h.prop.Plates[m.pi]
		p.Components = slices.Clone(p.Components)
		c := &p.Components[mv.ci]
		if mv.grams > 0 || (mv.portions == 0 && c.Grams > 0) {
			c.Grams = mv.grams
			continue
		}
		if c.SessionKey != "" {
			for si := range h.prop.Sessions {
				if h.prop.Sessions[si].Key == c.SessionKey {
					h.prop.Sessions[si].Portions += m.eaters * (mv.portions - c.Portions)
				}
			}
		}
		c.Portions = mv.portions
	}
	// No move may open a soft rule the plan had closed (the week's protein, say).
	opened := false
	for k := range softKeys(evaluateLean(Merge(h.s, h.in.Target, h.prop), h.in.Target).Issues) {
		if !wasSoft[k] {
			opened = true
		}
	}
	if opened || len(openHard(Merge(h.s, h.in.Target, h.prop), h.in.Target)) > was {
		saved.Plates, saved.Sessions = plates, sessions
		h.prop = saved
		return false
	}
	return true
}
