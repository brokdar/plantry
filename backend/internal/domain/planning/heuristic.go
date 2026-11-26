package planning

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/nutrition"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Heuristic is the deterministic chooser for planning without AI (spec
// §6.7, D17): a greedy walk over the target's empty plan cells in cell
// order. Each cell eats from an existing session with room left (fixed
// sessions first, then the most recent proposed one). Otherwise a cell
// before the day's last open cell grows the latest proposed session that may
// feed it, so cooking happens at dinner; a cell nothing can feed (day one,
// or batch cooking off) gets a new session, sized to the recipe's reference
// portions over the following cells within its keep days, plus a fresh side
// for a main. Ties break by food id, so the same input gives the same plan.
type Heuristic struct{}

// Choose implements Chooser. It fails with ErrNotEnoughRecipes when no
// recipe fits a cell.
func (Heuristic) Choose(ctx context.Context, in ChooseInput) (Proposal, error) {
	recordContext(ctx, in, "heuristic", nil)
	h := newPlanner(in)
	var prop Proposal
	var err error
	if in.Side != nil {
		prop, err = h.side(*in.Side)
	} else {
		h, prop, err = planWithForcedCooks(in, h)
		if err == nil {
			// The week's meat budget and one fish meal, by verified swaps
			// (the picks above only weigh them as tie-breaks).
			var notes []string
			prop, notes = guardWeek(in.State, in.Target, prop, idSet(in.Exclude), in.Instruction, sidePickerFor(in, prop))
			prop = varyBatches(in.State, in.Target, prop, idSet(in.Exclude))
			prop.Notes = joinNotes(append([]string{prop.Notes}, notes...)...)
		}
	}
	h.recordChoices(ctx)
	return prop, err
}

// epsilon absorbs float noise in portion sums.
const epsilon = 1e-9

// supply is a session cells can eat from: fixed (id) or proposed (index
// into the proposal's sessions, with its side's index or -1).
type supply struct {
	id       int64
	proposed int
	side     int
	food     int64
	cook     plate.CookSession
	keep     int
	// free is what is left for fresh and leftover cells, frozen for cells
	// past keep days.
	free   float64
	frozen float64
}

type planner struct {
	in    ChooseInput
	s     State
	x     *index
	order plate.SlotOrder
	prop  Proposal
	// open are the target's empty plan cells in cell order; done marks the
	// ones the proposal fills.
	open     []plate.CellKey
	done     map[cellID]bool
	supplies []*supply
	// occasions holds each day's cook cells (H2), routine cells excluded.
	occasions map[int]map[cellID]bool
	meatDays  map[int]bool
	// balance marks the final balancing pass over a plan a model made
	// (balancePlan): fresh components grow too and a day may take a side recipe.
	balance bool
	// sided are the days that took a side recipe in that pass.
	sided map[int]bool
	// meatLimitG is the DGE meat limit prorated to the planned days (S7).
	meatLimitG float64
	// allowMeat lets a leftover add a meat day past the limit: the last
	// resort for a cell nothing else can fill (H1 weighs more than H5); it
	// also lets a batch pass its S8 dish cap then.
	allowMeat bool
	// notes are the run-note lines of rules the heuristic had to bend or
	// could not meet (a meat day past the limit, a cell without a main).
	notes      []string
	seen       map[int64][]time.Time
	used       map[int64]int
	avoid      map[int64]bool
	avoidBatch map[int64]bool
	// exclude are foods the target may not hold at all (ChooseInput.Exclude).
	exclude map[int64]bool
	// routineFoods are the recipes the target's routine plates eat: they are
	// breakfast routines, never planned into another cell.
	routineFoods map[int64]bool
	// explorePlaced and fillPlaced count the explore recipes and the
	// never-eaten fillers the proposal cooks toward the quota (D17).
	explorePlaced, fillPlaced int
	// topUpBase is the portions of a component (plate, component index) before
	// the kcal passes grew it: the 1.5x cap holds over all of them.
	topUpBase map[[2]int]float64
	// floorLift is set while topUpFloors lifts a day to its safety floor: any
	// verified food may fill the gap, so the plate-fit starch filter is off.
	floorLift bool
	// nutritionPass is set while nutritionFill lifts what the culinary
	// filler passes left short (plate fit is off for that pass).
	nutritionPass bool
	// adds counts each day's no-cook adds over every pass (dayAdds).
	adds map[int]*dayAdds
	// choices log what was chosen per cell and why (the run's
	// heuristic.choices step).
	choices []heuristicChoice
	// stranded are the cells left empty because their day's cooking cap was
	// used and no batch could feed them; forced are cells that must cook
	// (and are never covered by another cook) so the stranded cell they map to
	// can eat their leftovers; the recipes cooked there must keep that long.
	stranded []plate.CellKey
	forced   map[cellID]plate.CellKey
}

func newPlanner(in ChooseInput) *planner {
	s := in.State
	h := &planner{
		in: in, s: s, x: newIndex(s, in.Target), order: s.SlotOrder,
		done: map[cellID]bool{}, occasions: map[int]map[cellID]bool{},
		meatDays: map[int]bool{}, seen: map[int64][]time.Time{}, used: map[int64]int{},
		avoid: idSet(in.Avoid), avoidBatch: idSet(in.AvoidBatches), exclude: idSet(in.Exclude), routineFoods: nil,
	}
	h.routineFoods = h.x.routineFoods()
	h.meatLimitG = DGEMeatMaxG
	if b, ok := MeatBudgetOf(s, in.Target); ok {
		h.meatLimitG = b.LimitG
	}
	for day, cells := range h.x.occasions {
		h.occasions[day] = map[cellID]bool{}
		for c := range cells {
			h.occasions[day][c] = true
		}
	}
	for _, c := range s.Cells {
		if c.Mode == availability.ModePlan && h.x.inTarget(c.Key()) && len(h.x.byCell[idOf(c.Key())]) == 0 {
			h.open = append(h.open, c.Key())
		}
	}
	slices.SortStableFunc(h.open, func(a, b plate.CellKey) int { return plate.Compare(a, b, h.order) })
	for _, p := range s.Plates {
		for _, c := range p.Components {
			if f, ok := s.Foods[c.FoodID]; ok && f.DietClass == diet.Omnivore && s.Window.Contains(p.Date) {
				h.meatDays[dayOf(p.Date)] = true
			}
		}
	}
	for _, ss := range s.Sessions {
		h.seen[ss.FoodID] = append(h.seen[ss.FoodID], ss.CookDate)
	}
	for _, hp := range s.History {
		for _, id := range hp.FoodIDs {
			h.seen[id] = append(h.seen[id], hp.Date)
		}
	}
	for _, ss := range s.Sessions {
		f, ok := s.Foods[ss.FoodID]
		if !ok || !varietyRole(f.Role) || !f.Composed() {
			continue
		}
		use := h.x.usage(ss)
		sup := &supply{
			id: ss.ID, proposed: -1, side: -1, food: ss.FoodID, cook: ss.CookSession, keep: h.x.keepDays(ss.FoodID),
			free:   ss.Portions - ss.FrozenPortions - (use.Total - use.Frozen),
			frozen: ss.FrozenPortions - use.Frozen,
		}
		if sup.free > epsilon || sup.frozen > epsilon {
			h.supplies = append(h.supplies, sup)
		}
	}
	return h
}

