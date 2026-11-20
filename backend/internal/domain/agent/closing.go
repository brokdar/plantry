package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

// The closing option of a result: one worked alternative, checked on a copy of
// the plan, or none. The builders below are the only places that make one for
// a refused write or a day under its minimum, and next names an option only
// when it is there.

// refusalOffer is, for an op the guard left out, the nearest verified
// alternative that does what the user asked as far as the rules allow: a
// replacement for a removed add-on, another main or side for a refused
// replace, a smaller step for a refused quantity, a swap of the main dishes
// for a refused swap of two meals. Nil when none passes the check.
func refusalOffer(ctx context.Context, svc Services, w window.Window, op changeOp, first bool) map[string]any {
	switch op.Op {
	case "remove":
		return replacementOffer(ctx, svc, w, op, first)
	case "replace", "set_quantity", "move":
	default:
		return nil
	}
	if svc.State == nil {
		return nil
	}
	st, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return nil
	}
	st.Avoid, st.PairAddOns = svc.Removed.avoid(ctx), true
	var o map[string]any
	switch op.Op {
	case "replace":
		o = replaceAlternative(ctx, svc, w, st, op)
	case "set_quantity":
		o = smallerStep(ctx, svc, w, st, op)
	case "move":
		o = mainSwap(ctx, svc, w, st, op)
	}
	if o != nil {
		o["recommended"] = first
		o["recommended_because"] = "checked on a copy of the plan: no new problem, no new warning"
	}
	return o
}

// componentOf finds the plate and component an op names: by component id, or
// by plate and position (main by default).
func componentOf(st planning.State, op changeOp) (planning.Plate, planning.Component, bool) {
	for _, p := range st.Plates {
		if op.PlateComponentID == 0 && p.ID != op.PlateID {
			continue
		}
		for _, c := range p.Components {
			switch {
			case op.PlateComponentID != 0 && c.ID == op.PlateComponentID:
				return p, c, true
			case op.PlateComponentID == 0 && c.Position == plate.Position(cmpOr(op.Position, "main")):
				return p, c, true
			}
		}
	}
	return planning.Plate{}, planning.Component{}, false
}

func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// replaceAlternative is the best clean other dish for the place a refused
// replace named.
func replaceAlternative(ctx context.Context, svc Services, w window.Window, st planning.State, op changeOp) map[string]any {
	p, c, ok := componentOf(st, op)
	if !ok {
		return nil
	}
	req := planning.AltRequest{PlateID: p.ID, What: planning.AltMain, Limit: 6}
	if c.Position != plate.PositionMain {
		req.What, req.ComponentID = planning.AltSide, c.ID
	}
	res, err := planning.Alternatives(st, planning.WindowTarget(st), req)
	if err != nil || res.Current == nil {
		return nil
	}
	a, ok := firstClean(res.Alternatives, func(ops []planning.OptionOp) bool { return offerIsClean(ctx, svc, w, ops) })
	if !ok {
		return nil
	}
	return map[string]any{
		"what":   fmt.Sprintf("use %s instead of %s on %s: the dish you asked for cannot go there", food.Quote(a.Name), food.Quote(res.Current.Name), mealName(st, p)),
		"ops":    a.Ops,
		"effect": map[string]any{"day": dayLabel(p.Date), "day_kcal_after": a.DayKcal},
	}
}

// smallerStep is the biggest part of a refused quantity change that passes
// the check: halfway, then a quarter of the way, in quarter portions.
func smallerStep(ctx context.Context, svc Services, w window.Window, st planning.State, op changeOp) map[string]any {
	p, c, ok := componentOf(st, op)
	if !ok || op.PlateComponentID == 0 || c.Portions == nil || op.Portions == nil {
		return nil
	}
	cur, req := *c.Portions, *op.Portions
	for _, f := range []float64{0.5, 0.25} {
		q := math.Round((cur+(req-cur)*f)*4) / 4
		if q == cur || q == req || q <= 0 {
			continue
		}
		id := c.ID
		ops := []planning.OptionOp{{Op: "set_quantity", PlateComponentID: &id, Portions: &q}}
		if offerIsClean(ctx, svc, w, ops) {
			return map[string]any{
				"what": fmt.Sprintf("set %s on %s to %s portions instead of %s: the whole step cannot go through", food.Quote(foodNameOf(st, c.FoodID)), mealName(st, p), fmtNum(q), fmtNum(req)),
				"ops":  ops,
			}
		}
	}
	return nil
}

