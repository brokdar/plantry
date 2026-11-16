package agent_test

import (
	"fmt"
	"testing"

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
	e := newPlanToolEnv(t)
	p, sid := e.mainSession()
	v := e.session(sid)

	out := e.mustOp("set_session_portions", fmt.Sprintf(`{"session_id":%d,"portions":%v}`, sid, v.Consumed+2))
	after, ok := out["after"].(map[string]any)
	require.True(t, ok, "set_session_portions carries an after block: %v", out)
	sessions := after["sessions"].([]any)
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
