package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/food"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
)

const findWaysName = "find_ways"

const findWaysSchema = `{
  "type":"object",
  "required":["goal"],
  "properties":{
    "goal":{"type":"string","enum":["lighter","heartier","more_protein","lower_day","raise_day"],"description":"lighter|heartier: one lunch or dinner plate (plate_id); more_protein: one plate (plate_id) or the whole day (date); lower_day|raise_day: the whole day (date), an explicit eat less (more) that day"},
    "plate_id":{"type":"integer","minimum":1},
    "date":{"type":"string","pattern":"^[0-9]{4}-[0-9]{2}-[0-9]{2}$","description":"more_protein, lower_day, raise_day: the day"},
    "limit":{"type":"integer","minimum":1,"maximum":4},
    "target_kcal":{"type":"number","minimum":1,"description":"lower_day: the kcal the user named for the day, when they named one"},
    "to_limit":{"type":"boolean","description":"true when the user asked for a LIGHT (filling) meal itself (a light dinner, properly light, leicht), not just a lighter (heartier) one: only a way at the light limit meets it, so the search also swaps the main for a lighter dish"}
  },
  "additionalProperties":false
}`

const findWaysDescription = `Find verified ways to meet a relative request ("make dinner lighter", "heartier", "more protein") in one call. Set to_limit true when the user asked for a LIGHT (filling) meal itself ("a light dinner", "properly light", "leicht"), not just a lighter one: then only a way at the light limit meets the request, offer[0] is one when no way is, and next says what to do. goal: lighter, heartier (one lunch or dinner plate), more_protein (a plate, or the day with date; at least 10 g more), lower_day or raise_day (the day with date: an explicit "eat less (more) on Monday"; the day is marked light (hearty) and the ways bring it into the adjusted range). The food must change in the asked direction by a meaningful amount (at least 10% of the meal or 80 kcal, the smaller): a meal that already meets its cap or minimum still has to change. ways change ONLY the named meal (a smaller or larger portion, leaving out or adding a side or add-on, a lighter or heartier main), the day stays above its floor (heartier: in its band); the first one goes as far as that allows, so reaches_cap false means lighter (heartier) but not yet a light (hearty) meal, why says by how much. offer holds at most two compound ways that also change another meal of the day (a bigger portion of its main and a matching side or bread, or a smaller one) so the named meal reaches its cap or minimum, and may end that meal's light mark: never apply an offer unasked, present it as a choice. Every way was applied to a copy and checked like apply_changes before it is listed. RECOMMENDATION: exactly one way or offer is marked recommended with recommended_because: present it as your recommendation in one phrase. Nothing here adds a food the user removed, a cheese, or a protein that does not fit the dish. RULE: a request that names a meal or a day and a direction is carried out in the same turn with the first way (the result says what to do in next), and the offer, if any, is mentioned as a choice; a request that names neither ("make the plan healthier") gets one question first. A way or offer whose day still ends under its range has fix {what, ops, day_kcal_after}: one small addition of normal food (never cheese or nuts) that puts the day inside its range, checked on a copy of the plan with the way's ops plus the fix's; say the shortfall plainly and offer the fix as your one recommendation, apply its ops only when the user says yes. Saves nothing.
Every way and offer also has meal_vs_limit and day_vs_range (ready phrases: use them, never subtract). Returns JSON {next (what to do now), impossible? (the number the user named cannot be met: ways is empty, offer[0] is the nearest safe option), goal:{kind, plate_id?, date?} (pass it as it is to apply_changes), max_kcal? (the most a light meal holds), min_kcal? (the least a filling meal holds), before:{meal_kcal, meal_protein_g, day_kcal, day_protein_g} (per eater), floor_kcal, band_low, band_high (the day), ways:[{kind portion|remove|add|swap, what (the change in words), reaches_cap (more_protein: always true; lower_day and raise_day: the day is in the adjusted range), why?, ops (apply_changes ops), light? (marks to pass with the ops), before and after:{meal_kcal, meal_protein_g, day_kcal, day_protein_g}, new_issues?}], offer?:[the same with changes_other_meal true, kind compound and moved (what changes on the other meal)], empty_reason? (no way changes only this meal, with the numbers)}. To take a way or an offer, call apply_changes with its ops, its light and the goal exactly as returned; the result's request says whether it is satisfied.`

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
				// TargetKcal is the number the user named for a day.
				TargetKcal float64 `json:"target_kcal"`
				ToLimit    bool    `json:"to_limit"`
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
			st.Avoid, st.PairAddOns = svc.Removed.avoid(ctx), true
			res, err := planning.Ways(st, planning.WindowTarget(st), planning.WayRequest{PlateID: in.PlateID, Date: in.Date, Goal: in.Goal, Limit: 8, TargetKcal: in.TargetKcal, ToLimit: in.ToLimit})
			if err != nil {
				return nil, ToolEffectNone, err
			}
			goal := applyGoal{Kind: in.Goal, PlateID: in.PlateID, Date: in.Date}
			if in.PlateID != 0 {
				goal.Date = ""
			}
			if in.Goal == planning.GoalLowerDay || in.Goal == planning.GoalRaiseDay {
				goal = applyGoal{Kind: in.Goal, Date: res.Date}
			}
			limit := in.Limit
			if limit <= 0 {
				limit = 4
			}
			var ways, offers []planning.Way
			var dropped []string
			verify := func(in []planning.Way, max int) []planning.Way {
				var out []planning.Way
				for _, way := range in {
					checked, why := checkWay(ctx, svc, w, goal, res.Date, way)
					if why != "" {
						dropped = append(dropped, why)
						continue
					}
					if out = append(out, checked); len(out) == max {
						break
					}
				}
				return out
			}
			ways, offers = planning.DemoteUnclean(verify(res.Ways, limit)), planning.DemoteUnclean(verify(res.Offers, 2))
			if in.Goal == planning.GoalLighter && !in.ToLimit && res.Before.MealKcal > res.MaxKcal && (len(ways) == 0 || !ways[0].Reaches) {
				// The step stops short of light, or the meal cannot lighten alone:
				// the one thing offered is the compound way that makes the meal
				// properly light, never a mid-way one.
				closest := offers
				offers = nil
				if full, err := planning.Ways(st, planning.WindowTarget(st), planning.WayRequest{PlateID: in.PlateID, Date: in.Date, Goal: in.Goal, Limit: 8, ToLimit: true}); err == nil {
					for _, o := range planning.DemoteUnclean(verify(full.Offers, 2)) {
						if o.Reaches {
							offers = append(offers, o)
						}
					}
				}
				offers = orClosest(offers, closest, res.MaxKcal)
			}
			if res.Impossible && len(offers) == 0 {
				// The real write refits what a main swap leaves beside it and can
				// take the day under its minimum: the best allowed step is then
				// searched without changing a main.
				if kept, err := planning.Ways(st, planning.WindowTarget(st), planning.WayRequest{Date: in.Date, Goal: in.Goal, Limit: 8, TargetKcal: in.TargetKcal, KeepMains: true}); err == nil && kept.Impossible {
					offers = planning.DemoteUnclean(verify(kept.Offers, 2))
				}
			}
			recommend(ways, offers, in.ToLimit)
			nameWays(st, ways)
			nameWays(st, offers)
			if res.Impossible {
				for i := range offers {
					if offers[i].PlateID == 0 {
						offers[i].What = fmt.Sprintf("take %s down to %.0f kcal, the lowest it goes inside its range: %s", dayLabelStr(res.Date), offers[i].After.DayKcal, offers[i].What)
					}
				}
			}
			out := map[string]any{"next": nextStep(ways, offers, res, in.ToLimit), "goal": goal, "before": res.Before, "ways": nonNil(ways)}
			if len(offers) > 0 {
				out["offer"] = offers
			}
			for k, v := range map[string]float64{"max_kcal": res.MaxKcal, "min_kcal": res.MinKcal, "floor_kcal": res.FloorKcal, "band_low": res.BandLow, "band_high": res.BandHigh} {
				if v > 0 {
					out[k] = v
				}
			}
			switch {
			case len(ways) > 0:
			case res.EmptyWhy != "":
				out["empty_reason"] = res.EmptyWhy
				if len(dropped) > 0 && len(offers) == 0 {
					out["empty_reason"] = res.EmptyWhy + " Checked against the real plan and dropped: " + strings.Join(dropped, "; ")
				}
			case len(dropped) > 0:
				out["empty_reason"] = "every way failed the check against the real plan: " + strings.Join(dropped, "; ")
			default:
				out["empty_reason"] = "no verified way found"
			}
			return mustJSON(out), ToolEffectNone, nil
		},
	}
}

