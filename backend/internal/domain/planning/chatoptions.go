package planning

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/availability"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// Portion bounds of an S6/S6b portion option: a composed food's new portions
// stay within them, a leaf's new grams within half and one and a half times
// the current ones.
const (
	optPortionMin = 0.75
	optPortionMax = 1.5
	optGramsMin   = 0.5
	maxCookMoves  = 2
	maxSwapOpts   = 2
	// MaxOptionIssues caps the issues IssueOptions computes options for, the
	// earliest by date: each option re-evaluates the plan, and the chat tools
	// read the plan on every call.
	MaxOptionIssues = 5
	maxCellTries    = 12
)

// IssueOptions returns issues with computed options on the chat's S6/S6b
// (a lighter or heartier main or side, a portion change, removing an add-on)
// and H2 (move a batch's cook to another cell, or swap the fresh cook for a
// leftover or a no-cook food). Each option is verified like an S2/S3 one: the
// plan re-evaluated with it applied has no new hard issue and the issue is
// gone, so an S6 option may leave the day under its band (a light or hearty
// request ranks above the day and week bands) but never under H9. Only the
// first MaxOptionIssues such issues by date, and only those keep accepts (nil
// accepts all), get options. ev is the evaluation of s the issues come from. The planner's Evaluate has none of
// these options; only the chat tools call this.
func IssueOptions(s State, t Target, ev Evaluation, issues []Issue, keep func(Issue) bool) []Issue {
	out := slices.Clone(issues)
	x := newIndex(s, t)
	order := make([]int, len(out))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(out[a].Date, out[b].Date) })
	n := 0
	for _, i := range order {
		is := out[i]
		light := is.Rule == "S6" || is.Rule == "S6b"
		cook := is.Rule == "H2" && is.Code == CodeTooManyCookOccasions
		if (!light && !cook) || (keep != nil && !keep(is)) {
			continue
		}
		if n++; n > MaxOptionIssues {
			break
		}
		if light {
			out[i].Options = x.lightOptions(ev, t, is)
		} else {
			out[i].Options = x.cookOptions(ev, t, is)
		}
	}
	return out
}

// paramF reads a numeric issue param.
func paramF(is Issue, key string) float64 {
	switch v := is.Params[key].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	}
	return 0
}

// gone reports whether next, re-evaluated, adds no hard issue that base
// lacked and no longer holds the issue is. It returns that evaluation.
func gone(next State, t Target, base Evaluation, is Issue) (Evaluation, bool) {
	ev := Evaluate(next, t)
	hard, id := hardKeys(base.Issues), issueID(is)
	for _, e := range ev.Issues {
		if issueID(e) == id || (e.Severity == SeverityViolation && !hard[issueID(e)]) {
			return ev, false
		}
	}
	return ev, true
}

// afterFigures are the plate's kcal and its day's kcal and cook occasions in ev.
func afterFigures(ev Evaluation, p Plate) (plateKcal, dayKcal float64, cooks int) {
	date := p.Date.Format(time.DateOnly)
	for _, pe := range ev.Plates {
		if pe.PlateID == p.ID {
			plateKcal = math.Round(pe.Kcal)
		}
	}
	for _, d := range ev.Days {
		if d.Date == date {
			dayKcal, cooks = math.Round(d.Kcal), d.CookOccasions
		}
	}
	return
}

func (s State) plateAt(date string, slot int64) (int, bool) {
	i := slices.IndexFunc(s.Plates, func(p Plate) bool {
		return p.Date.Format(time.DateOnly) == date && p.SlotID == slot && !p.Confirmed && p.ID > 0
	})
	return i, i >= 0
}