func (h *planner) plan() (Proposal, error) {
	for _, c := range h.open {
		if h.done[idOf(c)] {
			continue
		}
		eaters := h.eaters(c.Date)
		limit := h.s.Settings.MaxCookOccasionsPerDay
		maxed := limit > 0 && len(h.occasions[dayOf(c.Date)]) >= limit
		// The user's cooking-day limit: a day that does not cook yet is closed
		// once the limit's days cook (a day that cooks already stays open).
		daysUsed := h.in.MaxCookDays > 0 && len(h.occasions[dayOf(c.Date)]) == 0 && h.cookDays() >= h.in.MaxCookDays
		maxed = maxed || daysUsed
		// A forced cell cooks unless earlier cooks of the rerun used its
		// day's cap meanwhile.
		if _, ok := h.forced[idOf(c)]; ok && (!maxed || h.occasions[dayOf(c.Date)][idOf(c)]) {
			if err := h.cook(c); err != nil {
				return Proposal{}, err
			}
			continue
		}
		if sup := h.supplyFor(c, eaters); sup != nil {
			h.eat(sup, c, eaters, false)
			continue
		}
		// Cook on the day's last open cell (dinner, spec §6.7 step 1): an
		// earlier cell eats from the latest proposed session that may still
		// feed it, which cooks more; so does any cell of a day that may not
		// cook again. Only a cell nothing can feed cooks early.
		if h.s.Settings.BatchCooking && (maxed || !h.lastOpenOfDay(c)) {
			if sup := h.growable(c); sup != nil {
				h.eat(sup, c, eaters, true)
				continue
			}
		}
		if maxed && !h.occasions[dayOf(c.Date)][idOf(c)] {
			if h.eatAnyway(c, eaters) {
				continue
			}
			if daysUsed && (limit <= 0 || len(h.occasions[dayOf(c.Date)]) < limit) {
				// Only the cooking-day limit stops this cell: an empty meal breaks
				// a hard rule, a third cooking day does not. Cook, and say so.
				h.notes = append(h.notes, fmt.Sprintf(h.text(textCookDays), h.weekday(c.Date), h.in.MaxCookDays))
				if err := h.cook(c); err != nil {
					return Proposal{}, err
				}
				continue
			}
			// The day's cap is used by other cells' cooks (confirmed ones
			// included) and no batch can feed c: leave it empty; the checker
			// reports it (H1). A cell that already is an occasion may cook.
			h.choices = append(h.choices, heuristicChoice{Date: c.Date.Format(time.DateOnly), SlotID: c.SlotID, Reason: "left_empty_cook_cap"})
			h.stranded = append(h.stranded, c)
			continue
		}
		if err := h.cook(c); err != nil {
			return Proposal{}, err
		}
	}
	h.topUpFloors()
	h.fitMealsToBands()
	h.prop.MaxCookDays = h.in.MaxCookDays
	h.prop.OwnSet = true
	h.prop.Own = strings.TrimSpace(strings.Join(append([]string{h.exploreNote()}, h.notes...), "\n"))
	if note := lightNote(h.s.Light, h.in.Locale); note != "" {
		h.notes = append(h.notes, note)
	}
	if note := kcalNote(Merge(h.s, h.in.Target, h.prop), h.in.Target); note != "" {
		h.notes = append(h.notes, note)
	}
	if note := floorNote(Merge(h.s, h.in.Target, h.prop), h.in.Target); note != "" {
		h.notes = append(h.notes, note)
	}
	if note := mealNote(Merge(h.s, h.in.Target, h.prop), h.in.Target); note != "" {
		h.notes = append(h.notes, note)
	}
	if note := proteinNote(Merge(h.s, h.in.Target, h.prop), h.in.Target); note != "" {
		h.notes = append(h.notes, note)
	}
	h.prop.Notes = strings.Join(append([]string{h.exploreNote()}, h.notes...), "\n")
	h.prop.Notes = strings.TrimSpace(h.prop.Notes)
	return h.prop, nil
}

// planWithForcedCooks plans, and while a cell is left empty by the cooking
// cap that a cook the greedy walk did not make could fill, reruns with those
// cooks forced. It returns the planner of the last run.
func planWithForcedCooks(in ChooseInput, h *planner) (*planner, Proposal, error) {
	prop, err := h.plan()
	forced := map[cellID]plate.CellKey{}
	for err == nil && len(h.stranded) > 0 {
		before := len(forced)
		for _, c := range h.stranded {
			if f, ok := h.feederFor(c, forced); ok {
				forced[idOf(f)] = c
			}
		}
		if len(forced) == before {
			break
		}
		h = newPlanner(in)
		h.forced = forced
		prop, err = h.plan()
	}
	return h, prop, err
}