// orClosest is the offers of a lighter request: the ones that reach the light
// limit, else (none does within the kitchen rules) the closest verified
// compound way, labelled as what it is.
func orClosest(reaching, closest []planning.Way, maxKcal float64) []planning.Way {
	if len(reaching) > 0 {
		return reaching
	}
	for i := range closest {
		closest[i].Why = closestWhy(closest[i], maxKcal)
	}
	return closest
}

// closestWhy labels an offer that stops short of the light limit: the meal's
// kcal and that it is not a light meal, ahead of any note the way carried.
func closestWhy(o planning.Way, maxKcal float64) string {
	why := fmt.Sprintf("the closest it gets: the meal holds %.0f kcal, not a light meal (the limit is %.0f kcal)", o.After.MealKcal, maxKcal)
	if o.Why != "" {
		why += "; " + o.Why
	}
	return why
}

// nameWays says which meal and day a way is about ("Tue 24 Nov dinner: ...")
// and names the day on the other meals it moves, so the model never has to
// guess which meal a figure belongs to.
func nameWays(st planning.State, ways []planning.Way) {
	for i := range ways {
		if f := ways[i].Fix; f != nil {
			f.What = fixWords(st, f)
			// A fix closes the gap: the way's note no longer says "the closest".
			ways[i].Why = strings.TrimSuffix(strings.Replace(ways[i].Why, ": the closest the other meal can get it", "", 1), "; ")
		}
		p := slices.IndexFunc(st.Plates, func(q planning.Plate) bool { return q.ID == ways[i].PlateID })
		if p < 0 {
			continue
		}
		pl := st.Plates[p]
		slot := strings.TrimPrefix(st.SlotNames[pl.SlotID], "slot.")
		day := dayLabel(pl.Date)
		ways[i].What = day + " " + slot + ": " + ways[i].What
		moved := slices.Clone(ways[i].Moved)
		for j, m := range moved {
			if k := strings.Index(m, ": "); k > 0 {
				moved[j] = day + " " + m
			}
		}
		ways[i].Moved = moved
	}
}