func foodNameOf(st planning.State, id int64) string { return st.Foods[id].Name }

// mainSwap is, for a refused swap of two meals (the cooking and the leftovers
// that follow a moved plate broke a rule), the swap of their main dishes with
// the sides where they are, when that passes the check.
func mainSwap(ctx context.Context, svc Services, w window.Window, st planning.State, op changeOp) map[string]any {
	if string(op.Occupant) != `"swap"` {
		return nil
	}
	var to struct {
		Date   string `json:"date"`
		SlotID int64  `json:"slot_id"`
	}
	if json.Unmarshal(op.To, &to) != nil {
		return nil
	}
	ai := indexPlate(st, op.PlateID)
	bi := slices.IndexFunc(st.Plates, func(p planning.Plate) bool {
		return p.Date.Format("2006-01-02") == to.Date && p.SlotID == to.SlotID
	})
	if ai < 0 || bi < 0 {
		return nil
	}
	a, b := st.Plates[ai], st.Plates[bi]
	mainOf := func(p planning.Plate) (planning.Component, bool) {
		for _, c := range p.Components {
			if c.Position == plate.PositionMain && c.ID != 0 {
				return c, true
			}
		}
		return planning.Component{}, false
	}
	ca, ok1 := mainOf(a)
	cb, ok2 := mainOf(b)
	if !ok1 || !ok2 || ca.FoodID == cb.FoodID {
		return nil
	}
	ida, idb := ca.ID, cb.ID
	ops := []planning.OptionOp{
		{Op: "replace", PlateComponentID: &ida, FoodID: cb.FoodID},
		{Op: "replace", PlateComponentID: &idb, FoodID: ca.FoodID},
	}
	if !offerIsClean(ctx, svc, w, ops) {
		return nil
	}
	return map[string]any{
		"what": fmt.Sprintf("swap the main dishes of %s and %s (%s and %s) and leave the sides where they are: moving the whole meals cannot go through", mealName(st, a), mealName(st, b), food.Quote(foodNameOf(st, ca.FoodID)), food.Quote(foodNameOf(st, cb.FoodID))),
		"ops":  ops,
	}
}

// floorFixOffer is the verified fix for a day a saved change left under its
// minimum: the first add that reaches it, else the adds that reach it together
// (at most three, on different meals), each from the issue's own options.
// Nil when no such fix passes the check: the reply then says so.
func floorFixOffer(ctx context.Context, svc Services, w window.Window, is planning.Issue, all []planning.Issue) map[string]any {
	kcal, floor := paramNum(is, "kcal"), paramNum(is, "floor_kcal")
	// The options of the day's own issue and of its energy band issue (a day
	// under its minimum is under its band too, and that issue may be older).
	opts := slices.Clone(is.Options)
	for _, x := range all {
		if x.Date == is.Date && x.Code == planning.CodeKcalBelowBand {
			opts = append(opts, x.Options...)
		}
	}
	if kcal <= 0 || floor <= 0 || len(opts) == 0 {
		return nil
	}
	slices.SortStableFunc(opts, func(a, b planning.Option) int {
		switch {
		case a.KcalDelta > b.KcalDelta:
			return -1
		case a.KcalDelta < b.KcalDelta:
			return 1
		}
		return 0
	})
	opOf := func(o planning.Option) []planning.OptionOp {
		if len(o.Ops) > 0 {
			return o.Ops
		}
		return []planning.OptionOp{o.Op}
	}
	var ops []planning.OptionOp
	var names []string
	var used []string
	gain := 0.0
	for _, o := range opts {
		key := fmt.Sprintf("%s|%d", o.Date, o.SlotID)
		if o.KcalDelta <= 0 || len(opOf(o)) == 0 || slices.Contains(used, key) {
			continue
		}
		used = append(used, key)
		ops = append(ops, opOf(o)...)
		names = append(names, optionWords(o))
		if gain += o.KcalDelta; kcal+gain >= floor || len(names) == 3 {
			break
		}
	}
	if kcal+gain < floor || !offerIsClean(ctx, svc, w, ops) {
		return nil
	}
	return map[string]any{
		"what":                strings.Join(names, " and ") + fmt.Sprintf(" so %s reaches its minimum of %.0f kcal", dayLabelStr(is.Date), floor),
		"ops":                 ops,
		"recommended":         true,
		"recommended_because": "brings the day above its minimum, no new warning",
		"effect":              map[string]any{"day": dayLabelStr(is.Date), "day_kcal_after": math.Round(kcal + gain)},
	}
}

