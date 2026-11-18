package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

const findWaysName = "find_ways"

const findWaysSchema = `{
  "type":"object",
  "required":["goal"],
  "properties":{
    "goal":{"type":"string","enum":["lighter","heartier","more_protein"],"description":"lighter|heartier: one lunch or dinner plate (plate_id); more_protein: one plate (plate_id) or the whole day (date)"},
    "plate_id":{"type":"integer","minimum":1},
    "date":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$","description":"more_protein for the day"},
    "limit":{"type":"integer","minimum":1,"maximum":4}
  },
  "additionalProperties":false
}`

const findWaysDescription = `Find verified ways to meet a relative request ("make dinner lighter", "heartier", "more protein") in one call. goal: lighter (the meal must come under its light cap), heartier (the meal must reach its hearty minimum) or more_protein (a plate, or the day with date; at least 10 g more). For lighter and heartier the food must change in the asked direction by a meaningful amount (at least 10% of the meal or 80 kcal, the smaller of the two): a meal that already meets its cap or minimum still has to change. A way is a smaller or larger portion, leaving out or adding a side or add-on, a lighter or heartier main, or a compound way that also changes the day's other meal (adds food there when lightening, takes some away when making heartier) so the day stays above its floor and within its band; a compound way may drop the light mark of that other meal (its light entry says so). Every way was applied to a copy of the plan and checked like apply_changes before it is listed: no new hard issue, the day above its floor, in the band where possible. Ranked: reaches the goal, no new soft issue, least change, the day closest to its target. Saves nothing.
Returns compact JSON (a field that is 0, false or empty is left out) {goal:{kind, plate_id?, date?} (pass it as it is to apply_changes), max_kcal? (light cap), min_kcal? (hearty minimum), before:{meal_kcal, meal_protein_g, day_kcal, day_protein_g} (per eater), floor_kcal, band_low, band_high (the day), ways:[{kind portion|remove|add|swap|compound, what (the change in words), moved? (a compound way: what changes on the other meal), reaches (false: the closest way, why says how far it gets and what stops it), why?, ops (apply_changes ops), light? (the marks to pass to apply_changes with the ops), before and after:{meal_kcal, meal_protein_g, day_kcal, day_protein_g}, new_issues?}], empty_reason?}. To take a way, call apply_changes with its ops, its light and the goal exactly as returned; the result's request says whether the request is satisfied.`

func toolFindWays(svc Services) Tool {
	return Tool{
		Name:        findWaysName,
		Description: findWaysDescription,
		Schema:      json.RawMessage(findWaysSchema),
		Handler: func(ctx context.Context, input json.RawMessage) (json.RawMessage, ToolEffect, error) {
			var in struct {
				Goal    string `json:"goal"`
				PlateID int64  `json:"plate_id"`
				Date    string `json:"date"`
				Limit   int    `json:"limit"`
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
			}
			if svc.State == nil || svc.Tx == nil {
				return nil, ToolEffectNone, fmt.Errorf("%w: find_ways is not available (no plan reader)", domain.ErrInvalidInput)
			}
			w, ok := windowFrom(ctx)
			if !ok {
				return nil, ToolEffectNone, fmt.Errorf("%w: no plan window in this conversation turn", domain.ErrInvalidInput)
			}
			st, err := svc.State(ctx, w, todayFrom(ctx))
			if err != nil {
				return nil, ToolEffectNone, fmt.Errorf("read the plan: %w", err)
			}
			st.Light = planning.MergeLight(st.Light, lightFrom(ctx))
			res, err := planning.Ways(st, planning.WindowTarget(st), planning.WayRequest{PlateID: in.PlateID, Date: in.Date, Goal: in.Goal, Limit: 8})
			if err != nil {
				return nil, ToolEffectNone, err
			}
			goal := applyGoal{Kind: in.Goal, PlateID: in.PlateID, Date: in.Date}
			if in.PlateID != 0 {
				goal.Date = ""
			}
			limit := in.Limit
			if limit <= 0 {
				limit = 4
			}
			var ways []planning.Way
			var dropped []string
			for _, way := range res.Ways {
				checked, why := checkWay(ctx, svc, w, goal, res.Date, way)
				if why != "" {
					dropped = append(dropped, why)
					continue
				}
				ways = append(ways, checked)
				if len(ways) == limit {
					break
				}
			}
			out := map[string]any{"goal": goal, "before": res.Before, "ways": nonNil(ways)}
			for k, v := range map[string]float64{"max_kcal": res.MaxKcal, "min_kcal": res.MinKcal, "floor_kcal": res.FloorKcal, "band_low": res.BandLow, "band_high": res.BandHigh} {
				if v > 0 {
					out[k] = v
				}
			}
			switch {
			case len(ways) > 0:
			case res.EmptyWhy != "":
				out["empty_reason"] = res.EmptyWhy
			case len(dropped) > 0:
				out["empty_reason"] = "every way failed the check against the real plan: " + strings.Join(dropped, "; ")
			default:
				out["empty_reason"] = "no verified way found"
			}
			return mustJSON(out), ToolEffectNone, nil
		},
	}
}

