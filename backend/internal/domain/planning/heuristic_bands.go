package planning

import (
	"cmp"
	"math"
	"slices"

	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// fitMealsToBands is PL7: after the day passes, the proposed lunches and
// dinners of a day get the portions of their mains and sides moved on the
// quarter grid (up to two components a meal) and an add-on may move to another
// meal of the day, so as many as possible land inside their slot band (SB1/SB5,
// percent of the day's full target) while the day stays inside its own band
// and above its floor. A recipe's batch grows or shrinks by the same amount (H3
// holds). The best combination of a day is kept only when it adds no hard or
// soft issue. A light or hearty meal (marked, or on a light or hearty day) and
// a main that eats a persisted batch stay as they are.
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
	// addOns are the components (index) of no-cook leaves that may move to
	// another meal of the day, with their kcal per eater.
	addOns map[int]float64
}

// bandReloc moves the add-on ci of meal from to meal to (the same grams).
type bandReloc struct {
	from, to, ci int
	kcal         float64
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
	var platesOf []Plate // the merged plate of each meal
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
		id := int64(-(pi + 1))
		m := bandMeal{pi: pi, kcal: plateKcal[id], lo: lo, hi: hi, eaters: p.eaters(h.s), moves: []bandMove{{ci: -1}}, addOns: map[int]float64{}}
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
			case !f.Composed() && c.Grams > 0:
				m.addOns[ci] = f.Macros.Kcal * c.Grams / 100
				if f.Macros.Protein >= 10 { // protein stays: it carries the week target
					continue
				}
				for _, k := range []float64{0.5, 0.75} {
					g := math.Round(c.Grams*k/roundGrams) * roundGrams
					if g < c.Grams {
						m.moves = append(m.moves, bandMove{ci: ci, grams: g, dKcal: f.Macros.Kcal * (g - c.Grams) / 100})
					}
				}
			}
		}
		var at Plate
		for _, mp := range merged.Plates {
			if mp.ID == id {
				at = mp
			}
		}
		meals = append(meals, m)
		platesOf = append(platesOf, at)
		if len(meals) == 3 {
			break
		}
	}
	if len(meals) == 0 {
		return
	}
	// The options of a meal: no change, one move, or two moves on different
	// components.
	opts := make([][][]bandMove, len(meals))
	for i, m := range meals {
		opts[i] = [][]bandMove{nil}
		for a := 1; a < len(m.moves); a++ {
			opts[i] = append(opts[i], []bandMove{m.moves[a]})
			for b := a + 1; b < len(m.moves); b++ {
				if m.moves[b].ci != m.moves[a].ci {
					opts[i] = append(opts[i], []bandMove{m.moves[a], m.moves[b]})
				}
			}
		}
	}
	// An add-on may move to another meal of the day that fits it and does not
	// hold that food yet.
	relocs := []*bandReloc{nil}
	for i, m := range meals {
		for ci, k := range m.addOns {
			f := h.s.Foods[h.prop.Plates[m.pi].Components[ci].FoodID]
			for j := range meals {
				if j == i || slices.ContainsFunc(h.prop.Plates[meals[j].pi].Components, func(c ProposedComponent) bool { return c.FoodID == f.ID }) ||
					!merged.addOnFits(platesOf[j], f, slotName(merged, platesOf[j].SlotID)) {
					continue
				}
				relocs = append(relocs, &bandReloc{from: i, to: j, ci: ci, kcal: k})
			}
		}
	}
	slices.SortStableFunc(relocs[1:], func(a, b *bandReloc) int {
		return cmp.Or(cmp.Compare(a.from, b.from), cmp.Compare(a.ci, b.ci), cmp.Compare(a.to, b.to))
	})
	dayLo, dayHi := dk.Adjusted*y.pol.DayLow, dk.Adjusted*y.pol.DayHigh
	floor := dk.Floor(y.pol)
	dayDist := func(v float64) float64 { return math.Max(0, math.Max(dayLo-v, v-dayHi)) }
	startIn := 0
	for _, m := range meals {
		if m.kcal >= m.lo && m.kcal <= m.hi {
			startIn++
		}
	}
	type pick struct {
		mv     [][]bandMove
		reloc  *bandReloc
		in     int
		dayGap float64
	}
	// Every combination that puts more meals into their band is a candidate;
	// the best are tried in turn, since a check after applying (a soft rule the
	// plan had closed, such as the day's protein) can refuse the first.
	var picks []pick
	for _, rl := range relocs {
		var walk func(i int, mv [][]bandMove)
		walk = func(i int, mv [][]bandMove) {
			if i == len(meals) {
				day := dk.Kcal
				in, left := 0, 0
				for j, m := range meals {
					v := m.kcal
					for _, x := range mv[j] {
						v += x.dKcal
						day += x.dKcal
					}
					if rl != nil && rl.from == j {
						v -= rl.kcal
					}
					if rl != nil && rl.to == j {
						v += rl.kcal
					}
					if v >= m.lo && v <= m.hi {
						in++
					} else if m.kcal >= m.lo && m.kcal <= m.hi {
						left++
					}
				}
				// A day gives up kcal only down to its own band and while the
				// week has room (the week outweighs a meal's place in its band,
				// priority 2 over 7), and it never moves further from its band.
				if day < floor || day < dayLo-epsilon && day < dk.Kcal || dk.Kcal-day > *room+epsilon || dayDist(day) > dayDist(dk.Kcal)+epsilon {
					return
				}
				// A day brought into its own band (PL3) counts too, while no meal
				// leaves its band.
				if in > startIn || (left == 0 && in == startIn && dayDist(dk.Kcal) > epsilon && dayDist(day) <= epsilon) {
					picks = append(picks, pick{mv: slices.Clone(mv), reloc: rl, in: in, dayGap: dayDist(day)})
				}
				return
			}
			for _, o := range opts[i] {
				if rl != nil && rl.from == i && slices.ContainsFunc(o, func(x bandMove) bool { return x.ci == rl.ci }) {
					continue // the add-on that moves is not also shrunk
				}
				walk(i+1, append(mv, o))
			}
		}
		walk(0, nil)
	}
	// The most meals in band first, then the day nearest its band, then the
	// smallest change (the fewest moves), in the order found.
	size := func(p pick) int {
		n := 0
		for _, o := range p.mv {
			n += len(o)
		}
		if p.reloc != nil {
			n++
		}
		return n
	}
	slices.SortStableFunc(picks, func(a, b pick) int {
		return cmp.Or(cmp.Compare(b.in, a.in), cmp.Compare(a.dayGap, b.dayGap), cmp.Compare(size(a), size(b)))
	})
	for i, p := range picks {
		if i == maxBandTries {
			return
		}
		if h.applyMoves(meals, p.mv, p.reloc) {
			day := dk.Kcal
			for _, o := range p.mv {
				for _, x := range o {
					day += x.dKcal
				}
			}
			*room -= dk.Kcal - day
			return
		}
	}
}

