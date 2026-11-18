package planning

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/diet"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
)

// A relative request ("lighter", "heartier", "more protein") is met only when
// the food changes in the asked direction by a meaningful amount: Ways
// computes verified ways to get there, apply_changes judges the result
// (Satisfied). A lighter meal must come under its light cap and a heartier
// one reach its minimum, while the day stays above its floor and, where it
// can, in its band: when the target meal alone cannot do that, a compound way
// moves energy to the day's other meal.
const (
	GoalLighter     = "lighter"
	GoalHeartier    = "heartier"
	GoalMoreProtein = "more_protein"

	// RelMinKcal and RelMinShare: a meaningful change of a meal's energy is
	// at least RelMinShare of the meal or RelMinKcal, whichever is smaller.
	RelMinKcal  = 80.0
	RelMinShare = 0.10
	// RelMinProteinG is a meaningful change of protein, meal or day.
	RelMinProteinG = 10.0

	// maxWays is the most ways Ways returns (a caller that checks them again
	// asks for more than it shows); defaultWays is what it returns by default.
	maxWays        = 8
	maxOffers      = 4
	defaultWays    = 4
	wayPortionMin  = 0.5
	wayCompSteps   = 3
	maxCompPerKind = 3
	wayAddTries    = 12
	wayMaxServings = 2
	// wayBreadServings: a bread beside the other meal may be a large helping.
	wayBreadServings = 3
)

// MeaningfulKcal reports whether a change of delta kcal (in the asked
// direction) on a meal of before kcal is a meaningful change of food.
func MeaningfulKcal(before, delta float64) bool {
	return delta >= math.Min(RelMinKcal, RelMinShare*before)-1e-9
}

// WayRequest asks for the ways to reach goal on one plate (lighter, heartier,
// more_protein) or, for more_protein only, on the day of Date.
type WayRequest struct {
	PlateID int64
	Date    string
	Goal    string
	Limit   int
}

// WayFigures are per eater: the energy and protein of the meal and of its
// day.
type WayFigures struct {
	MealKcal     float64 `json:"meal_kcal"`
	MealProteinG float64 `json:"meal_protein_g"`
	DayKcal      float64 `json:"day_kcal"`
	DayProteinG  float64 `json:"day_protein_g"`
}

// Way is one verified way to reach the goal: ready ops for apply_changes.
type Way struct {
	// Kind is portion, remove, add, swap or compound (the target meal and
	// another meal of the day change).
	Kind string `json:"kind"`
	// What says the change in plain words; Moved says, for a compound way,
	// what changes on the other meal.
	What  string   `json:"what"`
	Moved []string `json:"moved,omitempty"`
	// Reaches is false for the closest way when none reaches the goal; Why
	// then says how far it gets and what stops it.
	Reaches bool   `json:"reaches_cap"`
	Why     string `json:"why,omitempty"`
	// ChangesOther marks a compound way: it changes another meal of the day
	// and is only ever offered, never a default.
	ChangesOther bool `json:"changes_other_meal,omitempty"`
	// Light is the mark to pass to apply_changes with Ops: set only when the
	// meal meets its cap (minimum) after the way.
	Light  []LightIntent `json:"light,omitempty"`
	Before WayFigures    `json:"before"`
	After  WayFigures    `json:"after"`
	// NewIssues are the soft issues the way adds (without options).
	NewIssues []Issue    `json:"new_issues,omitempty"`
	Ops       []OptionOp `json:"ops"`

	// PlateID is the plate the figures belong to.
	PlateID int64 `json:"plate_id"`
	// Meaningful: the food changed in the asked direction by enough.
	Meaningful bool `json:"-"`
	rank       wayRank
	next       State
}

type wayRank struct {
	notReaching bool
	// newSoft counts the new soft issues but the day band's; belowBand is
	// whether the day band is missed.
	newSoft   int
	belowBand int
	progress  float64
	cost      int
	// protein is the protein the way adds (more_protein only): more first.
	protein float64
	// step is how far the way moves the meal's energy (light and hearty):
	// the smallest sufficient change first.
	step   float64
	dayFit float64
}

// WayResult is the outcome of Ways.
type WayResult struct {
	Goal    string `json:"goal"`
	PlateID int64  `json:"plate_id,omitempty"`
	Date    string `json:"date"`
	// MaxKcal (lighter) or MinKcal (heartier) is the meal's cap or minimum.
	MaxKcal float64 `json:"max_kcal,omitempty"`
	MinKcal float64 `json:"min_kcal,omitempty"`
	// Before are the target meal's and its day's figures now.
	Before WayFigures `json:"before"`
	// Day limits: the energy floor and the band, per eater (after the mark
	// the way would store).
	FloorKcal float64 `json:"floor_kcal,omitempty"`
	BandLow   float64 `json:"band_low,omitempty"`
	BandHigh  float64 `json:"band_high,omitempty"`
	Ways      []Way   `json:"ways"`
	// Offers are compound ways: the named meal reaches its cap (minimum)
	// because another meal of the day changes. Never applied unasked.
	Offers     []Way  `json:"offer,omitempty"`
	EmptyWhy   string `json:"empty_why,omitempty"`
	Candidates int    `json:"candidates"`
}

// wayEdit is a change of the plan under test: the state after it and the ops.
type wayEdit struct {
	s     State
	ops   []OptionOp
	what  []string
	moved []string
	// clear are the stored marks of other meals the way drops (scope none).
	clear []LightIntent
	cost  int
	kind  string
	plate int64
}

