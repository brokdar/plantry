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
	"github.com/jaltszeimer/plantry/backend/internal/domain/plate"
)

// mainOf returns the main component of p.
func mainOf(t *testing.T, p plate.Plate) plate.PlateComponent {
	t.Helper()
	for _, c := range p.Components {
		if c.Position == plate.PositionMain {
			return c
		}
	}
	require.Fail(t, "plate has no main")
	return plate.PlateComponent{}
}

// A dry-run copy creates plates that are gone after the rollback: their ids in
// copies[*] are replaced by provisional: true (the real run keeps them).
func TestApplyChanges_DryRunCopyIdsAreProvisional(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.twoDinners()
	lunch := e.slotID("lunch")
	next := p.Date.AddDate(0, 0, 1).Format(time.DateOnly)
	if q := e.plateAtCell(next, lunch); q != nil {
		_, err := e.plates.Delete(context.Background(), q.ID)
		require.NoError(t, err)
	}
	op := fmt.Sprintf(`[{"op":"copy","plate_id":%d,"to":[%s],"as":"fresh"}]`, p.ID, cellJSON(next, lunch))

	dry, real, _ := e.mealOp(op)

	copies := applied0(t, dry)["copies"].([]any)
	require.Len(t, copies, 1)
	c := copies[0].(map[string]any)
	assert.Equal(t, true, c["provisional"], "the dry run's plate does not exist")
	assert.NotContains(t, c, "plate_id")
	assert.Equal(t, next, c["date"])
	assert.EqualValues(t, p.ID, applied0(t, dry)["plate_id"], "the source plate keeps its id")

	realCopy := applied0(t, real)["copies"].([]any)[0].(map[string]any)
	assert.NotContains(t, realCopy, "provisional")
	assert.NotZero(t, realCopy["plate_id"])
}

// away's cells are built as a typed list too; a dry run leaves nothing
// provisional there (cells carry no ids), but the list must survive the walk.
func TestApplyChanges_DryRunAwayKeepsCells(t *testing.T) {
	e := newPlanToolEnv(t)
	day := planStart.AddDate(0, 0, 2).Format(time.DateOnly)
	dry, _, _ := e.mealOp(fmt.Sprintf(`[{"op":"away","date":%q}]`, day))
	cells := applied0(t, dry)["cells"].([]any)
	require.Len(t, cells, 3)
	assert.Equal(t, day, cells[0].(map[string]any)["date"])
}

