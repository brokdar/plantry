package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// relEnv is the batch-cooker week with meals of a few hundred kcal on a
// 2000 kcal target, so a light cap and a hearty minimum mean something.
func relEnv(t *testing.T) *planToolEnv {
	e := newPlanToolEnv(t)
	e.withMacros()
	e.sql(`UPDATE foods SET kcal_100g = 300 WHERE name = 'Kichererbsen'`)
	return e
}

// draftMeals are the window's draft lunch and dinner plates.
func (e *planToolEnv) draftMeals() []plate.Plate {
	var out []plate.Plate
	for _, p := range e.week() {
		if p.State == plate.StateDraft && (p.SlotID == e.slotID("lunch") || p.SlotID == e.slotID("dinner")) {
			out = append(out, p)
		}
	}
	return out
}

func relMaps(v any) []map[string]any {
	var out []map[string]any
	for _, x := range v.([]any) {
		out = append(out, x.(map[string]any))
	}
	return out
}

// applyWay applies a way of find_ways with its light and the goal, as the
// model is told to.
func (e *planToolEnv) applyWay(res, way map[string]any, dryRun bool) (map[string]any, error) {
	in := map[string]any{"ops": way["ops"], "goal": res["goal"], "dry_run": dryRun}
	if l, ok := way["light"]; ok {
		in["light"] = l
	}
	raw, err := json.Marshal(in)
	require.NoError(e.t, err)
	return e.exec("apply_changes", string(raw))
}

// Every way and offer find_ways lists passes the dry run of apply_changes
// with its own ops, light and goal: no new hard issue, satisfied, and the
// cap flag agrees. Ways change only the named meal; an offer is the only
// compound kind, and it never adds a filler (only a side recipe or bread).
func TestFindWays_EveryWayAndOfferPassesItsOwnDryRun(t *testing.T) {
	e := relEnv(t)
	ctx, _ := e.chat()
	checked, offers, goals := 0, 0, map[string]int{}
	for _, p := range e.draftMeals() {
		for _, goal := range []string{"lighter", "heartier", "more_protein"} {
			out, _, _, err := e.ts.Execute(ctx, "find_ways", json.RawMessage(fmt.Sprintf(`{"goal":%q,"plate_id":%d}`, goal, p.ID)))
			require.NoError(t, err, "plate %d %s", p.ID, goal)
			var res map[string]any
			require.NoError(t, json.Unmarshal(out, &res))
			var all []map[string]any
			// Exactly one way or offer is recommended, with its reason.
			rec := 0
			for _, k := range []string{"ways", "offer"} {
				if res[k] == nil {
					continue
				}
				for _, w := range relMaps(res[k]) {
					if w["recommended"] == true {
						rec++
						assert.NotEmpty(t, w["recommended_because"])
					}
				}
			}
			if res["ways"] != nil && len(relMaps(res["ways"])) > 0 || res["offer"] != nil {
				assert.Equal(t, 1, rec, "%s plate %d: exactly one recommended", goal, p.ID)
				assert.NotEmpty(t, res["next"])
			}
			for _, w := range relMaps(res["ways"]) {
				assert.NotEqual(t, "compound", w["kind"], "a compound way is never a default")
				assert.Nil(t, w["changes_other_meal"])
				all = append(all, w)
			}
			if res["offer"] != nil {
				for _, o := range relMaps(res["offer"]) {
					assert.Equal(t, "compound", o["kind"])
					assert.Equal(t, true, o["changes_other_meal"])
					assert.NotEmpty(t, o["moved"])
					for _, op := range relMaps(o["ops"]) {
						if op["op"] != "add" {
							continue
						}
						var kind, role string
						require.NoError(t, e.db.QueryRow(`SELECT kind, COALESCE(role,'') FROM foods WHERE id = ?`, int64(op["food_id"].(float64))).Scan(&kind, &role))
						assert.Equal(t, "composed", kind, "an offer adds a side recipe, not a filler")
						assert.Contains(t, []string{"side_starch", "side_veg"}, role)
					}
					all = append(all, o)
					offers++
				}
			}
			for _, way := range all {
				got, err := e.applyWayCtx(ctx, res, way)
				require.NoError(t, err, "plate %d %s: %v", p.ID, goal, way["what"])
				fresh := relMaps(got["after"].(map[string]any)["issues"].(map[string]any)["new"])
				for _, is := range fresh {
					assert.NotEqual(t, "problem", is["severity"], "%s %v adds %v", goal, way["what"], is["label"])
					// A new warning is allowed only when the way says so.
					assert.Contains(t, fmt.Sprint(way["why"]), "new warning", "%s %v creates %v without saying", goal, way["what"], is["label"])
				}
				req := got["request"].(map[string]any)
				assert.Equal(t, true, req["satisfied"], "%s %v: %v", goal, way["what"], req["why"])
				for _, k := range []string{"reached_light", "reached_hearty"} {
					if v, ok := req[k]; ok {
						assert.Equal(t, way["reaches_limit"], v, "%s %v", goal, way["what"])
					}
				}
				checked++
				goals[goal]++
			}
		}
	}
	assert.Positive(t, checked)
	for _, g := range []string{"lighter", "heartier", "more_protein"} {
		assert.Positive(t, goals[g], "ways for %s", g)
	}
	t.Logf("checked %d ways and offers, %d offers", checked, offers)
}