type waySearch struct {
	t          Target
	goal       string
	pi         int // target plate index, -1 for a day request
	date       string
	capKcal    float64
	minKcal    float64
	base       Evaluation
	hard, soft map[string]bool
	baseFig    WayFigures // the target plate's figures; day request: unused
	household  diet.Class
	tried      int
}

// Ways returns up to maxWays ways to reach req.Goal, best first, each applied
// to a copy of s, re-evaluated and kept only when it adds no hard issue (the
// energy floor included). Ranked by: reaches the goal, no new soft issue (the
// day in its band), least disruption, the day closest to its target. When no
// way reaches the goal, the closest verified one comes alone, with the reason.
// Pure: nothing writes.
func Ways(s State, t Target, req WayRequest) (WayResult, error) {
	w := &waySearch{t: t, goal: req.Goal, pi: -1}
	switch req.Goal {
	case GoalLighter, GoalHeartier, GoalMoreProtein:
	default:
		return WayResult{}, fmt.Errorf("%w: goal must be lighter, heartier or more_protein", domain.ErrInvalidInput)
	}
	if req.PlateID != 0 {
		w.pi = slices.IndexFunc(s.Plates, func(p Plate) bool { return p.ID == req.PlateID })
		if w.pi < 0 || !s.Window.Contains(s.Plates[w.pi].Date) {
			return WayResult{}, fmt.Errorf("%w: plate %d", domain.ErrNotFound, req.PlateID)
		}
		p := s.Plates[w.pi]
		if p.Confirmed {
			return WayResult{}, fmt.Errorf("%w: plate %d is confirmed", domain.ErrInvalidInput, p.ID)
		}
		w.date = p.Date.Format(time.DateOnly)
	} else {
		if req.Goal != GoalMoreProtein {
			return WayResult{}, fmt.Errorf("%w: goal %s needs plate_id (one meal)", domain.ErrInvalidInput, req.Goal)
		}
		d, err := time.Parse(time.DateOnly, req.Date)
		if err != nil || !s.Window.Contains(d) {
			return WayResult{}, fmt.Errorf("%w: give plate_id, or a date inside the plan window", domain.ErrInvalidInput)
		}
		w.date = req.Date
	}
	if DietRestricts(s.Profile.DietType) {
		w.household = diet.Class(s.Profile.DietType)
	}
	res := WayResult{Goal: req.Goal, Date: w.date}
	var intent *LightIntent
	if req.Goal != GoalMoreProtein {
		p := s.Plates[w.pi]
		var err error
		light := AltLighter
		if req.Goal == GoalHeartier {
			light = AltHeartier
		}
		if s, w.capKcal, w.minKcal, err = s.withAltLight(t, p, light); err != nil {
			return WayResult{}, err
		}
		level := ""
		if req.Goal == GoalHeartier {
			level = LevelHearty
		}
		for _, n := range lightSlots {
			if s.slotIs(p.SlotID, n) {
				intent = &LightIntent{Date: w.date, Scope: LightScopeMeal, Slot: n, Level: level}
			}
		}
		res.MaxKcal, res.MinKcal = math.Round(w.capKcal), math.Round(w.minKcal)
	}
	w.base = Evaluate(s, t)
	w.hard, w.soft = hardKeys(w.base.Issues), softKeys(w.base.Issues)
	if w.pi >= 0 {
		res.PlateID = s.Plates[w.pi].ID
		w.baseFig = w.figures(w.base, res.PlateID)
		res.Before = w.baseFig
	} else {
		res.Before = w.figures(w.base, 0)
	}
	if d, ok := w.dayEval(w.base); ok {
		res.FloorKcal, res.BandLow, res.BandHigh = math.Round(d.FloorKcal), math.Round(d.BandLow), math.Round(d.BandHigh)
	}

	var found, offers []Way
	comps := map[string]int{}
	// compensating re-evaluates the plan many times: a few primary edits of
	// each kind get it.
	canCompensate := func(e wayEdit) bool {
		comps[e.kind]++
		return comps[e.kind] <= maxCompPerKind
	}
	offer := func(e wayEdit, ev Evaluation) {
		if !canCompensate(e) {
			return
		}
		for _, c := range w.compensate(e, ev) {
			if rc := w.check(c.s); !rc.other && !rc.floor && (w.goal == GoalLighter || !w.outOfBand(rc.ev)) {
				a := w.way(c, rc, intent)
				a.ChangesOther = true
				offers = append(offers, a)
			}
		}
	}
	consider := func(e wayEdit) {
		r := w.check(e.s)
		if r.other {
			return
		}
		way := w.way(e, r, intent)
		switch {
		case r.floor:
			// The meal alone would break the floor: only a way that makes
			// the other meal grow gets it to its cap.
			if way.Reaches && w.goal == GoalLighter {
				offer(e, r.ev)
			}
		case w.goal == GoalHeartier && w.outOfBand(r.ev):
			if way.Reaches {
				offer(e, r.ev)
			}
		case way.Meaningful:
			found = append(found, way)
		}
	}
	for _, e := range w.primary(s) {
		consider(e)
	}
	res.Candidates = w.tried
	limit := req.Limit
	if limit <= 0 {
		limit = defaultWays
	}
	limit = min(limit, maxWays)
	found = dedupeWays(found)
	slices.SortStableFunc(found, func(a, b Way) int { return a.cmp(b) })
	if len(found) > limit {
		found = found[:limit]
	}
	offers = dedupeWays(offers)
	slices.SortStableFunc(offers, func(a, b Way) int { return a.cmp(b) })
	if len(offers) > maxOffers {
		offers = offers[:maxOffers]
	}
	res.Ways, res.Offers = found, offers
	if len(found) == 0 {
		res.EmptyWhy = w.noWayWhy(res)
	}
	for i := range res.Ways {
		res.Ways[i].next = State{}
	}
	for i := range res.Offers {
		res.Offers[i].next = State{}
	}
	return res, nil
}

