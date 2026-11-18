package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jaltszeimer/plantry/backend/internal/domain"
	"github.com/jaltszeimer/plantry/backend/internal/domain/agent"
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// undoLast runs undo_last_change in the chat turn of conv.
func (e *planToolEnv) undoLast(ctx context.Context, conv int64, input string) map[string]any {
	e.t.Helper()
	out, _, _, err := e.ts.Execute(agent.WithTurn(ctx, conv), "undo_last_change", json.RawMessage(input))
	require.NoError(e.t, err)
	var m map[string]any
	require.NoError(e.t, json.Unmarshal(out, &m))
	return m
}

func lines(m map[string]any) []string {
	var out []string
	for _, l := range m["restored"].([]any) {
		out = append(out, l.(string))
	}
	return out
}

// A move that swaps two dinners is undone exactly (plates, components, the
// batch links) and said in plain words; undone once, it is not undoable again,
// neither by the tool nor by the HTTP undo.
func TestUndoLastChange_MoveSwapRestoresExactly(t *testing.T) {
	e := newPlanToolEnv(t)
	p, q := e.twoDinners()
	dump := e.planDump()
	ctx, conv := e.chat()
	ch := e.change(ctx, "apply_changes", fmt.Sprintf(`{"ops":[{"op":"move","plate_id":%d,"to":%s,"occupant":"swap"}]}`, p.ID, cellJSON(q.DateString(), q.SlotID)))
	require.NotEqual(t, dump, e.planDump())

	res := e.undoLast(ctx, conv, `{}`)
	assert.EqualValues(t, 1, res["undone"])
	assert.Nil(t, res["refused"])
	assert.Equal(t, dump, e.planDump(), "the plan is exactly as before")
	got := lines(res)
	require.NotEmpty(t, got)
	assert.Contains(t, got[0], "dinner: ")
	t.Log(res["message"], got)

	_, err := e.changes.Undo(context.Background(), ch.ID)
	assert.ErrorIs(t, err, domain.ErrAIChangeUndone, "the board's undo sees the same record")
	again := e.undoLast(ctx, conv, `{}`)
	assert.EqualValues(t, 0, again["undone"])
	assert.Contains(t, again["message"], "Nothing to undo")
	assert.Equal(t, dump, e.planDump())
}

// count 2 undoes the two newest changes, newest first; an undone change is
// skipped. Two changes of the same meal cannot both come back exactly: the
// older one is refused with its reason.
func TestUndoLastChange_CountAndSkipsUndone(t *testing.T) {
	e := newPlanToolEnv(t)
	a, b := e.twoDinners()
	dump := e.planDump()
	ctx, conv := e.chat()
	e.changeOp(ctx, "set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":3}`, a.ID))
	mid := e.planDump()
	second := e.changeOp(ctx, "set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":2}`, b.ID))

	_, err := e.changes.Undo(context.Background(), second.ID)
	require.NoError(t, err, "undone on the board")
	assert.Equal(t, mid, e.planDump())
	res := e.undoLast(ctx, conv, `{"count":2}`)
	assert.EqualValues(t, 1, res["undone"], "only one change is left")
	assert.Equal(t, dump, e.planDump())

	e.changeOp(ctx, "set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":3}`, a.ID))
	e.changeOp(ctx, "set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":2}`, b.ID))
	res = e.undoLast(ctx, conv, `{"count":2}`)
	assert.EqualValues(t, 2, res["undone"])
	assert.Nil(t, res["refused"])
	assert.Equal(t, dump, e.planDump())

	e.changeOp(ctx, "set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":3}`, a.ID))
	e.changeOp(ctx, "set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":2}`, a.ID))
	res = e.undoLast(ctx, conv, `{"count":2}`)
	assert.EqualValues(t, 1, res["undone"])
	assert.Contains(t, res["refused"].(map[string]any)["reason"], "shares meals")
}

// A newer edit of the plan refuses the undo with a reason and writes nothing.
func TestUndoLastChange_StaleIsRefused(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.mainSession()
	ctx, conv := e.chat()
	ch := e.changeOp(ctx, "set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":3}`, p.ID))
	four := 4.0
	_, err := e.plates.Update(context.Background(), p.ID, plate.PlatePatch{Eaters: &four})
	require.NoError(t, err)
	dump, n := e.planDump(), e.eventCount()

	res := e.undoLast(ctx, conv, `{}`)
	assert.EqualValues(t, 0, res["undone"])
	refused := res["refused"].(map[string]any)
	assert.Contains(t, refused["reason"], "changed since")
	assert.NotEmpty(t, refused["what_to_do"])
	assert.Equal(t, dump, e.planDump())
	assert.Equal(t, n, e.eventCount())
	var undone *string
	require.NoError(t, e.db.QueryRow(`SELECT undone_at FROM ai_changes WHERE id = ?`, ch.ID).Scan(&undone))
	assert.Nil(t, undone)
}

// A confirmed meal refuses the undo and points at unlock.
func TestUndoLastChange_ConfirmedIsRefused(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.mainSession()
	ctx, conv := e.chat()
	e.changeOp(ctx, "set_plate_eaters", fmt.Sprintf(`{"plate_id":%d,"eaters":3}`, p.ID))
	_, err := e.plates.Confirm(context.Background(), plate.RowSet{PlateIDs: []int64{p.ID}})
	require.NoError(t, err)

	res := e.undoLast(ctx, conv, `{}`)
	assert.EqualValues(t, 0, res["undone"])
	assert.Contains(t, res["refused"].(map[string]any)["what_to_do"], "unlock")
}