func nonNil(w []planning.Way) []planning.Way {
	if w == nil {
		return []planning.Way{}
	}
	return w
}

// checkWay runs a way through the check apply_changes uses (its ops, light
// and goal, rolled back) and returns it with the figures that check read; a
// non-empty reason means the way is not offered: the call failed or it adds a
// hard issue.
func checkWay(ctx context.Context, svc Services, w window.Window, goal applyGoal, date string, way planning.Way) (planning.Way, string) {
	ops := make([]changeOp, 0, len(way.Ops))
	for _, o := range way.Ops {
		raw, err := json.Marshal(o)
		if err != nil {
			return way, err.Error()
		}
		var op changeOp
		if err := json.Unmarshal(raw, &op); err != nil {
			return way, err.Error()
		}
		ops = append(ops, op)
	}
	light, err := parseLight(way.Light)
	if err != nil {
		return way, err.Error()
	}
	run, err := runApply(withLight(ctx, light), svc, w, applyInput{Ops: ops, DryRun: true, Light: light, Goal: &goal, quiet: true}, nil)
	if err != nil {
		return way, way.What + ": " + err.Error()
	}
	was := issueMap(run.before.Issues)
	for _, is := range run.after.Issues {
		if _, old := was[issueKey(is)]; !old && is.Severity == planning.SeverityViolation {
			return way, fmt.Sprintf("%s: it adds a hard issue (%s)", way.What, planning.PlainText(is.Code).Label)
		}
	}
	// The figures are the ones the write would show.
	way.Before = planning.FiguresOf(run.before, way.PlateID, date)
	way.After = planning.FiguresOf(run.after, way.PlateID, date)
	if req, ok := run.result["request"].(map[string]any); ok {
		sat, _ := req["satisfied"].(bool)
		if way.Reaches && !sat {
			way.Reaches, way.Why = false, fmt.Sprint(req["why"])
		}
	}
	return way, ""
}

// checkMarks refuses a light or hearty mark on a meal that does not meet its
// cap (minimum) after the change: the mark is derived from the result.
func checkMarks(st planning.State, light []planning.LightIntent, ev planning.Evaluation) error {
	for _, l := range light {
		if l.Scope != planning.LightScopeMeal {
			continue
		}
		for _, is := range ev.Issues {
			if is.SlotID == nil || is.Date != l.Date || (is.Code != planning.CodeMealNotLight && is.Code != planning.CodeMealNotHearty) {
				continue
			}
			if strings.TrimPrefix(st.SlotNames[*is.SlotID], "slot.") != l.Slot {
				continue
			}
			kcal, _ := is.Params["kcal"].(float64)
			if is.Code == planning.CodeMealNotLight {
				limit, _ := is.Params["max_kcal"].(float64)
				return fmt.Errorf("%w: %s on %s would hold %.0f kcal, over its light cap of %.0f kcal: a light mark is stored only on a meal that meets its cap. Use find_ways with goal lighter to get verified ways to the cap, and pass the mark with them", domain.ErrInvalidInput, l.Slot, l.Date, kcal, limit)
			}
			least, _ := is.Params["min_kcal"].(float64)
			return fmt.Errorf("%w: %s on %s would hold %.0f kcal, under its hearty minimum of %.0f kcal: a hearty mark is stored only on a meal that meets its minimum. Use find_ways with goal heartier to get verified ways to the minimum, and pass the mark with them", domain.ErrInvalidInput, l.Slot, l.Date, kcal, least)
		}
	}
	return nil
}