// feederFor is the latest open cell before the stranded cell c that may cook
// (a lunch or dinner of a day with a free cooking occasion, not cooking yet)
// for which a main exists whose leftovers reach c within its keep days.
func (h *planner) feederFor(c plate.CellKey, forced map[cellID]plate.CellKey) (plate.CellKey, bool) {
	limit := h.s.Settings.MaxCookOccasionsPerDay
	for _, o := range slices.Backward(h.open) {
		_, isForced := forced[idOf(o)]
		if !plate.Before(o, c, h.order) || isForced || h.x.routine(o) || h.x.mealOf(o.SlotID) != mealFull {
			continue
		}
		occ := h.occasions[dayOf(o.Date)]
		if occ[idOf(o)] || (limit > 0 && len(occ) >= limit) {
			continue
		}
		for _, f := range h.s.Foods {
			if h.mainCandidate(f, o) && reaches(f, o, c, h.s.Settings) {
				return o, true
			}
		}
	}
	return plate.CellKey{}, false
}

// reaches reports whether a batch of f cooked on from still feeds c as a
// leftover within f's keep days.
func reaches(f FoodFacts, from, c plate.CellKey, set Settings) bool {
	return plate.Classify(c, plate.CookSession{CookDate: from.Date, CookSlotID: from.SlotID}, set.KeepDays(f.KeepDays)) == plate.KindLeftover
}

// eatAnyway fills c from a batch even if that adds a meat day past the
// limit, when the day's cooking cap leaves nothing else.
func (h *planner) eatAnyway(c plate.CellKey, eaters float64) bool {
	h.allowMeat = true
	defer func() { h.allowMeat = false }()
	place := func(sup *supply, grow bool) {
		h.allowMeat = false
		if h.supplyOverMeat(sup, c) {
			h.notes = append(h.notes, fmt.Sprintf(h.text(textMeatOver), h.weekday(c.Date), *h.s.Profile.MeatDaysPerWeekMax))
		}
		h.eat(sup, c, eaters, grow)
	}
	if sup := h.supplyFor(c, eaters); sup != nil {
		place(sup, false)
		return true
	}
	if h.s.Settings.BatchCooking {
		if sup := h.growable(c); sup != nil {
			place(sup, true)
			return true
		}
	}
	return false
}

// supplyOverMeat reports whether eating sup (and its side) on c adds a meat
// day past meat_days_per_week_max (H5): leftovers count as meat days too.
func (h *planner) supplyOverMeat(sup *supply, c plate.CellKey) bool {
	if h.allowMeat {
		return false
	}
	if h.overMeat(h.s.Foods[sup.food], c) {
		return true
	}
	return sup.proposed >= 0 && sup.side >= 0 && h.overMeat(h.s.Foods[h.prop.Sessions[sup.side].FoodID], c)
}

// exploreNote names a shortfall of the explore quota: fewer explore recipes
// than asked, filled with never-eaten ones where possible; empty when met.
func (h *planner) exploreNote() string {
	q := h.in.ExploreQuota
	if q <= 0 || h.explorePlaced >= q {
		return ""
	}
	return fmt.Sprintf(h.text(textExploreShortfall), h.explorePlaced, q, h.fillPlaced)
}

// lastOpenOfDay reports whether no open cell of c's date comes after c.
func (h *planner) lastOpenOfDay(c plate.CellKey) bool {
	for _, o := range h.open {
		if dayOf(o.Date) == dayOf(c.Date) && !h.done[idOf(o)] && plate.Before(c, o, h.order) {
			return false
		}
	}
	return true
}

// eaters is the eater count of a date's plates (D16).
func (h *planner) eaters(d time.Time) float64 {
	if n := h.in.EatersByDate[d.Format(time.DateOnly)]; n > 0 {
		return n
	}
	return h.s.Profile.HouseholdPortions
}

// kindAt tells how cell c may eat from sup: fresh, leftover or frozen, or
// false when it may not (before the cook cell, past keep days without
// freezing, or past the freezer keep days).
func (h *planner) kindAt(sup *supply, c plate.CellKey) (plate.LinkKind, bool) {
	if plate.Before(c, sup.cook.CookCell(), h.order) {
		return "", false
	}
	k := plate.Classify(c, sup.cook, sup.keep)
	if k == plate.KindFrozen && (!h.s.Settings.FreezingEnabled || c.Date.After(sup.cook.CookDate.AddDate(0, 0, h.s.Settings.FreezerKeepDays))) {
		return "", false
	}
	return k, true
}

// supplyFor picks the session cell c eats from: a fixed one before a
// proposed one, then the most recent cook cell.
func (h *planner) supplyFor(c plate.CellKey, eaters float64) *supply {
	if !h.s.Settings.BatchCooking {
		return nil
	}
	// A replaced plate leaves its batch unless nothing else can feed the cell.
	if best := h.pickSupply(c, eaters, true); best != nil {
		return best
	}
	return h.pickSupply(c, eaters, false)
}

func (h *planner) pickSupply(c plate.CellKey, eaters float64, skipAvoided bool) *supply {
	var best *supply
	for _, sup := range h.supplies {
		k, ok := h.kindAt(sup, c)
		if !ok || k == plate.KindFresh {
			continue
		}
		if skipAvoided && sup.proposed < 0 && h.avoidBatch[sup.food] {
			continue
		}
		// An excluded food (a replaced plate) feeds the cell only as the last
		// resort of eatAnyway: an empty cell (H1) weighs more.
		if h.supplyOverMeat(sup, c) || (h.exclude[sup.food] && !h.allowMeat) {
			continue
		}
		room := sup.free
		if k == plate.KindFrozen {
			room = sup.frozen
		}
		if room+epsilon < eaters {
			continue
		}
		if best == nil || (best.proposed >= 0 && sup.proposed < 0) ||
			(best.proposed >= 0) == (sup.proposed >= 0) && plate.Before(best.cook.CookCell(), sup.cook.CookCell(), h.order) {
			best = sup
		}
	}
	return best
}

// growable is the most recent proposed session c could eat from as a
// leftover once it cooks more: the way to fill a cell on a day that may not
// cook again.
func (h *planner) growable(c plate.CellKey) *supply {
	var best *supply
	for _, sup := range h.supplies {
		if sup.proposed < 0 {
			continue
		}
		if k, ok := h.kindAt(sup, c); !ok || k != plate.KindLeftover || h.supplyOverMeat(sup, c) || h.supplyAtDishCap(sup, c) || h.exclude[sup.food] {
			continue
		}
		if best == nil || plate.Before(best.cook.CookCell(), sup.cook.CookCell(), h.order) {
			best = sup
		}
	}
	return best
}