// fixWords says a day's fix in kitchen words: "add 120 g of Ciabatta to Tue 24
// Nov lunch" or "raise Curry on Tue 24 Nov lunch to 1.5 portions".
func fixWords(st planning.State, f *planning.WayFix) string {
	where := dayLabelStr(f.Date)
	if n := strings.TrimPrefix(st.SlotNames[f.SlotID], "slot."); n != "" {
		where += " " + n
	}
	amount := ""
	switch {
	case f.Grams > 0:
		amount = fmt.Sprintf("%.0f g", math.Round(f.Grams))
	case f.Portions > 0:
		amount = fmtNum(f.Portions) + " portions"
	}
	if f.Action == "raise" {
		return fmt.Sprintf("raise %s on %s to %s", f.Name, where, amount)
	}
	return fmt.Sprintf("add %s of %s to %s", amount, f.Name, where)
}

// fixNote is the sentence that tells what to do about a day the applied way
// leaves under its range: say it plainly and offer the one verified fix, never
// apply it unasked; "" when the day is in range or no fix exists.
func fixNote(w planning.Way) string {
	if w.Fix == nil {
		return ""
	}
	return fmt.Sprintf(". The day still ends under its range: say so plainly, and offer ONE small fix from normal food as your recommendation: %s (the day would then hold %.0f kcal, inside its range); apply the fix's ops (fix.ops) only when the user says yes", w.Fix.What, w.Fix.DayKcal)
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
	ops, err := changeOpsOf(way.Ops)
	if err != nil {
		return way, err.Error()
	}
	light, err := parseLight(way.Light)
	if err != nil {
		return way, err.Error()
	}
	run, err := runApply(context.WithValue(withLight(ctx, light), fixDateKey{}, date), svc, w, applyInput{Ops: ops, DryRun: true, Light: light, Goal: &goal, quiet: true}, nil)
	if err != nil {
		return way, way.What + ": " + err.Error()
	}
	was := issueMap(run.before.Issues)
	var fresh []planning.Issue
	for _, is := range run.after.Issues {
		if _, old := was[issueKey(is)]; old {
			continue
		}
		if is.Severity == planning.SeverityViolation {
			return way, fmt.Sprintf("%s: it adds a problem (%s)", way.What, planning.PlainText(is.Code).Label)
		}
		is.Options = nil
		fresh = append(fresh, is)
	}
	// A weekly food limit (meat, fish, legumes), a dish repeat or an add-on rule
	// the write makes worse is a warning too.
	way.Worsened = nil
	if iss, ok := run.result["after"].(map[string]any)["issues"].(map[string]any); ok {
		if l, ok := iss["worsened"].([]any); ok {
			for _, x := range l {
				m, _ := x.(map[string]any)
				if rule, _ := m["rule"].(string); rule == "S7" || rule == "S8" || rule == "S9" {
					code, _ := m["code"].(string)
					if lbl := planning.PlainText(code).Label; !slices.Contains(way.Worsened, lbl) {
						way.Worsened = append(way.Worsened, lbl)
					}
				}
			}
		}
	}
	// What the write itself reports as new is the truth about warnings.
	way.NewIssues, way.Why = fresh, strings.Join(slicesDeleteFlag(way.Why), "")
	// A side that does not fit the new main goes in the same change: the way
	// lists that step too (first, so the write has nothing left to fix).
	if applied, ok := run.result["applied"].([]map[string]any); ok {
		if extra, said := refitOpsOf(applied); len(extra) > 0 {
			way.Ops = append(extra, way.Ops...)
			way.What += "; " + strings.Join(said, "; ")
		}
	}
	// The figures are the ones the write would show.
	way.Before = planning.FiguresOf(run.before, way.PlateID, date)
	way.After = planning.FiguresOf(run.after, way.PlateID, date)
	for _, d := range run.after.Days {
		if d.Date == date {
			way.RangeLow, way.RangeHigh = d.BandLow, d.BandHigh
			// The planner wrote its own sentence about the day's range; the write's
			// is the truth (ending a light mark moves the range).
			if strings.HasPrefix(way.Why, "the day then holds") {
				note := ""
				if _, rest, ok := strings.Cut(way.Why, "; "); ok {
					note = rest
				}
				way.Why = note
				if d.Kcal < d.BandLow-0.5 {
					sentence := fmt.Sprintf("the day then holds %.0f kcal, %.0f under its range (%.0f to %.0f kcal): the closest the other meal can get it", d.Kcal, d.BandLow-d.Kcal, d.BandLow, d.BandHigh)
					way.Why = strings.TrimSuffix(sentence+"; "+note, "; ")
				}
			}
		}
	}
	if req, ok := run.result["request"].(map[string]any); ok {
		sat, _ := req["satisfied"].(bool)
		reached := sat
		for _, k := range []string{"reached_light", "reached_hearty", "reached_band"} {
			if v, ok := req[k].(bool); ok {
				reached = sat && v
			}
		}
		if !sat {
			return way, way.What + ": " + fmt.Sprint(req["why"])
		}
		if way.Reaches && !reached {
			way.Reaches, way.Why = false, fmt.Sprint(req["why"])
		}
		if !way.Reaches && way.Why == "" && !reached {
			way.Why = fmt.Sprint(req["why"])
		}
	}
	way.Fix = findFix(ctx, svc, w, goal, light, date, way, run)
	return way, ""
}