// lightOptions are the verified ways to make the plate of an S6 (lighter) or
// S6b (heartier) issue meet its cap or minimum, at most one portion change,
// one add-on removal and maxSwapOpts swaps, in that order.
func (x *index) lightOptions(ev Evaluation, t Target, is Issue) []Option {
	opts := []Option{}
	if is.SlotID == nil {
		return opts
	}
	pi, ok := x.s.plateAt(is.Date, *is.SlotID)
	if !ok || x.routine(x.s.Plates[pi].Cell()) {
		return opts
	}
	p := x.s.Plates[pi]
	hearty := is.Rule == "S6b"
	edit := func(mut func(c []Component) []Component) State {
		next := x.s
		next.Plates = slices.Clone(x.s.Plates)
		q := p
		q.Components = mut(slices.Clone(p.Components))
		next.Plates[pi] = q
		return next
	}
	plateKcal := 0.0
	for _, c := range p.Components {
		if f, ok := x.s.Foods[c.FoodID]; ok {
			plateKcal += f.Macros.Kcal * c.Multiplier()
		}
	}
	// A portion change: the main first, then the sides, the one that closes
	// the gap alone.
	need := plateKcal - (paramF(is, "max_kcal") - 1)
	if hearty {
		need = paramF(is, "min_kcal") + 1 - plateKcal
	}
	order := make([]int, len(p.Components))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int {
		return cmp.Compare(btoi(!isMainComp(x.s, p, a)), btoi(!isMainComp(x.s, p, b)))
	})
portions:
	for _, ci := range order {
		c := p.Components[ci]
		f, ok := x.s.Foods[c.FoodID]
		if !ok || !f.NutritionComplete || f.Macros.Kcal <= 0 || need <= 0 {
			continue
		}
		var cur, step, per float64
		switch {
		case c.Portions != nil && *c.Portions > 0:
			cur, step, per = *c.Portions, roundPortions, f.Macros.Kcal
		case c.Grams != nil && *c.Grams > 0:
			cur, step, per = *c.Grams, roundGrams, f.Macros.Kcal/100
		default:
			continue
		}
		n := cur - need/per
		if hearty {
			n = math.Ceil((cur+need/per)/step-1e-9) * step
		} else {
			n = math.Floor(n/step+1e-9) * step
		}
		if n <= 0 || n == cur {
			continue
		}
		if c.Portions != nil && (n < optPortionMin-1e-9 || n > optPortionMax+1e-9) {
			continue
		}
		if c.Grams != nil && (n < cur*optGramsMin-1e-9 || n > cur*optPortionMax+1e-9) {
			continue
		}
		next := edit(func(cs []Component) []Component {
			if c.Portions != nil {
				cs[ci].Portions = &n
			} else {
				cs[ci].Grams = &n
			}
			return cs
		})
		after, good := gone(next, t, ev, is)
		if !good {
			continue
		}
		o := x.lightOption(after, p, "portions", c.FoodID, f.Name, plateKcal)
		from := cur
		op := OptionOp{Op: "set_quantity", PlateComponentID: &c.ID}
		if c.Portions != nil {
			o.Portions, o.From, op.Portions = &n, &from, &n
		} else {
			g := n
			o.Grams, o.From, op.Amount, op.Unit = &g, &from, &g, "g"
		}
		o.Op = op
		opts = append(opts, o)
		break portions
	}
	if !hearty {
		for ci, c := range p.Components {
			f, ok := x.s.Foods[c.FoodID]
			if !ok || len(p.Components) < 2 || !x.addOn(c) {
				continue
			}
			next := edit(func(cs []Component) []Component { return slices.Delete(cs, ci, ci+1) })
			after, good := gone(next, t, ev, is)
			if !good {
				continue
			}
			o := x.lightOption(after, p, "remove", c.FoodID, f.Name, plateKcal)
			o.Op = OptionOp{Op: "remove", PlateComponentID: &c.ID}
			opts = append(opts, o)
			break
		}
	}
	light := AltLighter
	if hearty {
		light = AltHeartier
	}
	swaps := 0
	for _, req := range x.swapRequests(p) {
		req.Light = light
		res, err := Alternatives(x.s, t, req)
		if err != nil {
			continue
		}
		// The whole-meal swap has its own slot: it is the only option for a
		// plate far over its cap.
		whole := req.What == AltMeal
		for _, a := range res.Alternatives {
			if len(a.NewIssues) > 0 || len(a.Ops) == 0 || (!whole && swaps >= maxSwapOpts) {
				continue
			}
			opts = append(opts, x.altOption(a, p, plateKcal))
			if !whole {
				swaps++
			}
			break
		}
	}
	return opts
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// swapRequests are the swaps worth asking for on p: the whole meal when it
// has sides or add-ons (replace the main, drop the rest), its main (when
// composed) and its largest side (a leaf too).
func (x *index) swapRequests(p Plate) []AltRequest {
	var out []AltRequest
	if len(p.Components) > 1 {
		out = append(out, AltRequest{PlateID: p.ID, What: AltMeal, Limit: 3})
	}
	side, sideKcal := 0, -1.0
	for i, c := range p.Components {
		f, ok := x.s.Foods[c.FoodID]
		if !ok {
			continue
		}
		if isMainComp(x.s, p, i) {
			if f.Composed() {
				out = append(out, AltRequest{PlateID: p.ID, What: AltMain, ComponentID: c.ID, Limit: 3})
			}
			continue
		}
		if k := f.Macros.Kcal * c.Multiplier(); k > sideKcal {
			side, sideKcal = int(c.ID), k
		}
	}
	if side > 0 {
		out = append(out, AltRequest{PlateID: p.ID, What: AltSide, ComponentID: int64(side), Limit: 3})
	}
	return out
}

