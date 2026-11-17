package planning

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/settings"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// What an alternative replaces or adds on a plate.
const (
	// AltMeal replaces the main and drops every other component.
	AltMeal = "meal"
	// AltMain replaces only the main.
	AltMain = "main"
	// AltSide replaces the given or the only side.
	AltSide = "side"
	// AltAdd adds a side, extra or snack.
	AltAdd = "add"
)

// Alternative sources.
const (
	SourceFresh   = "fresh"
	SourceSession = "session"
	SourceLeaf    = "leaf"
	// SourceCookAt cooks the recipe fresh on another cell that already cooks
	// (or, failing that, an earlier day with a free occasion), so the day of
	// the plate gets no new cooking occasion.
	SourceCookAt = "cook_at"
)

// altSessionKey is the key of the session a cook_at candidate adds to the
// copy it evaluates.
const altSessionKey = "alt"

const (
	defaultAltLimit = 5
	maxAltLimit     = 10
)

// AltRequest asks for alternatives on one plate.
type AltRequest struct {
	PlateID int64
	What    string
	// ComponentID names the component to replace (main or side); optional.
	ComponentID int64
	Constraints AltConstraints
	// Limit is the number of results: default 5, at most 10.
	Limit int
}

// AltConstraints narrow the candidates. Zero values do not filter.
type AltConstraints struct {
	// MaxMinutes bounds a fresh cook's prep + cook minutes; unknown minutes
	// fail it (a session or a leaf needs no cooking and passes).
	MaxMinutes int
	// Diet is a diet class the candidate must fit (on top of the profile's).
	Diet string
	// Roles: the candidate's role (a leaf's effective role) is one of them.
	Roles []food.Role
	// Search matches every word against name and tags as find_foods does.
	Search string
	// Tags: the candidate carries every one.
	Tags []string
	// MinProteinG and MaxKcal bound the new component, per eater.
	MinProteinG float64
	MaxKcal     float64
	NoCook      bool
}

// Alternative is one verified way to change the plate. Figures are per
// eater, for the plate and its day after the change.
type Alternative struct {
	FoodID    int64  `json:"food_id"`
	Name      string `json:"name"`
	Source    string `json:"source"`
	SessionID *int64 `json:"session_id,omitempty"`
	// CookAt is the cell a cook_at source cooks on.
	CookAt *OptionCell `json:"cook_at,omitempty"`
	// TotalMinutes is the fresh or cook_at cook's prep + cook minutes (0
	// otherwise).
	TotalMinutes int     `json:"total_minutes"`
	Kcal         float64 `json:"kcal"`
	ProteinG     float64 `json:"protein_g"`
	DayKcal      float64 `json:"day_kcal"`
	// DayPct and WeekPct are the day's kcal against its target and the
	// weekly ledger's pct (0 without a target).
	DayPct           float64 `json:"day_pct"`
	WeekPct          float64 `json:"week_pct"`
	CookOccasionsDay int     `json:"cook_occasions_day"`
	// NewIssues are the soft issues the change adds (without options).
	NewIssues []Issue `json:"new_issues"`
	// Ops are the apply_changes ops that make the change, in order.
	Ops []OptionOp `json:"ops"`

	rank altRank
}

// altRank is the ranking key, compared field by field (lower first).
type altRank struct {
	newIssues int
	addsCook  bool
	// source orders a session before a cook_at before a fresh cook or leaf.
	source    int
	dishMeals int
	sameRole  bool
	kcalFit   float64
	lastEaten int64 // unix days; never eaten sorts first
}

// AltResult is the outcome of Alternatives. EmptyReason is set only when
// no candidate is left.
type AltResult struct {
	Alternatives []Alternative `json:"alternatives"`
	EmptyReason  *EmptyReason  `json:"empty_reason,omitempty"`
}

// EmptyReason says why nothing fits: how many candidates fit the position
// and constraints (0: none, loosen a constraint) and how many of them each
// new hard issue dropped, keyed "rule.code" (a candidate counts once, under
// its first new hard issue).
type EmptyReason struct {
	Candidates int            `json:"candidates"`
	Dropped    map[string]int `json:"dropped,omitempty"`
}