// cmp ranks ways: those meeting the cap (minimum) first, the fewest new soft
// issues; then, among those that meet it, the day in its band, the least
// change and the smallest sufficient one; among those that do not, the
// furthest toward the goal (as far as the floor lets it).
func (a Way) cmp(b Way) int {
	if c := cmp.Or(boolCmp(a.rank.notReaching, b.rank.notReaching), cmp.Compare(a.rank.newSoft, b.rank.newSoft)); c != 0 {
		return c
	}
	if a.rank.notReaching {
		return cmp.Or(cmp.Compare(-math.Floor(a.rank.progress/150), -math.Floor(b.rank.progress/150)), cmp.Compare(a.rank.cost, b.rank.cost), cmp.Compare(-a.rank.progress, -b.rank.progress), cmp.Compare(a.rank.belowBand, b.rank.belowBand), cmp.Compare(a.rank.cost, b.rank.cost))
	}
	return cmp.Or(
		cmp.Compare(a.rank.belowBand, b.rank.belowBand),
		cmp.Compare(a.rank.cost, b.rank.cost),
		cmp.Compare(-a.rank.protein, -b.rank.protein),
		cmp.Compare(a.rank.step, b.rank.step),
		cmp.Compare(a.rank.dayFit, b.rank.dayFit),
	)
}

func dedupeWays(in []Way) []Way {
	seen := map[string]bool{}
	var out []Way
	for _, a := range in {
		k, _ := json.Marshal([]any{a.Ops, a.Light})
		if seen[string(k)] {
			continue
		}
		seen[string(k)] = true
		out = append(out, a)
	}
	return out
}

// wayCheck is a verified state: no hard issue the base lacked (other), the
// energy floor broken (floor), the soft issues it adds.
type wayCheck struct {
	ev      Evaluation
	other   bool
	floor   bool
	newSoft []Issue
}

func (w *waySearch) check(next State) wayCheck {
	w.tried++
	r := wayCheck{ev: Evaluate(next, w.t)}
	for _, is := range r.ev.Issues {
		switch {
		case is.Severity == SeverityViolation && !w.hard[issueID(is)]:
			switch {
			case is.Code == CodeKcalBelowFloor:
				r.floor = true
			case is.Rule == "H3":
				// A batch the change over-draws: apply_changes resizes one that
				// exactly covers its meals (D11), so the real check decides.
			default:
				r.other = true
			}
		case is.Severity == SeveritySoft && !w.soft[softKey(is)]:
			is.Options = nil
			r.newSoft = append(r.newSoft, is)
		}
	}
	if w.pi >= 0 {
		if why := newNutritionGap(w.base, r.ev, next.Plates[w.pi]); why != "" {
			r.other = true
		}
	}
	return r
}

func (w *waySearch) dayEval(ev Evaluation) (DayEval, bool) {
	for _, d := range ev.Days {
		if d.Date == w.date {
			return d, true
		}
	}
	return DayEval{}, false
}

func (w *waySearch) figures(ev Evaluation, plateID int64) WayFigures {
	return FiguresOf(ev, plateID, w.date)
}

// FiguresOf are the figures of plate plateID (0: none) and of the day date in ev.
func FiguresOf(ev Evaluation, plateID int64, date string) WayFigures {
	var f WayFigures
	for _, pe := range ev.Plates {
		if pe.PlateID == plateID {
			f.MealKcal, f.MealProteinG = math.Round(pe.Kcal), math.Round(pe.ProteinG*10)/10
		}
	}
	for _, d := range ev.Days {
		if d.Date == date {
			f.DayKcal, f.DayProteinG = math.Round(d.Kcal), math.Round(d.ProteinG*10)/10
		}
	}
	return f
}

// outOfBand: the day is outside the band the goal moved it to (lighter: under
// it; heartier: over it).
func (w *waySearch) outOfBand(ev Evaluation) bool {
	d, ok := w.dayEval(ev)
	if !ok || d.BandHigh <= 0 {
		return false
	}
	if w.goal == GoalLighter {
		return d.Kcal < d.BandLow-1e-9
	}
	return d.Kcal > d.BandHigh+1e-9
}

// reaches judges the target meal's figures against the goal: it changed
// meaningfully and meets its cap (minimum).
func (w *waySearch) reaches(before, after WayFigures) bool {
	changed, reached, _ := JudgeRelative(w.goal, w.pi >= 0, w.capKcal, w.minKcal, before, after)
	return changed && reached
}