// findFix is the one small fix for a day the way leaves under its range: the
// first option of the day's energy issue (checked in the way's own dry run)
// that adds or enlarges a normal food on another meal, is no cheese or nut and
// puts the day inside its range, confirmed by a dry run of the way's ops plus
// the fix's. nil when the day is in range or no option holds.
func findFix(ctx context.Context, svc Services, w window.Window, goal applyGoal, light []planning.LightIntent, date string, way planning.Way, run applyRun) *planning.WayFix {
	var day *planning.DayEval
	for i := range run.after.Days {
		if run.after.Days[i].Date == date {
			day = &run.after.Days[i]
		}
	}
	if day == nil || day.BandLow == 0 || day.Kcal >= day.BandLow-0.5 {
		return nil
	}
	for _, is := range run.after.Issues {
		if is.Date != date || is.Code != planning.CodeKcalBelowBand {
			continue
		}
		for _, o := range fixCandidates(is.Options, day.Kcal, day.BandLow, day.BandHigh) {
			if o.PlateID != nil && *o.PlateID == way.PlateID {
				continue
			}
			fixOps := o.Ops
			if len(fixOps) == 0 {
				fixOps = []planning.OptionOp{o.Op}
			}
			ops, err := changeOpsOf(append(slices.Clone(way.Ops), fixOps...))
			if err != nil {
				continue
			}
			both, err := runApply(withLight(ctx, light), svc, w, applyInput{Ops: ops, DryRun: true, Light: light, Goal: &goal, quiet: true}, nil)
			if err != nil {
				continue
			}
			was := issueMap(both.before.Issues)
			clean := true
			for _, x := range both.after.Issues {
				if _, old := was[issueKey(x)]; !old && x.Severity == planning.SeverityViolation {
					clean = false
				}
			}
			for _, d := range both.after.Days {
				if clean && d.Date == date && d.Kcal >= d.BandLow-0.5 && d.Kcal <= d.BandHigh+0.5 {
					fix := &planning.WayFix{Action: o.Action, Name: o.Name, Date: o.Date, SlotID: o.SlotID, DayKcal: math.Round(d.Kcal), Ops: fixOps}
					if o.Grams != nil {
						fix.Grams = *o.Grams
					}
					if o.Portions != nil {
						fix.Portions = *o.Portions
					}
					return fix
				}
			}
		}
	}
	return nil
}

