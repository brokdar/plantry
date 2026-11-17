package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/planning"
	"github.com/jaltszeimer/plantry/backend/internal/domain/window"
	"github.com/jaltszeimer/plantry/backend/internal/testhelper/testsvc"
)

func (e *planToolEnv) lightOf(ctx context.Context, date string) string {
	e.t.Helper()
	out, _, _, err := e.ts.Execute(ctx, "get_plan", json.RawMessage(`{}`))
	require.NoError(e.t, err)
	var res struct {
		Evaluation struct {
			Days []struct {
				Date  string `json:"date"`
				Light string `json:"light"`
			} `json:"days"`
		} `json:"evaluation"`
	}
	require.NoError(e.t, json.Unmarshal(out, &res))
	for _, d := range res.Evaluation.Days {
		if d.Date == date {
			return d.Light
		}
	}
	return ""
}

// A light request is kept: the next call, which does not repeat it, still
// evaluates the day as light (and the state the chat prompt is built from
// carries it); a none entry clears it.
func TestApplyChanges_LightIsStoredAndCleared(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	p, _ := e.freshMain()
	date := p.DateString()
	op := fmt.Sprintf(`"ops":[{"op":"set_eaters","plate_id":%d,"eaters":2}]`, p.ID)

	assert.Empty(t, e.lightOf(ctx, date))
	_, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(fmt.Sprintf(`{"light":[{"date":%q,"scope":"day"}],%s}`, date, op)))
	require.NoError(t, err)
	assert.Equal(t, "day", e.lightOf(ctx, date), "turn 2 without light still evaluates the light day")
	st, err := testsvc.PlanState(e.db)(ctx, window.Window{Start: planStart, End: planStart.AddDate(0, 0, 6)}, planToday)
	require.NoError(t, err)
	assert.Equal(t, []planning.LightIntent{{Date: date, Scope: "day"}}, st.Light)

	_, _, _, err = e.ts.Execute(ctx, "apply_changes", json.RawMessage(fmt.Sprintf(`{"light":[{"date":%q,"scope":"none"}],"ops":[{"op":"set_eaters","plate_id":%d,"eaters":1}]}`, date, p.ID)))
	require.NoError(t, err)
	assert.Empty(t, e.lightOf(ctx, date), "scope none clears the stored request")
}

func TestApplyChanges_LightDryRunStoresNothing(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	p, _ := e.freshMain()
	_, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(fmt.Sprintf(`{"dry_run":true,"light":[{"date":%q,"scope":"day"}],"ops":[{"op":"set_eaters","plate_id":%d,"eaters":2}]}`, p.DateString(), p.ID)))
	require.NoError(t, err)
	assert.Empty(t, e.lightOf(ctx, p.DateString()))
}

// A request the plan cannot honour goes back to the model.
func TestApplyChanges_LightRejected(t *testing.T) {
	e := newPlanToolEnv(t)
	ctx, _ := e.chat()
	p, _ := e.freshMain()
	op := fmt.Sprintf(`"ops":[{"op":"set_eaters","plate_id":%d,"eaters":2}]`, p.ID)
	day := func(n int) string { return planStart.AddDate(0, 0, n).Format("2006-01-02") }

	four := fmt.Sprintf(`{"dry_run":true,"light":[{"date":%q,"scope":"day"},{"date":%q,"scope":"day"},{"date":%q,"scope":"day"},{"date":%q,"scope":"day"}],%s}`, day(0), day(1), day(2), day(3), op)
	_, _, _, err := e.ts.Execute(ctx, "apply_changes", json.RawMessage(four))
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Contains(t, err.Error(), "lose", "the message points to the goal")

	three := fmt.Sprintf(`{"light":[{"date":%q,"scope":"day"},{"date":%q,"scope":"day"},{"date":%q,"scope":"day"}],%s}`, day(0), day(1), day(2), op)
	_, _, _, err = e.ts.Execute(ctx, "apply_changes", json.RawMessage(three))
	require.NoError(t, err)
	more := fmt.Sprintf(`{"dry_run":true,"light":[{"date":%q,"scope":"day"}],%s}`, day(3), op)
	_, _, _, err = e.ts.Execute(ctx, "apply_changes", json.RawMessage(more))
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "the cap counts the stored days")
	meal := fmt.Sprintf(`{"dry_run":true,"light":[{"date":%q,"scope":"meal","slot":"dinner"}],%s}`, day(3), op)
	_, _, _, err = e.ts.Execute(ctx, "apply_changes", json.RawMessage(meal))
	assert.NoError(t, err, "meal intents are not capped")

	outside := fmt.Sprintf(`{"dry_run":true,"light":[{"date":%q,"scope":"day"}],%s}`, day(20), op)
	_, _, _, err = e.ts.Execute(ctx, "apply_changes", json.RawMessage(outside))
	assert.ErrorIs(t, err, domain.ErrInvalidInput, "a date outside the window")
}