func (e *planToolEnv) applyWayCtx(ctx context.Context, res, way map[string]any) (map[string]any, error) {
	in := map[string]any{"ops": way["ops"], "goal": res["goal"], "dry_run": true}
	if l, ok := way["light"]; ok {
		in["light"] = l
	}
	raw, _ := json.Marshal(in)
	out, _, _, err := e.ts.Execute(ctx, "apply_changes", raw)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	require.NoError(e.t, json.Unmarshal(out, &m))
	return m, nil
}

// A light mark is stored only on a meal that meets its cap: the call is
// refused with the numbers and a pointer to find_ways; a hearty mark the same.
func TestApplyChanges_MarkOnAMealThatMissesItsLimitIsRefused(t *testing.T) {
	e := relEnv(t)
	e.sql(`UPDATE user_profile SET kcal_target = 600 WHERE id = 1`) // a share of 200 kcal: cap 150, minimum 250
	ctx, _ := e.chat()
	var p plate.Plate
	for _, q := range e.draftMeals() {
		if q.SlotID == e.slotID("dinner") {
			p = q
			break
		}
	}
	require.NotZero(t, p.ID)
	for level, want := range map[string]string{"light": "a light meal holds at most", "hearty": "a filling meal holds at least"} {
		in := fmt.Sprintf(`{"dry_run":true,"light":[{"date":%q,"scope":"meal","slot":"dinner","level":%q}],"ops":[{"op":"set_eaters","plate_id":%d,"eaters":1}]}`, p.DateString(), level, p.ID)
		_, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(in))
		if level == "hearty" {
			// A 750 kcal dinner is above a 250 kcal minimum: no refusal.
			require.NoError(t, err)
			continue
		}
		require.ErrorIs(t, err, domain.ErrInvalidInput)
		assert.Contains(t, err.Error(), want)
		assert.Contains(t, err.Error(), "find_ways")
		assert.NotContains(t, err.Error(), "S6")
	}
	e.sql(`UPDATE user_profile SET kcal_target = 20000 WHERE id = 1`) // a share of 6667: minimum 8333
	in := fmt.Sprintf(`{"dry_run":true,"light":[{"date":%q,"scope":"meal","slot":"dinner","level":"hearty"}],"ops":[{"op":"set_eaters","plate_id":%d,"eaters":1}]}`, p.DateString(), p.ID)
	_, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(in))
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Contains(t, err.Error(), "a filling meal holds at least")
}