// isDayFix reports whether an option is a fix from normal food for a day under
// its range: an add or raise, no cheese or nut, that lands the day inside the
// range.
func isDayFix(o planning.Option, low, high float64) bool {
	return (o.Action == "add" || o.Action == "raise") && !planning.IsCheeseOrNut(o.Name) && o.DayKcal >= low-0.5 && o.DayKcal <= high+0.5
}

// fixMargin is how far past the range's low end the fix aims: the day lands
// just inside, not at the target.
const fixMargin = 50

// smallFix sizes the smallest normal-food step that takes a day of kcal to the
// low end of its range plus a small margin: a piece of fruit (whole servings)
// while the gap is under 150 kcal, else bread (half servings), the other kind
// when that one cannot (at most two servings). It resizes an add option the
// planner offered; nil when none fits.
func smallFix(opts []planning.Option, kcal, low, high float64) *planning.Option {
	gap := low - kcal
	if gap <= 0 {
		return nil
	}
	order := []string{string(food.RoleSideStarch), string(food.RoleSideVeg)}
	step := map[string]float64{string(food.RoleSideStarch): 0.5, string(food.RoleSideVeg): 1}
	if gap < 150 {
		order[0], order[1] = order[1], order[0]
	}
	for _, role := range order {
		for _, o := range opts {
			if o.Action != "add" || o.LeafRole != role || o.Grams == nil || *o.Grams <= 0 || o.ServingG <= 0 || o.KcalDelta <= 0 || planning.IsCheeseOrNut(o.Name) {
				continue
			}
			perG := o.KcalDelta / *o.Grams
			for n := step[role]; n <= 2; n += step[role] {
				g := math.Round(n * o.ServingG)
				if d := g * perG; kcal+d >= low+fixMargin && kcal+d <= high {
					o.Grams, o.KcalDelta, o.DayKcal = &g, math.Round(d), math.Round(kcal+d)
					amount := g
					o.Op.Amount = &amount
					o.Ops = nil
					return &o
				}
			}
		}
	}
	return nil
}

// fixCandidates are the options to try as a day's fix, best first: the small
// sized one, then any option from normal food that lands the day inside.
func fixCandidates(opts []planning.Option, kcal, low, high float64) []planning.Option {
	var out []planning.Option
	if o := smallFix(opts, kcal, low, high); o != nil {
		out = append(out, *o)
	}
	for _, o := range opts {
		if isDayFix(o, low, high) {
			out = append(out, o)
		}
	}
	return out
}