// JudgeRelative decides a relative request: changed is true when the food
// moved in the asked direction by a meaningful amount (energy: at least
// RelMinShare of the meal or RelMinKcal, the smaller; protein: RelMinProteinG
// of the meal, or of the day when meal is false). reached is true when the
// meal is at or under its light cap (maxKcal > 0) or at or over its hearty
// minimum (minKcal > 0); for more_protein it equals changed. A request is
// satisfied when changed. why says in plain words, with the numbers, what is
// missing.
func JudgeRelative(goal string, meal bool, maxKcal, minKcal float64, before, after WayFigures) (changed, reached bool, why string) {
	switch goal {
	case GoalLighter:
		changed = MeaningfulKcal(before.MealKcal, before.MealKcal-after.MealKcal)
		reached = maxKcal <= 0 || after.MealKcal <= maxKcal+0.5
		switch {
		case !changed:
			why = fmt.Sprintf("the meal holds %.0f kcal after, %.0f before: not lighter by a meaningful amount (at least %.0f kcal)", after.MealKcal, before.MealKcal, math.Min(RelMinKcal, RelMinShare*before.MealKcal))
		case !reached:
			why = fmt.Sprintf("lighter, but not a light meal yet: it now holds %.0f kcal (was %.0f), its light cap is %.0f kcal", after.MealKcal, before.MealKcal, maxKcal)
		}
		return changed, reached, why
	case GoalHeartier:
		changed = MeaningfulKcal(before.MealKcal, after.MealKcal-before.MealKcal)
		reached = minKcal <= 0 || after.MealKcal >= minKcal-0.5
		switch {
		case !changed:
			why = fmt.Sprintf("the meal holds %.0f kcal after, %.0f before: not heartier by a meaningful amount (at least %.0f kcal)", after.MealKcal, before.MealKcal, math.Min(RelMinKcal, RelMinShare*before.MealKcal))
		case !reached:
			why = fmt.Sprintf("heartier, but not a hearty meal yet: it now holds %.0f kcal (was %.0f), its hearty minimum is %.0f kcal", after.MealKcal, before.MealKcal, minKcal)
		}
		return changed, reached, why
	}
	gain, what := after.DayProteinG-before.DayProteinG, "day"
	if meal {
		gain, what = after.MealProteinG-before.MealProteinG, "meal"
	}
	changed = gain >= RelMinProteinG-1e-9
	if !changed {
		why = fmt.Sprintf("the %s gains %.1f g protein, a meaningful change is at least %.0f g", what, gain, RelMinProteinG)
	}
	return changed, changed, why
}

// progress is how far a way moves the goal's figure (larger is closer).
func (w *waySearch) progress(a Way) float64 {
	switch w.goal {
	case GoalLighter:
		return a.Before.MealKcal - a.After.MealKcal
	case GoalHeartier:
		return a.After.MealKcal - a.Before.MealKcal
	}
	if w.pi >= 0 {
		return a.After.MealProteinG - a.Before.MealProteinG
	}
	return a.After.DayProteinG - a.Before.DayProteinG
}

func (w *waySearch) way(e wayEdit, r wayCheck, intent *LightIntent) Way {
	before := w.baseFig
	if w.pi < 0 {
		before = w.figures(w.base, e.plate)
	}
	after := w.figures(r.ev, e.plate)
	a := Way{
		Kind: e.kind, What: strings.Join(e.what, ", "), Moved: e.moved, Ops: e.ops, PlateID: e.plate,
		Before: before, After: after, NewIssues: r.newSoft, next: e.s,
	}
	if w.goal == GoalMoreProtein {
		// The day's figures are the day's, whichever plate changed.
		a.Before.DayKcal, a.Before.DayProteinG = w.figures(w.base, 0).DayKcal, w.figures(w.base, 0).DayProteinG
	}
	changed, reached, _ := JudgeRelative(w.goal, w.pi >= 0, w.capKcal, w.minKcal, before, after)
	a.Meaningful, a.Reaches = changed, changed && reached
	if a.Reaches && intent != nil {
		a.Light = append(a.Light, *intent)
	}
	a.Light = append(a.Light, e.clear...)
	fit := 0.0
	if d, ok := w.dayEval(r.ev); ok && d.TargetKcal != nil {
		fit = math.Abs(d.Kcal - *d.TargetKcal)
	}
	other := 0
	for _, is := range r.newSoft {
		if is.Rule != "S2" {
			other++
		}
	}
	a.rank = wayRank{notReaching: !a.Reaches, newSoft: other, belowBand: len(r.newSoft) - other, cost: e.cost, dayFit: fit}
	a.rank.progress = w.progress(a)
	if w.goal != GoalMoreProtein {
		a.rank.step = math.Round(w.progress(a) / 20)
	} else {
		// Whole 5 g steps: a gram more does not outrank the day's fit.
		a.rank.protein = math.Floor(w.progress(a) / 5)
	}
	return a
}

// noWayWhy says in plain words, with the numbers, why the named meal alone
// has no meaningful way.
func (w *waySearch) noWayWhy(res WayResult) string {
	switch w.goal {
	case GoalLighter:
		return fmt.Sprintf("This meal cannot get meaningfully lighter on its own: it holds %.0f kcal (a change needs at least %.0f kcal) and the day holds %.0f kcal, its minimum is %.0f kcal, so the day has only %.0f kcal of room. Its light cap is %.0f kcal.",
			res.Before.MealKcal, math.Min(RelMinKcal, RelMinShare*res.Before.MealKcal), res.Before.DayKcal, res.FloorKcal, math.Max(0, res.Before.DayKcal-res.FloorKcal), res.MaxKcal)
	case GoalHeartier:
		return fmt.Sprintf("This meal cannot get meaningfully heartier on its own without taking the day (%.0f kcal) over its band top of %.0f kcal.", res.Before.DayKcal, res.BandHigh)
	}
	return "no verified change adds enough protein without breaking a hard rule"
}

// ---------------------------------------------------------------------------
// primary edits
// ---------------------------------------------------------------------------