// optionWords says an add or raise option in kitchen words: "add 120 g of
// "Ciabatta" to Tue 24 Nov" or "raise "Curry" to 1.5 portions on Tue 24 Nov".
func optionWords(o planning.Option) string {
	amount := ""
	switch {
	case o.Grams != nil:
		amount = fmt.Sprintf("%.0f g", math.Round(*o.Grams))
	case o.Portions != nil:
		amount = fmtNum(*o.Portions) + " portions"
	}
	where := strings.TrimSpace(dayLabelStr(o.Date) + " " + o.Slot)
	if o.Action == "raise" {
		return fmt.Sprintf("raise %s to %s on %s", food.Quote(o.Name), amount, where)
	}
	return fmt.Sprintf("add %s of %s to %s", amount, food.Quote(o.Name), where)
}

// unLightOffer is RL9: when no unmarked planned meal of a day left under its
// minimum can take a fix, the one offer removes the light mark of the marked
// lunch or dinner that has the most room, applied only on a yes (the offer
// carries the mark's removal and nothing else). nil when the day has no
// marked meal: the reply then says plainly that nothing can be offered.
func unLightOffer(ctx context.Context, svc Services, w window.Window, is planning.Issue) map[string]any {
	if svc.State == nil {
		return nil
	}
	st, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return nil
	}
	var best *planning.Plate
	var mark planning.LightIntent
	bestKcal := math.Inf(1)
	ev := planning.Evaluate(st, planning.WindowTarget(st))
	kcal := map[int64]float64{}
	for _, pe := range ev.Plates {
		kcal[pe.PlateID] = pe.Kcal
	}
	for i, p := range st.Plates {
		if p.Date.Format(time.DateOnly) != is.Date {
			continue
		}
		light := st.LightMealAt(p.Date, p.SlotID)
		if !light || kcal[p.ID] >= bestKcal {
			continue
		}
		best, bestKcal = &st.Plates[i], kcal[p.ID]
		for _, n := range []string{"lunch", "dinner"} {
			if strings.TrimPrefix(st.SlotNames[p.SlotID], "slot.") == n {
				mark = planning.LightIntent{Date: is.Date, Scope: planning.LightScopeNone, Slot: n}
			}
		}
	}
	if best == nil || mark.Slot == "" {
		return nil
	}
	meal := mealName(st, *best)
	return map[string]any{
		"what":                fmt.Sprintf("remove the light mark of %s, so it can take the food the day is missing; I add it only after you say yes", meal),
		"light":               []planning.LightIntent{mark},
		"recommended":         true,
		"recommended_because": "no other planned meal of the day can take more food while it is marked light",
		"effect":              map[string]any{"day": dayLabelStr(is.Date), "day_kcal_now": math.Round(paramNum(is, "kcal")), "day_minimum": math.Round(paramNum(is, "floor_kcal"))},
	}
}
