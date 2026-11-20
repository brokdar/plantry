package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
)

// refitSides runs after a main was replaced: a side that does not fit the new
// main (the potatoes of the old dish beside a pasta) is swapped for the best
// fitting side, or left out when none fits, inside the same change. Each step
// is returned with its ops, so the result tells it and a way can list it.
func refitSides(ctx context.Context, svc Services, plateID int64, b *batch) []map[string]any {
	w, ok := windowFrom(ctx)
	if !ok || svc.State == nil {
		return nil
	}
	var out []map[string]any
	for range 3 {
		st, err := svc.State(ctx, w, todayFrom(ctx))
		if err != nil {
			return out
		}
		st.Avoid, st.PairAddOns = svc.Removed.avoid(ctx), true
		pi := indexPlate(st, plateID)
		if pi < 0 {
			return out
		}
		p := st.Plates[pi]
		bad := st.MisfitSides(p)
		if len(bad) == 0 {
			return out
		}
		_, ops, text := misfitFix(st, p, bad[0], plateID)
		changeOps, err := changeOpsOf(ops)
		if err != nil {
			return out
		}
		var shown []map[string]any
		for _, op := range changeOps {
			if _, err := applyOp(ctx, svc, op, b); err != nil {
				return out
			}
			shown = append(shown, opJSON(op))
		}
		out = append(out, map[string]any{"what": text, "ops": shown})
	}
	return out
}

// misfitFix is the step for the misfit side id of plate p: swap it for the best
// fitting side, or leave it out when none fits. The text says which, in the
// words of a finished step ("swapped A for B: it does not fit the new main").
func misfitFix(st planning.State, p planning.Plate, id, plateID int64) (to string, ops []planning.OptionOp, text string) {
	old := ""
	for _, c := range p.Components {
		if c.ID == id {
			old = food.Quote(st.Foods[c.FoodID].Name)
		}
	}
	ops = []planning.OptionOp{{Op: "remove", PlateComponentID: &id}}
	text = fmt.Sprintf("left out %s: it does not fit the new main", old)
	if res, err := planning.Alternatives(st, planning.WindowTarget(st), planning.AltRequest{PlateID: plateID, What: planning.AltSide, ComponentID: id, Limit: 8}); err == nil {
		for _, a := range res.Alternatives {
			if len(a.NewIssues) == 0 && len(a.Ops) > 0 && a.Role != string(food.RoleSideStarch) {
				ops, to = a.Ops, food.Quote(a.Name)
				text = fmt.Sprintf("swapped %s for %s: it does not fit the new main", old, to)
				break
			}
		}
	}
	return to, ops, text
}

// sideClash is what a user-named main swap leaves beside the new main: the
// first side that does not fit it, with the one fix on offer (nothing is
// changed). nil when every side fits.
func sideClash(ctx context.Context, svc Services, plateID int64) map[string]any {
	w, ok := windowFrom(ctx)
	if !ok || svc.State == nil {
		return nil
	}
	st, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return nil
	}
	st.Avoid, st.PairAddOns = svc.Removed.avoid(ctx), true
	pi := indexPlate(st, plateID)
	if pi < 0 {
		return nil
	}
	p := st.Plates[pi]
	bad := st.MisfitSides(p)
	if len(bad) == 0 {
		return nil
	}
	to, ops, _ := misfitFix(st, p, bad[0], plateID)
	name := ""
	for _, c := range p.Components {
		if c.ID == bad[0] {
			name = food.Quote(st.Foods[c.FoodID].Name)
		}
	}
	what := "leave out " + name
	if to != "" {
		what = fmt.Sprintf("swap %s for %s", name, to)
	}
	return map[string]any{
		"plate_id": plateID, "date": p.Date.Format(time.DateOnly), "side": name, "ops": ops,
		"clash": name + " stays on the plate and does not fit the new main (a pasta, rice or potato main takes one starch, not two)",
		"what":  what + " so the plate fits the new main",
	}
}

// refitOpsOf are the ops a write's refit steps applied (they are part of the
// change a way or an alternative promises); they go before the main's own op,
// so the plate has no misfit left for the write to fix on its own.
func refitOpsOf(applied []map[string]any) (ops []planning.OptionOp, what []string) {
	for _, m := range applied {
		steps, _ := m["refit"].([]map[string]any)
		for _, st := range steps {
			what = append(what, fmt.Sprint(st["what"]))
			shown, _ := st["ops"].([]map[string]any)
			for _, o := range shown {
				var op planning.OptionOp
				if raw, err := json.Marshal(o); err == nil && json.Unmarshal(raw, &op) == nil {
					ops = append(ops, op)
				}
			}
		}
	}
	return ops, what
}