// maxBandTries bounds the combinations fitDay applies and checks per day
// (each check re-evaluates the plan).
const maxBandTries = 8

// applyMoves applies each meal's moves (a recipe's batch by the same amount)
// and the add-on move, and keeps them when no hard or soft issue is added;
// else it restores the proposal.
func (h *planner) applyMoves(meals []bandMeal, moves [][]bandMove, reloc *bandReloc) bool {
	saved := h.prop
	plates := slices.Clone(h.prop.Plates)
	sessions := slices.Clone(h.prop.Sessions)
	was := len(openHard(Merge(h.s, h.in.Target, h.prop), h.in.Target))
	wasSoft := softIssues(evaluateLean(Merge(h.s, h.in.Target, h.prop), h.in.Target).Issues)
	h.prop.Sessions = slices.Clone(h.prop.Sessions)
	for i, m := range meals {
		for _, mv := range moves[i] {
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
	}
	if reloc != nil {
		from, to := &h.prop.Plates[meals[reloc.from].pi], &h.prop.Plates[meals[reloc.to].pi]
		c := from.Components[reloc.ci]
		from.Components = slices.Delete(slices.Clone(from.Components), reloc.ci, reloc.ci+1)
		to.Components = append(slices.Clone(to.Components), c)
	}
	// No move may open a soft rule the plan had closed (the week's protein,
	// say), except a day's protein (priority 4) on a day the move brings back
	// into its kcal range (priority 3, spec section 2.5).
	nowSoft := softIssues(evaluateLean(Merge(h.s, h.in.Target, h.prop), h.in.Target).Issues)
	opened := false
	for k, is := range nowSoft {
		if _, had := wasSoft[k]; had {
			continue
		}
		if is.Code == CodeProteinLow && is.Date != "" && closedDayBand(wasSoft, nowSoft, is.Date) {
			continue
		}
		opened = true
	}
	if opened || len(openHard(Merge(h.s, h.in.Target, h.prop), h.in.Target)) > was {
		saved.Plates, saved.Sessions = plates, sessions
		h.prop = saved
		return false
	}
	return true
}

// softIssues are the soft issues of a list by softKey.
func softIssues(issues []Issue) map[string]Issue {
	m := map[string]Issue{}
	for _, is := range issues {
		if is.Severity == SeveritySoft {
			m[softKey(is)] = is
		}
	}
	return m
}

// closedDayBand reports whether the day date had a kcal range issue (S2) in
// was that now is gone.
func closedDayBand(was, now map[string]Issue, date string) bool {
	for k, is := range was {
		if _, still := now[k]; !still && is.Date == date && (is.Code == CodeKcalAboveBand || is.Code == CodeKcalBelowBand) {
			return true
		}
	}
	return false
}
