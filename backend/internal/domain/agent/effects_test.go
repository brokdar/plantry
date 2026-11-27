package agent_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

// Mutating tools return the state after the write, so the model need not
// call get_plan to verify.
func TestTool_MutationsReturnWhatChanged(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	p, sid := e.mainSession()
	v := e.session(sid)

	out := e.mustOp("set_session_portions", fmt.Sprintf(`{"session_id":%d,"portions":%v}`, sid, v.Consumed+2))
	after, ok := out["after"].(map[string]any)
	require.True(t, ok, "set_session_portions carries an after block: %v", out)
	sessions := after["batches"].([]any)
	require.Len(t, sessions, 1)
	s0 := sessions[0].(map[string]any)
	assert.EqualValues(t, sid, s0["session_id"])
	assert.InDelta(t, v.Consumed+2, s0["portions"], 1e-9)
	assert.InDelta(t, v.Consumed, s0["consumed"], 1e-9)
	assert.InDelta(t, 2, s0["surplus"], 1e-9)
	assert.NotEmpty(t, s0["food"])
	assert.Contains(t, after, "issues")

	out = e.mustOp("set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":2}`, p.ID))
	plates := out["after"].(map[string]any)["plates"].([]any)
	require.Len(t, plates, 1)
	pl := plates[0].(map[string]any)
	assert.EqualValues(t, p.ID, pl["plate_id"])
	assert.Equal(t, 2.0, pl["eaters"])
	assert.NotEmpty(t, pl["components"])

	comp := p.Components[0]
	out = e.mustOp("swap_food", fmt.Sprintf(`{"plate_component_id":%d,"new_food_id":%d}`, comp.ID, comp.FoodID))
	assert.Contains(t, out, "after")
	assert.NotEmpty(t, out["name"])
}

// Deficit guard (spec §3.4, H3): fewer portions than the plates eat is
// refused with the numbers, and nothing is written.
func TestTool_SetSessionPortionsRefusesDeficit(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	_, sid := e.mainSession()
	v := e.session(sid)
	require.Greater(t, v.Consumed, 1.0, "fixture: the batch feeds more than one portion")

	_, err := e.op("set_session_portions", fmt.Sprintf(`{"session_id":%d,"portions":%v}`, sid, v.Consumed-1))
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Contains(t, err.Error(), fmt.Sprintf("eat %v", v.Consumed))
	assert.InDelta(t, v.Portions, e.session(sid).Portions, 1e-9, "a refused call writes nothing")

	e.mustOp("set_session_portions", fmt.Sprintf(`{"session_id":%d,"portions":%v}`, sid, v.Consumed))
}

// The chat prompt shows H3/H4 (a batch eaten 8 of 6) and each food's role and
// kcal with the day's kcal.
func TestChatPrompt_ShowsDeficitAndFacts(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	_, sid := e.mainSession()
	v := e.session(sid)
	e.sql(`UPDATE cook_sessions SET portions = ? WHERE id = ?`, v.Consumed-1, sid)
	st, err := testsvc.PlanningDeps(e.db).Builder.Build(e.ctx, window.Window{Start: planStart, End: planStart.AddDate(0, 0, 6)}, planToday, planStart, planStart.AddDate(0, 0, 6))
	require.NoError(t, err)

	var rules []string
	for _, is := range agent.ChatIssues(st) {
		rules = append(rules, is.Rule)
	}
	assert.Contains(t, rules, "H3")

	facts := agent.ChatFacts(st)
	assert.Contains(t, facts, "| main |")
	assert.Contains(t, facts, "Kcal per eater and day")
}

// A confirmed plate of a hidden substitution variant 4 weeks ago, with
// nothing newer, counts for its recipe's 8-week count.
func TestBuilder_OldHiddenVariantPlateCountsForItsRecipe(t *testing.T) {
	t.Parallel()
	e := newPlanToolEnv(t)
	// The keep-days lookback must not reach the plate (freezing stretches it).
	require.NoError(t, testsvc.Settings(e.db).Set(context.Background(), "plan.freezing_enabled", "false"))
	var chili int64
	require.NoError(t, e.db.QueryRow(`SELECT id FROM foods WHERE name = 'Chili'`).Scan(&chili))
	variant := e.insert(`INSERT INTO foods (name, kind, role, reference_portions, archived) VALUES ('Chili (Schalotte)', 'composed', 'main', 4, 1)`)
	e.sql(`INSERT INTO hidden_variants (food_id, base_food_id) VALUES (?, ?)`, variant, chili)
	slotID := e.insert(`INSERT INTO time_slots (name_key, icon, sort_order, active) VALUES ('slot.snack', 'moon', 9, 1)`)
	p := e.insert(`INSERT INTO plates (slot_id, date, state) VALUES (?, ?, 'confirmed')`, slotID, planToday.AddDate(0, 0, -28).Format(time.DateOnly))
	cookDate := planToday.AddDate(0, 0, -28).Format(time.DateOnly)
	sess := e.insert(`INSERT INTO cook_sessions (food_id, cook_date, cook_slot_id, portions) VALUES (?, ?, ?, 1)`, variant, cookDate, slotID)
	e.sql(`INSERT INTO plate_components (plate_id, food_id, portions, cook_session_id, sort_order) VALUES (?, ?, 1, ?, 0)`, p, variant, sess)

	st, err := testsvc.PlanningDeps(e.db).Builder.Build(e.ctx, window.Window{Start: planStart, End: planStart.AddDate(0, 0, 6)}, planToday, planStart, planStart.AddDate(0, 0, 6))
	require.NoError(t, err)
	assert.Equal(t, 1, st.Foods[chili].TimesEaten8Weeks)
}
