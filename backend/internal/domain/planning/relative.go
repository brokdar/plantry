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
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
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
	// GoalLowerDay and GoalRaiseDay: an explicit "eat less (more) on this
	// day": the day's target moves by its day intent (LightDayShare,
	// HeartyDayShare) and the ways bring the day into the new band.
	GoalLowerDay = "lower_day"
	GoalRaiseDay = "raise_day"

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
)

// MeaningfulKcal reports whether a change of delta kcal (in the asked
// direction) on a meal of before kcal is a meaningful change of food.
func MeaningfulKcal(before, delta float64) bool {
	return delta >= math.Min(RelMinKcal, RelMinShare*before)-1e-9
}

// MeaningfulMealKcal is MeaningfulKcal for one meal: a noticeable step is a
// tenth of the meal whatever its size (a big dinner raised by 8 % is not
// "heartier").
func MeaningfulMealKcal(before, delta float64) bool {
	return delta >= RelMinShare*before-1e-9
}

// WayRequest asks for the ways to reach goal on one plate (lighter, heartier,
// more_protein) or, for more_protein only, on the day of Date.
type WayRequest struct {
	PlateID int64
	Date    string
	Goal    string
	Limit   int
	// TargetKcal is the number the user named for a day (lower_day,
	// raise_day); 0 when none.
	TargetKcal float64
	// Marked lets a day goal store a light (hearty) day mark. Without it, and
	// without a number named, Ways first changes food inside the day's own
	// range and offers the mark only when that cannot meet the wish.
	Marked bool
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
	// Worsened names the weekly limits (meat, fish, legumes) or repeats the
	// way makes worse without creating a new issue.
	Worsened []string `json:"worsens,omitempty"`
	// BelowBand marks an offer whose day still ends under its band (Why says
	// by how much): the closest the other meal can get it.
	BelowBand bool `json:"below_band,omitempty"`
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
	// Recommended marks the one way or offer the model presents as its
	// recommendation, Because says why in one phrase (set by the caller that
	// checks the ways).
	Recommended bool   `json:"recommended,omitempty"`
	Because     string `json:"recommended_because,omitempty"`
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
	// bandGap is how far under its band the day ends (kcal, 0 inside it).
	bandGap  float64
	progress float64
	cost     int
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
	Offers []Way `json:"offer,omitempty"`
	// Impossible: the user's number cannot be met (under the day's minimum);
	// Ways is empty and Offers holds the nearest safe option.
	Impossible bool   `json:"impossible,omitempty"`
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
	// note says which bound the way kept (shown as the way's why).
	note string
	// group is what the compensation budget counts the edit under (kind
	// when empty).
	group string
	plate int64
}

type waySearch struct {
	t          Target
	goal       string
	pi         int // target plate index, -1 for a day request
	date       string
	capKcal    float64
	minKcal    float64
	bandHigh   float64
	base       Evaluation
	hard, soft map[string]bool
	warn       map[string]bool
	s0         State      // the state under test before any edit (marked)
	baseFig    WayFigures // the target plate's figures; day request: unused
	household  diet.Class
	tried      int
	// noMark: a day goal searched without a day mark (the range stays).
	noMark bool
}

// Ways returns up to maxWays ways to reach req.Goal, best first, each applied
// to a copy of s, re-evaluated and kept only when it adds no hard issue (the
// energy floor included). Ranked by: reaches the goal, no new soft issue (the
// day in its band), least disruption, the day closest to its target. When no
// way reaches the goal, the closest verified one comes alone, with the reason.
// Pure: nothing writes.
func Ways(s State, t Target, req WayRequest) (WayResult, error) {
	if (req.Goal == GoalLowerDay || req.Goal == GoalRaiseDay) && req.TargetKcal <= 0 && !req.Marked {
		return waysDay(s, t, req)
	}
	return ways(s, t, req)
}

// waysDay answers "less (more) on this day" without a number: ways that change
// food inside the day's own range come first and carry no mark. Only when none
// does, the marked ways (the day's goal moves) come back as offers, with the
// number the mark would move the goal to; the user decides.
func waysDay(s State, t Target, req WayRequest) (WayResult, error) {
	plain, err := ways(s, t, req)
	if err != nil || len(plain.Ways) > 0 {
		return plain, err
	}
	req.Marked = true
	marked, err := ways(s, t, req)
	if err != nil {
		return plain, err
	}
	marked.Offers = append(marked.Ways, marked.Offers...)
	marked.Ways = nil
	marked.EmptyWhy = fmt.Sprintf("inside the day's range of %.0f to %.0f kcal nothing changes by a real step; marking the day as %s would move its range to %.0f to %.0f kcal",
		plain.BandLow, plain.BandHigh, map[string]string{GoalLowerDay: "a light day", GoalRaiseDay: "a filling day"}[req.Goal], marked.BandLow, marked.BandHigh)
	return marked, nil
}