// supplyAtDishCap reports whether growing sup for a lunch or dinner on c
// would make its dish the main of more meals than S8 allows (never while
// eatAnyway fills a cell nothing else can: S8 is soft). Portions a batch
// already has are eaten regardless.
func (h *planner) supplyAtDishCap(sup *supply, c plate.CellKey) bool {
	return !h.allowMeat && h.x.mealOf(c.SlotID) == mealFull && h.dishFull(sup.food)
}

// eat places a plate on c eating from sup (and its side); grow cooks the
// portions it needs first.
func (h *planner) eat(sup *supply, c plate.CellKey, eaters float64, grow bool) {
	k, _ := h.kindAt(sup, c)
	switch {
	case grow:
		h.prop.Sessions[sup.proposed].Portions += eaters
		if sup.side >= 0 {
			h.prop.Sessions[sup.side].Portions += eaters
		}
	case k == plate.KindFrozen:
		sup.frozen -= eaters
	default:
		sup.free -= eaters
	}
	pp := ProposedPlate{Date: c.Date, SlotID: c.SlotID, Eaters: eaters, Reason: h.leftoverReason(sup.cook.CookDate)}
	code := "leftover_fixed_batch"
	switch {
	case grow:
		code = "leftover_grown_batch"
	case k == plate.KindFrozen:
		code = "frozen_portion"
	case sup.proposed >= 0:
		code = "leftover_proposed_batch"
	}
	if sup.proposed >= 0 {
		pp.Components = append(pp.Components, ProposedComponent{FoodID: sup.food, Portions: 1, SessionKey: h.prop.Sessions[sup.proposed].Key})
		if sup.side >= 0 {
			side := h.prop.Sessions[sup.side]
			pp.Components = append(pp.Components, ProposedComponent{FoodID: side.FoodID, Portions: 1, SessionKey: side.Key})
		}
	} else {
		pp.Components = append(pp.Components, ProposedComponent{FoodID: sup.food, Portions: 1, SessionID: sup.id})
	}
	h.place(pp, code)
}

func (h *planner) place(pp ProposedPlate, code string) {
	h.logChoice(pp, code)
	h.done[idOf(plate.CellKey{Date: pp.Date, SlotID: pp.SlotID})] = true
	for _, pc := range pp.Components {
		if h.s.Foods[pc.FoodID].DietClass == diet.Omnivore && h.s.Window.Contains(pp.Date) {
			h.meatDays[dayOf(pp.Date)] = true
		}
	}
	h.prop.Plates = append(h.prop.Plates, pp)
}

// cook starts a session (plus a side for a main) on c and places the plates
// it covers.
func (h *planner) cook(c plate.CellKey) error {
	main, ok := h.pickMain(c)
	if !ok && h.x.mealOf(c.SlotID) == mealFull && hasRecipes(h.s) {
		// No main can be cooked here (a library of standalone recipes, or a
		// diet that rules the mains out): a lunch or dinner without a main
		// breaks the rule the checker enforces, so the cell stays empty and
		// the checker reports it (H1).
		h.notes = append(h.notes, fmt.Sprintf(h.text(textNoMain), h.weekday(c.Date), slotName(h.s, c.SlotID)))
		h.choices = append(h.choices, heuristicChoice{Date: c.Date.Format(time.DateOnly), SlotID: c.SlotID, Reason: "left_empty_no_main"})
		return nil
	}
	if !ok {
		return fmt.Errorf("%w: no recipe for %s", domain.ErrNotEnoughRecipes, c.Date.Format(time.DateOnly))
	}
	f := h.s.Foods[main]
	keep := h.s.Settings.KeepDays(f.KeepDays)
	freezable := f.Freezable
	side := int64(0)
	if f.Role == food.RoleMain {
		// A light meal takes a side only while the plate stays within its
		// share.
		if id, ok := h.pickSide(c, main, nil, 0, ""); ok && !(h.s.lightMeal(c.Date, c.SlotID) && f.Macros.Kcal+h.s.Foods[id].Macros.Kcal > h.kcalShare(c)) {
			side = id
			sf := h.s.Foods[side]
			keep = min(keep, h.s.Settings.KeepDays(sf.KeepDays))
			freezable = freezable && sf.Freezable
		}
	}
	cook := plate.CookSession{FoodID: main, CookDate: c.Date, CookSlotID: c.SlotID}
	omni := f.DietClass == diet.Omnivore || (side != 0 && h.s.Foods[side].DietClass == diet.Omnivore)
	cover := h.cover(c, cook, keep, freezable, f.ReferencePortions, omni)

	reason := h.recipeReason(f)
	ps := ProposedSession{Key: fmt.Sprintf("s%d", len(h.prop.Sessions)+1), FoodID: main, CookDate: c.Date, CookSlotID: c.SlotID, Reason: reason}
	for _, cc := range cover {
		ps.Portions += cc.eaters
		if cc.frozen {
			ps.FrozenPortions += cc.eaters
		}
	}
	h.prop.Sessions = append(h.prop.Sessions, ps)
	sup := &supply{proposed: len(h.prop.Sessions) - 1, side: -1, food: main, cook: cook, keep: keep}
	if side != 0 {
		ss := ps
		ss.Key = fmt.Sprintf("s%d", len(h.prop.Sessions)+1)
		ss.FoodID = side
		ss.Reason = h.recipeReason(h.s.Foods[side])
		h.prop.Sessions = append(h.prop.Sessions, ss)
		sup.side = len(h.prop.Sessions) - 1
		h.used[side]++
	}
	h.supplies = append(h.supplies, sup)
	h.used[main]++
	switch {
	case f.Explore():
		h.explorePlaced++
	case f.LastEaten == nil && h.in.ExploreQuota > h.explorePlaced+h.fillPlaced:
		h.fillPlaced++
	}
	h.addOccasion(c)
	for _, cc := range cover {
		pp := ProposedPlate{Date: cc.cell.Date, SlotID: cc.cell.SlotID, Eaters: cc.eaters, Reason: reason}
		if !cc.cell.Equal(c) {
			pp.Reason = h.leftoverReason(c.Date)
		}
		pp.Components = append(pp.Components, ProposedComponent{FoodID: main, Portions: 1, SessionKey: ps.Key})
		if sup.side >= 0 {
			pp.Components = append(pp.Components, ProposedComponent{FoodID: side, Portions: 1, SessionKey: h.prop.Sessions[sup.side].Key})
		}
		code := "cook_new"
		if !cc.cell.Equal(c) {
			code = "leftover_of_new_batch"
		}
		h.place(pp, code)
	}
	return nil
}

