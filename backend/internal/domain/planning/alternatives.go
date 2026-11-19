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

// Light asks (AltRequest.Light): candidates that make the plate's meal
// lighter (within the S6 cap) or heartier (at least the S6b minimum).
const (
	AltLighter  = "lighter"
	AltHeartier = "heartier"
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
	// Light is "lighter" or "heartier" (a lunch or dinner plate): the meal
	// is evaluated as a light (hearty) meal and only candidates whose plate
	// meets its cap (minimum) are kept. Empty: no such filter.
	Light string
	// TolerateFloor keeps a candidate that takes its day under the energy
	// floor (the compound ways of Ways make that up on another meal; their
	// own check drops the ones that do not).
	TolerateFloor bool
	// NoLimit lists every verified candidate (Ways picks among them itself).
	NoLimit bool
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
	// Role is the candidate's role (a leaf's effective role).
	Role string `json:"role,omitempty"`
	// TotalMinutes is the fresh or cook_at cook's prep + cook minutes (0
	// otherwise).
	TotalMinutes int `json:"total_minutes"`
	// FoodKcal and FoodProteinG are the new component alone, PlateKcal and
	// PlateProteinG the whole plate after the change, DayKcal its day; all
	// per eater.
	FoodKcal      float64 `json:"food_kcal"`
	FoodProteinG  float64 `json:"food_protein_g"`
	PlateKcal     float64 `json:"plate_kcal"`
	PlateProteinG float64 `json:"plate_protein_g"`
	DayKcal       float64 `json:"day_kcal"`
	// DayPct and WeekPct are the day's kcal against its target and the
	// weekly ledger's pct (0 without a target).
	DayPct  float64 `json:"day_pct"`
	WeekPct float64 `json:"week_pct"`
	// WeekMeatG is the week's meat after the change, per eater (g).
	WeekMeatG        float64 `json:"week_meat_g,omitempty"`
	CookOccasionsDay int     `json:"cook_occasions_day"`
	// Recommended marks the first alternative (one per result); Because
	// says why in one phrase.
	Recommended bool   `json:"recommended,omitempty"`
	Because     string `json:"recommended_because,omitempty"`
	// NewIssues are the soft issues the change adds (without options).
	NewIssues []Issue `json:"new_issues"`
	// Ops are the apply_changes ops that make the change, in order.
	Ops []OptionOp `json:"ops"`

	rank altRank
	// plateKcal is PlateKcal unrounded, for the light filter.
	plateKcal float64
	// next is the state with the change applied (Ways builds on it).
	next State
}

// altRank is the ranking key, compared field by field (lower first).
type altRank struct {
	newIssues int
	addsCook  bool
	// sideFit puts a composed side of the replaced side's role (0) before
	// every other candidate (1) when a side is replaced; 0 otherwise.
	sideFit int
	// source orders a session before a cook_at before a fresh cook or leaf.
	source    int
	dishMeals int
	// weekCook: the food already has a cook in the window (variety_repeat_week).
	weekCook  bool
	sameRole  bool
	kcalFit   float64
	lastEaten int64 // unix days; never eaten sorts first
}

// AltResult is the outcome of Alternatives. EmptyReason is set only when
// no candidate is left.
type AltResult struct {
	// Current is the food the alternatives replace (nil for an add).
	Current      *AltCurrent   `json:"current,omitempty"`
	Alternatives []Alternative `json:"alternatives"`
	// Hint says what the request left out: a meal swap drops the plate's
	// other components.
	Hint        string       `json:"hint,omitempty"`
	EmptyReason *EmptyReason `json:"empty_reason,omitempty"`
}

// AltCurrent is the component an alternative replaces, with the plate it sits
// on (per eater). MaxKcal or MinKcal is the cap or minimum of a light or hearty
// meal.
type AltCurrent struct {
	FoodID       int64   `json:"food_id"`
	Name         string  `json:"name"`
	Role         string  `json:"role,omitempty"`
	TotalMinutes int     `json:"total_minutes"`
	FoodKcal     float64 `json:"food_kcal"`
	PlateKcal    float64 `json:"plate_kcal"`
	DayKcal      float64 `json:"day_kcal"`
	Intent       string  `json:"intent,omitempty"`
	MaxKcal      float64 `json:"max_kcal"`
	MinKcal      float64 `json:"min_kcal"`
}