func ways(s State, t Target, req WayRequest) (WayResult, error) {
	s.PairAddOns = true
	w := &waySearch{t: t, goal: req.Goal, pi: -1, noMark: !req.Marked && (req.Goal == GoalLowerDay || req.Goal == GoalRaiseDay) && req.TargetKcal <= 0}
	switch req.Goal {
	case GoalLighter, GoalHeartier, GoalMoreProtein, GoalLowerDay, GoalRaiseDay:
	default:
		return WayResult{}, fmt.Errorf("%w: goal must be lighter, heartier, more_protein, lower_day or raise_day", domain.ErrInvalidInput)
	}
	dayGoal := req.Goal == GoalLowerDay || req.Goal == GoalRaiseDay
	if req.PlateID != 0 && !dayGoal {
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
		if req.Goal == GoalLighter || req.Goal == GoalHeartier {
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
	if dayGoal && !w.noMark {
		in := LightIntent{Date: w.date, Scope: LightScopeDay}
		if req.Goal == GoalRaiseDay {
			in.Level = LevelHearty
		}
		if err := ResolveLight(s, []LightIntent{in}); err != nil {
			return WayResult{}, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
		}
		s.Light = MergeLight(s.Light, []LightIntent{in})
		intent = &in
	}
	if req.Goal == GoalLighter || req.Goal == GoalHeartier {
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
	w.s0 = s
	w.base = Evaluate(s, t)
	w.hard, w.soft = hardKeys(w.base.Issues), softKeys(w.base.Issues)
	w.warn = map[string]bool{}
	for _, is := range w.base.Issues {
		if is.Severity == SeverityWarning {
			w.warn[issueID(is)] = true
		}
	}
	if w.pi >= 0 {
		res.PlateID = s.Plates[w.pi].ID
		w.baseFig = w.figures(w.base, res.PlateID)
		res.Before = w.baseFig
	} else {
		res.Before = w.figures(w.base, 0)
	}
	if d, ok := w.dayEval(w.base); ok {
		res.FloorKcal, res.BandLow, res.BandHigh = math.Round(d.FloorKcal), math.Round(d.BandLow), math.Round(d.BandHigh)
		w.bandHigh = d.BandHigh
		if dayGoal {
			w.capKcal, w.minKcal = d.BandHigh, d.BandLow
			res.MaxKcal, res.MinKcal = res.BandHigh, res.BandLow
		}
	}

	var found, offers []Way
	comps := map[string]int{}
	// compensating re-evaluates the plan many times: a few primary edits of
	// each kind get it.
	canCompensate := func(e wayEdit) bool {
		g := cmp.Or(e.group, e.kind)
		comps[g]++
		return comps[g] <= maxCompPerKind
	}
	offer := func(e wayEdit, ev Evaluation) {
		if !canCompensate(e) {
			return
		}
		for _, c := range w.compensate(e, ev) {
			if rc := w.check(c.s); !rc.other && !rc.floor && (w.goal == GoalLighter || !w.outOfBand(rc.ev)) {
				a := w.way(c, rc, intent)
				a.ChangesOther = true
				if d, ok := w.dayEval(rc.ev); ok && w.outOfBand(rc.ev) {
					a.BelowBand = true
					a.Why = fmt.Sprintf("the day then holds %.0f kcal, %.0f under its band (%.0f to %.0f kcal): the closest the other meal can get it", d.Kcal, d.BandLow-d.Kcal, d.BandLow, d.BandHigh)
					if c.note != "" {
						a.Why += "; " + c.note
					}
				}
				offers = append(offers, a)
			}
		}
	}
	consider := func(e wayEdit) {
		if w.goal == GoalLighter && !w.mealOK(e.s) {
			return
		}
		r := w.check(e.s)
		if r.other || w.leavesRange(r.ev) {
			return
		}
		way := w.way(e, r, intent)
		switch {
		case r.floor:
			// The meal alone would break the floor: only a way that makes
			// the other meal grow gets it to its cap.
			// A light meal that does not fit the day is still a lighter one: the
			// other meal grows within its bound and the offer says how far it got.
			if way.Meaningful && w.goal == GoalLighter {
				offer(e, r.ev)
			}
		case w.goal == GoalHeartier && w.outOfBand(r.ev):
			if way.Reaches {
				offer(e, r.ev)
			}
		case way.Meaningful && (way.Reaches || !w.dayGoal()):
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
	if w.dayGoal() && req.TargetKcal > 0 && req.Goal == GoalLowerDay {
		if d, ok := w.dayEval(w.base); ok && req.TargetKcal < d.FloorKcal-0.5 {
			res.Impossible = true
			res.EmptyWhy = fmt.Sprintf("%.0f kcal is below the day's minimum of %.0f kcal", req.TargetKcal, d.FloorKcal)
			res.Offers = w.dayGreedy(s, intent, func(d DayEval) float64 { return d.FloorKcal + 25 }, false)
			for i := range res.Offers {
				res.Offers[i].next = State{}
			}
			return res, nil
		}
	}
	if w.dayGoal() && len(found) == 0 {
		found = w.dayGreedy(s, intent, w.rangeAim, true)
	}
	res.Ways, res.Offers = DemoteUnclean(found), DemoteUnclean(offers)
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
		cmp.Compare(a.rank.bandGap, b.rank.bandGap),
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
	// worse names a weekly limit the change makes worse without a new issue
	// (more meat in a week already over the DGE limit).
	worse []string
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
		case (is.Severity == SeveritySoft && !w.soft[softKey(is)]) || (is.Severity == SeverityWarning && !w.warn[issueID(is)]):
			is.Options = nil
			r.newSoft = append(r.newSoft, is)
		}
	}
	if w.pi >= 0 {
		if why := newNutritionGap(w.base, r.ev, next.Plates[w.pi]); why != "" {
			r.other = true
		}
	}
	if MeatOverStep(w.base, r.ev) {
		r.other = true
	}
	if a, b := w.base.Week, r.ev.Week; a != nil && b != nil && b.MeatG > a.MeatG+1 &&
		slices.ContainsFunc(r.ev.Issues, func(is Issue) bool { return is.Code == CodeWeekMeatAboveDGE }) {
		r.worse = append(r.worse, PlainText(CodeWeekMeatAboveDGE).Label)
	}
	return r
}

// leavesRange: without a day mark a way keeps the day inside its own range.
func (w *waySearch) leavesRange(ev Evaluation) bool {
	if !w.noMark {
		return false
	}
	d, ok := w.dayEval(ev)
	if !ok || d.BandHigh <= 0 {
		return false
	}
	if w.goal == GoalLowerDay {
		return d.Kcal < d.BandLow-1e-9
	}
	return d.Kcal > d.BandHigh+1e-9
}

func (w *waySearch) dayGoal() bool { return w.goal == GoalLowerDay || w.goal == GoalRaiseDay }

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
		changed = MeaningfulMealKcal(before.MealKcal, before.MealKcal-after.MealKcal)
		reached = maxKcal <= 0 || after.MealKcal <= maxKcal+0.5
		switch {
		case !changed:
			why = fmt.Sprintf("the meal holds %.0f kcal after, %.0f before: not lighter by a meaningful amount (at least %.0f kcal)", after.MealKcal, before.MealKcal, RelMinShare*before.MealKcal)
		case !reached:
			why = fmt.Sprintf("lighter, but not a light meal yet: it now holds %.0f kcal (was %.0f), a light meal holds at most %.0f kcal", after.MealKcal, before.MealKcal, maxKcal)
		}
		return changed, reached, why
	case GoalHeartier:
		changed = MeaningfulMealKcal(before.MealKcal, after.MealKcal-before.MealKcal)
		reached = minKcal <= 0 || after.MealKcal >= minKcal-0.5
		switch {
		case !changed:
			why = fmt.Sprintf("the meal holds %.0f kcal after, %.0f before: not heartier by a meaningful amount (at least %.0f kcal)", after.MealKcal, before.MealKcal, RelMinShare*before.MealKcal)
		case !reached:
			why = fmt.Sprintf("heartier, but not a filling meal yet: it now holds %.0f kcal (was %.0f), a filling meal holds at least %.0f kcal", after.MealKcal, before.MealKcal, minKcal)
		}
		return changed, reached, why
	}
	switch goal {
	case GoalLowerDay:
		changed = MeaningfulKcal(before.DayKcal, before.DayKcal-after.DayKcal)
		reached = maxKcal <= 0 || after.DayKcal <= maxKcal+0.5
		switch {
		case !changed:
			why = fmt.Sprintf("the day holds %.0f kcal after, %.0f before: not lower by a meaningful amount (at least %.0f kcal)", after.DayKcal, before.DayKcal, math.Min(RelMinKcal, RelMinShare*before.DayKcal))
		case !reached:
			why = fmt.Sprintf("lower, but not in the lowered range yet: the day holds %.0f kcal (was %.0f), the range tops at %.0f kcal", after.DayKcal, before.DayKcal, maxKcal)
		}
		return changed, reached, why
	case GoalRaiseDay:
		changed = MeaningfulKcal(before.DayKcal, after.DayKcal-before.DayKcal)
		reached = minKcal <= 0 || after.DayKcal >= minKcal-0.5
		switch {
		case !changed:
			why = fmt.Sprintf("the day holds %.0f kcal after, %.0f before: not higher by a meaningful amount (at least %.0f kcal)", after.DayKcal, before.DayKcal, math.Min(RelMinKcal, RelMinShare*before.DayKcal))
		case !reached:
			why = fmt.Sprintf("higher, but not in the raised range yet: the day holds %.0f kcal (was %.0f), the range starts at %.0f kcal", after.DayKcal, before.DayKcal, minKcal)
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
	case GoalLowerDay:
		return a.Before.DayKcal - a.After.DayKcal
	case GoalRaiseDay:
		return a.After.DayKcal - a.Before.DayKcal
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
		Before: before, After: after, NewIssues: r.newSoft, Worsened: r.worse, next: e.s,
	}
	if w.goal == GoalMoreProtein {
		// The day's figures are the day's, whichever plate changed.
		a.Before.DayKcal, a.Before.DayProteinG = w.figures(w.base, 0).DayKcal, w.figures(w.base, 0).DayProteinG
	}
	changed, reached, _ := JudgeRelative(w.goal, w.pi >= 0, w.capKcal, w.minKcal, before, after)
	if w.goal == GoalRaiseDay && w.bandHigh > 0 && after.DayKcal > w.bandHigh+0.5 {
		reached = false
	}
	a.Meaningful, a.Reaches = changed, changed && reached
	if e.note != "" && a.Why == "" {
		a.Why = e.note
	}
	if a.Reaches && intent != nil {
		a.Light = append(a.Light, *intent)
	}
	a.Light = append(a.Light, e.clear...)
	fit := 0.0
	if d, ok := w.dayEval(r.ev); ok && d.TargetKcal != nil {
		fit = math.Abs(d.Kcal - *d.TargetKcal)
		if w.goal == GoalLighter && d.BandHigh > 0 {
			fit = math.Abs(d.Kcal - dayAim(d))
		}
	}
	other := 0
	for _, is := range r.newSoft {
		if is.Rule != "S2" {
			other++
		}
	}
	a.rank = wayRank{notReaching: !a.Reaches, newSoft: other, belowBand: len(r.newSoft) - other, cost: e.cost, dayFit: fit}
	a.rank.progress = w.progress(a)
	if d, ok := w.dayEval(r.ev); ok && d.BandLow > 0 {
		a.rank.bandGap = math.Round(math.Max(0, d.BandLow-d.Kcal)/10) * 10
	}
	if w.goal != GoalMoreProtein {
		a.rank.step = math.Round(w.progress(a) / 20)
	} else {
		// Whole 5 g steps: a gram more does not outrank the day's fit.
		a.rank.protein = math.Floor(w.progress(a) / 5)
		a.rank.bandGap = 0
	}
	return a
}

// noWayWhy says in plain words, with the numbers, why the named meal alone
// has no meaningful way.
func (w *waySearch) noWayWhy(res WayResult) string {
	switch w.goal {
	case GoalLighter:
		return fmt.Sprintf("This meal cannot get meaningfully lighter on its own: it holds %.0f kcal (a change needs at least %.0f kcal) and the day holds %.0f kcal, its minimum is %.0f kcal, so the day has only %.0f kcal of room. A light meal holds at most %.0f kcal.",
			res.Before.MealKcal, RelMinShare*res.Before.MealKcal, res.Before.DayKcal, res.FloorKcal, math.Max(0, res.Before.DayKcal-res.FloorKcal), res.MaxKcal)
	case GoalHeartier:
		return fmt.Sprintf("This meal cannot get meaningfully heartier on its own without taking the day (%.0f kcal) over its band top of %.0f kcal.", res.Before.DayKcal, res.BandHigh)
	case GoalLowerDay, GoalRaiseDay:
		return fmt.Sprintf("No single change of one meal brings the day (%.0f kcal) into its adjusted range of %.0f to %.0f kcal without breaking the energy floor of %.0f kcal.", res.Before.DayKcal, res.BandLow, res.BandHigh, res.FloorKcal)
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
			out = append(out, w.sideSwapEdits(s, pi)...)
		case GoalLowerDay:
			out = append(out, w.quantityEdits(s, pi, -1)...)
			out = append(out, w.removeEdits(s, pi)...)
			out = append(out, w.leanEdits(s, pi)...)
			out = append(out, w.swapEdits(s, p, AltMain, 0)...)
		case GoalRaiseDay:
			out = append(out, w.quantityEdits(s, pi, +1)...)
			// A side or bread that belongs to the dish, no gap fillers.
			for _, c := range w.breadCands(s, p, (w.minKcal-w.figures(w.base, 0).DayKcal)/2, w.base) {
				out = append(out, w.addEdit(s, pi, c, ""))
			}
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
		factors := []float64{0.75, 0.5, 0.25}
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

// wayLevels are the quantities a way may give component c of plate p: never a
// fraction under one portion per eater of a dish cooked fresh for this plate
// alone (nobody cooks half a portion); a smaller serving of a leftover or of a
// batch other plates eat from is fine.
func (s State) wayLevels(p Plate, c Component, dir int) []float64 {
	all := levels(c, dir)
	if c.Portions == nil || !s.cookedFreshFor(p, c) {
		return all
	}
	return slices.DeleteFunc(all, func(n float64) bool { return n < 1-1e-9 })
}

// cookedFreshFor reports whether c's batch cooks at p's own cell and no other
// plate eats from it.
func (s State) cookedFreshFor(p Plate, c Component) bool {
	same := func(o Component) bool {
		return (c.SessionID != 0 && o.SessionID == c.SessionID) || (c.SessionKey != "" && o.SessionKey == c.SessionKey)
	}
	if c.SessionID == 0 && c.SessionKey == "" {
		return true // a fresh session of its own
	}
	cooksHere := false
	for _, ss := range s.Sessions {
		if (c.SessionID != 0 && ss.ID == c.SessionID) || (c.SessionKey != "" && ss.Key == c.SessionKey) {
			cooksHere = ss.CookDate.Equal(p.Date) && ss.CookSlotID == p.SlotID
		}
	}
	if !cooksHere {
		return false
	}
	for _, q := range s.Plates {
		if q.ID == p.ID {
			continue
		}
		for _, o := range q.Components {
			if same(o) {
				return false
			}
		}
	}
	return true
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
		return fmt.Sprintf("%s: %s portions (was %s)", food.Quote(f.Name), trimNum(n), trimNum(*c.Portions))
	}
	return fmt.Sprintf("%s: %.0f g (was %.0f g)", food.Quote(f.Name), n, *c.Grams)
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
		for _, n := range s.wayLevels(p, c, dir) {
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
			names = append(names, food.Quote(s.Foods[c.FoodID].Name))
		}
	}
	mc := p.Components[mi]
	mf, ok := s.Foods[mc.FoodID]
	if !ok || !mf.NutritionComplete {
		return nil
	}
	var out []wayEdit
	for _, n := range append([]float64{-1}, s.wayLevels(p, mc, -1)...) {
		ns, nops, what := removeComponents(s, pi, drop...), slices.Clone(ops), []string{"leave out " + strings.Join(names, " and ")}
		if n > 0 {
			mci := mi - countBelow(drop, mi)
			ns = setQuantity(ns, pi, mci, n)
			nops = append([]OptionOp{quantityOp(p, mc, n)}, nops...)
			what = append([]string{quantityWords(mf, mc, n)}, what...)
		}
		out = append(out, wayEdit{s: ns, ops: nops, what: what, cost: 2 + len(drop) + btoi(n > 0), kind: "portion", plate: p.ID})
	}
	out = append(out, w.keepOneSideEdits(s, pi, mi)...)
	return out
}

// keepOneSideEdits keep one side of the plate (as it is or smaller) next to
// the main (as it is or smaller) and leave the other components out: a
// compound way can then fill the meal up to its cap instead of stopping at the
// bare main.
func (w *waySearch) keepOneSideEdits(s State, pi, mi int) []wayEdit {
	p := s.Plates[pi]
	mc := p.Components[mi]
	mf, ok := s.Foods[mc.FoodID]
	if !ok || !mf.NutritionComplete {
		return nil
	}
	var out []wayEdit
	for k, kc := range p.Components {
		kf, ok := s.Foods[kc.FoodID]
		if k == mi || !ok || !kf.NutritionComplete {
			continue
		}
		var drop []int
		var rm []OptionOp
		var names []string
		for ci, c := range p.Components {
			if ci != mi && ci != k {
				drop = append(drop, ci)
				cid := c.ID
				rm = append(rm, OptionOp{Op: "remove", PlateComponentID: &cid})
				names = append(names, food.Quote(s.Foods[c.FoodID].Name))
			}
		}
		for _, mn := range append([]float64{-1}, s.wayLevels(p, mc, -1)...) {
			for _, kn := range append([]float64{-1}, s.wayLevels(p, kc, -1)...) {
				if mn < 0 && kn < 0 && len(drop) == 0 {
					continue
				}
				ns := removeComponents(s, pi, drop...)
				var ops []OptionOp
				var what []string
				if mn > 0 {
					ns = setQuantity(ns, pi, mi-countBelow(drop, mi), mn)
					ops, what = append(ops, quantityOp(p, mc, mn)), append(what, quantityWords(mf, mc, mn))
				}
				if kn > 0 {
					ns = setQuantity(ns, pi, k-countBelow(drop, k), kn)
					ops, what = append(ops, quantityOp(p, kc, kn)), append(what, quantityWords(kf, kc, kn))
				}
				if len(drop) > 0 {
					ops, what = append(ops, rm...), append(what, "leave out "+strings.Join(names, " and "))
				}
				out = append(out, wayEdit{s: ns, ops: ops, what: what, cost: 3 + len(drop) + btoi(mn > 0) + btoi(kn > 0), kind: "portion", group: "keep", plate: p.ID})
			}
		}
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
		if !ok || onPlate[id] || !f.NutritionComplete || f.Macros.Kcal <= 0 || f.Sweet || !fitsDiet(f.DietClass, w.household) || (f.Composed() && f.Role == food.RoleStandalone) || (!f.Composed() && !s.addOnFits(p, f, slotName(s, p.SlotID))) {
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
		words = fmt.Sprintf("add %s portions of %s (+%.0f kcal)", trimNum(*c.comp.Portions), food.Quote(c.f.Name), c.kcal)
	} else {
		op.Amount, op.Unit = c.comp.Grams, "g"
		words = fmt.Sprintf("add %.0f g %s (+%.0f kcal)", c.grams, food.Quote(c.f.Name), c.kcal)
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
		// Only a lunch or dinner takes over energy: nobody has potatoes for
		// breakfast.
		if !slices.ContainsFunc(lightSlots, func(n string) bool { return s.slotIs(p.SlotID, n) }) {
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
	if w.pi < 0 || w.goal == GoalMoreProtein || w.dayGoal() {
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
	if d.Kcal >= math.Max(d.FloorKcal, d.BandLow)-1e-9 {
		return 0
	}
	return math.Max(d.FloorKcal, dayAim(d)) - d.Kcal
}

// dayAim is the kcal a compound way aims the day at: the lower third of its
// band, not the floor and not the band's edge.
func dayAim(d DayEval) float64 {
	if d.BandHigh <= 0 {
		return d.Kcal
	}
	return d.BandLow + (d.BandHigh-d.BandLow)/3
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
		if w.goal == GoalLighter && d.BandHigh > 0 {
			fit = math.Abs(d.Kcal - dayAim(d))
		}
		if d.BandHigh > 0 && d.Kcal > d.BandHigh+1e-9 {
			fit += 5000
		}
	}
	for _, is := range r.newSoft {
		if is.Rule != "S2" {
			soft++
		}
	}
	return fit + 400*float64(soft+len(r.worse)) + 100000*float64(btoi(r.floor)), true
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
	// The other meal never grows past the biggest meal of its kind the plan
	// already has (else a filling meal of this household): no lunch of 1,100 kcal
	// for a household whose lunches hold 330 to 740.
	bound, boundWhy := w.mealBound(e.s, q)
	pick := func(cands []wayEdit) (wayEdit, bool) {
		best, bestScore, ok := wayEdit{}, math.Inf(1), false
		for _, c := range cands {
			r := w.check(c.s)
			if sc, good := w.score(r); good && sc < bestScore && w.plateKcal(r.ev, q.ID) <= bound+1e-9 {
				best, bestScore, ok = c, sc, true
			}
		}
		return best, ok
	}
	if boundWhy != "" {
		e.note = "the " + who + " stays within " + boundWhy
	}
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
	// Then a side recipe from the library (verified like find_alternatives:
	// cooked on the day's cook cell or eaten from a batch), then bread up to
	// its add-on limit.
	if w.lift(ev) > 0 {
		res, err := Alternatives(e.s, w.t, AltRequest{PlateID: q.ID, What: AltAdd, Limit: 8, Constraints: AltConstraints{Roles: []food.Role{food.RoleSideStarch, food.RoleSideVeg}}})
		if err == nil {
			var cands []wayEdit
			for _, a := range res.Alternatives {
				if len(a.Ops) > 0 && a.Source != SourceLeaf && (roleOf(e.s.Foods[a.FoodID]) != food.RoleSideStarch || !hasStarch(e.s, e.s.Plates[qi])) {
					cands = append(cands, wayEdit{s: a.next, ops: a.Ops, what: []string{"add " + a.Name}, cost: 2})
				}
			}
			if c, ok := pick(cands); ok {
				apply(c)
			}
		}
	}
	if need := w.lift(ev); need > 0 && !hasStarch(e.s, e.s.Plates[qi]) {
		var cands []wayEdit
		for _, ac := range w.breadCands(e.s, e.s.Plates[qi], need+1, ev) {
			cands = append(cands, w.addEdit(e.s, qi, ac, ""))
		}
		if c, ok := pick(cands); ok {
			apply(c)
		}
	}
	return e, ev
}

// hasStarch reports whether plate p already holds a starch side (a second one
// is never added: potatoes and bread on one plate).
func hasStarch(s State, p Plate) bool {
	for _, c := range p.Components {
		if f, ok := s.Foods[c.FoodID]; ok && roleOf(f) == food.RoleSideStarch {
			return true
		}
	}
	return false
}

// plateKcal is plate id's kcal per eater in ev.
func (w *waySearch) plateKcal(ev Evaluation, id int64) float64 {
	for _, pe := range ev.Plates {
		if pe.PlateID == id {
			return pe.Kcal
		}
	}
	return 0
}

// mealBound is the most kcal a compensating meal q may hold: the biggest meal
// of its slot the plan has on other days (never below what q holds now), or,
// with none, a filling meal's minimum. The text says which.
func (w *waySearch) mealBound(s State, q Plate) (float64, string) {
	cur := w.plateKcal(w.base, q.ID)
	best := 0.0
	for _, pe := range w.base.Plates {
		if pe.SlotID == q.SlotID && pe.Date != w.date && pe.PlateID > 0 {
			best = math.Max(best, pe.Kcal)
		}
	}
	slot := slotName(s, q.SlotID)
	if best > 0 {
		return math.Max(best, cur), fmt.Sprintf("the biggest %s of your plan (%.0f kcal)", slot, best)
	}
	if _, min, err := MealLimit(s, w.t, q.ID, GoalHeartier); err == nil && min > 0 {
		return math.Max(min, cur), fmt.Sprintf("a filling %s (%.0f kcal)", slot, min)
	}
	return math.Inf(1), ""
}

// breadCands are the bread leaves that fit a plate, sized closest to want kcal
// but not above the add-on limit the rules judge them by (addCapG on the
// day's target): a larger helping would be a new warning.
func (w *waySearch) breadCands(s State, p Plate, want float64, ev Evaluation) []addCand {
	onPlate := map[int64]bool{}
	for _, c := range p.Components {
		onPlate[c.FoodID] = true
	}
	target := 0.0
	if d, ok := w.dayEval(ev); ok && d.TargetKcal != nil {
		target = *d.TargetKcal
	}
	var out []addCand
	for id, f := range s.Foods {
		if onPlate[id] || f.Composed() || !f.NoCook || f.LeafRole != food.RoleSideStarch || !f.NutritionComplete || f.Macros.Kcal <= 0 || f.Sweet ||
			!fitsDiet(f.DietClass, w.household) || !s.addOnFits(p, f, slotName(s, p.SlotID)) {
			continue
		}
		limit := addCapG(f, target)
		var best *addCand
		for n := 0.5; n*ServingGrams(f) <= limit+1e-9 || best == nil; n += 0.5 {
			g := math.Min(math.Round(n*ServingGrams(f)/roundGrams)*roundGrams, math.Floor(limit/roundGrams)*roundGrams)
			if g <= 0 {
				break
			}
			ac := addCand{f: f, comp: Component{FoodID: id, Grams: &g, Proposed: true}, kcal: f.Macros.Kcal * g / 100, grams: g}
			if best == nil || math.Abs(ac.kcal-want) < math.Abs(best.kcal-want) {
				best = &ac
			}
			if g >= limit-1e-9 {
				break
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

// dayGreedy builds a way of up to three edits, each on a component not
// yet changed and each the one that brings the day closest to the aim of its
// adjusted range (the lower third for a raise, the upper third for a lower),
// until the day is inside the range: a day goal that no single change reaches.
func (w *waySearch) dayGreedy(s State, intent *LightIntent, aimOf func(DayEval) float64, stop bool) []Way {
	cur := wayEdit{s: s, kind: "day"}
	used := map[int64]bool{} // components an earlier edit of the way changed
	var last Way
	for range wayCompSteps {
		best, bestScore := wayEdit{}, math.Inf(1)
		for _, e := range w.primary(cur.s) {
			if slices.ContainsFunc(e.ops, func(o OptionOp) bool { return o.PlateComponentID != nil && used[*o.PlateComponentID] }) {
				continue
			}
			r := w.check(e.s)
			if r.other || r.floor || w.leavesRange(r.ev) {
				continue
			}
			d, ok := w.dayEval(r.ev)
			if !ok || d.BandHigh <= 0 {
				continue
			}
			sc := math.Abs(d.Kcal-aimOf(d)) + 400*float64(len(r.worse))
			for _, is := range r.newSoft {
				if is.Rule != "S2" {
					sc += 400
				}
			}
			if sc < bestScore {
				best, bestScore = e, sc
			}
		}
		if len(best.ops) == 0 {
			return nil
		}
		for _, o := range best.ops {
			if o.PlateComponentID != nil {
				used[*o.PlateComponentID] = true
			}
		}
		cur.s = best.s
		cur.ops = append(slices.Clone(cur.ops), best.ops...)
		cur.what = append(slices.Clone(cur.what), best.what...)
		cur.cost += best.cost
		r := w.check(cur.s)
		if r.other || r.floor {
			return nil
		}
		way := w.way(cur, r, intent)
		if stop && way.Reaches {
			return []Way{way}
		}
		last = way
	}
	if !stop && len(last.Ops) > 0 {
		return []Way{last}
	}
	return nil
}

// isAddOn reports whether component c of plate p is an add-on (an extra, or a
// no-cook leaf that is not the main) rather than a side or the main.
func isAddOn(s State, p Plate, i int) bool {
	c := p.Components[i]
	if isMainComp(s, p, i) {
		return false
	}
	f, ok := s.Foods[c.FoodID]
	return c.Position == plate.PositionExtra || (ok && !f.Composed() && slices.Contains(s.NoCook, c.FoodID))
}

// mealOK keeps a lighter meal a meal: its main stays at half a portion (half
// the grams of a leaf main) or more, and a plate that had a side keeps a side
// (a lighter side before none), an add-on being no side.
func (w *waySearch) mealOK(next State) bool {
	if w.pi < 0 {
		return true
	}
	was, now := w.s0.Plates[w.pi], next.Plates[w.pi]
	sides := func(s State, p Plate) (n int) {
		for i := range p.Components {
			if !isMainComp(s, p, i) && !isAddOn(s, p, i) {
				n++
			}
		}
		return n
	}
	if sides(w.s0, was) > 0 && sides(next, now) == 0 {
		return false
	}
	var wasMain float64
	for i, c := range was.Components {
		if isMainComp(w.s0, was, i) {
			wasMain = c.Multiplier()
			if c.Portions != nil {
				wasMain = 1 // portions: the absolute half portion below
			}
		}
	}
	for i, c := range now.Components {
		if !isMainComp(next, now, i) {
			continue
		}
		if c.Portions != nil {
			return *c.Portions >= wayPortionMin-1e-9
		}
		return c.Multiplier() >= 0.5*wasMain-1e-9
	}
	return false // no main left
}

// sideSwapEdits swap a side for a lighter one from the library, with the main
// at its current portion or a smaller one and the add-ons kept or left out: a
// meal keeps its side without the side being the potatoes.
func (w *waySearch) sideSwapEdits(s State, pi int) []wayEdit {
	p := s.Plates[pi]
	mi := -1
	for i := range p.Components {
		if isMainComp(s, p, i) {
			mi = i
		}
	}
	if mi < 0 {
		return nil
	}
	side, sideKcal := -1, -1.0
	for i, c := range p.Components {
		if f, ok := s.Foods[c.FoodID]; ok && !isMainComp(s, p, i) && !isAddOn(s, p, i) && f.Macros.Kcal*c.Multiplier() > sideKcal {
			side, sideKcal = i, f.Macros.Kcal*c.Multiplier()
		}
	}
	if side < 0 {
		return nil
	}
	mc := p.Components[mi]
	mf := s.Foods[mc.FoodID]
	var out []wayEdit
	for _, mn := range append([]float64{-1}, s.wayLevels(p, mc, -1)...) {
		for _, dropAdd := range []bool{false, true} {
			s1 := s
			var pre []OptionOp
			var what []string
			if mn > 0 {
				s1 = setQuantity(s1, pi, mi, mn)
				pre, what = append(pre, quantityOp(p, mc, mn)), append(what, quantityWords(mf, mc, mn))
			}
			if dropAdd {
				var drop []int
				for i := range p.Components {
					if isAddOn(s, p, i) {
						drop = append(drop, i)
						cid := p.Components[i].ID
						pre = append(pre, OptionOp{Op: "remove", PlateComponentID: &cid})
						what = append(what, "leave out "+s.Foods[p.Components[i].FoodID].Name)
					}
				}
				if len(drop) == 0 {
					continue
				}
				// Component ids stay, the side's index shifts: Alternatives
				// finds it by id.
				s1 = removeComponents(s1, pi, drop...)
			}
			res, err := Alternatives(s1, w.t, AltRequest{PlateID: p.ID, What: AltSide, ComponentID: p.Components[side].ID, NoLimit: true, TolerateFloor: true})
			if err != nil {
				continue
			}
			// The lightest plates first: a lighter side is the point.
			// A side recipe, not a raw leaf (a leaf beside the main is an add-on).
			alts := slices.DeleteFunc(slices.Clone(res.Alternatives), func(a Alternative) bool { return a.Source == SourceLeaf })
			slices.SortStableFunc(alts, func(a, b Alternative) int { return cmp.Compare(a.PlateKcal, b.PlateKcal) })
			alts = alts[:min(len(alts), 3)]
			for _, a := range alts {
				if len(a.Ops) == 0 {
					continue
				}
				ops := append(slices.Clone(pre), a.Ops...)
				out = append(out, wayEdit{s: a.next, ops: ops, what: append(slices.Clone(what), "swap "+food.Quote(s.Foods[p.Components[side].FoodID].Name)+" for "+food.Quote(a.Name)), cost: 4 + len(pre), kind: "swap", group: "sideswap", plate: p.ID})
			}
		}
	}
	return out
}

// rangeAim is the kcal a day goal aims at: the lower third of the raised
// range, the upper third of the lowered one.
func (w *waySearch) rangeAim(d DayEval) float64 {
	if w.goal == GoalLowerDay {
		return d.BandHigh - (d.BandHigh-d.BandLow)/3
	}
	return dayAim(d)
}

// DemoteUnclean keeps only the candidates that create no new warning when one
// of them is clean; when none is, all stay (ranked as they were) and each says
// which warning it creates.
func DemoteUnclean(in []Way) []Way {
	clean := slices.DeleteFunc(slices.Clone(in), func(a Way) bool { return len(a.NewIssues) > 0 || len(a.Worsened) > 0 })
	if len(clean) > 0 {
		return clean
	}
	for i := range in {
		var labels []string
		for _, is := range in[i].NewIssues {
			if l := PlainText(is.Code).Label; !slices.Contains(labels, l) {
				labels = append(labels, l)
			}
		}
		labels = append(labels, in[i].Worsened...)
		if len(labels) > 0 {
			note := "creates a new warning: " + strings.Join(labels, ", ")
			in[i].Why = strings.TrimSpace(strings.Join([]string{in[i].Why, note}, "; "))
			in[i].Why = strings.TrimPrefix(in[i].Why, "; ")
		}
	}
	return in
}