// cookDays is the number of days with a cooking time so far.
func (h *planner) cookDays() int {
	n := 0
	for _, occ := range h.occasions {
		if len(occ) > 0 {
			n++
		}
	}
	return n
}

func (h *planner) addOccasion(c plate.CellKey) {
	if h.x.routine(c) {
		return
	}
	d := dayOf(c.Date)
	if h.occasions[d] == nil {
		h.occasions[d] = map[cellID]bool{}
	}
	h.occasions[d][idOf(c)] = true
}

type covered struct {
	cell   plate.CellKey
	eaters float64
	frozen bool
}

// cover is the cells a new session on c feeds: c, then (with batch cooking)
// the next open cells within keep days, or past them as frozen portions
// when freezing is on and the recipe freezable, until the recipe's
// reference portions are reached.
//
// A batch with an omnivore recipe skips cells whose day would be a new meat
// day past meat_days_per_week_max (H5).
func (h *planner) cover(c plate.CellKey, cook plate.CookSession, keep int, freezable bool, ref float64, omni bool) []covered {
	out := []covered{{cell: c, eaters: h.eaters(c.Date)}}
	if !h.s.Settings.BatchCooking {
		return out
	}
	total := out[0].eaters
	useBy := cook.CookDate.AddDate(0, 0, keep)
	freezeBy := cook.CookDate.AddDate(0, 0, h.s.Settings.FreezerKeepDays)
	meat := maps.Clone(h.meatDays)
	if omni && h.s.Window.Contains(c.Date) {
		meat[dayOf(c.Date)] = true
	}
	limit := h.s.Profile.MeatDaysPerWeekMax
	// S8: the batch stops growing once its dish is the main of as many
	// lunches and dinners as the setting allows.
	dishes := h.dishMeals(cook.FoodID)
	if h.x.mealOf(c.SlotID) == mealFull {
		dishes++
	}
	for _, o := range h.open {
		if total+epsilon >= ref {
			break
		}
		if h.done[idOf(o)] || !plate.Before(c, o, h.order) || h.isForced(o) {
			continue
		}
		full := h.x.mealOf(o.SlotID) == mealFull
		if full && dishes >= h.s.Settings.MaxDishMeals() {
			continue
		}
		if omni && limit != nil && h.s.Window.Contains(o.Date) && !meat[dayOf(o.Date)] {
			if len(meat)+1 > *limit {
				continue
			}
			meat[dayOf(o.Date)] = true
		}
		frozen := o.Date.After(useBy)
		if frozen && (!h.s.Settings.FreezingEnabled || !freezable || o.Date.After(freezeBy)) {
			break
		}
		n := h.eaters(o.Date)
		out = append(out, covered{cell: o, eaters: n, frozen: frozen})
		total += n
		if full {
			dishes++
		}
	}
	return out
}

// candidate is a recipe's rank for a cook cell: lower keys first.
type candidate struct {
	id   int64
	keys []float64
}

// pickMain ranks the recipes that may be cooked as a meal on c (spec §6.7
// step 2, D17): not yet in the proposal, not eaten within variety weeks
// (H8), not replaced, an explore recipe (newest first) while the explore
// quota is open, then never-eaten ones to fill a shortfall; never-eaten
// recipes last otherwise; known diet class, least recently eaten, favorite,
// liked, closest to the meal's kcal share, food id. Diet-incompatible recipes never
// qualify; one that adds a meat day past the limit only when nothing else
// does. A recipe a routine plate eats is never picked, and mains come before
// standalone and role-less recipes among equals (a breakfast recipe is not a
// lunch).
func (h *planner) pickMain(c plate.CellKey) (int64, bool) {
	span := 7 * h.s.Settings.VarietyWeeks
	share := h.kcalShare(c)
	light := h.s.lightMeal(c.Date, c.SlotID) || h.s.lightDay(c.Date)
	var cands []candidate
	dge := h.dgeState()
	for _, f := range h.s.Foods {
		if !h.mainCandidate(f, c) || !h.reachesForced(f, c) {
			continue
		}
		isNew := f.LastEaten == nil
		newKey, exploreOrder := b2f(isNew), 0.0
		if h.in.ExploreQuota > h.explorePlaced+h.fillPlaced {
			// Explore recipes first, newest food id (latest accept) first,
			// then never-eaten fillers, then the rest.
			switch {
			case f.Explore():
				newKey, exploreOrder = 0, -float64(f.ID)
			case isNew:
				newKey = 1
			default:
				newKey = 2
			}
		}
		last := 0.0
		if f.LastEaten != nil {
			last = float64(f.LastEaten.Unix())
		}
		fit := 0.0
		if share > 0 {
			fit = math.Abs(f.Macros.Kcal - share)
		}
		// A light cell asks for a recipe within its (reduced) share first.
		heavy := light && share > 0 && f.Macros.Kcal > share
		// S8: a dish already at its weekly cap comes only when nothing
		// else qualifies.
		atCap := h.x.mealOf(c.SlotID) == mealFull && h.dishFull(f.ID)
		cands = append(cands, candidate{id: f.ID, keys: []float64{
			b2f(h.s.tooSlow(f, c)), b2f(h.overAllowance(f, c)), b2f(atCap), b2f(h.overMeat(f, c)), b2f(heavy), float64(h.used[f.ID]), b2f(h.recent(f.ID, c.Date, span)), b2f(h.avoid[f.ID]), b2f(f.Role != food.RoleMain),
			newKey, exploreOrder, b2f(!diet.Valid(f.DietClass)), last, b2f(!f.Favorite), -f.LikeScore, math.Round(fit / dgeFitBucket), b2f(h.nearFamily(f, c)), dge.fish(f), dge.legume(f), dge.meat(f), fit,
		}})
	}
	return best(cands)
}