// Alternatives returns up to req.Limit candidates for req's plate, each
// applied to a copy of s and re-evaluated: a candidate that adds a hard
// issue is dropped. Ranked by fewest new soft issues, no new cooking
// occasion, source (a session, then a cook_at, then a fresh cook), the
// main's dish count after the change, no second side of the same role, kcal
// fit to a light or hearty cell, then the oldest last eaten. A recipe passing
// both fresh and cook_at is listed once, by the better rank. Pure: nothing
// writes.
func Alternatives(s State, t Target, req AltRequest) (AltResult, error) {
	pi := slices.IndexFunc(s.Plates, func(p Plate) bool { return p.ID == req.PlateID })
	if req.PlateID <= 0 || pi < 0 || !s.Window.Contains(s.Plates[pi].Date) {
		return AltResult{}, fmt.Errorf("%w: plate %d", domain.ErrNotFound, req.PlateID)
	}
	p := s.Plates[pi]
	if p.Confirmed {
		return AltResult{}, fmt.Errorf("%w: plate %d is confirmed", domain.ErrInvalidInput, p.ID)
	}
	old, drop, err := s.altTarget(p, req)
	if err != nil {
		return AltResult{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultAltLimit
	}
	limit = min(limit, maxAltLimit)

	base := Evaluate(s, t)
	hard, soft := hardKeys(base.Issues), softKeys(base.Issues)
	x := newIndex(s, t)
	baseCooks := x.occasions[dayOf(p.Date)]
	var out []Alternative
	cands := s.altCandidates(x, p, old, req)
	dropped := map[string]int{}
	for _, c := range cands {
		a, why := s.tryAlt(t, pi, old, drop, c, hard, soft, len(baseCooks))
		if why != "" {
			dropped[why]++
			continue
		}
		out = append(out, a)
	}
	slices.SortStableFunc(out, func(a, b Alternative) int {
		ra, rb := a.rank, b.rank
		return cmp.Or(
			cmp.Compare(ra.newIssues, rb.newIssues),
			boolCmp(ra.addsCook, rb.addsCook),
			cmp.Compare(ra.source, rb.source),
			cmp.Compare(ra.dishMeals, rb.dishMeals),
			boolCmp(ra.sameRole, rb.sameRole),
			cmp.Compare(ra.kcalFit, rb.kcalFit),
			cmp.Compare(ra.lastEaten, rb.lastEaten),
			cmp.Compare(a.FoodID, b.FoodID),
			cmp.Compare(ptrVal(a.SessionID), ptrVal(b.SessionID)),
		)
	})
	// One cooking way per recipe: the better of fresh and cook_at.
	cooked := map[int64]bool{}
	out = slices.DeleteFunc(out, func(a Alternative) bool {
		if a.Source != SourceFresh && a.Source != SourceCookAt {
			return false
		}
		if cooked[a.FoodID] {
			return true
		}
		cooked[a.FoodID] = true
		return false
	})
	if len(out) > limit {
		out = out[:limit]
	}
	res := AltResult{Alternatives: out}
	if len(out) == 0 {
		res.EmptyReason = &EmptyReason{Candidates: len(cands)}
		if len(dropped) > 0 {
			res.EmptyReason.Dropped = dropped
		}
	}
	return res, nil
}

// Alternatives builds the window at start (a shopping day) the way the
// generator does, in one unit of work, and returns Alternatives for req over
// every cell of the window (POST /api/plan/alternatives). Nothing writes.
func (s *Service) Alternatives(ctx context.Context, start, today time.Time, req AltRequest) (AltResult, error) {
	var out AltResult
	err := s.d.Tx.InTx(ctx, func(ctx context.Context) error {
		sd, err := plate.IntSetting(ctx, s.d.Settings, settings.KeyPlanShoppingDay)
		if err != nil {
			return err
		}
		start := dateOnly(start)
		if !window.IsStart(start, sd) {
			return fmt.Errorf("%w: %s", domain.ErrInvalidPlanStart, start.Format(time.DateOnly))
		}
		w := window.Containing(start, sd)
		st, err := s.d.Builder.Build(ctx, w, dateOnly(today), w.Start, w.End)
		if err != nil {
			return err
		}
		out, err = Alternatives(st, WindowTarget(st), req)
		return err
	})
	return out, err
}

// altTarget resolves the component req replaces (-1 for add) and the other
// components a meal swap drops.
func (s State) altTarget(p Plate, req AltRequest) (int, []int, error) {
	mainID := s.mainOf(p)
	isMainComp := func(i int) bool {
		c := p.Components[i]
		return c.Position == plate.PositionMain || (c.Position == "" && c.FoodID == mainID)
	}
	byID := func() (int, error) {
		i := slices.IndexFunc(p.Components, func(c Component) bool { return c.ID == req.ComponentID })
		if i < 0 {
			return -1, fmt.Errorf("%w: plate_component %d on plate %d", domain.ErrNotFound, req.ComponentID, p.ID)
		}
		return i, nil
	}
	switch req.What {
	case AltMeal, AltMain:
		i := -1
		if req.ComponentID != 0 {
			var err error
			if i, err = byID(); err != nil {
				return -1, nil, err
			}
		} else if mainID != 0 {
			for j := range p.Components {
				if isMainComp(j) {
					i = j
					break
				}
			}
		}
		if i < 0 {
			return -1, nil, fmt.Errorf("%w: plate %d has no main", domain.ErrInvalidInput, p.ID)
		}
		if f, ok := s.Foods[p.Components[i].FoodID]; ok && !f.Composed() {
			return -1, nil, fmt.Errorf("%w: plate_component %d is a leaf, not a main; use what=side or add", domain.ErrInvalidInput, p.Components[i].ID)
		}
		var drop []int
		if req.What == AltMeal {
			for j := range p.Components {
				if j != i {
					drop = append(drop, j)
				}
			}
		}
		return i, drop, nil
	case AltSide:
		if req.ComponentID != 0 {
			i, err := byID()
			return i, nil, err
		}
		var sides []int
		for j := range p.Components {
			if !isMainComp(j) {
				sides = append(sides, j)
			}
		}
		if len(sides) != 1 {
			return -1, nil, fmt.Errorf("%w: plate %d has %d sides, name plate_component_id", domain.ErrInvalidInput, p.ID, len(sides))
		}
		return sides[0], nil, nil
	case AltAdd:
		return -1, nil, nil
	}
	return -1, nil, fmt.Errorf("%w: what must be meal, main, side or add", domain.ErrInvalidInput)
}

// altCand is one candidate before verification.
type altCand struct {
	food    FoodFacts
	session *Session
	// grow is what an exactly covering session grows by when the plate
	// joins it (D11).
	grow float64
	// cookAt is the cook cell of a cook_at candidate.
	cookAt *plate.CellKey
	comp   Component
}

// altCandidates lists the foods that fit req's position and constraints,
// each fresh (or as a leaf) and once per batch of it that can feed the cell.
func (s State) altCandidates(x *index, p Plate, old int, req AltRequest) []altCand {
	onPlate := map[int64]bool{}
	for _, c := range p.Components {
		onPlate[c.FoodID] = true
	}
	var oldComp *Component
	if old >= 0 {
		oldComp = &p.Components[old]
	}
	kind := x.mealOf(p.SlotID)
	// An omnivore household eats any class (unknown too, as H5 warns only);
	// a restricted one needs a compatible class.
	var household diet.Class
	if DietRestricts(s.Profile.DietType) {
		household = diet.Class(s.Profile.DietType)
	}
	var words [][]diet.Alt
	for _, w := range strings.Fields(req.Constraints.Search) {
		words = append(words, diet.SearchAlternatives(w))
	}
	ids := make([]int64, 0, len(s.Foods))
	for id := range s.Foods {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var out []altCand
	for _, id := range ids {
		f := s.Foods[id]
		if onPlate[id] || !fitsPosition(f, req.What, kind) || !fitsDiet(f.DietClass, household) ||
			(req.Constraints.Diet != "" && !fitsDiet(f.DietClass, diet.Class(req.Constraints.Diet))) ||
			!req.Constraints.matches(f, words) {
			continue
		}
		comp := Component{FoodID: id, Proposed: true}
		switch {
		case f.Composed():
			n := 1.0
			if oldComp != nil && oldComp.Portions != nil && *oldComp.Portions > 0 {
				n = *oldComp.Portions
			}
			comp.Portions = &n
		default:
			g := float64(TypicalLeafGrams)
			if oldComp != nil && oldComp.Grams != nil && *oldComp.Grams > 0 {
				g = *oldComp.Grams
			}
			comp.Grams = &g
		}
		if oldComp != nil {
			comp.ID, comp.Position = oldComp.ID, oldComp.Position
		}
		m := comp.Multiplier()
		if (req.Constraints.MinProteinG > 0 && f.Macros.Protein*m < req.Constraints.MinProteinG) ||
			(req.Constraints.MaxKcal > 0 && f.Macros.Kcal*m > req.Constraints.MaxKcal) {
			continue
		}
		// Unknown minutes (0) never pass max_minutes, as in find_foods.
		if !f.Composed() || req.Constraints.MaxMinutes <= 0 || (f.Minutes > 0 && f.Minutes <= req.Constraints.MaxMinutes) {
			out = append(out, altCand{food: f, comp: comp})
			if f.Composed() {
				if k, ok := s.cookAtCell(x, p, id); ok {
					out = append(out, altCand{food: f, cookAt: &k, comp: comp})
				}
			}
		}
		if !f.Composed() {
			continue
		}
		for i := range s.Sessions {
			ss := &s.Sessions[i]
			if ss.ID <= 0 || ss.FoodID != id || !s.cookedBy(ss.CookSession, p) ||
				ss.CookDate.AddDate(0, 0, x.keepDays(id)).Before(p.Date) {
				continue
			}
			// A batch that exactly covers its meals grows with the plate
			// (D11); one with a surplus must hold the plate's portions.
			need, spare := comp.Multiplier()*p.Eaters, ss.Portions-x.usage(*ss).Total
			var grow float64
			switch {
			case math.Abs(spare) < plate.PortionEpsilon:
				grow = need
			case spare < need:
				continue
			}
			sc := comp
			sc.ID, sc.SessionID = 0, ss.ID
			out = append(out, altCand{food: f, session: ss, grow: grow, comp: sc})
		}
	}
	return out
}

// cookAtCell picks where a cook_at candidate of recipe foodID cooks for p
// when p's own cell does not cook yet: the latest cell that already cooks on
// or before p's cell, else the latest plan cell of an earlier day with a free
// cooking occasion. Either lies in the window, not before today, within the
// recipe's keep days of p, and is not p's cell.
func (s State) cookAtCell(x *index, p Plate, foodID int64) (plate.CellKey, bool) {
	own := idOf(p.Cell())
	if x.occasions[own.day][own] {
		return plate.CellKey{}, false
	}
	keep := x.keepDays(foodID)
	var best, free *Cell
	for i := range s.Cells {
		c := &s.Cells[i]
		k := c.Key()
		if c.Mode != availability.ModePlan || idOf(k) == own || dayOf(c.Date) < dayOf(s.Today) ||
			c.Date.AddDate(0, 0, keep).Before(p.Date) ||
			!s.cookedBy(plate.CookSession{CookDate: c.Date, CookSlotID: c.SlotID}, p) {
			continue
		}
		d := dayOf(c.Date)
		switch {
		case x.occasions[d][idOf(k)]:
			best = c // cells are in (date, slot order) order: the last is the latest
		case d < own.day && len(x.occasions[d]) < s.Settings.MaxCookOccasionsPerDay && !x.onlyConfirmed(k):
			free = c
		}
	}
	switch {
	case best != nil:
		return best.Key(), true
	case free != nil:
		return free.Key(), true
	}
	return plate.CellKey{}, false
}

// onlyConfirmed reports whether the cell holds plates and all are confirmed:
// a draft cook there would sit among rows the user locked in.
func (x *index) onlyConfirmed(k plate.CellKey) bool {
	idx := x.byCell[idOf(k)]
	for _, i := range idx {
		if !x.s.Plates[i].Confirmed {
			return false
		}
	}
	return len(idx) > 0
}

// cookedBy reports whether ss cooks on or before p's cell.
func (s State) cookedBy(ss plate.CookSession, p Plate) bool {
	cd, pd := dayOf(ss.CookDate), dayOf(p.Date)
	return cd < pd || (cd == pd && s.SlotOrder[ss.CookSlotID] <= s.SlotOrder[p.SlotID])
}

func fitsPosition(f FoodFacts, what string, kind mealKind) bool {
	role := f.Role
	if !f.Composed() {
		role = f.LeafRole
	}
	switch what {
	case AltMeal, AltMain:
		return f.Composed() && isMain(f.Role, kind)
	case AltSide:
		return sideRole(role)
	case AltAdd:
		return sideRole(role) || role == food.RoleDessert || role == food.RoleDrink || f.NoCook
	}
	return false
}

func fitsDiet(class, want diet.Class) bool {
	return want == "" || (diet.Valid(class) && diet.Compatible(class, want))
}

// matches applies the minutes-free filters: roles, search, tags, no-cook.
func (c AltConstraints) matches(f FoodFacts, words [][]diet.Alt) bool {
	role := f.Role
	if !f.Composed() {
		role = f.LeafRole
	}
	if len(c.Roles) > 0 && !slices.Contains(c.Roles, role) {
		return false
	}
	if c.NoCook && !f.NoCook {
		return false
	}
	for _, t := range c.Tags {
		if !slices.Contains(f.Tags, t) {
			return false
		}
	}
	hay := diet.Fold(f.Name + " " + strings.Join(f.Tags, " "))
	for _, alts := range words {
		if !diet.MatchesWord(hay, alts) {
			return false
		}
	}
	return true
}

// tryAlt applies c to a copy of s, re-evaluates it and builds the result;
// false when the change adds a hard issue.
func (s State) tryAlt(t Target, pi, old int, drop []int, c altCand, hard, soft map[string]bool, baseCooks int) (Alternative, string) {
	p := s.Plates[pi]
	next := s
	next.Plates = slices.Clone(s.Plates)
	np := p
	np.Components = nil
	for i, comp := range p.Components {
		if i == old || slices.Contains(drop, i) {
			if i == old && c.session == nil {
				np.Components = append(np.Components, c.comp)
			}
			continue
		}
		np.Components = append(np.Components, comp)
	}
	if old < 0 || c.session != nil {
		np.Components = append(np.Components, c.comp)
	}
	switch {
	case c.cookAt != nil:
		next.Sessions = append(slices.Clone(s.Sessions), Session{
			CookSession: plate.CookSession{FoodID: c.food.ID, CookDate: c.cookAt.Date, CookSlotID: c.cookAt.SlotID, Portions: c.comp.Multiplier() * p.Eaters},
			Key:         altSessionKey,
		})
		np.Components[slices.IndexFunc(np.Components, func(comp Component) bool { return comp.FoodID == c.food.ID })].SessionKey = altSessionKey
	case c.grow > 0:
		next.Sessions = slices.Clone(s.Sessions)
		for i := range next.Sessions {
			if next.Sessions[i].ID == c.session.ID {
				next.Sessions[i].Portions += c.grow
			}
		}
	}
	next.Plates[pi] = np

	ev := Evaluate(next, t)
	var newIssues []Issue
	for _, is := range ev.Issues {
		if is.Severity == SeverityViolation && !hard[issueID(is)] {
			return Alternative{}, is.Rule + "." + is.Code
		}
		if is.Severity == SeveritySoft && !soft[softKey(is)] {
			is.Options = nil
			newIssues = append(newIssues, is)
		}
	}
	a := Alternative{FoodID: c.food.ID, Name: c.food.Name, Source: SourceFresh, NewIssues: newIssues}
	switch {
	case c.session != nil:
		id := c.session.ID
		a.Source, a.SessionID = SourceSession, &id
	case !c.food.Composed():
		a.Source = SourceLeaf
	case c.cookAt != nil:
		a.Source, a.TotalMinutes = SourceCookAt, c.food.Minutes
		a.CookAt = &OptionCell{Date: c.cookAt.Date.Format(time.DateOnly), SlotID: c.cookAt.SlotID}
	default:
		a.TotalMinutes = c.food.Minutes
	}
	date := p.Date.Format(time.DateOnly)
	for _, pe := range ev.Plates {
		if pe.PlateID == p.ID {
			a.Kcal, a.ProteinG = math.Round(pe.Kcal), math.Round(pe.ProteinG*10)/10
		}
	}
	for _, d := range ev.Days {
		if d.Date == date {
			a.DayKcal, a.CookOccasionsDay = math.Round(d.Kcal), d.CookOccasions
			if d.TargetKcal != nil && *d.TargetKcal > 0 {
				a.DayPct = math.Round(d.Kcal / *d.TargetKcal * 1000) / 10
			}
		}
	}
	if ev.Week != nil {
		a.WeekPct = math.Round(ev.Week.Pct*10) / 10
	}
	a.Ops = s.altOps(p, old, drop, c, a.CookAt)
	y := newIndex(next, t)
	a.rank = altRank{
		newIssues: len(newIssues),
		addsCook:  len(y.occasions[dayOf(p.Date)]) > baseCooks,
		source:    sourceRank(a.Source),
		sameRole:  s.sameSideRole(np, c),
		kcalFit:   y.kcalFit(np, a.Kcal),
		dishMeals: y.dishMeals()[c.food.ID],
		lastEaten: math.MinInt64,
	}
	if c.food.LastEaten != nil {
		a.rank.lastEaten = c.food.LastEaten.Unix() / 86400
	}
	return a, ""
}

func sourceRank(src string) int {
	switch src {
	case SourceSession:
		return 0
	case SourceCookAt:
		return 1
	}
	return 2
}

// altOps writes the change as apply_changes ops: a swap of the old
// component (a fresh cook or a leaf), a replace with cook_at (or an add with
// cook_at), or an add eating from the batch plus a remove of the old one; a
// meal swap also removes the other components.
func (s State) altOps(p Plate, old int, drop []int, c altCand, cookAt *OptionCell) []OptionOp {
	id := p.ID
	quantity := func(op OptionOp) OptionOp {
		if c.comp.Portions != nil {
			op.Portions = c.comp.Portions
		} else if c.comp.Grams != nil {
			op.Amount, op.Unit = c.comp.Grams, "g"
		}
		return op
	}
	var ops []OptionOp
	switch {
	case cookAt != nil && old >= 0:
		cid := p.Components[old].ID
		ops = append(ops, quantity(OptionOp{Op: "replace", PlateID: &id, PlateComponentID: &cid, FoodID: c.food.ID, CookAt: cookAt}))
	case cookAt != nil:
		ops = append(ops, quantity(OptionOp{Op: "add", PlateID: &id, FoodID: c.food.ID, CookAt: cookAt}))
	case old >= 0 && c.session == nil:
		cid := p.Components[old].ID
		ops = append(ops, quantity(OptionOp{Op: "swap", PlateComponentID: &cid, FoodID: c.food.ID}))
	default:
		op := quantity(OptionOp{Op: "add", PlateID: &id, FoodID: c.food.ID})
		if c.session != nil {
			sid := c.session.ID
			op.SessionID = &sid
		}
		ops = append(ops, op)
		if old >= 0 {
			cid := p.Components[old].ID
			ops = append(ops, OptionOp{Op: "remove", PlateComponentID: &cid})
		}
	}
	for _, i := range drop {
		cid := p.Components[i].ID
		ops = append(ops, OptionOp{Op: "remove", PlateComponentID: &cid})
	}
	return ops
}

// sameSideRole reports whether c is a side whose role another component of
// the plate after the change already has.
func (s State) sameSideRole(np Plate, c altCand) bool {
	role := roleOf(c.food)
	if !sideRole(role) {
		return false
	}
	seen := false
	for _, comp := range np.Components {
		if f, ok := s.Foods[comp.FoodID]; ok && roleOf(f) == role {
			if comp.FoodID == c.food.ID && !seen {
				seen = true
				continue
			}
			return true
		}
	}
	return false
}

func roleOf(f FoodFacts) food.Role {
	if f.Composed() {
		return f.Role
	}
	return f.LeafRole
}

// kcalFit is how far the plate's kcal misses its cell's share under a light
// or hearty intent (0 without one): a light meal over its cap, a hearty meal
// under its minimum, a light or hearty day's meal off the day's even share.
func (x *index) kcalFit(p Plate, kcal float64) float64 {
	var dk *DayKcal
	for _, a := range x.dayAccounts([]time.Time{p.Date}) {
		dk = &a
	}
	if dk == nil || dk.Active() == 0 {
		return 0
	}
	switch {
	case x.s.lightMeal(p.Date, p.SlotID):
		return max(0, kcal-dk.mealCap())
	case x.s.heartyMeal(p.Date, p.SlotID):
		return max(0, HeartyMealShare*dk.normalShare()-kcal)
	case x.s.lightDay(p.Date), x.s.heartyDay(p.Date):
		return math.Abs(kcal - dk.Adjusted/float64(dk.Active()))
	}
	return 0
}

// softKeys are the soft issues of issues by softKey.
func softKeys(issues []Issue) map[string]bool {
	m := map[string]bool{}
	for _, is := range issues {
		if is.Severity == SeveritySoft {
			m[softKey(is)] = true
		}
	}
	return m
}

// softKey identifies a soft issue across evaluations: its id plus the food
// of a per-food issue (S8 has no date).
func softKey(is Issue) string {
	return fmt.Sprintf("%s|%v", issueID(is), is.Params["food_id"])
}

func boolCmp(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

func ptrVal(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