func (w *waySearch) primary(s State) []wayEdit {
	var out []wayEdit
	plates := []int{w.pi}
	if w.pi < 0 {
		plates = plates[:0]
		for i, p := range s.Plates {
			if p.Date.Format(time.DateOnly) == w.date && w.editable(s, p) {
				plates = append(plates, i)
			}
		}
	}
	for _, pi := range plates {
		p := s.Plates[pi]
		switch w.goal {
		case GoalLighter:
			out = append(out, w.quantityEdits(s, pi, -1)...)
			out = append(out, w.removeEdits(s, pi)...)
			out = append(out, w.leanEdits(s, pi)...)
			out = append(out, w.swapEdits(s, p, AltMain, 0)...)
			out = append(out, w.swapEdits(s, p, AltMeal, 0)...)
		case GoalHeartier:
			out = append(out, w.quantityEdits(s, pi, +1)...)
			out = append(out, w.addEdits(s, pi, w.minKcal-w.baseFig.MealKcal+1, false)...)
			out = append(out, w.swapEdits(s, p, AltMain, 0)...)
		default:
			out = append(out, w.quantityEdits(s, pi, +1)...)
			out = append(out, w.addEdits(s, pi, 0, true)...)
			out = append(out, w.swapEdits(s, p, AltMain, w.mainProtein(s, p)+RelMinProteinG)...)
		}
	}
	return out
}

// editable: a draft plate with an id that is not on a routine cell.
func (w *waySearch) editable(s State, p Plate) bool {
	if p.Confirmed || p.ID <= 0 || !s.Window.Contains(p.Date) {
		return false
	}
	cell := p.Cell()
	for _, c := range s.Cells {
		if c.Key() == cell && c.Mode == "routine" {
			return false
		}
	}
	return true
}

func (w *waySearch) mainProtein(s State, p Plate) float64 {
	for i, c := range p.Components {
		if isMainComp(s, p, i) {
			if f, ok := s.Foods[c.FoodID]; ok {
				return f.Macros.Protein * c.Multiplier()
			}
		}
	}
	return 0
}

func setQuantity(s State, pi, ci int, n float64) State {
	s.Plates = slices.Clone(s.Plates)
	p := s.Plates[pi]
	p.Components = slices.Clone(p.Components)
	c := p.Components[ci]
	if c.Portions != nil {
		c.Portions = &n
	} else {
		c.Grams = &n
	}
	p.Components[ci] = c
	s.Plates[pi] = p
	return s
}

func removeComponents(s State, pi int, drop ...int) State {
	s.Plates = slices.Clone(s.Plates)
	p := s.Plates[pi]
	var keep []Component
	for i, c := range p.Components {
		if !slices.Contains(drop, i) {
			keep = append(keep, c)
		}
	}
	p.Components = keep
	s.Plates[pi] = p
	return s
}

// levels are the quantities a component may take in direction dir (-1 less,
// +1 more): composed portions on the quarter grid within [wayPortionMin,
// MaxDishPortions], a leaf's grams at 75 and 50 % (125 and 150 %).
func levels(c Component, dir int) []float64 {
	var out []float64
	switch {
	case c.Portions != nil && *c.Portions > 0:
		for n := *c.Portions + float64(dir)*roundPortions; n >= wayPortionMin-1e-9 && n <= MaxDishPortions+1e-9; n += float64(dir) * roundPortions {
			out = append(out, n)
		}
	case c.Grams != nil && *c.Grams > 0:
		factors := []float64{0.75, 0.5}
		if dir > 0 {
			factors = []float64{1.25, 1.5}
		}
		for _, f := range factors {
			if g := math.Round(*c.Grams*f/roundGrams) * roundGrams; g > 0 && g != *c.Grams {
				out = append(out, g)
			}
		}
	}
	return out
}

func quantityOp(p Plate, c Component, n float64) OptionOp {
	cid := c.ID
	op := OptionOp{Op: "set_quantity", PlateComponentID: &cid}
	if c.Portions != nil {
		op.Portions = &n
	} else {
		op.Amount, op.Unit = &n, "g"
	}
	return op
}

func quantityWords(f FoodFacts, c Component, n float64) string {
	if c.Portions != nil {
		return fmt.Sprintf("%s: %s portions (was %s)", f.Name, trimNum(n), trimNum(*c.Portions))
	}
	return fmt.Sprintf("%s: %.0f g (was %.0f g)", f.Name, n, *c.Grams)
}

func trimNum(n float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", n), "0"), ".")
}

func (w *waySearch) quantityEdits(s State, pi, dir int) []wayEdit {
	p := s.Plates[pi]
	var out []wayEdit
	for ci, c := range p.Components {
		f, ok := s.Foods[c.FoodID]
		if !ok || !f.NutritionComplete || f.Macros.Kcal <= 0 {
			continue
		}
		for _, n := range levels(c, dir) {
			out = append(out, wayEdit{
				s: setQuantity(s, pi, ci, n), ops: []OptionOp{quantityOp(p, c, n)}, what: []string{quantityWords(f, c, n)},
				cost: 1, kind: "portion", plate: p.ID,
			})
		}
	}
	return out
}

func (w *waySearch) removeEdits(s State, pi int) []wayEdit {
	p := s.Plates[pi]
	var out []wayEdit
	if len(p.Components) < 2 {
		return nil
	}
	for ci, c := range p.Components {
		if isMainComp(s, p, ci) {
			continue
		}
		f := s.Foods[c.FoodID]
		cid := c.ID
		out = append(out, wayEdit{
			s: removeComponents(s, pi, ci), ops: []OptionOp{{Op: "remove", PlateComponentID: &cid}},
			what: []string{"leave out " + f.Name}, cost: 2, kind: "remove", plate: p.ID,
		})
	}
	return out
}

