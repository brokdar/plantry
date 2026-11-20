package planning

import (
	"math"
	"slices"
	"time"

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
	for _, dk := range days {
		if dk.Away {
			continue
		}
		h.fitDay(y, dk, *k)
	}
}

// bandMeal is a meal of one day whose main's portions may move.
type bandMeal struct {
	pi, ci   int
	kcal     float64   // the plate, per eater
	lo, hi   float64   // its band in kcal
	perPort  float64   // kcal per eater of one portion of the main
	options  []float64 // portions to try
	eaters   float64
	portions float64
}

func (h *planner) fitDay(y *index, dk DayKcal, kk float64) {
	k := kk
	merged := Merge(h.s, h.in.Target, h.prop)
	ev := Evaluate(merged, h.in.Target)
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
		lo, hi := b.MinPct/100*k, b.MaxPct/100*k
		switch {
		case h.s.lightMeal(p.Date, p.SlotID): // the explicit request outranks the band: the cap
			lo, hi = 0, LightMealShare*dk.mealShare(p.SlotID)
		case h.s.heartyMeal(p.Date, p.SlotID): // the filling minimum
			lo, hi = HeartyMealShare*dk.mealShare(p.SlotID), math.Inf(1)
		}
		ci := h.bandMain(p)
		if ci < 0 {
			continue
		}
		c := p.Components[ci]
		f := h.s.Foods[c.FoodID]
		floor := wayPortionShared
		if c.SessionKey == "" {
			floor = wayPortionMin
		}
		m := bandMeal{
			pi: pi, ci: ci, kcal: plateKcal[int64(-(pi + 1))], lo: lo, hi: hi,
			perPort: f.Macros.Kcal, eaters: p.eaters(h.s), portions: c.Portions,
		}
		for n := floor; n <= MaxDishPortions+epsilon; n += 0.25 {
			m.options = append(m.options, n)
		}
		meals = append(meals, m)
		if len(meals) == 3 {
			break
		}
	}
	if len(meals) == 0 {
		return
	}
	inBand := func(m bandMeal, n float64) bool {
		v := m.kcal + (n-m.portions)*m.perPort
		return v >= m.lo && v <= m.hi
	}
	dayLo, dayHi := dk.Adjusted*y.pol.DayLow, dk.Adjusted*y.pol.DayHigh
	floor := dk.Floor(y.pol)
	dayDist := func(v float64) float64 { return math.Max(0, math.Max(dayLo-v, v-dayHi)) }
	startIn := 0
	for _, m := range meals {
		if inBand(m, m.portions) {
			startIn++
		}
	}
	type pick struct {
		ns      []float64
		in      int
		dayGap  float64
		changed int
	}
	best := pick{in: startIn, dayGap: dayDist(dk.Kcal)}
	best.ns = make([]float64, len(meals))
	for i, m := range meals {
		best.ns[i] = m.portions
	}
	var walk func(i int, ns []float64)
	walk = func(i int, ns []float64) {
		if i == len(meals) {
			day := dk.Kcal
			in, changed := 0, 0
			for j, m := range meals {
				day += (ns[j] - m.portions) * m.perPort
				if inBand(m, ns[j]) {
					in++
				}
				if ns[j] != m.portions {
					changed++
				}
			}
			// A change never takes the day below its target when it was above it: the
			// week outweighs a meal's place in its band (priority 2 over 7).
			if day < floor || day < math.Min(dk.Kcal, dk.Adjusted)-epsilon || dayDist(day) > dayDist(dk.Kcal)+epsilon {
				return
			}
			if in > best.in || (in == best.in && in > startIn && dayDist(day) < best.dayGap-epsilon) {
				best = pick{ns: slices.Clone(ns), in: in, dayGap: dayDist(day), changed: changed}
			}
			return
		}
		for _, n := range meals[i].options {
			walk(i+1, append(ns, n))
		}
	}
	walk(0, nil)
	if best.in <= startIn {
		return
	}
	h.applyPortions(meals, best.ns, dk.Date)
}

// applyPortions sets the mains' portions (their batches by the same amount) and
// keeps them when no hard issue is added; else it restores the proposal.
func (h *planner) applyPortions(meals []bandMeal, ns []float64, date time.Time) {
	saved := h.prop
	plates := slices.Clone(h.prop.Plates)
	sessions := slices.Clone(h.prop.Sessions)
	was := len(openHard(Merge(h.s, h.in.Target, h.prop), h.in.Target))
	for i, m := range meals {
		p := &h.prop.Plates[m.pi]
		p.Components = slices.Clone(p.Components)
		c := &p.Components[m.ci]
		if c.SessionKey != "" {
			for si := range h.prop.Sessions {
				if h.prop.Sessions[si].Key == c.SessionKey {
					h.prop.Sessions[si].Portions += m.eaters * (ns[i] - c.Portions)
				}
			}
		}
		c.Portions = ns[i]
	}
	if len(openHard(Merge(h.s, h.in.Target, h.prop), h.in.Target)) > was {
		saved.Plates, saved.Sessions = plates, sessions
		h.prop = saved
	}
}

// bandMain is the index of the main component of proposed plate p whose
// portions may move: a recipe main cooked fresh for the plate or eating a batch
// this proposal cooks; -1 when none.
func (h *planner) bandMain(p ProposedPlate) int {
	for i, c := range p.Components {
		f, ok := h.s.Foods[c.FoodID]
		if !ok || c.Portions <= 0 || c.SessionID != 0 || !f.Composed() || f.Macros.Kcal <= 0 || f.Role != "main" {
			continue
		}
		return i
	}
	return -1
}