// MeatStepG is how much more weekly meat counts as a real step.
const MeatStepG = 25.0

// MeatOverStep reports whether the change from before to after raises the
// week's meat by a real step while the week is over its meat limit: such a
// change is never offered by a tool; one the user names is saved and its
// week effect told.
func MeatOverStep(before, after Evaluation) bool {
	a, b := before.Week, after.Week
	return a != nil && b != nil && b.MeatG >= a.MeatG+MeatStepG &&
		slices.ContainsFunc(after.Issues, func(is Issue) bool { return is.Code == CodeWeekMeatAboveDGE })
}

// EmptyReason says why nothing fits: how many candidates fit the position
// and constraints (0: none, loosen a constraint) and how many of them each
// new hard issue dropped, keyed "rule.code" (a candidate counts once, under
// its first new hard issue).
type EmptyReason struct {
	Candidates int            `json:"candidates"`
	Dropped    map[string]int `json:"dropped,omitempty"`
	// Why says in plain words, with the numbers, what blocked the
	// candidates (the best a blocked one would have reached).
	Why []string `json:"why,omitempty"`
}

// Alternatives returns up to req.Limit candidates for req's plate, each
// applied to a copy of s and re-evaluated: a candidate that adds a hard
// issue is dropped. Ranked by fewest new soft issues, no new cooking
// occasion, source (a session, then a cook_at, then a fresh cook), the
// main's dish count after the change, a recipe with no cook yet in the window
// before one that has (variety_repeat_week), no second side of the same role, kcal
// fit to a light or hearty cell, then the oldest last eaten; when a side is
// replaced a composed side of the same role comes first. A food is listed once,
// by its best-ranked source. Pure: nothing writes.
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

	var lightMax, lightMin float64
	if req.Light != "" {
		var err error
		if s, lightMax, lightMin, err = s.withAltLight(t, p, req.Light); err != nil {
			return AltResult{}, err
		}
	}
	base := Evaluate(s, t)
	hard, soft := hardKeys(base.Issues), softKeys(base.Issues)
	x := newIndex(s, t)
	baseCooks := x.occasions[dayOf(p.Date)]
	cookedInWindow := map[int64]bool{}
	for _, ev := range x.cooks {
		if s.Window.Contains(ev.cell.Date) {
			cookedInWindow[ev.foodID] = true
		}
	}
	var out []Alternative
	cands := s.altCandidates(x, p, old, req)
	dropped := map[string]int{}
	blocked := map[string]Issue{} // per drop key the blocking issue that came closest
	lightBest := math.NaN()       // the plate kcal closest to the light cap or minimum
	for _, c := range cands {
		a, why, by := s.tryAlt(t, pi, old, drop, c, base, hard, soft, len(baseCooks), cookedInWindow, req.TolerateFloor)
		if why != "" {
			dropped[why]++
			if by != nil && closer(why, *by, blocked[why]) {
				blocked[why] = *by
			}
			continue
		}
		if (lightMax > 0 && a.plateKcal > lightMax+1e-9) || (lightMin > 0 && a.plateKcal < lightMin-1e-9) {
			dropped[lightDropKey(req.Light)]++
			if math.IsNaN(lightBest) || (req.Light == AltLighter && a.plateKcal < lightBest) || (req.Light == AltHeartier && a.plateKcal > lightBest) {
				lightBest = a.plateKcal
			}
			continue
		}
		out = append(out, a)
	}
	slices.SortStableFunc(out, func(a, b Alternative) int {
		ra, rb := a.rank, b.rank
		return cmp.Or(
			cmp.Compare(ra.newIssues, rb.newIssues),
			boolCmp(ra.addsCook, rb.addsCook),
			cmp.Compare(ra.sideFit, rb.sideFit),
			cmp.Compare(ra.source, rb.source),
			cmp.Compare(ra.dishMeals, rb.dishMeals),
			boolCmp(ra.weekCook, rb.weekCook),
			boolCmp(ra.sameRole, rb.sameRole),
			cmp.Compare(ra.kcalFit, rb.kcalFit),
			cmp.Compare(ra.lastEaten, rb.lastEaten),
			cmp.Compare(a.FoodID, b.FoodID),
			cmp.Compare(ptrVal(a.SessionID), ptrVal(b.SessionID)),
		)
	})
	// One way per food: the best-ranked of its sources (a batch, cook_at,
	// fresh).
	listed := map[int64]bool{}
	out = slices.DeleteFunc(out, func(a Alternative) bool {
		if listed[a.FoodID] {
			return true
		}
		listed[a.FoodID] = true
		return false
	})
	if len(out) > limit && !req.NoLimit {
		out = out[:limit]
	}
	if len(out) > 0 {
		out[0].Recommended = true
		out[0].Because = "the best-ranked candidate: no new problem, the fewest new warnings, no extra cooking where possible"
		if len(out[0].NewIssues) > 0 {
			out[0].Because = "the best-ranked candidate, but it creates a new warning (see new_issues)"
		}
	}
	res := AltResult{Alternatives: out}
	if old >= 0 {
		res.Current = altCurrent(s, base, p, old)
	}
	if len(drop) > 0 {
		res.Hint = mealHint(s, base, p, drop)
	}
	if len(out) == 0 {
		res.EmptyReason = &EmptyReason{Candidates: len(cands)}
		if len(dropped) > 0 {
			res.EmptyReason.Dropped = dropped
		}
		res.EmptyReason.Why = dropWords(len(cands), dropped, blocked, req.Light, lightMax, lightMin, lightBest)
	}
	return res, nil
}

