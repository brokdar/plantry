package agent_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// R12a C1: a refusal for too many cooking times names what already cooks.
func TestApplyChanges_CookingTimesRefusalNamesTheBatch(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	e.setCookLimit(1)
	p, _ := e.freshMain()
	slotID := e.emptyOtherSlot(p)
	side := e.sides[0]
	res, err := e.exec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"add","date":%q,"slot_id":%d,"food_id":%d,"portions":1}]}`, p.DateString(), slotID, side))
	require.NoError(t, err)
	skipped := relMaps(res["skipped"])
	require.Len(t, skipped, 1, "%v", res)
	mainName := e.mustExec("find_foods", fmt.Sprintf(`{"ids":[%d]}`, p.Components[0].FoodID))["items"].([]any)[0].(map[string]any)["name"].(string)
	assert.Contains(t, skipped[0]["why"], "cooking times per day (2 cooking times, at most 1; already cooking that day: ")
	assert.Contains(t, skipped[0]["why"], fmt.Sprintf("%q", mainName))
	assert.Contains(t, skipped[0]["why_de"], "an dem Tag wird schon gekocht: ")
	assert.Contains(t, skipped[0]["why_de"], mainName)

	// A refusal for another rule is not given the cooking list.
	raw, _ := json.Marshal(skipped[0])
	assert.NotContains(t, string(raw), "eine Regel wäre")
}

// R12a C2: a dish that already has a batch on another day is offered as that
// batch with its cooking moved onto the day's cooking time, checked; when the
// move does not pass the check nothing is offered and the next step says so.
func TestApplyChanges_ExistingBatchIsOfferedAsMoveCook(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	e.setCookLimit(1)
	// Main 04 (food 5) cooks on Tue 10 Nov; adding it to Sat 7 Nov lunch would
	// be a second cooking time on Saturday.
	res, err := e.exec("apply_changes", `{"dry_run":true,"ops":[{"op":"add","plate_id":2,"food_id":5,"portions":1}]}`)
	require.NoError(t, err)
	offers := relMaps(res["offers"])
	require.Len(t, offers, 1, "%v", res)
	ops := relMaps(offers[0]["ops"])
	require.Len(t, ops, 2)
	assert.Equal(t, "move_cook", ops[0]["op"])
	assert.EqualValues(t, 7, ops[0]["session_id"])
	assert.Equal(t, "add", ops[1]["op"])
	assert.EqualValues(t, 7, ops[1]["session_id"], "the new meal eats from the batch")
	assert.Contains(t, offers[0]["what"], `cook "Main 04" at Sat 7 Nov breakfast instead of Tue 10 Nov breakfast`)
	assert.Contains(t, offers[0]["what_de"], "„Main 04“ bei Samstag, 7. Nov. Frühstück statt bei Dienstag, 10. Nov. Frühstück kochen")
	assert.NotContains(t, res["next"], "no checked alternative exists")

	// The offer's ops go through as they are.
	raw, err := json.Marshal(map[string]any{"ops": offers[0]["ops"]})
	require.NoError(t, err)
	done, err := e.exec("apply_changes", string(raw))
	require.NoError(t, err)
	assert.NotEqual(t, false, done["saved"], "%v", done)
	assert.Empty(t, done["skipped"])
}

func TestApplyChanges_ExistingBatchOfferNeedsAPassingCheck(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	e.setCookLimit(1)
	// Chili (food 2) cooks on Sat 7 Nov and is eaten that day: moving its
	// cooking to Sun 8 Nov puts the Saturday meals before the cook.
	res, err := e.exec("apply_changes", `{"dry_run":true,"ops":[{"op":"add","plate_id":5,"food_id":2,"portions":1}]}`)
	require.NoError(t, err)
	require.Len(t, relMaps(res["skipped"]), 1, "%v", res)
	assert.Nil(t, res["offers"], "an unchecked offer is never made: %v", res["offers"])
	assert.Contains(t, res["next"], "no checked alternative exists")
}