// batchPlates is the number of plates a recipe cooked on c would feed at most:
// its reference portions over the eaters of the day (1 without batch cooking).
// A cheap stand-in for cover, which the candidate loop cannot afford.
func (h *planner) batchPlates(f FoodFacts, c plate.CellKey) int {
	eaters := h.eaters(c.Date)
	if !h.s.Settings.BatchCooking || eaters <= 0 {
		return 1
	}
	return int((f.ReferencePortions + epsilon) / eaters)
}

// overAllowance reports whether cooking f on c takes longer than the kitchen's
// time allowance (KIT-1): the weekday one, the weekend one on Saturday and
// Sunday and on a batch cook day (a recipe for at least 3 plates). A soft
// preference only: it ranks below the wish's hard limit and above the rest.
// Unknown minutes never exceed it.
func (h *planner) overAllowance(f FoodFacts, c plate.CellKey) bool {
	p := h.s.Profile
	if !f.Composed() || f.Minutes <= 0 || p.MinutesWeekday == 0 {
		return false
	}
	limit := p.MinutesWeekday
	if wd := c.Date.Weekday(); wd == time.Saturday || wd == time.Sunday || h.batchPlates(f, c) >= batchDayPlates {
		limit = p.MinutesWeekend
	}
	return f.Minutes > limit
}

// batchDayPlates is the plates a session must feed to count as a batch cook
// day (KIT-1).
const batchDayPlates = 3

// pairInPlanner turns the add-on pairing rules on in the planner's own top-ups (tests switch it off
// to measure what it changes).
var pairInPlanner = true

// dgeFitBucket is the kcal width within which the kcal fit counts as equal,
// so the DGE keys decide only between recipes that fit the meal equally
// well: they never trade kcal fit for a food group. It is deliberately
// tight: any wider bucket (tried 2-60 kcal) re-ordered near-ties and
// regressed macro_daily_pct of replan_one_day by about +3 points.
const dgeFitBucket = 1

// dgeState is the DGE weekly food-group standing of the plan so far, for
// the tie-break of pickMain (see groups.go; soft, below kcal and protein).
type dgeState struct {
	needFish, needLegume bool
	meatG                float64
	limitG               float64
}

// dgeState counts what the fixed plates and the recipes cooked so far hold:
// a fish or legume main is wanted while none is planned, and meat is
// estimated as one portion per eater for each of a recipe's reference
// portions (a batch feeds that many cells).
func (h *planner) dgeState() dgeState {
	t := diet.Class(h.s.Profile.DietType)
	d := dgeState{needFish: t == "" || diet.Compatible(diet.Pescetarian, t), needLegume: true, limitG: h.meatLimitG}
	// note takes the groups of one portion eaten (g) and the portions of
	// meat that count against the week.
	note := func(g FoodGroups, meatPortions float64) {
		d.needFish = d.needFish && g.FishG < minMealG
		d.needLegume = d.needLegume && g.LegumeG < minMealG
		d.meatG += g.MeatG * meatPortions
	}
	for _, p := range h.s.Plates {
		if !h.s.Window.Contains(p.Date) {
			continue
		}
		var pg FoodGroups // the sum of the plate: a meal is judged on the plate
		for _, c := range p.Components {
			pg = pg.add(h.s.Foods[c.FoodID].Groups.times(c.Multiplier()))
		}
		note(pg, 1)
	}
	for _, id := range slices.Sorted(maps.Keys(h.used)) { // sorted: float sums are order-dependent
		f := h.s.Foods[id]
		note(f.Groups, float64(h.used[id])*f.ReferencePortions)
	}
	return d
}

// fish is 0 for a fish main (a fish meal, see minMealG) while the week has none (1 otherwise).
func (d dgeState) fish(f FoodFacts) float64 { return b2f(!(d.needFish && f.Groups.FishG >= minMealG)) }

// legume is 0 for a legume main while the week has none.
func (d dgeState) legume(f FoodFacts) float64 {
	return b2f(!(d.needLegume && f.Groups.LegumeG >= minMealG))
}

// meat is 1 for a recipe that takes the estimated week past the DGE limit
// prorated to the planned days (the guard, guardWeek, trims what is left).
func (d dgeState) meat(f FoodFacts) float64 {
	return b2f(f.Groups.MeatG > 0 && d.meatG+f.Groups.MeatG*f.ReferencePortions > d.limitG)
}

// mainCandidate reports whether f may be cooked as the main of c.
func (h *planner) mainCandidate(f FoodFacts, c plate.CellKey) bool {
	if !f.Composed() || !varietyRole(f.Role) || !h.fits(f) || h.routineFoods[f.ID] || h.exclude[f.ID] {
		return false
	}
	// A lunch or dinner takes a main; a standalone recipe is a breakfast.
	return isMain(f.Role, h.x.mealOf(c.SlotID))
}

func (h *planner) isForced(c plate.CellKey) bool {
	_, ok := h.forced[idOf(c)]
	return ok
}

// reachesForced is false for a recipe too short-lived to feed the stranded
// cell a forced cook on c is for; true when c is not forced.
func (h *planner) reachesForced(f FoodFacts, c plate.CellKey) bool {
	target, ok := h.forced[idOf(c)]
	return !ok || reaches(f, c, target, h.s.Settings)
}

