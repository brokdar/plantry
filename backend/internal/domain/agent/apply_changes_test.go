package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// dbCounts is what an apply_changes dry run must leave untouched.
func (e *planToolEnv) dbCounts() map[string]int {
	e.t.Helper()
	out := map[string]int{}
	for _, table := range []string{"plates", "plate_components", "cook_sessions", "plate_events", "availability_overrides", "ai_changes"} {
		var n int
		require.NoError(e.t, e.db.QueryRow(`SELECT COUNT(*) FROM `+table).Scan(&n), table)
		out[table] = n
	}
	var portions float64
	require.NoError(e.t, e.db.QueryRow(`SELECT COALESCE(SUM(portions), 0) FROM cook_sessions`).Scan(&portions))
	out["portions"] = int(portions * 100)
	return out
}

// freshMain returns a draft plate whose main is cooked on the plate's own
// cell, with the session id.
func (e *planToolEnv) freshMain() (plate.Plate, int64) {
	e.t.Helper()
	for _, p := range e.week() {
		for _, c := range p.Components {
			if c.FoodRole == nil || *c.FoodRole != "main" || c.CookSessionID == nil || p.State != plate.StateDraft {
				continue
			}
			v := e.session(*c.CookSessionID)
			if v.CookDate.Equal(p.Date) && v.CookSlotID == p.SlotID {
				return p, v.ID
			}
		}
	}
	e.t.Fatal("no draft plate cooks its own main")
	return plate.Plate{}, 0
}

// emptyOtherSlot clears and returns another slot of p's date.
func (e *planToolEnv) emptyOtherSlot(p plate.Plate) int64 {
	e.t.Helper()
	for _, q := range e.week() {
		if q.Date.Equal(p.Date) && q.SlotID != p.SlotID {
			_, err := e.plates.Delete(context.Background(), q.ID)
			require.NoError(e.t, err)
			return q.SlotID
		}
	}
	var id int64
	require.NoError(e.t, e.db.QueryRow(`SELECT id FROM time_slots WHERE id != ? ORDER BY id LIMIT 1`, p.SlotID).Scan(&id))
	return id
}

// issueRules are the labels of an issue list: the model reads no rule codes.
func issueRules(t *testing.T, res map[string]any, key string) []string {
	t.Helper()
	issues := res["after"].(map[string]any)["issues"].(map[string]any)
	var rules []string
	for _, is := range issues[key].([]any) {
		rules = append(rules, is.(map[string]any)["label"].(string))
	}
	return rules
}

// A dry run reports the full result and writes nothing: no row of any plan
// table, no override, no ai_changes row and no change handed to the loop.
func TestApplyChanges_DryRunLeavesTheDatabaseUnchanged(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	p, sid := e.freshMain()
	slotID := e.emptyOtherSlot(p)
	v := e.session(sid)
	before := e.dbCounts()

	input := fmt.Sprintf(`{"dry_run":true,"ops":[
		{"op":"set_eaters","plate_id":%d,"eaters":3},
		{"op":"set_portions","session_id":%d,"portions":%v},
		{"op":"add","date":%q,"slot_id":%d,"food_id":%d,"portions":1},
		{"op":"set_availability","date":%q,"slot_id":%d,"mode":"none"}]}`,
		p.ID, sid, v.Portions+3, p.DateString(), slotID, e.sides[0], p.DateString(), slotID)
	out, _, ch, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(input))
	require.NoError(t, err)
	assert.Nil(t, ch, "a dry run is no change")
	var res map[string]any
	require.NoError(t, json.Unmarshal(out, &res))
	assert.Equal(t, true, res["dry_run"])
	assert.Len(t, res["applied"], 4)
	after := res["after"].(map[string]any)
	assert.NotEmpty(t, after["days"], "the result reports the touched days")
	assert.Contains(t, after, "issues")

	assert.Equal(t, before, e.dbCounts(), "nothing was written")
	assert.InDelta(t, v.Portions, e.session(sid).Portions, 1e-9)
	got, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err)
	assert.InDelta(t, p.Eaters, got.Eaters, 1e-9)

	// The same ops for real do write and record one change.
	real := e.change(ctx, "apply_changes", fmt.Sprintf(`{"ops":[{"op":"set_eaters","plate_id":%d,"eaters":3}]}`, p.ID))
	assert.Equal(t, "apply_changes", real.TextKey)
	assert.Equal(t, before["ai_changes"]+1, e.dbCounts()["ai_changes"])
}