// fixOption is the first fix candidate of a below-range day energy issue (nil
// for any other issue).
func fixOption(is planning.Issue) *planning.Option {
	if is.Code != planning.CodeKcalBelowBand {
		return nil
	}
	low, high := num(is.Params["band_low_kcal"]), num(is.Params["band_high_kcal"])
	if low == 0 || high == 0 {
		return nil
	}
	if c := fixCandidates(is.Options, num(is.Params["kcal"]), low, high); len(c) > 0 {
		return &c[0]
	}
	return nil
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
				return markRefusal{
					err: fmt.Errorf("%w: the %s on %s would still hold %.0f kcal, more than a light meal should stay under (%.0f kcal): make it smaller first (find_ways with goal lighter shows how) or leave the light mark off", domain.ErrInvalidInput, l.Slot, l.Date, kcal, limit),
					tell: []string{
						fmt.Sprintf("I did not mark %s %s as light and saved nothing: it would still hold %.0f kcal, more than a light meal (at most %.0f kcal).", dayLabelStr(l.Date), l.Slot, kcal, limit),
						"I can make it smaller first, or leave the light mark off: say so if you want one of them.",
					},
				}
			}
			least, _ := is.Params["min_kcal"].(float64)
			return markRefusal{
				err: fmt.Errorf("%w: the %s on %s would hold only %.0f kcal, less than a filling meal needs at least (%.0f kcal): make it bigger first (find_ways with goal heartier shows how) or leave the filling mark off", domain.ErrInvalidInput, l.Slot, l.Date, kcal, least),
				tell: []string{
					fmt.Sprintf("I did not mark %s %s as filling and saved nothing: it would hold only %.0f kcal, less than a filling meal (at least %.0f kcal).", dayLabelStr(l.Date), l.Slot, kcal, least),
					"I can make it bigger first, or leave the filling mark off: say so if you want one of them.",
				},
			}
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
	case planning.GoalLighter, planning.GoalHeartier, planning.GoalMoreProtein, planning.GoalLowerDay, planning.GoalRaiseDay:
	default:
		return nil, fmt.Errorf("%w: goal.kind must be lighter, heartier, more_protein, lower_day or raise_day", domain.ErrInvalidInput)
	}
	dayGoal := g.Kind == planning.GoalLowerDay || g.Kind == planning.GoalRaiseDay
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
	} else if (g.Kind == planning.GoalLighter || g.Kind == planning.GoalHeartier) || date == "" {
		return nil, fmt.Errorf("%w: goal %s needs %s", domain.ErrInvalidInput, g.Kind, map[bool]string{true: "date", false: "plate_id"}[dayGoal])
	}
	var maxKcal, minKcal float64
	if dayGoal {
		for _, d := range after.Days {
			if d.Date == date {
				maxKcal, minKcal = d.BandHigh, d.BandLow
			}
		}
		if g.Kind == planning.GoalLowerDay {
			minKcal = 0
		} else {
			maxKcal = 0
		}
	} else if g.Kind != planning.GoalMoreProtein {
		if maxKcal, minKcal, err = planning.MealLimit(st, planning.WindowTarget(st), g.PlateID, g.Kind); err != nil {
			return nil, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
		}
	}
	b, a := planning.FiguresOf(before, g.PlateID, date), planning.FiguresOf(after, g.PlateID, date)
	changed, reached, why := planning.JudgeRelative(g.Kind, g.PlateID != 0, maxKcal, minKcal, b, a)
	satisfied := changed
	if !changed && (g.Kind == planning.GoalLighter || g.Kind == planning.GoalHeartier) {
		for _, l := range light {
			if l.Scope == planning.LightScopeMeal && l.Date == date && l.Hearty() == (g.Kind == planning.GoalHeartier) {
				return nil, fmt.Errorf("%w: %s: a mark alone does not make a meal %s. Take the ops of a way from find_ways with goal %s", domain.ErrInvalidInput, why, g.Kind, g.Kind)
			}
		}
	}
	req := map[string]any{"goal": g.Kind, "before": b, "after": a, "satisfied": satisfied}
	switch g.Kind {
	case planning.GoalLighter:
		req["reached_light"] = reached
	case planning.GoalHeartier:
		req["reached_hearty"] = reached
	case planning.GoalLowerDay, planning.GoalRaiseDay:
		req["reached_band"] = reached
	}
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

// nextStep tells the model what to do with the result, computed here so it
// reads it at the moment of choice.
func nextStep(ways, offers []planning.Way, res planning.WayResult, toLimit bool) string {
	text := nextStepText(ways, offers, res, toLimit)
	switch {
	case toLimit && len(offers) > 0 && offers[0].Reaches && (len(ways) == 0 || !ways[0].Reaches):
		text += fixNote(offers[0])
	case len(ways) > 0:
		text += fixNote(ways[0])
	case len(offers) > 0:
		text += fixNote(offers[0])
	}
	if len(ways) > 1 && strings.HasPrefix(text, "apply ways[0]") {
		text += ". The ways are alternatives, each from the same starting plan: apply one, never add their effects together"
	}
	return text
}