// addOn reports whether c is an add-on: an extra, or a no-cook leaf that is
// not the main.
func (x *index) addOn(c Component) bool {
	if c.Position == plate.PositionExtra {
		return true
	}
	f := x.s.Foods[c.FoodID]
	return c.Position != plate.PositionMain && !f.Composed() && slices.Contains(x.s.NoCook, c.FoodID)
}

func (x *index) lightOption(after Evaluation, p Plate, action string, foodID int64, name string, before float64) Option {
	pk, dk, cooks := afterFigures(after, p)
	id := p.ID
	return Option{
		Action: action, FoodID: foodID, Name: name, Date: p.Date.Format(time.DateOnly), SlotID: p.SlotID, PlateID: &id,
		PlateKcal: pk, KcalDelta: math.Round(pk - before), DayKcal: dk, CookOccasions: cooks,
	}
}

// altOption writes an alternative as a swap option.
func (x *index) altOption(a Alternative, p Plate, plateKcal float64) Option {
	id := p.ID
	o := Option{
		Action: "swap", FoodID: a.FoodID, Name: a.Name, Date: p.Date.Format(time.DateOnly), SlotID: p.SlotID, PlateID: &id,
		KcalDelta: math.Round(a.PlateKcal - plateKcal), PlateKcal: a.PlateKcal, DayKcal: a.DayKcal, CookOccasions: a.CookOccasionsDay,
		Op: a.Ops[0],
	}
	if len(a.Ops) > 1 {
		o.Ops = a.Ops
	}
	return o
}