// lightDropKey is the dropped key of a candidate that misses the light ask.
func lightDropKey(light string) string {
	if light == AltHeartier {
		return "S6b." + CodeMealNotHearty
	}
	return "S6." + CodeMealNotLight
}

// withAltLight returns s with the plate's meal marked light (hearty) for the
// ask, and the cap (minimum) of that meal per eater.
func (s State) withAltLight(t Target, p Plate, light string) (State, float64, float64, error) {
	var level string
	switch light {
	case AltLighter:
	case AltHeartier:
		level = LevelHearty
	default:
		return s, 0, 0, fmt.Errorf("%w: light must be lighter or heartier", domain.ErrInvalidInput)
	}
	var name string
	for _, n := range lightSlots {
		if s.slotIs(p.SlotID, n) {
			name = n
		}
	}
	if name == "" {
		return s, 0, 0, fmt.Errorf("%w: light applies to a lunch or dinner plate", domain.ErrInvalidInput)
	}
	s.Light = MergeLight(s.Light, []LightIntent{{Date: p.Date.Format(time.DateOnly), Scope: LightScopeMeal, Slot: name, Level: level}})
	var dk *DayKcal
	for _, a := range newIndex(s, t).dayAccounts([]time.Time{p.Date}) {
		dk = &a
	}
	if dk == nil || dk.Target <= 0 {
		return s, 0, 0, fmt.Errorf("%w: light needs a kcal target in the profile", domain.ErrInvalidInput)
	}
	if level == LevelHearty {
		return s, 0, HeartyMealShare * dk.normalShare(), nil
	}
	return s, dk.mealCap(), 0, nil
}

// altCurrent describes the component at old of p and its plate.
func altCurrent(s State, base Evaluation, p Plate, old int) *AltCurrent {
	comp := p.Components[old]
	f := s.Foods[comp.FoodID]
	cur := &AltCurrent{FoodID: comp.FoodID, Name: f.Name, Role: string(roleOf(f))}
	if f.Composed() {
		cur.TotalMinutes = f.Minutes
	}
	for _, pe := range base.Plates {
		if pe.PlateID != p.ID {
			continue
		}
		cur.PlateKcal = math.Round(pe.Kcal)
		if old < len(pe.Components) {
			cur.FoodKcal = math.Round(pe.Components[old].Kcal)
		}
		cur.Intent, cur.MaxKcal, cur.MinKcal = pe.Intent, pe.MaxKcal, pe.MinKcal
	}
	for _, d := range base.Days {
		if d.Date == p.Date.Format(time.DateOnly) {
			cur.DayKcal = math.Round(d.Kcal)
		}
	}
	return cur
}