// judgeGoal builds the request block of an apply_changes call with a goal:
// the figures before and after for the target meal and its day, whether the
// food changed in the asked direction (and met the cap or minimum), and why
// not. A light or hearty mark stored on the target for a request that changed
// no food is refused.
func judgeGoal(ctx context.Context, svc Services, w window.Window, g applyGoal, light []planning.LightIntent, before, after planning.Evaluation) (map[string]any, error) {
	switch g.Kind {
	case planning.GoalLighter, planning.GoalHeartier, planning.GoalMoreProtein:
	default:
		return nil, fmt.Errorf("%w: goal.kind must be lighter, heartier or more_protein", domain.ErrInvalidInput)
	}
	st, err := svc.State(ctx, w, todayFrom(ctx))
	if err != nil {
		return nil, err
	}
	st.Light = planning.MergeLight(st.Light, light)
	date := g.Date
	if g.PlateID != 0 {
		i := indexPlate(st, g.PlateID)
		if i < 0 {
			return nil, fmt.Errorf("%w: goal plate %d is not in the plan after the change", domain.ErrInvalidInput, g.PlateID)
		}
		date = st.Plates[i].Date.Format(time.DateOnly)
	} else if g.Kind != planning.GoalMoreProtein || date == "" {
		return nil, fmt.Errorf("%w: goal %s needs plate_id (more_protein also takes date)", domain.ErrInvalidInput, g.Kind)
	}
	var maxKcal, minKcal float64
	if g.Kind != planning.GoalMoreProtein {
		if maxKcal, minKcal, err = planning.MealLimit(st, planning.WindowTarget(st), g.PlateID, g.Kind); err != nil {
			return nil, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
		}
	}
	b, a := planning.FiguresOf(before, g.PlateID, date), planning.FiguresOf(after, g.PlateID, date)
	changed, satisfied, why := planning.JudgeRelative(g.Kind, g.PlateID != 0, maxKcal, minKcal, b, a)
	if !changed && g.Kind != planning.GoalMoreProtein {
		for _, l := range light {
			if l.Scope == planning.LightScopeMeal && l.Date == date && l.Hearty() == (g.Kind == planning.GoalHeartier) {
				return nil, fmt.Errorf("%w: %s: a mark alone does not make a meal %s. Take the ops of a way from find_ways with goal %s", domain.ErrInvalidInput, why, g.Kind, g.Kind)
			}
		}
	}
	req := map[string]any{"goal": g.Kind, "before": b, "after": a, "satisfied": satisfied}
	if g.PlateID == 0 {
		// A day request: the figures are the day's.
		req["before"], req["after"] = map[string]any{"day_kcal": b.DayKcal, "day_protein_g": b.DayProteinG}, map[string]any{"day_kcal": a.DayKcal, "day_protein_g": a.DayProteinG}
	}
	target := map[string]any{"date": date}
	if g.PlateID != 0 {
		target["plate_id"] = g.PlateID
	}
	req["target"] = target
	if maxKcal > 0 {
		req["max_kcal"] = maxKcal
	}
	if minKcal > 0 {
		req["min_kcal"] = minKcal
	}
	if why != "" {
		req["why"] = why
	}
	return req, nil
}

func indexPlate(st planning.State, id int64) int {
	for i, p := range st.Plates {
		if p.ID == id {
			return i
		}
	}
	return -1
}