// pickSide ranks the side recipes for main (beside the plate's others)
// cooked on c: of role when given (a swap keeps veg veg and starch starch
// unless none exists), within the meat limit, not replaced, with plate fit on
// not served on another plate of the day (sideToday), used least in the
// proposal (the week's side rotation), then with plate fit on: no clash
// with the plate, the least plate fat above the week's range (excessFat),
// the side whose kcal come closest to what the plate lacks to its share (in
// sideKcalBucket steps), the fewest raised gaps (PLATE-4, PLATE-5), then as
// pickMain. The swapped-out food, a side the household would rather not
// get (State.RatherNot: the plate goes without a side instead; addSide may
// still add one for nutrition) and, with plate fit on, the proposal's
// excluded foods are no candidate, nor is the main or a food the plate
// already holds (no plate carries the same food twice).
func (h *planner) pickSide(c plate.CellKey, main int64, others []int64, exclude int64, role food.Role) (int64, bool) {
	fit := h.s.plateFitOn(main)
	need := 0.0               // with plate fit on: the kcal the plate lacks to its share
	var held nutrition.Macros // with plate fit on: the plate without the side
	if fit {
		for _, id := range append([]int64{main}, others...) {
			held.Kcal += h.s.Foods[id].Macros.Kcal
			held.Fat += h.s.Foods[id].Macros.Fat
		}
		if share := h.kcalShare(c); share > 0 {
			need = share
			for _, id := range append([]int64{main}, others...) {
				need -= h.s.Foods[id].Macros.Kcal
			}
		}
	}
	var cands []candidate
	for _, f := range h.s.Foods {
		if !f.Composed() || !sideRole(f.Role) || f.ID == exclude || f.ID == main || slices.Contains(others, f.ID) || (fit && h.exclude[f.ID]) || h.s.RatherNot[f.ID] || !h.fits(f) || !h.reachesForced(f, c) {
			continue
		}
		last := 0.0
		if f.LastEaten != nil {
			last = float64(f.LastEaten.Unix())
		}
		keys := []float64{b2f(role != "" && f.Role != role), b2f(h.overMeat(f, c)), b2f(h.avoid[f.ID])}
		if fit {
			// With plate fit on, a side the day already serves on another
			// plate comes after every other (no side twice a day).
			keys = append(keys, b2f(h.sideToday(f.ID, c)))
		}
		keys = append(keys, float64(h.used[f.ID]))
		if fit {
			clash, raised := h.s.sideFit(main, others, f)
			if clash {
				continue
			}
			fat := excessFat(nutrition.Macros{Kcal: held.Kcal + f.Macros.Kcal, Fat: held.Fat + f.Macros.Fat}, 1, h.x.pol.FatHigh)
			kcalKey := 0.0
			if need > 0 {
				kcalKey = math.Round(math.Abs(need-f.Macros.Kcal) / sideKcalBucket)
			}
			keys = append(keys, fat, kcalKey, float64(raised))
		}
		cands = append(cands, candidate{id: f.ID, keys: append(keys,
			b2f(!diet.Valid(f.DietClass)), last, b2f(!f.Favorite), -f.LikeScore,
		)})
	}
	return best(cands)
}

// sideToday reports whether food id is on another plate of c's day than c:
// a confirmed plate of the state or a plate of the proposal so far.
func (h *planner) sideToday(id int64, c plate.CellKey) bool {
	holds := func(d time.Time, slot int64, has func(int64) bool) bool {
		return dayOf(d) == dayOf(c.Date) && slot != c.SlotID && has(id)
	}
	for _, p := range h.prop.Plates {
		if holds(p.Date, p.SlotID, func(id int64) bool {
			return slices.ContainsFunc(p.Components, func(pc ProposedComponent) bool { return pc.FoodID == id })
		}) {
			return true
		}
	}
	for _, p := range h.s.Plates {
		if p.Confirmed && holds(p.Date, p.SlotID, func(id int64) bool {
			return slices.ContainsFunc(p.Components, func(pc Component) bool { return pc.FoodID == id })
		}) {
			return true
		}
	}
	return false
}

func best(cands []candidate) (int64, bool) {
	if len(cands) == 0 {
		return 0, false
	}
	slices.SortFunc(cands, func(a, b candidate) int {
		for i := range a.keys {
			if c := cmp.Compare(a.keys[i], b.keys[i]); c != 0 {
				return c
			}
		}
		return cmp.Compare(a.id, b.id)
	})
	return cands[0].id, true
}