// A call that only sets a mark needs no no-op op when the meal meets its cap;
// for a relative request (a goal) ops are required and the food must change.
func TestApplyChanges_MarkOnlyAndGoalRules(t *testing.T) {
	e := relEnv(t)
	e.sql(`UPDATE user_profile SET kcal_target = 20000 WHERE id = 1`) // caps far above any meal
	ctx, _ := e.chat()
	p := e.draftMeals()[0]
	slot := "lunch"
	if p.SlotID == e.slotID("dinner") {
		slot = "dinner"
	}
	mark := fmt.Sprintf(`[{"date":%q,"scope":"meal","slot":%q}]`, p.DateString(), slot)
	_, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(`{"dry_run":true,"light":`+mark+`}`))
	require.NoError(t, err, "a mark on a meal under its cap needs no op")

	_, _, _, err = e.ts.Execute(ctx, "apply_changes", json.RawMessage(`{"dry_run":true}`))
	require.ErrorIs(t, err, domain.ErrInvalidInput, "neither ops nor light")

	goal := fmt.Sprintf(`{"kind":"lighter","plate_id":%d}`, p.ID)
	_, _, _, err = e.ts.Execute(ctx, "apply_changes", json.RawMessage(`{"dry_run":true,"goal":`+goal+`,"light":`+mark+`}`))
	require.ErrorIs(t, err, domain.ErrInvalidInput, "a goal needs ops")

	noop := fmt.Sprintf(`[{"op":"set_eaters","plate_id":%d,"eaters":1}]`, p.ID)
	_, _, _, err = e.ts.Execute(ctx, "apply_changes", json.RawMessage(`{"dry_run":true,"goal":`+goal+`,"light":`+mark+`,"ops":`+noop+`}`))
	require.ErrorIs(t, err, domain.ErrInvalidInput, "a mark with no food change does not make a meal lighter")
	assert.Contains(t, err.Error(), "find_ways")

	// Without the mark the call is allowed and says it is not satisfied.
	out, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(`{"dry_run":true,"goal":`+goal+`,"ops":`+noop+`}`))
	require.NoError(t, err)
	var res map[string]any
	require.NoError(t, json.Unmarshal(out, &res))
	req := res["request"].(map[string]any)
	assert.Equal(t, false, req["satisfied"])
	assert.NotEmpty(t, req["why"])
}

// Taking the first way for real: the request is satisfied, the mark is
// stored with it and the food changed.
func TestFindWays_ApplyFirstWayStoresMarkAndSatisfies(t *testing.T) {
	e := relEnv(t)
	ctx, _ := e.chat()
	for _, p := range e.draftMeals() {
		out, _, _, err := e.ts.Execute(ctx, "find_ways", json.RawMessage(fmt.Sprintf(`{"goal":"lighter","plate_id":%d}`, p.ID)))
		require.NoError(t, err)
		var res map[string]any
		require.NoError(t, json.Unmarshal(out, &res))
		ways := relMaps(res["ways"])
		if len(ways) == 0 || ways[0]["reaches_limit"] != true || ways[0]["light"] == nil {
			continue
		}
		got, err := e.applyWay(res, ways[0], false)
		require.NoError(t, err)
		req := got["request"].(map[string]any)
		assert.Equal(t, true, req["satisfied"])
		before, after := req["before"].(map[string]any), req["after"].(map[string]any)
		assert.Greater(t, before["meal_kcal"], after["meal_kcal"])
		var n int
		require.NoError(t, e.db.QueryRow(`SELECT COUNT(*) FROM plan_light WHERE date = ?`, p.DateString()).Scan(&n))
		assert.Positive(t, n, "the mark of the way is stored")
		return
	}
	t.Skip("no lunch or dinner of the fixture has a lighter way that reaches its cap")
}