// cookOptions are the verified ways to bring a day with too many cooking
// occasions within the limit: move a batch cooked that day to another cell
// (at most maxCookMoves, nearest first; a cell of the day that already holds a
// cook is nearest), when no single move does it move all of the day's batches
// to one cell (one option with an op per batch), or swap the main of a plate
// that cooks fresh for a leftover or a no-cook food (at most maxSwapOpts).
func (x *index) cookOptions(ev Evaluation, t Target, is Issue) []Option {
	opts := []Option{}
	day, err := time.Parse(time.DateOnly, is.Date)
	if err != nil {
		return opts
	}
	limit := x.s.Settings.MaxCookOccasionsPerDay
	hard, id := hardKeys(ev.Issues), issueID(is)
	resolves := func(next State) bool {
		for _, e := range Check(next, t) {
			if issueID(e) == id || (e.Severity == SeverityViolation && !hard[issueID(e)]) {
				return false
			}
		}
		return true
	}
	for _, ss := range x.s.Sessions {
		if ss.ID <= 0 || ss.Confirmed || ss.Proposed() || dayOf(ss.CookDate) != dayOf(day) || len(opts) >= maxCookMoves {
			continue
		}
		cells := slices.Clone(x.s.Cells)
		slices.SortStableFunc(cells, func(a, b Cell) int {
			return cmp.Compare(math.Abs(float64(dayOf(a.Date)-dayOf(day))), math.Abs(float64(dayOf(b.Date)-dayOf(day))))
		})
		tries := 0
		for _, c := range cells {
			k := c.Key()
			if tries >= maxCellTries || len(opts) >= maxCookMoves {
				break
			}
			// Another cell of the day is a target only when it already holds
			// a cook: the day then has one occasion fewer.
			if dayOf(c.Date) == dayOf(day) && (!x.occasions[dayOf(day)][idOf(k)] || (dayOf(c.Date) == dayOf(ss.CookDate) && c.SlotID == ss.CookSlotID)) || c.Date.Before(x.s.Today) || !x.s.Window.Contains(c.Date) || c.Mode == availability.ModeNone || x.routine(k) || !x.inTarget(k) {
				continue
			}
			tries++
			next := x.s
			next.Sessions = slices.Clone(x.s.Sessions)
			for i := range next.Sessions {
				if next.Sessions[i].ID == ss.ID {
					next.Sessions[i].CookDate, next.Sessions[i].CookSlotID = c.Date, c.SlotID
				}
			}
			if !resolves(next) {
				continue
			}
			sid := ss.ID
			date := c.Date.Format(time.DateOnly)
			opts = append(opts, Option{
				Action: "move_cook", FoodID: ss.FoodID, Name: x.s.Foods[ss.FoodID].Name, Date: date, SlotID: c.SlotID,
				CookOccasions: len(newIndex(next, t).occasions[dayOf(day)]),
				Op:            OptionOp{Op: "move_cook", SessionID: &sid, CookAt: &OptionCell{Date: date, SlotID: c.SlotID}},
			})
		}
	}
	if len(opts) == 0 {
		opts = append(opts, x.groupedCookMove(t, day, resolves)...)
	}
	swaps := 0
	for _, p := range x.s.Plates {
		if dayOf(p.Date) != dayOf(day) || p.Confirmed || p.ID <= 0 || !x.occasions[dayOf(day)][idOf(p.Cell())] || x.routine(p.Cell()) {
			continue
		}
		plateKcal := 0.0
		for _, c := range p.Components {
			plateKcal += x.s.Foods[c.FoodID].Macros.Kcal * c.Multiplier()
		}
		res, err := Alternatives(x.s, t, AltRequest{PlateID: p.ID, What: AltMain, Limit: maxAltLimit})
		if err != nil {
			continue
		}
		for _, a := range res.Alternatives {
			if a.CookOccasionsDay <= limit && len(a.NewIssues) == 0 && len(a.Ops) > 0 && swaps < maxSwapOpts {
				opts = append(opts, x.altOption(a, p, plateKcal))
				swaps++
			}
		}
	}
	return opts
}

// groupedCookMove is the option that moves every batch cooked on day to one
// of the day's cook cells, for a day where moving one batch alone leaves too
// many occasions: one option whose ops are a move_cook per batch.
func (x *index) groupedCookMove(t Target, day time.Time, resolves func(State) bool) []Option {
	var group []Session
	for _, ss := range x.s.Sessions {
		if ss.ID > 0 && !ss.Confirmed && !ss.Proposed() && dayOf(ss.CookDate) == dayOf(day) {
			group = append(group, ss)
		}
	}
	if len(group) < 2 {
		return nil
	}
	for _, c := range x.s.Cells {
		k := c.Key()
		if dayOf(c.Date) != dayOf(day) || !x.occasions[dayOf(day)][idOf(k)] || c.Date.Before(x.s.Today) || !x.s.Window.Contains(c.Date) || c.Mode == availability.ModeNone || x.routine(k) || !x.inTarget(k) {
			continue
		}
		next := x.s
		next.Sessions = slices.Clone(x.s.Sessions)
		var ops []OptionOp
		var names []string
		date := c.Date.Format(time.DateOnly)
		for i := range next.Sessions {
			ss := &next.Sessions[i]
			if !slices.ContainsFunc(group, func(g Session) bool { return g.ID == ss.ID }) || (dayOf(ss.CookDate) == dayOf(c.Date) && ss.CookSlotID == c.SlotID) {
				continue
			}
			sid := ss.ID
			ss.CookDate, ss.CookSlotID = c.Date, c.SlotID
			names = append(names, x.s.Foods[ss.FoodID].Name)
			ops = append(ops, OptionOp{Op: "move_cook", SessionID: &sid, CookAt: &OptionCell{Date: date, SlotID: c.SlotID}})
		}
		if len(ops) < 2 || !resolves(next) {
			continue
		}
		return []Option{{
			Action: "move_cook", FoodID: group[0].FoodID, Name: strings.Join(names, ", "), Date: date, SlotID: c.SlotID,
			CookOccasions: len(newIndex(next, t).occasions[dayOf(day)]), Op: ops[0], Ops: ops,
		}}
	}
	return nil
}