func nextStepText(ways, offers []planning.Way, res planning.WayResult, toLimit bool) string {
	switch {
	case toLimit && len(offers) > 0 && offers[0].Reaches && (len(ways) == 0 || !ways[0].Reaches):
		// A light meal was asked for: only the offer is one.
		return fmt.Sprintf("the user asked for a light meal and only offer[0] is one (%.0f kcal, a light meal holds at most %.0f); ways[] only make it lighter. If the user asked for another meal to make up the difference (a bigger lunch) or has said yes to that, apply offer[0] now, with its light and goal as returned, and tell what leaves and what comes; otherwise present offer[0] as your one recommendation in a sentence. Nothing else is applied", offers[0].After.MealKcal, res.MaxKcal)
	case res.Impossible && len(offers) > 0:
		return fmt.Sprintf("change nothing: say why with the numbers (%s); END YOUR REPLY with offer[0] as your one recommendation, the lowest the day goes (%.0f kcal) and how, and ask whether to do it", res.EmptyWhy, offers[0].After.DayKcal)
	case res.Impossible:
		return fmt.Sprintf("change nothing: say why with the numbers (%s); offer offer[0] as the nearest safe option and ask whether to do it", res.EmptyWhy)
	case (res.Goal == planning.GoalLighter || res.Goal == planning.GoalHeartier) && len(ways) > 0 && !ways[0].Reaches:
		// Out of reach of the limit, a real step is still what was asked: apply it
		// now, say how far it got, and offer the rest.
		limit := fmt.Sprintf("a filling meal needs at least %.0f kcal", res.MinKcal)
		if res.Goal == planning.GoalLighter {
			limit = fmt.Sprintf("a light meal should stay under %.0f kcal and going lower would take the day under its minimum of %.0f kcal unless another meal grows", res.MaxKcal, res.FloorKcal)
		}
		text := fmt.Sprintf("apply ways[0] now and do not ask first: it takes the meal from %.0f to %.0f kcal, a real step and as far as the day allows (%s). Tell the user plainly how far it got and why it stops there", ways[0].Before.MealKcal, ways[0].After.MealKcal, limit)
		if len(offers) > 0 {
			rec := offers[0]
			for _, o := range offers {
				if o.Recommended {
					rec = o
				}
			}
			text += fmt.Sprintf(". If the user asked for another meal to make up the difference (a bigger lunch) or has said yes to that, apply offer[0] instead of ways[0]: it takes the meal to %.0f kcal; otherwise END YOUR REPLY with the recommended offer as the way further, in one sentence (I can also %s; the day would then hold %.0f kcal): never leave it out, and apply it only when the user says yes", offers[0].After.MealKcal, rec.What, rec.After.DayKcal)
		}
		return text
	case len(ways) > 0 && len(offers) > 0:
		return "apply ways[0] now with apply_changes (its ops, light and goal as returned), tell the user what changed with before and after, say if it is not yet a light (hearty) meal, and present the recommended one as your recommendation: apply an offer only when the user says yes"
	case len(ways) > 0:
		return "apply ways[0] now with apply_changes (its ops, light and goal as returned), then tell the user what changed with before and after"
	case len(offers) > 0:
		return "nothing can be applied on its own here: explain why with the numbers and present offer[0] as your one recommended action ('I would ...'), naming what changes in both meals and the day fix if it has one. If the user already asked for the other meal to make up the difference (a bigger lunch) or has said yes to it, apply offer[0] now; otherwise apply it only when the user says yes"
	}
	return "nothing to apply: explain why with the numbers and what the user could change"
}

// slicesDeleteFlag drops the warning note an earlier stage appended to a way's
// reason (the check just rebuilt the warnings from the write's own report).
func slicesDeleteFlag(why string) []string {
	if i := strings.Index(why, "creates a new warning"); i >= 0 {
		why = strings.TrimRight(strings.TrimSpace(why[:i]), ";")
	}
	return []string{strings.TrimSpace(why)}
}

// markRefusal is a refused light or filling mark: the error for the log and
// the short tell the reply says (what was refused, why with the number, the
// one thing that can be done instead).
type markRefusal struct {
	err  error
	tell []string
}

func (e markRefusal) Error() string { return e.err.Error() }
func (e markRefusal) Unwrap() error { return e.err }