// E013: a limit inside constraints is refused with the place it belongs.
func TestFindAlternatives_MisplacedLimitIsRedirected(t *testing.T) {
	e := relEnv(t)
	ctx, _ := e.chat()
	p := e.draftMeals()[0]
	_, _, _, err := e.ts.Execute(ctx, "find_alternatives", json.RawMessage(fmt.Sprintf(`{"plate_id":%d,"what":"main","constraints":{"limit":3}}`, p.ID)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "top level")
	_, _, _, err = e.ts.Execute(ctx, "find_alternatives", json.RawMessage(fmt.Sprintf(`{"plate_id":%d,"what":"main","limit":3}`, p.ID)))
	require.NoError(t, err)
}

// E008: lower_day and raise_day come with a next step, every way passes its
// dry run and lands the day in the adjusted range.
func TestFindWays_DayGoalsAndNextStep(t *testing.T) {
	e := relEnv(t)
	ctx, _ := e.chat()
	dates := map[string]bool{}
	for _, p := range e.draftMeals() {
		dates[p.DateString()] = true
	}
	checked, nothing := 0, 0
	for date := range dates {
		for _, goal := range []string{"lower_day", "raise_day"} {
			out, _, _, err := e.ts.Execute(ctx, "find_ways", json.RawMessage(fmt.Sprintf(`{"goal":%q,"date":%q}`, goal, date)))
			require.NoError(t, err, "%s %s", goal, date)
			var res map[string]any
			require.NoError(t, json.Unmarshal(out, &res))
			next, _ := res["next"].(string)
			ways := relMaps(res["ways"])
			if len(ways) == 0 {
				assert.Contains(t, next, "nothing")
				assert.NotEmpty(t, res["empty_reason"])
				nothing++
				continue
			}
			assert.Contains(t, next, "apply ways[0] now")
			for _, way := range ways {
				got, err := e.applyWayCtx(ctx, res, way)
				require.NoError(t, err, "%s %s: %v", goal, date, way["what"])
				req := got["request"].(map[string]any)
				assert.Equal(t, true, req["satisfied"], "%v", req["why"])
				assert.Equal(t, true, req["reached_range"], "%v", req["why"])
				for _, is := range relMaps(got["after"].(map[string]any)["issues"].(map[string]any)["new"]) {
					assert.NotEqual(t, "problem", is["severity"])
				}
				checked++
			}
		}
	}
	assert.Positive(t, checked)
	t.Logf("%d day ways checked, %d day requests without a way", checked, nothing)
}

func TestFindWays_NextStepWithAnOffer(t *testing.T) {
	// An offer alone: explain and ask; ways and an offer: apply and mention.
	e := relEnv(t)
	ctx, _ := e.chat()
	for _, p := range e.draftMeals() {
		out, _, _, err := e.ts.Execute(ctx, "find_ways", json.RawMessage(fmt.Sprintf(`{"goal":"lighter","plate_id":%d}`, p.ID)))
		require.NoError(t, err)
		var res map[string]any
		require.NoError(t, json.Unmarshal(out, &res))
		next, _ := res["next"].(string)
		hasWays, hasOffer := len(relMaps(res["ways"])) > 0, res["offer"] != nil
		switch {
		case hasWays && hasOffer:
			assert.Contains(t, next, "present offer[0] as a choice")
		case hasWays:
			assert.NotContains(t, next, "offer")
		case hasOffer:
			assert.Contains(t, next, "only when the user says yes")
		default:
			assert.Contains(t, next, "nothing to apply")
		}
	}
}

// mainComponentID returns the id of the plate's main component.
func mainComponentID(t *testing.T, p plate.Plate) int64 {
	t.Helper()
	for _, c := range p.Components {
		if c.FoodRole != nil && *c.FoodRole == "main" {
			return c.ID
		}
	}
	t.Fatalf("plate %d has no main", p.ID)
	return 0
}

// E018b: a write never leaves a day under its minimum. Of several ops the safe
// subset is saved and the rest is named with numbers; a single unsafe op is
// refused (nothing saved); explicit away and all-safe calls are untouched.
func TestApplyChanges_NeverLeavesADayUnderItsMinimum(t *testing.T) {
	e := relEnv(t)
	ctx, _ := e.chat()
	var dinners []plate.Plate
	for _, p := range e.draftMeals() {
		if p.SlotID == e.slotID("dinner") {
			dinners = append(dinners, p)
		}
	}
	require.GreaterOrEqual(t, len(dinners), 2)
	a, b := dinners[0], dinners[1]
	require.NotEqual(t, a.DateString(), b.DateString())

	// A floor just under day A's kcal: any real cut of A falls under it.
	gp := e.mustExec("get_plan", fmt.Sprintf(`{"from":%q,"to":%q}`, a.DateString(), a.DateString()))
	var kcal float64
	for _, d := range gp["evaluation"].(map[string]any)["days"].([]any) {
		if m := d.(map[string]any); m["date"] == a.DateString() {
			kcal = m["kcal"].(float64)
		}
	}
	require.Positive(t, kcal)
	e.sql(`UPDATE user_profile SET kcal_target = ? WHERE id = 1`, (kcal-5)/0.75)
	portions := func(p plate.Plate, id int64) float64 {
		for _, q := range e.week() {
			if q.ID == p.ID {
				for _, c := range q.Components {
					if c.ID == id {
						return *c.Portions
					}
				}
			}
		}
		t.Fatal("component gone")
		return 0
	}
	ma, mb := mainComponentID(t, a), mainComponentID(t, b)
	cut := fmt.Sprintf(`{"op":"set_quantity","plate_component_id":%d,"portions":0.5}`, ma)
	raise := fmt.Sprintf(`{"op":"set_quantity","plate_component_id":%d,"portions":1.5}`, mb)
	was := portions(a, ma)

	t.Run("the safe subset is saved, the unsafe op named with numbers", func(t *testing.T) {
		res, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(`{"ops":[`+cut+`,`+raise+`]}`))
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(res, &got))
		skipped := relMaps(got["skipped"])
		require.Len(t, skipped, 1)
		assert.EqualValues(t, 1, skipped[0]["op"])
		assert.Contains(t, skipped[0]["why"], "under its minimum")
		assert.Contains(t, got["next"], "safe part is applied (1 of 2")
		assert.Empty(t, plannerWord.FindAllString(string(res), -1), "kitchen words only")
		assert.NotRegexp(t, isoDate, fmt.Sprint(got["next"], got["skipped"]), "dates the way the user reads them")
		assert.Contains(t, got["_result"], "SAVED")
		assert.NotContains(t, got["_result"], "NOT SAVED")
		assert.InDelta(t, was, portions(a, ma), 1e-9, "the unsafe cut was not saved")
		assert.InDelta(t, 1.5, portions(b, mb), 1e-9, "the safe raise was")
		for _, is := range relMaps(got["after"].(map[string]any)["issues"].(map[string]any)["new"]) {
			assert.NotEqual(t, "problem", is["severity"])
		}
	})
	t.Run("a single unsafe op is refused, not half-applied", func(t *testing.T) {
		res, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(`{"ops":[`+cut+`]}`))
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(res, &got))
		assert.Equal(t, false, got["saved"])
		assert.Contains(t, got["refused"], "under its minimum")
		assert.Contains(t, got["next"], "nothing was saved")
		assert.Contains(t, got["_result"], "NOT SAVED")
		assert.NotRegexp(t, isoDate, fmt.Sprint(got["refused"], got["next"]))
		assert.Empty(t, plannerWord.FindAllString(string(res), -1), "kitchen words only")
		assert.InDelta(t, was, portions(a, ma), 1e-9)
	})
	t.Run("a call that is all safe has no skipped part", func(t *testing.T) {
		res, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(`{"dry_run":true,"ops":[`+raise+`]}`))
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(res, &got))
		assert.Nil(t, got["skipped"])
		assert.Contains(t, got["_result"], "NOT SAVED: this was only a check")
	})
	t.Run("away is explicit and not judged", func(t *testing.T) {
		res, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(fmt.Sprintf(`{"ops":[{"op":"away","date":%q}]}`, a.DateString())))
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(res, &got))
		assert.Nil(t, got["skipped"])
		assert.Nil(t, got["saved"])
	})
}