// fits reports whether f may be planned for the profile's diet type: an
// unknown class is only a warning (H5), so it fits.
func (h *planner) fits(f FoodFacts) bool {
	t := diet.Class(h.s.Profile.DietType)
	return t == "" || !diet.Valid(f.DietClass) || diet.Compatible(f.DietClass, t)
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func sideRole(r food.Role) bool {
	switch r {
	case food.RoleSideStarch, food.RoleSideVeg, food.RoleSideProtein:
		return true
	}
	return false
}

// recent reports whether food was eaten or cooked within span days before
// date, or anywhere later in the state (H8).
func (h *planner) recent(foodID int64, date time.Time, span int) bool {
	if span <= 0 {
		return false
	}
	since := date.AddDate(0, 0, -span)
	if le := h.s.Foods[foodID].LastEaten; le != nil && !le.Before(since) {
		return true
	}
	for _, d := range h.seen[foodID] {
		if !d.Before(since) {
			return true
		}
	}
	return false
}

// overMeat reports whether cooking f on c adds a meat day past
// meat_days_per_week_max (H5).
func (h *planner) overMeat(f FoodFacts, c plate.CellKey) bool {
	limit := h.s.Profile.MeatDaysPerWeekMax
	if limit == nil || f.DietClass != diet.Omnivore || h.meatDays[dayOf(c.Date)] || !h.s.Window.Contains(c.Date) {
		return false
	}
	return len(h.meatDays)+1 > *limit
}

// kcalShare is one meal's share of the daily kcal target on the cell's day:
// the target over the day's cells that need a plate; 0 without a target. A
// light day asks LightDayShare of it, a light meal LightMealShare of the
// meal's normal share (the evaluator's cap, mealCap).
func (h *planner) kcalShare(c plate.CellKey) float64 {
	t := h.s.Profile.KcalTarget
	if t == nil || *t <= 0 {
		return 0
	}
	var cells []Cell
	n := 0
	for _, o := range h.s.Cells {
		if dayOf(o.Date) == dayOf(c.Date) {
			cells = append(cells, o)
			if o.Mode != availability.ModeNone {
				n++
			}
		}
	}
	if n == 0 {
		return 0
	}
	// The meal aims at its slot's expected share of the day's full target (SB2).
	// A routine meal that misses its own share moves it (SB5).
	weights, total := daySlotShares(h.s, cells)
	share := *t * weights[c.SlotID] / total
	if accounts := h.x.dayAccounts([]time.Time{c.Date}); len(accounts) > 0 {
		share = accounts[0].mealShare(c.SlotID)
	}
	switch {
	case h.s.lightMeal(c.Date, c.SlotID):
		return LightMealShare * share
	case h.s.heartyMeal(c.Date, c.SlotID):
		return HeartyMealShare * share
	case h.s.lightDay(c.Date):
		return share * LightDayShare
	case h.s.heartyDay(c.Date):
		return share * HeartyDayShare
	}
	return share
}

// side swaps a plate's side (the side scope): a new side recipe cooked at
// the old side's occasion with its portions, on every plate that ate the old
// one.
func (h *planner) side(sw SideSwap) (Proposal, error) {
	c := plate.CellKey{Date: sw.CookDate, SlotID: sw.CookSlotID}
	h.avoid[sw.FoodID] = true
	main, others := h.swapPlate(sw)
	id, ok := h.pickSide(c, main, others, sw.FoodID, sw.Role)
	if !ok {
		return Proposal{}, fmt.Errorf("%w: no other side", domain.ErrNotEnoughRecipes)
	}
	reason := h.text(textDifferentSide)
	ps := ProposedSession{Key: "s1", FoodID: id, CookDate: sw.CookDate, CookSlotID: sw.CookSlotID, Portions: sw.Portions, FrozenPortions: sw.FrozenPortions, Reason: reason}
	p := Proposal{Sessions: []ProposedSession{ps}}
	h.choices = append(h.choices, heuristicChoice{Date: sw.CookDate.Format(time.DateOnly), SlotID: sw.CookSlotID, Foods: []int64{id}, Reason: "side_swap"})
	for _, sp := range sw.Plates {
		p.Plates = append(p.Plates, ProposedPlate{
			PlateID: sp.PlateID, Date: sp.Date, SlotID: sp.SlotID, Eaters: sp.Eaters, Reason: reason,
			Components: []ProposedComponent{{FoodID: id, Portions: sp.PerEater, SessionKey: ps.Key}},
		})
	}
	return p, nil
}

// swapPlate is the main and the other sides of the first plate that eats
// the replaced side (main 0 when the state does not hold it).
func (h *planner) swapPlate(sw SideSwap) (int64, []int64) {
	for _, p := range h.s.Plates {
		if len(sw.Plates) == 0 || p.ID != sw.Plates[0].PlateID {
			continue
		}
		main := h.s.mainOf(p)
		var others []int64
		for _, c := range p.Components {
			if c.FoodID != sw.FoodID && c.FoodID != main {
				others = append(others, c.FoodID)
			}
		}
		return main, others
	}
	return 0, nil
}

// Reasons ("Why this?", D10) the heuristic writes, in the profile's locale
// (English unless German).
type textKey int

const (
	textNew textKey = iota
	textFavorite
	textLongest
	textLeftovers
	textDifferentSide
	textExploreShortfall
	textMeatOver
	textNoMain
	textCookMoved
	textCookDays
)

var texts = map[string]map[textKey]string{
	"en": {
		textNew:              "New: never eaten yet",
		textFavorite:         "A favorite you haven't had in a while",
		textLongest:          "Not eaten for the longest time",
		textLeftovers:        "Leftovers from %s",
		textDifferentSide:    "A different side",
		textExploreShortfall: "Explore quota: %d of %d explore recipes available, filled with %d never-eaten",
		textMeatOver:         "%s is a meat day past the limit of %d per week: no other recipe fit",
		textNoMain:           "No main recipe available for %s %s: the cell stays empty",
		textCookMoved:        "%s now cooked %s %s",
		textCookDays:         "%s needed a cooking day beyond your limit of %d: without it a meal would stay empty",
	},
	"de": {
		textNew:              "Neu: noch nie gegessen",
		textFavorite:         "Ein Lieblingsgericht, das es länger nicht gab",
		textLongest:          "Am längsten nicht gegessen",
		textLeftovers:        "Reste von %s",
		textDifferentSide:    "Eine andere Beilage",
		textExploreShortfall: "Entdecker-Quote: %d von %d Entdecker-Rezepten verfügbar, aufgefüllt mit %d bisher nie gegessenen Rezepten",
		textMeatOver:         "%s ist ein Fleischtag über dem Wochenlimit von %d: kein anderes Rezept passte",
		textNoMain:           "Kein Hauptgericht verfügbar für %s %s: die Zelle bleibt leer",
		textCookMoved:        "%s wird jetzt %s %s gekocht",
		textCookDays:         "%s brauchte einen Kochtag über deinem Limit von %d: sonst bliebe eine Mahlzeit leer",
	},
}

var weekdays = map[string][7]string{
	"en": {"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
	"de": {"Sonntag", "Montag", "Dienstag", "Mittwoch", "Donnerstag", "Freitag", "Samstag"},
}

func (h *planner) lang() string {
	if h.in.Locale == "de" {
		return "de"
	}
	return "en"
}

func (h *planner) text(k textKey) string { return texts[h.lang()][k] }

func (h *planner) weekday(d time.Time) string { return weekdays[h.lang()][d.Weekday()] }

// hasRecipes reports whether the library holds any recipe.
func hasRecipes(s State) bool {
	for _, f := range s.Foods {
		if f.Composed() {
			return true
		}
	}
	return false
}

func (h *planner) recipeReason(f FoodFacts) string {
	switch {
	case f.LastEaten == nil:
		return h.text(textNew)
	case f.Favorite:
		return h.text(textFavorite)
	}
	return h.text(textLongest)
}

func (h *planner) leftoverReason(cookDate time.Time) string {
	return fmt.Sprintf(h.text(textLeftovers), weekdays[h.lang()][cookDate.Weekday()])
}

// distinctDays is the number of days the target's cells span.
func distinctDays(t Target) int {
	days := map[int]bool{}
	for _, c := range t.Cells {
		days[dayOf(c.Date)] = true
	}
	return len(days)
}