// A field that is on no op stays refused; a harmless one the op does not take
// is ignored and reported; any other foreign field is refused.
func TestApplyChanges_ForeignFields(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.twoDinners()
	old := mainOf(t, p)
	food := e.unusedFood("main")

	t.Run("redundant plate_id is ignored and reported", func(t *testing.T) {
		dump := e.planDump()
		dry := e.mustExec("apply_changes", fmt.Sprintf(`{"dry_run":true,"ops":[{"op":"swap","plate_component_id":%d,"food_id":%d,"plate_id":%d,"date":%q}]}`, old.ID, food, p.ID, p.DateString()))
		assert.Equal(t, []any{"date", "plate_id"}, applied0(t, dry)["ignored_fields"])
		assert.Equal(t, dump, e.planDump())

		real := e.mustExec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"swap","plate_component_id":%d,"food_id":%d,"plate_id":%d}]}`, old.ID, food, p.ID))
		assert.Equal(t, []any{"plate_id"}, applied0(t, real)["ignored_fields"])
		got, err := e.plates.Get(context.Background(), p.ID)
		require.NoError(t, err)
		assert.Equal(t, food, mainOf(t, *got).FoodID, "the swap happened")
	})

	t.Run("an op that takes no foreign field reports none", func(t *testing.T) {
		q := e.mustExec("apply_changes", fmt.Sprintf(`{"dry_run":true,"ops":[{"op":"set_eaters","plate_id":%d,"eaters":2}]}`, p.ID))
		assert.NotContains(t, applied0(t, q), "ignored_fields")
	})

	for _, field := range []string{`"session_id":5`, `"cook_at":{"date":"2026-11-08","slot_id":1}`, `"portions":2`, `"mode":"none"`, `"position":"main"`, `"to":{"date":"2026-11-08","slot_id":1}`, `"quantity":2`} {
		t.Run("refused "+field, func(t *testing.T) {
			before := e.planDump()
			_, err := e.exec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"remove","plate_component_id":%d,%s}]}`, old.ID, field))
			require.ErrorIs(t, err, domain.ErrInvalidInput)
			assert.Contains(t, err.Error(), "op 1 (remove): unknown field")
			assert.Contains(t, err.Error(), "remove takes plate_component_id")
			assert.Equal(t, before, e.planDump())
		})
	}

	t.Run("swap with session_id is refused", func(t *testing.T) {
		_, err := e.exec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"swap","plate_component_id":%d,"food_id":%d,"session_id":3}]}`, old.ID, food))
		require.ErrorIs(t, err, domain.ErrInvalidInput)
		assert.Contains(t, err.Error(), `op 1 (swap): unknown field "session_id"`)
	})
}

// away with date + slot_id and no cells is that one cell.
func TestApplyChanges_AwayDateAndSlotIsOneCell(t *testing.T) {
	e := newPlanToolEnv(t)
	day := planStart.AddDate(0, 0, 2).Format(time.DateOnly)
	lunch, dinner := e.slotID("lunch"), e.slotID("dinner")
	dump := e.planDump()

	_, real, ch := e.mealOp(fmt.Sprintf(`[{"op":"away","date":%q,"slot_id":%d}]`, day, lunch))

	cells := applied0(t, real)["cells"].([]any)
	require.Len(t, cells, 1)
	assert.EqualValues(t, lunch, cells[0].(map[string]any)["slot_id"])
	assert.NotContains(t, applied0(t, real), "ignored_fields")
	assert.Nil(t, e.plateAtCell(day, lunch))
	assert.NotNil(t, e.plateAtCell(day, dinner), "the other cells of the day stay")
	e.undoRestores(ch, dump)

	// A slot_id next to cells is refused.
	_, err := e.exec("apply_changes", fmt.Sprintf(`{"ops":[{"op":"away","cells":[%s],"slot_id":%d}]}`, cellJSON(day, lunch), lunch))
	require.ErrorIs(t, err, domain.ErrInvalidInput)
}

// Copying FROM a meal that is already past is allowed (repeat last week's
// meal); the targets stay guarded.
func TestApplyChanges_CopyFromPastMeal(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.twoDinners()
	lunch := e.slotID("lunch")
	target := p.Date.AddDate(0, 0, 4).Format(time.DateOnly)
	if q := e.plateAtCell(target, lunch); q != nil {
		_, err := e.plates.Delete(context.Background(), q.ID)
		require.NoError(t, err)
	}
	// The chat's today is after the source plate, before the target.
	ctx := agent.WithToday(e.ctx, p.Date.AddDate(0, 0, 2))
	in := fmt.Sprintf(`{"ops":[{"op":"copy","plate_id":%d,"to":[%s],"as":"fresh"}]}`, p.ID, cellJSON(target, lunch))

	out, _, _, err := e.ts.Execute(ctx, "apply_changes", []byte(in))
	require.NoError(t, err, "a past source is fine")
	assert.Contains(t, string(out), "copies")
	require.NotNil(t, e.plateAtCell(target, lunch))

	// A past target is refused.
	past := cellJSON(p.DateString(), e.slotID("breakfast"))
	_, _, _, err = e.ts.Execute(ctx, "apply_changes", []byte(fmt.Sprintf(`{"ops":[{"op":"copy","plate_id":%d,"to":[%s],"as":"fresh"}]}`, p.ID, past)))
	require.ErrorIs(t, err, domain.ErrAIPastDate)
}

// A change line lists the exact cells the board pulses ("pulse"): a move's
// source and target (and the occupant's cell), an away cell, copy targets.
func TestApplyChanges_LinesListPulseCells(t *testing.T) {
	cellsOf := func(ch *agent.Change) [][2]any {
		var out [][2]any
		for _, op := range opsOf(ch) {
			for _, c := range op["pulse"].([]any) {
				m := c.(map[string]any)
				out = append(out, [2]any{m["date"], m["slot_id"]})
			}
		}
		return out
	}

	t.Run("move with occupant to a cell", func(t *testing.T) {
		e := newPlanToolEnv(t)
		p, q := e.twoDinners()
		lunch := e.slotID("lunch")
		if r := e.plateAtCell(q.DateString(), lunch); r != nil {
			_, err := e.plates.Delete(context.Background(), r.ID)
			require.NoError(t, err)
		}
		_, _, ch := e.mealOp(fmt.Sprintf(`[{"op":"move","plate_id":%d,"to":%s,"occupant":%s}]`, p.ID, cellJSON(q.DateString(), q.SlotID), cellJSON(q.DateString(), lunch)))
		assert.Equal(t, [][2]any{
			{p.DateString(), float64(p.SlotID)}, {q.DateString(), float64(q.SlotID)}, {q.DateString(), float64(lunch)},
		}, cellsOf(ch))
	})

	t.Run("away date + slot is one cell", func(t *testing.T) {
		e := newPlanToolEnv(t)
		day := planStart.AddDate(0, 0, 2).Format(time.DateOnly)
		lunch := e.slotID("lunch")
		_, _, ch := e.mealOp(fmt.Sprintf(`[{"op":"away","date":%q,"slot_id":%d}]`, day, lunch))
		assert.Equal(t, [][2]any{{day, float64(lunch)}}, cellsOf(ch))
	})

	t.Run("copy targets", func(t *testing.T) {
		e := newPlanToolEnv(t)
		p, _ := e.twoDinners()
		lunch := e.slotID("lunch")
		next := p.Date.AddDate(0, 0, 1).Format(time.DateOnly)
		if q := e.plateAtCell(next, lunch); q != nil {
			_, err := e.plates.Delete(context.Background(), q.ID)
			require.NoError(t, err)
		}
		_, _, ch := e.mealOp(fmt.Sprintf(`[{"op":"copy","plate_id":%d,"to":[%s],"as":"fresh"}]`, p.ID, cellJSON(next, lunch)))
		assert.Equal(t, [][2]any{{next, float64(lunch)}}, cellsOf(ch))
	})

	t.Run("delete_plate", func(t *testing.T) {
		e := newPlanToolEnv(t)
		p, _ := e.twoDinners()
		_, _, ch := e.mealOp(fmt.Sprintf(`[{"op":"delete_plate","plate_id":%d}]`, p.ID))
		assert.Equal(t, [][2]any{{p.DateString(), float64(p.SlotID)}}, cellsOf(ch))
	})
}

// A change written before a change could hold several cells stores the single
// cell object in availability_before_json; its undo still restores the cell.
func TestApplyChanges_UndoLegacySingleCellChange(t *testing.T) {
	e := newPlanToolEnv(t)
	p, _ := e.twoDinners()
	dump := e.planDump()
	ctx, _ := e.chat()
	ch := e.change(ctx, "apply_changes", fmt.Sprintf(`{"ops":[{"op":"set_availability","date":%q,"slot_id":%d,"mode":"none","note":"out"}]}`, p.DateString(), p.SlotID))
	require.Len(t, ch.Cells, 1)
	require.NotEqual(t, dump, e.planDump(), "the change wrote")

	// Rewrite the stored list as the legacy single object.
	e.sql(`UPDATE ai_changes SET availability_before_json = json_extract(availability_before_json, '$[0]') WHERE id = ?`, ch.ID)
	var raw string
	require.NoError(t, e.db.QueryRow(`SELECT availability_before_json FROM ai_changes WHERE id = ?`, ch.ID).Scan(&raw))
	require.Equal(t, byte('{'), raw[0], "stored as a single object")
	stored, err := sqliteChange(e, ch.ID)
	require.NoError(t, err)
	require.Len(t, stored.Cells, 1)

	e.undoRestores(ch, dump)
}
