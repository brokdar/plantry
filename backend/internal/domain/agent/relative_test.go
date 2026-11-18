package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
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

// Every way find_ways lists passes the dry run of apply_changes with its own
// ops, light and goal: no new hard issue, and a way that reaches the goal is
// reported satisfied.
func TestFindWays_EveryWayPassesItsOwnDryRun(t *testing.T) {
	e := relEnv(t)
	ctx, _ := e.chat()
	checked, goals := 0, map[string]int{}
	for _, p := range e.draftMeals() {
		for _, goal := range []string{"lighter", "heartier", "more_protein"} {
			out, _, _, err := e.ts.Execute(ctx, "find_ways", json.RawMessage(fmt.Sprintf(`{"goal":%q,"plate_id":%d}`, goal, p.ID)))
			require.NoError(t, err, "plate %d %s", p.ID, goal)
			var res map[string]any
			require.NoError(t, json.Unmarshal(out, &res))
			for _, way := range relMaps(res["ways"]) {
				got, err := e.applyWayCtx(ctx, res, way)
				require.NoError(t, err, "plate %d %s: %v", p.ID, goal, way["what"])
				for _, is := range relMaps(got["after"].(map[string]any)["issues"].(map[string]any)["new"]) {
					assert.NotEqual(t, "violation", is["severity"], "%s %v adds %v", goal, way["what"], is["label"])
				}
				req := got["request"].(map[string]any)
				assert.Equal(t, way["reaches"] == true, req["satisfied"] == true, "%s %v: %v", goal, way["what"], req["why"])
				checked++
				goals[goal]++
			}
		}
	}
	assert.Positive(t, checked)
	for _, g := range []string{"lighter", "heartier", "more_protein"} {
		assert.Positive(t, goals[g], "ways for %s", g)
	}
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
	for level, want := range map[string]string{"light": "light cap", "hearty": "hearty minimum"} {
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
	assert.Contains(t, err.Error(), "hearty minimum")
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
		if len(ways) == 0 || ways[0]["reaches"] != true || ways[0]["light"] == nil {
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
