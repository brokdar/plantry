package planning

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
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
func (Heuristic) Choose(_ context.Context, in ChooseInput) (Proposal, error) {
	h := newPlanner(in)
	if in.Side != nil {
		return h.side(*in.Side)
	}
	return h.plan()
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
	seen      map[int64][]time.Time
	used      map[int64]int
	avoid     map[int64]bool
	newPlaced bool
}

func newPlanner(in ChooseInput) *planner {
	s := in.State
	h := &planner{
		in: in, s: s, x: newIndex(s, in.Target), order: s.SlotOrder,
		done: map[cellID]bool{}, occasions: map[int]map[cellID]bool{},
		meatDays: map[int]bool{}, seen: map[int64][]time.Time{}, used: map[int64]int{},
		avoid: idSet(in.Avoid),
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
		if sup := h.supplyFor(c, eaters); sup != nil {
			h.eat(sup, c, eaters, false)
			continue
		}
		// Cook on the day's last open cell (dinner, spec §6.7 step 1): an
		// earlier cell eats from the latest proposed session that may still
		// feed it, which cooks more; so does any cell of a day that may not
		// cook again. Only a cell nothing can feed cooks early.
		limit := h.s.Settings.MaxCookOccasionsPerDay
		maxed := limit > 0 && len(h.occasions[dayOf(c.Date)]) >= limit
		if h.s.Settings.BatchCooking && (maxed || !h.lastOpenOfDay(c)) {
			if sup := h.growable(c); sup != nil {
				h.eat(sup, c, eaters, true)
				continue
			}
		}
		if err := h.cook(c); err != nil {
			return Proposal{}, err
		}
	}
	return h.prop, nil
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
	var best *supply
	for _, sup := range h.supplies {
		k, ok := h.kindAt(sup, c)
		if !ok || k == plate.KindFresh {
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
		if k, ok := h.kindAt(sup, c); !ok || k != plate.KindLeftover {
			continue
		}
		if best == nil || plate.Before(best.cook.CookCell(), sup.cook.CookCell(), h.order) {
			best = sup
		}
	}
	return best
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
	if sup.proposed >= 0 {
		pp.Components = append(pp.Components, ProposedComponent{FoodID: sup.food, Portions: 1, SessionKey: h.prop.Sessions[sup.proposed].Key})
		if sup.side >= 0 {
			side := h.prop.Sessions[sup.side]
			pp.Components = append(pp.Components, ProposedComponent{FoodID: side.FoodID, Portions: 1, SessionKey: side.Key})
		}
	} else {
		pp.Components = append(pp.Components, ProposedComponent{FoodID: sup.food, Portions: 1, SessionID: sup.id})
	}
	h.place(pp)
}

func (h *planner) place(pp ProposedPlate) {
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
	if !ok {
		return fmt.Errorf("%w: no recipe for %s", domain.ErrNotEnoughRecipes, c.Date.Format(time.DateOnly))
	}
	f := h.s.Foods[main]
	keep := h.s.Settings.KeepDays(f.KeepDays)
	freezable := f.Freezable
	side := int64(0)
	if f.Role == food.RoleMain {
		if id, ok := h.pickSide(c, 0); ok {
			side = id
			sf := h.s.Foods[side]
			keep = min(keep, h.s.Settings.KeepDays(sf.KeepDays))
			freezable = freezable && sf.Freezable
		}
	}
	cook := plate.CookSession{FoodID: main, CookDate: c.Date, CookSlotID: c.SlotID}
	cover := h.cover(c, cook, keep, freezable, f.ReferencePortions)

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
	if f.LastEaten == nil {
		h.newPlaced = true
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
		h.place(pp)
	}
	return nil
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
func (h *planner) cover(c plate.CellKey, cook plate.CookSession, keep int, freezable bool, ref float64) []covered {
	out := []covered{{cell: c, eaters: h.eaters(c.Date)}}
	if !h.s.Settings.BatchCooking {
		return out
	}
	total := out[0].eaters
	useBy := cook.CookDate.AddDate(0, 0, keep)
	freezeBy := cook.CookDate.AddDate(0, 0, h.s.Settings.FreezerKeepDays)
	for _, o := range h.open {
		if total+epsilon >= ref {
			break
		}
		if h.done[idOf(o)] || !plate.Before(c, o, h.order) {
			continue
		}
		frozen := o.Date.After(useBy)
		if frozen && (!h.s.Settings.FreezingEnabled || !freezable || o.Date.After(freezeBy)) {
			break
		}
		n := h.eaters(o.Date)
		out = append(out, covered{cell: o, eaters: n, frozen: frozen})
		total += n
	}
	return out
}

// candidate is a recipe's rank for a cook cell: lower keys first.
type candidate struct {
	id   int64
	keys []float64
}

// pickMain ranks the recipes that may be cooked as a meal on c (spec §6.7
// step 2, D15, D17): not yet in the proposal, not eaten within variety
// weeks (H8), not replaced, a never-eaten recipe first when the request asks for one
// (else last), known diet class, least recently eaten, favorite, liked,
// closest to the meal's kcal share, food id. Diet-incompatible recipes never
// qualify; one that adds a meat day past the limit only when nothing else
// does.
func (h *planner) pickMain(c plate.CellKey) (int64, bool) {
	span := 7 * h.s.Settings.VarietyWeeks
	share := h.kcalShare(c.Date)
	var cands []candidate
	for _, f := range h.s.Foods {
		if !f.Composed() || !varietyRole(f.Role) || !h.fits(f) {
			continue
		}
		isNew := f.LastEaten == nil
		newKey := b2f(isNew)
		if h.in.IncludeNewRecipe && !h.newPlaced {
			newKey = b2f(!isNew)
		}
		last := 0.0
		if f.LastEaten != nil {
			last = float64(f.LastEaten.Unix())
		}
		fit := 0.0
		if share > 0 {
			fit = math.Abs(f.Macros.Kcal - share)
		}
		cands = append(cands, candidate{id: f.ID, keys: []float64{
			b2f(h.overMeat(f, c)), float64(h.used[f.ID]), b2f(h.recent(f.ID, c.Date, span)), b2f(h.avoid[f.ID]),
			newKey, b2f(!diet.Valid(f.DietClass)), last, b2f(!f.Favorite), -f.LikeScore, fit,
		}})
	}
	return best(cands)
}

// pickSide ranks the side recipes for a main cooked on c: not replaced, not
// the excluded food, used least in the proposal, then as pickMain.
func (h *planner) pickSide(c plate.CellKey, exclude int64) (int64, bool) {
	var cands []candidate
	for _, f := range h.s.Foods {
		if !f.Composed() || !sideRole(f.Role) || f.ID == exclude || !h.fits(f) {
			continue
		}
		last := 0.0
		if f.LastEaten != nil {
			last = float64(f.LastEaten.Unix())
		}
		cands = append(cands, candidate{id: f.ID, keys: []float64{
			b2f(h.overMeat(f, c)), b2f(h.avoid[f.ID]), float64(h.used[f.ID]), b2f(!diet.Valid(f.DietClass)),
			last, b2f(!f.Favorite), -f.LikeScore,
		}})
	}
	return best(cands)
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

// kcalShare is one meal's share of the daily kcal target on date: the
// target over the date's cells that need a plate; 0 without a target.
func (h *planner) kcalShare(date time.Time) float64 {
	t := h.s.Profile.KcalTarget
	if t == nil || *t <= 0 {
		return 0
	}
	n := 0
	for _, c := range h.s.Cells {
		if dayOf(c.Date) == dayOf(date) && c.Mode != availability.ModeNone {
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return *t / float64(n)
}

// side swaps a plate's side (the side scope): a new side recipe cooked at
// the old side's occasion with its portions, on every plate that ate the old
// one.
func (h *planner) side(sw SideSwap) (Proposal, error) {
	c := plate.CellKey{Date: sw.CookDate, SlotID: sw.CookSlotID}
	h.avoid[sw.FoodID] = true
	id, ok := h.pickSide(c, sw.FoodID)
	if !ok {
		return Proposal{}, fmt.Errorf("%w: no other side", domain.ErrNotEnoughRecipes)
	}
	reason := h.text(textDifferentSide)
	ps := ProposedSession{Key: "s1", FoodID: id, CookDate: sw.CookDate, CookSlotID: sw.CookSlotID, Portions: sw.Portions, FrozenPortions: sw.FrozenPortions, Reason: reason}
	p := Proposal{Sessions: []ProposedSession{ps}}
	for _, sp := range sw.Plates {
		p.Plates = append(p.Plates, ProposedPlate{
			PlateID: sp.PlateID, Date: sp.Date, SlotID: sp.SlotID, Eaters: sp.Eaters, Reason: reason,
			Components: []ProposedComponent{{FoodID: id, Portions: sp.PerEater, SessionKey: ps.Key}},
		})
	}
	return p, nil
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
)

var texts = map[string]map[textKey]string{
	"en": {
		textNew:           "New: never eaten yet",
		textFavorite:      "A favorite you haven't had in a while",
		textLongest:       "Not eaten for the longest time",
		textLeftovers:     "Leftovers from %s",
		textDifferentSide: "A different side",
	},
	"de": {
		textNew:           "Neu: noch nie gegessen",
		textFavorite:      "Ein Lieblingsgericht, das es länger nicht gab",
		textLongest:       "Am längsten nicht gegessen",
		textLeftovers:     "Reste von %s",
		textDifferentSide: "Eine andere Beilage",
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