// leanEdits drop every component but the main and give the main a smaller
// portion: the lightest plate of the same dish.
func (w *waySearch) leanEdits(s State, pi int) []wayEdit {
	p := s.Plates[pi]
	mi := -1
	for ci := range p.Components {
		if isMainComp(s, p, ci) {
			mi = ci
		}
	}
	if mi < 0 || len(p.Components) < 2 {
		return nil
	}
	var drop []int
	var ops []OptionOp
	var names []string
	for ci, c := range p.Components {
		if ci != mi {
			drop = append(drop, ci)
			cid := c.ID
			ops = append(ops, OptionOp{Op: "remove", PlateComponentID: &cid})
			names = append(names, s.Foods[c.FoodID].Name)
		}
	}
	mc := p.Components[mi]
	mf, ok := s.Foods[mc.FoodID]
	if !ok || !mf.NutritionComplete {
		return nil
	}
	var out []wayEdit
	for _, n := range append([]float64{-1}, levels(mc, -1)...) {
		ns, nops, what := removeComponents(s, pi, drop...), slices.Clone(ops), []string{"leave out " + strings.Join(names, " and ")}
		if n > 0 {
			mci := mi - countBelow(drop, mi)
			ns = setQuantity(ns, pi, mci, n)
			nops = append([]OptionOp{quantityOp(p, mc, n)}, nops...)
			what = append([]string{quantityWords(mf, mc, n)}, what...)
		}
		out = append(out, wayEdit{s: ns, ops: nops, what: what, cost: 2 + len(drop) + btoi(n > 0), kind: "portion", plate: p.ID})
	}
	return out
}

func countBelow(drop []int, i int) int {
	n := 0
	for _, d := range drop {
		if d < i {
			n++
		}
	}
	return n
}

func (w *waySearch) swapEdits(s State, p Plate, what string, minProtein float64) []wayEdit {
	req := AltRequest{PlateID: p.ID, What: what, Limit: 6, TolerateFloor: w.goal == GoalLighter, Constraints: AltConstraints{MinProteinG: minProtein}}
	res, err := Alternatives(s, w.t, req)
	if err != nil {
		return nil
	}
	var out []wayEdit
	for _, a := range res.Alternatives {
		if len(a.Ops) == 0 {
			continue
		}
		words := "swap the main for " + a.Name
		if what == AltMeal {
			words = "swap the whole meal for " + a.Name
		}
		out = append(out, wayEdit{s: a.next, ops: a.Ops, what: []string{words}, cost: 5 + btoi(what == AltMeal), kind: "swap", plate: p.ID})
	}
	return out
}

// addCand is a sized add-on of a no-cook food.
type addCand struct {
	f     FoodFacts
	comp  Component
	kcal  float64
	prot  float64
	grams float64
}

// addCands sizes every no-cook food that fits p once: want kcal (closest
// size) or, for protein, the smallest size with RelMinProteinG (else the
// largest). A leaf is offered in half servings up to addCapG, a recipe in
// half portions up to 1.5.
func (w *waySearch) addCands(s State, p Plate, want float64, protein bool) []addCand {
	onPlate := map[int64]bool{}
	for _, c := range p.Components {
		onPlate[c.FoodID] = true
	}
	var out []addCand
	for _, id := range s.NoCook {
		f, ok := s.Foods[id]
		if !ok || onPlate[id] || !f.NutritionComplete || f.Macros.Kcal <= 0 || f.Sweet || !fitsDiet(f.DietClass, w.household) {
			continue
		}
		var best *addCand
		consider := func(c Component, kcal, prot, grams float64) {
			ac := addCand{f: f, comp: c, kcal: kcal, prot: prot, grams: grams}
			switch {
			case best == nil:
				best = &ac
			case protein && best.prot < RelMinProteinG && ac.prot > best.prot:
				best = &ac
			case protein && best.prot >= RelMinProteinG && ac.prot >= RelMinProteinG && ac.prot < best.prot:
				best = &ac
			case !protein && math.Abs(ac.kcal-want) < math.Abs(best.kcal-want):
				best = &ac
			}
		}
		if f.Composed() {
			for n := 0.5; n <= MaxDishPortions+1e-9; n += 0.5 {
				n := n
				consider(Component{FoodID: id, Portions: &n, Proposed: true}, f.Macros.Kcal*n, f.Macros.Protein*n, 0)
			}
		} else {
			serving := ServingGrams(f)
			limit := addCapG(f, 0)
			for n := 0.5; n <= wayMaxServings+1e-9; n += 0.5 {
				g := math.Round(n*serving/roundGrams) * roundGrams
				if g <= 0 || g > limit+1e-9 {
					continue
				}
				consider(Component{FoodID: id, Grams: &g, Proposed: true}, f.Macros.Kcal*g/100, f.Macros.Protein*g/100, g)
			}
		}
		if best != nil {
			out = append(out, *best)
		}
	}
	if protein {
		slices.SortStableFunc(out, func(a, b addCand) int {
			return cmp.Or(cmp.Compare(-btoi(a.prot >= RelMinProteinG), -btoi(b.prot >= RelMinProteinG)), cmp.Compare(a.kcal/math.Max(a.prot, 1), b.kcal/math.Max(b.prot, 1)))
		})
	} else {
		slices.SortStableFunc(out, func(a, b addCand) int {
			return cmp.Compare(math.Abs(a.kcal-want), math.Abs(b.kcal-want))
		})
	}
	if len(out) > wayAddTries {
		out = out[:wayAddTries]
	}
	return out
}

func (w *waySearch) addEdits(s State, pi int, want float64, protein bool) []wayEdit {
	p := s.Plates[pi]
	var out []wayEdit
	for _, c := range w.addCands(s, p, want, protein) {
		out = append(out, w.addEdit(s, pi, c, ""))
	}
	return out
}