// What find_alternatives shows is what a write of the alternative's ops would
// produce: a candidate that repeats a dish (seen only by the real check) lists
// that warning and is never the recommended one.
func TestFindAlternatives_ListedWarningsAreTheWritesWarnings(t *testing.T) {
	e := relEnv(t)
	shown, repeats := 0, 0
	for _, p := range e.draftMeals() {
		res := e.mustExec("find_alternatives", fmt.Sprintf(`{"plate_id":%d,"what":"main","limit":10}`, p.ID))
		for _, a := range relMaps(res["alternatives"]) {
			ops, err := json.Marshal(a["ops"])
			require.NoError(t, err)
			got := e.mustExec("apply_changes", `{"dry_run":true,"ops":`+string(ops)+`}`)
			fresh := relMaps(got["after"].(map[string]any)["issues"].(map[string]any)["new"])
			listed, _ := a["new_issues"].([]any)
			assert.Len(t, listed, len(fresh), "plate %d %v: listed %v, the write says %v", p.ID, a["name"], listed, fresh)
			if len(listed) > 0 {
				assert.NotEqual(t, true, a["recommended"], "%v creates a warning", a["name"])
			}
			if strings.Contains(fmt.Sprint(fresh), "dish repeated") {
				repeats++
				assert.Contains(t, fmt.Sprint(listed), "dish repeated", "%v repeats a dish", a["name"])
			}
			shown++
		}
	}
	require.NotZero(t, shown)
	require.NotZero(t, repeats, "the week has a candidate that repeats a dish")
}