// mealHint says what a meal swap drops: the plate's other components.
func mealHint(s State, base Evaluation, p Plate, drop []int) string {
	var names []string
	kcal := 0.0
	for _, pe := range base.Plates {
		if pe.PlateID != p.ID {
			continue
		}
		for _, j := range drop {
			if j < len(pe.Components) {
				names = append(names, food.Quote(pe.Components[j].Name))
				kcal += pe.Components[j].Kcal
			}
		}
	}
	return fmt.Sprintf("what=meal also removes %s (%.0f kcal per eater) from the plate; what=main keeps them", strings.Join(names, ", "), kcal)
}

// closer reports whether blocking issue is a closer miss than the one kept
// (the zero Issue: none yet) for the drop key why: for the floor, the day that
// came nearest to it.
func closer(why string, is, kept Issue) bool {
	if kept.Rule == "" {
		return true
	}
	if why == "H9."+CodeKcalBelowFloor {
		k, _ := is.Params["kcal"].(float64)
		kk, _ := kept.Params["kcal"].(float64)
		return k > kk
	}
	return false
}

// dropWords turns what blocked the candidates into plain sentences with the
// numbers the model needs to decide (the floor and the day it would reach,
// the cap and the plate it would reach).
func dropWords(candidates int, dropped map[string]int, blocked map[string]Issue, light string, lightMax, lightMin, lightBest float64) []string {
	if candidates == 0 {
		return []string{"no food fits the position and the constraints: loosen one"}
	}
	keys := make([]string, 0, len(dropped))
	for k := range dropped {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int { return cmp.Or(cmp.Compare(dropped[b], dropped[a]), cmp.Compare(a, b)) })
	num := func(is Issue, k string) float64 {
		switch v := is.Params[k].(type) {
		case float64:
			return v
		case int:
			return float64(v)
		}
		return 0
	}
	var out []string
	for _, k := range keys {
		n, is := dropped[k], blocked[k]
		switch k {
		case "H9." + CodeKcalBelowFloor:
			out = append(out, fmt.Sprintf("%s: %d would leave the day under its kcal floor; the best reaches %.0f kcal, the floor is %.0f kcal", PlainText(CodeKcalBelowFloor).Label, n, num(is, "kcal"), num(is, "floor_kcal")))
		case "H2." + CodeTooManyCookOccasions:
			out = append(out, fmt.Sprintf("%s: %d would add a cooking time to a day that already has its maximum (%.0f of %.0f)", PlainText(CodeTooManyCookOccasions).Label, n, num(is, "count"), num(is, "max")))
		case "S6." + CodeMealNotLight:
			out = append(out, fmt.Sprintf("light: %d would keep the meal over its cap of %.0f kcal; the lightest plate is %.0f kcal", n, lightMax, lightBest))
		case "S6b." + CodeMealNotHearty:
			out = append(out, fmt.Sprintf("hearty: %d would keep the meal under its minimum of %.0f kcal; the heartiest plate is %.0f kcal", n, lightMin, lightBest))
		case altNutritionMissing:
			out = append(out, fmt.Sprintf("%d lack nutrition data, so the plate or day could not be judged", n))
		default:
			out = append(out, fmt.Sprintf("%s: %d candidates dropped", k, n))
		}
	}
	return out
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
		if (req.What == AltAdd || req.What == AltSide) && !s.addOnFits(p, f, slotName(s, p.SlotID)) {
			continue
		}
		// Breakfast food is no main for lunch or dinner either.
		if s.PairAddOns && !s.MealFits(f, slotName(s, p.SlotID)) {
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
		return sideRole(role) && (f.Composed() || f.NoCook)
	case AltAdd:
		return (f.Composed() || f.NoCook) && (sideRole(role) || role == food.RoleDessert || role == food.RoleDrink || f.NoCook)
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
func (s State) tryAlt(t Target, pi, old int, drop []int, c altCand, base Evaluation, hard, soft map[string]bool, baseCooks int, cookedInWindow map[int64]bool, tolerateFloor bool) (Alternative, string, *Issue) {
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
	if why := newNutritionGap(base, ev, p); why != "" {
		return Alternative{}, why, nil
	}
	if MeatOverStep(base, ev) {
		return Alternative{}, "S7." + CodeWeekMeatAboveDGE, nil
	}
	var newIssues []Issue
	for _, is := range ev.Issues {
		if is.Severity == SeverityViolation && !hard[issueID(is)] && (!tolerateFloor || is.Code != CodeKcalBelowFloor) {
			return Alternative{}, is.Rule + "." + is.Code, &is
		}
		if is.Severity == SeveritySoft && !soft[softKey(is)] {
			is.Options = nil
			newIssues = append(newIssues, is)
		}
	}
	m := c.comp.Multiplier()
	a := Alternative{
		FoodID: c.food.ID, Name: c.food.Name, Source: SourceFresh, NewIssues: newIssues, Role: string(roleOf(c.food)),
		FoodKcal: math.Round(c.food.Macros.Kcal * m), FoodProteinG: math.Round(c.food.Macros.Protein*m*10) / 10,
	}
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
			a.PlateKcal, a.PlateProteinG, a.plateKcal = math.Round(pe.Kcal), math.Round(pe.ProteinG*10)/10, pe.Kcal
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
		a.WeekMeatG = ev.Week.MeatG
	}
	a.Ops = s.altOps(p, old, drop, c, a.CookAt)
	a.next = next
	y := newIndex(next, t)
	a.rank = altRank{
		newIssues: len(newIssues),
		addsCook:  len(y.occasions[dayOf(p.Date)]) > baseCooks,
		source:    sourceRank(a.Source),
		sameRole:  s.sameSideRole(np, c),
		kcalFit:   y.kcalFit(np, a.PlateKcal),
		dishMeals: y.dishMeals()[c.food.ID],
		weekCook:  c.session == nil && c.food.Composed() && cookedInWindow[c.food.ID],
		lastEaten: math.MinInt64,
	}
	if c.food.LastEaten != nil {
		a.rank.lastEaten = c.food.LastEaten.Unix() / 86400
	}
	if old >= 0 && !isMainComp(s, p, old) {
		oldRole := roleOf(s.Foods[p.Components[old].FoodID])
		if !c.food.Composed() || roleOf(c.food) != oldRole {
			a.rank.sideFit = 1
		}
	}
	return a, "", nil
}

// isMainComp reports whether component i of p is its main.
func isMainComp(s State, p Plate, i int) bool {
	c := p.Components[i]
	return c.Position == plate.PositionMain || (c.Position == "" && c.FoodID == s.mainOf(p))
}

// altNutritionMissing is the drop key of a candidate whose incomplete
// nutrition data turns a complete plate or day into an unjudgeable one.
const altNutritionMissing = "nutrition_missing"

// newNutritionGap returns altNutritionMissing when the change leaves p's
// plate or its day short of nutrition data that base had.
func newNutritionGap(base, ev Evaluation, p Plate) string {
	complete := func(e Evaluation) bool {
		for _, pe := range e.Plates {
			if pe.PlateID == p.ID {
				return !slices.ContainsFunc(pe.Components, func(c ComponentEval) bool { return !c.NutritionComplete })
			}
		}
		return true
	}
	missing := func(e Evaluation) bool {
		date := p.Date.Format(time.DateOnly)
		for _, d := range e.Days {
			if d.Date == date {
				return d.NutritionMissing
			}
		}
		return false
	}
	if (complete(base) && !complete(ev)) || (!missing(base) && missing(ev)) {
		return altNutritionMissing
	}
	return ""
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
// cook_at), or a replace eating from the batch (an add when nothing is
// replaced); a
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
	case old >= 0:
		// A batch source replaces in place, so the position is kept.
		cid, sid := p.Components[old].ID, c.session.ID
		ops = append(ops, quantity(OptionOp{Op: "replace", PlateID: &id, PlateComponentID: &cid, FoodID: c.food.ID, SessionID: &sid}))
	default:
		op := quantity(OptionOp{Op: "add", PlateID: &id, FoodID: c.food.ID})
		if c.session != nil {
			sid := c.session.ID
			op.SessionID = &sid
		}
		ops = append(ops, op)
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