func (w *waySearch) addEdit(s State, pi int, c addCand, who string) wayEdit {
	p := s.Plates[pi]
	ns := s
	ns.Plates = slices.Clone(s.Plates)
	np := p
	np.Components = append(slices.Clone(p.Components), c.comp)
	ns.Plates[pi] = np
	id := p.ID
	op := OptionOp{Op: "add", PlateID: &id, FoodID: c.f.ID}
	var words string
	if c.comp.Portions != nil {
		op.Portions = c.comp.Portions
		words = fmt.Sprintf("add %s portions of %s (+%.0f kcal)", trimNum(*c.comp.Portions), c.f.Name, c.kcal)
	} else {
		op.Amount, op.Unit = c.comp.Grams, "g"
		words = fmt.Sprintf("add %.0f g %s (+%.0f kcal)", c.grams, c.f.Name, c.kcal)
	}
	if who != "" {
		words = who + ": " + words
	}
	return wayEdit{s: ns, ops: []OptionOp{op}, what: []string{words}, cost: 2, kind: "add", plate: p.ID}
}

// ---------------------------------------------------------------------------
// compensation on the day's other meals
// ---------------------------------------------------------------------------

// otherPlates are the day's other plates the compensation may change, in slot
// order: drafts, not routine, and without a light or hearty mark of their own
// that the change would work against.
func (w *waySearch) otherPlates(s State, target Plate, up bool) []int {
	var out []int
	for i, p := range s.Plates {
		if p.ID == target.ID || p.Date.Format(time.DateOnly) != w.date || !w.editable(s, p) {
			continue
		}
		if !up && s.heartyMeal(p.Date, p.SlotID) {
			continue
		}
		out = append(out, i)
	}
	slices.SortStableFunc(out, func(a, b int) int {
		return cmp.Compare(s.SlotOrder[s.Plates[a].SlotID], s.SlotOrder[s.Plates[b].SlotID])
	})
	return out
}

// compensate returns compound ways: the primary edit e plus changes on one
// other meal of the day that bring the day back toward its band: lighter and
// the day under it, more food there (a raised main, then up to wayCompSteps
// add-ons); heartier and the day over it, less food there.
func (w *waySearch) compensate(e wayEdit, ev Evaluation) []wayEdit {
	if w.pi < 0 || w.goal == GoalMoreProtein {
		return nil
	}
	up := w.goal == GoalLighter
	var out []wayEdit
	target := e.s.Plates[w.pi]
	for _, qi := range w.otherPlates(e.s, target, up) {
		q := e.s.Plates[qi]
		// A light meal of its own may have to give up its mark to take the
		// energy the lighter one frees: a way with that is marked so.
		clears := []bool{false}
		if up && e.s.lightMeal(q.Date, q.SlotID) {
			clears = append(clears, true)
		}
		for _, clear := range clears {
			cur, curEv := e, ev
			cur.kind = "compound"
			if clear {
				cur.s, cur.clear = w.clearMark(cur.s, q), append(slices.Clone(cur.clear), w.markOf(cur.s, q, false))
				cur.cost += 3
				curEv = Evaluate(cur.s, w.t)
			}
			if up {
				cur, curEv = w.compUp(cur, curEv, qi)
			} else {
				cur, curEv = w.compDown(cur, curEv, qi)
			}
			_ = curEv
			if len(cur.moved) > 0 {
				if clear {
					cur.moved = append([]string{slotName(e.s, q.SlotID) + " is no longer a light meal"}, cur.moved...)
				}
				out = append(out, cur)
			}
		}
	}
	return out
}

// lift is the kcal the day still lacks to its floor and band bottom (0 when it
// holds both).
func (w *waySearch) lift(ev Evaluation) float64 {
	d, ok := w.dayEval(ev)
	if !ok {
		return 0
	}
	return math.Max(0, math.Max(d.FloorKcal, d.BandLow)-d.Kcal)
}

// score ranks a state during compensation: lower is better. A hard issue
// other than the floor rules it out.
func (w *waySearch) score(r wayCheck) (float64, bool) {
	if r.other {
		return 0, false
	}
	fit, soft := 0.0, 0
	if d, ok := w.dayEval(r.ev); ok && d.TargetKcal != nil {
		fit = math.Abs(d.Kcal - *d.TargetKcal)
		if d.BandHigh > 0 && d.Kcal > d.BandHigh+1e-9 {
			fit += 5000
		}
	}
	for _, is := range r.newSoft {
		if is.Rule != "S2" {
			soft++
		}
	}
	return fit + 400*float64(soft) + 100000*float64(btoi(r.floor)), true
}

// compUp grows the other meal the way a person would: a larger portion of
// its own main (up to MaxDishPortions), then a matching side or bread, never
// a gap filler that does not belong to the dish.
func (w *waySearch) compUp(e wayEdit, ev Evaluation, qi int) (wayEdit, Evaluation) {
	q := e.s.Plates[qi]
	who := slotName(e.s, q.SlotID)
	apply := func(c wayEdit) {
		e.s = c.s
		e.ops = append(slices.Clone(e.ops), c.ops...)
		e.moved = append(slices.Clone(e.moved), who+": "+c.what[0])
		e.cost += c.cost
		ev = Evaluate(e.s, w.t)
	}
	pick := func(cands []wayEdit) (wayEdit, bool) {
		best, bestScore, ok := wayEdit{}, math.Inf(1), false
		for _, c := range cands {
			if sc, good := w.score(w.check(c.s)); good && sc < bestScore {
				best, bestScore, ok = c, sc, true
			}
		}
		return best, ok
	}
	bread := false // one bread at most
	var raise []wayEdit
	for qi2, c := range q.Components {
		f, ok := e.s.Foods[c.FoodID]
		if !isMainComp(e.s, q, qi2) || !ok || !f.NutritionComplete {
			continue
		}
		for _, n := range levels(c, +1) {
			raise = append(raise, wayEdit{s: setQuantity(e.s, qi, qi2, n), ops: []OptionOp{quantityOp(q, c, n)}, what: []string{quantityWords(f, c, n)}, cost: 1})
		}
	}
	if c, ok := pick(raise); ok {
		apply(c)
	}
	for range wayCompSteps - 1 {
		need := w.lift(ev)
		if need <= 0 {
			break
		}
		var cands []wayEdit
		for _, ac := range w.sideCands(e.s, e.s.Plates[qi], need+1, bread) {
			cands = append(cands, w.addEdit(e.s, qi, ac, ""))
		}
		c, ok := pick(cands)
		if !ok {
			break
		}
		apply(c)
		bread = bread || c.ops[0].Amount != nil
	}
	return e, ev
}