// All ops share one transaction: a failing op names itself and rolls back
// the ops before it.
func TestApplyChanges_FailingOpRollsBackTheBatch(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.freshMain()
	before := e.dbCounts()
	_, err := e.exec("apply_changes", fmt.Sprintf(`{"ops":[
		{"op":"set_eaters","plate_id":%d,"eaters":3},
		{"op":"set_eaters","plate_id":999999,"eaters":2}]}`, p.ID))
	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Contains(t, err.Error(), "op 2 of 2 (set_eaters)")
	assert.Equal(t, before, e.dbCounts())
	got, err := e.plates.Get(context.Background(), p.ID)
	require.NoError(t, err)
	assert.InDelta(t, p.Eaters, got.Eaters, 1e-9, "the first op was rolled back")
}

// The Friday salad case: a second recipe on a day that already cooks is a
// second cooking occasion (H2) unless it is cooked together with the existing
// batch. cook_at puts it on the existing cook cell, so H2 stays quiet.
func TestApplyChanges_CookAtSharesTheCookCell(t *testing.T) {
	e := newPlanToolEnv(t)
	e.setCookLimit(1)
	p, _ := e.freshMain()
	slotID := e.emptyOtherSlot(p)
	side := e.sides[0]

	alone, err := e.exec("apply_changes", fmt.Sprintf(
		`{"dry_run":true,"ops":[{"op":"add","date":%q,"slot_id":%d,"food_id":%d,"portions":1}]}`, p.DateString(), slotID, side))
	require.NoError(t, err)
	assert.Contains(t, issueRules(t, alone, "new"), "cooking times per day", "cooking the side in its own cell is a second occasion")

	shared, err := e.exec("apply_changes", fmt.Sprintf(
		`{"ops":[{"op":"add","date":%q,"slot_id":%d,"food_id":%d,"portions":1,"cook_at":{"date":%q,"slot_id":%d}}]}`,
		p.DateString(), slotID, side, p.DateString(), p.SlotID))
	require.NoError(t, err)
	assert.NotContains(t, issueRules(t, shared, "new"), "cooking times per day")
	days := shared["after"].(map[string]any)["days"].([]any)
	require.NotEmpty(t, days)
	day := days[0].(map[string]any)
	assert.Equal(t, p.DateString(), day["date"])
	assert.EqualValues(t, 1, day["cooking_times"], "one cooking occasion that day")
}

// find_foods no_cook returns leaves that need no cooking (raw produce) and
// leaves out cooked ones.
func TestFindFoods_NoCookReturnsLeaves(t *testing.T) {
	e := newPlanToolEnv(t)
	e.insert(`INSERT INTO foods (name, kind, source, shop_section, kcal_100g, protein_100g, fat_100g, carbs_100g) VALUES ('Gurke', 'leaf', 'manual', 'produce', 15, 0.7, 0.1, 3.6)`)
	e.insert(`INSERT INTO foods (name, kind, source, kcal_100g, protein_100g, fat_100g, carbs_100g) VALUES ('Reis', 'leaf', 'manual', 130, 2.7, 0.3, 28)`)

	res := e.mustExec("find_foods", `{"no_cook":true}`)
	var names []string
	for _, it := range res["items"].([]any) {
		f := it.(map[string]any)
		names = append(names, f["name"].(string))
		assert.Equal(t, "leaf", f["kind"])
		assert.Equal(t, true, f["no_cook"])
		assert.Equal(t, false, f["counts_as_cooking"])
		assert.Contains(t, f, "per_100g")
	}
	assert.Equal(t, []string{"Gurke"}, names)
}

func (e *planToolEnv) setCookLimit(n int) {
	e.t.Helper()
	require.NoError(e.t, testsvc.Settings(e.db).Set(context.Background(), "plan.max_cook_occasions_per_day", fmt.Sprint(n)))
}