// sideCands are the sides that fit a plate: a side recipe (starch or
// vegetable) or a bread leaf, sized closest to want kcal. No cheese, nuts or
// other fillers.
func (w *waySearch) sideCands(s State, p Plate, want float64, noBread bool) []addCand {
	onPlate := map[int64]bool{}
	for _, c := range p.Components {
		onPlate[c.FoodID] = true
	}
	var out []addCand
	for id, f := range s.Foods {
		if onPlate[id] || !f.NutritionComplete || f.Macros.Kcal <= 0 || f.Sweet || !fitsDiet(f.DietClass, w.household) {
			continue
		}
		var best *addCand
		try := func(c Component, kcal, grams float64) {
			if best == nil || math.Abs(kcal-want) < math.Abs(best.kcal-want) {
				best = &addCand{f: f, comp: c, kcal: kcal, grams: grams}
			}
		}
		switch {
		case f.Composed() && (f.Role == food.RoleSideStarch || f.Role == food.RoleSideVeg):
			for n := 0.5; n <= MaxDishPortions+1e-9; n += 0.5 {
				n := n
				try(Component{FoodID: id, Portions: &n, Proposed: true}, f.Macros.Kcal*n, 0)
			}
		case !noBread && !f.Composed() && f.NoCook && f.LeafRole == food.RoleSideStarch:
			limit := math.Min(wayBreadServings*ServingGrams(f), maxAddFillerGrams)
			for n := 0.5; n <= wayBreadServings+1e-9; n += 0.5 {
				g := math.Round(n*ServingGrams(f)/roundGrams) * roundGrams
				if g > 0 && g <= limit+1e-9 {
					try(Component{FoodID: id, Grams: &g, Proposed: true}, f.Macros.Kcal*g/100, g)
				}
			}
		}
		if best != nil {
			out = append(out, *best)
		}
	}
	slices.SortStableFunc(out, func(a, b addCand) int {
		return cmp.Or(cmp.Compare(math.Abs(a.kcal-want), math.Abs(b.kcal-want)), cmp.Compare(a.f.ID, b.f.ID))
	})
	if len(out) > wayAddTries {
		out = out[:wayAddTries]
	}
	return out
}

func (w *waySearch) compDown(e wayEdit, ev Evaluation, qi int) (wayEdit, Evaluation) {
	q := e.s.Plates[qi]
	who := slotName(e.s, q.SlotID)
	for range wayCompSteps {
		d, ok := w.dayEval(ev)
		if !ok || d.BandHigh <= 0 || d.Kcal <= d.BandHigh+1e-9 {
			break
		}
		q = e.s.Plates[qi]
		var cands []wayEdit
		cands = append(cands, w.quantityEdits(e.s, qi, -1)...)
		cands = append(cands, w.removeEdits(e.s, qi)...)
		best, bestScore := wayEdit{}, math.Inf(1)
		for _, c := range cands {
			r := w.check(c.s)
			sc, good := w.score(r)
			dd, _ := w.dayEval(r.ev)
			if !good || dd.Kcal < math.Max(dd.FloorKcal, dd.BandLow)-1e-9 {
				continue
			}
			if sc < bestScore {
				best, bestScore = c, sc
			}
		}
		if len(best.ops) == 0 {
			break
		}
		e.s = best.s
		e.ops = append(slices.Clone(e.ops), best.ops...)
		e.moved = append(slices.Clone(e.moved), who+": "+best.what[0])
		e.cost += best.cost
		ev = Evaluate(e.s, w.t)
	}
	return e, ev
}

// markOf is the scope none intent that clears the meal mark of plate q.
func (w *waySearch) markOf(s State, q Plate, _ bool) LightIntent {
	for _, n := range lightSlots {
		if s.slotIs(q.SlotID, n) {
			return LightIntent{Date: q.Date.Format(time.DateOnly), Scope: LightScopeNone, Slot: n}
		}
	}
	return LightIntent{}
}

// clearMark returns s without the meal mark of plate q.
func (w *waySearch) clearMark(s State, q Plate) State {
	s.Light = MergeLight(s.Light, []LightIntent{w.markOf(s, q, false)})
	return s
}

// MealLimit is the light cap (lighter) or hearty minimum (heartier) of the
// meal on plate plateID, per eater, as the light mark would set it.
func MealLimit(s State, t Target, plateID int64, goal string) (maxKcal, minKcal float64, err error) {
	pi := slices.IndexFunc(s.Plates, func(p Plate) bool { return p.ID == plateID })
	if pi < 0 {
		return 0, 0, fmt.Errorf("%w: plate %d", domain.ErrNotFound, plateID)
	}
	light := AltLighter
	if goal == GoalHeartier {
		light = AltHeartier
	}
	_, maxKcal, minKcal, err = s.withAltLight(t, s.Plates[pi], light)
	return maxKcal, minKcal, err
}
